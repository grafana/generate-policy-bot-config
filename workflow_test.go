package main

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestGitHubWorkflowUnmarshalYAML(t *testing.T) {
	testCases := []struct {
		name        string
		yamlContent string
		expected    gitHubWorkflow
		expectError bool
	}{
		{
			name:        "on as string",
			yamlContent: "on: pull_request",
			expected: gitHubWorkflow{
				On: githubWorkflowHeader{
					PullRequest: &gitHubWorkflowOnPullRequest{},
				},
			},
		},
		{
			name:        "on as list",
			yamlContent: "on: [pull_request, pull_request_target]",
			expected: gitHubWorkflow{
				On: githubWorkflowHeader{
					PullRequest:       &gitHubWorkflowOnPullRequest{},
					PullRequestTarget: &gitHubWorkflowOnPullRequest{},
				},
			},
		},
		{
			name: "on as map",
			yamlContent: `
on:
  pull_request:
    branches: [main]
    paths: [src/**]
  pull_request_target:
    paths-ignore: [docs/**]
`,
			expected: gitHubWorkflow{
				On: githubWorkflowHeader{
					PullRequest: &gitHubWorkflowOnPullRequest{
						Branches: []string{"main"},
						Paths:    []string{"src/**"},
					},
					PullRequestTarget: &gitHubWorkflowOnPullRequest{
						PathsIgnore: []string{"docs/**"},
					},
				},
			},
		},
		{
			name: "on as map with all fields",
			yamlContent: `
on:
  pull_request:
    branches: [main, develop]
    paths: [src/**]
    paths-ignore: [docs/**]
  pull_request_target:
    branches: [release/*]
    paths: [config/**]
    paths-ignore: [README.md]
`,
			expected: gitHubWorkflow{
				On: githubWorkflowHeader{
					PullRequest: &gitHubWorkflowOnPullRequest{
						Branches:    []string{"main", "develop"},
						Paths:       []string{"src/**"},
						PathsIgnore: []string{"docs/**"},
					},
					PullRequestTarget: &gitHubWorkflowOnPullRequest{
						Branches:    []string{"release/*"},
						Paths:       []string{"config/**"},
						PathsIgnore: []string{"README.md"},
					},
				},
			},
		},
		{
			name:        "invalid type",
			yamlContent: "on: 42",
			expectError: true,
		},
		{
			name:        "empty on field",
			yamlContent: "on: {}",
			expected:    gitHubWorkflow{},
		},
		{
			name:        "on with unsupported event",
			yamlContent: "on: push",
			expected:    gitHubWorkflow{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var wf gitHubWorkflow
			err := yaml.Unmarshal([]byte(tc.yamlContent), &wf)

			if tc.expectError {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tc.expected, wf)
		})
	}
}

func TestGitHubWorkflowIsPullRequestWorkflow(t *testing.T) {
	testCases := []struct {
		name     string
		workflow gitHubWorkflow
		ok       bool
	}{
		{
			name: "pull_request only",
			workflow: gitHubWorkflow{
				On: githubWorkflowHeader{
					PullRequest: &gitHubWorkflowOnPullRequest{},
				},
			},
			ok: true,
		},
		{
			name: "pull_request_target only",
			workflow: gitHubWorkflow{
				On: githubWorkflowHeader{
					PullRequestTarget: &gitHubWorkflowOnPullRequest{},
				},
			},
			ok: true,
		},
		{
			name: "both pull_request and pull_request_target",
			workflow: gitHubWorkflow{
				On: githubWorkflowHeader{
					PullRequest:       &gitHubWorkflowOnPullRequest{},
					PullRequestTarget: &gitHubWorkflowOnPullRequest{},
				},
			},
			ok: true,
		},
		{
			name:     "neither pull_request nor pull_request_target",
			workflow: gitHubWorkflow{},
			ok:       false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.workflow.isPullRequestWorkflow()
			require.Equal(t, tc.ok, result)
		})
	}
}

func TestGitHubWorkflowPaths(t *testing.T) {
	workflow := gitHubWorkflow{
		On: githubWorkflowHeader{
			PullRequest: &gitHubWorkflowOnPullRequest{
				Paths: []string{"src/**", "tests/**"},
			},
			PullRequestTarget: &gitHubWorkflowOnPullRequest{
				Paths: []string{"docs/**", "README.md"},
			},
		},
	}

	expected := []string{"src/**", "tests/**", "docs/**", "README.md"}
	result := workflow.paths()
	require.ElementsMatch(t, expected, result)
}

func TestGitHubWorkflowIgnorePaths(t *testing.T) {
	workflow := gitHubWorkflow{
		On: githubWorkflowHeader{
			PullRequest: &gitHubWorkflowOnPullRequest{
				PathsIgnore: []string{"vendor/**", "*.md"},
			},
			PullRequestTarget: &gitHubWorkflowOnPullRequest{
				PathsIgnore: []string{"docs/**", "CHANGELOG.md"},
			},
		},
	}

	expected := []string{"vendor/**", "*.md", "docs/**", "CHANGELOG.md"}
	result := workflow.ignorePaths()
	require.ElementsMatch(t, expected, result)
}

func FuzzGitHubWorkflowUnmarshalYAML(f *testing.F) {
	f.Add([]byte("on: pull_request"))
	f.Add([]byte("on: [pull_request, pull_request_target]"))
	f.Add([]byte(`
on:
  pull_request:
    paths: ["src/**"]
`))

	f.Fuzz(func(t *testing.T, data []byte) {
		var wf gitHubWorkflow
		// We're not testing yaml.Unmarshal itself, so we can ignore its errors
		_ = yaml.Unmarshal(data, &wf)

		// Check that no matter what input we get, isPullRequestWorkflow doesn't panic
		_ = wf.isPullRequestWorkflow()

		// Check that paths() and ignorePaths() don't panic
		_ = wf.paths()
		_ = wf.ignorePaths()
	})
}

func TestTypes(t *testing.T) {
	testCases := []struct {
		name     string
		workflow gitHubWorkflow
		expected []string
	}{
		{
			name: "pull_request only",
			workflow: gitHubWorkflow{
				On: githubWorkflowHeader{
					PullRequest: &gitHubWorkflowOnPullRequest{},
				},
			},
			expected: defaultTypes,
		},
		{
			name: "pull_request_target only",
			workflow: gitHubWorkflow{
				On: githubWorkflowHeader{
					PullRequestTarget: &gitHubWorkflowOnPullRequest{},
				},
			},
			expected: defaultTypes,
		},
		{
			name: "both pull_request and pull_request_target",
			workflow: gitHubWorkflow{
				On: githubWorkflowHeader{
					PullRequest:       &gitHubWorkflowOnPullRequest{},
					PullRequestTarget: &gitHubWorkflowOnPullRequest{},
				},
			},
			expected: defaultTypes,
		},
		{
			name:     "neither pull_request nor pull_request_target",
			workflow: gitHubWorkflow{},
			expected: nil,
		},
		{
			name: "pull_request with types",
			workflow: gitHubWorkflow{
				On: githubWorkflowHeader{
					PullRequest: &gitHubWorkflowOnPullRequest{
						Types: []string{"opened", "reopened"},
					},
				},
			},
			expected: []string{"opened", "reopened"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.workflow.types()
			require.ElementsMatch(t, tc.expected, result)
		})
	}
}
