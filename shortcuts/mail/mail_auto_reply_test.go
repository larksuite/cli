// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"encoding/base64"
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
					"enabled":             true,
					"content_summary":     "OOO",
					"start_time":          "1786723200000",
					"end_time":            "1787068799999",
					"time_zone":           "Asia/Shanghai",
					"only_send_to_tenant": true,
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
	if autoReply["enabled"] != true {
		t.Fatalf("enabled = %#v, want true", autoReply["enabled"])
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
					"enabled":             false,
					"content_html":        "<p>Old</p>",
					"content_summary":     "Old",
					"start_time":          "1786636800000",
					"end_time":            "1786895999999",
					"time_zone":           "Asia/Shanghai",
					"only_send_to_tenant": true,
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
					"enabled":             true,
					"content_summary":     "Out today",
					"start_time":          "1786723200000",
					"end_time":            "1787068799999",
					"time_zone":           "Asia/Shanghai",
					"only_send_to_tenant": false,
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

	assertAutoReplyPayloadValue(t, captured, "enabled", true)
	assertAutoReplyPayloadValue(t, captured, "content_html", "<p>Out today</p>")
	assertAutoReplyPayloadValue(t, captured, "content_summary", "Out today")
	assertAutoReplyPayloadValue(t, captured, "start_time", "1786723200000")
	assertAutoReplyPayloadValue(t, captured, "end_time", "1787068799999")
	assertAutoReplyPayloadValue(t, captured, "time_zone", "Asia/Shanghai")
	assertAutoReplyPayloadValue(t, captured, "only_send_to_tenant", false)
	assertAutoReplyPayloadAbsent(t, captured, "auto_reply")
	assertAutoReplyPayloadAbsent(t, captured, "enable")
	assertAutoReplyPayloadAbsent(t, captured, "content")
	assertAutoReplyPayloadAbsent(t, captured, "timezone")
	assertAutoReplyPayloadAbsent(t, captured, "only_send_inner_sender")
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
					"enabled":             true,
					"start_time":          "1786723200000",
					"end_time":            "1787068799999",
					"time_zone":           "Asia/Shanghai",
					"only_send_to_tenant": true,
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

	assertAutoReplyPayloadValue(t, captured, "content_html", "<p>From file</p>")
	assertAutoReplyPayloadValue(t, captured, "content_summary", "From file")
	assertAutoReplyPayloadValue(t, captured, "enabled", true)
	assertAutoReplyPayloadValue(t, captured, "only_send_to_tenant", true)
	assertAutoReplyPayloadAbsent(t, captured, "auto_reply")
}

func TestMailAutoReplyEmbedsLocalImages(t *testing.T) {
	chdirTemp(t)
	png, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("logo.png", png, 0644); err != nil {
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
					"enabled":             true,
					"start_time":          "1786723200000",
					"end_time":            "1787068799999",
					"time_zone":           "Asia/Shanghai",
					"only_send_to_tenant": false,
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
			"data": map[string]interface{}{"auto_reply": map[string]interface{}{"content_summary": "With image"}},
		},
	})

	if err := runMountedMailShortcut(t, MailAutoReplyModify, []string{"+auto-reply-modify", "--content", `<p>Hi<img src="logo.png"></p>`, "--summary", "With image"}, f, stdout); err != nil {
		t.Fatalf("runMountedMailShortcut() error = %v", err)
	}
	reg.Verify(t)

	html, _ := captured["content_html"].(string)
	if !strings.Contains(html, `src="data:image/png;base64,`) {
		t.Fatalf("content_html should embed local image as data URI, got %q", html)
	}
	if strings.Contains(html, `src="logo.png"`) {
		t.Fatalf("local image path should have been replaced, got %q", html)
	}
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

func assertAutoReplyPayloadAbsent(t *testing.T, payload map[string]interface{}, key string) {
	t.Helper()
	if _, ok := payload[key]; ok {
		t.Fatalf("%s should be absent (payload=%#v)", key, payload)
	}
}
