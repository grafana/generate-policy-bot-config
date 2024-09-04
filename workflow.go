package main

import (
	"fmt"
	"slices"

	"gopkg.in/yaml.v3"
)

// Default here:
// https://docs.github.com/en/actions/using-workflows/events-that-trigger-workflows#pull_request_review
var defaultTypes = []string{
	"opened",
	"reopened",
	"synchronize",
}

// gitHubWorkflowOnPullRequest represents the configuration for pull request
// triggers in a GitHub Actions workflow.
type gitHubWorkflowOnPullRequest struct {
	Branches    []string
	Paths       []string
	PathsIgnore []string `yaml:"paths-ignore"`
	Types       []string
}

// githubWorkflowHeader represents the 'on' section of a GitHub Actions workflow file.
type githubWorkflowHeader struct {
	PullRequest       *gitHubWorkflowOnPullRequest `yaml:"pull_request"`
	PullRequestTarget *gitHubWorkflowOnPullRequest `yaml:"pull_request_target"`
}

// gitHubWorkflow represents a GitHub Actions workflow file.
type gitHubWorkflow struct {
	On githubWorkflowHeader
}

// gitHubWorkflowCollection represents a collection of GitHub Actions workflows.
// It is a map where the key is the workflow's path.
type gitHubWorkflowCollection map[string]gitHubWorkflow

// UnmarshalYAML implements custom unmarshaling for githubWorkflowHeader. This
// is needed because the `on` field can be a string, list or map.
func (wfh *githubWorkflowHeader) UnmarshalYAML(node *yaml.Node) error {
	var rawValue interface{}
	if err := node.Decode(&rawValue); err != nil {
		return errWorkflowParse{Err: err}
	}

	switch v := rawValue.(type) {
	case string:
		return wfh.unmarshalString(v)
	case []interface{}:
		return wfh.unmarshalList(v)
	case map[string]interface{}:
		return wfh.unmarshalMap(node)
	default:
		return errUnexpectedType{Type: fmt.Sprintf("%T", v)}
	}
}

// unmarshalString handles unmarshaling when the 'on' field is a string.
func (wfh *githubWorkflowHeader) unmarshalString(s string) error {
	switch s {
	case "pull_request":
		wfh.PullRequest = &gitHubWorkflowOnPullRequest{}
	case "pull_request_target":
		wfh.PullRequestTarget = &gitHubWorkflowOnPullRequest{}
	}
	return nil
}

// unmarshalList handles unmarshaling when the 'on' field is a list.
func (wfh *githubWorkflowHeader) unmarshalList(list []interface{}) error {
	for _, item := range list {
		if s, ok := item.(string); ok {
			if err := wfh.unmarshalString(s); err != nil {
				return errWorkflowParse{Err: err}
			}
		}
	}
	return nil
}

// unmarshalMap handles unmarshaling when the 'on' field is a map.
func (wfh *githubWorkflowHeader) unmarshalMap(node *yaml.Node) error {
	type rawHeader githubWorkflowHeader
	if err := node.Decode((*rawHeader)(wfh)); err != nil {
		return errWorkflowParse{Err: err}
	}

	// This section handles the case where the 'on' field is a map, but the
	// `pull_request` or `pull_request_target` keys are empty, i.e. `on:
	// {pull_request: {}}`.
	var raw map[string]interface{}
	if err := node.Decode(&raw); err != nil {
		return errWorkflowParse{Err: err}
	}

	if _, ok := raw["pull_request"]; wfh.PullRequest == nil && ok {
		wfh.PullRequest = &gitHubWorkflowOnPullRequest{}
	}
	if _, ok := raw["pull_request_target"]; wfh.PullRequestTarget == nil && ok {
		wfh.PullRequestTarget = &gitHubWorkflowOnPullRequest{}
	}

	return nil
}

// isPullRequestWorkflow checks if the workflow is triggered by pull requests.
func (wf gitHubWorkflow) isPullRequestWorkflow() bool {
	return wf.On.PullRequest != nil || wf.On.PullRequestTarget != nil
}

// branches returns the combined branches from PullRequest and PullRequestTarget.
// These are the branches that might trigger a run on a pull request.
func (wf gitHubWorkflow) branches() []string {
	var branches []string
	if wf.On.PullRequest != nil {
		branches = append(branches, wf.On.PullRequest.Branches...)
	}
	if wf.On.PullRequestTarget != nil {
		branches = append(branches, wf.On.PullRequestTarget.Branches...)
	}
	return branches
}

// paths returns the combined paths from PullRequest and PullRequestTarget.
// These are the paths that might trigger a run on a pull request.
func (wf gitHubWorkflow) paths() []string {
	var paths []string
	if wf.On.PullRequest != nil {
		paths = append(paths, wf.On.PullRequest.Paths...)
	}
	if wf.On.PullRequestTarget != nil {
		paths = append(paths, wf.On.PullRequestTarget.Paths...)
	}
	return paths
}

// ignorePaths returns the combined ignore paths from PullRequest and
// PullRequestTarget. These are the paths that should be ignored when
// determining if a run should be triggered on a pull request.
func (wf gitHubWorkflow) ignorePaths() []string {
	var ignorePaths []string
	if wf.On.PullRequest != nil {
		ignorePaths = append(ignorePaths, wf.On.PullRequest.PathsIgnore...)
	}
	if wf.On.PullRequestTarget != nil {
		ignorePaths = append(ignorePaths, wf.On.PullRequestTarget.PathsIgnore...)
	}
	return ignorePaths
}

func (wf gitHubWorkflow) types() []string {
	if wf.On.PullRequest == nil && wf.On.PullRequestTarget == nil {
		return nil
	}

	var types []string
	if wf.On.PullRequest != nil {
		types = append(types, wf.On.PullRequest.Types...)
	}
	if wf.On.PullRequestTarget != nil {
		types = append(types, wf.On.PullRequestTarget.Types...)
	}

	if len(types) == 0 {
		return defaultTypes
	}

	// Dedupe
	slices.Sort(types)
	_ = slices.Compact(types)

	return types
}

func (wf gitHubWorkflow) runsOnSynchronize() bool {
	types := wf.types()

	return slices.Contains(types, "synchronize")
}
