// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"encoding/json"
	"strings"
	"testing"
)

// parseSchema is a tiny test helper: take an inline JSON Schema string,
// hand back a *schemaProperty for validateAgainstSchema. Lets test
// cases declare their schema inline rather than hand-building structs.
func parseSchema(t *testing.T, raw string) *schemaProperty {
	t.Helper()
	var s schemaProperty
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		t.Fatalf("bad inline schema %q: %v", raw, err)
	}
	return &s
}

// parseValue decodes a JSON literal the same way encoding/json gives
// validateAgainstSchema its input (numbers → float64, objects →
// map[string]interface{}, arrays → []interface{}).
func parseValue(t *testing.T, raw string) interface{} {
	t.Helper()
	var v interface{}
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		t.Fatalf("bad inline value %q: %v", raw, err)
	}
	return v
}

// TestValidateAgainstSchema_EnumCaseNormalization pins the case-insensitive
// enum tolerance: a value matching an allowed enum entry except for casing is
// rewritten in place to the canonical spelling (so the case-sensitive backend
// accepts it), while genuinely-unknown values still fail. Only fires for enum
// fields nested in an object/array — the pivot values[].summarize_by path.
func TestValidateAgainstSchema_EnumCaseNormalization(t *testing.T) {
	t.Parallel()

	schema := parseSchema(t, `{"type":"object","properties":{"summarize_by":{"type":"string","enum":["sum","count","average"]}}}`)

	t.Run("rewrites case-only mismatch in place", func(t *testing.T) {
		obj := map[string]interface{}{"summarize_by": "SUM"}
		if err := validateAgainstSchema(obj, schema, ""); err != nil {
			t.Fatalf("case-only value should pass after normalization, got: %v", err)
		}
		if got := obj["summarize_by"]; got != "sum" {
			t.Errorf("summarize_by = %q, want normalized %q", got, "sum")
		}
	})

	t.Run("leaves exact match untouched", func(t *testing.T) {
		obj := map[string]interface{}{"summarize_by": "count"}
		if err := validateAgainstSchema(obj, schema, ""); err != nil {
			t.Fatalf("exact match should pass: %v", err)
		}
		if got := obj["summarize_by"]; got != "count" {
			t.Errorf("exact value mutated to %q", got)
		}
	})

	t.Run("unknown value still fails", func(t *testing.T) {
		obj := map[string]interface{}{"summarize_by": "COUNTA"}
		if err := validateAgainstSchema(obj, schema, ""); err == nil {
			t.Fatal("unknown enum value should fail")
		} else if !strings.Contains(err.Error(), "not in enum") {
			t.Errorf("want enum error, got: %v", err)
		}
	})

	t.Run("normalizes inside array-of-objects (values[] shape)", func(t *testing.T) {
		arrSchema := parseSchema(t, `{"type":"array","items":{"type":"object","properties":{"summarize_by":{"type":"string","enum":["sum","count"]}}}}`)
		arr := []interface{}{
			map[string]interface{}{"summarize_by": "Sum"},
			map[string]interface{}{"summarize_by": "COUNT"},
		}
		if err := validateAgainstSchema(arr, arrSchema, ""); err != nil {
			t.Fatalf("array case normalization failed: %v", err)
		}
		if got := arr[0].(map[string]interface{})["summarize_by"]; got != "sum" {
			t.Errorf("arr[0] summarize_by = %q, want sum", got)
		}
		if got := arr[1].(map[string]interface{})["summarize_by"]; got != "count" {
			t.Errorf("arr[1] summarize_by = %q, want count", got)
		}
	})
}

// TestValidateAgainstSchema is the validator's contract test: every
// supported keyword (type, enum, oneOf, required, nested properties,
// array items, nullable, minimum/maximum, minItems/maxItems) gets a
// pass + fail case, and the failure message is asserted to mention
// the JSON path and the violated constraint. Together these pin the
// validator's behaviour without going through any shortcut wiring.
func TestValidateAgainstSchema(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		schema    string
		value     string
		wantOK    bool
		wantInErr string // substring required in error message when !wantOK
	}{
		// ─── type ─────────────────────────────────────────────────────
		{"type string ok", `{"type":"string"}`, `"hi"`, true, ""},
		{"type string wrong", `{"type":"string"}`, `42`, false, `expected type "string"`},
		{"type number ok", `{"type":"number"}`, `3.14`, true, ""},
		{"type number wrong", `{"type":"number"}`, `"x"`, false, `got "string"`},
		{"type integer ok", `{"type":"integer"}`, `5`, true, ""},
		{"type integer fractional rejected", `{"type":"integer"}`, `5.5`, false, `expected type "integer"`},
		{"type boolean ok", `{"type":"boolean"}`, `true`, true, ""},
		{"type array ok", `{"type":"array"}`, `[1,2]`, true, ""},
		{"type object ok", `{"type":"object"}`, `{"a":1}`, true, ""},

		// ─── nullable short-circuit ───────────────────────────────────
		{"nullable null accepted", `{"type":"string","nullable":true}`, `null`, true, ""},
		{"nullable schema still type-checks non-null", `{"type":"string","nullable":true}`, `42`, false, `expected type "string"`},
		{"nullable schema accepts matching type", `{"type":"string","nullable":true}`, `"x"`, true, ""},
		{"null rejected when nullable not set", `{"type":"string"}`, `null`, false, `expected type "string"`},

		// ─── enum ────────────────────────────────────────────────────
		{"enum hit", `{"type":"string","enum":["asc","desc"]}`, `"asc"`, true, ""},
		{"enum miss", `{"type":"string","enum":["asc","desc"]}`, `"sideways"`, false, `not in enum ["asc", "desc"]`},

		// ─── oneOf ───────────────────────────────────────────────────
		{"oneOf string branch", `{"oneOf":[{"type":"string"},{"type":"number"}]}`, `"x"`, true, ""},
		{"oneOf number branch", `{"oneOf":[{"type":"string"},{"type":"number"}]}`, `7`, true, ""},
		{"oneOf no branch", `{"oneOf":[{"type":"string"},{"type":"number"}]}`, `true`, false, `oneOf alternatives`},

		// ─── required ────────────────────────────────────────────────
		{
			"required key present",
			`{"type":"object","required":["a"],"properties":{"a":{"type":"string"}}}`,
			`{"a":"x"}`, true, "",
		},
		{
			"required key missing",
			`{"type":"object","required":["a"]}`,
			`{}`, false, `required property "a"`,
		},

		// ─── nested properties recurse ───────────────────────────────
		{
			"nested property wrong type",
			`{"type":"object","properties":{"inner":{"type":"object","properties":{"x":{"type":"number"}}}}}`,
			`{"inner":{"x":"oops"}}`, false, `inner.x: expected type "number"`,
		},

		// ─── array items recurse with [i] path ───────────────────────
		{
			"array items ok",
			`{"type":"array","items":{"type":"string"}}`,
			`["a","b"]`, true, "",
		},
		{
			"array item wrong type pinpoints index",
			`{"type":"array","items":{"type":"string"}}`,
			`["a",2,"c"]`, false, `[1]: expected type "string"`,
		},

		// ─── numeric bounds (P0 additions) ───────────────────────────
		{"minimum ok", `{"type":"number","minimum":0}`, `0`, true, ""},
		{"minimum fail", `{"type":"number","minimum":0}`, `-1`, false, `below minimum`},
		{"maximum ok", `{"type":"number","maximum":100}`, `100`, true, ""},
		{"maximum fail", `{"type":"number","maximum":100}`, `101`, false, `above maximum`},
		{"minimum on integer", `{"type":"integer","minimum":10}`, `5`, false, `below minimum`},

		// ─── array length bounds (P0 additions) ──────────────────────
		{"minItems ok", `{"type":"array","minItems":1}`, `[1]`, true, ""},
		{"minItems fail", `{"type":"array","minItems":1}`, `[]`, false, `array has 0 items, minimum is 1`},
		{"maxItems ok", `{"type":"array","maxItems":3}`, `[1,2,3]`, true, ""},
		{"maxItems fail", `{"type":"array","maxItems":3}`, `[1,2,3,4]`, false, `array has 4 items, maximum is 3`},

		// ─── combined bounds inside nested array of objects ──────────
		{
			"nested minimum in array item objects",
			`{"type":"array","items":{"type":"object","properties":{"row":{"type":"integer","minimum":0}}}}`,
			`[{"row":0},{"row":-1}]`, false, `[1].row: value -1 is below minimum 0`,
		},

		// ─── additionalProperties absent: lenient (default) ──────────
		{
			"extras allowed when additionalProperties absent",
			`{"type":"object","properties":{"a":{"type":"string"}}}`,
			`{"a":"x","whatever":42}`, true, "",
		},

		// ─── additionalProperties:false: strict mode ─────────────────
		{
			"extras allowed when additionalProperties:true (explicit)",
			`{"type":"object","properties":{"a":{"type":"string"}},"additionalProperties":true}`,
			`{"a":"x","extra":1}`, true, "",
		},
		{
			"extras rejected when additionalProperties:false",
			`{"type":"object","properties":{"a":{"type":"string"}},"additionalProperties":false}`,
			`{"a":"x","typo":1}`, false, `unexpected property "typo"`,
		},
		{
			"declared property still accepted under strict mode",
			`{"type":"object","properties":{"a":{"type":"string"}},"additionalProperties":false}`,
			`{"a":"x"}`, true, "",
		},

		// ─── additionalProperties:<schema>: extras must match ────────
		{
			"extras pass when matching additionalProperties schema",
			`{"type":"object","properties":{"name":{"type":"string"}},"additionalProperties":{"type":"array","items":{"type":"string"}}}`,
			`{"name":"x","g1":["a","b"],"g2":["c"]}`, true, "",
		},
		{
			"extras fail when wrong type for additionalProperties schema",
			`{"type":"object","additionalProperties":{"type":"array","items":{"type":"string"}}}`,
			`{"g1":[1,2]}`, false, `g1[0]: expected type "string"`,
		},
		{
			"extras fail when value isn't even right kind",
			`{"type":"object","additionalProperties":{"type":"array"}}`,
			`{"key":"not-an-array"}`, false, `key: expected type "array"`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := parseSchema(t, tc.schema)
			v := parseValue(t, tc.value)
			err := validateAgainstSchema(v, s, "")
			if tc.wantOK {
				if err != nil {
					t.Fatalf("expected pass, got error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got pass", tc.wantInErr)
			}
			if !strings.Contains(err.Error(), tc.wantInErr) {
				t.Errorf("error = %q, want substring %q", err.Error(), tc.wantInErr)
			}
		})
	}
}

// TestValidateAgainstSchema_EnumErrorEnhancements pins the three
// enum-error UX upgrades together:
//   - the failing value is quoted in JSON form ("SUM", not bare SUM)
//   - the allowed list is JSON-quoted ("sum", not bare sum) and gets
//     truncated past 8 entries with an "N more" hint
//   - case-only mismatches surface a `did you mean` suggestion
//     pointing at the canonical spelling
func TestValidateAgainstSchema_EnumErrorEnhancements(t *testing.T) {
	t.Parallel()

	t.Run("small enum is fully listed and quoted", func(t *testing.T) {
		t.Parallel()
		s := parseSchema(t, `{"type":"string","enum":["asc","desc"]}`)
		err := validateAgainstSchema("sideways", s, "order")
		if err == nil {
			t.Fatal("expected enum violation")
		}
		msg := err.Error()
		if !strings.Contains(msg, `value "sideways"`) {
			t.Errorf("want failing value quoted; got %q", msg)
		}
		if !strings.Contains(msg, `["asc", "desc"]`) {
			t.Errorf("want enum list comma+quote formatted; got %q", msg)
		}
	})

	t.Run("large enum is truncated with overflow hint", func(t *testing.T) {
		t.Parallel()
		// 12 values; default enumDisplayLimit is 8.
		s := parseSchema(t, `{"type":"string","enum":[
			"a","b","c","d","e","f","g","h","i","j","k","l"
		]}`)
		err := validateAgainstSchema("z", s, "x")
		if err == nil {
			t.Fatal("expected enum violation")
		}
		msg := err.Error()
		if !strings.Contains(msg, "4 more") {
			t.Errorf("want overflow hint '4 more'; got %q", msg)
		}
		if strings.Contains(msg, `"i"`) || strings.Contains(msg, `"l"`) {
			t.Errorf("want truncation to first 8; got %q", msg)
		}
		if !strings.Contains(msg, `"h"`) { // 8th entry should be present.
			t.Errorf("want first 8 entries shown; got %q", msg)
		}
	})

	t.Run("case-only mismatch produces did-you-mean hint", func(t *testing.T) {
		t.Parallel()
		s := parseSchema(t, `{"type":"string","enum":["sum","count","average"]}`)
		err := validateAgainstSchema("SUM", s, "")
		if err == nil {
			t.Fatal("expected enum violation")
		}
		if !strings.Contains(err.Error(), `did you mean "sum"?`) {
			t.Errorf("want did-you-mean hint; got %q", err.Error())
		}
	})

	t.Run("no did-you-mean when value is not a near miss", func(t *testing.T) {
		t.Parallel()
		s := parseSchema(t, `{"type":"string","enum":["sum","count"]}`)
		err := validateAgainstSchema("BOGUS", s, "")
		if err == nil {
			t.Fatal("expected enum violation")
		}
		if strings.Contains(err.Error(), "did you mean") {
			t.Errorf("want no hint for unrelated value; got %q", err.Error())
		}
	})

	t.Run("did-you-mean only triggers for strings (not numbers)", func(t *testing.T) {
		t.Parallel()
		s := parseSchema(t, `{"enum":[1,2,3]}`)
		err := validateAgainstSchema(float64(4), s, "")
		if err == nil {
			t.Fatal("expected enum violation")
		}
		if strings.Contains(err.Error(), "did you mean") {
			t.Errorf("numeric enum should not get casing hint; got %q", err.Error())
		}
		// And the failing numeric value still surfaces in JSON form.
		if !strings.Contains(err.Error(), "value 4 ") {
			t.Errorf("want numeric value in error; got %q", err.Error())
		}
	})
}

// TestValidateInputAgainstSchema_RealEnumCaseNormalized confirms the
// case-insensitive enum tolerance fires against the real embedded schema for
// the most common real-world miscue — pivot summarize_by upper-cased. "SUM" is
// rewritten to "sum" in place and the input passes; previously this surfaced a
// did-you-mean error, but in-place canonicalization fixes it so the agent's first try wins.
func TestValidateInputAgainstSchema_RealEnumCaseNormalized(t *testing.T) {
	t.Parallel()
	fv := mapFlagView{command: "+pivot-create"}
	in := map[string]interface{}{
		"properties": map[string]interface{}{
			"values": []interface{}{
				map[string]interface{}{"field": "A", "summarize_by": "SUM"},
			},
		},
	}
	if err := validateInputAgainstSchema(fv, in); err != nil {
		t.Fatalf("upper-case summarize_by should be normalized and pass, got: %v", err)
	}
	vals := in["properties"].(map[string]interface{})["values"].([]interface{})
	if got := vals[0].(map[string]interface{})["summarize_by"]; got != "sum" {
		t.Errorf("summarize_by = %q, want normalized to %q", got, "sum")
	}
}

// TestValidateAgainstSchema_NilSchemaSafe pins the defensive
// `if schema == nil { return nil }` guard. Current production callers
// always hand validator a real schema, but the guard means future
// programmatic construction (or a malformed schema sub-tree decoded
// as a nil pointer inside oneOf) won't crash with a nil deref.
func TestValidateAgainstSchema_NilSchemaSafe(t *testing.T) {
	t.Parallel()
	if err := validateAgainstSchema("anything", nil, ""); err != nil {
		t.Errorf("nil schema should noop; got %v", err)
	}
}

// TestValidateAgainstSchema_AdditionalPropertiesSortedFirstFailure
// asserts that when multiple extras violate additionalProperties:false,
// the *alphabetically first* extra is the one reported — without the
// sort, Go map iteration would make the failing key non-deterministic
// across runs and the error message would flake.
func TestValidateAgainstSchema_AdditionalPropertiesSortedFirstFailure(t *testing.T) {
	t.Parallel()
	schema := parseSchema(t, `{
		"type":"object",
		"properties":{"declared":{"type":"string"}},
		"additionalProperties":false
	}`)
	// Three extras; "alpha" comes first when sorted.
	value := parseValue(t, `{"declared":"ok","zeta":1,"alpha":2,"middle":3}`)
	for i := 0; i < 30; i++ {
		err := validateAgainstSchema(value, schema, "")
		if err == nil {
			t.Fatalf("iter %d: expected extras to be rejected", i)
		}
		if !strings.Contains(err.Error(), `"alpha"`) {
			t.Fatalf("iter %d: expected alphabetically first extra to be reported; got %v", i, err)
		}
	}
}

// TestValidateAgainstSchema_ArrayItemRequired pins that `required`
// fires inside array items too — the recursion path applies the same
// object-level rules at every level, so a missing key in items
// surfaces as `[i].missing` and not a silently-passed item.
func TestValidateAgainstSchema_ArrayItemRequired(t *testing.T) {
	t.Parallel()
	schema := parseSchema(t, `{
		"type":"array",
		"items":{
			"type":"object",
			"required":["id"],
			"properties":{"id":{"type":"string"}}
		}
	}`)
	value := parseValue(t, `[{"id":"a"},{"name":"b"}]`)
	err := validateAgainstSchema(value, schema, "")
	if err == nil {
		t.Fatal("expected required violation on items[1]")
	}
	if !strings.Contains(err.Error(), `required property "id"`) || !strings.Contains(err.Error(), "[1]") {
		t.Errorf("expected required-id at [1]; got %v", err)
	}
}

// TestValidateAgainstSchema_DeterministicPropertyOrder regresses the
// "iterate properties in sorted key order" guarantee so that the
// first-failure error message is stable across runs (Go map iteration
// is randomized — without the sort, a schema with two bad fields
// would non-deterministically report either one).
func TestValidateAgainstSchema_DeterministicPropertyOrder(t *testing.T) {
	t.Parallel()
	schema := parseSchema(t, `{
		"type":"object",
		"properties":{
			"a":{"type":"string"},
			"b":{"type":"string"},
			"c":{"type":"string"}
		}
	}`)
	value := parseValue(t, `{"a":1,"b":2,"c":3}`)
	// Run many times; "a" must always be the reported field (sorted first).
	for i := 0; i < 50; i++ {
		err := validateAgainstSchema(value, schema, "")
		if err == nil || !strings.Contains(err.Error(), "a:") {
			t.Fatalf("iter %d: expected error mentioning 'a:'; got %v", i, err)
		}
	}
}

// TestValidateInputAgainstSchema_RealSchema exercises the full
// (command, flag) lookup pipeline against the real embedded
// flag-schemas.json — confirms that an out-of-enum summarize_by
// surfaces a descriptive error all the way through, and that a
// well-formed input passes. Mirrors what shortcut tests check, but
// without booting cobra.
func TestValidateInputAgainstSchema_RealSchema(t *testing.T) {
	t.Parallel()
	fv := mapFlagView{command: "+pivot-create"}

	// Schema-conformant: values[0].summarize_by="sum" is in enum.
	good := map[string]interface{}{
		"properties": map[string]interface{}{
			"values": []interface{}{
				map[string]interface{}{"field": "A", "summarize_by": "sum"},
			},
		},
	}
	if err := validateInputAgainstSchema(fv, good); err != nil {
		t.Errorf("good input rejected: %v", err)
	}

	// Schema-violating: a value with no case-only match still fails loudly
	// (case normalization only rescues casing mistakes, not unknown words).
	bad := map[string]interface{}{
		"properties": map[string]interface{}{
			"values": []interface{}{
				map[string]interface{}{"field": "A", "summarize_by": "bogus"},
			},
		},
	}
	err := validateInputAgainstSchema(fv, bad)
	ve := requireValidation(t, err, "summarize_by")
	if !strings.Contains(ve.Message, "not in enum") {
		t.Errorf("error = %q, want enum hint", ve.Message)
	}
}

// TestValidateInputAgainstSchema_RealMinItems exercises a P0
// addition end-to-end: +pivot-create properties.values has
// minItems:1, so an explicit empty values array is rejected by the
// schema validator (previously slipped past).
func TestValidateInputAgainstSchema_RealMinItems(t *testing.T) {
	t.Parallel()
	fv := mapFlagView{command: "+pivot-create"}
	bad := map[string]interface{}{
		"properties": map[string]interface{}{
			"values": []interface{}{}, // minItems:1 violated
		},
	}
	err := validateInputAgainstSchema(fv, bad)
	ve := requireValidation(t, err, "values")
	if !strings.Contains(ve.Message, "minimum is 1") {
		t.Errorf("error = %q, want minimum-is-1 hint", ve.Message)
	}
}

// TestValidateInputAgainstSchema_RealMinimum exercises another P0
// addition: +chart-create properties.position.row has minimum:0, so
// row:-1 must be rejected before the request hits the wire.
func TestValidateInputAgainstSchema_RealMinimum(t *testing.T) {
	t.Parallel()
	fv := mapFlagView{command: "+chart-create"}
	bad := map[string]interface{}{
		"properties": map[string]interface{}{
			"position": map[string]interface{}{"row": float64(-1), "col": "A"},
			"size":     map[string]interface{}{"width": float64(400), "height": float64(300)},
		},
	}
	err := validateInputAgainstSchema(fv, bad)
	ve := requireValidation(t, err, "row")
	if !strings.Contains(ve.Message, "below minimum") {
		t.Errorf("error = %q, want below-minimum hint", ve.Message)
	}
}

// TestValidateInputAgainstSchema_RealAdditionalProperties pins the
// additionalProperties: <schema> form against the real embedded
// schema. +pivot-create properties.collapse is declared as a dynamic
// map<field-name, array<string>>; passing a non-string in any value
// must be rejected end-to-end.
func TestValidateInputAgainstSchema_RealAdditionalProperties(t *testing.T) {
	t.Parallel()
	fv := mapFlagView{command: "+pivot-create"}

	good := map[string]interface{}{
		"properties": map[string]interface{}{
			"values":   []interface{}{map[string]interface{}{"field": "A", "summarize_by": "sum"}},
			"collapse": map[string]interface{}{"region": []interface{}{"NA", "EU"}},
		},
	}
	if err := validateInputAgainstSchema(fv, good); err != nil {
		t.Errorf("schema-conformant collapse rejected: %v", err)
	}

	bad := map[string]interface{}{
		"properties": map[string]interface{}{
			"values":   []interface{}{map[string]interface{}{"field": "A", "summarize_by": "sum"}},
			"collapse": map[string]interface{}{"region": []interface{}{"NA", 42}}, // 42 violates items.type=string
		},
	}
	err := validateInputAgainstSchema(fv, bad)
	ve := requireValidation(t, err, "collapse")
	if !strings.Contains(ve.Message, `expected type "string"`) {
		t.Errorf("error = %q, want string-type hint", ve.Message)
	}
}

// TestValidateInputAgainstSchema_UnknownCommand returns nil — schema
// validation is opportunistic, an unknown command never errors. Lets
// shortcuts opt out simply by not registering a schema entry.
func TestValidateInputAgainstSchema_UnknownCommand(t *testing.T) {
	t.Parallel()
	fv := mapFlagView{command: "+definitely-not-a-shortcut"}
	if err := validateInputAgainstSchema(fv, map[string]interface{}{"properties": "anything"}); err != nil {
		t.Errorf("unknown command should noop; got %v", err)
	}
}

// TestValidateInputAgainstSchema_SkipOperations confirms that the
// operations skip-list entry is honoured: even with a clearly
// malformed operations value, validateInputAgainstSchema is a no-op
// because translator-side validation owns that contract.
func TestValidateInputAgainstSchema_SkipOperations(t *testing.T) {
	t.Parallel()
	fv := mapFlagView{command: "+batch-update"}
	input := map[string]interface{}{
		"operations": "definitely-not-an-array",
	}
	if err := validateInputAgainstSchema(fv, input); err != nil {
		t.Errorf("operations should be skipped; got %v", err)
	}
}

// TestValidateValueAgainstSchema_PrintSchemaHint pins the highest-value
// recovery affordance for composite-JSON flags: when the shape is wrong, the
// error must point the agent straight at --print-schema (with the right
// command + flag) instead of leaving it to guess across retries. +cells-set
// --cells expects a 2-D array; a bare string trips the top-level type check.
func TestValidateValueAgainstSchema_PrintSchemaHint(t *testing.T) {
	t.Parallel()
	fv := mapFlagView{command: "+cells-set"}
	err := validateValueAgainstSchema(fv, "cells", "not-an-array")
	// Underlying shape error is preserved (substring callers still match).
	ve := requireValidation(t, err, `expected type "array"`)
	// And the actionable --print-schema hint is appended with the exact
	// command + flag, so a copy-paste fetches the schema for this pair.
	if !strings.Contains(ve.Message, "lark-cli sheets +cells-set --print-schema --flag-name cells") {
		t.Errorf("want --print-schema hint with command+flag; got %q", ve.Message)
	}
	if ve.Param != "--cells" {
		t.Errorf("param = %q, want --cells", ve.Param)
	}
}
