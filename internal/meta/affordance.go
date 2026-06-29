// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package meta

import "encoding/json"

// Affordance is the hand-authored usage guidance overlaid on a method: when to
// use it, when not to, prerequisites, few-shot examples, and related methods.
// It is the single typed model of the affordance shape; the envelope renderer
// and the command help both parse through ParsedAffordance so the vocabulary
// is defined once. The JSON tags double as the envelope's wire shape.
type Affordance struct {
	UseWhen       []string         `json:"use_when,omitempty"`
	DoNotUseWhen  []string         `json:"do_not_use_when,omitempty"`
	Prerequisites []string         `json:"prerequisites,omitempty"`
	Examples      []AffordanceCase `json:"examples,omitempty"`
	Related       []string         `json:"related,omitempty"`
}

// AffordanceCase is one few-shot example: a one-line description and a
// ready-to-run command.
type AffordanceCase struct {
	Description string `json:"description"`
	Command     string `json:"command"`
}

// ParsedAffordance decodes the method's raw affordance overlay into the typed
// Affordance. ok is false when the method carries no affordance, the JSON is
// malformed, or every section is empty — so callers can treat "no guidance"
// uniformly.
func (m Method) ParsedAffordance() (Affordance, bool) {
	if len(m.Affordance) == 0 {
		return Affordance{}, false
	}
	var a Affordance
	if json.Unmarshal(m.Affordance, &a) != nil {
		return Affordance{}, false
	}
	if len(a.UseWhen) == 0 && len(a.DoNotUseWhen) == 0 && len(a.Prerequisites) == 0 && len(a.Examples) == 0 && len(a.Related) == 0 {
		return Affordance{}, false
	}
	return a, true
}
