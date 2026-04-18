// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package contentsafety

import "regexp"

type injectionRule struct {
	ID      string
	Pattern *regexp.Regexp
}

func compiledInjectionRules() []injectionRule {
	return []injectionRule{
		{
			ID: "instruction_override",
			Pattern: regexp.MustCompile(
				`(?i)ignore\s+(all\s+|any\s+|the\s+)?(previous|prior|above|earlier)\s+(instructions?|prompts?|directives?)`,
			),
		},
		{
			ID: "role_injection",
			Pattern: regexp.MustCompile(
				`(?i)<\s*/?\s*(system|assistant|tool|user|developer)\s*>`,
			),
		},
		{
			ID: "system_prompt_leak",
			Pattern: regexp.MustCompile(
				`(?i)\b(reveal|print|show|output|display|repeat)\s+(your|the|all)\s+(system\s+|initial\s+|original\s+)?(prompt|instructions?|rules?)`,
			),
		},
		{
			ID: "delimiter_smuggle",
			Pattern: regexp.MustCompile(
				`<\|im_(start|end|sep)\|>|<\|endoftext\|>|###\s*(system|assistant|user)\s*:`,
			),
		},
	}
}
