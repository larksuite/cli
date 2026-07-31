// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package catalog

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/larksuite/cli/internal/event/schemas"
)

// Compile canonicalizes and validates every declaration, resolves each key's
// output schema, and projects the result into an immutable snapshot. It is
// the only way to obtain a Snapshot; invalid declarations produce an error
// and never a live snapshot.
func Compile(defs []KeyDefinition, strategies StrategySet) (*Snapshot, error) {
	var problems []string
	fail := func(format string, args ...any) {
		problems = append(problems, fmt.Sprintf(format, args...))
	}

	entries := make(map[string]*Entry, len(defs))
	keys := make([]string, 0, len(defs))

	for i := range defs {
		def := Canonicalize(defs[i])

		if def.Key == "" {
			fail("declaration %d: Key must not be empty", i)
			continue
		}
		if _, dup := entries[def.Key]; dup {
			fail("duplicate EventKey: %s", def.Key)
			continue
		}
		if errs := validateDefinition(&def); len(errs) > 0 {
			problems = append(problems, errs...)
			continue
		}

		schemaJSON, orphans, err := resolveSchemaJSON(&def)
		if err != nil {
			fail("EventKey %s: resolve output schema: %v", def.Key, err)
			continue
		}
		if len(orphans) > 0 {
			fail("EventKey %s: field overrides point at paths the schema does not have: %s",
				def.Key, strings.Join(orphans, ", "))
			continue
		}
		if isPlaceholderSchema(schemaJSON) {
			fail("EventKey %s: declared schema resolves to an empty placeholder; declare the real output shape", def.Key)
			continue
		}

		preparation := StrategyNone
		if def.PreConsume != nil {
			preparation = StrategyLegacyPreConsume
		}
		if strategies == nil || !strategies.Has(preparation) {
			fail("EventKey %s: preparation strategy %q is not provided by the strategy set", def.Key, preparation)
			continue
		}

		mode := OutputProcessed
		if def.Schema.Native != nil {
			mode = OutputNative
		}
		entries[def.Key] = &Entry{
			descriptor: Descriptor{
				Key:                   def.Key,
				Domain:                DerivedDomain(&def),
				DisplayName:           def.DisplayName,
				Description:           def.Description,
				EventType:             def.EventType,
				SubscriptionType:      def.SubscriptionType,
				Params:                cloneParams(def.Params),
				Scopes:                append([]string(nil), def.Scopes...),
				AuthTypes:             append([]string(nil), def.AuthTypes...),
				RequiredConsoleEvents: append([]string(nil), def.RequiredConsoleEvents...),
			},
			output: OutputContract{
				Mode:       mode,
				SchemaJSON: schemaJSON,
				JQRootPath: jqRootPath(mode),
			},
			capability: Capability{
				Preparation:    preparation,
				BufferSize:     def.BufferSize,
				Workers:        def.Workers,
				SingleConsumer: def.SingleConsumer,
			},
			binding: RuntimeBinding{
				NormalizeParams: def.NormalizeParams,
				Match:           def.Match,
				Process:         def.Process,
				PreConsume:      def.PreConsume,
			},
			canonical: def,
		}
		keys = append(keys, def.Key)
	}

	if len(problems) > 0 {
		return nil, errors.New("event catalog rejected:\n  " + strings.Join(problems, "\n  "))
	}

	sort.Strings(keys)
	return &Snapshot{entries: entries, keys: keys}, nil
}

// jqRootPath states where consumers address output fields from: native keys
// deliver the V2 envelope (fields under .event), processed keys deliver the
// processor's flat shape (fields at the root).
func jqRootPath(mode OutputMode) string {
	if mode == OutputNative {
		return ".event"
	}
	return "."
}

// validateDefinition runs the per-declaration contract checks. It reports
// every violation instead of stopping at the first.
func validateDefinition(def *KeyDefinition) []string {
	var out []string
	fail := func(format string, args ...any) {
		out = append(out, fmt.Sprintf(format, args...))
	}

	if def.EventType == "" {
		fail("EventKey %s: EventType must not be empty", def.Key)
	}
	if def.SubscriptionType != SubTypeEvent && def.SubscriptionType != SubTypeCallback {
		fail("EventKey %s: SubscriptionType must be %q or %q; got %q",
			def.Key, SubTypeEvent, SubTypeCallback, def.SubscriptionType)
	}
	if def.Domain != "" && def.Domain != DerivedDomain(&KeyDefinition{Key: def.Key}) {
		fail("EventKey %s: explicit Domain %q does not match the key's first segment", def.Key, def.Domain)
	}

	nativeSet := def.Schema.Native != nil
	customSet := def.Schema.Custom != nil
	switch {
	case nativeSet && customSet:
		fail("EventKey %s: Schema.Native and Schema.Custom are mutually exclusive", def.Key)
	case !nativeSet && !customSet:
		fail("EventKey %s: Schema requires either Native or Custom", def.Key)
	}
	if nativeSet && def.Process != nil {
		fail("EventKey %s: Schema.Native forbids Process (Process produces a complete shape — use Schema.Custom)", def.Key)
	}
	if spec := def.Schema.Native; spec != nil {
		out = append(out, validateSpec(def.Key, "Schema.Native", spec)...)
	}
	if spec := def.Schema.Custom; spec != nil {
		out = append(out, validateSpec(def.Key, "Schema.Custom", spec)...)
	}

	for _, p := range def.Params {
		switch p.Type {
		case "", ParamString, ParamBool, ParamInt:
		case ParamEnum, ParamMulti:
			if len(p.Values) == 0 {
				fail("EventKey %s: param %q type %q requires Values", def.Key, p.Name, p.Type)
			}
			for _, v := range p.Values {
				if v.Desc == "" {
					fail("EventKey %s: param %q value %q requires non-empty Desc", def.Key, p.Name, v.Value)
				}
			}
		default:
			fail("EventKey %s: param %q has unknown type %q", def.Key, p.Name, p.Type)
		}
	}

	for _, t := range def.AuthTypes {
		if t != "user" && t != "bot" {
			fail("EventKey %s: AuthTypes elements must be \"user\" or \"bot\"; got %q", def.Key, t)
		}
	}
	return out
}

func validateSpec(key, field string, s *SchemaSpec) []string {
	typeSet := s.Type != nil
	rawSet := len(s.Raw) > 0
	if typeSet == rawSet {
		return []string{fmt.Sprintf("EventKey %s: %s requires exactly one of Type or Raw", key, field)}
	}
	return nil
}

// resolveSchemaJSON returns the final JSON Schema for a declaration
// (reflected base, V2-wrapped for native keys, field overlay applied);
// orphans lists override pointers that resolved to nothing.
func resolveSchemaJSON(def *KeyDefinition) (json.RawMessage, []string, error) {
	spec, isNative := pickSpec(def.Schema)
	if spec == nil {
		return nil, nil, nil
	}

	base, err := renderSpec(spec)
	if err != nil {
		return nil, nil, err
	}
	if base == nil {
		return nil, nil, nil
	}

	if isNative {
		base = schemas.WrapV2Envelope(base)
	}

	if len(def.Schema.FieldOverrides) > 0 {
		var parsed map[string]any
		if err := json.Unmarshal(base, &parsed); err != nil {
			return nil, nil, fmt.Errorf("parse base schema for field overrides: %w", err)
		}
		orphans := schemas.ApplyFieldOverrides(parsed, def.Schema.FieldOverrides)
		out, err := json.Marshal(parsed)
		if err != nil {
			return nil, nil, fmt.Errorf("serialize schema with field overrides: %w", err)
		}
		return out, orphans, nil
	}

	return base, nil, nil
}

// pickSpec returns the non-nil spec and whether it is native (V2-wrapped).
func pickSpec(s SchemaDef) (*SchemaSpec, bool) {
	if s.Native != nil {
		return s.Native, true
	}
	if s.Custom != nil {
		return s.Custom, false
	}
	return nil, false
}

// renderSpec produces a JSON Schema from Type (reflected) or Raw (copied).
func renderSpec(s *SchemaSpec) (json.RawMessage, error) {
	if s.Type != nil {
		return schemas.FromType(s.Type), nil
	}
	if len(s.Raw) > 0 {
		buf := make(json.RawMessage, len(s.Raw))
		copy(buf, s.Raw)
		return buf, nil
	}
	return nil, errors.New("schema spec has neither Type nor Raw")
}

// isPlaceholderSchema rejects declarations whose schema decodes but describes
// nothing: an empty document, empty object, or null. Per-declaration checks
// only see "raw bytes are non-empty" — this closes that gap.
func isPlaceholderSchema(schema json.RawMessage) bool {
	trimmed := bytes.TrimSpace(schema)
	if len(trimmed) == 0 {
		return true
	}
	if bytes.Equal(trimmed, []byte("null")) || bytes.Equal(trimmed, []byte("{}")) {
		return true
	}
	var asMap map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &asMap); err == nil && len(asMap) == 0 {
		return true
	}
	return false
}
