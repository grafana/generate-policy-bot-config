package internal

import (
	"fmt"
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

// mergeApprovals handles merging two approval policies. The approval policies
// are slices. We need to do something a little bit more complicated than simply
// appending the two slices together. Our generated policies are under a top
// level `or` key. If we simply merge the two slices, we'll end up with two `or`
// keys, which is valid but not what we want. So if we find an "or" policy with
// a special value of `MERGE_WITH_GENERATED`, we merge the generated policy with
// the mergeWith policy's first `or` key, which is the generated policy.
func mergeApprovals(generatedApproval, mergeWithApproval approval.Policy) (approval.Policy, error) {
	if len(generatedApproval) == 0 {
		return mergeWithApproval, nil
	}

	if len(mergeWithApproval) == 0 {
		return generatedApproval, nil
	}

	for _, incoming := range mergeWithApproval {
		operators, ok := incoming.(map[string]interface{})
		if !ok {
			generatedApproval = append(generatedApproval, incoming)
			continue
		}

		for operator, policy := range operators {
			if operator != "or" {
				generatedApproval = append(generatedApproval, incoming)
				continue
			}

			orSlice, ok := policy.([]interface{})
			if !ok {
				generatedApproval = append(generatedApproval, incoming)
				continue
			}

			if orSlice[0] != "MERGE_WITH_GENERATED" {
				generatedApproval = append(generatedApproval, incoming)
				continue
			}

			approvals, ok := generatedApproval[0].(map[string]interface{})
			if !ok {
				return nil, ErrInvalidPolicyBotConfig{Err: fmt.Errorf("generated approval is not a map")}
			}

			mergeWith, ok := approvals["or"]
			if !ok {
				return nil, ErrInvalidPolicyBotConfig{Err: fmt.Errorf("the generated approval does not have an `or` field")}
			}

			mergeWithI, ok := mergeWith.([]interface{})
			if !ok {
				return nil, ErrInvalidPolicyBotConfig{Err: fmt.Errorf("generated approval's `or` field is not a slice")}
			}

			mergeWith = append(mergeWithI, orSlice[1:]...)

			approvals["or"] = mergeWith
		}
	}

	return generatedApproval, nil
}

// MergeConfigs combines a generated config with an existing config using deep merging.
// The existing config takes precedence over the generated config.
func MergeConfigs(generated, mergeWith policy.Config) (policy.Config, error) {
	slog.Debug("merging user-provided policy with generated policy")

	// Don't know how to sensibly merge disapprovals (and anyway, we don't
	// generate one so one side should always be empty). error if both sides
	// have disapprovals.
	if generated.Policy.Disapproval != nil && mergeWith.Policy.Disapproval != nil {
		return policy.Config{}, errMergeDisapproval{}
	}

	disapproval := generated.Policy.Disapproval
	if disapproval == nil {
		disapproval = mergeWith.Policy.Disapproval
	}

	approvals, err := mergeApprovals(generated.Policy.Approval, mergeWith.Policy.Approval)
	if err != nil {
		return policy.Config{}, err
	}

	merged := policy.Config{
		Policy: policy.Policy{
			Approval:    approvals,
			Disapproval: disapproval,
		},
		ApprovalRules: append(generated.ApprovalRules, mergeWith.ApprovalRules...),
	}

	if err := checkApprovalRuleDupes(merged.ApprovalRules); err != nil {
		return policy.Config{}, err
	}

	slog.Debug(
		"merged policies",
		"n_approval_rules_left", len(generated.ApprovalRules),
		"n_approval_rules_right", len(mergeWith.ApprovalRules),
		"n_approval_rules_merged", len(merged.ApprovalRules),
		"n_approval_policies_left", len(generated.Policy.Approval),
		"n_approval_policies_right", len(mergeWith.Policy.Approval),
		"n_approval_policies_merged", len(merged.Policy.Approval),
		"has_disapproval_left", generated.Policy.Disapproval != nil,
		"has_disapproval_right", mergeWith.Policy.Disapproval != nil,
	)

	return merged, nil
}
