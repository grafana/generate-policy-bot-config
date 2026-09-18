package internal

import (
	"bytes"
	"testing"

	"github.com/palantir/policy-bot/policy"
	"github.com/palantir/policy-bot/policy/approval"
	"github.com/palantir/policy-bot/policy/disapproval"
	"github.com/palantir/policy-bot/policy/predicate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// mustParseConfig parses a hand-written config the way the tool does.
func mustParseConfig(t *testing.T, config string) Config {
	t.Helper()

	var parsed Config
	require.NoError(t, yaml.Unmarshal([]byte(config), &parsed))

	return parsed
}

// mustParseRules parses the approval rules of a hand-written config.
func mustParseRules(t *testing.T, config string) []yaml.Node {
	t.Helper()

	return mustParseConfig(t, config).ApprovalRules
}

// mustWriteConfig writes a config out the way the tool does, so that tests can
// assert on what a user would end up with.
func mustWriteConfig(t *testing.T, config Config) string {
	t.Helper()

	var buf bytes.Buffer
	require.NoError(t, WriteYamlToWriter(&buf, config))

	return buf.String()
}

func TestMergeConfigs_MergeApprovalPolicies(t *testing.T) {
	generated := policy.Config{
		Policy: policy.Policy{
			Approval: approval.Policy{
				map[string][]string{
					"and": {
						"rule1",
						"rule2",
					},
				},
			},
		},
		ApprovalRules: []*approval.Rule{
			{Name: "rule1"},
			{Name: "rule2"},
		},
	}
	mergeWith := mustParseConfig(t, `
policy:
  approval:
    - or:
        - rule3
        - rule4

approval_rules:
  - name: rule3
  - name: rule4
`)

	expected := `policy:
  approval:
    - and:
        - rule1
        - rule2
    - or:
        - rule3
        - rule4
approval_rules:
  - name: rule1
  - name: rule2
  - name: rule3
  - name: rule4
`

	merged, err := MergeConfigs(generated, mergeWith)
	require.NoError(t, err)
	assert.Equal(t, expected, mustWriteConfig(t, merged))
}

func TestMergeConfigs_MergeApprovalPoliciesWithGeneratedApproval(t *testing.T) {
	generated := policy.Config{
		Policy: policy.Policy{
			Approval: approval.Policy{
				map[string]interface{}{
					"or": []interface{}{
						map[string]interface{}{
							"and": []interface{}{
								"rule1",
								"rule2",
							},
						},
					},
				},
			},
		},
		ApprovalRules: []*approval.Rule{
			{Name: "rule1"},
			{Name: "rule2"},
		},
	}
	mergeWith := mustParseConfig(t, `
policy:
  approval:
    - or:
        - MERGE_WITH_GENERATED
        - rule3
    - and:
        - rule4

approval_rules:
  - name: rule3
  - name: rule4
`)

	expected := `policy:
  approval:
    - or:
        - and:
            - rule1
            - rule2
        - rule3
    - and:
        - rule4
approval_rules:
  - name: rule1
  - name: rule2
  - name: rule3
  - name: rule4
`

	merged, err := MergeConfigs(generated, mergeWith)
	require.NoError(t, err)
	assert.Equal(t, expected, mustWriteConfig(t, merged))
}

// TestMergeConfigs_PreservesExplicitlyEmptyValues checks that a field which is
// set to an explicitly empty value survives the merge. Policy Bot's own types
// can't express this — they tag these fields `omitempty`, so encoding them again
// drops them — which is why hand-written config is kept as raw YAML.
//
// `methods.comments` is the case that matters in practice: an empty list is the
// only way to say that no comment should count as approval, and losing it means
// Policy Bot silently reinstates its ":+1:" and "👍" defaults.
func TestMergeConfigs_PreservesExplicitlyEmptyValues(t *testing.T) {
	generated := policy.Config{
		ApprovalRules: []*approval.Rule{
			{Name: "rule1"},
		},
	}
	mergeWith := mustParseConfig(t, `
approval_rules:
  - name: needs a review from the team
    requires:
      count: 1
      teams:
        - grafana/some-team
    options:
      methods:
        comments: []
        github_review: true
`)

	expected := `approval_rules:
  - name: rule1
  - name: needs a review from the team
    requires:
      count: 1
      teams:
        - grafana/some-team
    options:
      methods:
        comments: []
        github_review: true
`

	merged, err := MergeConfigs(generated, mergeWith)
	require.NoError(t, err)
	assert.Equal(t, expected, mustWriteConfig(t, merged))
}

// TestMergeConfigs_PreservesCommentsAndFormatting checks that hand-written rules
// come out the way they went in, rather than reformatted into the shape of
// Policy Bot's structs. Comments are worth keeping because they are often the
// only explanation of why a rule exists.
func TestMergeConfigs_PreservesCommentsAndFormatting(t *testing.T) {
	generated := policy.Config{
		ApprovalRules: []*approval.Rule{
			{Name: "rule1"},
		},
	}
	mergeWith := mustParseConfig(t, `
approval_rules:
  # Anybody with write access can override the policies by commenting.
  - name: override policies
    requires:
      count: 1
      permissions:
        - write
    options:
      methods:
        comments:
          - "policy bot: approve"
        github_review: false
`)

	expected := `approval_rules:
  - name: rule1
  # Anybody with write access can override the policies by commenting.
  - name: override policies
    requires:
      count: 1
      permissions:
        - write
    options:
      methods:
        comments:
          - "policy bot: approve"
        github_review: false
`

	merged, err := MergeConfigs(generated, mergeWith)
	require.NoError(t, err)
	assert.Equal(t, expected, mustWriteConfig(t, merged))
}

func TestMergeConfigs_MergeWithDisapprovalInmergeWithConfig(t *testing.T) {
	generated := policy.Config{
		Policy: policy.Policy{
			Approval: approval.Policy{
				map[string][]string{
					"and": {
						"rule1",
					},
				},
			},
		},
		ApprovalRules: []*approval.Rule{
			{Name: "rule1"},
		},
	}
	mergeWith := mustParseConfig(t, `
policy:
  disapproval:
    if:
      changed_files:
        paths:
          - \.go$
    options:
      methods:
        disapprove:
          comments:
            - Disapproved
`)

	expected := `policy:
  approval:
    - and:
        - rule1
  disapproval:
    if:
      changed_files:
        paths:
          - \.go$
    options:
      methods:
        disapprove:
          comments:
            - Disapproved
approval_rules:
  - name: rule1
`

	merged, err := MergeConfigs(generated, mergeWith)
	require.NoError(t, err)
	assert.Equal(t, expected, mustWriteConfig(t, merged))
}

func TestMergeConfigs_ErrorOnBothConfigsHavingDisapproval(t *testing.T) {
	generated := policy.Config{
		Policy: policy.Policy{
			Disapproval: &disapproval.Policy{
				Predicates: predicate.Predicates{
					ChangedFiles: &predicate.ChangedFiles{
						Paths: mustRegexpsFromGlobs(t, []string{"*.go"}),
					},
				},
			},
		},
	}
	mergeWith := mustParseConfig(t, `
policy:
  disapproval:
    if:
      changed_files:
        paths:
          - \.js$
`)

	_, err := MergeConfigs(generated, mergeWith)
	require.ErrorIs(t, err, errMergeDisapproval{})
}

// TestMergeConfigs_GeneratedDisapprovalIsKept covers the case where only the
// generated config has a disapproval policy. We don't generate one today, so
// this is here to make sure it wouldn't be dropped if we ever did.
func TestMergeConfigs_GeneratedDisapprovalIsKept(t *testing.T) {
	generated := policy.Config{
		Policy: policy.Policy{
			Disapproval: &disapproval.Policy{
				Predicates: predicate.Predicates{
					ChangedFiles: &predicate.ChangedFiles{
						Paths: mustRegexpsFromGlobs(t, []string{"*.go"}),
					},
				},
			},
		},
	}
	mergeWith := mustParseConfig(t, `
approval_rules:
  - name: rule1
`)

	merged, err := MergeConfigs(generated, mergeWith)
	require.NoError(t, err)

	require.False(t, merged.Policy.Disapproval.IsZero())
	assert.Contains(t, mustWriteConfig(t, merged), "disapproval:")
}

func TestCheckApprovalRuleDupes(t *testing.T) {
	tests := []struct {
		name     string
		rules    string
		errNames []string
	}{
		{
			name: "No duplicates",
			rules: `
approval_rules:
  - name: rule1
  - name: rule2
  - name: rule3
`,
		},
		{
			name: "One duplicate",
			rules: `
approval_rules:
  - name: rule1
  - name: rule2
  - name: rule1
`,
			errNames: []string{"rule1"},
		},
		{
			name: "Multiple duplicates",
			rules: `
approval_rules:
  - name: rule1
  - name: rule2
  - name: rule1
  - name: rule3
  - name: rule2
`,
			errNames: []string{"rule1", "rule2"},
		},
		{
			name: "Rules with fields in any order",
			rules: `
approval_rules:
  - requires:
      count: 1
    name: rule1
  - name: rule2
  - name: rule1
    requires:
      count: 2
`,
			errNames: []string{"rule1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkApprovalRuleDupes(mustParseRules(t, tt.rules))

			if len(tt.errNames) == 0 {
				require.NoError(t, err)
				return
			}

			require.Error(t, err)
			var dupeErr errMergeDuplicateApprovalRules
			require.ErrorAs(t, err, &dupeErr)
			assert.ElementsMatch(t, tt.errNames, dupeErr.names)
		})
	}
}

func TestApprovalRuleName(t *testing.T) {
	tests := []struct {
		name     string
		rule     string
		expected string
	}{
		{
			name:     "Name first",
			rule:     "name: rule1\nrequires:\n  count: 1\n",
			expected: "rule1",
		},
		{
			name:     "Name last",
			rule:     "requires:\n  count: 1\nname: rule1\n",
			expected: "rule1",
		},
		{
			name:     "No name",
			rule:     "requires:\n  count: 1\n",
			expected: "",
		},
		{
			name:     "Not a mapping",
			rule:     "- rule1\n",
			expected: "",
		},
		{
			name:     "Nested name is not the rule name",
			rule:     "requires:\n  name: not-the-rule-name\n",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var node yaml.Node
			require.NoError(t, yaml.Unmarshal([]byte(tt.rule), &node))
			require.Equal(t, yaml.DocumentNode, node.Kind)

			assert.Equal(t, tt.expected, approvalRuleName(*node.Content[0]))
		})
	}
}
