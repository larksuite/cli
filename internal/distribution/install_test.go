// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package distribution

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/larksuite/cli/internal/skillscheck"
	"github.com/larksuite/cli/internal/vfs"
)

func TestInstallPreparedVerificationFailureDoesNotMutate(t *testing.T) {
	root := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", filepath.Join(root, "config"))
	executable := filepath.Join(root, "bin", "lark-cli")
	mustWrite(t, executable, "old")
	binary := filepath.Join(root, "prepared", "lark-cli")
	mustWrite(t, binary, "new")
	mustWrite(t, filepath.Join(root, "prepared", "skills", "managed", "SKILL.md"), "new")
	prepared := &preparedUpdate{
		Manifest:   &Manifest{Version: "target"},
		BinaryPath: binary,
		SkillsRoot: filepath.Join(root, "prepared", "skills"),
	}
	err := installPrepared(prepared, InstallOptions{
		ExecutablePath: executable,
		SkillsDir:      filepath.Join(root, "skills"),
		VerifyBinary:   func(string, string) error { return errors.New("bad binary") },
	})
	if err == nil {
		t.Fatal("installPrepared succeeded")
	}
	assertFile(t, executable, "old")
}

func TestInstallPreparedBinaryCommitFailureRollsBackSkillsAndState(t *testing.T) {
	root := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", filepath.Join(root, "config"))
	executable := filepath.Join(root, "bin", "lark-cli")
	mustWrite(t, executable, "old")
	mustWrite(t, filepath.Join(executable+".old", "block-removal"), "blocked")
	skillsDir := filepath.Join(root, "skills")
	mustWrite(t, filepath.Join(skillsDir, "managed", "SKILL.md"), "old")
	if err := skillscheck.WriteState(skillscheck.SkillsState{Version: "old", OfficialSkills: []string{"managed"}}); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(root, "prepared", "lark-cli")
	mustWrite(t, binary, "new")
	mustWrite(t, filepath.Join(root, "prepared", "skills", "managed", "SKILL.md"), "new")
	prepared := &preparedUpdate{
		Manifest:   &Manifest{Version: "target"},
		BinaryPath: binary,
		SkillsRoot: filepath.Join(root, "prepared", "skills"),
	}
	if err := installPrepared(prepared, InstallOptions{
		ExecutablePath: executable,
		SkillsDir:      skillsDir,
		VerifyBinary:   func(string, string) error { return nil },
	}); err == nil {
		t.Fatal("installPrepared succeeded")
	}
	assertFile(t, filepath.Join(skillsDir, "managed", "SKILL.md"), "old")
	state, ok, err := skillscheck.ReadState()
	if err != nil || !ok || state.Version != "old" {
		t.Fatalf("state after rollback = %#v, %v, %v", state, ok, err)
	}
	assertFile(t, executable, "old")
}

func mustWrite(t *testing.T, path, value string) {
	t.Helper()
	if err := vfs.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := vfs.WriteFile(path, []byte(value), 0o755); err != nil {
		t.Fatal(err)
	}
}

func assertFile(t *testing.T, path, want string) {
	t.Helper()
	got, err := vfs.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("%s = %q, want %q", path, got, want)
	}
}
