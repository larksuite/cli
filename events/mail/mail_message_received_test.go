// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/larksuite/cli/internal/event"
)

func TestMain(m *testing.M) {
	for _, k := range Keys() {
		event.RegisterKey(k)
	}
	os.Exit(m.Run())
}

// ---- KeyDefinition field assertions ----

func TestMailKeysReturnsMailMessageReceived(t *testing.T) {
	keys := Keys()
	if len(keys) != 1 {
		t.Fatalf("expected 1 key, got %d", len(keys))
	}
	if keys[0].Key != "mail.user_mailbox.event.message_received_v1" {
		t.Errorf("unexpected key: %q", keys[0].Key)
	}
}

func TestMailKeyDefinitionScopesAlignsWithMailWatch(t *testing.T) {
	// Single source of truth: mail_watch.go:98 (7 items, same order)
	wantScopes := []string{
		"mail:event",
		"mail:user_mailbox.event.mail_address:read",
		"mail:user_mailbox:readonly",
		"mail:user_mailbox.message:readonly",
		"mail:user_mailbox.message.address:read",
		"mail:user_mailbox.message.subject:read",
		"mail:user_mailbox.message.body:read",
	}
	got := Keys()[0].Scopes
	if len(got) != 7 {
		t.Fatalf("expected 7 scopes, got %d: %v", len(got), got)
	}
	for i, want := range wantScopes {
		if got[i] != want {
			t.Errorf("scope[%d]: want %q, got %q", i, want, got[i])
		}
	}
}

func TestMailKeyDefinitionAuthTypesIsUserOnly(t *testing.T) {
	at := Keys()[0].AuthTypes
	if len(at) != 1 || at[0] != "user" {
		t.Errorf("expected AuthTypes=[\"user\"], got %v", at)
	}
}

func TestMailKeyDefinitionRequiredConsoleEvents(t *testing.T) {
	rce := Keys()[0].RequiredConsoleEvents
	if len(rce) != 1 || rce[0] != "mail.user_mailbox.event.message_received_v1" {
		t.Errorf("unexpected RequiredConsoleEvents: %v", rce)
	}
}

func TestMailKeyDefinitionParamMailboxDefault(t *testing.T) {
	params := Keys()[0].Params
	if len(params) != 1 {
		t.Fatalf("expected 1 param, got %d", len(params))
	}
	p := params[0]
	if p.Name != "mailbox" {
		t.Errorf("param name: want \"mailbox\", got %q", p.Name)
	}
	if p.Type != event.ParamString {
		t.Errorf("param type: want ParamString, got %v", p.Type)
	}
	if p.Required {
		t.Error("mailbox param must not be Required")
	}
	if p.Default != "me" {
		t.Errorf("param default: want \"me\", got %q", p.Default)
	}
	if !strings.Contains(p.Description, "comma-separated") {
		t.Errorf("param description should mention comma-separated, got %q", p.Description)
	}
}

// ---- parseMailboxes ----

func TestParseMailboxes_DefaultMe(t *testing.T) {
	got := parseMailboxes("")
	if len(got) != 1 || got[0] != "me" {
		t.Errorf("expected [\"me\"], got %v", got)
	}
}

func TestParseMailboxes_TrimsWhitespace(t *testing.T) {
	got := parseMailboxes("  alice@x  ,  bob@x  ")
	want := []string{"alice@x", "bob@x"}
	if len(got) != len(want) {
		t.Fatalf("want %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("[%d] want %q, got %q", i, want[i], got[i])
		}
	}
}

func TestParseMailboxes_DedupPreservingOrder(t *testing.T) {
	got := parseMailboxes("alice@x,bob@x,alice@x,carol@x")
	want := []string{"alice@x", "bob@x", "carol@x"}
	if len(got) != len(want) {
		t.Fatalf("want %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("[%d] want %q, got %q", i, want[i], got[i])
		}
	}
}

func TestParseMailboxes_DropsEmptyEntries(t *testing.T) {
	got := parseMailboxes("alice@x,,bob@x,")
	want := []string{"alice@x", "bob@x"}
	if len(got) != len(want) {
		t.Fatalf("want %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("[%d] want %q, got %q", i, want[i], got[i])
		}
	}
}

// ---- mock APIClient for PreConsume tests ----

type mockCall struct {
	method string
	path   string
	body   interface{}
}

type mockAPIClient struct {
	calls   []mockCall
	failAt  int // 1-indexed; 0 = never fail
	failErr error
}

func (m *mockAPIClient) CallAPI(_ context.Context, method, path string, body interface{}) (json.RawMessage, error) {
	m.calls = append(m.calls, mockCall{method: method, path: path, body: body})
	if m.failAt > 0 && len(m.calls) == m.failAt {
		return nil, m.failErr
	}
	return json.RawMessage(`{"code":0}`), nil
}

// ---- PreConsume tests ----

func TestMailMessageReceivedPreConsume_SingleMailboxHappy(t *testing.T) {
	mc := &mockAPIClient{}
	cleanup, err := mailMessageReceivedPreConsume(context.Background(), mc, map[string]string{"mailbox": "alice@x"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if cleanup == nil {
		t.Fatal("expected non-nil cleanup")
	}
	if len(mc.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(mc.calls))
	}
	// url.PathEscape leaves @ unencoded (valid in path segments per RFC 3986)
	if !strings.Contains(mc.calls[0].path, "alice@x") || !strings.Contains(mc.calls[0].path, "subscribe") {
		t.Errorf("unexpected subscribe path: %s", mc.calls[0].path)
	}
}

func TestMailMessageReceivedPreConsume_MultiMailboxHappy(t *testing.T) {
	mc := &mockAPIClient{}
	cleanup, err := mailMessageReceivedPreConsume(context.Background(), mc, map[string]string{"mailbox": "a@x,b@x,c@x"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cleanup == nil {
		t.Fatal("expected cleanup")
	}
	if len(mc.calls) != 3 {
		t.Fatalf("expected 3 subscribe calls, got %d: %v", len(mc.calls), mc.calls)
	}
	// Verify order: a → b → c (url.PathEscape leaves @ unencoded)
	for i, want := range []string{"a@x", "b@x", "c@x"} {
		if !strings.Contains(mc.calls[i].path, want) || !strings.Contains(mc.calls[i].path, "subscribe") {
			t.Errorf("call[%d] path %q should contain %q/subscribe", i, mc.calls[i].path, want)
		}
	}
}

func TestMailMessageReceivedPreConsume_PartialFailureRollsBack(t *testing.T) {
	// Third call (carol) fails; rollback: bob/unsub → alice/unsub
	mc := &mockAPIClient{failAt: 3, failErr: errors.New("boom")}
	cleanup, err := mailMessageReceivedPreConsume(context.Background(), mc, map[string]string{"mailbox": "alice@x,bob@x,carol@x"})
	if err == nil {
		t.Fatal("expected error")
	}
	if cleanup != nil {
		t.Fatal("expected nil cleanup on failure")
	}
	if !strings.Contains(err.Error(), "carol@x") {
		t.Errorf("error should mention carol@x: %v", err)
	}
	if !strings.Contains(err.Error(), "mail event subscribe failed") {
		t.Errorf("error text mismatch: %v", err)
	}
	// calls: alice/sub, bob/sub, carol/sub(fail), bob/unsub, alice/unsub
	if len(mc.calls) != 5 {
		t.Fatalf("expected 5 calls (3 sub + 2 rollback), got %d: %v", len(mc.calls), mc.calls)
	}
	// rollback in reverse: bob then alice (url.PathEscape leaves @ unencoded)
	if !strings.Contains(mc.calls[3].path, "bob@x") || !strings.Contains(mc.calls[3].path, "unsubscribe") {
		t.Errorf("rollback call[3] should be bob/unsubscribe, got %q", mc.calls[3].path)
	}
	if !strings.Contains(mc.calls[4].path, "alice@x") || !strings.Contains(mc.calls[4].path, "unsubscribe") {
		t.Errorf("rollback call[4] should be alice/unsubscribe, got %q", mc.calls[4].path)
	}
}

func TestMailMessageReceivedPreConsume_FirstFailureNoRollback(t *testing.T) {
	mc := &mockAPIClient{failAt: 1, failErr: errors.New("denied")}
	cleanup, err := mailMessageReceivedPreConsume(context.Background(), mc, map[string]string{"mailbox": "alice@x"})
	if err == nil {
		t.Fatal("expected error")
	}
	if cleanup != nil {
		t.Fatal("expected nil cleanup")
	}
	// Only 1 call: alice/subscribe (fail). No rollback since subscribed list is empty.
	if len(mc.calls) != 1 {
		t.Fatalf("expected 1 call, got %d: %v", len(mc.calls), mc.calls)
	}
}

func TestMailMessageReceivedPreConsume_RollbackUnsubscribeFailureSwallowed(t *testing.T) {
	// 2nd subscribe fails; rollback: alice/unsub also fails (should be swallowed).
	callCount := 0
	mc := &mockAPIClient{}
	mc.failAt = 0 // use custom logic below

	// Build a custom mock: call 1 (alice/sub) OK, call 2 (bob/sub) FAIL, call 3 (alice/unsub rollback) FAIL
	mc2 := &customFailMockAPIClient{
		failCalls: map[int]error{
			2: errors.New("bob subscribe failed"),
			3: errors.New("rollback unsubscribe also failed"),
		},
	}
	_ = callCount

	cleanup, err := mailMessageReceivedPreConsume(context.Background(), mc2, map[string]string{"mailbox": "alice@x,bob@x"})
	if err == nil {
		t.Fatal("expected error from bob subscribe failure")
	}
	if cleanup != nil {
		t.Fatal("expected nil cleanup")
	}
	if !strings.Contains(err.Error(), "bob@x") {
		t.Errorf("error should mention bob@x: %v", err)
	}
	// 3 calls: alice/sub, bob/sub(fail), alice/unsub(fail, swallowed)
	if len(mc2.calls) != 3 {
		t.Fatalf("expected 3 calls, got %d", len(mc2.calls))
	}
}

func TestMailMessageReceivedPreConsume_CleanupReverseOrder(t *testing.T) {
	mc := &mockAPIClient{}
	cleanup, err := mailMessageReceivedPreConsume(context.Background(), mc, map[string]string{"mailbox": "a@x,b@x,c@x"})
	if err != nil || cleanup == nil {
		t.Fatalf("setup failed: err=%v", err)
	}
	callsBefore := len(mc.calls) // 3 subscribe calls
	cleanup()
	cleanupCalls := mc.calls[callsBefore:]
	if len(cleanupCalls) != 3 {
		t.Fatalf("expected 3 cleanup calls, got %d", len(cleanupCalls))
	}
	// Reverse order: c → b → a (url.PathEscape leaves @ unencoded)
	for i, want := range []string{"c@x", "b@x", "a@x"} {
		if !strings.Contains(cleanupCalls[i].path, want) || !strings.Contains(cleanupCalls[i].path, "unsubscribe") {
			t.Errorf("cleanup[%d] path %q should contain %q/unsubscribe", i, cleanupCalls[i].path, want)
		}
	}
}

func TestMailMessageReceivedPreConsume_CleanupIndependentFailures(t *testing.T) {
	// All subscribes succeed; cleanup of b fails but a and c should still be called.
	mc := &customFailMockAPIClient{
		failCalls: map[int]error{
			5: errors.New("b unsubscribe fail"), // call 4=a/sub, 5=b/sub, 6=c/sub (subscribe), then c=7,b=8,a=9 for cleanup
		},
	}
	cleanup, err := mailMessageReceivedPreConsume(context.Background(), mc, map[string]string{"mailbox": "a@x,b@x,c@x"})
	if err != nil || cleanup == nil {
		t.Fatalf("setup failed: err=%v", err)
	}
	// Override failCalls to fail the b unsubscribe in cleanup (2nd unsubscribe, i.e., the 5th overall call)
	mc.failCalls = map[int]error{
		5: errors.New("b cleanup fail"),
	}
	cleanup()
	// All 3 cleanup unsubscribes must be attempted despite one failing
	if len(mc.calls) != 6 {
		t.Fatalf("expected 6 calls (3 sub + 3 cleanup), got %d", len(mc.calls))
	}
}

func TestMailMessageReceivedPreConsume_DefaultMailboxMe(t *testing.T) {
	// Passing empty mailbox param should default to "me" via parseMailboxes
	mc := &mockAPIClient{}
	cleanup, err := mailMessageReceivedPreConsume(context.Background(), mc, map[string]string{"mailbox": ""})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cleanup == nil {
		t.Fatal("expected cleanup")
	}
	if len(mc.calls) != 1 || !strings.Contains(mc.calls[0].path, "/me/") {
		t.Errorf("expected subscribe call to /me/, got %v", mc.calls)
	}
}

// ---- processMailMessageReceived ----

func TestProcessMailMessageReceived_FlatShape(t *testing.T) {
	payload := `{
		"header": {
			"event_id": "ev_001",
			"event_type": "mail.user_mailbox.event.message_received_v1",
			"create_time": "1700000000000"
		},
		"event": {
			"mail_address": "alice@example.com",
			"message_id": "msg_abc",
			"sender": "bob@example.com",
			"subject": "Hello",
			"body": "World"
		}
	}`
	raw := &event.RawEvent{
		EventID:   "ev_001",
		EventType: mailEventType,
		Payload:   json.RawMessage(payload),
		Timestamp: time.Now(),
	}
	got, err := processMailMessageReceived(context.Background(), nil, raw, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var out MailMessageReceivedOutput
	if err := json.Unmarshal(got, &out); err != nil {
		t.Fatalf("unmarshal failed: %v\njson=%s", err, string(got))
	}
	if out.Type != "mail.user_mailbox.event.message_received_v1" {
		t.Errorf("Type = %q", out.Type)
	}
	if out.EventID != "ev_001" {
		t.Errorf("EventID = %q", out.EventID)
	}
	if out.Timestamp != "1700000000000" {
		t.Errorf("Timestamp = %q", out.Timestamp)
	}
	if out.Mailbox != "alice@example.com" {
		t.Errorf("Mailbox = %q", out.Mailbox)
	}
	if out.MessageID != "msg_abc" {
		t.Errorf("MessageID = %q", out.MessageID)
	}
	if out.Sender != "bob@example.com" {
		t.Errorf("Sender = %q", out.Sender)
	}
	if out.Subject != "Hello" {
		t.Errorf("Subject = %q", out.Subject)
	}
	if out.BodyExcerpt != "World" {
		t.Errorf("BodyExcerpt = %q", out.BodyExcerpt)
	}
}

func TestProcessMailMessageReceived_BodyExcerptTruncates140(t *testing.T) {
	body := strings.Repeat("x", 200)
	payload := fmt.Sprintf(`{
		"header": {"event_id": "ev_002", "event_type": "mail.user_mailbox.event.message_received_v1", "create_time": ""},
		"event": {"mail_address": "a@b.com", "message_id": "", "sender": "", "subject": "", "body": %q}
	}`, body)
	raw := &event.RawEvent{
		EventID:   "ev_002",
		EventType: mailEventType,
		Payload:   json.RawMessage(payload),
		Timestamp: time.Now(),
	}
	got, err := processMailMessageReceived(context.Background(), nil, raw, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var out MailMessageReceivedOutput
	if err := json.Unmarshal(got, &out); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if len(out.BodyExcerpt) != 140 {
		t.Errorf("BodyExcerpt length = %d, want 140", len(out.BodyExcerpt))
	}
}

func TestProcessMailMessageReceived_MalformedPayloadPassthrough(t *testing.T) {
	malformed := json.RawMessage(`not valid json`)
	raw := &event.RawEvent{
		EventID:   "ev_bad",
		EventType: mailEventType,
		Payload:   malformed,
		Timestamp: time.Now(),
	}
	got, err := processMailMessageReceived(context.Background(), nil, raw, nil)
	if err != nil {
		t.Fatalf("expected nil error on malformed payload, got %v", err)
	}
	if string(got) != string(malformed) {
		t.Errorf("expected passthrough of malformed payload, got %q", string(got))
	}
}

// ---- helpers ----

type customFailMockAPIClient struct {
	calls     []mockCall
	failCalls map[int]error // 1-indexed call number → error to return
}

func (m *customFailMockAPIClient) CallAPI(_ context.Context, method, path string, body interface{}) (json.RawMessage, error) {
	m.calls = append(m.calls, mockCall{method: method, path: path, body: body})
	if err, ok := m.failCalls[len(m.calls)]; ok {
		return nil, err
	}
	return json.RawMessage(`{"code":0}`), nil
}
