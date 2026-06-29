// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"testing"

	"github.com/spf13/cobra"
)

// 钉死域内 shortcut 数量。少一条（漏挂）或多一条（误加）都会被这个测试拦截。
// 6 基础 + 1 init + 3 publish + 1 env-pull + 4 db（table-list/table-schema/sql/dev-init）
// + 3 git-credential + 5 session（create/list/get/stop/chat）+ 1 session-messages-list = 24。
func TestAppsShortcuts_Returns24(t *testing.T) {
	got := Shortcuts()
	if len(got) != 24 {
		t.Fatalf("Shortcuts() returned %d entries, want 24", len(got))
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

func TestAppsGitCredentialHelper_IsNotAShortcut(t *testing.T) {
	for _, shortcut := range Shortcuts() {
		if shortcut.Command == "git-credential-helper" {
			t.Fatalf("git credential helper must be installed as a hidden apps command, not as a shortcut")
		}
	}
}

func TestAppsGitCredentialRemove_IsLocalCleanupWithoutScopes(t *testing.T) {
	if len(AppsGitCredentialRemove.Scopes) != 0 {
		t.Fatalf("git credential remove scopes = %#v, want none for local cleanup", AppsGitCredentialRemove.Scopes)
	}
}

func TestAppsGitCredentialList_IsLocalReadWithoutScopes(t *testing.T) {
	if len(AppsGitCredentialList.Scopes) != 0 {
		t.Fatalf("git credential list scopes = %#v, want none for local read", AppsGitCredentialList.Scopes)
	}
}

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
