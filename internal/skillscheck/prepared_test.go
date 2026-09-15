// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package skillscheck

import (
	"errors"
	"io/fs"
	"path/filepath"
	"reflect"
	"strings"
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

	update, err := SyncPreparedTree(PreparedTreeOptions{
		Root: source, Version: "target", SourceIdentity: "manifest:test", TargetDir: target,
	})
	if err != nil {
		t.Fatal(err)
	}
	if update == nil {
		t.Fatal("SyncPreparedTree returned no update")
	}
	update.Finalize()
	assertNoPreparedBackups(t, root)
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

func TestSyncPreparedTreeRollbackRestoresFilesAndExactState(t *testing.T) {
	root := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", filepath.Join(root, "config"))
	target, source := filepath.Join(root, "installed"), filepath.Join(root, "prepared")
	for _, name := range []string{"current", "retired", "custom"} {
		writePreparedTestFile(t, filepath.Join(target, name, "SKILL.md"), "old "+name)
	}
	for _, name := range []string{"current", "added"} {
		writePreparedTestFile(t, filepath.Join(source, name, "SKILL.md"), "new "+name)
	}
	previousState := "{\n  \"version\": \"old\", \"official_skills\": [\"current\", \"retired\"], \"extra\": true\n}\n"
	writePreparedTestFile(t, statePath(), previousState)

	update, err := SyncPreparedTree(PreparedTreeOptions{
		Root: source, TargetDir: target, Version: "new", SourceIdentity: "manifest:test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := update.Rollback(); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"current", "retired", "custom"} {
		assertPreparedTestFile(t, filepath.Join(target, name, "SKILL.md"), "old "+name)
	}
	if _, err := vfs.Stat(filepath.Join(target, "added")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("new Skill remains after rollback: %v", err)
	}
	assertPreparedTestFile(t, statePath(), previousState)
	assertNoPreparedBackups(t, root)
}

func TestInstallPreparedToTargetsRollsBackEarlierTargets(t *testing.T) {
	root := t.TempDir()
	source, first, second := filepath.Join(root, "prepared"), filepath.Join(root, "first"), filepath.Join(root, "second")
	writePreparedTestFile(t, filepath.Join(source, "current", "SKILL.md"), "new")
	writePreparedTestFile(t, filepath.Join(first, "current", "SKILL.md"), "old")
	writePreparedTestFile(t, second, "not a directory")
	plan := PlanSync(SyncInput{OfficialSkills: []string{"current"}, Force: true})
	if _, err := installPreparedToTargets(source, []string{first, second}, plan); err == nil {
		t.Fatal("installation to a file unexpectedly succeeded")
	}
	assertPreparedTestFile(t, filepath.Join(first, "current", "SKILL.md"), "old")
	assertPreparedTestFile(t, second, "not a directory")
	assertNoPreparedBackups(t, root)
}

func TestSyncPreparedTreeRollbackFailurePreservesBackup(t *testing.T) {
	root := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", filepath.Join(root, "config"))
	target, source := filepath.Join(root, "installed"), filepath.Join(root, "prepared")
	writePreparedTestFile(t, filepath.Join(target, "retired", "SKILL.md"), "old")
	writePreparedTestFile(t, filepath.Join(source, "current", "SKILL.md"), "new")
	previousState := "{\"version\":\"old\",\"official_skills\":[\"retired\"]}\n"
	writePreparedTestFile(t, statePath(), previousState)
	update, err := SyncPreparedTree(PreparedTreeOptions{Root: source, TargetDir: target, Version: "new"})
	if err != nil {
		t.Fatal(err)
	}
	// A new non-empty directory prevents the old directory from being restored.
	writePreparedTestFile(t, filepath.Join(target, "retired", "block"), "keep")
	if err := update.Rollback(); err == nil {
		t.Fatal("rollback unexpectedly replaced a non-empty directory")
	}
	assertPreparedTestFile(t, statePath(), previousState)
	assertPreparedTestFile(t, filepath.Join(target, "retired", "block"), "keep")
	entries, err := vfs.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".lark-cli-skills-old-") {
			assertPreparedTestFile(t, filepath.Join(root, entry.Name(), "retired", "SKILL.md"), "old")
			return
		}
	}
	t.Fatal("backup was not preserved after rollback failed")
}

func assertNoPreparedBackups(t *testing.T, parent string) {
	t.Helper()
	entries, err := vfs.ReadDir(parent)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".lark-cli-skills-") {
			t.Fatalf("temporary directory remains: %s", entry.Name())
		}
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
