// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"reflect"
	"testing"

	"github.com/larksuite/cli/internal/event/catalog"
)

func TestKeysCommentAddMetadata(t *testing.T) {
	keys := Keys()
	if len(keys) != 1 {
		t.Fatalf("len(Keys()) = %d, want 1", len(keys))
	}

	def := keys[0]
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
