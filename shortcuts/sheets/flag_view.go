// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// flagView is the read-only flag-accessor surface that every CLI-shape →
// MCP-tool-body translator (the *Input builders) depends on. It is satisfied
// as-is by *common.RuntimeContext (cobra-backed, used by standalone shortcut
// execution) and by mapFlagView (map-backed, used by +batch-update sub-ops).
//
// Routing both paths through the same interface lets a sub-op inside
// +batch-update reuse the exact same translator the standalone shortcut runs,
// so the generated MCP body is identical either way (enforced by the
// batch-vs-standalone contract test).
type flagView interface {
	Str(name string) string
	Int(name string) int
	Float64(name string) float64
	Bool(name string) bool
	StrArray(name string) []string
	StrSlice(name string) []string
	Changed(name string) bool
	// Command returns the shortcut command this view feeds (e.g.
	// "+pivot-create"). Used to look up the schema entry for
	// schema-driven flag validation; both standalone and batch sub-op
	// paths populate it so a sub-op gets validated against the same
	// schema as the standalone shortcut.
	Command() string
}

// mapFlagView adapts a +batch-update sub-op input object (decoded JSON) to the
// flagView interface so the standalone *Input translators can consume it.
//
// Keys are matched leniently against the CLI flag name: a translator asking for
// "source-range" finds either "source-range" or "source_range" in the map (the
// reference docs use CLI flag names; users frequently send the underscore
// form). Composite values (arrays / objects for flags like cells / properties /
// sort-keys) are re-encoded to a JSON string on Str() so the downstream
// parseJSONFlag round-trips them exactly as it would a CLI string argument.
//
// To mirror the standalone cobra layer exactly, value reads fall back to the
// flag's declared default (seeded from flag-defs.json), while Changed() reflects
// only what the user actually provided. This split matters because some
// translators branch on Changed() (e.g. omit target_index unless --index was
// set) and others read defaulted values (e.g. row-count defaults to 200).
type mapFlagView struct {
	raw      map[string]interface{} // user-supplied sub-op input (drives Changed)
	defaults map[string]interface{} // flag defaults (value fallback only)
	command  string                 // shortcut command (e.g. "+chart-create"); used by schema validator
}

func (m mapFlagView) Command() string { return m.command }

// newMapFlagViewForCommand wraps a sub-op input and seeds the value-fallback
// defaults declared for `command` in flag-defs.json, so an absent flag resolves
// to the same value the standalone cobra command would carry.
func newMapFlagViewForCommand(command string, input map[string]interface{}) mapFlagView {
	fv := mapFlagView{raw: input, defaults: map[string]interface{}{}, command: command}
	defs, err := loadFlagDefs()
	if err != nil {
		return fv
	}
	spec, ok := defs[command]
	if !ok {
		return fv
	}
	for _, df := range spec.Flags {
		if df.Kind == "system" || df.Default == "" {
			continue
		}
		fv.defaults[df.Name] = typedDefault(df)
	}
	return fv
}

// typedDefault converts a flag's string default to the Go type matching its
// declared kind, so Int()/Bool()/Float64() see the right type.
func typedDefault(df flagDef) interface{} {
	switch df.Type {
	case "bool":
		return df.Default == "true"
	case "int":
		var n int
		fmt.Sscanf(df.Default, "%d", &n)
		return n
	case "float64":
		var f float64
		fmt.Sscanf(df.Default, "%g", &f)
		return f
	default:
		return df.Default
	}
}

// lookup resolves a flag name for a VALUE read: user input first (hyphen↔
// underscore tolerant), then the seeded default. Returns the value and whether
// it was found in either source.
func (m mapFlagView) lookup(name string) (interface{}, bool) {
	if v, ok := m.lookupRaw(name); ok {
		return v, true
	}
	if m.defaults != nil {
		if v, ok := m.defaults[name]; ok {
			return v, true
		}
	}
	return nil, false
}

// lookupRaw resolves a flag name against the user-supplied input only, trying
// the exact key then the hyphen↔underscore variants.
func (m mapFlagView) lookupRaw(name string) (interface{}, bool) {
	if v, ok := m.raw[name]; ok {
		return v, true
	}
	if alt := strings.ReplaceAll(name, "-", "_"); alt != name {
		if v, ok := m.raw[alt]; ok {
			return v, true
		}
	}
	if alt := strings.ReplaceAll(name, "_", "-"); alt != name {
		if v, ok := m.raw[alt]; ok {
			return v, true
		}
	}
	return nil, false
}

func (m mapFlagView) Str(name string) string {
	v, ok := m.lookup(name)
	if !ok || v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	case bool, float64, int, int64:
		b, _ := json.Marshal(t)
		return string(b)
	default:
		// Arrays / objects (cells, properties, sort-keys, options, ...) are
		// re-encoded so the translator's parseJSONFlag re-parses them.
		b, err := json.Marshal(t)
		if err != nil {
			return ""
		}
		return string(b)
	}
}

func (m mapFlagView) Int(name string) int {
	v, ok := m.lookup(name)
	if !ok {
		return 0
	}
	switch t := v.(type) {
	case float64:
		return int(t)
	case int:
		return t
	case int64:
		return int(t)
	}
	return 0
}

func (m mapFlagView) Float64(name string) float64 {
	v, ok := m.lookup(name)
	if !ok {
		return 0
	}
	switch t := v.(type) {
	case float64:
		return t
	case int:
		return float64(t)
	case int64:
		return float64(t)
	}
	return 0
}

func (m mapFlagView) Bool(name string) bool {
	v, ok := m.lookup(name)
	if !ok {
		return false
	}
	b, _ := v.(bool)
	return b
}

func (m mapFlagView) StrArray(name string) []string {
	return m.strSliceLike(name)
}

func (m mapFlagView) StrSlice(name string) []string {
	return m.strSliceLike(name)
}

func (m mapFlagView) strSliceLike(name string) []string {
	v, ok := m.lookup(name)
	if !ok || v == nil {
		return nil
	}
	switch t := v.(type) {
	case []string:
		return t
	case []interface{}:
		out := make([]string, 0, len(t))
		for _, e := range t {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
		return out
	case string:
		// CSV / comma-separated (matches cobra StringSlice behavior).
		if t == "" {
			return nil
		}
		return strings.Split(t, ",")
	}
	return nil
}

func (m mapFlagView) Changed(name string) bool {
	_, ok := m.lookupRaw(name)
	return ok
}

// validateRawTypes rejects sub-op input fields whose JSON type contradicts the
// flag's declared type in flag-defs. +batch-update skips parse-time schema
// validation for `operations`, and Int/Float64/Bool silently fall back to
// the zero value on a type mismatch — so without this guard a wrong-typed scalar
// (e.g. "index":"abc" or "multiple":"true") would land as 0 / false instead of
// erroring, writing to the wrong place. Only numeric and boolean flags are
// checked; string and composite (array/object) flags stay permissive because
// Str() intentionally coerces them and the translator/schema validates shape.
//
// Returns a bare error; the +batch-update translator wraps it with the
// operations[i] (<shortcut>) context.
func (m mapFlagView) validateRawTypes() error {
	if len(m.raw) == 0 {
		return nil
	}
	defs, err := loadFlagDefs()
	if err != nil {
		return nil //nolint:nilerr // fail-open: if flag-defs can't load, skip type validation rather than block the batch
	}
	spec, ok := defs[m.command]
	if !ok {
		return nil
	}
	declaredType := make(map[string]string, len(spec.Flags))
	for _, df := range spec.Flags {
		declaredType[df.Name] = df.Type
	}
	for rawKey, val := range m.raw {
		name := rawKey
		typ, ok := declaredType[name]
		if !ok {
			// flag-defs use hyphen names; tolerate the underscore form users send.
			name = strings.ReplaceAll(rawKey, "_", "-")
			typ, ok = declaredType[name]
		}
		if !ok {
			continue // unknown key — leave it for the translator / schema layer
		}
		switch typ {
		case "int":
			// Int(): float64 → int(t) truncates, so a non-integer number would
			// be silently floored (1.9 → 1). Standalone cobra rejects it at
			// parse time; reject here too to keep batch/standalone parity.
			f, isNum := val.(float64)
			if !isNum {
				return fmt.Errorf("--%s must be a number, got %s", name, jsonTypeName(val))
			}
			if math.Trunc(f) != f {
				return fmt.Errorf("--%s must be an integer, got %s", name, strconv.FormatFloat(f, 'g', -1, 64))
			}
		case "float64":
			if _, isNum := val.(float64); !isNum {
				return fmt.Errorf("--%s must be a number, got %s", name, jsonTypeName(val))
			}
		case "bool":
			if _, isBool := val.(bool); !isBool {
				return fmt.Errorf("--%s must be a boolean, got %s", name, jsonTypeName(val))
			}
		}
	}
	return nil
}

// jsonTypeName names the JSON kind of a value decoded by encoding/json, for
// type-mismatch error messages.
func jsonTypeName(v interface{}) string {
	switch v.(type) {
	case nil:
		return "null"
	case bool:
		return "boolean"
	case float64:
		return "number"
	case string:
		return "string"
	case []interface{}:
		return "array"
	case map[string]interface{}:
		return "object"
	default:
		return fmt.Sprintf("%T", v)
	}
}
