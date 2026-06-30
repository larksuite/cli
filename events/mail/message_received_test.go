// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/event"
)

type call struct {
	method string
	path   string
	body   interface{}
}

type stubClient struct {
	responses map[string]json.RawMessage
	calls     []call
}

func (s *stubClient) CallAPI(_ context.Context, method, path string, body interface{}) (json.RawMessage, error) {
	s.calls = append(s.calls, call{method: method, path: path, body: body})
	if s.responses != nil {
		if raw := s.responses[method+" "+path]; raw != nil {
			return raw, nil
		}
	}
	return json.RawMessage(`{"code":0,"data":{}}`), nil
}

func TestKeysContainsMailEventKey(t *testing.T) {
	keys := Keys()
	if len(keys) != 1 {
		t.Fatalf("len(Keys()) = %d, want 1", len(keys))
	}
	def := keys[0]
	if def.Key != MessageReceivedEventKey || def.EventType != MessageReceivedEventKey {
		t.Fatalf("unexpected key definition: %#v", def)
	}
	subscriptionParams := 0
	for _, p := range def.Params {
		if p.SubscriptionKey {
			subscriptionParams++
			if p.Name != "mailbox" {
				t.Fatalf("SubscriptionKey param = %q, want mailbox", p.Name)
			}
		}
	}
	if subscriptionParams != 1 {
		t.Fatalf("SubscriptionKey count = %d, want 1", subscriptionParams)
	}
}

func TestNormalizeParamsResolvesMailboxAndFilterNames(t *testing.T) {
	rt := &stubClient{responses: map[string]json.RawMessage{
		"GET " + mailboxPath("me", "profile"):                json.RawMessage(`{"code":0,"data":{"user_mailbox":{"primary_email_address":"alice@example.com"}}}`),
		"GET " + mailboxPath("alice@example.com", "folders"): json.RawMessage(`{"code":0,"data":{"items":[{"id":"fld_news","name":"News"}]}}`),
		"GET " + mailboxPath("alice@example.com", "labels"):  json.RawMessage(`{"code":0,"data":{"items":[{"id":"lbl_team","name":"Team"}]}}`),
	}}
	params := map[string]string{
		"mailbox":    "me",
		"folder_ids": `["INBOX"]`,
		"folders":    `["News","archive"]`,
		"label_ids":  `["custom-label"]`,
		"labels":     `["Team","important"]`,
	}

	if err := normalizeParams(context.Background(), rt, params); err != nil {
		t.Fatalf("normalizeParams failed: %v", err)
	}
	if params["mailbox"] != "alice@example.com" {
		t.Fatalf("mailbox = %q, want resolved address", params["mailbox"])
	}
	assertJSONArrayValues(t, params["folder_ids"], []string{"ARCHIVED", "INBOX", "fld_news"})
	assertJSONArrayValues(t, params["label_ids"], []string{"IMPORTANT", "custom-label", "lbl_team"})
}

func TestPreConsumeSubscribesAndCleanupUnsubscribes(t *testing.T) {
	rt := &stubClient{}
	cleanup, err := preConsume(context.Background(), rt, map[string]string{"mailbox": "alice@example.com"})
	if err != nil {
		t.Fatalf("preConsume failed: %v", err)
	}
	if len(rt.calls) != 1 || rt.calls[0].method != "POST" || rt.calls[0].path != mailboxPath("alice@example.com", "event", "subscribe") {
		t.Fatalf("unexpected subscribe call: %#v", rt.calls)
	}
	if err := cleanup(); err != nil {
		t.Fatalf("cleanup failed: %v", err)
	}
	if len(rt.calls) != 2 || rt.calls[1].path != mailboxPath("alice@example.com", "event", "unsubscribe") {
		t.Fatalf("unexpected cleanup calls: %#v", rt.calls)
	}
}

func TestMatchMailboxFiltersRawEventMailbox(t *testing.T) {
	raw := rawMailEvent(`{"event":{"mail_address":"alice@example.com","message_id":"msg_1"}}`)
	if !matchMailbox(raw, map[string]string{"mailbox": "ALICE@example.com"}) {
		t.Fatal("expected case-insensitive mailbox match")
	}
	if matchMailbox(raw, map[string]string{"mailbox": "bob@example.com"}) {
		t.Fatal("expected mailbox mismatch to drop event")
	}
}

func TestProcessEventFormatDoesNotFetchWithoutFilters(t *testing.T) {
	rt := &stubClient{}
	raw := rawMailEvent(`{"event":{"mail_address":"alice@example.com","message_id":"msg_1"}}`)
	got, err := processMessageReceived(context.Background(), rt, raw, map[string]string{"mailbox": "alice@example.com", "msg_format": "event"})
	if err != nil {
		t.Fatalf("process failed: %v", err)
	}
	if len(rt.calls) != 0 {
		t.Fatalf("unexpected API calls: %#v", rt.calls)
	}
	if !strings.Contains(string(got), `"message_id":"msg_1"`) {
		t.Fatalf("raw event not returned: %s", got)
	}
}

func TestProcessOutputDirForcesFullFetchEvenForEventFormat(t *testing.T) {
	rt := &stubClient{responses: map[string]json.RawMessage{
		"GET " + mailboxPath("alice@example.com", "messages", "msg_1") + "?format=full": json.RawMessage(`{"code":0,"data":{"message":{"message_id":"msg_1","subject":"Hello","body_html":"PGgxPkhlbGxvPC9oMT4="}}}`),
	}}
	raw := rawMailEvent(`{"event":{"mail_address":"alice@example.com","message_id":"msg_1"}}`)
	got, err := processMessageReceived(context.Background(), rt, raw, map[string]string{
		"mailbox":               "alice@example.com",
		"msg_format":            "event",
		watchOutputDirFullParam: "true",
	})
	if err != nil {
		t.Fatalf("process failed: %v", err)
	}
	if len(rt.calls) != 1 || !strings.Contains(rt.calls[0].path, "?format=full") {
		t.Fatalf("expected full fetch, calls: %#v", rt.calls)
	}
	if !strings.Contains(string(got), `"subject":"Hello"`) {
		t.Fatalf("full message output missing subject: %s", got)
	}
	if strings.Contains(string(got), `"message":`) {
		t.Fatalf("output-dir payload should be bare full message, got: %s", got)
	}
}

func TestFetchMessageDecodeErrorIsTyped(t *testing.T) {
	rt := &stubClient{responses: map[string]json.RawMessage{
		"GET " + mailboxPath("alice@example.com", "messages", "msg_1") + "?format=metadata": json.RawMessage(`{not json`),
	}}

	_, err := fetchMessage(context.Background(), rt, "alice@example.com", "msg_1", "metadata")
	if err == nil {
		t.Fatal("expected decode error")
	}
	var internalErr *errs.InternalError
	if !errors.As(err, &internalErr) {
		t.Fatalf("error type = %T, want *errs.InternalError", err)
	}
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed problem, got %T", err)
	}
	if p.Subtype != errs.SubtypeInvalidResponse {
		t.Fatalf("subtype = %q, want %q", p.Subtype, errs.SubtypeInvalidResponse)
	}
	var syntaxErr *json.SyntaxError
	if !errors.As(err, &syntaxErr) {
		t.Fatalf("decode cause not preserved: %v", err)
	}
}

func TestProcessFiltersFetchedMessageByLabel(t *testing.T) {
	rt := &stubClient{responses: map[string]json.RawMessage{
		"GET " + mailboxPath("alice@example.com", "messages", "msg_1") + "?format=metadata": json.RawMessage(`{"code":0,"data":{"message":{"message_id":"msg_1","folder_id":"INBOX","label_ids":["OTHER"]}}}`),
	}}
	raw := rawMailEvent(`{"event":{"mail_address":"alice@example.com","message_id":"msg_1"}}`)
	got, err := processMessageReceived(context.Background(), rt, raw, map[string]string{
		"mailbox":    "alice@example.com",
		"msg_format": "metadata",
		"label_ids":  `["FLAGGED"]`,
	})
	if err != nil {
		t.Fatalf("process failed: %v", err)
	}
	if got != nil {
		t.Fatalf("expected filtered event to be dropped, got %s", got)
	}
}

func TestProcessMinimalReturnsTrimmedMessage(t *testing.T) {
	rt := &stubClient{responses: map[string]json.RawMessage{
		"GET " + mailboxPath("alice@example.com", "messages", "msg_1") + "?format=metadata": json.RawMessage(`{"code":0,"data":{"message":{"message_id":"msg_1","thread_id":"thr_1","folder_id":"INBOX","label_ids":["FLAGGED"],"subject":"hidden"}}}`),
	}}
	raw := rawMailEvent(`{"event":{"mail_address":"alice@example.com","message_id":"msg_1"}}`)
	got, err := processMessageReceived(context.Background(), rt, raw, map[string]string{
		"mailbox":    "alice@example.com",
		"msg_format": "minimal",
	})
	if err != nil {
		t.Fatalf("process failed: %v", err)
	}
	if strings.Contains(string(got), "subject") {
		t.Fatalf("minimal output leaked subject: %s", got)
	}
	if !strings.Contains(string(got), `"message_id":"msg_1"`) {
		t.Fatalf("minimal output missing message_id: %s", got)
	}
}

func rawMailEvent(payload string) *event.RawEvent {
	return &event.RawEvent{EventType: MessageReceivedEventKey, Payload: json.RawMessage(payload)}
}

func assertJSONArrayValues(t *testing.T, raw string, want []string) {
	t.Helper()
	var got []string
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("unmarshal %q: %v", raw, err)
	}
	if len(got) != len(want) {
		t.Fatalf("values = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("values = %#v, want %#v", got, want)
		}
	}
}
