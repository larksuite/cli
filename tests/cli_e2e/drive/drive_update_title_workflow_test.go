// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// TestDrive_UpdateTitleWorkflow renames a file and the folder holding it, and
// proves the new titles are what a later metadata read returns.
func TestDrive_UpdateTitleWorkflow(t *testing.T) {
	clie2e.SkipWithoutTenantAccessToken(t)
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)

	suffix := clie2e.GenerateSuffix()
	folderToken := createDriveFolder(t, parentT, ctx, "lark-cli-e2e-drive-update-title-"+suffix, "")
	workDir := t.TempDir()

	sourceName := "update-title-source.txt"
	if err := os.WriteFile(filepath.Join(workDir, sourceName), []byte("drive update-title e2e\n"), 0o644); err != nil {
		t.Fatalf("write source file: %v", err)
	}

	uploadResult, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"drive", "+upload",
			"--file", sourceName,
			"--folder-token", folderToken,
		},
		WorkDir:   workDir,
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	uploadResult.AssertExitCode(t, 0)
	uploadResult.AssertStdoutStatus(t, true)

	fileToken := gjson.Get(uploadResult.Stdout, "data.file_token").String()
	require.NotEmpty(t, fileToken, "uploaded file should have a token, stdout:\n%s", uploadResult.Stdout)
	parentT.Cleanup(func() {
		cleanupCtx, cleanupCancel := clie2e.CleanupContext()
		defer cleanupCancel()

		deleteResult, deleteErr := clie2e.RunCmdWithRetry(cleanupCtx, clie2e.Request{
			Args:      []string{"drive", "+delete", "--file-token", fileToken, "--type", "file", "--yes"},
			DefaultAs: "bot",
		}, clie2e.RetryOptions{})
		clie2e.ReportCleanupFailure(parentT, "delete drive file "+fileToken, deleteResult, deleteErr)
	})

	// The title replaces the whole file name, so the extension has to be part of
	// it for the renamed file to stay a .txt file.
	renamedFileName := "update-title-renamed-" + suffix + ".txt"
	renameFileResult, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"drive", "+update-title",
			"--token", fileToken,
			"--type", "file",
			"--title", renamedFileName,
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	renameFileResult.AssertExitCode(t, 0)
	renameFileResult.AssertStdoutStatus(t, true)

	if got := gjson.Get(renameFileResult.Stdout, "data.updated").Bool(); !got {
		t.Fatalf("data.updated=false, want true\nstdout:\n%s", renameFileResult.Stdout)
	}
	if got := gjson.Get(renameFileResult.Stdout, "data.file_token").String(); got != fileToken {
		t.Fatalf("data.file_token=%q, want %q\nstdout:\n%s", got, fileToken, renameFileResult.Stdout)
	}
	if got := gjson.Get(renameFileResult.Stdout, "data.title").String(); got != renamedFileName {
		t.Fatalf("data.title=%q, want %q\nstdout:\n%s", got, renamedFileName, renameFileResult.Stdout)
	}
	if got := gjson.Get(renameFileResult.Stdout, "data.type").String(); got != "file" {
		t.Fatalf("data.type=%q, want file\nstdout:\n%s", got, renameFileResult.Stdout)
	}
	assertDriveTitle(t, ctx, fileToken, "file", renamedFileName)

	// Default keep policy: a title without an extension gains the current one
	// instead of silently turning the file into an extension-less blob.
	keptName := "update-title-kept-" + suffix
	keepResult, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"drive", "+update-title",
			"--token", fileToken,
			"--type", "file",
			"--title", keptName,
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	keepResult.AssertExitCode(t, 0)
	keepResult.AssertStdoutStatus(t, true)

	if got := gjson.Get(keepResult.Stdout, "data.extension_appended").String(); got != ".txt" {
		t.Fatalf("data.extension_appended=%q, want .txt\nstdout:\n%s", got, keepResult.Stdout)
	}
	if got := gjson.Get(keepResult.Stdout, "data.previous_title").String(); got != renamedFileName {
		t.Fatalf("data.previous_title=%q, want %q\nstdout:\n%s", got, renamedFileName, keepResult.Stdout)
	}
	if got := gjson.Get(keepResult.Stdout, "data.title").String(); got != keptName+".txt" {
		t.Fatalf("data.title=%q, want %q\nstdout:\n%s", got, keptName+".txt", keepResult.Stdout)
	}
	assertDriveTitle(t, ctx, fileToken, "file", keptName+".txt")

	// allow submits the title verbatim, which is the only way to drop the
	// extension on purpose.
	strippedName := "update-title-stripped-" + suffix
	allowResult, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"drive", "+update-title",
			"--token", fileToken,
			"--type", "file",
			"--title", strippedName,
			"--on-extension-mismatch", "allow",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	allowResult.AssertExitCode(t, 0)
	allowResult.AssertStdoutStatus(t, true)
	if allowResult.Stdout != "" && gjson.Get(allowResult.Stdout, "data.previous_title").Exists() {
		t.Fatalf("allow should not read the current title, so previous_title must be absent\nstdout:\n%s", allowResult.Stdout)
	}
	assertDriveTitle(t, ctx, fileToken, "file", strippedName)

	// Renaming the folder keeps its token, so the cleanup hook registered above
	// still resolves it.
	renamedFolderName := "lark-cli-e2e-drive-update-title-renamed-" + suffix
	renameFolderResult, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"drive", "+update-title",
			"--token", folderToken,
			"--type", "folder",
			"--new-title", renamedFolderName,
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	renameFolderResult.AssertExitCode(t, 0)
	renameFolderResult.AssertStdoutStatus(t, true)
	assertDriveTitle(t, ctx, folderToken, "folder", renamedFolderName)

	// A type that does not match the token is a lookup failure, not a type
	// error: the endpoint reports 981003 for both.
	mismatchResult, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"drive", "+update-title",
			"--token", fileToken,
			"--type", "docx",
			"--title", "should not apply",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	if mismatchResult.ExitCode == 0 {
		t.Fatalf("renaming a file as docx should fail\nstdout:\n%s\nstderr:\n%s", mismatchResult.Stdout, mismatchResult.Stderr)
	}
	if got := gjson.Get(mismatchResult.Stderr, "error.code").Int(); got != 981003 {
		t.Fatalf("error.code=%d, want 981003\nstderr:\n%s", got, mismatchResult.Stderr)
	}
	assertDriveTitle(t, ctx, fileToken, "file", strippedName)
}

func assertDriveTitle(t *testing.T, ctx context.Context, token, docType, wantTitle string) {
	t.Helper()

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      []string{"drive", "+inspect", "--url", token, "--type", docType},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	result.AssertStdoutStatus(t, true)

	if got := gjson.Get(result.Stdout, "data.title").String(); got != wantTitle {
		t.Fatalf("inspected title for %s/%s = %q, want %q\nstdout:\n%s", docType, token, got, wantTitle, result.Stdout)
	}
}
