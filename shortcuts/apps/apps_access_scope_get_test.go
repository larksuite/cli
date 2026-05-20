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
				"scope":       3,
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
	if !strings.Contains(got, `"scope": 3`) {
		t.Fatalf("scope int not preserved (expect raw 3): %s", got)
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
			"data": map[string]interface{}{"scope": 1, "require_login": false},
		},
	})

	if err := runAppsShortcut(t, AppsAccessScopeGet,
		[]string{"+access-scope-get", "--app-id", "app_x", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	got := stdout.String()
	if !strings.Contains(got, `"scope": 1`) {
		t.Fatalf("scope=1 missing: %s", got)
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
			"data": map[string]interface{}{"scope": 2},
		},
	})

	if err := runAppsShortcut(t, AppsAccessScopeGet,
		[]string{"+access-scope-get", "--app-id", "app_x", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	if !strings.Contains(stdout.String(), `"scope": 2`) {
		t.Fatalf("scope=2 missing: %s", stdout.String())
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
