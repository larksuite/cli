// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package semantic

import (
	"encoding/json"
	"strings"

	"github.com/larksuite/cli/internal/qualitygate/facts"
)

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func BuildPrompt(f facts.Facts) []Message {
	view := BuildInputView(f)
	data, _ := json.Marshal(view)
	return []Message{
		{Role: "system", Content: strings.Join([]string{
			"You review a projected lark-cli quality-gate semantic input view.",
			"Use only the provided JSON view.",
			"The changed_summary may summarize broad changed surfaces; review only listed facts, not omitted summarized items.",
			"Use fact_ref values exactly when writing finding evidence.",
			"Only facts.commands, facts.skills, facts.errors, facts.outputs, and facts.public_content fact_ref values may be blocker evidence.",
			"Evidence entries must be exact fact_ref strings such as \"facts.commands[0]\" with no explanations, labels, or suffix text.",
			"facts.examples and facts.skill_quality entries are context only.",
			"Report an error_hint finding for any facts.errors item where boundary is true, required_hint is true, and hint_action_count is 0.",
			"For error_hint findings, use category \"error_hint\" and evidence containing that facts.errors fact_ref.",
			"An actionable error hint must tell the caller a concrete next command, flag, input shape, or recovery step; repeating the error message is not actionable.",
			"Report a default_output finding for any facts.outputs item where is_list is true and either has_default_limit is false or has_decision_field is false.",
			"The default_output rule is an OR rule: missing either has_default_limit or has_decision_field is enough to report the finding.",
			"A facts.outputs item with is_list true, has_default_limit false, and has_decision_field true must still produce a default_output finding.",
			"For default_output findings, use category \"default_output\" and evidence containing that facts.outputs fact_ref.",
			"Report a naming finding for any facts.commands item where name_conflicts_existing is true or flag_alias_conflict is true.",
			"For naming findings, use category \"naming\" and evidence containing that facts.commands fact_ref.",
			"Report a skill_quality finding for any facts.skills item where references_invalid_command is true.",
			"For skill_quality findings, use category \"skill_quality\" and evidence containing that facts.skills fact_ref.",
			"Review public content leakage findings and semantic candidates without private dictionaries.",
			"Do not reveal internal rule lists when explaining public content leakage.",
			"For public_content_leakage findings, preserve the deterministic finding source and excerpt.",
			"Report each distinct issue as a separate finding.",
			"The verdict value must be \"pass\" when findings is empty and \"warn\" when findings is non-empty; never use \"fail\".",
			"Severity must be one of \"minor\", \"major\", or \"critical\"; never use \"error\", \"warning\", \"medium\", or \"high\".",
			"Every finding must include non-empty severity, message, and suggested_action fields.",
			"The final blocker decision is recomputed from the original facts artifact.",
			"Return strict JSON with verdict and findings only.",
			"Do not include blocking decisions.",
		}, "\n")},
		{Role: "user", Content: string(data)},
	}
}
