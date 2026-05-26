// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package minutes

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/larksuite/cli/internal/event"
	"github.com/larksuite/cli/internal/validate"
)

type stubAPIClient struct {
	callFn func(ctx context.Context, method, path string, body any) (json.RawMessage, error)
}

func (s *stubAPIClient) CallAPI(ctx context.Context, method, path string, body any) (json.RawMessage, error) {
	if s.callFn == nil {
		return nil, nil
	}
	return s.callFn(ctx, method, path, body)
}

func assertSubscriptionRequest(t *testing.T, gotBody any, wantEventType string) {
	t.Helper()
	want := map[string]string{"event_type": wantEventType}
	if !reflect.DeepEqual(gotBody, want) {
		t.Fatalf("request body = %#v, want %#v", gotBody, want)
	}
}

func TestMain(m *testing.M) {
	for _, k := range Keys() {
		event.RegisterKey(k)
	}
	os.Exit(m.Run())
}

func TestMinutesKeys_ProcessedMinuteGeneratedRegistered(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	def, ok := event.Lookup(eventTypeMinuteGenerated)
	if !ok {
		t.Fatalf("%s should be registered via Keys()", eventTypeMinuteGenerated)
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
	if len(def.Scopes) != 1 || def.Scopes[0] != "minutes:minutes.basic:read" {
		t.Errorf("Scopes = %v", def.Scopes)
	}
	if len(def.AuthTypes) != 1 || def.AuthTypes[0] != "user" {
		t.Errorf("AuthTypes = %v", def.AuthTypes)
	}
}

func TestProcessMinutesMinuteGenerated(t *testing.T) {
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
					"minute": {
						"token": "<doc_token_001>",
						"title": "产品周会的视频会议",
						"note_id": "7616590025794260496"
					}
				}
			}`), nil
		},
	}

	out := runMinuteGenerated(t, rt, `{
		"schema": "2.0",
		"header": {
			"event_id": "ev_minute_001",
			"event_type": "minutes.minute.generated_v1",
			"create_time": "1608725989000"
		},
		"event": {
			"minute_token": "<doc_token_001>",
			"minute_source": {
				"source_type": "meeting",
				"source_entity_id": "6911188411934433028"
			}
		}
	}`)

	if gotMethod != "GET" {
		t.Errorf("detail method = %q, want GET", gotMethod)
	}
	if gotPath != fmt.Sprintf("/open-apis/minutes/v1/minutes/%s", validate.EncodePathSegment("<doc_token_001>")) {
		t.Errorf("detail path = %q", gotPath)
	}
	if out.Type != eventTypeMinuteGenerated {
		t.Errorf("Type = %q", out.Type)
	}
	if out.EventID != "ev_minute_001" || out.Timestamp != "1608725989000" {
		t.Errorf("EventID/Timestamp = %q/%q", out.EventID, out.Timestamp)
	}
	if out.MinuteToken != "<doc_token_001>" {
		t.Errorf("MinuteToken = %q", out.MinuteToken)
	}
	if out.Title != "产品周会的视频会议" {
		t.Errorf("Title = %q", out.Title)
	}
	if out.MinuteSource == nil {
		t.Fatal("MinuteSource should not be nil")
	}
	if out.MinuteSource.SourceType != "meeting" || out.MinuteSource.SourceEntityID != "6911188411934433028" {
		t.Errorf("MinuteSource = %+v", out.MinuteSource)
	}
}

func TestProcessMinutesMinuteGenerated_DetailFailureFallsBackToBaseFields(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	called := 0
	rt := &stubAPIClient{
		callFn: func(_ context.Context, method, path string, body any) (json.RawMessage, error) {
			called++
			return nil, context.DeadlineExceeded
		},
	}

	out := runMinuteGenerated(t, rt, `{
		"schema": "2.0",
		"header": {
			"event_id": "ev_minute_002",
			"event_type": "minutes.minute.generated_v1",
			"create_time": "1608725989001"
		},
		"event": {
			"minute_token": "<doc_token_004>",
			"minute_source": {
				"source_type": "meeting",
				"source_entity_id": "7641156270787481117"
			}
		}
	}`)

	wantCalls := 1 + minutesDetailMaxRetries
	if called != wantCalls {
		t.Fatalf("detail API called %d times, want %d", called, wantCalls)
	}
	if out.MinuteToken != "<doc_token_004>" {
		t.Errorf("MinuteToken = %q", out.MinuteToken)
	}
	if out.Title != "" {
		t.Errorf("Title = %q, want empty", out.Title)
	}
	if out.MinuteSource == nil {
		t.Fatal("MinuteSource should remain from event payload")
	}
	if out.MinuteSource.SourceType != "meeting" || out.MinuteSource.SourceEntityID != "7641156270787481117" {
		t.Errorf("MinuteSource = %+v", out.MinuteSource)
	}
}

func TestProcessMinutesMinuteGenerated_EmptyTitleRetriesAndSucceeds(t *testing.T) {
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
						"minute": {
							"title": ""
						}
					}
				}`), nil
			}
			return json.RawMessage(`{
				"code": 0,
				"msg": "success",
				"data": {
					"minute": {
						"title": "delayed title"
					}
				}
			}`), nil
		},
	}

	out := runMinuteGenerated(t, rt, `{
		"schema": "2.0",
		"header": {
			"event_id": "ev_minute_retry",
			"event_type": "minutes.minute.generated_v1",
			"create_time": "1608725989000"
		},
		"event": {
			"minute_token": "<doc_token_003>"
		}
	}`)

	if called != 2 {
		t.Fatalf("detail API called %d times, want 2 (1 initial + 1 retry)", called)
	}
	if out.Title != "delayed title" {
		t.Errorf("Title = %q, want delayed title", out.Title)
	}
}

func TestProcessMinutesMinuteGenerated_EmptyTitleExhaustsRetries(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	called := 0
	rt := &stubAPIClient{
		callFn: func(_ context.Context, _, _ string, _ any) (json.RawMessage, error) {
			called++
			return json.RawMessage(`{
				"code": 0,
				"msg": "success",
				"data": {
					"minute": {
						"title": ""
					}
				}
			}`), nil
		},
	}

	out := runMinuteGenerated(t, rt, `{
		"schema": "2.0",
		"header": {
			"event_id": "ev_minute_exhaust",
			"event_type": "minutes.minute.generated_v1",
			"create_time": "1608725989000"
		},
		"event": {
			"minute_token": "<doc_token_002>"
		}
	}`)

	wantCalls := 1 + minutesDetailMaxRetries
	if called != wantCalls {
		t.Fatalf("detail API called %d times, want %d", called, wantCalls)
	}
	if out.Title != "" {
		t.Errorf("Title = %q, want empty after exhausted retries", out.Title)
	}
}

func TestMinutesMinuteGenerated_PreConsumeSubscriptionLifecycle(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	def, ok := event.Lookup(eventTypeMinuteGenerated)
	if !ok {
		t.Fatalf("%s should be registered via Keys()", eventTypeMinuteGenerated)
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
	if calls[0].method != "POST" || calls[0].path != pathMinuteSubscribe {
		t.Fatalf("subscribe call = %+v", calls[0])
	}
	assertSubscriptionRequest(t, calls[0].body, eventTypeMinuteGenerated)

	cleanup()
	if len(calls) != 2 {
		t.Fatalf("calls after cleanup = %d, want 2", len(calls))
	}
	if calls[1].method != "POST" || calls[1].path != pathMinuteUnsubscribe {
		t.Fatalf("unsubscribe call = %+v", calls[1])
	}
	assertSubscriptionRequest(t, calls[1].body, eventTypeMinuteGenerated)
}

func TestProcessMinutesMinuteGenerated_MalformedPayload(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	raw := &event.RawEvent{
		EventType: eventTypeMinuteGenerated,
		Payload:   json.RawMessage(`not json`),
		Timestamp: time.Now(),
	}
	got, err := processMinutesMinuteGenerated(context.Background(), nil, raw, nil)
	if err != nil {
		t.Fatalf("Process should swallow parse errors, got %v", err)
	}
	if string(got) != "not json" {
		t.Errorf("malformed fallback output = %q, want original bytes", string(got))
	}
}

func runMinuteGenerated(t *testing.T, rt event.APIClient, payload string) MinutesMinuteGeneratedOutput {
	t.Helper()
	raw := &event.RawEvent{
		EventType: eventTypeMinuteGenerated,
		Payload:   json.RawMessage(payload),
		Timestamp: time.Now(),
	}
	got, err := processMinutesMinuteGenerated(context.Background(), rt, raw, nil)
	if err != nil {
		t.Fatalf("Process error: %v", err)
	}
	var out MinutesMinuteGeneratedOutput
	if err := json.Unmarshal(got, &out); err != nil {
		t.Fatalf("Process output is not valid MinutesMinuteGeneratedOutput JSON: %v\nraw=%s", err, string(got))
	}
	return out
}
