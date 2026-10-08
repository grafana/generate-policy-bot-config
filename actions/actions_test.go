package actions

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

type actionStep struct {
	ID  string            `yaml:"id"`
	Env map[string]string `yaml:"env"`
	Run string            `yaml:"run"`
}

type actionResult struct {
	ExitCode int
	Output   string
}

func actionSteps(t *testing.T, action string) []actionStep {
	t.Helper()

	contents, err := os.ReadFile(filepath.Join(action, "action.yml"))
	require.NoError(t, err)

	var manifest struct {
		Runs struct {
			Steps []actionStep `yaml:"steps"`
		} `yaml:"runs"`
	}
	require.NoError(t, yaml.Unmarshal(contents, &manifest))
	return manifest.Runs.Steps
}

func runAction(t *testing.T, action, workspace string, values map[string]string) actionResult {
	t.Helper()
	steps := actionSteps(t, action)
	return runActionStep(t, steps[len(steps)-1], action, workspace, values)
}

func runActionStep(t *testing.T, step actionStep, action, workspace string, values map[string]string) actionResult {
	t.Helper()

	replacements := make([]string, 0, len(values)*2)
	for key, value := range values {
		replacements = append(replacements, "${{ "+key+" }}", value)
	}
	replacer := strings.NewReplacer(replacements...)

	cmd := exec.Command("bash", "--noprofile", "--norc", "-e", "-o", "pipefail", "-c", replacer.Replace(step.Run))
	cmd.Dir = workspace
	cmd.Env = append(os.Environ(), "GITHUB_ACTION_PATH="+action, "RUNNER_TEMP="+t.TempDir())
	for key, value := range step.Env {
		cmd.Env = append(cmd.Env, key+"="+replacer.Replace(value))
	}

	output, err := cmd.CombinedOutput()
	exitCode := 0
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		exitCode = exitErr.ExitCode()
		err = nil
	}
	require.NoError(t, err)

	return actionResult{ExitCode: exitCode, Output: string(output)}
}

func TestGoCache(t *testing.T) {
	var step actionStep
	for _, candidate := range actionSteps(t, "check-for-drift") {
		if candidate.ID == "cache" {
			step = candidate
			break
		}
	}
	require.NotEmpty(t, step.Run, "the action must calculate its own Go cache key")

	source := filepath.Join(t.TempDir(), "action source")
	action := filepath.Join(source, "actions", "check-for-drift")
	require.NoError(t, os.MkdirAll(action, 0o755))
	checksumFile := []byte("generator dependencies\n")
	require.NoError(t, os.WriteFile(filepath.Join(source, "go.sum"), checksumFile, 0o600))
	workspace := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(workspace, "go.sum"), []byte("consumer dependencies\n"), 0o600))
	outputFile := filepath.Join(workspace, "outputs")
	moduleCache := t.TempDir()
	buildCache := t.TempDir()
	t.Setenv("GITHUB_OUTPUT", outputFile)
	t.Setenv("GOMODCACHE", moduleCache)
	t.Setenv("GOCACHE", buildCache)

	result := runActionStep(t, step, action, workspace, nil)
	outputs, err := os.ReadFile(outputFile)
	require.NoError(t, err)
	digest := sha256.Sum256(checksumFile)
	type cacheResult struct {
		Run     actionResult
		Outputs string
	}
	require.Equal(t, cacheResult{
		Outputs: "checksum=" + hex.EncodeToString(digest[:]) + "\nmodules=" + moduleCache + "\nbuild=" + buildCache + "\n",
	}, cacheResult{Run: result, Outputs: string(outputs)})
}

func TestCheckForDrift(t *testing.T) {
	action, err := filepath.Abs("check-for-drift")
	require.NoError(t, err)
	bin := t.TempDir()
	generator := filepath.Join(bin, "generate-policy-bot-config")
	build := exec.Command("go", "build", "-o", generator, "../cmd/generate-policy-bot-config")
	output, err := build.CombinedOutput()
	require.NoError(t, err, string(output))

	tests := []struct {
		name         string
		input        string
		merge        string
		change       string
		missing      bool
		missingInput bool
		exitCode     int
		message      string
	}{
		{name: "No merge file", input: ".policy.yml"},
		{name: "Merge file", input: ".policy.yml", merge: "policy.yml"},
		{name: "Literal paths", input: "policy \"$(touch injected).yml", merge: "merge \"$(touch injected).yml"},
		{name: "Content drift", input: ".policy.yml", merge: "policy.yml", change: "changed", exitCode: 1, message: "Drift detected:"},
		{name: "Trailing newline drift", input: ".policy.yml", merge: "policy.yml", change: "newline", exitCode: 1, message: "Drift detected:"},
		{name: "Generation failure", input: ".policy.yml", merge: "missing.yml", missing: true, exitCode: 1, message: "failed to open merge file"},
		{name: "Missing policy", input: ".policy.yml", merge: "policy.yml", missingInput: true, exitCode: 2, message: "No such file or directory"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workspace := t.TempDir()
			workflows := filepath.Join(workspace, ".github", "workflows")
			require.NoError(t, os.MkdirAll(workflows, 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(workflows, "test.yml"), []byte("name: Test\non: pull_request\njobs:\n  test:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo ok\n"), 0o600))

			if tt.merge != "" && !tt.missing {
				require.NoError(t, os.WriteFile(filepath.Join(workspace, tt.merge), []byte("policy:\n  approval:\n    - and: []\n"), 0o600))
			}
			merge := tt.merge
			if tt.missing {
				merge = ""
			}
			generate := exec.Command(generator, "--output", "-", "--merge-with="+merge, ".")
			generate.Dir = workspace
			config, err := generate.Output()
			require.NoError(t, err)
			switch tt.change {
			case "changed":
				config = append(config, []byte("# changed\n")...)
			case "newline":
				config = append(config, '\n')
			}
			if !tt.missingInput {
				require.NoError(t, os.WriteFile(filepath.Join(workspace, tt.input), config, 0o600))
			}

			result := runAction(t, action, workspace, map[string]string{
				"inputs.input_file": tt.input,
				"inputs.merge_with": tt.merge,
			})
			require.Equal(t, tt.exitCode, result.ExitCode, result.Output)
			message := tt.message
			if message == "" {
				message = "No drift detected: " + tt.input + " is up-to-date.\n"
			}
			require.Contains(t, result.Output, message)
			if tt.missing || tt.missingInput {
				require.NotContains(t, result.Output, "Drift detected:")
			}
			_, err = os.Stat(filepath.Join(workspace, "injected"))
			require.ErrorIs(t, err, os.ErrNotExist)
		})
	}
}

func TestValidate(t *testing.T) {
	action, err := filepath.Abs("validate")
	require.NoError(t, err)
	for _, status := range []int{http.StatusOK, http.StatusUnprocessableEntity, http.StatusInternalServerError} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			workspace := t.TempDir()
			policy := "policy $(touch injected).yml"
			contents := "policy: {}\n"
			require.NoError(t, os.WriteFile(filepath.Join(workspace, policy), []byte(contents), 0o600))
			type request struct {
				Method string
				Body   string
				Query  string
			}
			requests := make(chan request, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, readErr := io.ReadAll(r.Body)
				if readErr != nil {
					http.Error(w, readErr.Error(), http.StatusInternalServerError)
					return
				}
				requests <- request{Method: r.Method, Body: string(body), Query: r.URL.RawQuery}
				w.WriteHeader(status)
				_, _ = io.WriteString(w, "validation response\n")
			}))
			defer server.Close()

			query := "probe=$(touch$IFS'injected')"
			result := runAction(t, action, workspace, map[string]string{
				"inputs.policy":              policy,
				"inputs.validation_endpoint": server.URL + "?" + query,
			})
			exitCode := 0
			if status != http.StatusOK {
				exitCode = 22
			}
			require.Equal(t, exitCode, result.ExitCode, result.Output)
			require.Contains(t, result.Output, "validation response\n")
			if status != http.StatusOK {
				require.Contains(t, result.Output, "curl: (22)")
			}
			select {
			case got := <-requests:
				require.Equal(t, request{Method: http.MethodPut, Body: contents, Query: query}, got)
			default:
				t.Fatal("validation endpoint did not receive the policy")
			}
			_, err = os.Stat(filepath.Join(workspace, "injected"))
			require.ErrorIs(t, err, os.ErrNotExist)
		})
	}
}
