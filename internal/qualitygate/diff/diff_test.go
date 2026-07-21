// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package diff

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/larksuite/cli/internal/testutil/gitcmd"
)

func TestScopeIncludesChangedSkillAndRelatedDomain(t *testing.T) {
	scope := FromChangedFiles([]string{
		"skills/lark-doc/SKILL.md",
		"skills/lark-im/references/lark-im-chat-list.md",
		"internal/output/errors.go",
	})
	if !scope.AllSkills["lark-doc"] || !scope.AllSkills["lark-im"] {
		t.Fatalf("skill scope missing changed skills: %#v", scope.AllSkills)
	}
	if !scope.Global {
		t.Fatalf("internal/output/errors.go should trigger global checks")
	}
}

func TestScopeTreatsDeletedShortcutAsGlobal(t *testing.T) {
	scope := FromChangedFiles([]string{"shortcuts/mail/send.go"})
	if !scope.Global {
		t.Fatal("shortcut paths from git diff must force global checks, including deleted files")
	}
}

func TestScopeDoesNotTreatDefaultMetadataAsGlobal(t *testing.T) {
	scope := FromChangedFiles([]string{"internal/registry/meta_data_default.json"})
	if scope.Global {
		t.Fatal("default metadata changes should not force ordinary quality-gate global scope")
	}
}

func TestFileAtRevisionMissingClassifier(t *testing.T) {
	msg := "fatal: path 'internal/qualitygate/config/contracts/command_manifest.golden.json' exists on disk, but not in 'origin/main'"
	if !isFileAtRevisionMissing(msg) {
		t.Fatalf("expected missing file classifier to match")
	}
	if isFileAtRevisionMissing("fatal: ambiguous argument 'origin/missing'") {
		t.Fatalf("bad revision should not be treated as a missing file")
	}
}

func TestChangedFilesIncludingWorktree(t *testing.T) {
	repo := t.TempDir()
	runGit(t, repo, "init")
	runGit(t, repo, "config", "user.email", "test@example.com")
	runGit(t, repo, "config", "user.name", "Test User")

	writeFile(t, repo, "tracked.txt", "base\n")
	writeFile(t, repo, "staged.txt", "base\n")
	writeFile(t, repo, "unstaged.txt", "base\n")
	runGit(t, repo, "add", ".")
	runGit(t, repo, "commit", "-m", "base")
	base := gitOutput(t, repo, "rev-parse", "HEAD")

	writeFile(t, repo, "committed.txt", "committed\n")
	runGit(t, repo, "add", ".")
	runGit(t, repo, "commit", "-m", "committed")

	writeFile(t, repo, "staged.txt", "staged\n")
	runGit(t, repo, "add", "staged.txt")
	writeFile(t, repo, "unstaged.txt", "unstaged\n")
	writeFile(t, repo, "untracked.txt", "untracked\n")

	got, err := ChangedFilesIncludingWorktree(context.Background(), repo, base)
	if err != nil {
		t.Fatalf("ChangedFilesIncludingWorktree() error = %v", err)
	}
	want := []string{"committed.txt", "staged.txt", "unstaged.txt", "untracked.txt"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ChangedFilesIncludingWorktree() = %#v, want %#v", got, want)
	}
}

func TestChangedFilesHandlesWhitespacePaths(t *testing.T) {
	repo := t.TempDir()
	runGit(t, repo, "init")
	runGit(t, repo, "config", "user.email", "test@example.com")
	runGit(t, repo, "config", "user.name", "Test User")

	writeFile(t, repo, "base.txt", "base\n")
	runGit(t, repo, "add", ".")
	runGit(t, repo, "commit", "-m", "base")
	base := gitOutput(t, repo, "rev-parse", "HEAD")

	// A path containing spaces must survive intact. With whitespace splitting
	// this returned four mangled tokens instead of one path.
	writeFile(t, repo, "dir with space/a b.txt", "x\n")
	runGit(t, repo, "add", ".")
	runGit(t, repo, "commit", "-m", "spaced")

	got, err := ChangedFiles(context.Background(), repo, base)
	if err != nil {
		t.Fatalf("ChangedFiles() error = %v", err)
	}
	want := []string{"dir with space/a b.txt"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ChangedFiles() = %#v, want %#v", got, want)
	}
}

func writeFile(t *testing.T, repo, rel, content string) {
	t.Helper()
	path := filepath.Join(repo, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", rel, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
}

func runGit(t *testing.T, repo string, args ...string) {
	t.Helper()
	cmd := gitcmd.Command(repo, args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}

func gitOutput(t *testing.T, repo string, args ...string) string {
	t.Helper()
	cmd := gitcmd.Command(repo, args...)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git %v failed: %v", args, err)
	}
	return string(out[:len(out)-1])
}
