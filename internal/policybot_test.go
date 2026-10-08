package internal

import (
	"regexp/syntax"
	"testing"

	"github.com/palantir/policy-bot/policy"
	"github.com/palantir/policy-bot/policy/approval"
	"github.com/palantir/policy-bot/policy/common"
	"github.com/palantir/policy-bot/policy/predicate"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func mustRegexp(t *testing.T, pattern string) common.Regexp {
	t.Helper()

	result, err := common.NewRegexp(pattern)
	require.NoError(t, err)

	return result
}

func mustRegexps(t *testing.T, patterns ...string) []common.Regexp {
	t.Helper()

	result := make([]common.Regexp, len(patterns))
	for i, pattern := range patterns {
		result[i] = mustRegexp(t, pattern)
	}

	return result
}

func mustRegexpsFromGlobs(t *testing.T, globs []string) []common.Regexp {
	t.Helper()

	result, err := RegexpsFromGlobs(globs)
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
			result, err := RegexpsFromGlobs(tc.globs)

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
		workflow    GitHubWorkflow
		expected    *approval.Rule
		expectedErr bool
	}{
		{
			name: "workflow with paths",
			path: ".github/workflows/test.yml",
			workflow: GitHubWorkflow{
				On: githubWorkflowHeader{
					PullRequest: &gitHubWorkflowOnPullRequest{
						Paths:       []string{"src/**"},
						PathsIgnore: []string{"docs/**"},
					},
				},
			},
			expected: &approval.Rule{
				Name: "Workflow .github/workflows/test.yml succeeded or skipped",
				Predicates: predicate.Predicates{
					ChangedFiles: &predicate.ChangedFiles{
						Paths:       mustRegexpsFromGlobs(t, []string{"src/**"}),
						IgnorePaths: mustRegexpsFromGlobs(t, []string{"docs/**"}),
					},
					FileNotDeleted: &predicate.FileNotDeleted{
						Paths: mustRegexps(t, `^\.github/workflows/test\.yml$`),
					},
				},
				Requires: approval.Requires{
					Conditions: predicate.Predicates{
						HasWorkflowResult: &predicate.HasWorkflowResult{
							Conclusions: SkippedOrSuccess,
							Workflows:   []string{".github/workflows/test.yml"},
						},
					},
				},
			},
		},
		{
			name: "workflow without paths",
			path: ".github/workflows/build.yml",
			workflow: GitHubWorkflow{
				On: githubWorkflowHeader{
					PullRequest: &gitHubWorkflowOnPullRequest{},
				},
			},
			expected: &approval.Rule{
				Name: "Workflow .github/workflows/build.yml succeeded or skipped",
				Predicates: predicate.Predicates{
					FileNotDeleted: &predicate.FileNotDeleted{
						Paths: mustRegexps(t, `^\.github/workflows/build\.yml$`),
					},
				},
				Requires: approval.Requires{
					Conditions: predicate.Predicates{
						HasWorkflowResult: &predicate.HasWorkflowResult{
							Conclusions: SkippedOrSuccess,
							Workflows:   []string{".github/workflows/build.yml"},
						},
					},
				},
			},
		},
		{
			name: "workflow with branches",
			path: ".github/workflows/test.yml",
			workflow: GitHubWorkflow{
				On: githubWorkflowHeader{
					PullRequest: &gitHubWorkflowOnPullRequest{
						Branches: []string{"main", "develop"},
					},
				},
			},
			expected: &approval.Rule{
				Name: "Workflow .github/workflows/test.yml succeeded or skipped",
				Predicates: predicate.Predicates{
					TargetsBranch: &predicate.TargetsBranch{
						Pattern: mustRegexp(t, "(^main$|^develop$)"),
					},
					FileNotDeleted: &predicate.FileNotDeleted{
						Paths: mustRegexps(t, `^\.github/workflows/test\.yml$`),
					},
				},
				Requires: approval.Requires{
					Conditions: predicate.Predicates{
						HasWorkflowResult: &predicate.HasWorkflowResult{
							Conclusions: SkippedOrSuccess,
							Workflows:   []string{".github/workflows/test.yml"},
						},
					},
				},
			},
		},
		{
			name: "workflow with paths, ignore paths, and branches",
			path: ".github/workflows/test.yml",
			workflow: GitHubWorkflow{
				On: githubWorkflowHeader{
					PullRequest: &gitHubWorkflowOnPullRequest{
						Paths:       []string{"src/**"},
						PathsIgnore: []string{"docs/**"},
						Branches:    []string{"main", "develop"},
					},
				},
			},
			expected: &approval.Rule{
				Name: "Workflow .github/workflows/test.yml succeeded or skipped",
				Predicates: predicate.Predicates{
					ChangedFiles: &predicate.ChangedFiles{
						Paths:       mustRegexpsFromGlobs(t, []string{"src/**"}),
						IgnorePaths: mustRegexpsFromGlobs(t, []string{"docs/**"}),
					},
					TargetsBranch: &predicate.TargetsBranch{
						Pattern: mustRegexp(t, "(^main$|^develop$)"),
					},
					FileNotDeleted: &predicate.FileNotDeleted{
						Paths: mustRegexps(t, `^\.github/workflows/test\.yml$`),
					},
				},
				Requires: approval.Requires{
					Conditions: predicate.Predicates{
						HasWorkflowResult: &predicate.HasWorkflowResult{
							Conclusions: SkippedOrSuccess,
							Workflows:   []string{".github/workflows/test.yml"},
						},
					},
				},
			},
		},
		{
			name: "Invalid glob pattern (path)",
			path: ".github/workflows/invalid.yml",
			workflow: GitHubWorkflow{
				On: githubWorkflowHeader{
					PullRequest: &gitHubWorkflowOnPullRequest{
						Paths: []string{"[invalid-glob"},
					},
				},
			},
			expectedErr: true,
		},
		{
			name: "Invalid glob pattern (ignore path)",
			path: ".github/workflows/invalid.yml",
			workflow: GitHubWorkflow{
				On: githubWorkflowHeader{
					PullRequest: &gitHubWorkflowOnPullRequest{
						PathsIgnore: []string{"[invalid-glob"},
					},
				},
			},
			expectedErr: true,
		},
		{
			name: "Invalid glob pattern (branch)",
			path: ".github/workflows/invalid.yml",
			workflow: GitHubWorkflow{
				On: githubWorkflowHeader{
					PullRequest: &gitHubWorkflowOnPullRequest{
						Branches: []string{"[invalid-glob"},
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

// TestFileNotDeletedMatchesWorkflowLiterally checks that a workflow's own file
// name is matched as it is, even when it contains glob syntax.
func TestFileNotDeletedMatchesWorkflowLiterally(t *testing.T) {
	testCases := []struct {
		path   string
		regexp string
	}{
		{path: ".github/workflows/c++.yml", regexp: `^\.github/workflows/c\+\+\.yml$`},
		{path: ".github/workflows/ci[1].yml", regexp: `^\.github/workflows/ci\[1\]\.yml$`},
		{path: ".github/workflows/what?.yml", regexp: `^\.github/workflows/what\?\.yml$`},
		{path: ".github/workflows/{a,b}.yml", regexp: `^\.github/workflows/\{a,b\}\.yml$`},
	}

	for _, tc := range testCases {
		t.Run(tc.path, func(t *testing.T) {
			rule, err := makeApprovalRule(tc.path, GitHubWorkflow{
				On: githubWorkflowHeader{PullRequest: &gitHubWorkflowOnPullRequest{}},
			})
			require.NoError(t, err)

			require.Equal(t, &approval.Rule{
				Name: "Workflow " + tc.path + " succeeded or skipped",
				Predicates: predicate.Predicates{
					FileNotDeleted: &predicate.FileNotDeleted{
						Paths: []common.Regexp{mustRegexp(t, tc.regexp)},
					},
				},
				Requires: approval.Requires{
					Conditions: predicate.Predicates{
						HasWorkflowResult: &predicate.HasWorkflowResult{
							Conclusions: SkippedOrSuccess,
							Workflows:   []string{tc.path},
						},
					},
				},
			}, rule)
		})
	}
}

func TestMakeApprovalRuleInvalidWorkflowPath(t *testing.T) {
	_, err := makeApprovalRule(".github/workflows/\xff.yml", GitHubWorkflow{
		On: githubWorkflowHeader{PullRequest: &gitHubWorkflowOnPullRequest{}},
	})

	require.Equal(t, errInvalidWorkflowPath{
		Path: ".github/workflows/\xff.yml",
		Err:  &syntax.Error{Code: syntax.ErrInvalidUTF8, Expr: "\xff\\.yml$"},
	}, err)
}

func TestGitHubWorkflowCollectionPolicyBotConfig(t *testing.T) {
	workflows := GitHubWorkflowCollection{
		".github/workflows/test.yml": GitHubWorkflow{
			On: githubWorkflowHeader{
				PullRequest: &gitHubWorkflowOnPullRequest{
					Paths: []string{"src/**"},
				},
			},
		},
		".github/workflows/build.yml": GitHubWorkflow{
			On: githubWorkflowHeader{
				PullRequest: &gitHubWorkflowOnPullRequest{},
			},
		},
	}

	expected := policy.Config{
		Policy: policy.Policy{
			Approval: approval.Policy{
				map[string]interface{}{
					"or": []interface{}{
						map[string]interface{}{
							"and": []interface{}{
								"Workflow .github/workflows/build.yml succeeded or skipped",
								"Workflow .github/workflows/test.yml succeeded or skipped",
								DefaultToApproval,
							},
						},
					},
				},
			},
		},
		ApprovalRules: []*approval.Rule{
			{
				Name: "Workflow .github/workflows/build.yml succeeded or skipped",
				Predicates: predicate.Predicates{
					FileNotDeleted: &predicate.FileNotDeleted{
						Paths: mustRegexps(t, `^\.github/workflows/build\.yml$`),
					},
				},
				Requires: approval.Requires{
					Conditions: predicate.Predicates{
						HasWorkflowResult: &predicate.HasWorkflowResult{
							Conclusions: SkippedOrSuccess,
							Workflows:   []string{".github/workflows/build.yml"},
						},
					},
				},
			},
			{
				Name: "Workflow .github/workflows/test.yml succeeded or skipped",
				Predicates: predicate.Predicates{
					ChangedFiles: &predicate.ChangedFiles{
						Paths: mustRegexpsFromGlobs(t, []string{"src/**"}),
					},
					FileNotDeleted: &predicate.FileNotDeleted{
						Paths: mustRegexps(t, `^\.github/workflows/test\.yml$`),
					},
				},
				Requires: approval.Requires{
					Conditions: predicate.Predicates{
						HasWorkflowResult: &predicate.HasWorkflowResult{
							Conclusions: SkippedOrSuccess,
							Workflows:   []string{".github/workflows/test.yml"},
						},
					},
				},
			},
			{
				Name: DefaultToApproval,
			},
		},
	}

	result := workflows.PolicyBotConfig()

	require.Equal(t, expected, result)

	expectedBytes, err := yaml.Marshal(expected)
	require.NoError(t, err)

	resultBytes, err := yaml.Marshal(result)
	require.NoError(t, err)

	require.Equal(t, expectedBytes, resultBytes)

	// Check the order of the approval rules
	require.Equal(t, "Workflow .github/workflows/build.yml succeeded or skipped", result.ApprovalRules[0].Name)
	require.Equal(t, "Workflow .github/workflows/test.yml succeeded or skipped", result.ApprovalRules[1].Name)
}

func BenchmarkMakeApprovalRule(b *testing.B) {
	path := ".github/workflows/test.yml"
	workflow := GitHubWorkflow{
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
		_, _ = RegexpsFromGlobs([]string{glob})
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
		var wf GitHubWorkflow
		// We're not checking the result, just ensuring it doesn't panic
		_ = yaml.Unmarshal(yamlData, &wf)

		_, _ = makeApprovalRule(path, wf)
	})
}
