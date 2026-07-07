// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/larksuite/cli/internal/event"
	"github.com/larksuite/cli/internal/event/consume"
)

type stubAPIClient struct {
	callFn func(ctx context.Context, method, path string, body any) (json.RawMessage, error)
}

func (s *stubAPIClient) CallAPI(ctx context.Context, method, path string, body any) (json.RawMessage, error) {
	if s.callFn == nil {
		return json.RawMessage(`{"code":0,"data":{}}`), nil
	}
	return s.callFn(ctx, method, path, body)
}

func TestMailKeySubscriptionIdentity(t *testing.T) {
	def := Keys()[0]
	idA := consume.ComputeSubscriptionID(&def, map[string]string{
		"mailbox":    "a@example.com",
		"msg_format": "metadata",
		"folder_ids": "INBOX",
	})
	idB := consume.ComputeSubscriptionID(&def, map[string]string{
		"mailbox":    "b@example.com",
		"msg_format": "metadata",
		"folder_ids": "INBOX",
	})
	idAWithDifferentFilters := consume.ComputeSubscriptionID(&def, map[string]string{
		"mailbox":    "a@example.com",
		"msg_format": "full",
		"folder_ids": "SENT",
		"label_ids":  "FLAGGED",
	})
	if idA == idB {
		t.Fatalf("different mailboxes must produce distinct subscription IDs: %q", idA)
	}
	if idA != idAWithDifferentFilters {
		t.Fatalf("folder/label/msg_format filters must not affect subscription ID: %q != %q", idA, idAWithDifferentFilters)
	}
}

func TestNormalizeParamsResolvesMailboxAndFilters(t *testing.T) {
	var calls []string
	rt := &stubAPIClient{callFn: func(_ context.Context, method, path string, body any) (json.RawMessage, error) {
		calls = append(calls, method+" "+path)
		switch path {
		case "/open-apis/mail/v1/user_mailboxes/me/profile":
			return json.RawMessage(`{"code":0,"data":{"user_mailbox":{"primary_email_address":"user@example.com"}}}`), nil
		case "/open-apis/mail/v1/user_mailboxes/user@example.com/folders":
			return json.RawMessage(`{"code":0,"data":{"items":[{"id":"fld_custom","name":"Projects"}]}}`), nil
		default:
			t.Fatalf("unexpected API call: %s %s body=%#v", method, path, body)
			return nil, nil
		}
	}}
	params := map[string]string{
		"mailbox":   "me",
		"folders":   `["Projects"]`,
		"label_ids": `["FLAGGED"]`,
	}
	if err := normalizeMailMessageReceivedParams(context.Background(), rt, params); err != nil {
		t.Fatalf("NormalizeParams error: %v", err)
	}
	if params["mailbox"] != "user@example.com" {
		t.Fatalf("mailbox = %q", params["mailbox"])
	}
	if params["msg_format"] != "metadata" {
		t.Fatalf("msg_format = %q", params["msg_format"])
	}
	if params["folder_ids"] != "fld_custom" {
		t.Fatalf("folder_ids = %q", params["folder_ids"])
	}
	if params["label_ids"] != "FLAGGED" {
		t.Fatalf("label_ids = %q", params["label_ids"])
	}
	if _, ok := params["folders"]; ok {
		t.Fatal("folders param should be removed after normalization")
	}
	if len(calls) != 2 {
		t.Fatalf("API calls = %v", calls)
	}
}

func TestMatchMailMessageReceivedEnvelopeAndFlat(t *testing.T) {
	params := map[string]string{"mailbox": "user@example.com"}
	for _, payload := range []string{
		`{"header":{"event_type":"mail.user_mailbox.event.message_received_v1"},"event":{"mail_address":"USER@example.com","message_id":"msg_1"}}`,
		`{"mail_address":"USER@example.com","message_id":"msg_1"}`,
	} {
		raw := &event.RawEvent{Payload: json.RawMessage(payload)}
		if !matchMailMessageReceived(raw, params) {
			t.Fatalf("payload should match mailbox: %s", payload)
		}
	}
	raw := &event.RawEvent{Payload: json.RawMessage(`{"event":{"mail_address":"other@example.com","message_id":"msg_1"}}`)}
	if matchMailMessageReceived(raw, params) {
		t.Fatal("different mailbox should be dropped")
	}
}

func TestProcessMailMessageReceivedFiltersAndFormats(t *testing.T) {
	var gotPath string
	rt := &stubAPIClient{callFn: func(_ context.Context, method, path string, body any) (json.RawMessage, error) {
		if method != "GET" || body != nil {
			t.Fatalf("message fetch = %s body=%#v", method, body)
		}
		gotPath = path
		return json.RawMessage(`{"code":0,"data":{"message":{"message_id":"msg_1","thread_id":"thr_1","folder_id":"INBOX","label_ids":["FLAGGED"],"internal_date":"1742800000000","message_state":1,"subject":"hello"}}}`), nil
	}}
	raw := &event.RawEvent{EventType: EventTypeMessageReceived, Payload: json.RawMessage(`{"event":{"mail_address":"user@example.com","message_id":"msg_1"}}`)}
	out, err := processMailMessageReceived(context.Background(), rt, raw, map[string]string{
		"mailbox":    "user@example.com",
		"msg_format": "minimal",
		"folder_ids": "INBOX",
		"label_ids":  "FLAGGED",
	})
	if err != nil {
		t.Fatalf("Process error: %v", err)
	}
	if gotPath != "/open-apis/mail/v1/user_mailboxes/user@example.com/messages/msg_1?format=metadata" {
		t.Fatalf("message path = %q", gotPath)
	}
	var decoded MailMessageReceivedOutput
	if err := json.Unmarshal(out, &decoded); err != nil {
		t.Fatalf("output JSON: %v", err)
	}
	want := map[string]interface{}{
		"message_id":    "msg_1",
		"thread_id":     "thr_1",
		"folder_id":     "INBOX",
		"label_ids":     []interface{}{"FLAGGED"},
		"internal_date": "1742800000000",
		"message_state": float64(1),
	}
	if !reflect.DeepEqual(decoded.Message, want) {
		t.Fatalf("minimal message = %#v, want %#v", decoded.Message, want)
	}
}

func TestProcessMailMessageReceivedDropsNonMatchingFilters(t *testing.T) {
	rt := &stubAPIClient{callFn: func(_ context.Context, method, path string, body any) (json.RawMessage, error) {
		return json.RawMessage(`{"code":0,"data":{"message":{"message_id":"msg_1","folder_id":"SENT","label_ids":["OTHER"]}}}`), nil
	}}
	raw := &event.RawEvent{EventType: EventTypeMessageReceived, Payload: json.RawMessage(`{"event":{"mail_address":"user@example.com","message_id":"msg_1"}}`)}
	out, err := processMailMessageReceived(context.Background(), rt, raw, map[string]string{
		"mailbox":    "user@example.com",
		"msg_format": "metadata",
		"folder_ids": "INBOX",
	})
	if err != nil {
		t.Fatalf("Process error: %v", err)
	}
	if out != nil {
		t.Fatalf("nonmatching folder should drop event, got %s", out)
	}
}

func TestProcessMailMessageReceivedEmitsFetchFailure(t *testing.T) {
	rt := &stubAPIClient{callFn: func(_ context.Context, method, path string, body any) (json.RawMessage, error) {
		return nil, errors.New("missing scope")
	}}
	raw := &event.RawEvent{EventType: EventTypeMessageReceived, Payload: json.RawMessage(`{"event":{"mail_address":"user@example.com","message_id":"msg_1"}}`)}
	out, err := processMailMessageReceived(context.Background(), rt, raw, map[string]string{
		"mailbox":    "user@example.com",
		"msg_format": "metadata",
	})
	if err != nil {
		t.Fatalf("Process error: %v", err)
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal(out, &decoded); err != nil {
		t.Fatalf("output JSON: %v", err)
	}
	if decoded["ok"] != false {
		t.Fatalf("ok = %#v, want false", decoded["ok"])
	}
	errObj, ok := decoded["error"].(map[string]interface{})
	if !ok {
		t.Fatalf("error payload = %#v", decoded["error"])
	}
	if errObj["type"] != "fetch_message_failed" {
		t.Fatalf("error.type = %#v", errObj["type"])
	}
	if errObj["message_id"] != "msg_1" {
		t.Fatalf("error.message_id = %#v", errObj["message_id"])
	}
	if errObj["format"] != "metadata" {
		t.Fatalf("error.format = %#v", errObj["format"])
	}
	if errObj["message"] != "missing scope" {
		t.Fatalf("error.message = %#v", errObj["message"])
	}
	eventObj, ok := decoded["event"].(map[string]interface{})
	if !ok || eventObj["message_id"] != "msg_1" {
		t.Fatalf("event payload = %#v", decoded["event"])
	}
}

func TestPreConsumeSubscribeAndCleanupUseMailboxPath(t *testing.T) {
	var calls []string
	var bodies []any
	rt := &stubAPIClient{callFn: func(_ context.Context, method, path string, body any) (json.RawMessage, error) {
		calls = append(calls, method+" "+path)
		bodies = append(bodies, body)
		return json.RawMessage(`{"code":0,"data":{}}`), nil
	}}
	cleanup, err := mailSubscriptionPreConsume(EventTypeMessageReceived)(context.Background(), rt, map[string]string{"mailbox": "user+tag@example.com"})
	if err != nil {
		t.Fatalf("PreConsume error: %v", err)
	}
	if cleanup == nil {
		t.Fatal("cleanup is nil")
	}
	if err := cleanup(); err != nil {
		t.Fatalf("cleanup error: %v", err)
	}
	wantCalls := []string{
		"POST /open-apis/mail/v1/user_mailboxes/user+tag@example.com/event/subscribe",
		"POST /open-apis/mail/v1/user_mailboxes/user+tag@example.com/event/unsubscribe",
	}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("calls = %#v, want %#v", calls, wantCalls)
	}
	for _, body := range bodies {
		if !reflect.DeepEqual(body, map[string]int{"event_type": 1}) {
			t.Fatalf("body = %#v", body)
		}
	}
}
