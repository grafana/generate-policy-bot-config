package internal

import (
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

// TestRegexpsFromGlobsMatchGitHub checks globs against the examples in GitHub's
// filter pattern cheat sheet, plus some paths which they must not match.
func TestRegexpsFromGlobsMatchGitHub(t *testing.T) {
	testCases := []struct {
		glob  string
		paths map[string]bool
	}{
		{
			glob:  "**",
			paths: map[string]bool{"all/the/files.md": true},
		},
		{
			glob: "**.js",
			paths: map[string]bool{
				"index.js":      true,
				"js/index.js":   true,
				"src/js/app.js": true,
				"index.ts":      false,
			},
		},
		{
			glob: "docs/**",
			paths: map[string]bool{
				"docs/README.md":        true,
				"docs/mona/octocat.txt": true,
				"src/docs/README.md":    false,
			},
		},
		{
			glob: "docs/**/*.md",
			paths: map[string]bool{
				"docs/README.md":           true,
				"docs/mona/hello-world.md": true,
				"docs/a/markdown/file.md":  true,
				"docs/mona/octocat.txt":    false,
			},
		},
		{
			glob: "**/docs/**",
			paths: map[string]bool{
				"docs/hello.md":             true,
				"dir/docs/my-file.txt":      true,
				"space/docs/plan/space.doc": true,
				"dir/documents/file.txt":    false,
			},
		},
		{
			glob: "**/README.md",
			paths: map[string]bool{
				"README.md":     true,
				"js/README.md":  true,
				"js/README.txt": false,
			},
		},
		{
			glob: "**/*src/**",
			paths: map[string]bool{
				"a/src/app.js":          true,
				"my-src/code/js/app.js": true,
				"source/app.js":         false,
			},
		},
		{
			glob: "**/*-post.md",
			paths: map[string]bool{
				"my-post.md":         true,
				"path/their-post.md": true,
				"path/post.md":       false,
			},
		},
		{
			glob: "**/migrate-*.sql",
			paths: map[string]bool{
				"migrate-10909.sql":      true,
				"db/migrate-v1.0.sql":    true,
				"db/sept/migrate-v1.sql": true,
				"db/migrate-v1.0.txt":    false,
			},
		},
		{
			glob: "releases/**-alpha",
			paths: map[string]bool{
				"releases/3-alpha":      true,
				"releases/beta/3-alpha": true,
				"releases/3-beta":       false,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.glob, func(t *testing.T) {
			regexps, err := RegexpsFromGlobs([]string{tc.glob})
			require.NoError(t, err)

			matches := make(map[string]bool, len(tc.paths))
			for path := range tc.paths {
				matches[path] = regexps[0].Matches(path)
			}

			require.Equal(t, tc.paths, matches)
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
						Paths: mustRegexpsFromGlobs(t, []string{".github/workflows/test.yml"}),
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
						Paths: mustRegexpsFromGlobs(t, []string{".github/workflows/build.yml"}),
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
						Paths: mustRegexpsFromGlobs(t, []string{".github/workflows/test.yml"}),
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
						Paths: mustRegexpsFromGlobs(t, []string{".github/workflows/test.yml"}),
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
						Paths: mustRegexpsFromGlobs(t, []string{".github/workflows/build.yml"}),
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
						Paths: mustRegexpsFromGlobs(t, []string{".github/workflows/test.yml"}),
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
