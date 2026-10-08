package main

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/grafana/generate-policy-bot-config/internal"
	"github.com/jessevdk/go-flags"
	"github.com/palantir/policy-bot/policy"
	"github.com/palantir/policy-bot/policy/approval"
	"github.com/palantir/policy-bot/policy/common"
	"github.com/palantir/policy-bot/policy/predicate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func mustRegexps(t *testing.T, patterns ...string) []common.Regexp {
	t.Helper()

	result := make([]common.Regexp, len(patterns))
	for i, pattern := range patterns {
		re, err := common.NewRegexp(pattern)
		require.NoError(t, err)

		result[i] = re
	}

	return result
}

func TestParseFlags(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		expected    appFlags
		expectError bool
	}{
		{
			name: "Output to file",
			args: []string{"-o", "output.yml", "testdir"},
			expected: appFlags{
				Output: output{path: "output.yml"},
				Args:   rootArgs{Root: rootDir{os.DirFS("testdir")}},
			},
		},
		{
			name: "Output to stdout",
			args: []string{"-o", "-", "testdir"},
			expected: appFlags{
				Output: output{stdout: true},
				Args:   rootArgs{Root: rootDir{os.DirFS("testdir")}},
			},
		},
		{
			name: "Default output",
			args: []string{"testdir"},
			expected: appFlags{
				Output: output{path: ".policy.yml"},
				Args:   rootArgs{Root: rootDir{os.DirFS("testdir")}},
			},
		},
		{
			name: "Merge config from file",
			args: []string{"-m", "merge.yml", "testdir"},
			expected: appFlags{
				Output:    output{path: ".policy.yml"},
				MergeWith: mergeSource{path: "merge.yml"},
				Args:      rootArgs{Root: rootDir{os.DirFS("testdir")}},
			},
		},
		{
			name: "Merge config from stdin",
			args: []string{"-m", "-", "testdir"},
			expected: appFlags{
				Output:    output{path: ".policy.yml"},
				MergeWith: mergeSource{stdin: true},
				Args:      rootArgs{Root: rootDir{os.DirFS("testdir")}},
			},
		},
		{
			name:        "Missing directory",
			args:        []string{"-o", "output.yml"},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var conf appFlags
			parser := flags.NewParser(&conf, flags.Default)
			_, err := parser.ParseArgs(tt.args)

			if tt.expectError {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.expected, conf)
		})
	}
}

// mainArgsEnv is set when TestMainParseErrors runs the test binary again to
// call main, which exits the process. It contains the command-line arguments.
const mainArgsEnv = "GENERATE_POLICY_BOT_CONFIG_MAIN_ARGS"

func TestMainParseErrors(t *testing.T) {
	if args, ok := os.LookupEnv(mainArgsEnv); ok {
		os.Args = append([]string{"generate-policy-bot-config"}, strings.Fields(args)...)
		main()
		return
	}

	// usage is the start of the help text, up to the first blank line.
	type result struct {
		exitCode int
		usage    string
		stderr   string
	}

	tests := []struct {
		name     string
		args     string
		expected result
	}{
		{
			name:     "Help",
			args:     "--help",
			expected: result{exitCode: 0, usage: "Usage:\n  generate-policy-bot-config [OPTIONS] REPO_ROOT"},
		},
		{
			name:     "Missing directory",
			args:     "-o -",
			expected: result{exitCode: 1, stderr: "the required argument `REPO_ROOT` was not provided\n"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := exec.Command(os.Args[0], "-test.run=^TestMainParseErrors$")
			cmd.Env = append(os.Environ(), mainArgsEnv+"="+tt.args)

			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr

			exitCode := 0
			err := cmd.Run()

			var exitErr *exec.ExitError
			if errors.As(err, &exitErr) {
				exitCode = exitErr.ExitCode()
				err = nil
			}
			require.NoError(t, err)

			usage, _, _ := strings.Cut(stdout.String(), "\n\n")

			require.Equal(t, tt.expected, result{
				exitCode: exitCode,
				usage:    usage,
				stderr:   stderr.String(),
			})
		})
	}
}

func TestOutputWrite(t *testing.T) {
	errGenerate := errors.New("fake generate error")
	errRename := errors.New("fake rename error")
	errCreateTemp := errors.New("fake create temp error")

	// result is what write leaves behind: the files in the filesystem and what
	// was written to standard output.
	type result struct {
		files  map[string]string
		stdout string
	}

	tests := []struct {
		name          string
		output        output
		fsys          *internal.MemFS
		generateErr   error
		expected      result
		expectedError error
	}{
		{
			name:     "Standard output",
			output:   output{stdout: true},
			fsys:     &internal.MemFS{},
			expected: result{stdout: "content"},
		},
		{
			name:     "File",
			output:   output{path: "out/policy.yml"},
			fsys:     &internal.MemFS{},
			expected: result{files: map[string]string{"out/policy.yml": "content"}},
		},
		{
			name:          "Generate error removes the temporary file",
			output:        output{path: "out/policy.yml"},
			fsys:          &internal.MemFS{},
			generateErr:   errGenerate,
			expected:      result{files: map[string]string{}},
			expectedError: errGenerate,
		},
		{
			name:          "Rename error",
			output:        output{path: "out/policy.yml"},
			fsys:          &internal.MemFS{RenameErr: errRename},
			expected:      result{files: map[string]string{}},
			expectedError: errRename,
		},
		{
			name:          "Create temp error",
			output:        output{path: "out/policy.yml"},
			fsys:          &internal.MemFS{CreateTempErr: errCreateTemp},
			expectedError: errCreateTemp,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdout := &bytes.Buffer{}

			err := tt.output.write(tt.fsys, stdout, func(w io.Writer) error {
				_, err := io.WriteString(w, "content")
				require.NoError(t, err)

				return tt.generateErr
			})

			require.ErrorIs(t, err, tt.expectedError)
			require.Equal(t, tt.expected, result{files: tt.fsys.Files, stdout: stdout.String()})
		})
	}
}

func TestMergeSourceOpen(t *testing.T) {
	stdin := strings.NewReader("from stdin")
	mergeFile := strings.NewReader("from file")
	openFile := func(name string) (io.Reader, error) {
		if name != "merge.yml" {
			return nil, fs.ErrNotExist
		}

		return mergeFile, nil
	}

	tests := []struct {
		name          string
		source        mergeSource
		expected      reader
		expectedError error
	}{
		{
			name:     "No merge config",
			source:   mergeSource{},
			expected: reader{},
		},
		{
			name:     "Standard input",
			source:   mergeSource{stdin: true},
			expected: reader{Reader: stdin},
		},
		{
			name:     "File",
			source:   mergeSource{path: "merge.yml"},
			expected: reader{Reader: mergeFile, filename: "merge.yml"},
		},
		{
			name:          "Missing file",
			source:        mergeSource{path: "missing.yml"},
			expectedError: fs.ErrNotExist,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			merge, err := tt.source.open(openFile, stdin)

			require.ErrorIs(t, err, tt.expectedError)
			require.Equal(t, tt.expected, merge)
		})
	}
}

func TestListWorkflows(t *testing.T) {
	mapFS := fstest.MapFS{
		".github/workflows/workflow1.yml":      &fstest.MapFile{Data: []byte("")},
		".github/workflows/workflow2.yml":      &fstest.MapFile{Data: []byte("")},
		".github/workflows/workflow3.yaml":     &fstest.MapFile{Data: []byte("")},
		".github/workflows/not-a-workflow.txt": &fstest.MapFile{Data: []byte("")},
	}

	conf := appFlags{Args: rootArgs{Root: rootDir{mapFS}}}

	workflows, err := conf.listWorkflows()
	require.NoError(t, err)

	require.ElementsMatch(t, workflows, []string{".github/workflows/workflow1.yml", ".github/workflows/workflow2.yml", ".github/workflows/workflow3.yaml"})
}

func TestParsePRWorkflows(t *testing.T) {
	mapFS := fstest.MapFS{
		".github/workflows/pr_workflow.yml": &fstest.MapFile{Data: []byte(`
on:
  pull_request:
    paths: ["src/**"]
`)},
		".github/workflows/non_pr_workflow.yml": &fstest.MapFile{Data: []byte(`
on:
  push:
    branches: ["main"]
`)},
	}

	conf := appFlags{Args: rootArgs{Root: rootDir{mapFS}}}

	workflows, err := conf.parsePRWorkflows()
	require.NoError(t, err)

	require.Len(t, workflows, 1)
	require.Contains(t, workflows, ".github/workflows/pr_workflow.yml")
	require.NotContains(t, workflows, ".github/workflows/non_pr_workflow.yml")
}

func TestRun(t *testing.T) {
	tests := []struct {
		name           string
		workflowConfig string
		expectedConfig policy.Config
	}{
		{
			name: "Valid workflow",
			workflowConfig: `
on:
  pull_request:
    paths: ["src/**"]
`,
			expectedConfig: policy.Config{
				Policy: policy.Policy{
					Approval: approval.Policy(
						[]interface{}{
							map[string]interface{}{
								"or": []interface{}{
									map[string]interface{}{
										"and": []interface{}{
											"Workflow .github/workflows/workflow.yml succeeded or skipped",
											internal.DefaultToApproval,
										},
									},
								},
							},
						},
					),
				},
				ApprovalRules: []*approval.Rule{
					{
						Name: "Workflow .github/workflows/workflow.yml succeeded or skipped",
						Predicates: predicate.Predicates{
							ChangedFiles: &predicate.ChangedFiles{
								Paths: mustRegexps(t, "^src/.*$"),
							},
							FileNotDeleted: &predicate.FileNotDeleted{
								Paths: mustRegexps(t, `^\.github/workflows/workflow\.yml$`),
							},
						},
						Requires: approval.Requires{
							Conditions: predicate.Predicates{
								HasWorkflowResult: &predicate.HasWorkflowResult{
									Conclusions: internal.SkippedOrSuccess,
									Workflows:   []string{".github/workflows/workflow.yml"},
								},
							},
						},
					},
					{
						Name: internal.DefaultToApproval,
					},
				},
			},
		},
		{
			name: "Unsupported event",
			workflowConfig: `
on:
  invalid_event:
    paths: ["src/**"]
`,
			expectedConfig: policy.Config{},
		},
		{
			name: "Invalid yaml",
			workflowConfig: `
on:
  pull_request:
    paths: ["src/**"]
	  pull_request_target:
`,
			expectedConfig: policy.Config{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mapFS := fstest.MapFS{
				".github/workflows/workflow.yml": &fstest.MapFile{Data: []byte(tt.workflowConfig)},
			}

			outputBuffer := &bytes.Buffer{}
			conf := appFlags{Args: rootArgs{Root: rootDir{mapFS}}}

			err := conf.run("test-command", reader{}, outputBuffer)

			require.NoError(t, err)

			output := outputBuffer.String()
			require.Contains(t, output, "# This file is generated by test-command.")

			var parsedPolicy policy.Config
			err = yaml.Unmarshal(outputBuffer.Bytes(), &parsedPolicy)
			require.NoError(t, err)

			require.Equal(t, tt.expectedConfig, parsedPolicy)
		})
	}
}

func BenchmarkRun(b *testing.B) {
	mapFS := fstest.MapFS{
		".github/workflows/workflow.yml": &fstest.MapFile{Data: []byte(`
on:
  pull_request:
    paths: src/**
`)},
		".github/workflows/workflow2.yml": &fstest.MapFile{Data: []byte(`
on:
  pull_request:
    paths: ["src/**"]
    paths-ignore: ["docs/**"]
`)},
		".github/workflows/workflow3.yml": &fstest.MapFile{Data: []byte(`
on:
  workflow_dispatch:
`)},
		".github/workflows/workflow4.yml": &fstest.MapFile{Data: []byte(`
on:
  pull_request_target:
    branches: ["release/*"]
    paths: ["config/**"]
    paths-ignore: ["README.md"]
`)},
		".github/workflows/workflow5.yml": &fstest.MapFile{Data: []byte(`
on:
  pull_request:
    branches: ["main", "develop"]
    paths: ["src/**"]
    paths-ignore: ["docs/**"]

  pull_request_target:
    branches: ["release/*"]
    paths: ["config/**"]
    paths-ignore: ["README.md"]
`)},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf := &bytes.Buffer{}

		conf := appFlags{Args: rootArgs{Root: rootDir{mapFS}}}

		err := conf.run("test-command", reader{}, buf)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func baseConfig(t *testing.T) policy.Config {
	t.Helper()

	const workflowYml = `
on:
  pull_request:
    paths: ["src/**"]
`

	mapFS := fstest.MapFS{
		".github/workflows/workflow.yml": &fstest.MapFile{Data: []byte(workflowYml)},
	}

	flags := appFlags{
		Args: rootArgs{Root: rootDir{mapFS}},
	}
	workflows, err := flags.parsePRWorkflows()
	require.NoError(t, err)
	return workflows.PolicyBotConfig()
}

func expectedConfig(t *testing.T) policy.Config {
	t.Helper()

	return policy.Config{
		Policy: policy.Policy{
			Approval: approval.Policy{
				map[string]interface{}{
					"or": []interface{}{
						map[string]interface{}{
							"and": []interface{}{
								"Workflow .github/workflows/workflow.yml succeeded or skipped",
								internal.DefaultToApproval,
							},
						},
					},
				},
				map[string]interface{}{
					"or": []interface{}{"custom_rule"},
				},
			},
		},
		ApprovalRules: []*approval.Rule{
			{
				Name: "Workflow .github/workflows/workflow.yml succeeded or skipped",
				Predicates: predicate.Predicates{
					ChangedFiles: &predicate.ChangedFiles{
						Paths: mustRegexps(t, "^src/.*$"),
					},
					FileNotDeleted: &predicate.FileNotDeleted{
						Paths: mustRegexps(t, `^\.github/workflows/workflow\.yml$`),
					},
				},
				Requires: approval.Requires{
					Conditions: predicate.Predicates{
						HasWorkflowResult: &predicate.HasWorkflowResult{
							Conclusions: internal.SkippedOrSuccess,
							Workflows:   []string{".github/workflows/workflow.yml"},
						},
					},
				},
			},
			{Name: internal.DefaultToApproval},
			{Name: "custom_rule"},
		},
	}
}

func TestRunWithMerge(t *testing.T) {
	// Common setup
	mapFS := fstest.MapFS{
		".github/workflows/workflow.yml": &fstest.MapFile{Data: []byte(`
on:
  pull_request:
    paths: ["src/**"]
`)},
	}

	baseConfig := baseConfig(t)
	mergeConfigBytes := []byte(`
policy:
  approval:
    - or:
      - custom_rule

approval_rules:
  - name: custom_rule
`)

	expectedConfig := expectedConfig(t)

	// Helper function to create a config reader
	createReader := func(data []byte) reader {
		return reader{Reader: bytes.NewReader(data), filename: "merge.yml"}
	}

	// Helper function to create a fake stdin reader
	createFakeStdinReader := func(data []byte) reader {
		r, w, _ := os.Pipe()
		_, _ = w.Write(data)
		w.Close()
		return reader{Reader: r}
	}

	// Test cases
	tests := []struct {
		name           string
		mergeReader    reader
		expectedConfig policy.Config
		expectedError  error
	}{
		{
			name:           "Merge with custom config",
			mergeReader:    createReader(mergeConfigBytes),
			expectedConfig: expectedConfig,
		},
		{
			name:           "Merge with fake stdin",
			mergeReader:    createFakeStdinReader(mergeConfigBytes),
			expectedConfig: expectedConfig,
		},
		{
			name:          "Merge with invalid config",
			mergeReader:   createReader([]byte("invalid yaml")),
			expectedError: internal.ErrInvalidPolicyBotConfig{},
		},
		{
			name:           "No merge config (nil reader)",
			expectedConfig: baseConfig,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			outputBuffer := &bytes.Buffer{}
			conf := appFlags{Args: rootArgs{Root: rootDir{mapFS}}}

			err := conf.run("test-command", tt.mergeReader, outputBuffer)

			if tt.expectedError != nil {
				require.ErrorIs(t, err, tt.expectedError)
				return
			}

			require.NoError(t, err)

			if tt.mergeReader.filename != "" {
				require.Contains(t, outputBuffer.String(), "merge.yml")
			} else {
				require.NotContains(t, outputBuffer.String(), "merge.yml")
			}

			var resultConfig policy.Config
			err = yaml.Unmarshal(outputBuffer.Bytes(), &resultConfig)
			require.NoError(t, err)

			assert.Equal(t, tt.expectedConfig, resultConfig)
		})
	}
}
