// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package schema

import (
	"encoding/json"
	"fmt"
	"math/big"
	"reflect"
	"strings"
	"testing"
)

func validationSchema(property Property) *InputSchema {
	return &InputSchema{Type: "object", Properties: &OrderedProps{Map: map[string]Property{"value": property}}}
}

func TestValidateInputTypes(t *testing.T) {
	for _, tt := range []struct {
		kind, value string
		valid       bool
	}{
		{"string", `"text"`, true}, {"string", `5`, false}, {"string", `null`, false},
		{"boolean", `false`, true}, {"boolean", `"false"`, false}, {"boolean", `0`, false},
		{"object", `{}`, true}, {"object", `[]`, false}, {"object", `null`, false},
		{"array", `[]`, true}, {"array", `{}`, false}, {"array", `null`, false},
		{"number", `0.5`, true}, {"number", `1e1000000000`, true}, {"number", `"1"`, false},
		{"integer", `9223372036854775807`, true}, {"integer", `1.00`, true}, {"integer", `10e-1`, true},
		{"integer", `1.1`, false}, {"integer", `1e-1000000000`, false}, {"integer", `0e-999999999999999999999`, true},
		{"integer", `1e999999999999999999999`, true}, {"integer", `"1"`, false},
		{"null", `null`, true}, {"null", `false`, false}, {"", `null`, true}, {"", `{"a":[1]}`, true},
	} {
		t.Run(tt.kind+"/"+tt.value, func(t *testing.T) {
			result, err := ValidateInput(validationSchema(Property{Type: tt.kind}), `{"value":`+tt.value+`}`)
			if err != nil {
				t.Fatal(err)
			}
			if got := len(result.Violations) == 0; got != tt.valid {
				t.Fatalf("valid=%v; violations=%+v", got, result.Violations)
			}
			if !tt.valid && result.Violations[0].Path != "/value" {
				t.Fatalf("path=%q", result.Violations[0].Path)
			}
		})
	}
}

func TestValidateInputNestedRequiredAndPointers(t *testing.T) {
	inputSchema := &InputSchema{Type: "object", Required: []string{"data", "params"}, Properties: &OrderedProps{Map: map[string]Property{
		"params": {Type: "object", Required: []string{"a/b~c"}, Properties: &OrderedProps{Map: map[string]Property{"a/b~c": {Type: "string"}}}},
		"data":   {Type: "array", Items: &Property{Type: "object", Required: []string{"id"}, Properties: &OrderedProps{Map: map[string]Property{"id": {Type: "integer"}}}}},
	}}}
	result, err := ValidateInput(inputSchema, `{"params":{"a/b~c":null},"data":[{}, {"id":"PRIVATE_VALUE"}, {"id":1}]}`)
	if err != nil {
		t.Fatal(err)
	}
	want := []InputViolation{{"/data/0/id", "required property is missing"}, {"/data/1/id", "expected integer"}, {"/params/a~1b~0c", "expected string"}}
	if !reflect.DeepEqual(result.Violations, want) {
		t.Fatalf("got=%+v want=%+v", result.Violations, want)
	}
	for range 10 {
		again, err := ValidateInput(inputSchema, `{"data":[{}, {"id":"PRIVATE_VALUE"}, {"id":1}],"params":{"a/b~c":null}}`)
		if err != nil || !reflect.DeepEqual(again, result) {
			t.Fatalf("nondeterministic result=%+v err=%v", again, err)
		}
	}
	if strings.Contains(fmt.Sprint(result), "PRIVATE_VALUE") {
		t.Fatal("diagnostics contain submitted value")
	}
	missing, err := ValidateInput(inputSchema, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(missing.Violations, []InputViolation{{"/data", "required property is missing"}, {"/params", "required property is missing"}}) {
		t.Fatal(missing)
	}
}

func TestValidateInputRootAndRequiredDefaults(t *testing.T) {
	inputSchema := &InputSchema{Type: "object", Required: []string{"value"}, Properties: &OrderedProps{Map: map[string]Property{
		"value": {Type: "boolean", Default: true},
	}}}
	for _, input := range []string{`null`, `[]`, `"text"`, `12`, `false`} {
		result, err := ValidateInput(inputSchema, input)
		if err != nil || !reflect.DeepEqual(result.Violations, []InputViolation{{"", "expected object"}}) {
			t.Fatalf("%s: %+v %v", input, result, err)
		}
	}
	result, err := ValidateInput(inputSchema, `{}`)
	if err != nil || len(result.Violations) != 1 {
		t.Fatalf("default must not fill missing input: %+v %v", result, err)
	}
	result, err = ValidateInput(inputSchema, `{"value":false}`)
	if err != nil || len(result.Violations) != 0 {
		t.Fatalf("false is present: %+v %v", result, err)
	}
}

func TestValidateInputOpenObjectsFormatsAndEmptyItems(t *testing.T) {
	inputSchema := validationSchema(Property{Type: "object", Properties: &OrderedProps{Map: map[string]Property{
		"file":     {Type: "string", Format: "binary"},
		"date":     {Type: "string", Format: "date-time"},
		"items":    {Type: "array", Items: &Property{}},
		"no_items": {Type: "array"},
	}}})
	input := `{"unknown":"allowed","value":{"file":"/does/not/exist","date":"not-a-date","items":[1,null,{}],"no_items":[false],"extra":5}}`
	result, err := ValidateInput(inputSchema, input)
	if err != nil || len(result.Violations) != 0 {
		t.Fatalf("annotations/open objects: %+v %v", result, err)
	}
}

func TestValidateInputEnums(t *testing.T) {
	for _, tt := range []struct {
		name    string
		options []any
		value   string
		valid   bool
	}{
		{"string", []any{"active", "done"}, `"done"`, true},
		{"case", []any{"active"}, `"ACTIVE"`, false},
		{"integer", []any{int64(9223372036854775807)}, `9223372036854775807`, true},
		{"integer-neighbor", []any{int64(9223372036854775807)}, `9223372036854775806`, false},
		{"equivalent-numbers", []any{json.Number("10")}, `1e1`, true},
		{"boolean", []any{false}, `false`, true},
		{"no-coercion", []any{true}, `"true"`, false},
		{"null", []any{nil}, `null`, true},
		{"object", []any{map[string]any{"a": 1, "b": nil}}, `{"b":null,"a":1.0}`, true},
		{"missing-null-key", []any{map[string]any{"a": nil}}, `{"b":null}`, false},
		{"array", []any{[]any{1, "x"}}, `[1.0,"x"]`, true},
		{"array-order", []any{[]any{1, "x"}}, `["x",1]`, false},
		{"scalar-vs-object", []any{map[string]any{}}, `false`, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ValidateInput(validationSchema(Property{Enum: tt.options}), `{"value":`+tt.value+`}`)
			if err != nil || (len(result.Violations) == 0) != tt.valid {
				t.Fatalf("result=%+v err=%v", result, err)
			}
		})
	}
}

func TestValidateInputExactNumericBounds(t *testing.T) {
	for _, tt := range []struct {
		min, max, value string
		valid           bool
	}{
		{"0", "9223372036854775807", "9223372036854775807", true},
		{"0", "9223372036854775807", "9223372036854775808", false},
		{"9007199254740993", "9007199254740994", "9007199254740992", false},
		{"0.1", "0.3", "0.3", true}, {"0.1", "0.3", "0.30000000000000000001", false},
		{"-2", "-0.5", "-0.5000000000000001", true}, {"-2", "-0.5", "-0.4999999999999999", false},
		{"1e1000000000", "2e1000000000", "15e999999999", true},
		{"1e1000000000", "2e1000000000", "1e999999999999999999999999", false},
		{"0", "1e-999999999999999999999", "0e999999999999999999999999", true},
	} {
		t.Run(tt.value+"/"+tt.max, func(t *testing.T) {
			minimum, maximum := json.Number(tt.min), json.Number(tt.max)
			result, err := ValidateInput(validationSchema(Property{Type: "number", Minimum: &minimum, Maximum: &maximum}), `{"value":`+tt.value+`}`)
			if err != nil || (len(result.Violations) == 0) != tt.valid {
				t.Fatalf("result=%+v err=%v", result, err)
			}
		})
	}
}

func TestValidateInputMalformedDocuments(t *testing.T) {
	for _, input := range []string{"", " ", `{`, `{"x":`, `{"x":1,}`, `{1:2}`, `[1,]`, `{} {}`, `{} junk`, `{"value":NaN}`, `{"value":01}`, `{"value":1e}`, `{"x":1,"x":2}`, `{"x":1,"\u0078":2}`, `{"extra":[{"x":1,"x":2}]}`} {
		t.Run(input, func(t *testing.T) {
			if _, err := ValidateInput(validationSchema(Property{}), input); err == nil {
				t.Fatal("accepted invalid document")
			}
		})
	}
	if _, err := ValidateInput(nil, `{}`); err == nil {
		t.Fatal("accepted missing schema")
	}
}

func TestValidateInputDepthLimitAndDiagnosticLimit(t *testing.T) {
	inputSchema := validationSchema(Property{Type: "array", Items: &Property{Type: "integer"}})
	for _, count := range []int{MaxInputViolations, MaxInputViolations + 1, 1000} {
		input := `{"value":[` + strings.TrimSuffix(strings.Repeat(`"bad",`, count), ",") + `]}`
		result, err := ValidateInput(inputSchema, input)
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Violations) != min(count, MaxInputViolations) || result.Truncated != (count > MaxInputViolations) {
			t.Fatal(result)
		}
		if result.Violations[99].Path != "/value/99" {
			t.Fatal(result.Violations[99])
		}
	}
	for _, depth := range []int{MaxInputDepth - 1, MaxInputDepth} {
		input := `{"extra":` + strings.Repeat("[", depth) + "0" + strings.Repeat("]", depth) + `}`
		_, err := ValidateInput(validationSchema(Property{}), input)
		if (err != nil) != (depth == MaxInputDepth) {
			t.Fatalf("depth=%d err=%v", depth, err)
		}
	}
}

func TestCompareDecimalAgainstExactRationals(t *testing.T) {
	// Check an independent representation over signs, exponents, and fractions.
	values := []string{"0", "-0.0", "0e-20", "1", "-1", "1.000", "10e-1", "0.001", "-0.001", "12345.67", "9e8", "1e-20", "9223372036854775807", "9223372036854775808"}
	for _, a := range values {
		for _, b := range values {
			left, _ := new(big.Rat).SetString(a)
			right, _ := new(big.Rat).SetString(b)
			if got, want := compareDecimal(a, b), left.Cmp(right); got != want {
				t.Fatalf("%s vs %s: %d != %d", a, b, got, want)
			}
		}
	}
}

func FuzzValidateInput(f *testing.F) {
	for _, input := range []string{`{}`, `{"value":1}`, `{"value":[null]}`, `{"value":1e1000000000}`, `{"x":1,"x":2}`} {
		f.Add(input)
	}
	f.Fuzz(func(t *testing.T, input string) {
		result, _ := ValidateInput(validationSchema(Property{Type: "array", Items: &Property{Enum: []any{1, "x", nil}}}), input)
		if len(result.Violations) > MaxInputViolations {
			t.Fatal("unbounded diagnostics")
		}
	})
}
