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
	dir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", dir)
	if err := WriteStamp("1.0.21"); err != nil {
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

func TestInit_Drift_NoticeWithStampVersion(t *testing.T) {
	clearSkillsSkipEnv(t)
	resetPending(t)
	dir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", dir)
	if err := WriteStamp("1.0.20"); err != nil {
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
	// Even with an empty config dir (no stamp), DEV version should skip
	// the check entirely and never emit a notice.
	Init("DEV")
	if got := GetPending(); got != nil {
		t.Errorf("GetPending() = %+v, want nil (skip rules met)", got)
	}
}

func TestInit_ReadStampError_FailsClosed(t *testing.T) {
	clearSkillsSkipEnv(t)
	resetPending(t)
	dir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", dir)
	// Make the stamp path a directory so vfs.ReadFile returns a
	// non-ENOENT I/O error.
	if err := os.MkdirAll(filepath.Join(dir, "skills.stamp"), 0o755); err != nil {
		t.Fatal(err)
	}
	Init("1.0.21")
	if got := GetPending(); got != nil {
		t.Errorf("GetPending() = %+v, want nil (fail closed on I/O error)", got)
	}
}
