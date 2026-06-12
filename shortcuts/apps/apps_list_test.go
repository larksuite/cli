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

func TestAppsList_WithKeywordOwnershipAppType(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/spark/v1/apps?app_type=html&keyword=%E9%97%AE%E5%8D%B7&ownership=mine&page_size=20",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"items": []interface{}{}, "has_more": false},
		},
	})
	if err := runAppsShortcut(t, AppsList,
		[]string{"+list", "--keyword", "问卷", "--ownership", "mine", "--app-type", "html", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
}

func TestAppsList_InvalidOwnership(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	err := runAppsShortcut(t, AppsList,
		[]string{"+list", "--ownership", "bogus", "--as", "user"}, factory, stdout)
	if err == nil {
		t.Fatalf("expected enum validation error for --ownership bogus")
	}
}

func TestAppsList_InvalidAppType(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	err := runAppsShortcut(t, AppsList,
		[]string{"+list", "--app-type", "HTML", "--as", "user"}, factory, stdout)
	if err == nil {
		t.Fatalf("expected enum validation error for --app-type HTML (hard cut to lowercase)")
	}
}

func TestAppsList_DryRunWithFilters(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	if err := runAppsShortcut(t, AppsList,
		[]string{"+list", "--keyword", "q", "--ownership", "all", "--app-type", "full_stack", "--dry-run", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("dry-run err=%v", err)
	}
	got := stdout.String()
	for _, want := range []string{"keyword", "ownership", "app_type", "full_stack"} {
		if !strings.Contains(got, want) {
			t.Fatalf("dry-run missing %q: %s", want, got)
		}
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

func TestAppsList_TrimsIconURLAndCreatedAt(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/spark/v1/apps?page_size=20",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{
						"app_id":       "app_x",
						"name":         "Trim Me",
						"is_published": true,
						"online_url":   "https://example.com/spark/faas/app_x",
						"updated_at":   "2026-05-28T10:05:16Z",
						"created_at":   "2026-05-01T08:00:00Z",
						"icon_url":     "https://example.com/icon.png",
						"description":  "An app to test trimming",
					},
				},
				"page_token": "next_cursor",
				"has_more":   true,
			},
		},
	})

	if err := runAppsShortcut(t, AppsList, []string{"+list", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	got := stdout.String()
	for _, drop := range []string{"icon_url", "created_at"} {
		if strings.Contains(got, drop) {
			t.Fatalf("default output should not contain %q:\n%s", drop, got)
		}
	}
	for _, keep := range []string{"app_id", "name", "is_published", "online_url", "updated_at", "description"} {
		if !strings.Contains(got, keep) {
			t.Fatalf("default output missing %q:\n%s", keep, got)
		}
	}
}

func TestAppsList_PrettyShowsPublishFields(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/spark/v1/apps?page_size=20",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{
						"app_id":       "app_pub",
						"name":         "Published App",
						"is_published": true,
						"online_url":   "https://example.com/spark/faas/app_pub",
						"updated_at":   "2026-05-28T10:05:16Z",
					},
					map[string]interface{}{
						"app_id":       "app_draft",
						"name":         "Draft App",
						"is_published": false,
						"updated_at":   "2026-05-31T12:31:27Z",
					},
				},
				"has_more": false,
			},
		},
	})

	if err := runAppsShortcut(t, AppsList,
		[]string{"+list", "--format", "pretty", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	got := stdout.String()
	for _, want := range []string{"is_published", "online_url", "https://example.com/spark/faas/app_pub", "true", "false"} {
		if !strings.Contains(got, want) {
			t.Fatalf("pretty output missing %q:\n%s", want, got)
		}
	}
}
