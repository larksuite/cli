// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/httpmock"
)

func TestAppsSessionStop_Success(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps/app_x/sessions/conv_x/stop",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"stopped": true, "message": "running turn stopped"},
		},
	}
	reg.Register(stub)
	if err := runAppsShortcut(t, AppsSessionStop,
		[]string{"+session-stop", "--app-id", "app_x", "--session-id", "conv_x", "--turn-id", "8421374923", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	var sent map[string]interface{}
	if err := json.Unmarshal(stub.CapturedBody, &sent); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if sent["turn_id"] != "8421374923" {
		t.Fatalf("body.turn_id = %v", sent["turn_id"])
	}
}

func TestAppsSessionStop_PrettyStopped(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps/app_x/sessions/conv_x/stop",
		Body:   map[string]interface{}{"code": 0, "data": map[string]interface{}{"stopped": true, "message": "running turn stopped"}},
	})
	if err := runAppsShortcut(t, AppsSessionStop,
		[]string{"+session-stop", "--app-id", "app_x", "--session-id", "conv_x", "--turn-id", "8421374923", "--format", "pretty", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	if got := stdout.String(); !strings.Contains(got, "stopped turn 8421374923") {
		t.Fatalf("pretty stopped wrong: %q", got)
	}
}

func TestAppsSessionStop_PrettyNoOp(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps/app_x/sessions/conv_x/stop",
		Body:   map[string]interface{}{"code": 0, "data": map[string]interface{}{"stopped": false, "message": "turn already completed"}},
	})
	if err := runAppsShortcut(t, AppsSessionStop,
		[]string{"+session-stop", "--app-id", "app_x", "--session-id", "conv_x", "--turn-id", "t1", "--format", "pretty", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	if got := stdout.String(); !strings.Contains(got, "no-op") || !strings.Contains(got, "completed") {
		t.Fatalf("pretty no-op wrong: %q", got)
	}
}

func TestAppsSessionStop_RequiresTurnID(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	err := runAppsShortcut(t, AppsSessionStop,
		[]string{"+session-stop", "--app-id", "app_x", "--session-id", "conv_x", "--turn-id", "", "--as", "user"}, factory, stdout)
	if err == nil || !strings.Contains(err.Error(), "turn-id") {
		t.Fatalf("expected --turn-id required error, got %v", err)
	}
}

func TestAppsSessionStop_DryRun(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	if err := runAppsShortcut(t, AppsSessionStop,
		[]string{"+session-stop", "--app-id", "app_x", "--session-id", "conv_x", "--turn-id", "t1", "--dry-run", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("dry-run err=%v", err)
	}
	got := stdout.String()
	if !strings.Contains(got, "/open-apis/spark/v1/apps/app_x/sessions/conv_x/stop") {
		t.Fatalf("dry-run missing endpoint: %s", got)
	}
	if !strings.Contains(got, `"turn_id": "t1"`) {
		t.Fatalf("dry-run missing turn_id body: %s", got)
	}
}

// Encoding safeguard for the shared sessionPath helper (reused from Task 3).
func TestAppsSessionStop_EncodesPathSegments(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	if err := runAppsShortcut(t, AppsSessionStop,
		[]string{"+session-stop", "--app-id", "a/b", "--session-id", "c/d", "--turn-id", "t1", "--dry-run", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("dry-run err=%v", err)
	}
	got := stdout.String()
	if strings.Contains(got, "apps/a/b/sessions") || strings.Contains(got, "sessions/c/d/stop") {
		t.Fatalf("path segments must be encoded, got raw slash: %s", got)
	}
}
