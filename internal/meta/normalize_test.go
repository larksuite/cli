// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package meta

import (
	"reflect"
	"testing"
)

func TestField_CanonicalType(t *testing.T) {
	cases := map[string]string{
		"file":    "string", // meta_data's non-standard "file" is a string with binary format
		"list":    "array",  // "list" is meta_data's spelling of a JSON array
		"integer": "integer",
		"boolean": "boolean",
		"string":  "string",
		"":        "",
	}
	for in, want := range cases {
		if got := (Field{Type: in}).CanonicalType(); got != want {
			t.Errorf("CanonicalType(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestField_EnumValues(t *testing.T) {
	// string enum keeps source order (order can encode priority)
	if got := (Field{Type: "string", Enum: []any{"b", "a", "c"}}).EnumValues(); !reflect.DeepEqual(got, []any{"b", "a", "c"}) {
		t.Errorf("string enum = %v, want source order [b a c]", got)
	}
	// integer enum: string-stored literals coerced to int64 and numerically sorted
	if got := (Field{Type: "integer", Enum: []any{"10", "2", "1"}}).EnumValues(); !reflect.DeepEqual(got, []any{int64(1), int64(2), int64(10)}) {
		t.Errorf("integer enum = %v, want [1 2 10] coerced+sorted", got)
	}
	// options used when enum absent, deduped, source order for strings
	if got := (Field{Type: "string", Options: []Option{{Value: "x"}, {Value: "x"}, {Value: "y"}}}).EnumValues(); !reflect.DeepEqual(got, []any{"x", "y"}) {
		t.Errorf("options enum = %v, want [x y] deduped", got)
	}
	// uncoercible literal dropped
	if got := (Field{Type: "integer", Enum: []any{"1", "nope", "2"}}).EnumValues(); !reflect.DeepEqual(got, []any{int64(1), int64(2)}) {
		t.Errorf("bad enum = %v, want [1 2] (nope dropped)", got)
	}
	// no enum/options -> nil
	if got := (Field{Type: "string"}).EnumValues(); got != nil {
		t.Errorf("empty enum = %v, want nil", got)
	}
}

func TestField_EnumOptions(t *testing.T) {
	// options carry descriptions, kept paired with their (string) value in source order
	fo := Field{Type: "string", Options: []Option{
		{Value: "open_id", Description: "以 open_id 标识"},
		{Value: "open_id", Description: "dup ignored"}, // dedup keeps first
		{Value: "user_id", Description: "以 user_id 标识"},
	}}
	got := fo.EnumOptions()
	want := []EnumOption{
		{Value: "open_id", Description: "以 open_id 标识"},
		{Value: "user_id", Description: "以 user_id 标识"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("EnumOptions = %+v, want %+v", got, want)
	}

	// integer enum (bare form): values coerced + numerically sorted, no descriptions
	fi := Field{Type: "integer", Enum: []any{"10", "2", "1"}}
	gi := fi.EnumOptions()
	if len(gi) != 3 || gi[0].Value != int64(1) || gi[2].Value != int64(10) || gi[0].Description != "" {
		t.Errorf("EnumOptions(integer) = %+v, want [1 2 10] coerced+sorted, no desc", gi)
	}

	// EnumValues stays the value projection of EnumOptions (golden-critical)
	if !reflect.DeepEqual(fo.EnumValues(), []any{"open_id", "user_id"}) {
		t.Errorf("EnumValues diverged from EnumOptions values: %v", fo.EnumValues())
	}
	// unconstrained -> nil
	if (Field{Type: "string"}).EnumOptions() != nil {
		t.Error("EnumOptions should be nil when unconstrained")
	}
}

func TestField_EnumOptions_BothEnumAndOptions(t *testing.T) {
	// enum is the value set; descriptions backfilled from options, empty where absent
	f := Field{Type: "string", Enum: []any{"1", "2", "3", "4", "6"}, Options: []Option{
		{Value: "1", Description: "from"},
		{Value: "2", Description: "to"},
		{Value: "6", Description: "subject"},
	}}
	want := []EnumOption{
		{Value: "1", Description: "from"},
		{Value: "2", Description: "to"},
		{Value: "3", Description: ""},
		{Value: "4", Description: ""},
		{Value: "6", Description: "subject"},
	}
	if got := f.EnumOptions(); !reflect.DeepEqual(got, want) {
		t.Errorf("EnumOptions(enum+options) = %+v, want %+v", got, want)
	}

	// enum values stored as strings match option values stored as numbers
	fi := Field{Type: "integer", Enum: []any{"10", "2", "1"}, Options: []Option{
		{Value: 1, Description: "one"},
		{Value: 2, Description: "two"},
	}}
	wantI := []EnumOption{
		{Value: int64(1), Description: "one"},
		{Value: int64(2), Description: "two"},
		{Value: int64(10), Description: ""},
	}
	if got := fi.EnumOptions(); !reflect.DeepEqual(got, wantI) {
		t.Errorf("EnumOptions(integer enum+options) = %+v, want %+v", got, wantI)
	}
}

func TestField_Enum_NumberAndBoolean(t *testing.T) {
	// number: string-stored floats coerced to float64 and numerically sorted
	if got := (Field{Type: "number", Enum: []any{"2.5", "1.5", "10"}}).EnumValues(); !reflect.DeepEqual(got, []any{1.5, 2.5, float64(10)}) {
		t.Errorf("number enum = %v, want [1.5 2.5 10] coerced+sorted", got)
	}
	// number: uncoercible literal dropped
	if got := (Field{Type: "number", Enum: []any{"1.5", "x", "2.5"}}).EnumValues(); !reflect.DeepEqual(got, []any{1.5, 2.5}) {
		t.Errorf("number enum with bad value = %v, want [1.5 2.5]", got)
	}
	// boolean: true/false coerced and sorted (false before true); invalid dropped
	if got := (Field{Type: "boolean", Enum: []any{"true", "maybe", "false"}}).EnumValues(); !reflect.DeepEqual(got, []any{false, true}) {
		t.Errorf("boolean enum = %v, want [false true]", got)
	}
}

func TestField_EnumOptions_NonStringValuesNormalized(t *testing.T) {
	// JSON numbers/bools arrive already-typed (a number is float64, not a
	// string) — e.g. options[].value: 0. They must still be normalized to the
	// field's canonical type (int64 for "integer") and sorted numerically;
	// leaving them as float64 both yields the wrong type and defeats enumLess,
	// whose integer branch asserts int64 and would otherwise treat every value
	// as zero (no sort).
	if got := (Field{Type: "integer", Options: []Option{
		{Value: float64(10)}, {Value: float64(2)}, {Value: float64(1)},
	}}).EnumValues(); !reflect.DeepEqual(got, []any{int64(1), int64(2), int64(10)}) {
		t.Errorf("integer options from float64 = %#v, want [int64(1) int64(2) int64(10)]", got)
	}
	// bare enum form, JSON numbers
	if got := (Field{Type: "integer", Enum: []any{float64(3), float64(1), float64(2)}}).EnumValues(); !reflect.DeepEqual(got, []any{int64(1), int64(2), int64(3)}) {
		t.Errorf("integer enum from float64 = %#v, want [int64(1) int64(2) int64(3)]", got)
	}
	// number field: a whole-valued float stays float64
	if got := (Field{Type: "number", Enum: []any{float64(2), float64(1)}}).EnumValues(); !reflect.DeepEqual(got, []any{float64(1), float64(2)}) {
		t.Errorf("number enum from float64 = %#v, want [float64(1) float64(2)]", got)
	}
	// boolean field: native bools coerce + sort (false < true)
	if got := (Field{Type: "boolean", Enum: []any{true, false}}).EnumValues(); !reflect.DeepEqual(got, []any{false, true}) {
		t.Errorf("boolean enum from bool = %#v, want [false true]", got)
	}
	// non-integral float under integer is uncoercible -> dropped (mirrors how "2.5" fails ParseInt)
	if got := (Field{Type: "integer", Enum: []any{float64(1), float64(2.5), float64(3)}}).EnumValues(); !reflect.DeepEqual(got, []any{int64(1), int64(3)}) {
		t.Errorf("integer enum with fractional float = %#v, want [int64(1) int64(3)]", got)
	}
}

func TestField_CoercedDefaultAndExample(t *testing.T) {
	if got := (Field{Type: "integer", Default: "5"}).CoercedDefault(); got != int64(5) {
		t.Errorf("CoercedDefault integer = %v (%T), want int64(5)", got, got)
	}
	if got := (Field{Type: "integer", Default: "bad"}).CoercedDefault(); got != nil {
		t.Errorf("CoercedDefault uncoercible = %v, want nil", got)
	}
	if got := (Field{Type: "string"}).CoercedDefault(); got != nil {
		t.Errorf("CoercedDefault absent = %v, want nil", got)
	}
	if got := (Field{Type: "boolean", Example: "true"}).CoercedExample(); got != true {
		t.Errorf("CoercedExample boolean = %v, want true", got)
	}
}

func TestField_Bounds(t *testing.T) {
	f := Field{Min: "1", Max: "100"}
	if v := f.MinBound(); v == nil || *v != 1 {
		t.Errorf("MinBound = %v, want 1", v)
	}
	if v := f.MaxBound(); v == nil || *v != 100 {
		t.Errorf("MaxBound = %v, want 100", v)
	}
	if v := (Field{Min: "0.5"}).MinBound(); v == nil || *v != 0.5 {
		t.Errorf("MinBound fractional = %v, want 0.5", v)
	}
	if v := (Field{}).MinBound(); v != nil {
		t.Errorf("MinBound absent = %v, want nil", v)
	}
	if v := (Field{Max: "not_a_number"}).MaxBound(); v != nil {
		t.Errorf("MaxBound unparseable = %v, want nil", v)
	}
}
