// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package meta

import "encoding/json"

// Affordance is the typed usage guidance overlaid on a method. It is the single
// model the envelope renderer and the command help both parse, so the
// vocabulary is defined once; the JSON tags double as the envelope wire shape.
// Skills entries are either a bare skill name (e.g. "lark-doc") or a
// name/relative-path reference (e.g. "lark-contact/references/x.md"); both
// render as runnable `lark-cli skills read <entry>` pointers. Help validates
// each against the embedded skill tree (a name → its SKILL.md, a reference →
// that path) and drops any that do not resolve.
type Affordance struct {
	UseWhen       []string            `json:"use_when,omitempty"`
	AvoidWhen     []string            `json:"avoid_when,omitempty"`
	Prerequisites []string            `json:"prerequisites,omitempty"`
	Tips          []string            `json:"tips,omitempty"`
	Examples      []AffordanceCase    `json:"examples,omitempty"`
	Extensions    []AffordanceSection `json:"extensions,omitempty"`
	Related       []string            `json:"related,omitempty"`
	Skills        []string            `json:"skills,omitempty"`
}

// AffordanceCase is one few-shot example: a description and a ready-to-run command.
type AffordanceCase struct {
	Description string `json:"description,omitempty"`
	Command     string `json:"command"`
}

// AffordanceSection is a custom guidance section: any heading beyond the
// standard four (Avoid when / Prerequisites / Tips / Examples) flows through
// here with its label preserved, so authors can add sections without code
// changes.
type AffordanceSection struct {
	Label string   `json:"label"`
	Items []string `json:"items,omitempty"`
}

// ParsedAffordance decodes the method's overlay. ok is false when it is absent,
// malformed, or wholly empty — callers treat all three as "no guidance".
func (m Method) ParsedAffordance() (Affordance, bool) {
	if len(m.Affordance) == 0 {
		return Affordance{}, false
	}
	var a Affordance
	if json.Unmarshal(m.Affordance, &a) != nil {
		return Affordance{}, false
	}
	if len(a.UseWhen) == 0 && len(a.AvoidWhen) == 0 && len(a.Prerequisites) == 0 && len(a.Tips) == 0 && len(a.Examples) == 0 && len(a.Extensions) == 0 && len(a.Related) == 0 && len(a.Skills) == 0 {
		return Affordance{}, false
	}
	return a, true
}
