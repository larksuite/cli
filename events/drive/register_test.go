// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"reflect"
	"testing"

	"github.com/larksuite/cli/internal/event/catalog"
)

// keyByName finds a definition in the domain catalog, failing the test when
// the key is absent.
func keyByName(t *testing.T, name string) catalog.KeyDefinition {
	t.Helper()
	for _, def := range Keys() {
		if def.Key == name {
			return def
		}
	}
	t.Fatalf("EventKey %s is not registered", name)
	return catalog.KeyDefinition{}
}

func TestKeysRegistered(t *testing.T) {
	want := []string{
		eventTypeCommentAddV1,
		eventTypeBitableFieldChangedV1,
		eventTypeBitableRecordChangedV1,
	}
	keys := Keys()
	if len(keys) != len(want) {
		t.Fatalf("len(Keys()) = %d, want %d", len(keys), len(want))
	}
	for _, name := range want {
		keyByName(t, name)
	}
}

func TestKeysCommentAddMetadata(t *testing.T) {
	def := keyByName(t, eventTypeCommentAddV1)
	if def.Key != eventTypeCommentAddV1 || def.EventType != eventTypeCommentAddV1 {
		t.Errorf("Key/EventType = %q/%q, want %q", def.Key, def.EventType, eventTypeCommentAddV1)
	}
	if def.Schema.Custom == nil || def.Schema.Custom.Type != reflect.TypeOf(CommentAddOutput{}) {
		t.Fatalf("Schema.Custom = %#v, want CommentAddOutput", def.Schema.Custom)
	}
	if def.Schema.Native != nil {
		t.Fatal("Schema.Native must be nil for processed output")
	}
	if def.Process == nil {
		t.Fatal("Process is nil")
	}
	if def.PreConsume == nil {
		t.Fatal("PreConsume is nil; the comment notice subscription must be registered before consuming")
	}
	if !reflect.DeepEqual(def.Scopes, []string{"docs:document.comment:read"}) {
		t.Errorf("Scopes = %#v", def.Scopes)
	}
	if !reflect.DeepEqual(def.AuthTypes, []string{"user", "bot"}) {
		t.Errorf("AuthTypes = %#v", def.AuthTypes)
	}
	if !reflect.DeepEqual(def.RequiredConsoleEvents, []string{eventTypeCommentAddV1}) {
		t.Errorf("RequiredConsoleEvents = %#v", def.RequiredConsoleEvents)
	}
	if len(def.Params) != 0 {
		t.Errorf("Params = %#v, want none", def.Params)
	}
}

func TestCommentAddRegistersCleanly(t *testing.T) {
	snap, err := catalog.Compile(Keys(), catalog.StrategyRefs{
		catalog.StrategyNone,
		catalog.StrategyLegacyPreConsume,
	})
	if err != nil {
		t.Fatalf("catalog.Compile(Keys()): %v", err)
	}
	if _, ok := snap.Resolve(eventTypeCommentAddV1); !ok {
		t.Fatalf("snap.Resolve(%q): key missing from compiled catalog", eventTypeCommentAddV1)
	}
}

// TestKeysBitableMetadata pins what makes the two Base EventKeys usable: a
// native passthrough of the server payload, a per-file subscription whose
// token is part of the subscription identity, and a Match that keeps one
// consumer's stream to the Base it asked for.
func TestKeysBitableMetadata(t *testing.T) {
	tests := []struct {
		key    string
		native reflect.Type
		scopes []string
	}{
		{
			key:    eventTypeBitableFieldChangedV1,
			native: reflect.TypeOf(DriveFileBitableFieldChangedV1Data{}),
			scopes: []string{"base:field:read"},
		},
		{
			key:    eventTypeBitableRecordChangedV1,
			native: reflect.TypeOf(DriveFileBitableRecordChangedV1Data{}),
			scopes: []string{"base:record:read"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			def := keyByName(t, tt.key)
			if def.EventType != tt.key {
				t.Errorf("EventType = %q, want %q", def.EventType, tt.key)
			}
			if def.Schema.Native == nil || def.Schema.Native.Type != tt.native {
				t.Fatalf("Schema.Native = %#v, want %s", def.Schema.Native, tt.native)
			}
			if def.Schema.Custom != nil {
				t.Error("Schema.Custom must be nil for native output")
			}
			if def.Process != nil {
				t.Error("native output forbids Process")
			}
			if def.PreConsume == nil {
				t.Fatal("PreConsume is nil; the per-file subscription must be registered before consuming")
			}
			if def.Match == nil {
				t.Fatal("Match is nil; without it a consumer sees every subscribed Base")
			}
			if !reflect.DeepEqual(def.Scopes, tt.scopes) {
				t.Errorf("Scopes = %#v, want %#v", def.Scopes, tt.scopes)
			}
			if !reflect.DeepEqual(def.AuthTypes, []string{"user", "bot"}) {
				t.Errorf("AuthTypes = %#v", def.AuthTypes)
			}
			if !reflect.DeepEqual(def.RequiredConsoleEvents, []string{tt.key}) {
				t.Errorf("RequiredConsoleEvents = %#v", def.RequiredConsoleEvents)
			}
			if len(def.Params) != 1 {
				t.Fatalf("Params = %#v, want exactly file_token", def.Params)
			}
			p := def.Params[0]
			if p.Name != "file_token" || !p.Required || !p.SubscriptionKey {
				t.Errorf("param = %#v, want required subscription-key file_token", p)
			}
		})
	}
}
