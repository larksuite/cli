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
		"--mailbox", "user@example.com",
		"--enable",
		"--content", "<p>Out today</p>",
		"--start", "2026-08-15T09:00:00+08:00",
		"--end", "2026-08-18T09:00:00+08:00",
		"--timezone", "Asia/Shanghai",
		"--all",
		"--format", "json",
	}, f, stdout)
	if err != nil {
		t.Fatalf("runMountedMailShortcut() error = %v", err)
	}
	reg.Verify(t)

	assertAutoReplyPayloadValue(t, captured, "enabled", true)
	assertAutoReplyPayloadValue(t, captured, "content_html", "<p>Out today</p>")
	assertAutoReplyPayloadAbsent(t, captured, "content_summary")
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
	assertAutoReplyPayloadAbsent(t, captured, "content_summary")
	assertAutoReplyPayloadValue(t, captured, "enabled", true)
	assertAutoReplyPayloadValue(t, captured, "only_send_to_tenant", true)
	assertAutoReplyPayloadAbsent(t, captured, "auto_reply")
}

func TestMailAutoReplyUploadsLocalImages(t *testing.T) {
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
		Method: "POST",
		URL:    "/open-apis/drive/v1/medias/upload_all",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"file_token": "file_logo"},
		},
	})
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

	if err := runMountedMailShortcut(t, MailAutoReplyModify, []string{"+auto-reply-modify", "--content", `<p>Hi<img src="logo.png"></p>`}, f, stdout); err != nil {
		t.Fatalf("runMountedMailShortcut() error = %v", err)
	}
	reg.Verify(t)

	html, _ := captured["content_html"].(string)
	if !strings.Contains(html, `src="cid:`) {
		t.Fatalf("content_html should reference an uploaded image CID, got %q", html)
	}
	if strings.Contains(html, `src="logo.png"`) {
		t.Fatalf("local image path should have been replaced, got %q", html)
	}
	assertAutoReplyPayloadAbsent(t, captured, "content_summary")
	images, ok := captured["images"].([]interface{})
	if !ok || len(images) != 1 {
		t.Fatalf("images = %#v, want one uploaded image", captured["images"])
	}
	image := images[0].(map[string]interface{})
	if image["file_key"] != "file_logo" || image["image_name"] != "logo.png" {
		t.Fatalf("image metadata = %#v", image)
	}
	if cid, _ := image["cid"].(string); cid == "" || !strings.Contains(html, "cid:"+cid) {
		t.Fatalf("image cid = %q, html = %q", cid, html)
	}
}

func TestMailAutoReplyHydratesImagesIndependently(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    mailboxPath("me", "settings", "auto_reply"),
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"auto_reply": map[string]interface{}{
				"content_html": `<p>Hi<img src="cid:ok"><img src="cid:bad"></p>`,
				"images": []interface{}{
					map[string]interface{}{"cid": "ok", "image_name": "ok.png", "file_key": "file_ok", "file_size": 3},
					map[string]interface{}{"cid": "bad", "image_name": "bad.png", "file_key": "file_bad", "file_size": 4},
				},
			}},
		},
	})
	reg.Register(&httpmock.Stub{Method: "GET", URL: "/open-apis/drive/v1/medias/file_ok/download", RawBody: []byte("png"), ContentType: "image/png"})
	reg.Register(&httpmock.Stub{Method: "GET", URL: "/open-apis/drive/v1/medias/file_bad/download", Status: 404, Body: "missing"})

	if err := runMountedMailShortcut(t, MailAutoReply, []string{"+auto-reply", "--format", "json"}, f, stdout); err != nil {
		t.Fatalf("runMountedMailShortcut() error = %v", err)
	}
	autoReply := decodeShortcutEnvelopeData(t, stdout)["auto_reply"].(map[string]interface{})
	if autoReply["content"] == nil || autoReply["content_html"] != nil {
		t.Fatalf("content projection = %#v", autoReply)
	}
	images := autoReply["images"].([]interface{})
	first := images[0].(map[string]interface{})
	second := images[1].(map[string]interface{})
	if first["data"] != base64.StdEncoding.EncodeToString([]byte("png")) || first["file_key"] != nil {
		t.Fatalf("first image = %#v", first)
	}
	if second["error"] == nil || second["file_key"] != nil {
		t.Fatalf("second image = %#v", second)
	}
}

func TestMailAutoReplyContentFileRequiresCurrentDirectory(t *testing.T) {
	for _, path := range []string{"nested/auto_reply.html", "../auto_reply.html", "/tmp/auto_reply.html"} {
		t.Run(path, func(t *testing.T) {
			f, stdout, _, _ := mailShortcutTestFactory(t)
			err := runMountedMailShortcut(t, MailAutoReplyModify, []string{"+auto-reply-modify", "--content-file", path}, f, stdout)
			if err == nil {
				t.Fatal("expected content-file path validation error")
			}
			if !strings.Contains(err.Error(), "--content-file must be a file in the current directory") {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestMailAutoReplyRejectsUnsafeHTML(t *testing.T) {
	for _, content := range []string{
		`<p>away<script>alert(1)</script></p>`,
		`<p>away<img src="logo.png" onerror="alert(1)"></p>`,
		`<a href="javascript:alert(1)">click</a>`,
		"<a href=\"java\nscript:alert(1)\">click</a>",
		`<p style="background:url(javascript:alert(1))">away</p>`,
	} {
		t.Run(content, func(t *testing.T) {
			f, stdout, _, _ := mailShortcutTestFactory(t)
			err := runMountedMailShortcut(t, MailAutoReplyModify, []string{"+auto-reply-modify", "--content", content}, f, stdout)
			if err == nil {
				t.Fatal("expected unsafe html validation error")
			}
			if !strings.Contains(err.Error(), "contains unsafe html") {
				t.Fatalf("unexpected error: %v", err)
			}
		})
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

func TestMailAutoReplyInvalidTimezoneIsReportedBeforeStartParse(t *testing.T) {
	for _, args := range [][]string{
		{"+auto-reply-modify", "--timezone", "Mars/Olympus"},
		{"+auto-reply-modify", "--start", "2026-08-19T00:00:00", "--timezone", "Mars/Olympus"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			f, stdout, _, _ := mailShortcutTestFactory(t)
			err := runMountedMailShortcut(t, MailAutoReplyModify, args, f, stdout)
			if err == nil {
				t.Fatal("expected timezone validation error")
			}
			if !strings.Contains(err.Error(), `invalid --timezone "Mars/Olympus"`) {
				t.Fatalf("unexpected error: %v", err)
			}
			if strings.Contains(err.Error(), "--start must be Unix seconds or ISO 8601") {
				t.Fatalf("timezone error should not be wrapped as a start parse error: %v", err)
			}
		})
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
