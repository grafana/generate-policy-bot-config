package main

import (
	"testing"

	"github.com/palantir/policy-bot/policy"
	"github.com/palantir/policy-bot/policy/approval"
	"github.com/palantir/policy-bot/policy/common"
	"github.com/palantir/policy-bot/policy/predicate"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func mustRegexpsFromGlobs(t *testing.T, globs []string) []common.Regexp {
	t.Helper()

	result, err := regexpsFromGlobs(globs)
	require.NoError(t, err)

	return result
}

func TestRegexpsFromGlobs(t *testing.T) {
	testCases := []struct {
		name               string
		globs              []string
		expectedCount      int
		expectedErrorGlobs []string
	}{
		{
			name:          "valid globs",
			globs:         []string{"*.go", "src/**/*.js"},
			expectedCount: 2,
		},
		{
			name:               "single invalid glob",
			globs:              []string{"[invalid"},
			expectedErrorGlobs: []string{"[invalid"},
		},
		{
			name:               "multiple invalid globs",
			globs:              []string{"[invalid1", "[invalid2", "[invalid3"},
			expectedErrorGlobs: []string{"[invalid1", "[invalid2", "[invalid3"},
		},
		{
			name:               "mix of valid and invalid globs",
			globs:              []string{"*.go", "[invalid", "src/**/*.js", "[alsoInvalid"},
			expectedErrorGlobs: []string{"[invalid", "[alsoInvalid"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := regexpsFromGlobs(tc.globs)

			if tc.expectedErrorGlobs != nil {
				var errInvalidGlobs errInvalidGlobs
				require.ErrorAs(t, err, &errInvalidGlobs)
				require.Equal(t, tc.expectedErrorGlobs, errInvalidGlobs.Globs)
				return
			}

			require.NoError(t, err)
			require.Len(t, result, tc.expectedCount)
			for _, re := range result {
				require.IsType(t, common.Regexp{}, re)
			}
		})
	}
}

func TestMakeApprovalRule(t *testing.T) {
	testCases := []struct {
		name        string
		path        string
		workflow    gitHubWorkflow
		expected    *approval.Rule
		expectedErr bool
	}{
		{
			name: "workflow with paths",
			path: ".github/workflows/test.yml",
			workflow: gitHubWorkflow{
				On: githubWorkflowHeader{
					PullRequest: &gitHubWorkflowOnPullRequest{
						Paths:       []string{"src/**"},
						PathsIgnore: []string{"docs/**"},
					},
				},
			},
			expected: &approval.Rule{
				Name: ".github/workflows/test.yml built or skipped",
				Predicates: predicate.Predicates{
					ChangedFiles: &predicate.ChangedFiles{
						Paths:       mustRegexpsFromGlobs(t, []string{"src/**"}),
						IgnorePaths: mustRegexpsFromGlobs(t, []string{"docs/**"}),
					},
				},
				Requires: approval.Requires{
					Conditions: predicate.Predicates{
						HasWorkflowResult: &predicate.HasWorkflowResult{
							Conclusions: skippedOrSuccess,
							Workflows:   []string{".github/workflows/test.yml"},
						},
					},
				},
			},
		},
		{
			name: "workflow without paths",
			path: ".github/workflows/build.yml",
			workflow: gitHubWorkflow{
				On: githubWorkflowHeader{
					PullRequest: &gitHubWorkflowOnPullRequest{},
				},
			},
			expected: &approval.Rule{
				Name: ".github/workflows/build.yml built or skipped",
				Requires: approval.Requires{
					Conditions: predicate.Predicates{
						HasWorkflowResult: &predicate.HasWorkflowResult{
							Conclusions: skippedOrSuccess,
							Workflows:   []string{".github/workflows/build.yml"},
						},
					},
				},
			},
		},
		{
			name: "Invalid glob pattern",
			path: ".github/workflows/invalid.yml",
			workflow: gitHubWorkflow{
				On: githubWorkflowHeader{
					PullRequest: &gitHubWorkflowOnPullRequest{
						Paths: []string{"[invalid-glob"},
					},
				},
			},
			expectedErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := makeApprovalRule(tc.path, tc.workflow)

			if tc.expectedErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tc.expected, result)
		})
	}
}

func TestGitHubWorkflowCollectionPolicyBotConfig(t *testing.T) {
	workflows := gitHubWorkflowCollection{
		".github/workflows/test.yml": gitHubWorkflow{
			On: githubWorkflowHeader{
				PullRequest: &gitHubWorkflowOnPullRequest{
					Paths: []string{"src/**"},
				},
			},
		},
		".github/workflows/build.yml": gitHubWorkflow{
			On: githubWorkflowHeader{
				PullRequest: &gitHubWorkflowOnPullRequest{},
			},
		},
	}

	expected := policy.Config{
		Policy: policy.Policy{
			Approval: approval.Policy{
				map[string]interface{}{
					"and": []interface{}{
						".github/workflows/build.yml built or skipped",
						".github/workflows/test.yml built or skipped",
						defaultToApproval,
					},
				},
			},
		},
		ApprovalRules: []*approval.Rule{
			{
				Name: ".github/workflows/build.yml built or skipped",
				Requires: approval.Requires{
					Conditions: predicate.Predicates{
						HasWorkflowResult: &predicate.HasWorkflowResult{
							Conclusions: skippedOrSuccess,
							Workflows:   []string{".github/workflows/build.yml"},
						},
					},
				},
			},
			{
				Name: ".github/workflows/test.yml built or skipped",
				Predicates: predicate.Predicates{
					ChangedFiles: &predicate.ChangedFiles{
						Paths: mustRegexpsFromGlobs(t, []string{"src/**"}),
					},
				},
				Requires: approval.Requires{
					Conditions: predicate.Predicates{
						HasWorkflowResult: &predicate.HasWorkflowResult{
							Conclusions: skippedOrSuccess,
							Workflows:   []string{".github/workflows/test.yml"},
						},
					},
				},
			},
			{
				Name: defaultToApproval,
			},
		},
	}

	result := workflows.policyBotConfig()

	require.Equal(t, expected, result)

	expectedBytes, err := yaml.Marshal(expected)
	require.NoError(t, err)

	resultBytes, err := yaml.Marshal(result)
	require.NoError(t, err)

	require.Equal(t, expectedBytes, resultBytes)

	// Check the order of the approval rules
	require.Equal(t, ".github/workflows/build.yml built or skipped", result.ApprovalRules[0].Name)
	require.Equal(t, ".github/workflows/test.yml built or skipped", result.ApprovalRules[1].Name)
}

func BenchmarkMakeApprovalRule(b *testing.B) {
	path := ".github/workflows/test.yml"
	workflow := gitHubWorkflow{
		On: githubWorkflowHeader{
			PullRequest: &gitHubWorkflowOnPullRequest{
				Paths:       []string{"src/**", "tests/**"},
				PathsIgnore: []string{"docs/**"},
			},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := makeApprovalRule(path, workflow)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func FuzzRegexpsFromGlobs(f *testing.F) {
	f.Add("*.go")
	f.Add("src/**/*.js")
	f.Add("[invalid")

	f.Fuzz(func(t *testing.T, glob string) {
		// We're not checking the result, just ensuring it doesn't panic
		_, _ = regexpsFromGlobs([]string{glob})
	})
}

func FuzzMakeApprovalRule(f *testing.F) {
	f.Add(".gitub/workflows/foo.yml", []byte("on: pull_request"))
	f.Add(".github/workflows/a.yaml", []byte("on: [pull_request, pull_request_target]"))
	f.Add(".github/workflows/test.yml", []byte(`
on:
  pull_request:
    paths: ["src/**"]
`))
	f.Add("/!weird/,path.zzz", []byte(`
on:
  pull_request:
    paths: ["[invalid"]
`))

	f.Fuzz(func(t *testing.T, path string, yamlData []byte) {
		var wf gitHubWorkflow
		// We're not checking the result, just ensuring it doesn't panic
		_ = yaml.Unmarshal(yamlData, &wf)

		_, _ = makeApprovalRule(path, wf)
	})
}
