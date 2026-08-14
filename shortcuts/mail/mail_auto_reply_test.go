// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/httpmock"
)

func TestMailAutoReply(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    mailboxPath("me", "settings", "auto_reply"),
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"auto_reply": map[string]interface{}{
					"enable":                 true,
					"content_summary":        "OOO",
					"start_time":             "1786755600",
					"end_time":               "1787014800",
					"timezone":               "Asia/Shanghai",
					"only_send_inner_sender": true,
				},
			},
		},
	})

	if err := runMountedMailShortcut(t, MailAutoReply, []string{"+auto-reply", "--format", "json"}, f, stdout); err != nil {
		t.Fatalf("runMountedMailShortcut() error = %v", err)
	}
	reg.Verify(t)

	data := decodeShortcutEnvelopeData(t, stdout)
	autoReply, ok := data["auto_reply"].(map[string]interface{})
	if !ok {
		t.Fatalf("auto_reply missing from output: %#v", data)
	}
	if autoReply["enable"] != true {
		t.Fatalf("enable = %#v, want true", autoReply["enable"])
	}
	if autoReply["content_summary"] != "OOO" {
		t.Fatalf("content_summary = %#v", autoReply["content_summary"])
	}
}

func TestMailAutoReplyModifyBuildsFriendlyPayload(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	var captured map[string]interface{}
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    mailboxPath("user@example.com", "settings", "auto_reply"),
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"auto_reply": map[string]interface{}{
					"enable":                 false,
					"content":                "<p>Old</p>",
					"content_summary":        "Old",
					"start_time":             "1786669200",
					"end_time":               "1786928400",
					"timezone":               "Asia/Shanghai",
					"only_send_inner_sender": true,
				},
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "PUT",
		URL:    mailboxPath("user@example.com", "settings", "auto_reply"),
		BodyFilter: func(body []byte) bool {
			if err := json.Unmarshal(body, &captured); err != nil {
				t.Fatalf("unmarshal request body: %v; body=%s", err, body)
			}
			return true
		},
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"auto_reply": map[string]interface{}{
					"enable":                 true,
					"content_summary":        "Out today",
					"start_time":             "1786755600",
					"end_time":               "1787014800",
					"timezone":               "Asia/Shanghai",
					"only_send_inner_sender": false,
				},
			},
		},
	})

	err := runMountedMailShortcut(t, MailAutoReplyModify, []string{
		"+auto-reply-modify",
		"--from", "user@example.com",
		"--enable",
		"--content", "<p>Out today</p>",
		"--start", "2026-08-15T09:00:00+08:00",
		"--end", "2026-08-18T09:00:00+08:00",
		"--timezone", "Asia/Shanghai",
		"--external",
		"--format", "json",
	}, f, stdout)
	if err != nil {
		t.Fatalf("runMountedMailShortcut() error = %v", err)
	}
	reg.Verify(t)

	autoReply, ok := captured["auto_reply"].(map[string]interface{})
	if !ok {
		t.Fatalf("auto_reply missing from request: %#v", captured)
	}
	assertAutoReplyPayloadValue(t, autoReply, "enable", true)
	assertAutoReplyPayloadValue(t, autoReply, "content", "<p>Out today</p>")
	assertAutoReplyPayloadValue(t, autoReply, "content_summary", "Out today")
	assertAutoReplyPayloadValue(t, autoReply, "start_time", "1786755600")
	assertAutoReplyPayloadValue(t, autoReply, "end_time", "1787014800")
	assertAutoReplyPayloadValue(t, autoReply, "timezone", "Asia/Shanghai")
	assertAutoReplyPayloadValue(t, autoReply, "only_send_inner_sender", false)
}

func TestMailAutoReplyContentFile(t *testing.T) {
	chdirTemp(t)
	if err := os.WriteFile("auto_reply.html", []byte("<p>From file</p>"), 0644); err != nil {
		t.Fatal(err)
	}

	f, stdout, _, reg := mailShortcutTestFactory(t)
	var captured map[string]interface{}
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    mailboxPath("me", "settings", "auto_reply"),
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"auto_reply": map[string]interface{}{
					"enable":                 true,
					"start_time":             "1786755600",
					"end_time":               "1787014800",
					"timezone":               "Asia/Shanghai",
					"only_send_inner_sender": true,
				},
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "PUT",
		URL:    mailboxPath("me", "settings", "auto_reply"),
		BodyFilter: func(body []byte) bool {
			if err := json.Unmarshal(body, &captured); err != nil {
				t.Fatalf("unmarshal request body: %v; body=%s", err, body)
			}
			return true
		},
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{"auto_reply": map[string]interface{}{"content_summary": "From file"}},
		},
	})

	if err := runMountedMailShortcut(t, MailAutoReplyModify, []string{"+auto-reply-modify", "--content-file", "auto_reply.html"}, f, stdout); err != nil {
		t.Fatalf("runMountedMailShortcut() error = %v", err)
	}
	reg.Verify(t)

	autoReply := captured["auto_reply"].(map[string]interface{})
	assertAutoReplyPayloadValue(t, autoReply, "content", "<p>From file</p>")
	assertAutoReplyPayloadValue(t, autoReply, "content_summary", "From file")
	assertAutoReplyPayloadValue(t, autoReply, "enable", true)
	assertAutoReplyPayloadValue(t, autoReply, "only_send_inner_sender", true)
}

func TestMailAutoReplyRejectsConflictingFlags(t *testing.T) {
	f, stdout, _, _ := mailShortcutTestFactory(t)
	err := runMountedMailShortcut(t, MailAutoReplyModify, []string{"+auto-reply-modify", "--enable", "--disable"}, f, stdout)
	if err == nil {
		t.Fatal("expected conflict error")
	}
	if !strings.Contains(err.Error(), "--enable and --disable are mutually exclusive") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMailShortcutsIncludeAutoReply(t *testing.T) {
	want := map[string]bool{
		"+auto-reply":        false,
		"+auto-reply-modify": false,
	}
	for _, shortcut := range Shortcuts() {
		if _, ok := want[shortcut.Command]; ok {
			want[shortcut.Command] = true
		}
	}
	for command, found := range want {
		if !found {
			t.Fatalf("Shortcuts() missing %s", command)
		}
	}
}

func assertAutoReplyPayloadValue(t *testing.T, payload map[string]interface{}, key string, want interface{}) {
	t.Helper()
	if got := payload[key]; got != want {
		t.Fatalf("%s = %#v, want %#v (payload=%#v)", key, got, want, payload)
	}
}
