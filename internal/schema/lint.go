// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package schema

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"reflect"

	"github.com/larksuite/cli/internal/core"
)

var validJSONSchemaTypes = map[string]bool{
	"string":  true,
	"integer": true,
	"number":  true,
	"boolean": true,
	"array":   true,
	"object":  true,
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
		validatePropertyTypes(env.InputSchema.Properties, &errs)
	}
	if env.OutputSchema != nil && env.OutputSchema.Properties != nil {
		validatePropertyTypes(env.OutputSchema.Properties, &errs)
	}

	// ---- L2: type-level consistency ----
	if env.InputSchema != nil && env.InputSchema.Properties != nil {
		// Walk the whole property tree so format/min-max checks reach leaf
		// fields nested under the params/data wrapper.
		walkForL2(env.InputSchema.Properties, &errs)
		// Top-level required keys must exist in top-level properties.
		for _, r := range env.InputSchema.Required {
			if _, ok := env.InputSchema.Properties.Map[r]; !ok {
				errs = append(errs, fmt.Errorf("L2: required key %q not found in properties", r))
			}
		}
	}

	// ---- L3: cross-field self-consistency ----
	dangerExpected := env.Meta.Risk == core.RiskWrite || env.Meta.Risk == core.RiskHighRiskWrite
	if env.Meta.Danger != dangerExpected {
		errs = append(errs, fmt.Errorf("L3: _meta.danger=%v inconsistent with risk=%q", env.Meta.Danger, env.Meta.Risk))
	}

	// `yes` lives at inputSchema.properties.yes (sibling of params/data),
	// injected only for risk == RiskHighRiskWrite.
	hasYes := false
	if env.InputSchema != nil && env.InputSchema.Properties != nil {
		_, hasYes = env.InputSchema.Properties.Map["yes"]
	}
	wantYes := env.Meta.Risk == core.RiskHighRiskWrite
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

// walkForL2 recursively applies per-field L2 checks (format:binary on
// non-string; minimum>=maximum) plus the sub-object required-exists invariant.
// Required only matters on object-typed Properties (e.g. the params / data
// wrappers); leaf scalars ignore it.
func walkForL2(props *OrderedProps, errs *[]error) {
	if props == nil {
		return
	}
	for _, k := range props.Order {
		p := props.Map[k]
		if p.Format == "binary" && p.Type != "string" {
			*errs = append(*errs, fmt.Errorf("L2: field %q has format: binary but type = %q (want string)", k, p.Type))
		}
		if p.Minimum != nil && p.Maximum != nil {
			min, minOK := exactJSONNumber(*p.Minimum)
			max, maxOK := exactJSONNumber(*p.Maximum)
			switch {
			case !minOK || !maxOK:
				*errs = append(*errs, fmt.Errorf("L2: field %q has an invalid minimum or maximum", k))
			case min.Cmp(max) >= 0:
				*errs = append(*errs, fmt.Errorf(
					"L2: field %q minimum (%s) >= maximum (%s)",
					k,
					p.Minimum.String(),
					p.Maximum.String(),
				))
			}
		}
		if n := len(p.EnumDescriptions); n > 0 && n != len(p.Enum) {
			*errs = append(*errs, fmt.Errorf("L2: field %q enumDescriptions length (%d) != enum length (%d)", k, n, len(p.Enum)))
		}
		validateLiteralConstraints(k, "example", p.Example, p, errs)
		validateLiteralConstraints(k, "default", p.Default, p, errs)
		if len(p.Required) > 0 && p.Properties != nil {
			for _, r := range p.Required {
				if _, ok := p.Properties.Map[r]; !ok {
					*errs = append(*errs, fmt.Errorf("L2: required key %q in %q not found in its properties", r, k))
				}
			}
		}
		if p.Properties != nil {
			walkForL2(p.Properties, errs)
		}
	}
}

// validateLiteralConstraints keeps schema guidance self-consistent: a concrete
// example/default must be one of the declared enum values and a numeric literal
// must fall inside the advertised bounds. Empty strings are metadata's legacy
// spelling for "unspecified", so they are ignored here rather than treated as
// a concrete value.
func validateLiteralConstraints(field, label string, value any, p Property, errs *[]error) {
	if value == nil {
		return
	}
	if text, ok := value.(string); ok && text == "" {
		return
	}
	if len(p.Enum) > 0 && !literalInEnum(value, p.Enum) {
		*errs = append(*errs, fmt.Errorf("L2: field %q %s (%v) is not in enum", field, label, value))
	}
	if p.Type != "integer" && p.Type != "number" {
		return
	}

	number, ok := exactLiteralNumber(value)
	if !ok {
		*errs = append(*errs, fmt.Errorf("L2: field %q %s (%v) is not a valid number", field, label, value))
		return
	}
	if p.Minimum != nil {
		minimum, valid := exactJSONNumber(*p.Minimum)
		if valid && number.Cmp(minimum) < 0 {
			*errs = append(*errs, fmt.Errorf(
				"L2: field %q %s (%v) is below minimum (%s)",
				field, label, value, p.Minimum.String(),
			))
		}
	}
	if p.Maximum != nil {
		maximum, valid := exactJSONNumber(*p.Maximum)
		if valid && number.Cmp(maximum) > 0 {
			*errs = append(*errs, fmt.Errorf(
				"L2: field %q %s (%v) is above maximum (%s)",
				field, label, value, p.Maximum.String(),
			))
		}
	}
}

func literalInEnum(value any, enum []interface{}) bool {
	for _, candidate := range enum {
		if reflect.DeepEqual(value, candidate) {
			return true
		}
	}
	return false
}

func exactLiteralNumber(value any) (*big.Rat, bool) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, false
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var decoded any
	if err := decoder.Decode(&decoded); err != nil {
		return nil, false
	}
	number, ok := decoded.(json.Number)
	if !ok {
		return nil, false
	}
	return exactJSONNumber(number)
}

// exactJSONNumber validates n as a JSON number and converts it to an exact
// rational for comparisons. This preserves large integers, decimals, and
// scientific notation without routing them through float64.
func exactJSONNumber(n json.Number) (*big.Rat, bool) {
	var raw json.RawMessage
	if err := json.Unmarshal([]byte(n.String()), &raw); err != nil {
		return nil, false
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var value any
	if err := dec.Decode(&value); err != nil {
		return nil, false
	}
	validated, ok := value.(json.Number)
	if !ok {
		return nil, false
	}
	return new(big.Rat).SetString(validated.String())
}

// validatePropertyTypes walks an OrderedProps tree and asserts:
//   - every Property.Type is in validJSONSchemaTypes (or empty for nested objects with only properties)
//   - array Properties have Items
//
// Errors are appended to *errs.
func validatePropertyTypes(props *OrderedProps, errs *[]error) {
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
		if p.Properties != nil {
			validatePropertyTypes(p.Properties, errs)
		}
		// Validate the array-element schema itself, not only its child
		// properties — a primitive element with an invalid type (e.g.
		// `items.type = "list"`) would otherwise slip past lint.
		if p.Items != nil {
			validateItemSchema(k, p.Items, errs)
		}
	}
}

// validateItemSchema checks a single array element schema for invalid types,
// then recurses into any further nested properties/items.
func validateItemSchema(parentKey string, item *Property, errs *[]error) {
	if item.Type != "" && !validJSONSchemaTypes[item.Type] {
		*errs = append(*errs, fmt.Errorf("L1: array property %q items has invalid type %q", parentKey, item.Type))
	}
	if item.Type == "array" && item.Items == nil {
		*errs = append(*errs, fmt.Errorf("L1: array property %q items (nested array) missing items", parentKey))
	}
	if item.Properties != nil {
		validatePropertyTypes(item.Properties, errs)
	}
	if item.Items != nil {
		validateItemSchema(parentKey, item.Items, errs)
	}
}

// coverageBaseline is the per-metric warn threshold for L4 coverage checks.
// If the measured rate drops below the baseline, t.Logf emits a warning but
// does NOT fail the test. Adjust these constants upward as meta_data quality
// improves over time.
var coverageBaseline = map[string]float64{
	"description": 0.99,
	"scopes":      1.00,
	"doc_url":     0.98,
	"risk":        0.96,
}

// measureCoverage returns the non-empty rate for each tracked metric.
func measureCoverage(envs []Envelope) map[string]float64 {
	if len(envs) == 0 {
		return map[string]float64{
			"description": 0,
			"scopes":      0,
			"doc_url":     0,
			"risk":        0,
		}
	}
	total := float64(len(envs))
	var descNonEmpty, scopesNonEmpty, docURLNonEmpty, riskNonEmpty float64
	for _, e := range envs {
		if e.Description != "" {
			descNonEmpty++
		}
		if e.Meta == nil {
			continue
		}
		if len(e.Meta.Scopes) > 0 {
			scopesNonEmpty++
		}
		if e.Meta.DocURL != "" {
			docURLNonEmpty++
		}
		if e.Meta.Risk != "" {
			riskNonEmpty++
		}
	}
	return map[string]float64{
		"description": descNonEmpty / total,
		"scopes":      scopesNonEmpty / total,
		"doc_url":     docURLNonEmpty / total,
		"risk":        riskNonEmpty / total,
	}
}
