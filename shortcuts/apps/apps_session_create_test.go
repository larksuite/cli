// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/httpmock"
)

func TestAppsSessionCreate_Success(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps/app_x/sessions",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"session_id": "conv_new"},
		},
	}
	reg.Register(stub)
	if err := runAppsShortcut(t, AppsSessionCreate,
		[]string{"+session-create", "--app-id", "app_x", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	if got := stdout.String(); !strings.Contains(got, `"session_id": "conv_new"`) {
		t.Fatalf("stdout missing session_id: %s", got)
	}
	if len(stub.CapturedBody) != 0 {
		t.Fatalf("+session-create must POST with no body, got: %s", stub.CapturedBody)
	}
}

func TestAppsSessionCreate_Pretty(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps/app_x/sessions",
		Body:   map[string]interface{}{"code": 0, "data": map[string]interface{}{"session_id": "conv_new"}},
	})
	if err := runAppsShortcut(t, AppsSessionCreate,
		[]string{"+session-create", "--app-id", "app_x", "--format", "pretty", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	if got := stdout.String(); !strings.Contains(got, "session created: conv_new") {
		t.Fatalf("pretty output wrong: %q", got)
	}
}

func TestAppsSessionCreate_RequiresAppID(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	// present-but-blank --app-id passes cobra MarkFlagRequired, caught by Validate hook.
	err := runAppsShortcut(t, AppsSessionCreate, []string{"+session-create", "--app-id", "", "--as", "user"}, factory, stdout)
	if err == nil || !strings.Contains(err.Error(), "app-id") {
		t.Fatalf("expected --app-id required error, got %v", err)
	}
}

func TestAppsSessionCreate_DryRun(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	if err := runAppsShortcut(t, AppsSessionCreate,
		[]string{"+session-create", "--app-id", "app_x", "--dry-run", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("dry-run err=%v", err)
	}
	if got := stdout.String(); !strings.Contains(got, "/open-apis/spark/v1/apps/app_x/sessions") {
		t.Fatalf("dry-run missing endpoint: %s", got)
	}
}

func TestAppsSessionCreate_EncodesAppID(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	if err := runAppsShortcut(t, AppsSessionCreate,
		[]string{"+session-create", "--app-id", "a/b", "--dry-run", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("dry-run err=%v", err)
	}
	if got := stdout.String(); strings.Contains(got, "apps/a/b/sessions") {
		t.Fatalf("app_id must be path-encoded, got raw slash: %s", got)
	}
}
