// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package event

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	eventlib "github.com/larksuite/cli/internal/event"
	"github.com/larksuite/cli/internal/event/schemas"

	_ "github.com/larksuite/cli/events"
)

func TestRunSchema_ProcessedKey_Text(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, &core.CliConfig{AppID: "test"})

	if err := runSchema(f, "im.message.receive_v1", false); err != nil {
		t.Fatalf("runSchema: %v", err)
	}

	out := stdout.String()
	for _, want := range []string{
		"Key:", "im.message.receive_v1",
		"Event:", "im.message.receive_v1",
		"Output Schema:",
		`"message_id"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("schema output missing %q; got:\n%s", want, out)
		}
	}
}

func TestRunSchema_NativeKey_WrapsEnvelope(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, &core.CliConfig{AppID: "test"})

	if err := runSchema(f, "im.message.message_read_v1", false); err != nil {
		t.Fatalf("runSchema: %v", err)
	}

	out := stdout.String()
	for _, want := range []string{
		"Output Schema:",
		`"schema"`,
		`"header"`,
		`"event"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("native schema output missing %q; got:\n%s", want, out)
		}
	}
}

func TestRunSchema_UnknownKey_SuggestsAlternatives(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{AppID: "test"})

	err := runSchema(f, "im.message.recieve_v1", false)
	if err == nil {
		t.Fatal("expected error for unknown key")
	}
	msg := err.Error()
	if !strings.Contains(msg, "unknown EventKey") {
		t.Errorf("error should mention unknown EventKey: %q", msg)
	}
	if !strings.Contains(msg, "im.message.receive_v1") {
		t.Errorf("error should suggest the real key name (typo correction): %q", msg)
	}
}

func TestRunSchema_JSONOutput(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, &core.CliConfig{AppID: "test"})

	if err := runSchema(f, "im.message.receive_v1", true); err != nil {
		t.Fatalf("runSchema json: %v", err)
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, stdout.String())
	}
	for _, field := range []string{"key", "event_type", "schema", "resolved_output_schema"} {
		if _, ok := payload[field]; !ok {
			t.Errorf("JSON output missing field %q: %+v", field, payload)
		}
	}
	if payload["key"] != "im.message.receive_v1" {
		t.Errorf("key = %v, want im.message.receive_v1", payload["key"])
	}
}

func TestResolveSchemaJSON_CustomWithOverlay(t *testing.T) {
	const syntheticKey = "t.custom.overlay"
	t.Cleanup(func() { eventlib.UnregisterKeyForTest(syntheticKey) })

	type out struct {
		SenderID string `json:"sender_id"`
	}
	eventlib.RegisterKey(eventlib.KeyDefinition{
		Key:       syntheticKey,
		EventType: syntheticKey,
		Schema: eventlib.SchemaDef{
			Custom: &eventlib.SchemaSpec{Type: reflect.TypeOf(out{})},
			FieldOverrides: map[string]schemas.FieldMeta{
				"/sender_id": {Kind: "open_id"},
			},
		},
		Process: func(context.Context, eventlib.APIClient, *eventlib.RawEvent, map[string]string) (json.RawMessage, error) {
			return nil, nil
		},
	})
	def, _ := eventlib.Lookup(syntheticKey)
	resolved, orphans, err := resolveSchemaJSON(def)
	if err != nil || len(orphans) != 0 {
		t.Fatalf("resolve: err=%v orphans=%v", err, orphans)
	}
	var parsed map[string]interface{}
	if err := json.Unmarshal(resolved, &parsed); err != nil {
		t.Fatal(err)
	}
	got := parsed["properties"].(map[string]interface{})["sender_id"].(map[string]interface{})["format"]
	if got != "open_id" {
		t.Errorf("overlay format = %v, want open_id", got)
	}
}
