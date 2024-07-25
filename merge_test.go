package main

import (
	"testing"

	"github.com/palantir/policy-bot/policy"
	"github.com/palantir/policy-bot/policy/approval"
	"github.com/palantir/policy-bot/policy/common"
	"github.com/palantir/policy-bot/policy/disapproval"
	"github.com/palantir/policy-bot/policy/predicate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMergeConfigs_MergeApprovalPolicies(t *testing.T) {
	left := policy.Config{
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
	right := policy.Config{
		Policy: policy.Policy{
			Approval: approval.Policy{
				map[string][]string{
					"or": {
						"rule3",
						"rule4",
					},
				},
			},
		},
		ApprovalRules: []*approval.Rule{
			{Name: "rule3"},
			{Name: "rule4"},
		},
	}
	expected := policy.Config{
		Policy: policy.Policy{
			Approval: approval.Policy{
				map[string][]string{
					"and": {
						"rule1",
						"rule2",
					},
				},
				map[string][]string{
					"or": {
						"rule3",
						"rule4",
					},
				},
			},
		},
		ApprovalRules: []*approval.Rule{
			{Name: "rule1"},
			{Name: "rule2"},
			{Name: "rule3"},
			{Name: "rule4"},
		},
	}

	merged, err := mergeConfigs(left, right)
	require.NoError(t, err)
	assert.Equal(t, expected, merged)
}

func TestMergeConfigs_MergeWithDisapprovalInRightConfig(t *testing.T) {
	left := policy.Config{
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
	right := policy.Config{
		Policy: policy.Policy{
			Disapproval: &disapproval.Policy{
				Predicates: predicate.Predicates{
					ChangedFiles: &predicate.ChangedFiles{
						Paths: mustRegexpsFromGlobs(t, []string{"*.go"}),
					},
				},
				Options: disapproval.Options{
					Methods: disapproval.Methods{
						Disapprove: &common.Methods{
							Comments: []string{"Disapproved"},
						},
					},
				},
			},
		},
	}
	expected := policy.Config{
		Policy: policy.Policy{
			Approval: approval.Policy{
				map[string][]string{
					"and": {
						"rule1",
						"rule2",
					},
				},
			},
			Disapproval: &disapproval.Policy{
				Predicates: predicate.Predicates{
					ChangedFiles: &predicate.ChangedFiles{
						Paths: mustRegexpsFromGlobs(t, []string{"*.go"}),
					},
				},
				Options: disapproval.Options{
					Methods: disapproval.Methods{
						Disapprove: &common.Methods{
							Comments: []string{"Disapproved"},
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

	merged, err := mergeConfigs(left, right)
	require.NoError(t, err)
	assert.Equal(t, expected, merged)
}

func TestMergeConfigs_ErrorOnBothConfigsHavingDisapproval(t *testing.T) {
	left := policy.Config{
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
	right := policy.Config{
		Policy: policy.Policy{
			Disapproval: &disapproval.Policy{
				Predicates: predicate.Predicates{
					ChangedFiles: &predicate.ChangedFiles{
						Paths: mustRegexpsFromGlobs(t, []string{"*.js"}),
					},
				},
			},
		},
	}

	_, err := mergeConfigs(left, right)
	require.ErrorIs(t, err, errMergeDisapproval{})
}

func TestCheckApprovalRuleDupes(t *testing.T) {
	tests := []struct {
		name     string
		rules    []*approval.Rule
		errNames []string
	}{
		{
			name: "No duplicates",
			rules: []*approval.Rule{
				{Name: "rule1"},
				{Name: "rule2"},
				{Name: "rule3"},
			},
		},
		{
			name: "One duplicate",
			rules: []*approval.Rule{
				{Name: "rule1"},
				{Name: "rule2"},
				{Name: "rule1"},
			},
			errNames: []string{"rule1"},
		},
		{
			name: "Multiple duplicates",
			rules: []*approval.Rule{
				{Name: "rule1"},
				{Name: "rule2"},
				{Name: "rule1"},
				{Name: "rule3"},
				{Name: "rule2"},
			},
			errNames: []string{"rule1", "rule2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkApprovalRuleDupes(tt.rules)

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
