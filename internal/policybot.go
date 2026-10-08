package internal

import (
	"fmt"
	"io"
	"log/slog"
	"regexp"
	"slices"
	"strings"

	"github.com/palantir/policy-bot/policy"
	"github.com/palantir/policy-bot/policy/approval"
	"github.com/palantir/policy-bot/policy/common"
	"github.com/palantir/policy-bot/policy/predicate"
	"golang.org/x/exp/maps"
	"gopkg.in/yaml.v3"
)

const DefaultToApproval = "default to approval"

// SkippedOrSuccess contains the conclusions we always look for in a workflow
// run's conclusion. We only look at workflow runs which happened at all
// (because of the path filters). But we don't know if there was an `if`
// condition on any/all of the jobs. If there was, that's fine, and we should
// allow the approval rule.
var SkippedOrSuccess = predicate.AllowedConclusions{"skipped", "success"}

// requirement is one combination of path and branch filter terms under which a
// workflow runs.
type requirement struct {
	path   filterTerm
	branch filterTerm

	// needsWorkflow is whether the workflow only runs if the pull request
	// doesn't delete it.
	needsWorkflow bool
}

func (r requirement) sameTerms(other requirement) bool {
	return r.path.equal(other.path) && r.branch.equal(other.branch)
}

// requirements returns the combinations of path and branch filter terms under
// which the workflow runs. It runs if any of them matches a pull request. If
// two events have the same terms, the workflow needs to exist only if both
// events need it.
func requirements(wf GitHubWorkflow) ([]requirement, error) {
	var result []requirement

	for _, event := range wf.events() {
		pathTerms, err := event.pathTerms()
		if err != nil {
			return nil, err
		}

		branchTerms, err := event.branchTerms()
		if err != nil {
			return nil, err
		}

		for _, path := range pathTerms {
			for _, branch := range branchTerms {
				r := requirement{path: path, branch: branch, needsWorkflow: !event.fromBase}

				i := slices.IndexFunc(result, r.sameTerms)
				if i == -1 {
					result = append(result, r)
					continue
				}

				result[i].needsWorkflow = result[i].needsWorkflow && r.needsWorkflow
			}
		}
	}

	return result, nil
}

// changedFilesPredicate returns the changed_files predicate for a path term, or
// nil if the term doesn't restrict the changed files.
func changedFilesPredicate(term filterTerm) (*predicate.ChangedFiles, error) {
	if term.match == nil {
		return nil, nil
	}

	paths, err := compileRegexps(term.match)
	if err != nil {
		return nil, err
	}

	ignore, err := compileRegexps(term.except)
	if err != nil {
		return nil, err
	}

	return &predicate.ChangedFiles{Paths: paths, IgnorePaths: ignore}, nil
}

// branchPredicates returns the predicates for a branch term: a pull request's
// base branch has to match targets, and not exempt. Either is nil if the term
// doesn't restrict it.
func branchPredicates(term filterTerm) (targets, exempt *predicate.TargetsBranch, err error) {
	if term.match != nil {
		pattern, err := alternation(term.match)
		if err != nil {
			return nil, nil, err
		}

		targets = &predicate.TargetsBranch{Pattern: pattern}
	}

	if term.except != nil {
		pattern, err := alternation(term.except)
		if err != nil {
			return nil, nil, err
		}

		exempt = &predicate.TargetsBranch{Pattern: pattern}
	}

	return targets, exempt, nil
}

func compileRegexps(sources []string) ([]common.Regexp, error) {
	var regexps []common.Regexp
	for _, source := range sources {
		re, err := common.NewRegexp(source)
		if err != nil {
			return nil, err
		}

		regexps = append(regexps, re)
	}

	return regexps, nil
}

// alternation returns a regular expression which matches if any of sources
// does.
func alternation(sources []string) (common.Regexp, error) {
	return common.NewRegexp(fmt.Sprintf("(%s)", strings.Join(sources, "|")))
}

// makeApprovalRules returns the approval rules for a workflow, and the entries
// which refer to them in the policy's `and` list. There's a rule for each
// requirement, which requires the workflow to succeed or be skipped. A
// requirement with an exemption also has a rule which approves the pull request
// when its base branch is exempt, and the requirement's two rules go under an
// `or`.
func makeApprovalRules(path string, wf GitHubWorkflow) ([]*approval.Rule, []interface{}, error) {
	reqs, err := requirements(wf)
	if err != nil {
		return nil, nil, err
	}

	workflowRegexp, err := common.NewRegexp("^" + regexp.QuoteMeta(path) + "$")
	if err != nil {
		return nil, nil, errInvalidWorkflowPath{Path: path, Err: err}
	}

	var rules []*approval.Rule
	var entries []interface{}

	for i, req := range reqs {
		suffix := ""
		if len(reqs) > 1 {
			suffix = fmt.Sprintf(" (%d)", i+1)
		}

		changedFiles, err := changedFilesPredicate(req.path)
		if err != nil {
			return nil, nil, err
		}

		targets, exempt, err := branchPredicates(req.branch)
		if err != nil {
			return nil, nil, err
		}

		var notDeleted *predicate.FileNotDeleted
		if req.needsWorkflow {
			notDeleted = &predicate.FileNotDeleted{Paths: []common.Regexp{workflowRegexp}}
		}

		rule := &approval.Rule{
			Name: fmt.Sprintf("Workflow %s succeeded or skipped%s", path, suffix),
			Predicates: predicate.Predicates{
				ChangedFiles:   changedFiles,
				TargetsBranch:  targets,
				FileNotDeleted: notDeleted,
			},
			Requires: approval.Requires{
				Conditions: predicate.Predicates{
					HasWorkflowResult: &predicate.HasWorkflowResult{
						Conclusions: SkippedOrSuccess,
						Workflows:   []string{path},
					},
				},
			},
		}
		rules = append(rules, rule)

		if exempt == nil {
			entries = append(entries, rule.Name)
			continue
		}

		exemption := &approval.Rule{
			Name:       fmt.Sprintf("Workflow %s not required for this base branch%s", path, suffix),
			Predicates: predicate.Predicates{TargetsBranch: exempt},
		}
		rules = append(rules, exemption)
		entries = append(entries, map[string]interface{}{"or": []interface{}{rule.Name, exemption.Name}})
	}

	return rules, entries, nil
}

func (workflows GitHubWorkflowCollection) PolicyBotConfig() policy.Config {
	approvalRules := make([]*approval.Rule, 0, len(workflows))
	policyApprovals := make([]interface{}, 0, len(workflows))

	paths := maps.Keys(workflows)
	slices.Sort(paths)

	nWorkflows := 0
	for _, path := range paths {
		wf := workflows[path]

		rules, entries, err := makeApprovalRules(path, wf)
		if err != nil {
			slog.Warn("failed to build approval rules", "path", path, "error", err)
			continue
		}

		if len(rules) == 0 {
			slog.Info("workflow's filters don't match any pull request, so it is never required", "path", path)
			continue
		}

		slog.Debug("built approval rules", "path", path, "n_rules", len(rules))

		approvalRules = append(approvalRules, rules...)
		policyApprovals = append(policyApprovals, entries...)
		nWorkflows++
	}

	var andApprovals approval.Policy
	if len(policyApprovals) > 0 {
		// If there are any workflows, add a "default to approval" rule.
		// This is needed because if all the rules are skipped, the branch is
		// not approved. So PRs which don't cause any of the conditional
		// workflows to run would get stuck.
		approvalRules = append(approvalRules, &approval.Rule{
			Name: DefaultToApproval,
		})
		policyApprovals = append(policyApprovals, DefaultToApproval)

		andApprovals = approval.Policy{
			map[string]interface{}{
				"or": []interface{}{
					map[string]interface{}{
						"and": policyApprovals,
					},
				},
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

	slog.Info("built Policy Bot config", "n_workflows", nWorkflows)

	return config
}

func WriteYamlToWriter(w io.Writer, data interface{}) error {
	enc := yaml.NewEncoder(w)
	enc.SetIndent(2)
	defer enc.Close()

	if err := enc.Encode(data); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}
