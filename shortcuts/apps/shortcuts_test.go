// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"testing"

	"github.com/spf13/cobra"
)

// 钉死域内 shortcut 数量。少一条（漏挂）或多一条（误加）都会被这个测试拦截。
// 6 基础 + 1 init + 3 publish + 1 env-pull
//   - 6 observability（log-list/log-get/trace-list/trace-get/metric-list/analytics-list）
//   - 3 env（list/set/delete）
//   - 16 db（table-list/table-schema/sql/dev-init/data-import/data-export/changelog-list/
//     audit-status/audit-enable/audit-disable/audit-list/
//     env-diff/env-migrate/recovery-diff/recovery-apply/quota-get）
//   - 7 file（list/get/sign/download/upload/delete/quota-get）
//   - 3 git-credential
//   - 5 session（create/list/get/stop/chat）+ 1 session-messages-list
//   - 8 openapi-key（list/get/create/update/enable/disable/delete/reset）
//   - 3 plugin（install/uninstall/list）= 63。
func TestAppsShortcuts_Returns63(t *testing.T) {
	got := Shortcuts()
	if len(got) != 63 {
		t.Fatalf("Shortcuts() returned %d entries, want 63", len(got))
	}
}

func TestAppsShortcuts_DoesNotIncludeEnvGet(t *testing.T) {
	for _, sc := range Shortcuts() {
		switch sc.Command {
		case "+env-get", "+envvar-get", "+envvar-list", "+envvar-set", "+envvar-delete":
			t.Fatalf("Shortcuts() must not register %s", sc.Command)
		}
	}
}

func TestAppsShortcuts_DoesNotIncludeMetricQueryAliases(t *testing.T) {
	for _, sc := range Shortcuts() {
		switch sc.Command {
		case "+metric-query", "+analytics-query":
			t.Fatalf("Shortcuts() must not register %s", sc.Command)
		}
	}
}

func TestAppsShortcuts_EnvCommandsUseCanonicalNames(t *testing.T) {
	want := map[string]bool{
		"+env-list":   false,
		"+env-set":    false,
		"+env-delete": false,
	}
	for _, sc := range Shortcuts() {
		if _, ok := want[sc.Command]; ok {
			want[sc.Command] = true
			if sc.Hidden {
				t.Errorf("%s must be visible", sc.Command)
			}
		}
	}
	for cmd, found := range want {
		if !found {
			t.Errorf("Shortcuts() missing canonical %s", cmd)
		}
	}
}

// 确认 5 个 session 生命周期命令都已挂载。
func TestAppsShortcuts_IncludesSessionCommands(t *testing.T) {
	want := map[string]bool{
		"+session-create": false,
		"+session-list":   false,
		"+session-get":    false,
		"+session-stop":   false,
		"+chat":           false,
	}
	for _, sc := range Shortcuts() {
		if _, ok := want[sc.Command]; ok {
			want[sc.Command] = true
		}
	}
	for cmd, found := range want {
		if !found {
			t.Errorf("Shortcuts() missing %s", cmd)
		}
	}
}

// TestAppsGitCredentialHelper_IsNotAShortcut 确认 git credential helper 不作为 shortcut 暴露。
func TestAppsGitCredentialHelper_IsNotAShortcut(t *testing.T) {
	for _, shortcut := range Shortcuts() {
		if shortcut.Command == "git-credential-helper" {
			t.Fatalf("git credential helper must be installed as a hidden apps command, not as a shortcut")
		}
	}
}

// TestAppsGitCredentialRemove_IsLocalCleanupWithoutScopes 确认 git credential remove 是本地清理、不带任何 scope。
func TestAppsGitCredentialRemove_IsLocalCleanupWithoutScopes(t *testing.T) {
	if len(AppsGitCredentialRemove.Scopes) != 0 {
		t.Fatalf("git credential remove scopes = %#v, want none for local cleanup", AppsGitCredentialRemove.Scopes)
	}
}

// TestAppsGitCredentialList_IsLocalReadWithoutScopes 确认 git credential list 是本地读取、不带任何 scope。
func TestAppsGitCredentialList_IsLocalReadWithoutScopes(t *testing.T) {
	if len(AppsGitCredentialList.Scopes) != 0 {
		t.Fatalf("git credential list scopes = %#v, want none for local read", AppsGitCredentialList.Scopes)
	}
}

// TestInstallOnApps_AddsHiddenGitCredentialHelper 验证 InstallOnApps 挂载一个隐藏、带 RunE 且独立于 shortcut 管线的 git-credential-helper 命令。
func TestInstallOnApps_AddsHiddenGitCredentialHelper(t *testing.T) {
	parent := &cobra.Command{Use: "apps"}
	InstallOnApps(parent, nil)
	cmd, _, err := parent.Find([]string{"git-credential-helper"})
	if err != nil {
		t.Fatalf("find helper returned error: %v", err)
	}
	if cmd == nil || cmd.Name() != "git-credential-helper" {
		t.Fatalf("helper command not installed: %#v", cmd)
	}
	if !cmd.Hidden {
		t.Fatalf("git credential helper must be hidden")
	}
	if cmd.RunE == nil {
		t.Fatalf("git credential helper must run outside the shortcut pipeline")
	}
}
