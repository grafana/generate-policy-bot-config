package main

import (
	"fmt"
	"io"
	"iter"
	"log/slog"
	"slices"
	"strings"

	"github.com/palantir/policy-bot/policy"
	"github.com/palantir/policy-bot/policy/approval"
	"github.com/palantir/policy-bot/policy/common"
	"github.com/palantir/policy-bot/policy/predicate"
	"github.com/redmatter/go-globre"
	"golang.org/x/exp/maps"
	"gopkg.in/yaml.v3"
)

const defaultToApproval = "default to approval"

// skippedOrSuccess contains the conclusions we always look for in a workflow
// run's conclusion. We only look at workflow runs which happened at all
// (because of the path filters). But we don't know if there was an `if`
// condition on any/all of the jobs. If there was, that's fine, and we should
// allow the approval rule.
var skippedOrSuccess = predicate.AllowedConclusions{"skipped", "success"}

// regexpsFromGlobs converts a sequence of glob patterns into a sequence of regular
// expressions. A conversion is needed because policy-bot takes regular
// expressions and GitHub Actions workflows use glob patterns.
func regexpStringsFromGlobs(globs iter.Seq[string]) iter.Seq[string] {
	return func(yield func(string) bool) {
		for glob := range globs {
			regexp := globre.RegexFromGlob(glob)

			if !yield(regexp) {
				return
			}
		}
	}
}

// regexpsFromGlobs converts a sequence of glob patterns into a sequence of
// regular expressions in `policy-bot`'s `common.Regexp` wrapper type. If any of
// the glob patterns are invalid, it returns an error containing the invalid
// globs.
func regexpsFromGlobs(globs []string) ([]common.Regexp, error) {
	var errors errInvalidGlobs

	regexps := make([]common.Regexp, len(globs))

	for i, glob := range globs {
		regexp, err := common.NewRegexp(globre.RegexFromGlob(glob))
		if err != nil {
			errors.Globs = append(errors.Globs, glob)
			continue
		}

		regexps[i] = regexp
	}

	if len(errors.Globs) > 0 {
		return nil, errors
	}

	if len(regexps) == 0 {
		return nil, nil
	}

	return regexps, nil
}

func branchRegexp(branches []string) (common.Regexp, error) {
	if len(branches) == 0 {
		return common.Regexp{}, nil
	}

	branchFilterRegexps := slices.Collect(regexpStringsFromGlobs(slices.Values(branches)))

	regex, err := common.NewRegexp(fmt.Sprintf("(%s)", strings.Join(branchFilterRegexps, "|")))

	return regex, err
}

func makeApprovalRule(path string, wf gitHubWorkflow) (*approval.Rule, error) {
	name := fmt.Sprintf("Workflow %s succeeded or skipped", path)

	pathRegexes, err := regexpsFromGlobs(wf.paths())
	if err != nil {
		return nil, fmt.Errorf("couldn't parse path filters: %w", err)
	}

	ignoreRegexes, err := regexpsFromGlobs(wf.ignorePaths())
	if err != nil {
		return nil, fmt.Errorf("couldn't parse ignore path filters: %w", err)
	}

	var preds predicate.Predicates
	if len(pathRegexes) > 0 || len(ignoreRegexes) > 0 {
		preds.ChangedFiles = &predicate.ChangedFiles{
			Paths:       pathRegexes,
			IgnorePaths: ignoreRegexes,
		}
	}

	branchRegexp, err := branchRegexp(wf.branches())
	if err != nil {
		return nil, fmt.Errorf("couldn't parse branch filters: %w", err)
	}

	if branchRegexp != (common.Regexp{}) {
		preds.TargetsBranch = &predicate.TargetsBranch{
			Pattern: branchRegexp,
		}
	}

	requires := approval.Requires{
		Conditions: predicate.Predicates{
			HasWorkflowResult: &predicate.HasWorkflowResult{
				Conclusions: skippedOrSuccess,
				Workflows:   []string{path},
			},
		},
	}

	return &approval.Rule{
		Name:       name,
		Predicates: preds,
		Requires:   requires,
	}, nil
}

func (workflows gitHubWorkflowCollection) policyBotConfig() policy.Config {
	approvalRules := make([]*approval.Rule, 0, len(workflows))
	policyApprovals := make([]interface{}, 0, len(workflows))

	paths := maps.Keys(workflows)
	slices.Sort(paths)

	for _, path := range paths {
		wf := workflows[path]

		slog.Debug(
			"building approval rule",
			"path", path,
			"n_path_filters", len(wf.paths()),
			"n_ignore_path_filters", len(wf.ignorePaths()),
		)

		approvalRule, err := makeApprovalRule(path, wf)
		if err != nil {
			slog.Warn("failed to build approval rule", "path", path, "error", err)
			continue
		}

		approvalRules = append(approvalRules, approvalRule)
		policyApprovals = append(policyApprovals, approvalRule.Name)
	}

	var andApprovals approval.Policy
	if len(policyApprovals) > 0 {
		// If there are any workflows, add a "default to approval" rule.
		// This is needed because if all the rules are skipped, the branch is
		// not approved. So PRs which don't cause any of the conditional
		// workflows to run would get stuck.
		approvalRules = append(approvalRules, &approval.Rule{
			Name: defaultToApproval,
		})
		policyApprovals = append(policyApprovals, defaultToApproval)

		andApprovals = approval.Policy{
			map[string]interface{}{
				"and": policyApprovals,
			},
		}
	}

	config := policy.Config{
		Policy: policy.Policy{
			Approval: approval.Policy(
				andApprovals,
			),
		},
		ApprovalRules: approvalRules,
	}

	slog.Info("built Policy Bot config", "n_workflows", len(approvalRules))

	return config
}

func writeYamlToWriter(w io.Writer, data interface{}) error {
	enc := yaml.NewEncoder(w)
	enc.SetIndent(2)
	defer enc.Close()

	if err := enc.Encode(data); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}
