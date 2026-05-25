// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/httpmock"
)

func TestAppsList_FirstPage(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	stub := &httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/spark/v1/apps?page_size=20",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{"app_id": "app_a", "name": "Alpha", "updated_at": "2026-05-18T10:00:00Z"},
					map[string]interface{}{"app_id": "app_b", "name": "Beta", "updated_at": "2026-05-18T09:00:00Z"},
				},
				"page_token": "next_cursor",
				"has_more":   true,
			},
		},
	}
	reg.Register(stub)

	if err := runAppsShortcut(t, AppsList, []string{"+list", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	got := stdout.String()
	if !strings.Contains(got, "app_a") || !strings.Contains(got, "app_b") {
		t.Fatalf("output missing items: %s", got)
	}
	if !strings.Contains(got, "Alpha") || !strings.Contains(got, "Beta") {
		t.Fatalf("output missing item names: %s", got)
	}
}

func TestAppsList_WithPageToken(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	stub := &httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/spark/v1/apps?page_size=50&page_token=cursor_abc",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"items":    []interface{}{},
				"has_more": false,
			},
		},
	}
	reg.Register(stub)

	if err := runAppsShortcut(t, AppsList,
		[]string{"+list", "--page-size", "50", "--page-token", "cursor_abc", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
}

func TestAppsList_DryRun(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	if err := runAppsShortcut(t, AppsList,
		[]string{"+list", "--dry-run", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("dry-run err=%v", err)
	}
	got := stdout.String()
	if !strings.Contains(got, "/open-apis/spark/v1/apps") {
		t.Fatalf("dry-run missing endpoint: %s", got)
	}
	if !strings.Contains(got, "page_size") {
		t.Fatalf("dry-run missing page_size param: %s", got)
	}
}
