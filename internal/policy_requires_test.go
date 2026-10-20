package internal

import (
	"bytes"
	"context"
	"testing"

	"github.com/palantir/policy-bot/policy"
	"github.com/palantir/policy-bot/policy/common"
	"github.com/palantir/policy-bot/pull"
	"github.com/palantir/policy-bot/pull/pulltest"
	"github.com/stretchr/testify/require"
	yamlv2 "gopkg.in/yaml.v2"
	"gopkg.in/yaml.v3"
)

// pullRequest is the part of a pull request which decides whether GitHub runs
// a workflow for it.
type pullRequest struct {
	base    string
	files   []string
	deleted []string
}

// The statuses of a policy for a pull request when no workflow has run. The
// policy is pending exactly when it requires the workflow, and the "default to
// approval" rule approves it otherwise.
const (
	required    = common.StatusPending
	notRequired = common.StatusApproved
)

// evaluatePolicy generates a policy for a workflow with the given `on`
// section, and returns policy-bot's status for each pull request.
//
// The policy is written out and read back in the way that policy-bot reads
// `.policy.yml`, which also checks that policy-bot accepts it.
func evaluatePolicy(t *testing.T, on string, prs []pullRequest) []common.EvaluationStatus {
	t.Helper()

	var wf GitHubWorkflow
	require.NoError(t, yaml.Unmarshal([]byte("on:\n"+on), &wf))

	var written bytes.Buffer
	require.NoError(t, WriteYamlToWriter(&written, GitHubWorkflowCollection{".github/workflows/w.yml": wf}.PolicyBotConfig()))

	var config policy.Config
	require.NoError(t, yamlv2.UnmarshalStrict(written.Bytes(), &config))

	evaluator, err := policy.ParsePolicy(&config, nil)
	require.NoError(t, err)

	statuses := make([]common.EvaluationStatus, len(prs))
	for i, pr := range prs {
		var files []*pull.File
		for _, f := range pr.files {
			files = append(files, &pull.File{Filename: f, Status: pull.FileModified})
		}
		for _, f := range pr.deleted {
			files = append(files, &pull.File{Filename: f, Status: pull.FileDeleted})
		}

		result := evaluator.Evaluate(context.Background(), &pulltest.Context{
			BranchBaseName:    pr.base,
			ChangedFilesValue: files,
		})
		require.NoError(t, result.Error)

		statuses[i] = result.Status
	}

	return statuses
}

// The cases follow what GitHub was observed to do with the same filters and
// pull requests, on 2026-10-08.
func TestPolicyRequiresWorkflow(t *testing.T) {
	testCases := []struct {
		name     string
		on       string
		prs      []pullRequest
		expected []common.EvaluationStatus
	}{
		{
			name: "negated path, re-included by a later pattern",
			on: `
  pull_request:
    paths: ["*.md", "!README.md", "README*"]
`,
			prs: []pullRequest{
				{base: "main", files: []string{"hello.md"}},
				{base: "main", files: []string{"README.md"}},
				{base: "main", files: []string{"README.doc"}},
				{base: "main", files: []string{"docs/hello.md"}},
			},
			expected: []common.EvaluationStatus{required, required, required, notRequired},
		},
		{
			name: "negated path, without re-inclusion",
			on: `
  pull_request:
    paths: ["*.md", "!README.md"]
`,
			prs: []pullRequest{
				{base: "main", files: []string{"hello.md"}},
				{base: "main", files: []string{"README.md"}},
				{base: "main", files: []string{"README.md", "hello.md"}},
			},
			expected: []common.EvaluationStatus{required, notRequired, required},
		},
		{
			name: "negated paths after two runs of paths",
			on: `
  pull_request:
    paths: ["*.md", "!a.md", "b*", "!README.md"]
`,
			prs: []pullRequest{
				{base: "main", files: []string{"hello.md"}},
				{base: "main", files: []string{"a.md"}},
				{base: "main", files: []string{"b.md"}},
				{base: "main", files: []string{"bx"}},
				{base: "main", files: []string{"README.md"}},
			},
			expected: []common.EvaluationStatus{required, notRequired, required, required, notRequired},
		},
		{
			name: "only negated paths",
			on: `
  pull_request:
    paths: ["!a"]
`,
			prs: []pullRequest{
				{base: "main", files: []string{"a"}},
				{base: "main", files: []string{"b"}},
			},
			expected: []common.EvaluationStatus{notRequired, notRequired},
		},
		{
			name: "only negated paths on one event, and none on the other",
			on: `
  pull_request:
    paths: ["!a"]
  pull_request_target:
`,
			prs: []pullRequest{
				{base: "main", files: []string{"a"}},
				{base: "main", files: []string{"b"}},
			},
			expected: []common.EvaluationStatus{required, required},
		},
		{
			name: "escaped leading exclamation mark",
			on: `
  pull_request:
    paths: ['\!a']
`,
			prs: []pullRequest{
				{base: "main", files: []string{"!a"}},
				{base: "main", files: []string{"a"}},
			},
			expected: []common.EvaluationStatus{required, notRequired},
		},
		{
			name: "leading negated path",
			on: `
  pull_request:
    paths: ["!README.md", "*.md"]
`,
			prs: []pullRequest{
				{base: "main", files: []string{"README.md"}},
			},
			expected: []common.EvaluationStatus{required},
		},
		{
			name: "paths-ignore without paths",
			on: `
  pull_request:
    paths-ignore: ["docs/**"]
`,
			prs: []pullRequest{
				{base: "main", files: []string{"README.md"}},
				{base: "main", files: []string{"docs/README.md"}},
				{base: "main", files: []string{"docs/README.md", "README.md"}},
			},
			expected: []common.EvaluationStatus{required, notRequired, required},
		},
		{
			name: "paths-ignore with a negated pattern",
			on: `
  pull_request:
    paths-ignore: ["docs/**", "!docs/README.md"]
`,
			prs: []pullRequest{
				{base: "main", files: []string{"docs/README.md"}},
				{base: "main", files: []string{"docs/file.txt"}},
				{base: "main", files: []string{"README.md"}},
			},
			expected: []common.EvaluationStatus{required, notRequired, required},
		},
		{
			name: "paths-ignore with a leading negated pattern",
			on: `
  pull_request:
    paths-ignore: ["!docs/README.md", "docs/**"]
`,
			prs: []pullRequest{
				{base: "main", files: []string{"docs/README.md"}},
				{base: "main", files: []string{"README.md"}},
			},
			expected: []common.EvaluationStatus{notRequired, required},
		},
		{
			name: "paths-ignore with negated patterns after two runs",
			on: `
  pull_request:
    paths-ignore: ["docs/**", "!docs/api/**", "examples/**", "!examples/keep.md", "docs/api/generated/**"]
`,
			prs: []pullRequest{
				{base: "main", files: []string{"docs/api/generated/x"}},
				{base: "main", files: []string{"docs/api/x"}},
				{base: "main", files: []string{"docs/x"}},
				{base: "main", files: []string{"examples/keep.md"}},
				{base: "main", files: []string{"examples/x"}},
				{base: "main", files: []string{"README.md"}},
			},
			expected: []common.EvaluationStatus{notRequired, required, notRequired, required, notRequired, required},
		},
		// GitHub doesn't run a workflow with path filters for a pull request
		// which changes no files.
		{
			name: "paths-ignore with only negated patterns",
			on: `
  pull_request:
    paths-ignore: ["!a"]
`,
			prs: []pullRequest{
				{base: "main", files: []string{"a"}},
				{base: "main", files: []string{"b"}},
				{base: "main"},
			},
			expected: []common.EvaluationStatus{required, required, notRequired},
		},
		{
			name: "paths-ignore and a file name with a newline",
			on: `
  pull_request:
    paths-ignore: ["docs/**"]
`,
			prs: []pullRequest{
				{base: "main", files: []string{"new\nline.txt"}},
			},
			expected: []common.EvaluationStatus{required},
		},
		{
			name: "branches",
			on: `
  pull_request:
    branches: ["feature/*"]
`,
			prs: []pullRequest{
				{base: "feature/a", files: []string{"x"}},
				{base: "feature/b/c", files: []string{"x"}},
				{base: "main", files: []string{"x"}},
			},
			expected: []common.EvaluationStatus{required, notRequired, notRequired},
		},
		{
			name: "negated branch",
			on: `
  pull_request:
    branches: ["releases/**", "!releases/**-alpha"]
`,
			prs: []pullRequest{
				{base: "releases/v1", files: []string{"x"}},
				{base: "releases/v1-alpha", files: []string{"x"}},
				{base: "main", files: []string{"x"}},
			},
			expected: []common.EvaluationStatus{required, notRequired, notRequired},
		},
		{
			name: "negated branch, re-included by a later pattern",
			on: `
  pull_request:
    branches: ["releases/**", "!releases/**-alpha", "releases/v1-alpha"]
`,
			prs: []pullRequest{
				{base: "releases/v1", files: []string{"x"}},
				{base: "releases/v1-alpha", files: []string{"x"}},
				{base: "releases/v2-alpha", files: []string{"x"}},
			},
			expected: []common.EvaluationStatus{required, required, notRequired},
		},
		{
			name: "branches-ignore",
			on: `
  pull_request:
    branches-ignore: ["releases/**"]
`,
			prs: []pullRequest{
				{base: "main", files: []string{"x"}},
				{base: "releases/v1", files: []string{"x"}},
			},
			expected: []common.EvaluationStatus{required, notRequired},
		},
		{
			name: "branches-ignore with a negated pattern",
			on: `
  pull_request:
    branches-ignore: ["releases/**", "!releases/v1"]
`,
			prs: []pullRequest{
				{base: "main", files: []string{"x"}},
				{base: "releases/v1", files: []string{"x"}},
				{base: "releases/v1-alpha", files: []string{"x"}},
			},
			expected: []common.EvaluationStatus{required, required, notRequired},
		},
		{
			name: "branch and path filters together",
			on: `
  pull_request:
    branches: ["main"]
    paths: ["docs/**"]
`,
			prs: []pullRequest{
				{base: "main", files: []string{"docs/README.md"}},
				{base: "main", files: []string{"README.md"}},
				{base: "other", files: []string{"docs/README.md"}},
			},
			expected: []common.EvaluationStatus{required, notRequired, notRequired},
		},
		{
			name: "events whose filters differ only in exceptions",
			on: `
  pull_request:
    paths: ["**", "!b"]
  pull_request_target:
    paths: ["**"]
`,
			prs: []pullRequest{
				{base: "main", files: []string{"b"}},
			},
			expected: []common.EvaluationStatus{required},
		},
		{
			name: "each event's filters apply separately",
			on: `
  pull_request:
    paths: ["docs/**"]
  pull_request_target:
    paths-ignore: ["src/**"]
`,
			prs: []pullRequest{
				{base: "main", files: []string{"docs/README.md"}},
				{base: "main", files: []string{"README.md"}},
				{base: "main", files: []string{"src/main.go"}},
			},
			expected: []common.EvaluationStatus{required, required, notRequired},
		},
		{
			name: "each event's branch and path filters apply together",
			on: `
  pull_request:
    branches: ["main"]
    paths: ["src/**"]
  pull_request_target:
    branches: ["develop"]
    paths: ["docs/**"]
`,
			prs: []pullRequest{
				{base: "main", files: []string{"src/x"}},
				{base: "main", files: []string{"docs/x"}},
				{base: "develop", files: []string{"src/x"}},
				{base: "develop", files: []string{"docs/x"}},
			},
			expected: []common.EvaluationStatus{required, notRequired, notRequired, required},
		},
		// For pull_request, GitHub runs the workflow from the pull request's
		// merge commit, which doesn't have a deleted workflow. For
		// pull_request_target, it runs the workflow from the base repository's
		// default branch.
		{
			name: "pull_request, with the workflow deleted",
			on: `
  pull_request:
`,
			prs: []pullRequest{
				{base: "main", deleted: []string{".github/workflows/w.yml"}},
				{base: "main", deleted: []string{"x"}},
			},
			expected: []common.EvaluationStatus{notRequired, required},
		},
		{
			name: "pull_request_target, with the workflow deleted",
			on: `
  pull_request_target:
`,
			prs: []pullRequest{
				{base: "main", deleted: []string{".github/workflows/w.yml"}},
			},
			expected: []common.EvaluationStatus{required},
		},
		{
			name: "both events with the same filters, with the workflow deleted",
			on: `
  pull_request:
  pull_request_target:
`,
			prs: []pullRequest{
				{base: "main", deleted: []string{".github/workflows/w.yml"}},
			},
			expected: []common.EvaluationStatus{required},
		},
		{
			name: "both events with different filters, with the workflow deleted",
			on: `
  pull_request:
    paths: [".github/**"]
  pull_request_target:
    paths: ["src/**"]
`,
			prs: []pullRequest{
				{base: "main", deleted: []string{".github/workflows/w.yml"}},
				{base: "main", files: []string{"src/x"}, deleted: []string{".github/workflows/w.yml"}},
			},
			expected: []common.EvaluationStatus{notRequired, required},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.expected, evaluatePolicy(t, tc.on, tc.prs))
		})
	}
}
