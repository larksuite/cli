// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package skillscheck

import (
	"errors"
	"io/fs"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/larksuite/cli/internal/vfs"
)

func TestSyncPreparedTreeReplacesOfficialSkillsAndPreservesCustom(t *testing.T) {
	root := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", filepath.Join(root, "config"))
	target := filepath.Join(root, "installed")
	writePreparedTestFile(t, filepath.Join(target, "retired", "SKILL.md"), "old")
	writePreparedTestFile(t, filepath.Join(target, "custom", "SKILL.md"), "custom")
	if err := WriteState(SkillsState{Version: "old", OfficialSkills: []string{"retired"}}); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(root, "prepared")
	writePreparedTestFile(t, filepath.Join(source, "current", "SKILL.md"), "new")
	writePreparedTestFile(t, filepath.Join(source, "README.md"), "metadata")

	rollback, finalize, err := SyncPreparedTree(PreparedTreeOptions{
		Root: source, Version: "target", SourceIdentity: "manifest:test", TargetDir: target,
	})
	if err != nil {
		t.Fatal(err)
	}
	if rollback == nil || finalize == nil {
		t.Fatal("SyncPreparedTree returned incomplete transaction hooks")
	}
	finalize()
	assertPreparedTestFile(t, filepath.Join(target, "current", "SKILL.md"), "new")
	assertPreparedTestFile(t, filepath.Join(target, "custom", "SKILL.md"), "custom")
	if _, err := vfs.Stat(filepath.Join(target, "retired")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("retired Skill remains: %v", err)
	}
	state, ok, err := ReadState()
	if err != nil || !ok || state.Version != "target" || state.SourceIdentity != "manifest:test" ||
		!reflect.DeepEqual(state.OfficialSkills, []string{"current"}) {
		t.Fatalf("state = %#v, %v, %v", state, ok, err)
	}
}

func TestPreparedSkillsTargetsHonorDetectedAgentHomes(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(root, "claude"))
	t.Setenv("CODEX_HOME", filepath.Join(root, "codex"))
	got, err := preparedSkillsTargets("")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		filepath.Join(root, ".agents", "skills"),
		filepath.Join(root, "claude", "skills"),
		filepath.Join(root, "codex", "skills"),
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("targets = %#v, want %#v", got, want)
	}
}

func writePreparedTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := vfs.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := vfs.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func assertPreparedTestFile(t *testing.T, path, want string) {
	t.Helper()
	got, err := vfs.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("%s = %q, want %q", path, got, want)
	}
}
