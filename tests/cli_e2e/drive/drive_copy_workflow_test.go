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

func TestDrive_CopyWorkflow(t *testing.T) {
	clie2e.SkipWithoutTenantAccessToken(t)
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)

	suffix := clie2e.GenerateSuffix()
	folderToken := createDriveFolder(t, parentT, ctx, "lark-cli-e2e-drive-copy-"+suffix, "")
	workDir := t.TempDir()

	scheduleDelete := func(fileToken string) {
		t.Helper()
		if fileToken == "" {
			return
		}
		parentT.Cleanup(func() {
			cleanupCtx, cleanupCancel := clie2e.CleanupContext()
			defer cleanupCancel()

			deleteResult, deleteErr := clie2e.RunCmdWithRetry(cleanupCtx, clie2e.Request{
				Args:      []string{"drive", "+delete", "--file-token", fileToken, "--type", "file", "--yes"},
				DefaultAs: "bot",
			}, clie2e.RetryOptions{})
			clie2e.ReportCleanupFailure(parentT, "delete drive file "+fileToken, deleteResult, deleteErr)
		})
	}

	sourceContent := "drive copy e2e: source content\n"
	sourcePath := filepath.Join(workDir, "copy-source.txt")
	if err := os.WriteFile(sourcePath, []byte(sourceContent), 0o644); err != nil {
		t.Fatalf("write source file: %v", err)
	}

	uploadResult, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"drive", "+upload",
			"--file", "copy-source.txt",
			"--folder-token", folderToken,
		},
		WorkDir:   workDir,
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	uploadResult.AssertExitCode(t, 0)
	uploadResult.AssertStdoutStatus(t, true)
	sourceToken := gjson.Get(uploadResult.Stdout, "data.file_token").String()
	require.NotEmpty(t, sourceToken, "uploaded source should have a token, stdout:\n%s", uploadResult.Stdout)
	scheduleDelete(sourceToken)

	copyResult, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"drive", "+copy",
			"--token", sourceToken,
			"--type", "file",
			"--name", "copy-result.txt",
			"--folder-token", folderToken,
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	copyResult.AssertExitCode(t, 0)
	copyResult.AssertStdoutStatus(t, true)

	copiedToken := gjson.Get(copyResult.Stdout, "data.file_token").String()
	require.NotEmpty(t, copiedToken, "copy should return the new file token, stdout:\n%s", copyResult.Stdout)
	scheduleDelete(copiedToken)
	if copiedToken == sourceToken {
		t.Fatalf("copied token should differ from source token %q\nstdout:\n%s", sourceToken, copyResult.Stdout)
	}
	if got := gjson.Get(copyResult.Stdout, "data.name").String(); got != "copy-result.txt" {
		t.Fatalf("data.name=%q, want copy-result.txt\nstdout:\n%s", got, copyResult.Stdout)
	}
	if got := gjson.Get(copyResult.Stdout, "data.file_type").String(); got != "file" {
		t.Fatalf("data.file_type=%q, want file\nstdout:\n%s", got, copyResult.Stdout)
	}
	if got := gjson.Get(copyResult.Stdout, "data.source_file_token").String(); got != sourceToken {
		t.Fatalf("data.source_file_token=%q, want %q\nstdout:\n%s", got, sourceToken, copyResult.Stdout)
	}

	downloadResult, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"drive", "+download",
			"--file-token", copiedToken,
			"--output", "copy-downloaded.txt",
			"--overwrite",
		},
		WorkDir:   workDir,
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	downloadResult.AssertExitCode(t, 0)
	downloadResult.AssertStdoutStatus(t, true)

	mySpaceResult, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"drive", "+copy",
			"--token", sourceToken,
			"--type", "file",
			"--name", "copy-result-my-space.txt",
			"--folder-token", "my_space",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	mySpaceResult.AssertExitCode(t, 0)
	mySpaceResult.AssertStdoutStatus(t, true)

	mySpaceToken := gjson.Get(mySpaceResult.Stdout, "data.file_token").String()
	require.NotEmpty(t, mySpaceToken, "my_space copy should return the new file token, stdout:\n%s", mySpaceResult.Stdout)
	scheduleDelete(mySpaceToken)
	if got := gjson.Get(mySpaceResult.Stdout, "data.folder_token").String(); got == "" || got == "my_space" {
		t.Fatalf("data.folder_token=%q, want the resolved My Space root token\nstdout:\n%s", got, mySpaceResult.Stdout)
	}

	data, err := os.ReadFile(filepath.Join(workDir, "copy-downloaded.txt"))
	if err != nil {
		t.Fatalf("read downloaded copy: %v", err)
	}
	if string(data) != sourceContent {
		t.Fatalf("copied content=%q want %q", string(data), sourceContent)
	}
}
