// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/httpmock"
)

func TestAppsAccessScopeSet_Specific(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	stub := &httpmock.Stub{
		Method: "PUT",
		URL:    "/open-apis/miaoda/v1/apps/app_x/access-scope",
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
	if sent["scope"] != "specific" {
		t.Fatalf("scope = %v", sent["scope"])
	}
	targets, ok := sent["targets"].([]interface{})
	if !ok || len(targets) != 2 {
		t.Fatalf("targets = %v", sent["targets"])
	}
}

func TestAppsAccessScopeSet_Public(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "PUT",
		URL:    "/open-apis/miaoda/v1/apps/app_x/access-scope",
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
		URL:    "/open-apis/miaoda/v1/apps/app_x/access-scope",
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
