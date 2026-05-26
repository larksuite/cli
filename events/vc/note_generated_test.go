// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package vc

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/larksuite/cli/internal/event"
)

func TestVCKeys_ProcessedNoteGeneratedRegistered(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	def, ok := event.Lookup(eventTypeNoteGenerated)
	if !ok {
		t.Fatalf("%s should be registered via Keys()", eventTypeNoteGenerated)
	}
	if def.Schema.Custom == nil {
		t.Error("Processed key must set Schema.Custom")
	}
	if def.Schema.Native != nil {
		t.Error("Processed key must not set Schema.Native")
	}
	if def.Process == nil {
		t.Error("Process must not be nil for processed key")
	}
	if def.PreConsume == nil {
		t.Error("PreConsume must not be nil for processed key")
	}
	if len(def.Scopes) != 1 || def.Scopes[0] != "vc:note:read" {
		t.Errorf("Scopes = %v", def.Scopes)
	}
	if len(def.AuthTypes) != 1 || def.AuthTypes[0] != "user" {
		t.Errorf("AuthTypes = %v", def.AuthTypes)
	}
}

func TestProcessVCNoteGenerated(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	var gotMethod, gotPath string
	rt := &stubAPIClient{
		callFn: func(_ context.Context, method, path string, body any) (json.RawMessage, error) {
			gotMethod = method
			gotPath = path
			if body != nil {
				t.Fatalf("GET detail body = %#v, want nil", body)
			}
			return json.RawMessage(`{
				"code": 0,
				"msg": "success",
				"data": {
					"note": {
						"artifacts": [
							{"artifact_type": 1, "doc_token": "note_doc_token"},
							{"artifact_type": 2, "doc_token": "verbatim_doc_token"}
						],
						"note_source": {
							"source_type": "meeting",
							"source_entity_id": "6911188411934433028"
						}
					}
				}
			}`), nil
		},
	}

	out := runNoteGenerated(t, rt, `{
		"schema": "2.0",
		"header": {
			"event_id": "ev_vc_note_001",
			"event_type": "vc.note.generated_v1",
			"create_time": "1608725989000"
		},
		"event": {
			"note_id": "6943848821689040898"
		}
	}`)

	if gotMethod != "GET" {
		t.Errorf("detail method = %q, want GET", gotMethod)
	}
	if gotPath != "/open-apis/vc/v1/notes/6943848821689040898" {
		t.Errorf("detail path = %q", gotPath)
	}
	if out.Type != eventTypeNoteGenerated {
		t.Errorf("Type = %q", out.Type)
	}
	if out.EventID != "ev_vc_note_001" || out.Timestamp != "1608725989000" {
		t.Errorf("EventID/Timestamp = %q/%q", out.EventID, out.Timestamp)
	}
	if out.NoteID != "6943848821689040898" {
		t.Errorf("NoteID = %q", out.NoteID)
	}
	if out.NoteToken != "note_doc_token" {
		t.Errorf("NoteToken = %q", out.NoteToken)
	}
	if out.VerbatimToken != "verbatim_doc_token" {
		t.Errorf("VerbatimToken = %q", out.VerbatimToken)
	}
	if out.NoteSource == nil {
		t.Fatal("NoteSource should not be nil")
	}
	if out.NoteSource.SourceType != "meeting" || out.NoteSource.SourceEntityID != "6911188411934433028" {
		t.Errorf("NoteSource = %+v", out.NoteSource)
	}
}

func TestVCNoteGenerated_PreConsumeSubscriptionLifecycle(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	def, ok := event.Lookup(eventTypeNoteGenerated)
	if !ok {
		t.Fatalf("%s should be registered via Keys()", eventTypeNoteGenerated)
	}

	type call struct {
		method string
		path   string
		body   any
	}
	var calls []call
	rt := &stubAPIClient{
		callFn: func(_ context.Context, method, path string, body any) (json.RawMessage, error) {
			calls = append(calls, call{method: method, path: path, body: body})
			return json.RawMessage(`{"code":0,"msg":"success","data":{}}`), nil
		},
	}

	cleanup, err := def.PreConsume(context.Background(), rt, nil)
	if err != nil {
		t.Fatalf("PreConsume error: %v", err)
	}
	if cleanup == nil {
		t.Fatal("cleanup must not be nil")
	}
	if len(calls) != 1 {
		t.Fatalf("calls after subscribe = %d, want 1", len(calls))
	}
	if calls[0].method != "POST" || calls[0].path != pathNoteSubscribe {
		t.Fatalf("subscribe call = %+v", calls[0])
	}
	assertSubscriptionRequest(t, calls[0].body, eventTypeNoteGenerated)

	cleanup()
	if len(calls) != 2 {
		t.Fatalf("calls after cleanup = %d, want 2", len(calls))
	}
	if calls[1].method != "POST" || calls[1].path != pathNoteUnsubscribe {
		t.Fatalf("unsubscribe call = %+v", calls[1])
	}
	assertSubscriptionRequest(t, calls[1].body, eventTypeNoteGenerated)
}

func TestProcessVCNoteGenerated_DetailFailureFallsBackToBaseFields(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	called := 0
	rt := &stubAPIClient{
		callFn: func(_ context.Context, method, path string, body any) (json.RawMessage, error) {
			called++
			return nil, context.DeadlineExceeded
		},
	}

	out := runNoteGenerated(t, rt, `{
		"schema": "2.0",
		"header": {
			"event_id": "ev_vc_note_002",
			"event_type": "vc.note.generated_v1",
			"create_time": "1608725989001"
		},
		"event": {
			"note_id": "6943848821689040999"
		}
	}`)

	if called != 1 {
		t.Fatalf("detail API called %d times, want 1", called)
	}
	if out.NoteID != "6943848821689040999" {
		t.Errorf("NoteID = %q", out.NoteID)
	}
	if out.NoteToken != "" || out.VerbatimToken != "" {
		t.Errorf("NoteToken/VerbatimToken = %q/%q, want empty", out.NoteToken, out.VerbatimToken)
	}
	if out.NoteSource != nil {
		t.Errorf("NoteSource = %+v, want nil", out.NoteSource)
	}
}

func TestProcessVCNoteGenerated_EmptyTokensRetriesAndSucceeds(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	called := 0
	rt := &stubAPIClient{
		callFn: func(_ context.Context, _, _ string, _ any) (json.RawMessage, error) {
			called++
			if called <= 1 {
				return json.RawMessage(`{
					"code": 0,
					"msg": "success",
					"data": {
						"note": {
							"artifacts": [],
							"note_source": {"source_type": "meeting", "source_entity_id": "123"}
						}
					}
				}`), nil
			}
			return json.RawMessage(`{
				"code": 0,
				"msg": "success",
				"data": {
					"note": {
						"artifacts": [
							{"artifact_type": 1, "doc_token": "delayed_note_token"},
							{"artifact_type": 2, "doc_token": "delayed_verbatim_token"}
						],
						"note_source": {"source_type": "meeting", "source_entity_id": "123"}
					}
				}
			}`), nil
		},
	}

	out := runNoteGenerated(t, rt, `{
		"schema": "2.0",
		"header": {
			"event_id": "ev_vc_note_empty_retry",
			"event_type": "vc.note.generated_v1",
			"create_time": "1608725989000"
		},
		"event": {
			"note_id": "6943848821689040empty"
		}
	}`)

	if called != 2 {
		t.Fatalf("detail API called %d times, want 2 (1 initial + 1 retry)", called)
	}
	if out.NoteToken != "delayed_note_token" {
		t.Errorf("NoteToken = %q, want delayed_note_token", out.NoteToken)
	}
	if out.VerbatimToken != "delayed_verbatim_token" {
		t.Errorf("VerbatimToken = %q, want delayed_verbatim_token", out.VerbatimToken)
	}
}

func TestProcessVCNoteGenerated_EmptyTokensExhaustsRetries(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	called := 0
	rt := &stubAPIClient{
		callFn: func(_ context.Context, _, _ string, _ any) (json.RawMessage, error) {
			called++
			return json.RawMessage(`{
				"code": 0,
				"msg": "success",
				"data": {
					"note": {
						"artifacts": [],
						"note_source": {"source_type": "meeting", "source_entity_id": "123"}
					}
				}
			}`), nil
		},
	}

	out := runNoteGenerated(t, rt, `{
		"schema": "2.0",
		"header": {
			"event_id": "ev_vc_note_empty_exhaust",
			"event_type": "vc.note.generated_v1",
			"create_time": "1608725989000"
		},
		"event": {
			"note_id": "6943848821689040emptyex"
		}
	}`)

	wantCalls := 1 + vcNoteDetailMaxRetries
	if called != wantCalls {
		t.Fatalf("detail API called %d times, want %d", called, wantCalls)
	}
	if out.NoteToken != "" || out.VerbatimToken != "" {
		t.Errorf("NoteToken/VerbatimToken = %q/%q, want empty after exhausted retries", out.NoteToken, out.VerbatimToken)
	}
}

func TestProcessVCNoteGenerated_MalformedPayload(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	raw := &event.RawEvent{
		EventType: eventTypeNoteGenerated,
		Payload:   json.RawMessage(`not json`),
		Timestamp: time.Now(),
	}
	got, err := processVCNoteGenerated(context.Background(), nil, raw, nil)
	if err != nil {
		t.Fatalf("Process should swallow parse errors, got %v", err)
	}
	if string(got) != "not json" {
		t.Errorf("malformed fallback output = %q, want original bytes", string(got))
	}
}

func runNoteGenerated(t *testing.T, rt event.APIClient, payload string) VCNoteGeneratedOutput {
	t.Helper()
	raw := &event.RawEvent{
		EventType: eventTypeNoteGenerated,
		Payload:   json.RawMessage(payload),
		Timestamp: time.Now(),
	}
	got, err := processVCNoteGenerated(context.Background(), rt, raw, nil)
	if err != nil {
		t.Fatalf("Process error: %v", err)
	}
	var out VCNoteGeneratedOutput
	if err := json.Unmarshal(got, &out); err != nil {
		t.Fatalf("Process output is not valid VCNoteGeneratedOutput JSON: %v\nraw=%s", err, string(got))
	}
	return out
}
