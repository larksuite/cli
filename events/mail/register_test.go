// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"sync"
	"testing"

	"github.com/larksuite/cli/internal/event"
)

func TestMain(m *testing.M) {
	for _, k := range Keys() {
		event.RegisterKey(k)
	}
	os.Exit(m.Run())
}

func TestMailKeys_NativeRegistered(t *testing.T) {
	def, ok := event.Lookup(mailMessageReceivedKey)
	if !ok {
		t.Fatalf("%s should be registered via Keys()", mailMessageReceivedKey)
	}
	if def.Schema.Native == nil {
		t.Error("Schema.Native must be set for native key")
	}
	if def.Schema.Custom != nil {
		t.Error("Native key must not set Schema.Custom")
	}
	if def.Process != nil {
		t.Error("Native key must not set Process")
	}
	if def.Schema.Native != nil && def.Schema.Native.Type == nil {
		t.Error("Schema.Native.Type must reference an SDK type")
	}
	if def.EventType != mailMessageReceivedKey {
		t.Errorf("EventType = %q, want %q", def.EventType, mailMessageReceivedKey)
	}
	if def.PreConsume == nil {
		t.Error("PreConsume must be set so server-side subscription is opened on consume start")
	}
}

func TestMailKeys_Metadata(t *testing.T) {
	def, ok := event.Lookup(mailMessageReceivedKey)
	if !ok {
		t.Fatal("key not registered")
	}
	if len(def.Scopes) == 0 {
		t.Error("Scopes must not be empty — preflightScopes would bypass validation")
	}
	wantScopes := map[string]bool{
		"mail:event": false,
		"mail:user_mailbox.event.mail_address:read": false,
	}
	for _, s := range def.Scopes {
		if _, ok := wantScopes[s]; ok {
			wantScopes[s] = true
		}
	}
	for s, seen := range wantScopes {
		if !seen {
			t.Errorf("required scope missing: %s", s)
		}
	}
	if len(def.AuthTypes) != 1 || def.AuthTypes[0] != "user" {
		t.Errorf("AuthTypes = %v, want [user]", def.AuthTypes)
	}
	if len(def.RequiredConsoleEvents) != 1 || def.RequiredConsoleEvents[0] != mailMessageReceivedKey {
		t.Errorf("RequiredConsoleEvents = %v, want [%s]", def.RequiredConsoleEvents, mailMessageReceivedKey)
	}
}

func TestMailKeys_MailboxParam(t *testing.T) {
	def, ok := event.Lookup(mailMessageReceivedKey)
	if !ok {
		t.Fatal("key not registered")
	}
	if len(def.Params) != 1 {
		t.Fatalf("Params count = %d, want 1", len(def.Params))
	}
	p := def.Params[0]
	if p.Name != "mailbox" {
		t.Errorf("param name = %q, want mailbox", p.Name)
	}
	if p.Default != "me" {
		t.Errorf("param default = %q, want me", p.Default)
	}
	if p.Required {
		t.Error("mailbox param should not be required")
	}
	if p.Type != event.ParamString {
		t.Errorf("param type = %q, want string", p.Type)
	}
}

// fakeClient records every CallAPI invocation so tests can assert on
// PreConsume's subscribe + cleanup-unsubscribe sequence without touching
// the real OAPI.
type fakeClient struct {
	mu       sync.Mutex
	calls    []fakeCall
	subErr   error
	unsubErr error
}

type fakeCall struct {
	method string
	path   string
	body   any
}

func (f *fakeClient) CallAPI(_ context.Context, method, path string, body any) (json.RawMessage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, fakeCall{method: method, path: path, body: body})
	switch {
	case isSubscribePath(path):
		return json.RawMessage(`{"code":0,"msg":"success"}`), f.subErr
	case isUnsubscribePath(path):
		return json.RawMessage(`{"code":0,"msg":"success"}`), f.unsubErr
	}
	return json.RawMessage(`{}`), nil
}

func isSubscribePath(p string) bool   { return endsWith(p, "/event/subscribe") }
func isUnsubscribePath(p string) bool { return endsWith(p, "/event/unsubscribe") }

func endsWith(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}

func TestPreConsume_DefaultMailboxIsMe(t *testing.T) {
	fake := &fakeClient{}
	cleanup, err := preConsumeMailSubscribe(context.Background(), fake, nil)
	if err != nil {
		t.Fatalf("preConsume returned error: %v", err)
	}
	if cleanup == nil {
		t.Fatal("cleanup must be returned on success so unsubscribe runs on shutdown")
	}
	if len(fake.calls) != 1 {
		t.Fatalf("expected 1 subscribe call, got %d", len(fake.calls))
	}
	c := fake.calls[0]
	if c.method != "POST" {
		t.Errorf("method = %q, want POST", c.method)
	}
	wantPath := "/open-apis/mail/v1/user_mailboxes/me/event/subscribe"
	if c.path != wantPath {
		t.Errorf("path = %q, want %q", c.path, wantPath)
	}
	body, ok := c.body.(map[string]any)
	if !ok {
		t.Fatalf("body type = %T, want map[string]any", c.body)
	}
	if body["event_type"] != mailSubscribeEventTypeNewMessage {
		t.Errorf("body event_type = %v, want %d", body["event_type"], mailSubscribeEventTypeNewMessage)
	}
}

func TestPreConsume_CustomMailboxAppearsInPath(t *testing.T) {
	fake := &fakeClient{}
	_, err := preConsumeMailSubscribe(context.Background(), fake, map[string]string{"mailbox": "alice@example.com"})
	if err != nil {
		t.Fatalf("preConsume returned error: %v", err)
	}
	// @ is a valid pchar per RFC 3986 §3.3 — url.PathEscape correctly leaves it.
	wantPath := "/open-apis/mail/v1/user_mailboxes/alice@example.com/event/subscribe"
	if fake.calls[0].path != wantPath {
		t.Errorf("path = %q, want %q", fake.calls[0].path, wantPath)
	}
}

func TestPreConsume_MailboxWithSlashIsEscaped(t *testing.T) {
	// Defense against a malicious or malformed mailbox arg slipping a path
	// segment break (e.g. mailbox="me/admin"). url.PathEscape must catch this.
	fake := &fakeClient{}
	_, err := preConsumeMailSubscribe(context.Background(), fake, map[string]string{"mailbox": "me/admin"})
	if err != nil {
		t.Fatalf("preConsume returned error: %v", err)
	}
	wantPath := "/open-apis/mail/v1/user_mailboxes/me%2Fadmin/event/subscribe"
	if fake.calls[0].path != wantPath {
		t.Errorf("path = %q, want %q (slash must be escaped to prevent path traversal)", fake.calls[0].path, wantPath)
	}
}

func TestPreConsume_CleanupUnsubscribes(t *testing.T) {
	fake := &fakeClient{}
	cleanup, err := preConsumeMailSubscribe(context.Background(), fake, nil)
	if err != nil {
		t.Fatalf("preConsume returned error: %v", err)
	}
	cleanup()
	if len(fake.calls) != 2 {
		t.Fatalf("expected subscribe+unsubscribe (2 calls), got %d", len(fake.calls))
	}
	unsub := fake.calls[1]
	wantPath := "/open-apis/mail/v1/user_mailboxes/me/event/unsubscribe"
	if unsub.path != wantPath {
		t.Errorf("unsubscribe path = %q, want %q", unsub.path, wantPath)
	}
	if unsub.method != "POST" {
		t.Errorf("unsubscribe method = %q, want POST", unsub.method)
	}
	body, _ := unsub.body.(map[string]any)
	if body == nil || body["event_type"] != mailSubscribeEventTypeNewMessage {
		t.Errorf("unsubscribe body = %v, want event_type=%d", unsub.body, mailSubscribeEventTypeNewMessage)
	}
}

func TestPreConsume_SubscribeFailureReturnsError(t *testing.T) {
	fake := &fakeClient{subErr: errors.New("api blew up")}
	cleanup, err := preConsumeMailSubscribe(context.Background(), fake, nil)
	if err == nil {
		t.Fatal("preConsume must return error when subscribe API fails")
	}
	if cleanup != nil {
		t.Error("cleanup must be nil on error so consume.go does not call unsubscribe for a never-opened subscription")
	}
}

func TestPreConsume_EmptyMailboxParamFallsBackToMe(t *testing.T) {
	// Defends agent invocations that pass --param mailbox= (whitespace / unset).
	fake := &fakeClient{}
	_, err := preConsumeMailSubscribe(context.Background(), fake, map[string]string{"mailbox": "   "})
	if err != nil {
		t.Fatalf("preConsume returned error: %v", err)
	}
	wantPath := "/open-apis/mail/v1/user_mailboxes/me/event/subscribe"
	if fake.calls[0].path != wantPath {
		t.Errorf("path = %q, want %q", fake.calls[0].path, wantPath)
	}
}

func TestMailboxEventPath_AssemblesBothSegments(t *testing.T) {
	got := mailboxEventPath("me", "subscribe")
	want := "/open-apis/mail/v1/user_mailboxes/me/event/subscribe"
	if got != want {
		t.Errorf("mailboxEventPath() = %q, want %q", got, want)
	}
}
