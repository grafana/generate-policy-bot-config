package internal

import (
	"fmt"
	"strings"
)

// ErrNoWorkflows is returned when no workflows are found in the specified directory.
type ErrNoWorkflows struct {
}

func (e ErrNoWorkflows) Error() string {
	return "no workflows found in directory"
}

// errWorkflowParse is returned when a workflow file cannot be parsed.
type errWorkflowParse struct {
	Err error
}

func (e errWorkflowParse) Error() string {
	return fmt.Sprintf("failed to parse workflow: %v", e.Err)
}

// ErrInvalidWorkflow is returned when a workflow file cannot be parsed or is invalid.
// It will usually wrap an ErrWorkflowParse or ErrUnexpectedType.
type ErrInvalidWorkflow struct {
	Path string
	Err  error
}

func (e ErrInvalidWorkflow) Error() string {
	return fmt.Sprintf("invalid workflow file %s: %s", e.Path, e.Err)
}

func (e ErrInvalidWorkflow) Unwrap() error {
	return e.Err
}

// errUnexpectedType is returned when an unexpected type is encountered during YAML unmarshaling.
type errUnexpectedType struct {
	Type string
}

func (e errUnexpectedType) Error() string {
	return fmt.Sprintf("unexpected type for workflow `on`. got: %s. expected: string, list or map", e.Type)
}

// errInvalidGlobs is returned when an invalid glob pattern is encountered in a workflow file.
type errInvalidGlobs struct {
	Globs []string
}

func (e errInvalidGlobs) Error() string {
	return fmt.Sprintf("invalid globs: %v", strings.Join(e.Globs, ", "))
}

// errMergeDisapproval is returned when we try to merge configs which both
// contain disapproval rules. We don't know how to sensibly merge disapprovals,
// so we error.
type errMergeDisapproval struct{}

func (e errMergeDisapproval) Error() string {
	return "tried to merge two disapproval rules - this is not allowed"
}

// errMergeDuplicateApprovalRules is returned when we try to merge configs which
// each have an approval rule with the same name. We don't want to try to merge
// two rules with the same name. It's easier to reject the merge and ask the
// user to choose a different name.
type errMergeDuplicateApprovalRules struct {
	names []string
}

func (e errMergeDuplicateApprovalRules) Error() string {
	duplicateRules := strings.Join(e.names, ", ")

	return fmt.Sprintf("tried to merge two rules with the same name `%s` - this is not allowed", duplicateRules)
}

// ErrInvalidPolicyBotConfig is returned when a policy bot config cannot be
// unmarshaled.
type ErrInvalidPolicyBotConfig struct {
	Err error
}

func (e ErrInvalidPolicyBotConfig) Error() string {
	return fmt.Sprintf("invalid config: %v", e.Err)
}

func (e ErrInvalidPolicyBotConfig) Unwrap() error {
	return e.Err
}

// Is implements the errors.Is interface. The default implementaion of `Is`
// compares by value, which is not that useful for us. It would only return
// `true` when the wrapped error is exactly the same.
func (e ErrInvalidPolicyBotConfig) Is(target error) bool {
	_, ok := target.(ErrInvalidPolicyBotConfig)
	return ok
}
