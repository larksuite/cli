// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package rules

import "testing"

func TestSkillQualityWarnsExcessiveCritical(t *testing.T) {
	s := SkillDoc{
		File:        "skills/lark-demo/SKILL.md",
		Description: "Demo skill with a clear routing description",
		Body:        "CRITICAL one\nCRITICAL two\nCRITICAL three\nCRITICAL four\n",
	}
	diags, facts := CheckSkillQuality([]SkillDoc{s})
	if len(diags) != 1 || diags[0].Rule != "skill_critical_noise" {
		t.Fatalf("got %#v", diags)
	}
	if !facts[0].CriticalOverBudget {
		t.Fatalf("fact should mark critical over budget")
	}
}
