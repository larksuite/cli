// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/httpmock"
)

func sessionGetStub() *httpmock.Stub {
	return &httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/spark/v1/apps/app_x/sessions/conv_x",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"session_id":         "conv_x",
				"is_active":          true,
				"is_streaming":       true,
				"summary":            "正在补充...",
				"queued_count":       1,
				"latest_turn":        map[string]interface{}{"turn_id": "8421374923", "status": "running"},
				"next_poll_after_ms": 30000,
			},
		},
	}
}

func TestAppsSessionGet_Success(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(sessionGetStub())
	if err := runAppsShortcut(t, AppsSessionGet,
		[]string{"+session-get", "--app-id", "app_x", "--session-id", "conv_x", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	if got := stdout.String(); !strings.Contains(got, `"is_streaming": true`) {
		t.Fatalf("stdout missing is_streaming: %s", got)
	}
}

func TestAppsSessionGet_PrettyReadsNestedSnakeCase(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(sessionGetStub())
	if err := runAppsShortcut(t, AppsSessionGet,
		[]string{"+session-get", "--app-id", "app_x", "--session-id", "conv_x", "--format", "pretty", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	got := stdout.String()
	if !strings.Contains(got, "8421374923") || !strings.Contains(got, "running") {
		t.Fatalf("pretty must read latest_turn.turn_id/status: %s", got)
	}
	if !strings.Contains(got, "30000") {
		t.Fatalf("pretty must show next_poll_after_ms: %s", got)
	}
}

func TestAppsSessionGet_RequiresFlags(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	err := runAppsShortcut(t, AppsSessionGet, []string{"+session-get", "--app-id", "app_x", "--session-id", "", "--as", "user"}, factory, stdout)
	if err == nil || !strings.Contains(err.Error(), "session-id") {
		t.Fatalf("expected --session-id required error, got %v", err)
	}
}

func TestAppsSessionGet_DryRun(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	if err := runAppsShortcut(t, AppsSessionGet,
		[]string{"+session-get", "--app-id", "app_x", "--session-id", "conv_x", "--dry-run", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("dry-run err=%v", err)
	}
	if got := stdout.String(); !strings.Contains(got, "/open-apis/spark/v1/apps/app_x/sessions/conv_x") {
		t.Fatalf("dry-run missing endpoint: %s", got)
	}
}
