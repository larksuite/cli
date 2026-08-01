// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package event

import (
	"reflect"
	"strings"
	"testing"
)

// renderedDeclarationFields lists every JSON field the list/schema commands
// are allowed to expose, each with the reason it belongs to the public
// contract. Golden files pin today's bytes; this gate protects tomorrow: a
// field added to the rendered structs (promoted through the embedded
// definition or nested anywhere under it) must either appear here
// deliberately or be tagged `json:"-"`. The set is flat — an entry admits its
// rendered name at any nesting level, which is the same latitude
// encoding/json gives a name.
var renderedDeclarationFields = map[string]string{
	"key":                     "stable identifier agents subscribe by",
	"domain":                  "declared domain override; empty for every shipped key (filtering reads the derived descriptor value), so legacy output is byte-identical",
	"display_name":            "human-readable name for pickers",
	"description":             "what the event means (KeyDefinition) / what the parameter does (ParamDef)",
	"event_type":              "upstream event type behind this key",
	"subscription_type":       "which console ledger the precheck reads",
	"params":                  "declared consume parameters",
	"schema":                  "declared schema source (native/custom markers)",
	"scopes":                  "OAuth scopes required to consume",
	"auth_types":              "identities the key accepts",
	"required_console_events": "console switches that must be enabled",
	"buffer_size":             "delivery buffer size after normalization",
	"workers":                 "worker count after normalization",
	"single_consumer":         "whether a second consumer is rejected",
	"resolved_output_schema":  "fully resolved JSON schema of stdout events",
	"jq_root_path":            "schema command only: jq root for consuming stdout",

	// Nested under params (ParamDef): everything an agent needs to pass the
	// parameter correctly.
	"name":             "parameter name as passed via --param",
	"type":             "parameter value type (string/enum/multi/bool/int)",
	"required":         "whether the parameter must be provided",
	"default":          "value applied when the parameter is omitted",
	"values":           "allowed values for enum/multi parameters",
	"subscription_key": "whether the parameter is part of the subscription identity",

	// Nested under params.values (ParamValue).
	"value": "one allowed parameter value",
	"desc":  "what choosing this value means",

	// Nested under schema (SchemaDef / SchemaSpec): declaration markers only;
	// the resolved schema is the sibling resolved_output_schema.
	"native":          "marker for keys delivering the raw V2 envelope",
	"custom":          "marker for keys delivering processed output",
	"field_overrides": "per-field annotations overriding the reflected schema",
	"raw":             "raw declared schema bytes; empty for reflected types",

	// Nested under schema.field_overrides (schemas.FieldMeta). The type has
	// no json tags, so encoding/json renders the Go field names — pinned
	// as-is because retagging them would change the public bytes.
	"Description": "override for the field's schema description",
	"Enum":        "override for the field's allowed values",
	"Kind":        "override rendered as the field's schema format",
}

// TestRenderContract_NoRuntimeFieldLeaksIntoJSON walks both rendered shapes,
// following embedded struct promotion and recursing into every named type
// reachable through the rendered fields, and fails on any exported member
// that is neither allowlisted nor explicitly excluded from JSON.
func TestRenderContract_NoRuntimeFieldLeaksIntoJSON(t *testing.T) {
	emitted := map[string]bool{}
	for _, typ := range []reflect.Type{
		reflect.TypeFor[listRow](),
		reflect.TypeFor[schemaPayload](),
	} {
		walkRenderedFields(t, typ, emitted, map[reflect.Type]bool{})
	}

	if len(emitted) == 0 {
		t.Fatal("no rendered fields were visited; the gate scanned nothing")
	}
	// The embedded definition is where leaks would hide: prove promotion was
	// actually followed by requiring fields that only exist on it. The nested
	// sentinels prove each recursion path is really taken: subscription_key
	// (slice-of-struct: ParamDef), desc (slice inside a nested struct:
	// ParamValue), raw (pointer-to-struct: SchemaSpec), Enum (map value:
	// FieldMeta, rendered under its Go name because the type is untagged).
	for _, sentinel := range []string{
		"key", "event_type", "resolved_output_schema",
		"subscription_key", "desc", "raw", "Enum",
	} {
		if !emitted[sentinel] {
			t.Fatalf("field %q was not visited; the walker no longer reaches every rendered shape", sentinel)
		}
	}
	for name := range renderedDeclarationFields {
		if !emitted[name] {
			t.Errorf("allowlist entry %q is stale: no rendered struct emits it", name)
		}
	}
}

// walkRenderedFields records every JSON field name typ can render: embedded
// structs promote into the parent object, and any struct reachable through a
// field's type — behind pointers, slice/array elements, or map values — is
// walked in turn, so a field added to a nested type like ParamDef cannot
// escape the gate. visited breaks cycles; a type already recorded in this
// walk contributes nothing new.
func walkRenderedFields(t *testing.T, typ reflect.Type, emitted map[string]bool, visited map[reflect.Type]bool) {
	t.Helper()
	typ = nestedStructType(typ)
	if typ == nil || visited[typ] {
		return
	}
	visited[typ] = true
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		if !field.IsExported() {
			continue
		}
		tag := field.Tag.Get("json")
		if tag == "-" {
			continue
		}
		if field.Anonymous && tag == "" {
			if ft := nestedStructType(field.Type); ft != nil {
				// Embedded struct without a tag: fields promote into the
				// parent JSON object.
				walkRenderedFields(t, ft, emitted, visited)
				continue
			}
		}
		name, _, _ := strings.Cut(tag, ",")
		if name == "" {
			// encoding/json renders an untagged exported field under its Go
			// name (schemas.FieldMeta does this today); the rendered name is
			// what the contract governs, so it is what must be declared.
			name = field.Name
		}
		if _, ok := renderedDeclarationFields[name]; !ok {
			t.Errorf("%s.%s renders JSON field %q that is not in the declared output contract; add it deliberately or exclude it with json:\"-\"", typ.Name(), field.Name, name)
		}
		emitted[name] = true
		walkRenderedFields(t, field.Type, emitted, visited)
	}
}

// nestedStructType unwraps pointers, slice/array elements, and map values
// until it reaches the struct that would render as a JSON object; nil means
// the type renders as a leaf (scalar, string, raw bytes) and holds no fields
// to govern.
func nestedStructType(typ reflect.Type) reflect.Type {
	for {
		switch typ.Kind() {
		case reflect.Pointer, reflect.Slice, reflect.Array, reflect.Map:
			typ = typ.Elem()
		case reflect.Struct:
			return typ
		default:
			return nil
		}
	}
}
