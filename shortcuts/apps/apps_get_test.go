// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
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
	closeStatus := "close"
	emptyStatus := ""
	for _, tc := range []struct {
		name        string
		badgeStatus *string
		wantBadge   string
	}{
		{name: "with value", badgeStatus: &closeStatus, wantBadge: "badge_status: close\n"},
		{name: "missing field"},
		{name: "empty value", badgeStatus: &emptyStatus},
	} {
		t.Run(tc.name, func(t *testing.T) {
			factory, stdout, reg := newAppsExecuteFactory(t)
			app := map[string]interface{}{
				"app_id":       "app_test",
				"app_type":     "html",
				"name":         "PrettyApp",
				"description":  "A pretty test app",
				"is_published": true,
				"updated_at":   "2026-06-01T12:00:00Z",
			}
			if tc.badgeStatus != nil {
				app["badge_status"] = *tc.badgeStatus
			}
			reg.Register(&httpmock.Stub{
				Method: "GET",
				URL:    "/open-apis/spark/v1/apps/app_test",
				Body: map[string]interface{}{
					"code": 0,
					"data": map[string]interface{}{"app": app},
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
			if tc.wantBadge != "" && !strings.Contains(got, tc.wantBadge) {
				t.Fatalf("pretty output missing %q: %s", tc.wantBadge, got)
			}
			if tc.wantBadge == "" && strings.Contains(got, "badge_status:") {
				t.Fatalf("pretty output must omit an absent or empty badge_status: %s", got)
			}
		})
	}
}

func TestAppsGet_PrettyOutputIncludesIconURL(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/spark/v1/apps/app_test",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"app": map[string]interface{}{
					"app_id":   "app_test",
					"app_type": "full_stack",
					"name":     "IconApp",
					"icon_url": "https://example.com/icon.png",
				},
			},
		},
	})

	if err := runAppsShortcut(t, AppsGet,
		[]string{"+get", "--app-id", "app_test", "--format", "pretty", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	if got := stdout.String(); !strings.Contains(got, "icon_url: https://example.com/icon.png\n") {
		t.Fatalf("pretty output missing icon_url: %s", got)
	}
}

func TestAppsGet_InvalidBadgeStatusTypeReturnsTypedError(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/spark/v1/apps/app_test",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"app": map[string]interface{}{
					"app_id":       "app_test",
					"badge_status": 1,
				},
			},
		},
	})

	err := runAppsShortcut(t, AppsGet,
		[]string{"+get", "--app-id", "app_test", "--as", "user"},
		factory, stdout)
	var internalErr *errs.InternalError
	if !errors.As(err, &internalErr) || internalErr.Subtype != errs.SubtypeInvalidResponse {
		t.Fatalf("error = %T %v, want internal/invalid_response", err, err)
	}
	var decodeErr *json.UnmarshalTypeError
	if !errors.As(err, &decodeErr) || decodeErr.Field != "app.badge_status" {
		t.Fatalf("cause = %T %v, want json.UnmarshalTypeError for app.badge_status", decodeErr, decodeErr)
	}
	if stdout.Len() != 0 {
		t.Fatalf("invalid response must not emit a success result, got %q", stdout.String())
	}
}

func TestAppsGet_MetaDocTypeJSONPassthrough(t *testing.T) {
	for _, value := range []string{"apps", "miaoda_apps", ""} {
		for _, format := range []string{"json", "pretty"} {
			t.Run(value+"/"+format, func(t *testing.T) {
				factory, stdout, reg := newAppsExecuteFactory(t)
				app := map[string]interface{}{"app_id": "app_test", "meta_token": "metaTok"}
				if value != "" {
					app["meta_doc_type"] = value
				}
				reg.Register(&httpmock.Stub{Method: "GET", URL: "/open-apis/spark/v1/apps/app_test", Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{"app": app}}})
				if err := runAppsShortcut(t, AppsGet, []string{"+get", "--app-id", "app_test", "--format", format, "--as", "user"}, factory, stdout); err != nil {
					t.Fatal(err)
				}
				if format == "pretty" {
					if strings.Contains(stdout.String(), "meta_doc_type:") {
						t.Fatalf("pretty output must not add meta_doc_type: %s", stdout.String())
					}
				} else {
					var envelope struct {
						Data struct {
							App map[string]interface{} `json:"app"`
						} `json:"data"`
					}
					if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
						t.Fatal(err)
					}
					got, exists := envelope.Data.App["meta_doc_type"]
					if (value == "" && exists) || (value != "" && got != value) {
						t.Fatalf("meta_doc_type = %v (exists %v), want %q", got, exists, value)
					}
					if envelope.Data.App["meta_token"] != "metaTok" {
						t.Fatal("meta_token lost")
					}
				}
			})
		}
	}
}
