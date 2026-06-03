// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/httpmock"
)

func TestAppsChat_Success(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps/app_x/sessions/conv_x/chat",
		Body: map[string]interface{}{
			"code": 0,
			// v7.8: +chat is async and returns NO turn_id (turn not generated yet),
			// only next_poll_after_ms. turn_id is read later from +session-read.
			"data": map[string]interface{}{"next_poll_after_ms": 30000},
		},
	}
	reg.Register(stub)
	if err := runAppsShortcut(t, AppsChat,
		[]string{"+chat", "--app-id", "app_x", "--session-id", "conv_x", "--message", "把首页表头改成蓝色", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	var sent map[string]interface{}
	if err := json.Unmarshal(stub.CapturedBody, &sent); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if sent["message"] != "把首页表头改成蓝色" {
		t.Fatalf("body.message = %v", sent["message"])
	}
	if _, present := sent["attachment_ids"]; present {
		t.Fatalf("attachment_ids must not be sent this iteration: %v", sent)
	}
	if got := stdout.String(); !strings.Contains(got, `"next_poll_after_ms": 30000`) {
		t.Fatalf("stdout missing next_poll_after_ms: %s", got)
	}
}

func TestAppsChat_Pretty(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps/app_x/sessions/conv_x/chat",
		Body:   map[string]interface{}{"code": 0, "data": map[string]interface{}{"next_poll_after_ms": 30000}},
	})
	if err := runAppsShortcut(t, AppsChat,
		[]string{"+chat", "--app-id", "app_x", "--session-id", "conv_x", "--message", "hi", "--format", "pretty", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	if got := stdout.String(); !strings.Contains(got, "message sent") || !strings.Contains(got, "30000") {
		t.Fatalf("pretty wrong: %q", got)
	}
}

func TestAppsChat_RequiresMessage(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	err := runAppsShortcut(t, AppsChat,
		[]string{"+chat", "--app-id", "app_x", "--session-id", "conv_x", "--message", "", "--as", "user"}, factory, stdout)
	if err == nil || !strings.Contains(err.Error(), "message") {
		t.Fatalf("expected --message required error, got %v", err)
	}
}

// Security: a non-blank message that fails for another reason must never be echoed.
// Here we assert the blank-message error names the field only (no content leak path).
func TestAppsChat_ValidationDoesNotEchoMessage(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	// blank message triggers validation; the error must mention the flag, not any content.
	err := runAppsShortcut(t, AppsChat,
		[]string{"+chat", "--app-id", "", "--session-id", "conv_x", "--message", "secret-content-xyz", "--as", "user"}, factory, stdout)
	if err == nil {
		t.Fatalf("expected validation error")
	}
	if strings.Contains(err.Error(), "secret-content-xyz") {
		t.Fatalf("validation error must not echo --message content: %v", err)
	}
}

func TestAppsChat_DryRun(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	if err := runAppsShortcut(t, AppsChat,
		[]string{"+chat", "--app-id", "app_x", "--session-id", "conv_x", "--message", "hi", "--dry-run", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("dry-run err=%v", err)
	}
	got := stdout.String()
	if !strings.Contains(got, "/open-apis/spark/v1/apps/app_x/sessions/conv_x/chat") {
		t.Fatalf("dry-run missing endpoint: %s", got)
	}
	if !strings.Contains(got, `"message": "hi"`) {
		t.Fatalf("dry-run missing message body: %s", got)
	}
}
