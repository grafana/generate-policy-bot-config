package internal

import (
	"regexp"
	"regexp/syntax"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

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

func TestFilterListErrors(t *testing.T) {
	_, err := filterList{"[_]", "*.go", `!x\`}.includes()

	require.Equal(t, errInvalidFilterPatterns{
		{Pattern: "[_]", Err: errNotAlphanumeric{Char: "_"}},
		{Pattern: `!x\`, Err: errTrailingBackslash{}},
	}, err)
}

// lastMatchIncludes reports whether the last pattern in the list which matches
// the name isn't negated. That's how GitHub decides whether a filter list
// matches a name.
func lastMatchIncludes(t *testing.T, l filterList, name string) bool {
	t.Helper()

	included := false
	for _, pattern := range l {
		source, err := filterPattern(strings.TrimPrefix(pattern, "!")).regexp()
		require.NoError(t, err)

		if regexp.MustCompile(source).MatchString(name) {
			included = !strings.HasPrefix(pattern, "!")
		}
	}

	return included
}

// passes reports whether the name passes any of the terms.
func passes(terms []filterTerm, name string) bool {
	matchesAny := func(sources []string) bool {
		return slices.ContainsFunc(sources, func(source string) bool {
			return regexp.MustCompile(source).MatchString(name)
		})
	}

	return slices.ContainsFunc(terms, func(term filterTerm) bool {
		return (term.match == nil || matchesAny(term.match)) && !matchesAny(term.except)
	})
}

// FuzzFilterListTerms checks the terms for a comma-separated filter list
// against the last pattern which matches the name. An include list passes the
// name if that pattern isn't negated, and an exclude list passes it otherwise.
func FuzzFilterListTerms(f *testing.F) {
	f.Add("*.md,!README.md,README*", "README.md")
	f.Add("*.md,!a.md,b*,!README.md", "bx")
	f.Add("!README.md,*.md", "README.md")
	f.Add("docs/**,!docs/api/**,examples/**,!examples/keep.md,docs/api/generated/**", "docs/api/x")
	f.Add("!a", "a")

	f.Fuzz(func(t *testing.T, list string, name string) {
		if !utf8.ValidString(list) || !utf8.ValidString(name) {
			t.Skip("patterns and names from GitHub are valid UTF-8")
		}

		l := filterList(strings.Split(list, ","))

		includes, err := l.includes()
		if err != nil {
			t.Skip("invalid patterns are tested separately")
		}

		excludes, err := l.excludes()
		require.NoError(t, err)

		type result struct{ includes, excludes bool }

		included := lastMatchIncludes(t, l, name)

		require.Equal(t,
			result{includes: included, excludes: !included},
			result{includes: passes(includes, name), excludes: passes(excludes, name)},
		)
	})
}

func TestFilterErrorMessages(t *testing.T) {
	testCases := []struct {
		err      error
		expected string
	}{
		{
			err: errInvalidFilter{
				Event:  "pull_request",
				Filter: "paths",
				Err: errInvalidFilterPatterns{
					{Pattern: "[_]", Err: errNotAlphanumeric{Char: "_"}},
					{Pattern: "[]", Err: errEmptyBrackets{}},
				},
			},
			expected: `invalid paths filter for pull_request: invalid filter pattern "[_]": "_" in brackets isn't a letter or digit; invalid filter pattern "[]": "[]" is empty`,
		},
		{
			err:      errConflictingFilters{Event: "pull_request_target", Filter: "branches"},
			expected: "pull_request_target can't have both branches and branches-ignore",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.expected, func(t *testing.T) {
			require.Equal(t, tc.expected, tc.err.Error())
		})
	}
}

func TestMakeApprovalRulesStructure(t *testing.T) {
	notDeleted := &predicate.FileNotDeleted{Paths: []common.Regexp{mustRegexp(t, `^w\.yml$`)}}
	requires := approval.Requires{
		Conditions: predicate.Predicates{
			HasWorkflowResult: &predicate.HasWorkflowResult{
				Conclusions: SkippedOrSuccess,
				Workflows:   []string{"w.yml"},
			},
		},
	}

	testCases := []struct {
		name          string
		on            string
		expectedRules []*approval.Rule
		expected      []interface{}
	}{
		{
			name: "the same filters on both events",
			on:   "[pull_request, pull_request_target]",
			expectedRules: []*approval.Rule{
				{
					Name:     "Workflow w.yml succeeded or skipped",
					Requires: requires,
				},
			},
			expected: []interface{}{"Workflow w.yml succeeded or skipped"},
		},
		{
			name: "paths-ignore with a leading negated pattern",
			on: `
  pull_request:
    paths-ignore: ["!docs/README.md", "docs/**"]
`,
			expectedRules: []*approval.Rule{
				{
					Name: "Workflow w.yml succeeded or skipped",
					Predicates: predicate.Predicates{
						ChangedFiles: &predicate.ChangedFiles{
							Paths:       mustRegexps(t, `(?s)^.*$`),
							IgnorePaths: mustRegexps(t, `^docs/.*$`),
						},
						FileNotDeleted: notDeleted,
					},
					Requires: requires,
				},
			},
			expected: []interface{}{"Workflow w.yml succeeded or skipped"},
		},
		{
			name: "a negated branch",
			on: `
  pull_request:
    branches: ["releases/**", "!releases/**-alpha", "releases/v1-alpha"]
`,
			expectedRules: []*approval.Rule{
				{
					Name: "Workflow w.yml succeeded or skipped (1)",
					Predicates: predicate.Predicates{
						TargetsBranch:  &predicate.TargetsBranch{Pattern: mustRegexp(t, `(^releases/.*$)`)},
						FileNotDeleted: notDeleted,
					},
					Requires: requires,
				},
				{
					Name: "Workflow w.yml not required for this base branch (1)",
					Predicates: predicate.Predicates{
						TargetsBranch: &predicate.TargetsBranch{Pattern: mustRegexp(t, `(^releases/.*-alpha$)`)},
					},
				},
				{
					Name: "Workflow w.yml succeeded or skipped (2)",
					Predicates: predicate.Predicates{
						TargetsBranch:  &predicate.TargetsBranch{Pattern: mustRegexp(t, `(^releases/v1-alpha$)`)},
						FileNotDeleted: notDeleted,
					},
					Requires: requires,
				},
			},
			expected: []interface{}{
				map[string]interface{}{"or": []interface{}{
					"Workflow w.yml succeeded or skipped (1)",
					"Workflow w.yml not required for this base branch (1)",
				}},
				"Workflow w.yml succeeded or skipped (2)",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var wf GitHubWorkflow
			require.NoError(t, yaml.Unmarshal([]byte("on: "+tc.on), &wf))

			rules, entries, err := makeApprovalRules("w.yml", wf)
			require.NoError(t, err)

			require.Equal(t, tc.expectedRules, rules)
			require.Equal(t, tc.expected, entries)
		})
	}
}

func TestMakeApprovalRules(t *testing.T) {
	testCases := []struct {
		name        string
		path        string
		workflow    GitHubWorkflow
		expected    *approval.Rule
		expectedErr error
	}{
		{
			name: "workflow with paths",
			path: ".github/workflows/test.yml",
			workflow: GitHubWorkflow{
				On: githubWorkflowHeader{
					PullRequest: &gitHubWorkflowOnPullRequest{
						Paths: []string{"src/**"},
					},
				},
			},
			expected: &approval.Rule{
				Name: "Workflow .github/workflows/test.yml succeeded or skipped",
				Predicates: predicate.Predicates{
					ChangedFiles: &predicate.ChangedFiles{
						Paths: mustRegexps(t, `^src/.*$`),
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
			name: "workflow with paths and branches",
			path: ".github/workflows/test.yml",
			workflow: GitHubWorkflow{
				On: githubWorkflowHeader{
					PullRequest: &gitHubWorkflowOnPullRequest{
						Paths:    []string{"src/**"},
						Branches: []string{"main", "develop"},
					},
				},
			},
			expected: &approval.Rule{
				Name: "Workflow .github/workflows/test.yml succeeded or skipped",
				Predicates: predicate.Predicates{
					ChangedFiles: &predicate.ChangedFiles{
						Paths: mustRegexps(t, `^src/.*$`),
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
			name: "paths and paths-ignore together",
			path: ".github/workflows/invalid.yml",
			workflow: GitHubWorkflow{
				On: githubWorkflowHeader{
					PullRequest: &gitHubWorkflowOnPullRequest{
						Paths:       []string{"src/**"},
						PathsIgnore: []string{"docs/**"},
					},
				},
			},
			expectedErr: errConflictingFilters{Event: "pull_request", Filter: "paths"},
		},
		{
			name: "branches and branches-ignore together",
			path: ".github/workflows/invalid.yml",
			workflow: GitHubWorkflow{
				On: githubWorkflowHeader{
					PullRequest: &gitHubWorkflowOnPullRequest{
						Branches:       []string{"main"},
						BranchesIgnore: []string{"releases/**"},
					},
				},
			},
			expectedErr: errConflictingFilters{Event: "pull_request", Filter: "branches"},
		},
		{
			name: "invalid filter pattern in paths",
			path: ".github/workflows/invalid.yml",
			workflow: GitHubWorkflow{
				On: githubWorkflowHeader{
					PullRequest: &gitHubWorkflowOnPullRequest{
						Paths: []string{"[invalid-glob"},
					},
				},
			},
			expectedErr: errInvalidFilter{Event: "pull_request", Filter: "paths", Err: errInvalidFilterPatterns{{Pattern: "[invalid-glob", Err: errUnclosedBracket{}}}},
		},
		{
			name: "invalid filter pattern in paths-ignore",
			path: ".github/workflows/invalid.yml",
			workflow: GitHubWorkflow{
				On: githubWorkflowHeader{
					PullRequest: &gitHubWorkflowOnPullRequest{
						PathsIgnore: []string{"[invalid-glob"},
					},
				},
			},
			expectedErr: errInvalidFilter{Event: "pull_request", Filter: "paths-ignore", Err: errInvalidFilterPatterns{{Pattern: "[invalid-glob", Err: errUnclosedBracket{}}}},
		},
		{
			name: "invalid filter pattern in branches",
			path: ".github/workflows/invalid.yml",
			workflow: GitHubWorkflow{
				On: githubWorkflowHeader{
					PullRequest: &gitHubWorkflowOnPullRequest{
						Branches: []string{"[invalid-glob"},
					},
				},
			},
			expectedErr: errInvalidFilter{Event: "pull_request", Filter: "branches", Err: errInvalidFilterPatterns{{Pattern: "[invalid-glob", Err: errUnclosedBracket{}}}},
		},
		{
			name: "invalid filter pattern in pull_request_target branches-ignore",
			path: ".github/workflows/invalid.yml",
			workflow: GitHubWorkflow{
				On: githubWorkflowHeader{
					PullRequest: &gitHubWorkflowOnPullRequest{},
					PullRequestTarget: &gitHubWorkflowOnPullRequest{
						BranchesIgnore: []string{"[invalid-glob"},
					},
				},
			},
			expectedErr: errInvalidFilter{Event: "pull_request_target", Filter: "branches-ignore", Err: errInvalidFilterPatterns{{Pattern: "[invalid-glob", Err: errUnclosedBracket{}}}},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			rules, entries, err := makeApprovalRules(tc.path, tc.workflow)

			if tc.expectedErr != nil {
				require.Equal(t, tc.expectedErr, err)
				return
			}

			require.NoError(t, err)
			require.Equal(t, []*approval.Rule{tc.expected}, rules)
			require.Equal(t, []interface{}{tc.expected.Name}, entries)
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
			rules, _, err := makeApprovalRules(tc.path, GitHubWorkflow{
				On: githubWorkflowHeader{PullRequest: &gitHubWorkflowOnPullRequest{}},
			})
			require.NoError(t, err)

			require.Equal(t, []*approval.Rule{{
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
			}}, rules)
		})
	}
}

func TestMakeApprovalRulesInvalidWorkflowPath(t *testing.T) {
	_, _, err := makeApprovalRules(".github/workflows/\xff.yml", GitHubWorkflow{
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
						Paths: mustRegexps(t, `^src/.*$`),
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

func BenchmarkMakeApprovalRules(b *testing.B) {
	path := ".github/workflows/test.yml"
	workflow := GitHubWorkflow{
		On: githubWorkflowHeader{
			PullRequest: &gitHubWorkflowOnPullRequest{
				Paths: []string{"src/**", "tests/**", "!docs/**"},
			},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, err := makeApprovalRules(path, workflow)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func FuzzMakeApprovalRules(f *testing.F) {
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

		_, _, _ = makeApprovalRules(path, wf)
	})
}
