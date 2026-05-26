// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package skillscheck

import (
	"os"
	"path/filepath"
	"testing"
)

func resetPending(t *testing.T) {
	t.Helper()
	SetPending(nil)
	t.Cleanup(func() { SetPending(nil) })
}

func TestInit_InSync_NoNotice(t *testing.T) {
	clearSkillsSkipEnv(t)
	resetPending(t)
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	if err := WriteState(SkillsState{Version: "1.0.21"}); err != nil {
		t.Fatal(err)
	}
	Init("1.0.21")
	if got := GetPending(); got != nil {
		t.Errorf("GetPending() = %+v, want nil (in-sync)", got)
	}
}

func TestInit_ColdStart_NoNotice(t *testing.T) {
	clearSkillsSkipEnv(t)
	resetPending(t)
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	Init("1.0.21")
	if got := GetPending(); got != nil {
		t.Errorf("GetPending() = %+v, want nil (cold start is silent)", got)
	}
}

func TestInit_NormalizedVersion_NoNotice(t *testing.T) {
	clearSkillsSkipEnv(t)
	resetPending(t)
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	if err := WriteState(SkillsState{Version: "1.0.21"}); err != nil {
		t.Fatal(err)
	}
	Init("v1.0.21")
	if got := GetPending(); got != nil {
		t.Errorf("GetPending() = %+v, want nil (normalized versions are in-sync)", got)
	}
}

func TestInit_Drift_NoticeWithStateVersion(t *testing.T) {
	clearSkillsSkipEnv(t)
	resetPending(t)
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	if err := WriteState(SkillsState{Version: "1.0.20"}); err != nil {
		t.Fatal(err)
	}
	Init("1.0.21")
	got := GetPending()
	if got == nil {
		t.Fatal("GetPending() = nil, want non-nil for drift")
	}
	if got.Current != "1.0.20" || got.Target != "1.0.21" {
		t.Errorf("notice = %+v, want {Current:\"1.0.20\", Target:\"1.0.21\"}", got)
	}
}

func TestInit_Skipped_NoNotice(t *testing.T) {
	clearSkillsSkipEnv(t)
	resetPending(t)
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	Init("DEV")
	if got := GetPending(); got != nil {
		t.Errorf("GetPending() = %+v, want nil (skip rules met)", got)
	}
}

func TestInit_ReadStateError_FailsClosed(t *testing.T) {
	clearSkillsSkipEnv(t)
	resetPending(t)
	dir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", dir)
	if err := os.MkdirAll(filepath.Join(dir, "skills-state.json"), 0o755); err != nil {
		t.Fatal(err)
	}
	Init("1.0.21")
	if got := GetPending(); got != nil {
		t.Errorf("GetPending() = %+v, want nil (fail closed on I/O error)", got)
	}
}
