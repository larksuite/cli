// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// TestDrive_SyncWorkflow exercises +sync against a real Drive folder, proving
// that new_remote files are pulled, new_local files are pushed, and conflicts
// are resolved according to --on-conflict.
//
// Layout (before sync):
//
//	folder/                         (--folder-token target)
//	├── remote-only.txt   "remote"  ↔ (none)             → new_remote → pull
//	├── conflict.txt      "remote"  ↔ local: "local"     → modified → resolve
//	└── unchanged.txt     "match"   ↔ local: "match"     → unchanged → skip
//	local/                          (--local-dir target)
//	├── local-only.txt    "local"                       → new_local → push
//	├── conflict.txt      "local"                       → modified → resolve
//	└── unchanged.txt     "match"                       → unchanged → skip
func TestDrive_SyncWorkflow(t *testing.T) {
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	t.Cleanup(cancel)

	suffix := clie2e.GenerateSuffix()
	folderToken := createDriveFolder(t, parentT, ctx, "lark-cli-e2e-drive-sync-"+suffix, "")

	workDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workDir, "local"), 0o755); err != nil {
		t.Fatalf("mkdir local: %v", err)
	}

	writeLocal := func(rel, content string) {
		t.Helper()
		full := filepath.Join(workDir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir parent of %s: %v", rel, err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}

	uploadDriveFile := func(name, content string) string {
		t.Helper()
		stage := "_upload_" + name
		writeLocal(stage, content)
		t.Cleanup(func() { _ = os.Remove(filepath.Join(workDir, stage)) })

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"drive", "+upload",
				"--file", stage,
				"--folder-token", folderToken,
				"--name", name,
			},
			WorkDir:   workDir,
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		fileToken := gjson.Get(result.Stdout, "data.file_token").String()
		require.NotEmpty(t, fileToken, "uploaded file should have a token, stdout:\n%s", result.Stdout)

		parentT.Cleanup(func() {
			cleanupCtx, cleanupCancel := clie2e.CleanupContext()
			defer cleanupCancel()
			deleteResult, deleteErr := clie2e.RunCmdWithRetry(cleanupCtx, clie2e.Request{
				Args:      []string{"drive", "+delete", "--file-token", fileToken, "--type", "file", "--yes"},
				DefaultAs: "bot",
			}, clie2e.RetryOptions{})
			clie2e.ReportCleanupFailure(parentT, "delete drive file "+fileToken, deleteResult, deleteErr)
		})
		return fileToken
	}

	// --- Subtest: remote-wins (default) ---
	t.Run("remote-wins pulls new_remote and overwrites on conflict", func(t *testing.T) {
		tokUnchanged := uploadDriveFile("unchanged.txt", "match")
		tokConflict := uploadDriveFile("conflict.txt", "remote")
		tokRemoteOnly := uploadDriveFile("remote-only.txt", "remote")

		writeLocal("local/unchanged.txt", "match")
		writeLocal("local/conflict.txt", "local")
		writeLocal("local/local-only.txt", "local")

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"drive", "+sync",
				"--local-dir", "local",
				"--folder-token", folderToken,
				"--on-conflict", "remote-wins",
			},
			WorkDir:   workDir,
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		skipDriveStatusExactIfMissingDownloadScope(t, result)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		out := result.Stdout

		// Summary checks.
		if got := gjson.Get(out, "data.summary.pulled").Int(); got != 2 {
			t.Fatalf("pulled=%d want 2 (remote-only + conflict resolved by pull)\nstdout:\n%s", got, out)
		}
		if got := gjson.Get(out, "data.summary.pushed").Int(); got < 1 {
			t.Fatalf("pushed=%d want >=1 (local-only)\nstdout:\n%s", got, out)
		}
		if got := gjson.Get(out, "data.summary.failed").Int(); got != 0 {
			t.Fatalf("failed=%d want 0\nstdout:\n%s", got, out)
		}

		// Item-level checks.
		assertSyncItem(t, out, "downloaded", "pull", "remote-only.txt", tokRemoteOnly)
		assertSyncItem(t, out, "downloaded", "pull", "conflict.txt", tokConflict)
		assertSyncItem(t, out, "uploaded", "push", "local-only.txt", "")

		// Verify local file content after sync.
		conflictContent, err := os.ReadFile(filepath.Join(workDir, "local", "conflict.txt"))
		if err != nil {
			t.Fatalf("read conflict.txt: %v", err)
		}
		if string(conflictContent) != "remote" {
			t.Fatalf("conflict.txt content=%q want %q", string(conflictContent), "remote")
		}
		require.FileExists(t, filepath.Join(workDir, "local", "remote-only.txt"))

		// Convergence: +status should now show all files as unchanged.
		assertSyncConverges(t, ctx, workDir, folderToken, tokUnchanged)
	})

	// --- Subtest: local-wins ---
	t.Run("local-wins pushes new_local and overwrites remote on conflict", func(t *testing.T) {
		tokConflict := uploadDriveFile("conflict-lw.txt", "remote")
		_ = uploadDriveFile("remote-only-lw.txt", "remote")

		writeLocal("local/conflict-lw.txt", "local-wins")
		writeLocal("local/local-only-lw.txt", "local")
		writeLocal("local/remote-only-lw.txt", "already-here")

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"drive", "+sync",
				"--local-dir", "local",
				"--folder-token", folderToken,
				"--on-conflict", "local-wins",
			},
			WorkDir:   workDir,
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		skipDriveStatusExactIfMissingDownloadScope(t, result)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		out := result.Stdout

		// Conflict file should be overwritten with local version.
		assertSyncItem(t, out, "overwritten", "push", "conflict-lw.txt", tokConflict)

		// Verify local content is unchanged (local-wins).
		conflictContent, err := os.ReadFile(filepath.Join(workDir, "local", "conflict-lw.txt"))
		if err != nil {
			t.Fatalf("read conflict-lw.txt: %v", err)
		}
		if string(conflictContent) != "local-wins" {
			t.Fatalf("conflict-lw.txt content=%q want %q", string(conflictContent), "local-wins")
		}
	})

	// --- Subtest: keep-both ---
	t.Run("keep-both renames local and pulls remote", func(t *testing.T) {
		uploadDriveFile("conflict-kb.txt", "remote-kb")

		writeLocal("local/conflict-kb.txt", "local-kb")

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"drive", "+sync",
				"--local-dir", "local",
				"--folder-token", folderToken,
				"--on-conflict", "keep-both",
			},
			WorkDir:   workDir,
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		skipDriveStatusExactIfMissingDownloadScope(t, result)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		out := result.Stdout

		// Should have a renamed_local item and a downloaded item.
		assertSyncItem(t, out, "renamed_local", "conflict", "conflict-kb.txt", "")
		assertSyncItem(t, out, "downloaded", "pull", "conflict-kb.txt", "")

		// Original path now has remote content.
		origContent, err := os.ReadFile(filepath.Join(workDir, "local", "conflict-kb.txt"))
		if err != nil {
			t.Fatalf("read conflict-kb.txt: %v", err)
		}
		if string(origContent) != "remote-kb" {
			t.Fatalf("conflict-kb.txt content=%q want %q", string(origContent), "remote-kb")
		}

		// A suffixed sibling should exist with the local content.
		entries, err := os.ReadDir(filepath.Join(workDir, "local"))
		if err != nil {
			t.Fatalf("readdir local: %v", err)
		}
		var foundSuffixed bool
		for _, e := range entries {
			if strings.HasPrefix(e.Name(), "conflict-kb__lark_") && strings.HasSuffix(e.Name(), ".txt") {
				foundSuffixed = true
				suffixedContent, readErr := os.ReadFile(filepath.Join(workDir, "local", e.Name()))
				if readErr != nil {
					t.Fatalf("read suffixed file: %v", readErr)
				}
				if string(suffixedContent) != "local-kb" {
					t.Fatalf("suffixed file content=%q want %q", string(suffixedContent), "local-kb")
				}
				break
			}
		}
		if !foundSuffixed {
			t.Fatalf("expected suffixed sibling conflict-kb__lark_*.txt, entries: %v", entries)
		}
	})
}

// TestDrive_SyncEmptyDirWorkflow proves that empty local directories are
// created on Drive during +sync, and that a subsequent +status converges.
func TestDrive_SyncEmptyDirWorkflow(t *testing.T) {
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)

	suffix := clie2e.GenerateSuffix()
	folderToken := createDriveFolder(t, parentT, ctx, "lark-cli-e2e-drive-sync-emptydir-"+suffix, "")

	workDir := t.TempDir()
	// Create an empty subdirectory under local.
	if err := os.MkdirAll(filepath.Join(workDir, "local", "empty_sub"), 0o755); err != nil {
		t.Fatalf("mkdir local/empty_sub: %v", err)
	}

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"drive", "+sync",
			"--local-dir", "local",
			"--folder-token", folderToken,
		},
		WorkDir:   workDir,
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	result.AssertStdoutStatus(t, true)

	out := result.Stdout
	// Should report folder_created for the empty directory.
	if !strings.Contains(out, `"action": "folder_created"`) {
		t.Fatalf("expected folder_created action for empty directory, got:\n%s", out)
	}
	if !strings.Contains(out, `"empty_sub"`) {
		t.Fatalf("expected empty_sub in items, got:\n%s", out)
	}
}

// assertSyncItem checks that a sync item with the given action, direction,
// and rel_path exists in the output. If fileToken is non-empty, it also
// verifies the item carries that token.
func assertSyncItem(t *testing.T, stdout, action, direction, relPath, fileToken string) {
	t.Helper()
	items := gjson.Get(stdout, "data.items")
	if !items.IsArray() {
		t.Fatalf("data.items is not an array\nstdout:\n%s", stdout)
	}
	var found bool
	items.ForEach(func(_, item gjson.Result) bool {
		if item.Get("action").String() != action || item.Get("direction").String() != direction || item.Get("rel_path").String() != relPath {
			return true
		}
		found = true
		if fileToken != "" {
			if got := item.Get("file_token").String(); got != fileToken {
				t.Errorf("item %s/%s/%s file_token=%q want %q", action, direction, relPath, got, fileToken)
			}
		}
		return false
	})
	if !found {
		t.Fatalf("missing sync item action=%s direction=%s rel_path=%s\nstdout:\n%s", action, direction, relPath, stdout)
	}
}

// assertSyncConverges runs +status after a sync and asserts that all shared
// files are unchanged (i.e. the mirror has converged).
func assertSyncConverges(t *testing.T, ctx context.Context, workDir, folderToken, unchangedToken string) {
	t.Helper()
	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"drive", "+status",
			"--local-dir", "local",
			"--folder-token", folderToken,
		},
		WorkDir:   workDir,
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	skipDriveStatusExactIfMissingDownloadScope(t, result)
	result.AssertExitCode(t, 0)
	result.AssertStdoutStatus(t, true)

	out := result.Stdout
	if got := gjson.Get(out, "data.modified.#").Int(); got != 0 {
		t.Fatalf("post-sync +status modified=%d want 0\nstdout:\n%s", got, out)
	}
	if got := gjson.Get(out, "data.new_local.#").Int(); got != 0 {
		t.Fatalf("post-sync +status new_local=%d want 0\nstdout:\n%s", got, out)
	}
	if got := gjson.Get(out, "data.new_remote.#").Int(); got != 0 {
		t.Fatalf("post-sync +status new_remote=%d want 0\nstdout:\n%s", got, out)
	}
	if unchangedToken != "" {
		assertStatusBucketEntry(t, out, "unchanged", "unchanged.txt", unchangedToken)
	}
}
