package internal

import (
	"fmt"

	"github.com/palantir/policy-bot/policy/approval"
	"gopkg.in/yaml.v3"
)

// Config is a Policy Bot configuration which is about to be written out.
//
// It mirrors policy-bot's own `policy.Config`, except that the parts which can
// come from a hand-written config — the approval rules and the disapproval
// policy — are held as raw YAML nodes instead of policy-bot's types.
//
// This is because policy-bot's types are not safe to round-trip through YAML.
// They use the difference between an absent field and an explicitly empty one
// to decide whether to fall back to a default, but the fields are tagged
// `omitempty`, so an empty value is indistinguishable from an absent one once
// it has been encoded again. `methods.comments`, which lists the comments that
// count as approval, is the clearest example: setting it to `[]` is the only
// way to turn off approval by comment, but decoding and re-encoding it drops
// the field, and Policy Bot restores its ":+1:" and "👍" defaults. Policy Bot
// itself only ever reads configs, so this costs it nothing; we are the only
// ones who write them.
//
// Nodes sidestep this: whatever somebody wrote by hand is copied through
// untouched, and we don't have to track which of policy-bot's fields
// distinguish "unset" from "empty". Comments and key order survive too.
type Config struct {
	Policy        Policy      `yaml:"policy,omitempty"`
	ApprovalRules []yaml.Node `yaml:"approval_rules,omitempty"`
}

// Policy is the `policy` section of a Config.
//
// Approval keeps policy-bot's type because merging has to look inside it (see
// mergeApprovals), and it is safe to round-trip: `approval.Policy` is a tree of
// plain `interface{}` values, so it holds exactly what was written. Comments in
// this section are still lost, which is a smaller problem than a rule quietly
// changing meaning.
//
// Disapproval has to be a `yaml.Node` and not a `*yaml.Node`: yaml.v3 decodes
// into either, but only encodes the former. A pointer is encoded as if it were
// an ordinary struct, which produces `{}` and silently loses the policy. Use
// IsZero to test whether one was set.
type Policy struct {
	Approval    approval.Policy `yaml:"approval,omitempty"`
	Disapproval yaml.Node       `yaml:"disapproval,omitempty"`
}

// yamlNode converts a value into the YAML node that it marshals to, so that
// generated config can be held in the same form as hand-written config.
func yamlNode(value any) (yaml.Node, error) {
	raw, err := yaml.Marshal(value)
	if err != nil {
		return yaml.Node{}, fmt.Errorf("failed to marshal value: %w", err)
	}

	var document yaml.Node
	if err := yaml.Unmarshal(raw, &document); err != nil {
		return yaml.Node{}, fmt.Errorf("failed to unmarshal value: %w", err)
	}

	// Unmarshaling into a node always yields a document node wrapping the
	// value we actually want.
	if document.Kind == yaml.DocumentNode && len(document.Content) == 1 {
		return *document.Content[0], nil
	}

	return document, nil
}

// approvalRuleNodes converts generated approval rules into raw YAML nodes, so
// that they can sit in the same slice as the hand-written ones.
func approvalRuleNodes(rules []*approval.Rule) ([]yaml.Node, error) {
	nodes := make([]yaml.Node, 0, len(rules))

	for _, rule := range rules {
		node, err := yamlNode(rule)
		if err != nil {
			return nil, ErrInvalidPolicyBotConfig{
				Err: fmt.Errorf("failed to convert generated approval rule %q: %w", rule.Name, err),
			}
		}

		nodes = append(nodes, node)
	}

	return nodes, nil
}

// approvalRuleName returns the value of an approval rule's `name` field, or the
// empty string if it doesn't have one. Rules are held as nodes, so the name has
// to be looked up rather than read from a struct field.
func approvalRuleName(rule yaml.Node) string {
	if rule.Kind != yaml.MappingNode {
		return ""
	}

	// The children of a mapping node alternate between keys and values.
	for i := 0; i+1 < len(rule.Content); i += 2 {
		if rule.Content[i].Value == "name" {
			return rule.Content[i+1].Value
		}
	}

	return ""
}
