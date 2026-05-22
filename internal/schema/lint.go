// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package schema

import (
	"errors"
	"fmt"
)

var validJSONSchemaTypes = map[string]bool{
	"string":  true,
	"integer": true,
	"number":  true,
	"boolean": true,
	"array":   true,
	"object":  true,
}

var validXIn = map[string]bool{
	"path":  true,
	"query": true,
	"body":  true,
}

var validAccessTokens = map[string]bool{
	"user": true,
	"bot":  true,
}

// lintEnvelope runs L1-L3 checks and returns a list of errors. Empty slice
// means the envelope is compliant.
func lintEnvelope(env Envelope) []error {
	var errs []error

	// ---- L1: structural ----
	if env.Name == "" {
		errs = append(errs, errors.New("L1: name must not be empty"))
	}
	if env.InputSchema == nil {
		errs = append(errs, errors.New("L1: inputSchema must not be nil"))
	} else {
		if env.InputSchema.Type != "object" {
			errs = append(errs, fmt.Errorf("L1: inputSchema.type = %q, want \"object\"", env.InputSchema.Type))
		}
		if env.InputSchema.Properties == nil {
			errs = append(errs, errors.New("L1: inputSchema.properties must not be nil"))
		}
	}
	if env.OutputSchema == nil {
		errs = append(errs, errors.New("L1: outputSchema must not be nil"))
	} else {
		if env.OutputSchema.Type != "object" {
			errs = append(errs, fmt.Errorf("L1: outputSchema.type = %q, want \"object\"", env.OutputSchema.Type))
		}
	}
	if env.Meta == nil {
		errs = append(errs, errors.New("L1: _meta must not be nil"))
		// Cannot continue meta-dependent checks
		return errs
	}
	if env.Meta.EnvelopeVersion != "1.0" {
		errs = append(errs, fmt.Errorf("L1: _meta.envelope_version = %q, want \"1.0\"", env.Meta.EnvelopeVersion))
	}

	// L1: validate every Property type recursively
	if env.InputSchema != nil && env.InputSchema.Properties != nil {
		validatePropertyTypes(env.InputSchema.Properties, true, &errs)
	}
	if env.OutputSchema != nil && env.OutputSchema.Properties != nil {
		validatePropertyTypes(env.OutputSchema.Properties, false, &errs)
	}

	// ---- L2: type-level consistency ----
	if env.InputSchema != nil && env.InputSchema.Properties != nil {
		// path fields must be in required
		for _, k := range env.InputSchema.Properties.Order {
			p := env.InputSchema.Properties.Map[k]
			if p.XIn == "path" && !contains(env.InputSchema.Required, k) {
				errs = append(errs, fmt.Errorf("L2: path field %q must be in required", k))
			}
			if p.Format == "binary" && p.Type != "string" {
				errs = append(errs, fmt.Errorf("L2: field %q has format: binary but type = %q (want string)", k, p.Type))
			}
			if p.Minimum != nil && p.Maximum != nil && *p.Minimum >= *p.Maximum {
				errs = append(errs, fmt.Errorf("L2: field %q minimum (%v) >= maximum (%v)", k, *p.Minimum, *p.Maximum))
			}
		}
		// required keys must exist in properties
		for _, r := range env.InputSchema.Required {
			if _, ok := env.InputSchema.Properties.Map[r]; !ok {
				errs = append(errs, fmt.Errorf("L2: required key %q not found in properties", r))
			}
		}
	}

	// ---- L3: cross-field self-consistency ----
	dangerExpected := env.Meta.Risk == "write" || env.Meta.Risk == "high-risk-write"
	if env.Meta.Danger != dangerExpected {
		errs = append(errs, fmt.Errorf("L3: _meta.danger=%v inconsistent with risk=%q", env.Meta.Danger, env.Meta.Risk))
	}

	hasYes := env.InputSchema != nil && env.InputSchema.Properties != nil && env.InputSchema.Properties.Map != nil
	if hasYes {
		_, hasYes = env.InputSchema.Properties.Map["yes"]
	}
	wantYes := env.Meta.Risk == "high-risk-write"
	if hasYes != wantYes {
		errs = append(errs, fmt.Errorf("L3: inputSchema `yes` property=%v inconsistent with risk=%q", hasYes, env.Meta.Risk))
	}

	if len(env.Meta.AccessTokens) == 0 {
		errs = append(errs, errors.New("L3: _meta.access_tokens must not be empty"))
	}
	for _, t := range env.Meta.AccessTokens {
		if !validAccessTokens[t] {
			errs = append(errs, fmt.Errorf("L3: _meta.access_tokens contains invalid value %q (allowed: user, bot)", t))
		}
	}

	return errs
}

// validatePropertyTypes walks an OrderedProps tree and asserts:
//   - every Property.Type is in validJSONSchemaTypes (or empty for nested objects with only properties)
//   - array Properties have Items
//   - top-level Properties (isInputTop=true) have a valid XIn value
//
// Errors are appended to *errs.
func validatePropertyTypes(props *OrderedProps, isInputTop bool, errs *[]error) {
	if props == nil {
		return
	}
	for _, k := range props.Order {
		p := props.Map[k]
		if p.Type != "" && !validJSONSchemaTypes[p.Type] {
			*errs = append(*errs, fmt.Errorf("L1: property %q has invalid type %q", k, p.Type))
		}
		if p.Type == "array" && p.Items == nil {
			*errs = append(*errs, fmt.Errorf("L1: array property %q missing items", k))
		}
		if isInputTop && k != "yes" {
			if p.XIn == "" {
				*errs = append(*errs, fmt.Errorf("L1: top-level property %q missing x-in", k))
			} else if !validXIn[p.XIn] {
				*errs = append(*errs, fmt.Errorf("L1: top-level property %q has invalid x-in %q", k, p.XIn))
			}
		}
		// Recurse into nested properties (NOT input-top anymore)
		if p.Properties != nil {
			validatePropertyTypes(p.Properties, false, errs)
		}
		if p.Items != nil && p.Items.Properties != nil {
			validatePropertyTypes(p.Items.Properties, false, errs)
		}
	}
}

func contains(slice []string, s string) bool {
	for _, x := range slice {
		if x == s {
			return true
		}
	}
	return false
}
