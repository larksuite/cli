// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/httpmock"
)

func TestAppsGet_Success(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/spark/v1/apps/app_test",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"app": map[string]interface{}{
					"app_id":       "app_test",
					"app_type":     "html",
					"name":         "TestApp",
					"description":  "A test application",
					"icon_url":     "https://example.com/icon.svg",
					"is_published": true,
					"created_at":   "2026-05-18T10:00:00Z",
					"updated_at":   "2026-06-01T12:00:00Z",
				},
			},
		},
	})

	if err := runAppsShortcut(t, AppsGet,
		[]string{"+get", "--app-id", "app_test", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	got := stdout.String()
	if !strings.Contains(got, "app_test") {
		t.Fatalf("stdout missing app_id: %s", got)
	}
	if !strings.Contains(got, "html") {
		t.Fatalf("stdout missing app_type: %s", got)
	}
}

func TestAppsGet_RequiresAppID(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	err := runAppsShortcut(t, AppsGet,
		[]string{"+get", "--as", "user"}, factory, stdout)
	if err == nil || !strings.Contains(err.Error(), "app-id") {
		t.Fatalf("expected app-id required error, got %v", err)
	}
}

func TestAppsGet_EmptyAppID(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	err := runAppsShortcut(t, AppsGet,
		[]string{"+get", "--app-id", "", "--as", "user"}, factory, stdout)
	requireAppsValidationProblem(t, err)
}

func TestAppsGet_DryRun(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	if err := runAppsShortcut(t, AppsGet,
		[]string{"+get", "--app-id", "app_test", "--dry-run", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("dry-run err=%v", err)
	}
	got := stdout.String()
	if !strings.Contains(got, "/open-apis/spark/v1/apps/app_test") {
		t.Fatalf("dry-run missing API path: %s", got)
	}
}

func TestAppsGet_PrettyOutput(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/spark/v1/apps/app_test",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"app": map[string]interface{}{
					"app_id":       "app_test",
					"app_type":     "html",
					"name":         "PrettyApp",
					"description":  "A pretty test app",
					"is_published": true,
					"updated_at":   "2026-06-01T12:00:00Z",
				},
			},
		},
	})

	if err := runAppsShortcut(t, AppsGet,
		[]string{"+get", "--app-id", "app_test", "--format", "pretty", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	got := stdout.String()
	for _, want := range []string{"app_id:", "app_type:", "name:", "is_published:"} {
		if !strings.Contains(got, want) {
			t.Fatalf("pretty output missing %q: %s", want, got)
		}
	}
}
