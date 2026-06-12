// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/httpmock"
)

func TestAppsSessionList_Success(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/spark/v1/apps/app_x/sessions",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"sessions": []interface{}{
					map[string]interface{}{
						"session_id": "conv_a", "name": "建后台", "is_active": true,
						"created_at": "2026-05-28T10:00:00Z", "updated_at": "2026-05-28T11:00:00Z",
					},
				},
				"next_page_token": "",
				"has_more":        false,
			},
		},
	})
	if err := runAppsShortcut(t, AppsSessionList,
		[]string{"+session-list", "--app-id", "app_x", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	if got := stdout.String(); !strings.Contains(got, `"session_id": "conv_a"`) {
		t.Fatalf("stdout missing session: %s", got)
	}
}

func TestAppsSessionList_TableShowsKeyColumns(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/spark/v1/apps/app_x/sessions",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"sessions": []interface{}{
					map[string]interface{}{"session_id": "conv_a", "name": "建后台", "is_active": true, "updated_at": "2026-05-28T11:00:00Z"},
				},
			},
		},
	})
	if err := runAppsShortcut(t, AppsSessionList,
		[]string{"+session-list", "--app-id", "app_x", "--format", "table", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	got := stdout.String()
	if !strings.Contains(got, "conv_a") || !strings.Contains(got, "建后台") {
		t.Fatalf("table missing key columns: %s", got)
	}
}

func TestAppsSessionList_PassesPagination(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	if err := runAppsShortcut(t, AppsSessionList,
		[]string{"+session-list", "--app-id", "app_x", "--page-size", "50", "--page-token", "tok1", "--dry-run", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("dry-run err=%v", err)
	}
	got := stdout.String()
	if !strings.Contains(got, "page_size") || !strings.Contains(got, "50") {
		t.Fatalf("dry-run missing page_size: %s", got)
	}
	if !strings.Contains(got, "tok1") {
		t.Fatalf("dry-run missing page_token: %s", got)
	}
}

func TestAppsSessionList_RequiresAppID(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	err := runAppsShortcut(t, AppsSessionList, []string{"+session-list", "--app-id", "", "--as", "user"}, factory, stdout)
	if err == nil || !strings.Contains(err.Error(), "app-id") {
		t.Fatalf("expected --app-id required error, got %v", err)
	}
}
