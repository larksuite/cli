// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package convertlib

import (
	"fmt"
	"net/http"
	"reflect"
	"testing"
	"time"
)

func TestParseJSONObject(t *testing.T) {
	got, err := ParseJSONObject(`{"text":"hello","count":2}`)
	if err != nil {
		t.Fatalf("ParseJSONObject() error = %v", err)
	}
	if got["text"] != "hello" {
		t.Fatalf("ParseJSONObject() text = %#v, want %#v", got["text"], "hello")
	}

	if invalid, err := ParseJSONObject(`{invalid`); err == nil || invalid != nil {
		t.Fatalf("ParseJSONObject() invalid JSON = (%#v, %v), want (nil, err)", invalid, err)
	}
}

func TestBuildMentionKeyMap(t *testing.T) {
	mentions := []interface{}{
		map[string]interface{}{"key": "@_user_1", "name": "Alice"},
		map[string]interface{}{"key": "@_user_2", "name": "Bob"},
		map[string]interface{}{"key": "", "name": "Ignored"},
		map[string]interface{}{"key": "@_user_3"},
	}

	got := BuildMentionKeyMap(mentions)
	want := map[string]string{
		"@_user_1": "Alice",
		"@_user_2": "Bob",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("BuildMentionKeyMap() = %#v, want %#v", got, want)
	}
}

func TestResolveMentionKeys(t *testing.T) {
	got := ResolveMentionKeys("hi @_user_1 and @_user_2", map[string]string{
		"@_user_1": "Alice",
		"@_user_2": "Bob",
	})
	want := "hi @Alice and @Bob"
	if got != want {
		t.Fatalf("ResolveMentionKeys() = %q, want %q", got, want)
	}
}

func TestFormatTimestamp(t *testing.T) {
	sec := int64(1710500000)
	want := time.Unix(sec, 0).Local().Format("2006-01-02 15:04:05")

	if got := formatTimestamp("1710500000"); got != want {
		t.Fatalf("formatTimestamp(seconds) = %q, want %q", got, want)
	}
	if got := formatTimestamp("1710500000000"); got != want {
		t.Fatalf("formatTimestamp(milliseconds) = %q, want %q", got, want)
	}
	if got := formatTimestamp(""); got != "" {
		t.Fatalf("formatTimestamp(empty) = %q, want empty", got)
	}
	if got := formatTimestamp("not-a-number"); got != "" {
		t.Fatalf("formatTimestamp(invalid) = %q, want empty", got)
	}
	futureSec := int64(10000000000)
	wantFuture := time.Unix(futureSec, 0).Local().Format("2006-01-02 15:04:05")
	if got := formatTimestamp("10000000000"); got != wantFuture {
		t.Fatalf("formatTimestamp(future seconds) = %q, want %q", got, wantFuture)
	}
}

func TestAttachSenderNames(t *testing.T) {
	messages := []map[string]interface{}{
		{"sender": map[string]interface{}{"id": "ou_alice"}},
		{"sender": map[string]interface{}{"id": "ou_bob", "name": "Existing"}},
		{"sender": map[string]interface{}{"id": "ou_carol"}},
		{"sender": "not-a-map"},
	}
	nameMap := map[string]string{"ou_alice": "Alice"}

	AttachSenderNames(messages, nameMap)

	sender1 := messages[0]["sender"].(map[string]interface{})
	if sender1["name"] != "Alice" {
		t.Fatalf("AttachSenderNames() resolved name = %#v, want %#v", sender1["name"], "Alice")
	}

	sender2 := messages[1]["sender"].(map[string]interface{})
	if sender2["name"] != "Existing" {
		t.Fatalf("AttachSenderNames() existing name = %#v, want %#v", sender2["name"], "Existing")
	}

	sender3 := messages[2]["sender"].(map[string]interface{})
	if _, hasName := sender3["name"]; hasName {
		t.Fatalf("AttachSenderNames() unresolved sender should have no name, got %#v", sender3["name"])
	}
}

func TestExtractPostBlocksText(t *testing.T) {
	blocks := []interface{}{
		[]interface{}{
			map[string]interface{}{"tag": "text", "text": "hello "},
			map[string]interface{}{"tag": "at", "user_name": "Alice"},
			map[string]interface{}{"tag": "text", "text": " "},
			map[string]interface{}{"tag": "a", "text": "docs", "href": "https://example.com"},
		},
		[]interface{}{
			map[string]interface{}{"tag": "img", "image_key": "img_123"},
		},
		[]interface{}{},
	}

	got := extractPostBlocksText(blocks)
	want := "hello @Alice [docs](https://example.com)\n![Image](img_123)"
	if got != want {
		t.Fatalf("extractPostBlocksText() = %q, want %q", got, want)
	}
}

func TestResolveSenderNames(t *testing.T) {
	// Server-provided sender_name is harvested into the cache for both user and bot;
	// senders the server did not name are absent (id fallback downstream). There is no
	// contact/mention lookup, so no API call is ever made.
	rt := newBotConvertlibRuntime(t, convertlibRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		return nil, fmt.Errorf("no API call expected: %s", req.URL.String())
	}))

	messages := []map[string]interface{}{
		{"sender": map[string]interface{}{"sender_type": "user", "id": "ou_named", "sender_name": "Named User"}},
		{"sender": map[string]interface{}{"sender_type": "app", "id": "cli_bot", "sender_name": "Bot Alpha"}},
		{"sender": map[string]interface{}{"sender_type": "user", "id": "ou_unnamed"}},
	}

	got := ResolveSenderNames(rt, messages, nil)
	if got["ou_named"] != "Named User" {
		t.Fatalf("named user = %#v, want %#v", got["ou_named"], "Named User")
	}
	if got["cli_bot"] != "Bot Alpha" {
		t.Fatalf("named bot = %#v, want %#v", got["cli_bot"], "Bot Alpha")
	}
	if _, has := got["ou_unnamed"]; has {
		t.Fatalf("unnamed sender must not be resolved (no contact fallback), got %#v", got["ou_unnamed"])
	}
}

// TestResolveSenderNamesServerNameBeatsMention locks the priority: when a sender's id
// also appears as a mention, the server-provided sender_name must win over the mention
// name (which can be a remark/nickname), and no contact call is made.
func TestResolveSenderNamesServerNameBeatsMention(t *testing.T) {
	rt := newBotConvertlibRuntime(t, convertlibRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		return nil, fmt.Errorf("no contact call expected: %s", req.URL.String())
	}))
	messages := []map[string]interface{}{
		{
			"sender": map[string]interface{}{"sender_type": "user", "id": "ou_dual", "sender_name": "Server Name"},
			"mentions": []interface{}{
				map[string]interface{}{"id": "ou_dual", "name": "Mention Remark"},
			},
		},
	}
	got := ResolveSenderNames(rt, messages, nil)
	if got["ou_dual"] != "Server Name" {
		t.Fatalf("server sender_name must beat mention name: got %#v, want %#v", got["ou_dual"], "Server Name")
	}
}

// TestFormatMessageItemSenderPassthrough covers AC5: the formatted message must
// carry the sender object through verbatim — retaining open_bot_id and leaving
// id / id_type unchanged after enrichment.
func TestFormatMessageItemSenderPassthrough(t *testing.T) {
	runtime := newBotConvertlibRuntime(t, convertlibRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		return convertlibJSONResponse(200, map[string]interface{}{"code": 0, "data": map[string]interface{}{}}), nil
	}))
	m := map[string]interface{}{
		"message_id": "om_1",
		"msg_type":   "text",
		"body":       map[string]interface{}{"content": `{"text":"hi"}`},
		"sender": map[string]interface{}{
			"id":          "cli_bot",
			"id_type":     "app_id",
			"sender_type": "app",
			"sender_name": "Bot Alpha",
			"open_bot_id": "ou_bot",
		},
	}

	out := FormatMessageItem(m, runtime)
	sender, ok := out["sender"].(map[string]interface{})
	if !ok {
		t.Fatalf("formatted sender missing/mistyped: %#v", out["sender"])
	}
	if sender["open_bot_id"] != "ou_bot" {
		t.Fatalf("open_bot_id passthrough = %#v, want %#v", sender["open_bot_id"], "ou_bot")
	}
	if sender["id"] != "cli_bot" || sender["id_type"] != "app_id" {
		t.Fatalf("id/id_type must be unchanged, got id=%#v id_type=%#v", sender["id"], sender["id_type"])
	}
}

func TestPickSenderName(t *testing.T) {
	// Uses the server-provided sender_name.
	if got := pickSenderName(map[string]interface{}{"sender_name": "Bot Alpha"}); got != "Bot Alpha" {
		t.Fatalf("pickSenderName(sender_name) = %q, want %q", got, "Bot Alpha")
	}
	// sender_i18n_names is NOT consulted for the display name (it stays in output for
	// consumers that want a specific locale); no sender_name -> empty (caller uses id).
	i18nOnly := map[string]interface{}{
		"sender_i18n_names": map[string]interface{}{"en_us": "Bot Beta", "zh_cn": "机器人乙", "ja_jp": "ロボット"},
	}
	if got := pickSenderName(i18nOnly); got != "" {
		t.Fatalf("pickSenderName(i18n only, no sender_name) = %q, want empty", got)
	}
	// Empty sender_name -> empty (no i18n fallthrough).
	if got := pickSenderName(map[string]interface{}{"sender_name": ""}); got != "" {
		t.Fatalf("pickSenderName(empty sender_name) = %q, want empty", got)
	}
	// Nothing available -> empty (caller falls back to id).
	if got := pickSenderName(map[string]interface{}{"id": "cli_x"}); got != "" {
		t.Fatalf("pickSenderName(no name) = %q, want empty", got)
	}
}

// TestAttachSenderNamesPrefersProducerName covers AC1 (bot display name), AC2
// (user producer name), AC5 (open_bot_id passthrough) and AC3 (id fallback).
func TestAttachSenderNamesPrefersProducerName(t *testing.T) {
	i18n := map[string]interface{}{"en_us": "Bot Alpha", "zh_cn": "机器人甲"}
	messages := []map[string]interface{}{
		// bot sender with producer-filled sender_name (AC1) + sender_i18n_names + open_bot_id (AC5)
		{"sender": map[string]interface{}{"sender_type": "app", "id": "cli_bot", "sender_name": "机器人甲", "sender_i18n_names": i18n, "open_bot_id": "ou_bot"}},
		// user sender with producer-filled sender_name (AC2, unified read)
		{"sender": map[string]interface{}{"sender_type": "user", "id": "ou_user1", "sender_name": "Producer User"}},
		// user sender without producer name -> resolved from the shared name cache (nameMap)
		{"sender": map[string]interface{}{"sender_type": "user", "id": "ou_user2"}},
		// bot sender without any name -> stays id (AC3)
		{"sender": map[string]interface{}{"sender_type": "app", "id": "cli_unknown"}},
	}
	nameMap := map[string]string{"ou_user2": "Contact User"}

	AttachSenderNames(messages, nameMap)

	s0 := messages[0]["sender"].(map[string]interface{})
	if s0["name"] != "机器人甲" {
		t.Fatalf("bot sender name = %#v, want %#v", s0["name"], "机器人甲")
	}
	if s0["open_bot_id"] != "ou_bot" {
		t.Fatalf("bot open_bot_id passthrough = %#v, want %#v", s0["open_bot_id"], "ou_bot")
	}
	// sender_name is dropped (duplicate of name); sender_i18n_names is kept.
	if _, has := s0["sender_name"]; has {
		t.Fatalf("sender_name should be stripped from output, got %#v", s0["sender_name"])
	}
	if _, has := s0["sender_i18n_names"]; !has {
		t.Fatalf("sender_i18n_names should be preserved in output")
	}
	if s := messages[1]["sender"].(map[string]interface{}); s["name"] != "Producer User" {
		t.Fatalf("user producer name = %#v, want %#v", s["name"], "Producer User")
	}
	if s := messages[2]["sender"].(map[string]interface{}); s["name"] != "Contact User" {
		t.Fatalf("user contact-fallback name = %#v, want %#v", s["name"], "Contact User")
	}
	if s := messages[3]["sender"].(map[string]interface{}); s["name"] != nil {
		t.Fatalf("unresolved bot sender should keep no name (id fallback), got %#v", s["name"])
	}
}

// TestSystemMessageNeedsNoName documents that system messages — identified by
// msg_type=="system", not by any sender id — need no display name: the producer
// fills none and their sender carries no ou_ id, so they never hit the contact API
// and are left without a name (no error). An empty sender name is normal here.
func TestSystemMessageNeedsNoName(t *testing.T) {
	failIfContactCalled := newBotConvertlibRuntime(t, convertlibRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		return nil, fmt.Errorf("system message must not trigger any API call: %s", req.URL.String())
	}))

	messages := []map[string]interface{}{
		{"msg_type": "system", "sender": map[string]interface{}{"sender_type": "system"}},
		{"msg_type": "system"}, // system message without a sender object at all
	}

	got := ResolveSenderNames(failIfContactCalled, messages, nil)
	if len(got) != 0 {
		t.Fatalf("system messages resolved names = %#v, want empty", got)
	}

	AttachSenderNames(messages, got)
	if s := messages[0]["sender"].(map[string]interface{}); s["name"] != nil {
		t.Fatalf("system message sender should keep no name, got %#v", s["name"])
	}
}
