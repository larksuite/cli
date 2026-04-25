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

// Proves the pipeline catches orphan FieldOverrides paths, so TestAllKeys_FieldOverridePointersResolve isn't vacuous.
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
		"/valid_field/x": {Kind: "email"},
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
	vf := parsed["properties"].(map[string]interface{})["valid_field"].(map[string]interface{})
	if vf["format"] != "open_id" {
		t.Errorf("valid path not applied: %v", vf)
	}
}
