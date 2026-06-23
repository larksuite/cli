// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package meta

import (
	"fmt"
	"sort"
	"strconv"
)

// CanonicalType maps meta_data's non-standard type names to the standard
// JSON-Schema/type vocabulary used downstream (envelope render, flag kinds):
// "file" -> "string", "list" -> "array"; other types pass through unchanged.
func (f Field) CanonicalType() string {
	switch f.Type {
	case "file":
		return "string"
	case "list":
		return "array"
	default:
		return f.Type
	}
}

// coerceLiteral converts a meta_data literal (default/enum/example) to the
// field's canonical type. Literals may arrive as strings (meta_data's usual
// form) OR already typed — a JSON number unmarshals to float64, a JSON bool to
// bool — so both must be normalized to the SAME Go type the canonical type
// implies (int64 for "integer", float64 for "number", bool for "boolean").
// Otherwise enumLess, which type-asserts on that Go type, can't order the
// values. Returns (value, true) on success, (nil, false) when the literal
// cannot be represented in the declared type.
func coerceLiteral(canonicalType string, raw any) (any, bool) {
	switch canonicalType {
	case "integer":
		switch v := raw.(type) {
		case string:
			if n, err := strconv.ParseInt(v, 10, 64); err == nil {
				return n, true
			}
		case float64: // JSON number; accept only when it's a whole value
			if v == float64(int64(v)) {
				return int64(v), true
			}
		case int64:
			return v, true
		case int:
			return int64(v), true
		}
		return nil, false
	case "number":
		switch v := raw.(type) {
		case string:
			if n, err := strconv.ParseFloat(v, 64); err == nil {
				return n, true
			}
		case float64:
			return v, true
		case int64:
			return float64(v), true
		case int:
			return float64(v), true
		}
		return nil, false
	case "boolean":
		switch v := raw.(type) {
		case string:
			switch v {
			case "true":
				return true, true
			case "false":
				return false, true
			}
		case bool:
			return v, true
		}
		return nil, false
	default: // "string", "array", "" (objects), or unknown — pass through as-is
		return raw, true
	}
}

// enumLess orders two coerced enum values for the canonical type, so integer
// enums end up [1 2 10] not lexicographic [1 10 2].
func enumLess(canonicalType string, a, b any) bool {
	switch canonicalType {
	case "integer":
		ai, _ := a.(int64)
		bi, _ := b.(int64)
		return ai < bi
	case "number":
		af, _ := a.(float64)
		bf, _ := b.(float64)
		return af < bf
	case "boolean":
		ab, _ := a.(bool)
		bb, _ := b.(bool)
		return !ab && bb
	default:
		as, _ := a.(string)
		bs, _ := b.(string)
		return as < bs
	}
}

// EnumOption is one allowed value paired with its human description. The
// description comes from options[].description and is empty for the bare `enum`
// form (which carries no descriptions).
type EnumOption struct {
	Value       any
	Description string
}

// EnumOptions returns the field's allowed values paired with their descriptions
// — from enum (with descriptions backfilled from options when the field carries
// both forms), or from options when enum is absent — coerced to the canonical
// type and ordered: numeric and boolean values are sorted; string values keep
// source order (which can encode priority). Uncoercible literals are dropped.
// Returns nil when the field declares no enum constraint.
func (f Field) EnumOptions() []EnumOption {
	ct := f.CanonicalType()
	var out []EnumOption
	switch {
	case len(f.Enum) > 0:
		// key by raw literal so enum "1" and option 1 align across JSON types
		desc := make(map[string]string, len(f.Options))
		for _, o := range f.Options {
			desc[fmt.Sprintf("%v", o.Value)] = o.Description
		}
		for _, e := range f.Enum {
			if v, ok := coerceLiteral(ct, e); ok {
				out = append(out, EnumOption{Value: v, Description: desc[fmt.Sprintf("%v", e)]})
			}
		}
	case len(f.Options) > 0:
		seen := make(map[string]bool)
		for _, o := range f.Options {
			key := fmt.Sprintf("%v", o.Value)
			if seen[key] {
				continue
			}
			seen[key] = true
			if v, ok := coerceLiteral(ct, o.Value); ok {
				out = append(out, EnumOption{Value: v, Description: o.Description})
			}
		}
	}
	if len(out) > 0 && ct != "string" && ct != "" {
		sort.SliceStable(out, func(i, j int) bool { return enumLess(ct, out[i].Value, out[j].Value) })
	}
	return out
}

// EnumValues returns the field's allowed values — the value projection of
// EnumOptions, in the same order. nil when the field declares no enum
// constraint. (Kept as the values-only accessor for the envelope and flag
// completion, which don't need descriptions.)
func (f Field) EnumValues() []any {
	opts := f.EnumOptions()
	if len(opts) == 0 {
		return nil
	}
	out := make([]any, len(opts))
	for i, o := range opts {
		out[i] = o.Value
	}
	return out
}

// CoercedDefault returns Default coerced to the canonical type, or nil when the
// field has no default or the literal cannot be coerced.
func (f Field) CoercedDefault() any { return f.coerce(f.Default) }

// CoercedExample returns Example coerced to the canonical type, or nil when the
// field has no example or the literal cannot be coerced.
func (f Field) CoercedExample() any { return f.coerce(f.Example) }

func (f Field) coerce(raw any) any {
	if raw == nil {
		return nil
	}
	if v, ok := coerceLiteral(f.CanonicalType(), raw); ok {
		return v
	}
	return nil
}

// MinBound returns the field's min constraint as a number, or nil when absent
// or unparseable. meta_data carries min/max as strings and does not say
// whether they bound a value or a string's length; the accessors stay equally
// agnostic, so every renderer (envelope minimum/maximum, flag help) presents
// the same numbers without inventing a semantic the source doesn't declare.
func (f Field) MinBound() *float64 { return parseBound(f.Min) }

// MaxBound returns the field's max constraint as a number, or nil when absent
// or unparseable. See MinBound.
func (f Field) MaxBound() *float64 { return parseBound(f.Max) }

func parseBound(s string) *float64 {
	if s == "" {
		return nil
	}
	if v, err := strconv.ParseFloat(s, 64); err == nil {
		return &v
	}
	return nil
}
