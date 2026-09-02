// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package consume

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	event "github.com/larksuite/cli/internal/event"
	"github.com/larksuite/cli/internal/event/adapter/localbus/protocol"
	"github.com/larksuite/cli/internal/event/model"
	"github.com/larksuite/cli/internal/event/processing"
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

// A Process error is reported on stderr, and error text routinely embeds
// input fragments (a parse error quoting the payload, an API response echo).
// The diagnostic must therefore carry only a bounded prefix of the error, so
// content sitting past the cap never reaches stderr.
func TestProcessErrorDiagnostic_TruncatesLongErrorText(t *testing.T) {
	const sentinel = "PAYLOAD-FRAGMENT-IN-ERROR-zyx987"
	// The sentinel sits entirely beyond the truncation cap.
	longErr := strings.Repeat("x", diagnosticErrMaxLen+50) + sentinel

	// Control: an untruncated diagnostic would contain the sentinel, so the
	// leak assertion below is able to detect a regression.
	if !strings.Contains("WARN: Process error: "+longErr, sentinel) {
		t.Fatal("control failed: the sentinel is not in the raw error; the test cannot prove truncation")
	}

	keyDef := &event.KeyDefinition{
		Key:       "test.evt_process_error",
		EventType: "test.evt_process_error",
		Process: func(context.Context, event.APIClient, *event.RawEvent, map[string]string) (json.RawMessage, error) {
			return nil, errors.New(longErr)
		},
	}
	var stderr bytes.Buffer
	var stdout bytes.Buffer
	opts := Options{ErrOut: &stderr, Params: map[string]string{}}
	sink := &WriterSink{W: &stdout}

	evt := protocol.NewEvent(&model.Event{
		EventID:   "evt-process-err-1",
		EventType: "test.evt_process_error",
		Payload:   json.RawMessage(`{}`),
	}, 1)

	wrote, err := processAndOutput(context.Background(), keyDef, evt, opts, sink, nil)
	if wrote || err != nil {
		t.Fatalf("a Process error must drop the event without a sink error: wrote=%v err=%v", wrote, err)
	}
	if stdout.Len() != 0 {
		t.Errorf("nothing may reach stdout for a dropped event, got: %s", stdout.String())
	}
	diag := stderr.String()
	if !strings.Contains(diag, "WARN: Process error:") {
		t.Errorf("expected a process-error diagnostic, got: %q", diag)
	}
	if !strings.Contains(diag, "...(truncated)") {
		t.Errorf("a long error must be marked as truncated, got: %q", diag)
	}
	assertNoPayloadBytes(t, diag, sentinel)
}

// A short Process error passes through whole — truncation only engages past
// the cap, so ordinary diagnostics stay fully readable.
func TestProcessErrorDiagnostic_KeepsShortErrorIntact(t *testing.T) {
	const shortErr = "decode meeting id: unexpected end of JSON input"
	keyDef := &event.KeyDefinition{
		Key:       "test.evt_process_error_short",
		EventType: "test.evt_process_error_short",
		Process: func(context.Context, event.APIClient, *event.RawEvent, map[string]string) (json.RawMessage, error) {
			return nil, errors.New(shortErr)
		},
	}
	var stderr bytes.Buffer
	opts := Options{ErrOut: &stderr, Params: map[string]string{}}
	sink := &WriterSink{W: &bytes.Buffer{}}

	evt := protocol.NewEvent(&model.Event{
		EventID:   "evt-process-err-2",
		EventType: "test.evt_process_error_short",
		Payload:   json.RawMessage(`{}`),
	}, 1)

	if wrote, err := processAndOutput(context.Background(), keyDef, evt, opts, sink, nil); wrote || err != nil {
		t.Fatalf("a Process error must drop the event without a sink error: wrote=%v err=%v", wrote, err)
	}
	diag := stderr.String()
	if !strings.Contains(diag, "WARN: Process error: "+shortErr) {
		t.Errorf("a short error must be reported verbatim, got: %q", diag)
	}
	if strings.Contains(diag, "...(truncated)") {
		t.Errorf("a short error must not be marked as truncated, got: %q", diag)
	}
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
