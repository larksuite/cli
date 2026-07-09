// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// ─── schema-driven flag validation ────────────────────────────────────
//
// Composite JSON flags (--properties, --cells, --operations, …) carry
// non-trivial payloads whose shape is already pinned by the embedded
// data/flag-schemas.json (see flag_schema.go). Rather than hand-write
// per-spec validators for type / enum / required / nested checks, every
// such flag is run through validatePropertiesAgainstSchema after the
// shortcut's enhance hook has filled in any flat-flag-derived fields
// (schema describes the *final* tool input, not the raw --properties
// JSON the user typed). Cross-field business rules that JSON Schema
// can't express (e.g. sparkline-update requires sparkline_id per item)
// continue to live in spec.validateUpdateInput.
//
// The rule set is a subset of ai-tools/.../validate-tool-params.ts —
// type, enum, oneOf, required, nested properties, and array items.
// additionalProperties is intentionally lenient: the embedded schema
// is a sub-tree and may not be exhaustive, so rejecting unknown keys
// would be more disruptive than valuable.

// validateParsedJSONFlag validates the just-parsed value of a single
// JSON flag against its embedded schema, if one is registered for the
// (command, flag) pair. Called from parseJSONFlag so every JSON flag
// — sort-keys, options, border-styles, cells, operations, ranges, … —
// is checked at the user-input boundary, in user-input shape.
//
// `properties` is intentionally skipped here: its schema describes the
// *final* tool-input properties (the shape after enhance* hooks
// inject flat-flag-derived fields such as cond-format's rule_type),
// not what the user typed under --properties. The input-builder tail
// validates that one via validateInputAgainstSchema after enhance.
func validateParsedJSONFlag(fv flagView, name string, value interface{}) error {
	if fv == nil || value == nil {
		return nil
	}
	if _, skip := parseJSONFlagSkip[name]; skip {
		return nil
	}
	return validateValueAgainstSchema(fv, name, value)
}

// parseJSONFlagSkip lists flag names where parseJSONFlag-time schema
// validation is intentionally bypassed:
//
//   - properties: schema describes the *final* tool-input shape (after
//     enhance hooks inject flat-flag-derived fields); validated at the
//     input-builder tail via validateInputAgainstSchema instead.
//   - operations: +batch-update's translator does richer validation
//     (allowed-shortcut allow-list, fan-out rejection, …) with more
//     actionable error messages than a generic "not in enum [...]"
//     would. The translator path stays the source of truth.
var parseJSONFlagSkip = map[string]struct{}{
	"properties": {},
	"operations": {},
	"styles":     {},
}

// validateValueAgainstSchema is the (command, flag) → schema → check
// pipeline shared by both validateParsedJSONFlag (user shape) and
// validateInputAgainstSchema (wire shape).
func validateValueAgainstSchema(fv flagView, name string, value interface{}) error {
	command := fv.Command()
	if command == "" {
		return nil
	}
	// Fast path: commands without a registered schema can't fail this check,
	// so skip the 256KB flag-schemas.json parse entirely for them.
	if _, ok := commandsWithSchema[command]; !ok {
		return nil
	}
	idx, _ := loadFlagSchemas()
	if idx == nil {
		return nil
	}
	entry, ok := idx.Flags[command]
	if !ok {
		return nil
	}
	raw, ok := entry[name]
	if !ok {
		return nil
	}
	var schema schemaProperty
	json.Unmarshal(raw, &schema)
	if vErr := validateAgainstSchema(value, &schema, ""); vErr != nil {
		// Composite-JSON shape errors (e.g. +cells-set --cells, chart
		// --properties) are the highest-frequency usage-layer failure for
		// sheets, and agents often burn several retries guessing the shape.
		// Point them straight at --print-schema, which dumps the exact JSON
		// Schema for this (command, flag) pair. The hint is always actionable:
		// reaching this branch means entry[name] resolved a schema from the
		// embedded index, and --print-schema reads that same index, so the
		// suggested command is guaranteed to print it.
		return sheetsValidationForFlag(name,
			"--%s: %s; run `lark-cli sheets %s --print-schema --flag-name %s` to see the expected JSON Schema",
			name, vErr.Error(), command, name).WithCause(vErr)
	}
	return nil
}

// validateInputAgainstSchema validates input[flag] for every flag the
// embedded schema registers under the view's shortcut command. Returns
// nil when no schema is registered for the command, or when none of
// the registered flag names appear in `input` (schema describes the
// shape of values when they are present, not which flags must be
// present). Designed to be called at the tail of every input builder
// so wiring up a new shortcut requires only the standard one-line
// invocation, not a per-shortcut validator.
func validateInputAgainstSchema(fv flagView, input map[string]interface{}) error {
	if fv == nil || input == nil {
		return nil
	}
	command := fv.Command()
	if command == "" {
		return nil
	}
	// Fast path: commands without a registered schema have nothing to
	// validate, so skip the 256KB flag-schemas.json parse entirely.
	if _, ok := commandsWithSchema[command]; !ok {
		return nil
	}
	idx, _ := loadFlagSchemas()
	if idx == nil {
		return nil
	}
	entry, ok := idx.Flags[command]
	if !ok || len(entry) == 0 {
		return nil
	}

	// Deterministic order so error messages are stable across runs.
	flagNames := make([]string, 0, len(entry))
	for name := range entry {
		flagNames = append(flagNames, name)
	}
	sort.Strings(flagNames)

	for _, flagName := range flagNames {
		if _, skip := inputSchemaSkip[flagName]; skip {
			continue
		}
		// Input keys are wire-style (underscore); schema keys are CLI-style
		// (hyphen) — translate before lookup. Flags whose wire form lives
		// under a different key (e.g. --sort-keys → sort_conditions) won't
		// be found here; they're already validated in user shape via
		// parseJSONFlag → validateParsedJSONFlag.
		inputKey := strings.ReplaceAll(flagName, "-", "_")
		value, present := input[inputKey]
		if !present {
			continue
		}
		if err := validateValueAgainstSchema(fv, flagName, value); err != nil {
			return err
		}
	}
	return nil
}

// inputSchemaSkip mirrors parseJSONFlagSkip for the input-builder
// tail. Same rationale: bypass schema validation for flags where
// richer translator-side validation owns the contract (operations).
var inputSchemaSkip = map[string]struct{}{
	"operations": {},
}

// schemaProperty mirrors the JSON Schema subset used by
// data/flag-schemas.json. Unknown keys (description, …) are dropped —
// they're documentation.
//
// Minimum / Maximum / MinItems / MaxItems use *float64 / *int because
// 0 is a meaningful bound (e.g. chart row >= 0); nil distinguishes
// "no bound declared" from "bound is zero".
//
// AdditionalProperties handles the JSON Schema three-way:
//   - absent / true → lenient, any extra key allowed (validator's
//     default; matches the file header's "may not be exhaustive"
//     stance for schemas that simply don't declare it).
//   - false → strict, every extra key rejected.
//   - <schema> → extra keys allowed, but each value must validate
//     against this schema. Used today for pivot's dynamic
//     map<string, array<string>> fields (groups / collapse).
type schemaProperty struct {
	Type                 string                     `json:"type"`
	Nullable             bool                       `json:"nullable"`
	Enum                 []interface{}              `json:"enum"`
	Properties           map[string]*schemaProperty `json:"properties"`
	Required             []string                   `json:"required"`
	Items                *schemaProperty            `json:"items"`
	OneOf                []*schemaProperty          `json:"oneOf"`
	Minimum              *float64                   `json:"minimum"`
	Maximum              *float64                   `json:"maximum"`
	MinItems             *int                       `json:"minItems"`
	MaxItems             *int                       `json:"maxItems"`
	AdditionalProperties *additionalProps           `json:"additionalProperties"`
}

// additionalProps captures the three JSON Schema forms of
// `additionalProperties`. UnmarshalJSON decodes true / false / object
// into the same struct so callers can branch on (Strict, Schema).
type additionalProps struct {
	Strict bool            // true when schema declared additionalProperties:false
	Schema *schemaProperty // non-nil when declared as an object schema
}

func (a *additionalProps) UnmarshalJSON(data []byte) error {
	trimmed := strings.TrimSpace(string(data))
	switch trimmed {
	case "true":
		return nil // lenient — same as absent
	case "false":
		a.Strict = true
		return nil
	}
	var sub schemaProperty
	if err := json.Unmarshal(data, &sub); err != nil {
		return err
	}
	a.Schema = &sub
	return nil
}

// validateAgainstSchema recursively checks `value` against `schema`,
// prefixing any failure with the JSON path navigated so far.
func validateAgainstSchema(value interface{}, schema *schemaProperty, path string) error {
	if schema == nil {
		return nil // defensive — current callers always pass &schema, but
		// keeps validator safe for future programmatic construction.
	}
	if value == nil && schema.Nullable {
		return nil
	}

	if schema.Type != "" {
		if !matchesJSONType(value, schema.Type) {
			return fmt.Errorf("%sexpected type %q, got %q", pathPrefix(path), schema.Type, jsType(value))
		}
	}

	// Numeric bounds — only checked when value is a number (type mismatch
	// already reported above). Apply to both `number` and `integer` types.
	if num, ok := value.(float64); ok {
		if schema.Minimum != nil && num < *schema.Minimum {
			return fmt.Errorf("%svalue %v is below minimum %v", pathPrefix(path), num, *schema.Minimum)
		}
		if schema.Maximum != nil && num > *schema.Maximum {
			return fmt.Errorf("%svalue %v is above maximum %v", pathPrefix(path), num, *schema.Maximum)
		}
	}

	// Array length bounds — only checked when value is an array.
	if arr, ok := value.([]interface{}); ok {
		if schema.MinItems != nil && len(arr) < *schema.MinItems {
			return fmt.Errorf("%sarray has %d items, minimum is %d", pathPrefix(path), len(arr), *schema.MinItems)
		}
		if schema.MaxItems != nil && len(arr) > *schema.MaxItems {
			return fmt.Errorf("%sarray has %d items, maximum is %d", pathPrefix(path), len(arr), *schema.MaxItems)
		}
	}

	if len(schema.Enum) > 0 {
		matched := false
		for _, allowed := range schema.Enum {
			if jsonEqual(allowed, value) {
				matched = true
				break
			}
		}
		if !matched {
			msg := fmt.Sprintf("%svalue %s is not in enum %s",
				pathPrefix(path), formatJSONValue(value), formatEnum(schema.Enum))
			if hint := suggestEnumMatch(value, schema.Enum); hint != "" {
				msg += fmt.Sprintf(` (did you mean %q?)`, hint)
			}
			return fmt.Errorf("%s", msg)
		}
	}

	if len(schema.OneOf) > 0 {
		matched := false
		for _, sub := range schema.OneOf {
			if validateAgainstSchema(value, sub, path) == nil {
				matched = true
				break
			}
		}
		if !matched {
			return fmt.Errorf("%svalue does not match any of oneOf alternatives", pathPrefix(path))
		}
	}

	// Object-level checks. `required` and `properties` are independent
	// per JSON Schema: `required` enforces keys regardless of whether
	// the schema also describes their per-key shape via `properties`.
	if obj, ok := value.(map[string]interface{}); ok {
		for _, key := range schema.Required {
			if _, present := obj[key]; !present {
				return fmt.Errorf("required property %q is missing at %s", key, pathOrRoot(path))
			}
		}
		if schema.Properties != nil {
			keys := make([]string, 0, len(schema.Properties))
			for k := range schema.Properties {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, key := range keys {
				sub := schema.Properties[key]
				v, present := obj[key]
				if !present {
					continue
				}
				// Case-insensitive enum tolerance: when the value matches an
				// allowed enum entry except for casing, rewrite it in place to
				// the canonical spelling. The schema lists enums in their
				// canonical (lower-case) form, so "SUM" / "COUNTA" would
				// otherwise be rejected right here before the request is even
				// sent; normalizing kills the whole pivot summarize_by "SUM vs
				// sum" class. Genuinely-unknown values still fail below, with
				// their own did-you-mean hint.
				if sub != nil && len(sub.Enum) > 0 {
					if canon := suggestEnumMatch(v, sub.Enum); canon != "" {
						obj[key] = canon
						v = canon
					}
				}
				child := key
				if path != "" {
					child = path + "." + key
				}
				if err := validateAgainstSchema(v, sub, child); err != nil {
					return err
				}
			}
		}
		// additionalProperties: enforce only when explicitly declared.
		// Absent means lenient (matches the file header's stance). Sort
		// extras so the first rejection is deterministic across runs.
		if schema.AdditionalProperties != nil {
			extras := make([]string, 0)
			for key := range obj {
				if _, declared := schema.Properties[key]; declared {
					continue
				}
				extras = append(extras, key)
			}
			sort.Strings(extras)
			for _, key := range extras {
				if schema.AdditionalProperties.Strict {
					return fmt.Errorf("%sunexpected property %q (not declared in schema)", pathPrefix(path), key)
				}
				if schema.AdditionalProperties.Schema != nil {
					child := key
					if path != "" {
						child = path + "." + key
					}
					if err := validateAgainstSchema(obj[key], schema.AdditionalProperties.Schema, child); err != nil {
						return err
					}
				}
			}
		}
	}

	if schema.Type == "array" && schema.Items != nil {
		arr, ok := value.([]interface{})
		if !ok {
			return nil // type mismatch already reported above.
		}
		for i, item := range arr {
			child := fmt.Sprintf("%s[%d]", path, i)
			if err := validateAgainstSchema(item, schema.Items, child); err != nil {
				return err
			}
		}
	}

	return nil
}

func matchesJSONType(value interface{}, expected string) bool {
	switch expected {
	case "object":
		_, ok := value.(map[string]interface{})
		return ok
	case "array":
		_, ok := value.([]interface{})
		return ok
	case "string":
		_, ok := value.(string)
		return ok
	case "number":
		_, ok := value.(float64)
		return ok
	case "integer":
		f, ok := value.(float64)
		return ok && f == float64(int64(f))
	case "boolean":
		_, ok := value.(bool)
		return ok
	case "null":
		return value == nil
	}
	return true
}

func jsType(value interface{}) string {
	switch value.(type) {
	case nil:
		return "null"
	case map[string]interface{}:
		return "object"
	case []interface{}:
		return "array"
	case string:
		return "string"
	case float64:
		return "number"
	case bool:
		return "boolean"
	}
	return fmt.Sprintf("%T", value)
}

func jsonEqual(a, b interface{}) bool {
	ja, _ := json.Marshal(a)
	jb, _ := json.Marshal(b)
	return string(ja) == string(jb)
}

// formatJSONValue is the "what you actually passed" half of an enum
// error. Strings get JSON-quoted ("SUM"); everything else (numbers,
// booleans, null, objects, arrays) gets its JSON encoding. Marshal
// failure falls back to %v so we never panic just to format an error.
func formatJSONValue(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(b)
}

// formatEnum renders the allowed-values list for an enum error. Caps
// the visible entries at enumDisplayLimit so a 50-shortcut enum
// doesn't bury the actual error in a wall of options; the overflow
// hint tells the user how many more exist (and to consult --help /
// --print-schema for the full list).
const enumDisplayLimit = 8

func formatEnum(values []interface{}) string {
	if len(values) <= enumDisplayLimit {
		return "[" + joinFormatted(values) + "]"
	}
	shown := values[:enumDisplayLimit]
	return fmt.Sprintf("[%s, … (%d more)]", joinFormatted(shown), len(values)-enumDisplayLimit)
}

func joinFormatted(values []interface{}) string {
	parts := make([]string, 0, len(values))
	for _, v := range values {
		parts = append(parts, formatJSONValue(v))
	}
	return strings.Join(parts, ", ")
}

// suggestEnumMatch returns a "did you mean" candidate when the user's
// value differs from an allowed enum entry only in casing — the most
// common real-world mistake ("SUM" vs "sum", "True" vs "true"). The
// match is restricted to strings; non-string enums (numbers, etc.)
// don't have a casing notion. Returns "" when no near-miss exists.
func suggestEnumMatch(value interface{}, values []interface{}) string {
	s, ok := value.(string)
	if !ok {
		return ""
	}
	lower := strings.ToLower(s)
	for _, v := range values {
		if vs, ok := v.(string); ok && strings.ToLower(vs) == lower {
			if vs != s { // skip exact-equal (already would have matched).
				return vs
			}
		}
	}
	return ""
}

func pathPrefix(path string) string {
	if path == "" {
		return ""
	}
	return path + ": "
}

func pathOrRoot(path string) string {
	if path == "" {
		return "(root)"
	}
	return path
}
