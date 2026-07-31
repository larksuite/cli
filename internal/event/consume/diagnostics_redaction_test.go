// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package consume

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	event "github.com/larksuite/cli/internal/event"
	"github.com/larksuite/cli/internal/event/model"
	"github.com/larksuite/cli/internal/event/processing"
	"github.com/larksuite/cli/internal/event/adapter/localbus/protocol"
)

// assertNoPayloadBytes fails when a diagnostic line carries payload content.
// Diagnostics may name identity facts (event id, type, field names) but the
// payload itself must never reach stderr — it can hold user content, tokens,
// or anything else the upstream put there.
func assertNoPayloadBytes(t *testing.T, diagnostic, sentinel string) {
	t.Helper()
	if strings.Contains(diagnostic, sentinel) {
		t.Errorf("diagnostic leaks payload content: %s", diagnostic)
	}
}

// The detector itself must bite: a deliberately leaky diagnostic has to be
// caught, otherwise a green redaction suite proves nothing.
func TestRedactionDetector_CatchesALeak(t *testing.T) {
	const sentinel = "SENSITIVE-VALUE-123"
	leaky := "WARN: dropped event, payload was: {\"token\":\"" + sentinel + "\"}"
	if !strings.Contains(leaky, sentinel) {
		t.Fatal("control diagnostic lost its sentinel; the detector cannot be trusted")
	}
}

// A malformed payload is dropped through the real pipeline with a diagnostic
// that names the event, not its content.
func TestMalformedDrop_DiagnosticNamesIdentityOnly(t *testing.T) {
	const sentinel = "API-KEY-LIKE-CONTENT-abcdef123456"
	keyDef := &event.KeyDefinition{
		Key:       "test.evt_redaction",
		EventType: "test.evt_redaction",
		Process: func(_ context.Context, _ event.APIClient, raw *event.RawEvent, _ map[string]string) (json.RawMessage, error) {
			return nil, processing.DropMalformed(raw.EventType)
		},
	}
	var stderr bytes.Buffer
	var stdout bytes.Buffer
	opts := Options{ErrOut: &stderr, Params: map[string]string{}}
	sink := &WriterSink{W: &stdout}

	evt := protocol.NewEvent(&model.Event{
		EventID:   "evt-redact-1",
		EventType: "test.evt_redaction",
		Payload:   json.RawMessage(`{"garbage": "` + sentinel + `"`),
	}, 1)

	wrote, err := processAndOutput(context.Background(), keyDef, evt, opts, sink, nil)
	if wrote || err != nil {
		t.Fatalf("malformed event must be dropped silently from stdout: wrote=%v err=%v", wrote, err)
	}
	if stdout.Len() != 0 {
		t.Errorf("nothing may reach stdout for a dropped event, got: %s", stdout.String())
	}
	diag := stderr.String()
	if !strings.Contains(diag, "dropped: malformed payload") {
		t.Errorf("expected a malformed-drop diagnostic, got: %q", diag)
	}
	if !strings.Contains(diag, "evt-redact-1") {
		t.Errorf("diagnostic must anchor on the event id, got: %q", diag)
	}
	assertNoPayloadBytes(t, diag, sentinel)
}

// A metadata conflict is dropped through the real pipeline the same way.
func TestConflictDrop_DiagnosticNamesIdentityOnly(t *testing.T) {
	const sentinel = "PRIVATE-MESSAGE-TEXT-qwerty"
	keyDef := &event.KeyDefinition{
		Key:       "test.evt_conflict_redaction",
		EventType: "test.evt_conflict_redaction",
	}
	var stderr bytes.Buffer
	var stdout bytes.Buffer
	opts := Options{ErrOut: &stderr, Params: map[string]string{}}
	sink := &WriterSink{W: &stdout}

	payload, _ := json.Marshal(map[string]any{
		"header": map[string]string{"event_id": "evt-forged"},
		"event":  map[string]string{"text": sentinel},
	})
	evt := protocol.NewEvent(&model.Event{
		EventID:   "evt-real",
		EventType: "test.evt_conflict_redaction",
		Payload:   payload,
	}, 1)

	wrote, err := processAndOutput(context.Background(), keyDef, evt, opts, sink, nil)
	if wrote || err != nil {
		t.Fatalf("conflicting event must be dropped: wrote=%v err=%v", wrote, err)
	}
	if stdout.Len() != 0 {
		t.Errorf("nothing may reach stdout for a dropped event, got: %s", stdout.String())
	}
	diag := stderr.String()
	if !strings.Contains(diag, "conflicts with canonical metadata (field=event_id)") {
		t.Errorf("expected a conflict diagnostic naming the field, got: %q", diag)
	}
	assertNoPayloadBytes(t, diag, sentinel)
}
