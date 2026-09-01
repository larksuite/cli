// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package distribution

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/larksuite/cli/internal/skillscheck"
)

func TestInstallPreparedUpdatesManagedSkillsAndPreservesCustom(t *testing.T) {
	root := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", filepath.Join(root, "config"))
	executable := filepath.Join(root, "bin", "lark-cli")
	mustWrite(t, executable, "old")
	skillsDir := filepath.Join(root, "skills")
	mustWrite(t, filepath.Join(skillsDir, "old-managed", "SKILL.md"), "old")
	mustWrite(t, filepath.Join(skillsDir, "custom", "SKILL.md"), "custom")
	if err := skillscheck.WriteState(skillscheck.SkillsState{Version: "old", OfficialSkills: []string{"old-managed"}}); err != nil {
		t.Fatal(err)
	}
	preparedRoot := filepath.Join(root, "prepared")
	binary := filepath.Join(preparedRoot, "lark-cli")
	mustWrite(t, binary, "new")
	mustWrite(t, filepath.Join(preparedRoot, "skills", "new-managed", "SKILL.md"), "new")
	prepared := &preparedUpdate{Manifest: &Manifest{Version: "target"}, BinaryPath: binary, SkillsRoot: filepath.Join(preparedRoot, "skills"), SkillNames: []string{"new-managed"}}
	if err := installPrepared(prepared, InstallOptions{ExecutablePath: executable, SkillsDir: skillsDir, VerifyBinary: func(path, version string) error { return nil }}); err != nil {
		t.Fatal(err)
	}
	assertFile(t, executable, "new")
	assertFile(t, filepath.Join(skillsDir, "new-managed", "SKILL.md"), "new")
	assertFile(t, filepath.Join(skillsDir, "custom", "SKILL.md"), "custom")
	if _, err := os.Stat(filepath.Join(skillsDir, "old-managed")); !os.IsNotExist(err) {
		t.Fatalf("old managed Skill still exists: %v", err)
	}
	state, ok, err := skillscheck.ReadState()
	if err != nil || !ok || state.Version != "target" {
		t.Fatalf("state = %#v, %v, %v", state, ok, err)
	}
}

func TestInstallPreparedSyncsDetectedClaudeAndCodexSkillsDirs(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	t.Setenv("CODEX_HOME", "")
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", filepath.Join(root, "config"))
	if err := os.MkdirAll(filepath.Join(root, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".codex"), 0o755); err != nil {
		t.Fatal(err)
	}
	executable := filepath.Join(root, "bin", "lark-cli")
	mustWrite(t, executable, "old")
	preparedRoot := filepath.Join(root, "prepared")
	binary := filepath.Join(preparedRoot, "lark-cli")
	mustWrite(t, binary, "new")
	mustWrite(t, filepath.Join(preparedRoot, "skills", "managed", "SKILL.md"), "new")
	prepared := &preparedUpdate{
		Manifest:   &Manifest{Version: "target"},
		BinaryPath: binary,
		SkillsRoot: filepath.Join(preparedRoot, "skills"),
		SkillNames: []string{"managed"},
	}
	if err := installPrepared(prepared, InstallOptions{ExecutablePath: executable, VerifyBinary: func(path, version string) error { return nil }}); err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{
		filepath.Join(root, ".agents", "skills"),
		filepath.Join(root, ".claude", "skills"),
		filepath.Join(root, ".codex", "skills"),
	} {
		assertFile(t, filepath.Join(target, "managed", "SKILL.md"), "new")
	}
}

func TestDiscoverSkillsDirsHonorsAgentHomeOverrides(t *testing.T) {
	root := t.TempDir()
	claudeRoot := filepath.Join(root, "custom-claude")
	codexRoot := filepath.Join(root, "custom-codex")
	t.Setenv("HOME", root)
	t.Setenv("CLAUDE_CONFIG_DIR", claudeRoot)
	t.Setenv("CODEX_HOME", codexRoot)
	dirs, err := discoverSkillsDirs()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		filepath.Join(root, ".agents", "skills"),
		filepath.Join(claudeRoot, "skills"),
		filepath.Join(codexRoot, "skills"),
	}
	if !slices.Equal(dirs, want) {
		t.Fatalf("dirs = %#v, want %#v", dirs, want)
	}
}

func TestInstallSkillsToTargetsRollsBackEarlierTarget(t *testing.T) {
	root := t.TempDir()
	first := filepath.Join(root, "first", "skills")
	mustWrite(t, filepath.Join(first, "managed", "SKILL.md"), "old")
	blockedParent := filepath.Join(root, "blocked")
	mustWrite(t, blockedParent, "not a directory")
	preparedRoot := filepath.Join(root, "prepared")
	mustWrite(t, filepath.Join(preparedRoot, "skills", "managed", "SKILL.md"), "new")
	prepared := &preparedUpdate{
		Manifest:   &Manifest{Version: "target"},
		SkillsRoot: filepath.Join(preparedRoot, "skills"),
		SkillNames: []string{"managed"},
	}
	previous := &skillscheck.SkillsState{OfficialSkills: []string{"managed"}}
	if _, _, err := installSkillsToTargets(prepared, []string{first, filepath.Join(blockedParent, "skills")}, previous); err == nil {
		t.Fatal("installSkillsToTargets succeeded")
	}
	assertFile(t, filepath.Join(first, "managed", "SKILL.md"), "old")
}

func TestInstallPreparedVerificationFailureDoesNotMutate(t *testing.T) {
	root := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", filepath.Join(root, "config"))
	executable := filepath.Join(root, "bin", "lark-cli")
	mustWrite(t, executable, "old")
	binary := filepath.Join(root, "prepared", "lark-cli")
	mustWrite(t, binary, "new")
	mustWrite(t, filepath.Join(root, "prepared", "skills", "managed", "SKILL.md"), "new")
	prepared := &preparedUpdate{Manifest: &Manifest{Version: "target"}, BinaryPath: binary, SkillsRoot: filepath.Join(root, "prepared", "skills"), SkillNames: []string{"managed"}}
	err := installPrepared(prepared, InstallOptions{ExecutablePath: executable, SkillsDir: filepath.Join(root, "skills"), VerifyBinary: func(path, version string) error { return errors.New("bad binary") }})
	if err == nil {
		t.Fatal("InstallPrepared succeeded")
	}
	assertFile(t, executable, "old")
}

func TestMatchesVersionOutputSupportsOpaqueVersion(t *testing.T) {
	if !matchesVersionOutput("lark-cli version release channel 7\n", "release channel 7") {
		t.Fatal("version output did not match")
	}
	if matchesVersionOutput("lark-cli version release channel 8\n", "release channel 7") {
		t.Fatal("mismatched version output matched")
	}
}

func TestInstallPreparedBinaryCommitFailureRollsBackSkillsAndState(t *testing.T) {
	root := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", filepath.Join(root, "config"))
	missingExecutable := filepath.Join(root, "bin", "missing-lark-cli")
	skillsDir := filepath.Join(root, "skills")
	mustWrite(t, filepath.Join(skillsDir, "managed", "SKILL.md"), "old")
	before := skillscheck.SkillsState{Version: "old", OfficialSkills: []string{"managed"}}
	if err := skillscheck.WriteState(before); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(root, "prepared", "lark-cli")
	mustWrite(t, binary, "new")
	mustWrite(t, filepath.Join(root, "prepared", "skills", "managed", "SKILL.md"), "new")
	prepared := &preparedUpdate{Manifest: &Manifest{Version: "target"}, BinaryPath: binary, SkillsRoot: filepath.Join(root, "prepared", "skills"), SkillNames: []string{"managed"}}
	err := installPrepared(prepared, InstallOptions{ExecutablePath: missingExecutable, SkillsDir: skillsDir, VerifyBinary: func(path, version string) error { return nil }})
	if err == nil {
		t.Fatal("InstallPrepared succeeded")
	}
	assertFile(t, filepath.Join(skillsDir, "managed", "SKILL.md"), "old")
	state, ok, readErr := skillscheck.ReadState()
	if readErr != nil || !ok || state.Version != "old" {
		t.Fatalf("state after rollback = %#v, %v, %v", state, ok, readErr)
	}
}

func mustWrite(t *testing.T, path, value string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(value), 0o755); err != nil {
		t.Fatal(err)
	}
}

func assertFile(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("%s = %q, want %q", path, got, want)
	}
}
