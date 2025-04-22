package builder

import (
	"sort"
	"testing"

	v2 "github.com/vngcloud/vngcloud-go-sdk/v2/vngcloud/services/loadbalancer/v2"
)

type L7RuleRequestWithExpect struct {
	l7RuleWrapper
	expectRanking int
}

type l7RuleTestCase struct {
	Name  string
	Rules []L7RuleRequestWithExpect
}

// test cases for L7 rules priority
func Test_L7RulesPriority(t *testing.T) {
	testCases := []l7RuleTestCase{
		{
			Name: "Compare all types of Hostname",
			Rules: []L7RuleRequestWithExpect{
				{
					l7RuleWrapper: l7RuleWrapper{RuleType: v2.PolicyRuleTypeHOSTNAME, RuleValue: "example.com", CompareType: v2.PolicyCompareTypeEQUALS},
					expectRanking: 0,
				},
				{
					l7RuleWrapper: l7RuleWrapper{RuleType: v2.PolicyRuleTypeHOSTNAME, RuleValue: "example.com", CompareType: v2.PolicyCompareTypeSTARTSWITH},
					expectRanking: 1,
				},
				{
					l7RuleWrapper: l7RuleWrapper{RuleType: v2.PolicyRuleTypeHOSTNAME, RuleValue: "example.com", CompareType: v2.PolicyCompareTypeENDSWITH},
					expectRanking: 2,
				},
				{
					l7RuleWrapper: l7RuleWrapper{RuleType: v2.PolicyRuleTypeHOSTNAME, RuleValue: "example.com", CompareType: v2.PolicyCompareTypeCONTAINS},
					expectRanking: 3,
				},
				{
					l7RuleWrapper: l7RuleWrapper{RuleType: v2.PolicyRuleTypeHOSTNAME, RuleValue: "example.com", CompareType: v2.PolicyCompareTypeREGEX},
					expectRanking: 4,
				},
			},
		},
		{
			Name: "compare all types of Path",
			Rules: []L7RuleRequestWithExpect{
				{
					l7RuleWrapper: l7RuleWrapper{RuleType: v2.PolicyRuleTypePATH, RuleValue: "/api", CompareType: v2.PolicyCompareTypeEQUALS},
					expectRanking: 0,
				},
				{
					l7RuleWrapper: l7RuleWrapper{RuleType: v2.PolicyRuleTypePATH, RuleValue: "/api", CompareType: v2.PolicyCompareTypeSTARTSWITH},
					expectRanking: 1,
				},
				{
					l7RuleWrapper: l7RuleWrapper{RuleType: v2.PolicyRuleTypePATH, RuleValue: "/api", CompareType: v2.PolicyCompareTypeENDSWITH},
					expectRanking: 2,
				},
				{
					l7RuleWrapper: l7RuleWrapper{RuleType: v2.PolicyRuleTypePATH, RuleValue: "/api", CompareType: v2.PolicyCompareTypeCONTAINS},
					expectRanking: 3,
				},
				{
					l7RuleWrapper: l7RuleWrapper{RuleType: v2.PolicyRuleTypePATH, RuleValue: "/api", CompareType: v2.PolicyCompareTypeREGEX},
					expectRanking: 4,
				},
			},
		},
		{
			Name: "Hostname real case",
			Rules: []L7RuleRequestWithExpect{
				{
					l7RuleWrapper: l7RuleWrapper{RuleType: v2.PolicyRuleTypeHOSTNAME, RuleValue: "cdn.example.com", CompareType: v2.PolicyCompareTypeEQUALS},
					expectRanking: 0,
				},
				{
					l7RuleWrapper: l7RuleWrapper{RuleType: v2.PolicyRuleTypeHOSTNAME, RuleValue: "example.com", CompareType: v2.PolicyCompareTypeENDSWITH},
					expectRanking: 1,
				},
				{
					l7RuleWrapper: l7RuleWrapper{RuleType: v2.PolicyRuleTypeHOSTNAME, RuleValue: "*.example.com", CompareType: v2.PolicyCompareTypeREGEX},
					expectRanking: 2,
				},
			},
		},
		{
			Name: "Path real case",
			Rules: []L7RuleRequestWithExpect{
				{
					l7RuleWrapper: l7RuleWrapper{RuleType: v2.PolicyRuleTypePATH, RuleValue: "/", CompareType: v2.PolicyCompareTypeSTARTSWITH},
					expectRanking: 2,
				},
				{
					l7RuleWrapper: l7RuleWrapper{RuleType: v2.PolicyRuleTypePATH, RuleValue: "/api", CompareType: v2.PolicyCompareTypeEQUALS},
					expectRanking: 1,
				},
				{
					l7RuleWrapper: l7RuleWrapper{RuleType: v2.PolicyRuleTypePATH, RuleValue: "/uploads", CompareType: v2.PolicyCompareTypeEQUALS},
					expectRanking: 0,
				},
			},
		},
		{
			Name: "Equal Priority Stable Order",
			Rules: []L7RuleRequestWithExpect{
				{
					l7RuleWrapper: l7RuleWrapper{RuleType: v2.PolicyRuleTypePATH, RuleValue: "/a", CompareType: v2.PolicyCompareTypeEQUALS},
					expectRanking: 0,
				},
				{
					l7RuleWrapper: l7RuleWrapper{RuleType: v2.PolicyRuleTypePATH, RuleValue: "/c", CompareType: v2.PolicyCompareTypeEQUALS},
					expectRanking: 1,
				},
				{
					l7RuleWrapper: l7RuleWrapper{RuleType: v2.PolicyRuleTypePATH, RuleValue: "/b", CompareType: v2.PolicyCompareTypeEQUALS},
					expectRanking: 2,
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			// Sort rules by priority
			sort.Slice(tc.Rules, func(i, j int) bool {
				return tc.Rules[i].GetPriority() > tc.Rules[j].GetPriority()
			})

			// Verify the ranking matches expected
			for i, rule := range tc.Rules {
				if i != rule.expectRanking {
					t.Errorf("Rule %v at position %d, expected position %d (priority: %d)",
						rule.l7RuleWrapper, i, rule.expectRanking, rule.GetPriority())
				}
			}
		})
	}
}
