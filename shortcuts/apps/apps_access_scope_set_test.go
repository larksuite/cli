// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
)

func testRuntimeAccessScope(t *testing.T, scope, targets, approver string, applyEnabled, requireLogin bool) *common.RuntimeContext {
	t.Helper()
	cmd := &cobra.Command{Use: "access-scope-set"}
	cmd.Flags().String("scope", scope, "")
	cmd.Flags().String("targets", targets, "")
	cmd.Flags().String("approver", approver, "")
	cmd.Flags().Bool("apply-enabled", applyEnabled, "")
	cmd.Flags().Bool("require-login", requireLogin, "")
	return common.TestNewRuntimeContext(cmd, nil)
}

func TestBuildAccessScopeBody_Branches(t *testing.T) {
	t.Run("invalid scope", func(t *testing.T) {
		if _, err := buildAccessScopeBody(testRuntimeAccessScope(t, "bogus", "", "", false, false)); err == nil {
			t.Error("unknown scope must error")
		}
	})
	t.Run("specific with all target kinds and approver", func(t *testing.T) {
		body, err := buildAccessScopeBody(testRuntimeAccessScope(t,
			"specific",
			`[{"type":"user","id":"u1"},{"type":"department","id":"d1"},{"type":"chat","id":"c1"}]`,
			"ou_appr", true, false))
		if err != nil {
			t.Fatalf("err=%v", err)
		}
		if body["scope"] != "Range" {
			t.Errorf("scope=%v want Range", body["scope"])
		}
		for _, k := range []string{"users", "departments", "chats", "apply_config"} {
			if _, ok := body[k]; !ok {
				t.Errorf("missing %q in body=%v", k, body)
			}
		}
	})
	t.Run("specific with invalid targets JSON", func(t *testing.T) {
		if _, err := buildAccessScopeBody(testRuntimeAccessScope(t, "specific", "{bad", "", false, false)); err == nil {
			t.Error("invalid targets JSON must error")
		}
	})
	t.Run("public sets require_login", func(t *testing.T) {
		body, err := buildAccessScopeBody(testRuntimeAccessScope(t, "public", "", "", false, true))
		if err != nil {
			t.Fatalf("err=%v", err)
		}
		if body["scope"] != "All" || body["require_login"] != true {
			t.Errorf("public body=%v", body)
		}
	})
}

func TestAppsAccessScopeSet_Specific(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	stub := &httpmock.Stub{
		Method: "PUT",
		URL:    "/open-apis/spark/v1/apps/app_x/access-scope",
		Body:   map[string]interface{}{"code": 0, "data": map[string]interface{}{}},
	}
	reg.Register(stub)

	if err := runAppsShortcut(t, AppsAccessScopeSet, []string{
		"+access-scope-set",
		"--app-id", "app_x",
		"--scope", "specific",
		"--targets", `[{"type":"user","id":"ou_xxx"},{"type":"chat","id":"oc_xxx"}]`,
		"--apply-enabled",
		"--approver", "ou_yyy",
		"--as", "user",
	}, factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}

	var sent map[string]interface{}
	if err := json.Unmarshal(stub.CapturedBody, &sent); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	// 新协议：scope 是 string 枚举 (specific=Range)，targets 拆成 users/departments/chats
	if got, _ := sent["scope"].(string); got != "Range" {
		t.Fatalf("scope = %v, want %q", sent["scope"], "Range")
	}
	if _, present := sent["targets"]; present {
		t.Fatalf("legacy 'targets' field should not be sent: %v", sent)
	}
	users, _ := sent["users"].([]interface{})
	if len(users) != 1 || users[0] != "ou_xxx" {
		t.Fatalf("users = %v, want [ou_xxx]", sent["users"])
	}
	chats, _ := sent["chats"].([]interface{})
	if len(chats) != 1 || chats[0] != "oc_xxx" {
		t.Fatalf("chats = %v, want [oc_xxx]", sent["chats"])
	}
	if _, present := sent["departments"]; present {
		t.Fatalf("departments should be omitted when empty: %v", sent)
	}
}

func TestAppsAccessScopeSet_Public(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "PUT",
		URL:    "/open-apis/spark/v1/apps/app_x/access-scope",
		Body:   map[string]interface{}{"code": 0, "data": map[string]interface{}{}},
	})

	if err := runAppsShortcut(t, AppsAccessScopeSet, []string{
		"+access-scope-set",
		"--app-id", "app_x",
		"--scope", "public",
		"--require-login=false",
		"--as", "user",
	}, factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
}

func TestAppsAccessScopeSet_Tenant(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "PUT",
		URL:    "/open-apis/spark/v1/apps/app_x/access-scope",
		Body:   map[string]interface{}{"code": 0, "data": map[string]interface{}{}},
	})

	if err := runAppsShortcut(t, AppsAccessScopeSet, []string{
		"+access-scope-set",
		"--app-id", "app_x",
		"--scope", "tenant",
		"--as", "user",
	}, factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
}

func TestAppsAccessScopeSet_SpecificRequiresTargets(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	err := runAppsShortcut(t, AppsAccessScopeSet, []string{
		"+access-scope-set", "--app-id", "app_x", "--scope", "specific", "--as", "user",
	}, factory, stdout)
	if err == nil || !strings.Contains(err.Error(), "targets") {
		t.Fatalf("expected targets required error, got %v", err)
	}
}

func TestAppsAccessScopeSet_TenantRejectsExtraFlags(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	err := runAppsShortcut(t, AppsAccessScopeSet, []string{
		"+access-scope-set", "--app-id", "app_x", "--scope", "tenant",
		"--targets", `[]`, "--as", "user",
	}, factory, stdout)
	if err == nil {
		t.Fatalf("expected error when --targets passed with scope=tenant")
	}
}

func TestAppsAccessScopeSet_RejectsBadTargetType(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	err := runAppsShortcut(t, AppsAccessScopeSet, []string{
		"+access-scope-set", "--app-id", "app_x",
		"--scope", "specific",
		"--targets", `[{"type":"group","id":"oc_xxx"}]`,
		"--as", "user",
	}, factory, stdout)
	if err == nil || !strings.Contains(err.Error(), "type") {
		t.Fatalf("expected bad target type rejected, got %v", err)
	}
}

func TestAppsAccessScopeSet_ApproverRequiresApplyEnabled(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	err := runAppsShortcut(t, AppsAccessScopeSet, []string{
		"+access-scope-set", "--app-id", "app_x",
		"--scope", "specific",
		"--targets", `[{"type":"user","id":"ou_x"}]`,
		"--approver", "ou_y",
		"--as", "user",
	}, factory, stdout)
	if err == nil || !strings.Contains(err.Error(), "apply-enabled") {
		t.Fatalf("expected --approver requires --apply-enabled, got %v", err)
	}
}

func TestAppsAccessScopeSet_PublicRejectsApprover(t *testing.T) {
	// --approver 只在 specific + apply 流程下有意义；public 模式带它当前会被静默丢弃，
	// 是真实用户语义 bug。这条测试钉死 Validate 阶段拦截。
	factory, stdout, _ := newAppsExecuteFactory(t)
	err := runAppsShortcut(t, AppsAccessScopeSet, []string{
		"+access-scope-set", "--app-id", "app_x",
		"--scope", "public",
		"--require-login=false",
		"--approver", "ou_y",
		"--as", "user",
	}, factory, stdout)
	if err == nil || !strings.Contains(err.Error(), "--approver is not allowed when --scope=public") {
		t.Fatalf("expected --approver rejected for scope=public, got %v", err)
	}
}

func TestAppsAccessScopeSet_PublicRequiresExplicitRequireLogin(t *testing.T) {
	// bare --scope public without --require-login defaults silently to
	// require_login=false (Internet-public + no auth). Reject so the caller
	// has to make an explicit choice; matches SKILL.md "public 必传 --require-login".
	factory, stdout, _ := newAppsExecuteFactory(t)
	err := runAppsShortcut(t, AppsAccessScopeSet, []string{
		"+access-scope-set", "--app-id", "app_x",
		"--scope", "public",
		"--as", "user",
	}, factory, stdout)
	if err == nil || !strings.Contains(err.Error(), "--require-login is required when --scope=public") {
		t.Fatalf("expected --require-login required for public, got %v", err)
	}
}

func TestAppsAccessScopeSet_SpecificRejectsEmptyTargets(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	err := runAppsShortcut(t, AppsAccessScopeSet, []string{
		"+access-scope-set", "--app-id", "app_x",
		"--scope", "specific",
		"--targets", "[]",
		"--as", "user",
	}, factory, stdout)
	if err == nil || !strings.Contains(err.Error(), "--targets must contain at least one entry") {
		t.Fatalf("expected empty --targets rejected, got %v", err)
	}
}

func TestAppsAccessScopeSet_TrimsAppIDInPath(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "PUT",
		URL:    "/open-apis/spark/v1/apps/app_x/access-scope",
		Body:   map[string]interface{}{"code": 0, "data": map[string]interface{}{}},
	})

	if err := runAppsShortcut(t, AppsAccessScopeSet, []string{
		"+access-scope-set", "--app-id", "  app_x  ",
		"--scope", "tenant",
		"--as", "user",
	}, factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
}

func TestSplitAccessScopeTargets_Partitions(t *testing.T) {
	users, departments, chats := splitAccessScopeTargets([]map[string]interface{}{
		{"type": "user", "id": "u1"},
		{"type": "department", "id": "d1"},
		{"type": "chat", "id": "c1"},
		{"type": "user", "id": "  "},   // empty id skipped
		{"type": "unknown", "id": "x"}, // unknown type skipped
	})
	if len(users) != 1 || users[0] != "u1" {
		t.Errorf("users=%v want [u1]", users)
	}
	if len(departments) != 1 || departments[0] != "d1" {
		t.Errorf("departments=%v want [d1]", departments)
	}
	if len(chats) != 1 || chats[0] != "c1" {
		t.Errorf("chats=%v want [c1]", chats)
	}
}

func TestValidateTargetsJSON_Cases(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		wantErr bool
	}{
		{"invalid json", "{not json", true},
		{"empty array", "[]", true},
		{"bad type", `[{"type":"role","id":"r1"}]`, true},
		{"empty id", `[{"type":"user","id":"  "}]`, true},
		{"valid", `[{"type":"user","id":"u1"}]`, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := validateTargetsJSON(c.in)
			if (err != nil) != c.wantErr {
				t.Errorf("validateTargetsJSON(%q) err=%v wantErr=%v", c.in, err, c.wantErr)
			}
		})
	}
}
