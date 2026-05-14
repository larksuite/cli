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

func TestDrive_UploadWorkflow(t *testing.T) {
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)

	suffix := clie2e.GenerateSuffix()
	folderToken := createDriveFolder(t, parentT, ctx, "lark-cli-e2e-drive-upload-"+suffix, "")
	workDir := t.TempDir()

	cleanupTokens := map[string]struct{}{}
	scheduleDelete := func(fileToken string) {
		t.Helper()
		if fileToken == "" {
			return
		}
		if _, seen := cleanupTokens[fileToken]; seen {
			return
		}
		cleanupTokens[fileToken] = struct{}{}
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

	uploadFile := func(stageName, remoteName, content, fileToken string) string {
		t.Helper()
		stagePath := filepath.Join(workDir, stageName)
		if err := os.WriteFile(stagePath, []byte(content), 0o644); err != nil {
			t.Fatalf("write stage file %s: %v", stageName, err)
		}

		args := []string{
			"drive", "+upload",
			"--file", stageName,
			"--folder-token", folderToken,
			"--name", remoteName,
		}
		if fileToken != "" {
			args = append(args, "--file-token", fileToken)
		}

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      args,
			WorkDir:   workDir,
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		gotToken := gjson.Get(result.Stdout, "data.file_token").String()
		require.NotEmpty(t, gotToken, "uploaded file should have a token, stdout:\n%s", result.Stdout)
		if got := gjson.Get(result.Stdout, "data.file_name").String(); got != remoteName {
			t.Fatalf("data.file_name=%q want %q\nstdout:\n%s", got, remoteName, result.Stdout)
		}
		if got := gjson.Get(result.Stdout, "data.size").Int(); got != int64(len(content)) {
			t.Fatalf("data.size=%d want %d\nstdout:\n%s", got, len(content), result.Stdout)
		}
		return gotToken
	}

	initialContent := "drive upload e2e: initial content\n"
	initialToken := uploadFile("_upload_initial.txt", "overwrite.txt", initialContent, "")
	scheduleDelete(initialToken)

	updatedContent := "drive upload e2e: overwritten via file-token\n"
	overwriteToken := uploadFile("_upload_overwrite.txt", "overwrite.txt", updatedContent, initialToken)
	scheduleDelete(overwriteToken)

	if overwriteToken != initialToken {
		t.Fatalf("overwrite token=%q want original token=%q", overwriteToken, initialToken)
	}

	downloadResult, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"drive", "+download",
			"--file-token", overwriteToken,
			"--output", "downloaded.txt",
			"--overwrite",
		},
		WorkDir:   workDir,
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	downloadResult.AssertExitCode(t, 0)
	downloadResult.AssertStdoutStatus(t, true)

	data, err := os.ReadFile(filepath.Join(workDir, "downloaded.txt"))
	if err != nil {
		t.Fatalf("read downloaded file: %v", err)
	}
	if string(data) != updatedContent {
		t.Fatalf("downloaded content=%q want %q", string(data), updatedContent)
	}
}
