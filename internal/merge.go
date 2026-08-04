package internal

import (
	"fmt"
	"log/slog"

	"github.com/palantir/policy-bot/policy"
	"github.com/palantir/policy-bot/policy/approval"
	"gopkg.in/yaml.v3"
)

// checkApprovalRuleDupes checks for duplicate approval rule names. We don't
// want to try to merge two rules with the same name. It's easier to reject
// the merge and ask the user to choose a different name.
func checkApprovalRuleDupes(rules []yaml.Node) error {
	rulesByName := make(map[string]struct{})
	var duplicateNames []string
	for _, rule := range rules {
		name := approvalRuleName(rule)
		if _, ok := rulesByName[name]; ok {
			duplicateNames = append(duplicateNames, name)
		}
		rulesByName[name] = struct{}{}
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

// mergeDisapprovals picks the disapproval policy for the merged config. We
// don't know how to sensibly merge two of them, so error if both sides have
// one. In practice only the hand-written config ever does, because we don't
// generate disapproval policies.
func mergeDisapprovals(generated policy.Config, mergeWith Config) (yaml.Node, error) {
	switch {
	case generated.Policy.Disapproval != nil && !mergeWith.Policy.Disapproval.IsZero():
		return yaml.Node{}, errMergeDisapproval{}

	case generated.Policy.Disapproval != nil:
		node, err := yamlNode(generated.Policy.Disapproval)
		if err != nil {
			return yaml.Node{}, ErrInvalidPolicyBotConfig{
				Err: fmt.Errorf("failed to convert generated disapproval policy: %w", err),
			}
		}

		return node, nil

	default:
		return mergeWith.Policy.Disapproval, nil
	}
}

// MergeConfigs combines a generated config with an existing config using deep merging.
// The existing config takes precedence over the generated config.
//
// The hand-written config arrives as raw YAML nodes and is copied through
// as-is, so that it means the same thing after the merge as it did before. See
// Config for why that matters.
func MergeConfigs(generated policy.Config, mergeWith Config) (Config, error) {
	slog.Debug("merging user-provided policy with generated policy")

	disapproval, err := mergeDisapprovals(generated, mergeWith)
	if err != nil {
		return Config{}, err
	}

	approvals, err := mergeApprovals(generated.Policy.Approval, mergeWith.Policy.Approval)
	if err != nil {
		return Config{}, err
	}

	generatedRules, err := approvalRuleNodes(generated.ApprovalRules)
	if err != nil {
		return Config{}, err
	}

	merged := Config{
		Policy: Policy{
			Approval:    approvals,
			Disapproval: disapproval,
		},
		ApprovalRules: append(generatedRules, mergeWith.ApprovalRules...),
	}

	if err := checkApprovalRuleDupes(merged.ApprovalRules); err != nil {
		return Config{}, err
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
		"has_disapproval_right", !mergeWith.Policy.Disapproval.IsZero(),
	)

	return merged, nil
}
