// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/httpmock"
)

func TestAppsAccessScopeGet_Specific(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/spark/v1/apps/app_x/access-scope",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"scope":       "Range",
				"users":       []interface{}{"ou_x", "ou_y"},
				"departments": []interface{}{"od_z"},
				"chats":       []interface{}{"oc_g"},
				"apply_config": map[string]interface{}{
					"enabled":   true,
					"approvers": []interface{}{"ou_appr"},
				},
			},
		},
	})

	if err := runAppsShortcut(t, AppsAccessScopeGet,
		[]string{"+access-scope-get", "--app-id", "app_x", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	got := stdout.String()
	if !strings.Contains(got, `"scope": "Range"`) {
		t.Fatalf("scope string not preserved (expect raw \"Range\"): %s", got)
	}
	if !strings.Contains(got, `"ou_x"`) || !strings.Contains(got, `"od_z"`) || !strings.Contains(got, `"oc_g"`) {
		t.Fatalf("users/departments/chats fields missing in envelope: %s", got)
	}
	if !strings.Contains(got, `"ou_appr"`) {
		t.Fatalf("apply_config.approvers missing: %s", got)
	}
}

func TestAppsAccessScopeGet_Public(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/spark/v1/apps/app_x/access-scope",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"scope": "All", "require_login": false},
		},
	})

	if err := runAppsShortcut(t, AppsAccessScopeGet,
		[]string{"+access-scope-get", "--app-id", "app_x", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	got := stdout.String()
	if !strings.Contains(got, `"scope": "All"`) {
		t.Fatalf("scope=All missing: %s", got)
	}
	if !strings.Contains(got, `"require_login": false`) {
		t.Fatalf("require_login missing: %s", got)
	}
}

func TestAppsAccessScopeGet_Tenant(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/spark/v1/apps/app_x/access-scope",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"scope": "Tenant"},
		},
	})

	if err := runAppsShortcut(t, AppsAccessScopeGet,
		[]string{"+access-scope-get", "--app-id", "app_x", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	if !strings.Contains(stdout.String(), `"scope": "Tenant"`) {
		t.Fatalf("scope=Tenant missing: %s", stdout.String())
	}
}

func TestAppsAccessScopeGet_RequiresAppID(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	err := runAppsShortcut(t, AppsAccessScopeGet,
		[]string{"+access-scope-get", "--as", "user"}, factory, stdout)
	if err == nil || !strings.Contains(err.Error(), "app-id") {
		t.Fatalf("expected --app-id required, got %v", err)
	}
}

func TestAppsAccessScopeGet_TrimsAppIDInPath(t *testing.T) {
	// 与 +update 的 D1.2 修复对称：URL 拼接前必须 TrimSpace(app-id)，
	// 否则 " app_x " 会被 EncodePathSegment 编码进 path segment 出现空格转义。
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/spark/v1/apps/app_x/access-scope",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"scope": "Tenant"},
		},
	})

	if err := runAppsShortcut(t, AppsAccessScopeGet,
		[]string{"+access-scope-get", "--app-id", "  app_x  ", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
}
