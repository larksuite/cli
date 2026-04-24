// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package events

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/larksuite/cli/internal/event"
	"github.com/larksuite/cli/internal/event/schemas"
)

// TestAllKeys_FieldOverridePointersResolve guarantees every FieldOverrides
// path on every registered EventKey resolves to at least one node in the
// rendered schema. Orphan paths typically mean a typo or an SDK field
// rename — failing the build on orphans prevents silent schema drift.
//
// The package-level init() in register.go has already registered all
// domain keys by the time tests run, so we just iterate event.ListAll().
func TestAllKeys_FieldOverridePointersResolve(t *testing.T) {
	for _, def := range event.ListAll() {
		if len(def.Schema.FieldOverrides) == 0 {
			continue
		}
		raw := renderDefSchemaForLint(t, def)
		if raw == nil {
			t.Errorf("%s: FieldOverrides set but Schema has no Native/Custom spec", def.Key)
			continue
		}
		var parsed map[string]interface{}
		if err := json.Unmarshal(raw, &parsed); err != nil {
			t.Errorf("%s: parse schema: %v", def.Key, err)
			continue
		}
		orphans := schemas.ApplyFieldOverrides(parsed, def.Schema.FieldOverrides)
		if len(orphans) > 0 {
			t.Errorf("%s: orphan FieldOverrides paths (typo or SDK drift): %v", def.Key, orphans)
		}
	}
}

// renderDefSchemaForLint mirrors cmd/events/schema.go's resolve pipeline
// enough to produce the schema overrides are applied against. Kept inline
// here so the lint test has no dependency on cmd/events (which would
// create a package cycle).
func renderDefSchemaForLint(t *testing.T, def *event.KeyDefinition) json.RawMessage {
	t.Helper()
	spec, isNative := pickSpec(def.Schema)
	if spec == nil {
		return nil
	}
	raw := renderSpec(t, spec)
	if raw == nil {
		return nil
	}
	if isNative {
		raw = schemas.WrapV2Envelope(raw)
	}
	return raw
}

func pickSpec(s event.SchemaDef) (*event.SchemaSpec, bool) {
	if s.Native != nil {
		return s.Native, true
	}
	if s.Custom != nil {
		return s.Custom, false
	}
	return nil, false
}

// renderSpec never gets the "neither set" branch in production (validateSpec
// panics at RegisterKey time). Silent nil here is fine for the lint path.
func renderSpec(t *testing.T, s *event.SchemaSpec) json.RawMessage {
	t.Helper()
	if s.Type != nil {
		return schemas.FromType(s.Type)
	}
	if len(s.Raw) > 0 {
		return append(json.RawMessage{}, s.Raw...)
	}
	return nil
}

// TestOrphanDetectionMechanism proves the pipeline itself catches orphan
// paths — without this, TestAllKeys_FieldOverridePointersResolve would
// pass vacuously whenever no registered key uses FieldOverrides (which is
// the current state; both IM and mail rely on struct tags). The synthetic
// key below has one valid path and one deliberately-broken path; the
// valid one must resolve and the broken one must be reported.
func TestOrphanDetectionMechanism(t *testing.T) {
	type synthetic struct {
		ValidField string `json:"valid_field"`
	}
	spec := &event.SchemaSpec{Type: reflect.TypeOf(synthetic{})}
	raw := renderSpec(t, spec)
	if raw == nil {
		t.Fatal("renderSpec returned nil for synthetic type")
	}
	var parsed map[string]interface{}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	overrides := map[string]schemas.FieldMeta{
		"/valid_field":   {Kind: "open_id"},
		"/broken_typo":   {Kind: "chat_id"},
		"/valid_field/x": {Kind: "email"}, // dives past a scalar
	}
	orphans := schemas.ApplyFieldOverrides(parsed, overrides)
	wantOrphans := map[string]bool{"/broken_typo": true, "/valid_field/x": true}
	if len(orphans) != len(wantOrphans) {
		t.Fatalf("orphans = %v, want exactly %v", orphans, wantOrphans)
	}
	for _, o := range orphans {
		if !wantOrphans[o] {
			t.Errorf("unexpected orphan %q", o)
		}
	}
	// Confirm the valid path got applied.
	vf := parsed["properties"].(map[string]interface{})["valid_field"].(map[string]interface{})
	if vf["format"] != "open_id" {
		t.Errorf("valid path not applied: %v", vf)
	}
}
