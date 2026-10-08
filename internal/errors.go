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

// errInvalidWorkflowPath is returned when a workflow's path can't be matched by
// a regular expression, because it isn't valid UTF-8.
type errInvalidWorkflowPath struct {
	Path string
	Err  error
}

func (e errInvalidWorkflowPath) Error() string {
	return fmt.Sprintf("can't match workflow path %q: %v", e.Path, e.Err)
}

func (e errInvalidWorkflowPath) Unwrap() error {
	return e.Err
}

// errInvalidFilterPattern is returned when a workflow's branch or path filter
// isn't a valid GitHub Actions filter pattern. Err is one of the errors below,
// which says what's wrong with it.
type errInvalidFilterPattern struct {
	Pattern string
	Err     error
}

func (e errInvalidFilterPattern) Error() string {
	return fmt.Sprintf("invalid filter pattern %q: %v", e.Pattern, e.Err)
}

func (e errInvalidFilterPattern) Unwrap() error {
	return e.Err
}

// errInvalidFilterPatterns is returned when one or more of the patterns in a
// filter list are invalid. It has an error for each of them.
type errInvalidFilterPatterns []errInvalidFilterPattern

func (e errInvalidFilterPatterns) Error() string {
	messages := make([]string, len(e))
	for i, err := range e {
		messages[i] = err.Error()
	}

	return strings.Join(messages, "; ")
}

func (e errInvalidFilterPatterns) Unwrap() []error {
	errs := make([]error, len(e))
	for i, err := range e {
		errs[i] = err
	}

	return errs
}

// errInvalidFilter is returned when one of a workflow event's filters, like
// `paths` or `branches-ignore`, can't be used.
type errInvalidFilter struct {
	Event  string
	Filter string
	Err    error
}

func (e errInvalidFilter) Error() string {
	return fmt.Sprintf("invalid %s filter for %s: %v", e.Filter, e.Event, e.Err)
}

func (e errInvalidFilter) Unwrap() error {
	return e.Err
}

// errConflictingFilters is returned when a workflow event has both a filter and
// its `-ignore` variant, like `paths` and `paths-ignore`, which GitHub doesn't
// allow.
type errConflictingFilters struct {
	Event  string
	Filter string
}

func (e errConflictingFilters) Error() string {
	return fmt.Sprintf("%s can't have both %s and %s-ignore", e.Event, e.Filter, e.Filter)
}

// errMisplacedQuantifier is returned for a "?" or "+" which follows a wildcard
// or another "?" or "+".
type errMisplacedQuantifier struct {
	Quantifier string
	After      string
}

func (e errMisplacedQuantifier) Error() string {
	return fmt.Sprintf("%q can't follow %q", e.Quantifier, e.After)
}

// errTrailingBackslash is returned for a "\" at the end of a pattern.
type errTrailingBackslash struct{}

func (errTrailingBackslash) Error() string {
	return `"\\" at the end has nothing to escape`
}

// errInvalidEscape is returned for a "\" before a character which can't be
// escaped.
type errInvalidEscape struct {
	Escape string
}

func (e errInvalidEscape) Error() string {
	return fmt.Sprintf("%q isn't a valid escape", e.Escape)
}

// errUnclosedBracket is returned for a "[" with no "]" after it.
type errUnclosedBracket struct{}

func (errUnclosedBracket) Error() string {
	return `"[" has no closing "]"`
}

// errEmptyBrackets is returned for "[]".
type errEmptyBrackets struct{}

func (errEmptyBrackets) Error() string {
	return `"[]" is empty`
}

// errNotAlphanumeric is returned for a character in brackets which isn't an
// ASCII letter or digit.
type errNotAlphanumeric struct {
	Char string
}

func (e errNotAlphanumeric) Error() string {
	return fmt.Sprintf("%q in brackets isn't a letter or digit", e.Char)
}

// errRangeOutOfBounds is returned for a range in brackets whose ends aren't
// both within A-z, or both within 0-9.
type errRangeOutOfBounds struct {
	Range string
}

func (e errRangeOutOfBounds) Error() string {
	return fmt.Sprintf("range %q must be within A-z or 0-9", e.Range)
}

// errBackwardsRange is returned for a range in brackets whose first end comes
// after its second.
type errBackwardsRange struct {
	Range string
}

func (e errBackwardsRange) Error() string {
	return fmt.Sprintf("range %q goes backwards", e.Range)
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
