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

func TestDrive_DuplicateRemoteWorkflow(t *testing.T) {
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	t.Cleanup(cancel)

	uploadNamedFile := func(t *testing.T, workDir, folderToken, stageName, remoteName, content string) string {
		t.Helper()
		stagePath := filepath.Join(workDir, stageName)
		if err := os.WriteFile(stagePath, []byte(content), 0o644); err != nil {
			t.Fatalf("write stage file %s: %v", stageName, err)
		}
		t.Cleanup(func() { _ = os.Remove(stagePath) })

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"drive", "+upload",
				"--file", stageName,
				"--folder-token", folderToken,
				"--name", remoteName,
			},
			WorkDir:   workDir,
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		fileToken := gjson.Get(result.Stdout, "data.file_token").String()
		require.NotEmpty(t, fileToken, "uploaded file should have a token, stdout:\n%s", result.Stdout)
		return fileToken
	}

	t.Run("status and pull handle duplicate remote files", func(t *testing.T) {
		suffix := clie2e.GenerateSuffix()
		folderToken := createDriveFolder(t, parentT, ctx, "lark-cli-e2e-drive-dup-pull-"+suffix, "")

		workDir := t.TempDir()
		if err := os.MkdirAll(filepath.Join(workDir, "local"), 0o755); err != nil {
			t.Fatalf("mkdir local: %v", err)
		}

		firstToken := uploadNamedFile(t, workDir, folderToken, "_dup_first.txt", "dup.txt", "first")
		secondToken := uploadNamedFile(t, workDir, folderToken, "_dup_second.txt", "dup.txt", "second")

		statusResult, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"drive", "+status",
				"--local-dir", "local",
				"--folder-token", folderToken,
			},
			WorkDir:   workDir,
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		skipDriveStatusExactIfMissingDownloadScope(t, statusResult)
		if statusResult.ExitCode == 0 {
			t.Fatalf("+status should fail on duplicate remote rel_path\nstdout:\n%s\nstderr:\n%s", statusResult.Stdout, statusResult.Stderr)
		}
		if !strings.Contains(statusResult.Stderr, `"type": "duplicate_remote_path"`) {
			t.Fatalf("+status stderr should contain duplicate_remote_path\nstdout:\n%s\nstderr:\n%s", statusResult.Stdout, statusResult.Stderr)
		}

		pullFailResult, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"drive", "+pull",
				"--local-dir", "local",
				"--folder-token", folderToken,
			},
			WorkDir:   workDir,
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		if pullFailResult.ExitCode == 0 {
			t.Fatalf("+pull should fail on duplicate remote rel_path by default\nstdout:\n%s\nstderr:\n%s", pullFailResult.Stdout, pullFailResult.Stderr)
		}
		if !strings.Contains(pullFailResult.Stderr, `"type": "duplicate_remote_path"`) {
			t.Fatalf("+pull stderr should contain duplicate_remote_path\nstdout:\n%s\nstderr:\n%s", pullFailResult.Stdout, pullFailResult.Stderr)
		}
		if _, statErr := os.Stat(filepath.Join(workDir, "local", "dup.txt")); !os.IsNotExist(statErr) {
			t.Fatalf("default duplicate failure must not write dup.txt; stat err=%v", statErr)
		}

		pullRenameResult, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"drive", "+pull",
				"--local-dir", "local",
				"--folder-token", folderToken,
				"--on-duplicate-remote", "rename",
			},
			WorkDir:   workDir,
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		pullRenameResult.AssertExitCode(t, 0)
		pullRenameResult.AssertStdoutStatus(t, true)

		items := gjson.Get(pullRenameResult.Stdout, "data.items")
		if items.Array() == nil || len(items.Array()) != 2 {
			t.Fatalf("+pull rename should produce two items, stdout:\n%s", pullRenameResult.Stdout)
		}
		if got := gjson.Get(pullRenameResult.Stdout, "data.summary.downloaded").Int(); got != 2 {
			t.Fatalf("+pull rename downloaded=%d, want 2\nstdout:\n%s", got, pullRenameResult.Stdout)
		}
		relPaths := []string{
			gjson.Get(pullRenameResult.Stdout, "data.items.0.rel_path").String(),
			gjson.Get(pullRenameResult.Stdout, "data.items.1.rel_path").String(),
		}
		var renamedRel string
		for _, rel := range relPaths {
			if rel != "dup.txt" {
				renamedRel = rel
			}
		}
		if renamedRel == "" || !strings.HasPrefix(renamedRel, "dup__lark_") || !strings.HasSuffix(renamedRel, ".txt") {
			t.Fatalf("renamed rel_path = %q, want dup__lark_<hash>.txt\nstdout:\n%s", renamedRel, pullRenameResult.Stdout)
		}
		if !strings.Contains(pullRenameResult.Stdout, `"source_id":"hash_`) &&
			!strings.Contains(pullRenameResult.Stdout, `"source_id": "hash_`) {
			t.Fatalf("+pull rename stdout should contain source_id for duplicate items\nstdout:\n%s", pullRenameResult.Stdout)
		}
		if strings.Contains(pullRenameResult.Stdout, firstToken) || strings.Contains(pullRenameResult.Stdout, secondToken) {
			t.Fatalf("+pull rename stdout should not expose raw duplicate file tokens\nstdout:\n%s", pullRenameResult.Stdout)
		}
		require.FileExists(t, filepath.Join(workDir, "local", "dup.txt"))
		require.FileExists(t, filepath.Join(workDir, "local", filepath.FromSlash(renamedRel)))
	})

	t.Run("push resolves duplicate remote files and converges status", func(t *testing.T) {
		suffix := clie2e.GenerateSuffix()
		folderToken := createDriveFolder(t, parentT, ctx, "lark-cli-e2e-drive-dup-push-"+suffix, "")

		workDir := t.TempDir()
		if err := os.MkdirAll(filepath.Join(workDir, "local"), 0o755); err != nil {
			t.Fatalf("mkdir local: %v", err)
		}
		if err := os.WriteFile(filepath.Join(workDir, "local", "dup.txt"), []byte("local-overwrite"), 0o644); err != nil {
			t.Fatalf("write local dup.txt: %v", err)
		}

		_ = uploadNamedFile(t, workDir, folderToken, "_push_dup_first.txt", "dup.txt", "remote-first")
		time.Sleep(1200 * time.Millisecond)
		_ = uploadNamedFile(t, workDir, folderToken, "_push_dup_second.txt", "dup.txt", "remote-second")

		pushResult, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"drive", "+push",
				"--local-dir", "local",
				"--folder-token", folderToken,
				"--if-exists", "overwrite",
				"--on-duplicate-remote", "newest",
				"--delete-remote",
				"--yes",
			},
			WorkDir:   workDir,
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		pushResult.AssertExitCode(t, 0)
		pushResult.AssertStdoutStatus(t, true)
		if got := gjson.Get(pushResult.Stdout, "data.summary.uploaded").Int(); got != 1 {
			t.Fatalf("+push uploaded=%d, want 1\nstdout:\n%s", got, pushResult.Stdout)
		}
		if got := gjson.Get(pushResult.Stdout, "data.summary.deleted_remote").Int(); got != 1 {
			t.Fatalf("+push deleted_remote=%d, want 1\nstdout:\n%s", got, pushResult.Stdout)
		}

		statusResult, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"drive", "+status",
				"--local-dir", "local",
				"--folder-token", folderToken,
			},
			WorkDir:   workDir,
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		skipDriveStatusExactIfMissingDownloadScope(t, statusResult)
		statusResult.AssertExitCode(t, 0)
		statusResult.AssertStdoutStatus(t, true)
		if got := gjson.Get(statusResult.Stdout, "data.unchanged.#").Int(); got != 1 {
			t.Fatalf("+status unchanged count=%d, want 1\nstdout:\n%s", got, statusResult.Stdout)
		}
		if got := gjson.Get(statusResult.Stdout, "data.unchanged.0.rel_path").String(); got != "dup.txt" {
			t.Fatalf("+status unchanged rel_path=%q, want dup.txt\nstdout:\n%s", got, statusResult.Stdout)
		}
		if got := gjson.Get(statusResult.Stdout, "data.modified.#").Int(); got != 0 ||
			gjson.Get(statusResult.Stdout, "data.new_local.#").Int() != 0 ||
			gjson.Get(statusResult.Stdout, "data.new_remote.#").Int() != 0 {
			t.Fatalf("+status should converge to a clean unchanged mirror\nstdout:\n%s", statusResult.Stdout)
		}
	})

	t.Run("push overwrites nested remote file under its real parent", func(t *testing.T) {
		suffix := clie2e.GenerateSuffix()
		folderToken := createDriveFolder(t, parentT, ctx, "lark-cli-e2e-drive-nested-push-"+suffix, "")
		subFolderToken := createDriveFolder(t, parentT, ctx, "sub", folderToken)

		workDir := t.TempDir()
		if err := os.MkdirAll(filepath.Join(workDir, "local", "sub"), 0o755); err != nil {
			t.Fatalf("mkdir local/sub: %v", err)
		}
		if err := os.WriteFile(filepath.Join(workDir, "local", "sub", "keep.txt"), []byte("local-nested-overwrite"), 0o644); err != nil {
			t.Fatalf("write local/sub/keep.txt: %v", err)
		}

		existingToken := uploadNamedFile(t, workDir, subFolderToken, "_nested_keep.txt", "keep.txt", "remote-before")

		pushResult, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"drive", "+push",
				"--local-dir", "local",
				"--folder-token", folderToken,
				"--if-exists", "overwrite",
			},
			WorkDir:   workDir,
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		pushResult.AssertExitCode(t, 0)
		pushResult.AssertStdoutStatus(t, true)

		if got := gjson.Get(pushResult.Stdout, "data.summary.uploaded").Int(); got != 1 {
			t.Fatalf("nested +push uploaded=%d, want 1\nstdout:\n%s", got, pushResult.Stdout)
		}
		if got := gjson.Get(pushResult.Stdout, `data.items.#(rel_path="sub/keep.txt").action`).String(); got != "overwritten" {
			t.Fatalf("nested +push action=%q, want overwritten\nstdout:\n%s", got, pushResult.Stdout)
		}
		if got := gjson.Get(pushResult.Stdout, `data.items.#(rel_path="sub/keep.txt").file_token`).String(); got != existingToken {
			t.Fatalf("nested +push file_token=%q, want existing token %q\nstdout:\n%s", got, existingToken, pushResult.Stdout)
		}

		statusResult, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"drive", "+status",
				"--local-dir", "local",
				"--folder-token", folderToken,
			},
			WorkDir:   workDir,
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		skipDriveStatusExactIfMissingDownloadScope(t, statusResult)
		statusResult.AssertExitCode(t, 0)
		statusResult.AssertStdoutStatus(t, true)
		if got := gjson.Get(statusResult.Stdout, "data.unchanged.#").Int(); got != 1 {
			t.Fatalf("nested +status unchanged count=%d, want 1\nstdout:\n%s", got, statusResult.Stdout)
		}
		if got := gjson.Get(statusResult.Stdout, "data.unchanged.0.rel_path").String(); got != "sub/keep.txt" {
			t.Fatalf("nested +status unchanged rel_path=%q, want sub/keep.txt\nstdout:\n%s", got, statusResult.Stdout)
		}
		if got := gjson.Get(statusResult.Stdout, "data.modified.#").Int(); got != 0 ||
			gjson.Get(statusResult.Stdout, "data.new_local.#").Int() != 0 ||
			gjson.Get(statusResult.Stdout, "data.new_remote.#").Int() != 0 {
			t.Fatalf("nested overwrite should converge to a clean unchanged mirror\nstdout:\n%s", statusResult.Stdout)
		}
	})
}
