package main

import (
	"log/slog"

	"github.com/palantir/policy-bot/policy"
	"github.com/palantir/policy-bot/policy/approval"
)

// checkApprovalRuleDupes checks for duplicate approval rule names. We don't
// want to try to merge two rules with the same name. It's easier to reject
// the merge and ask the user to choose a different name.
func checkApprovalRuleDupes(rules []*approval.Rule) error {
	rulesByName := make(map[string]struct{})
	var duplicateNames []string
	for _, rule := range rules {
		if _, ok := rulesByName[rule.Name]; ok {
			duplicateNames = append(duplicateNames, rule.Name)
		}
		rulesByName[rule.Name] = struct{}{}
	}

	if len(duplicateNames) > 0 {
		return errMergeDuplicateApprovalRules{duplicateNames}
	}

	return nil
}

// mergeConfigs combines a generated config with an existing config using deep merging.
// The existing config takes precedence over the generated config.
func mergeConfigs(l, r policy.Config) (policy.Config, error) {
	slog.Debug("merging user-provided policy with generated policy")

	// Don't know how to sensibly merge disapprovals (and anyway, we don't
	// generate one so one side should always be empty). error if both sides
	// have disapprovals.
	if l.Policy.Disapproval != nil && r.Policy.Disapproval != nil {
		return policy.Config{}, errMergeDisapproval{}
	}

	disapproval := l.Policy.Disapproval
	if disapproval == nil {
		disapproval = r.Policy.Disapproval
	}

	merged := policy.Config{
		Policy: policy.Policy{
			Approval:    append(l.Policy.Approval, r.Policy.Approval...),
			Disapproval: disapproval,
		},
		ApprovalRules: append(l.ApprovalRules, r.ApprovalRules...),
	}

	if err := checkApprovalRuleDupes(merged.ApprovalRules); err != nil {
		return policy.Config{}, err
	}

	slog.Debug(
		"merged policies",
		"n_approval_rules_left", len(l.ApprovalRules),
		"n_approval_rules_right", len(r.ApprovalRules),
		"n_approval_rules_merged", len(merged.ApprovalRules),
		"n_approval_policies_left", len(l.Policy.Approval),
		"n_approval_policies_right", len(r.Policy.Approval),
		"n_approval_policies_merged", len(merged.Policy.Approval),
		"has_disapproval_left", l.Policy.Disapproval != nil,
		"has_disapproval_right", r.Policy.Disapproval != nil,
	)

	return merged, nil
}
