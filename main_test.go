package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/jessevdk/go-flags"
	"github.com/palantir/policy-bot/policy"
	"github.com/palantir/policy-bot/policy/approval"
	"github.com/palantir/policy-bot/policy/predicate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestParseFlags(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		expectedDir string
		expectedOut string
		expectError bool
	}{
		{
			name:        "Valid arguments",
			args:        []string{"-o", "output.yml"},
			expectedDir: "testdir",
			expectedOut: "output.yml",
		},
		{
			name:        "Output to stdout",
			args:        []string{"-o", "-"},
			expectedDir: "testdir",
			expectedOut: "-",
		},
		{
			name:        "Missing directory",
			args:        []string{"-o", "output.yml"},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			savedStdout := os.Stdout
			_, w, _ := os.Pipe()
			os.Stdout = w
			t.Cleanup(func() {
				os.Stdout = savedStdout
			})
			var conf appFlags
			t.Cleanup(func() {
				// Parsing may create a temporary file which we should clean up.
				err := conf.OutputWriter.Abort()
				require.NoError(t, err)
			})

			if tt.expectedDir != "" {
				testDir := filepath.Join(t.TempDir(), tt.expectedDir)
				tt.args = append(tt.args, testDir)

				require.NoError(t, os.MkdirAll(testDir, 0755))
				require.NoError(t, os.WriteFile(filepath.Join(testDir, "test_file.txt"), []byte(""), 0644))
			}

			parser := flags.NewParser(&conf, flags.Default)
			_, err := parser.ParseArgs(tt.args)

			if tt.expectError {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			// Check if the FS is pointing to the correct directory
			testFile := "test_file.txt"
			_, err = conf.Args.Root.Open(testFile)
			require.NoError(t, err)
			require.Equal(t, tt.expectedOut, conf.OutputWriter.dest)
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

type bytesBufferCloser struct {
	*bytes.Buffer
}

func (b *bytesBufferCloser) Close() error {
	return nil
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
											defaultToApproval,
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
								Paths: mustRegexpsFromGlobs(t, []string{"src/**"}),
							},
						},
						Requires: approval.Requires{
							Conditions: predicate.Predicates{
								HasWorkflowResult: &predicate.HasWorkflowResult{
									Conclusions: skippedOrSuccess,
									Workflows:   []string{".github/workflows/workflow.yml"},
								},
							},
						},
					},
					{
						Name: defaultToApproval,
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
			conf := appFlags{
				Args: rootArgs{Root: rootDir{mapFS}},
				OutputWriter: &renamingWriter{
					writeCloserRenamerRemover: nopRenamerRemover{
						&bytesBufferCloser{outputBuffer},
					},
				},
			}

			err := conf.run("test-command")

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

		conf := appFlags{
			Args: rootArgs{Root: rootDir{mapFS}},
			OutputWriter: &renamingWriter{
				writeCloserRenamerRemover: nopRenamerRemover{
					&bytesBufferCloser{buf},
				},
			},
		}

		err := conf.run("test-command")
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
	return workflows.policyBotConfig()
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
								defaultToApproval,
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
						Paths: mustRegexpsFromGlobs(t, []string{"src/**"}),
					},
				},
				Requires: approval.Requires{
					Conditions: predicate.Predicates{
						HasWorkflowResult: &predicate.HasWorkflowResult{
							Conclusions: skippedOrSuccess,
							Workflows:   []string{".github/workflows/workflow.yml"},
						},
					},
				},
			},
			{Name: defaultToApproval},
			{Name: "custom_rule"},
		},
	}
}

func testAppFlags(mapFS fstest.MapFS, outputBuffer *bytes.Buffer) appFlags {
	return appFlags{
		Args: rootArgs{Root: rootDir{mapFS}},
		OutputWriter: &renamingWriter{
			writeCloserRenamerRemover: nopRenamerRemover{
				&bytesBufferCloser{outputBuffer},
			},
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
		return reader{bytes.NewReader(data)}
	}

	// Helper function to create a fake stdin reader
	createFakeStdinReader := func(data []byte) reader {
		r, w, _ := os.Pipe()
		_, _ = w.Write(data)
		w.Close()
		return reader{r}
	}

	// Test cases
	tests := []struct {
		name           string
		setupReader    func() reader
		expectedConfig policy.Config
		expectedError  error
	}{
		{
			name:           "Merge with custom config",
			setupReader:    func() reader { return createReader(mergeConfigBytes) },
			expectedConfig: expectedConfig,
		},
		{
			name:           "Merge with fake stdin",
			setupReader:    func() reader { return createFakeStdinReader(mergeConfigBytes) },
			expectedConfig: expectedConfig,
		},
		{
			name:          "Merge with invalid config",
			setupReader:   func() reader { return createReader([]byte("invalid yaml")) },
			expectedError: errInvalidPolicyBotConfig{},
		},
		{
			name:           "No merge config (nil reader)",
			expectedConfig: baseConfig,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			outputBuffer := &bytes.Buffer{}
			conf := testAppFlags(mapFS, outputBuffer)

			if tt.setupReader != nil {
				conf.MergeConfig = tt.setupReader()
			}

			err := conf.run("test-command")

			if tt.expectedError != nil {
				require.ErrorIs(t, err, tt.expectedError)
				return
			}

			require.NoError(t, err)

			var resultConfig policy.Config
			err = yaml.Unmarshal(outputBuffer.Bytes(), &resultConfig)
			require.NoError(t, err)

			assert.Equal(t, tt.expectedConfig, resultConfig)
		})
	}
}
