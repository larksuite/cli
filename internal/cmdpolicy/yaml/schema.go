// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package yaml parses one or more Rules from yaml bytes. It is kept
// separate from the public extension/platform package so that platform
// stays free of yaml library dependencies -- plugins constructing a Rule
// in Go code never import yaml, only the file loader does.
//
// This package does **structural** parsing only (yaml syntax + unknown-field
// rejection). Semantic validation (valid MaxRisk enum, valid identity
// values, valid doublestar glob syntax) is centralised in
// internal/cmdpolicy.ValidateRule so a single contract is enforced regardless
// of whether the Rule came from yaml or from Plugin.Restrict.
package yaml

import (
	"errors"
	"fmt"
	"io"

	gopkgyaml "gopkg.in/yaml.v3"

	"github.com/larksuite/cli/extension/platform"
)

// ruleSchema is the internal yaml-tagged shape of one rule. Mirrors
// platform.Rule but lives here so the public Rule has no yaml tag baggage.
type ruleSchema struct {
	Name             string   `yaml:"name"`
	Description      string   `yaml:"description,omitempty"`
	Allow            []string `yaml:"allow,omitempty"`
	Deny             []string `yaml:"deny,omitempty"`
	MaxRisk          string   `yaml:"max_risk,omitempty"`
	Identities       []string `yaml:"identities,omitempty"`
	AllowUnannotated bool     `yaml:"allow_unannotated,omitempty"`
}

// fileSchema is the top-level document shape. Two mutually-exclusive
// layouts are accepted:
//
//   - a single rule written with flat top-level fields (the historical
//     layout; the inlined ruleSchema), or
//   - a "rules:" list of rule objects (multi-rule layout).
//
// Mixing the two (flat fields AND a rules: list in the same file) is a
// configuration error -- Parse rejects it rather than guessing intent.
//
// Rules is a pointer so Parse can tell "rules: key absent" (nil) apart
// from "rules: present but empty" (non-nil, len 0). The latter is a
// foot-gun -- a config generator that renders an empty list would
// otherwise yield a single all-zero Rule that lets every annotated
// command through -- so Parse rejects it outright.
type fileSchema struct {
	ruleSchema `yaml:",inline"`
	Rules      *[]ruleSchema `yaml:"rules,omitempty"`
}

// isZero reports whether every field is its zero value. Used to detect
// the flat-fields-plus-rules: mixing error.
func (s ruleSchema) isZero() bool {
	return s.Name == "" && s.Description == "" &&
		len(s.Allow) == 0 && len(s.Deny) == 0 &&
		s.MaxRisk == "" && len(s.Identities) == 0 && !s.AllowUnannotated
}

func (s ruleSchema) toRule() *platform.Rule {
	// Leave Identities nil when absent (omitempty-style), matching how the
	// Allow/Deny slices arrive nil from yaml. A zero-length but non-nil
	// slice is behaviourally identical to the engine but trips
	// reflect.DeepEqual in tests and reads as "explicitly empty".
	var idents []platform.Identity
	if len(s.Identities) > 0 {
		idents = make([]platform.Identity, len(s.Identities))
		for i, id := range s.Identities {
			idents[i] = platform.Identity(id)
		}
	}
	return &platform.Rule{
		Name:             s.Name,
		Description:      s.Description,
		Allow:            s.Allow,
		Deny:             s.Deny,
		MaxRisk:          platform.Risk(s.MaxRisk),
		Identities:       idents,
		AllowUnannotated: s.AllowUnannotated,
	}
}

// Parse decodes yaml bytes into one or more *platform.Rule. Unknown fields
// are rejected so an old binary cannot silently ignore new schema additions
// (forward-compat safeguard).
//
// The result always has at least one element: a flat-fields document
// yields a single rule (possibly an all-zero "no restriction" rule), and a
// "rules:" list yields one rule per entry.
//
// Semantic validation (MaxRisk taxonomy, identity values, glob syntax) is
// the caller's responsibility -- run each result through
// internal/cmdpolicy.ValidateRule before handing it to the engine.
func Parse(data []byte) ([]*platform.Rule, error) {
	var s fileSchema
	dec := gopkgyaml.NewDecoder(bytesReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&s); err != nil {
		return nil, fmt.Errorf("parse policy yaml: %w", err)
	}

	// Reject multi-document input: yaml.v3 only decodes one document
	// per call, so a stray "---" followed by another document would
	// silently drop the trailing rule.
	var extra fileSchema
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, fmt.Errorf("parse policy yaml: multiple YAML documents are not allowed")
		}
		return nil, fmt.Errorf("parse policy yaml: %w", err)
	}

	if s.Rules != nil {
		if len(*s.Rules) == 0 {
			return nil, fmt.Errorf("parse policy yaml: 'rules:' is present but empty; remove the key, or list at least one rule")
		}
		if !s.ruleSchema.isZero() {
			return nil, fmt.Errorf("parse policy yaml: top-level rule fields cannot be combined with a 'rules:' list; move every rule under 'rules:'")
		}
		out := make([]*platform.Rule, 0, len(*s.Rules))
		for _, rs := range *s.Rules {
			out = append(out, rs.toRule())
		}
		return out, nil
	}

	// Backward-compatible single top-level rule (flat fields).
	return []*platform.Rule{s.ruleSchema.toRule()}, nil
}
