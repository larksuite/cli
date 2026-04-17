// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"testing"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/tidwall/gjson"
)

// CreateDriveFolder creates a Drive folder, optionally under a parent folder, and
// deletes it during parent cleanup.
func CreateDriveFolder(t *testing.T, parentT *testing.T, ctx context.Context, name string, defaultAs string, parentFolderToken string) string {
	t.Helper()

	if defaultAs == "" {
		defaultAs = "bot"
	}

	args := []string{"drive", "+create-folder", "--name", name}
	if parentFolderToken != "" {
		args = append(args, "--folder-token", parentFolderToken)
	}

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      args,
		DefaultAs: defaultAs,
	})
	if err != nil {
		t.Fatalf("create drive folder %q: %v", name, err)
	}
	result.AssertExitCode(t, 0)
	result.AssertStdoutStatus(t, true)

	folderToken := gjson.Get(result.Stdout, "data.folder_token").String()
	if folderToken == "" {
		t.Fatalf("drive folder token should not be empty, stdout:\n%s", result.Stdout)
	}

	parentT.Cleanup(func() {
		cleanupCtx, cancel := clie2e.CleanupContext()
		defer cancel()

		deleteResult, deleteErr := clie2e.RunCmdWithRetry(cleanupCtx, clie2e.Request{
			Args:      []string{"drive", "+delete", "--file-token", folderToken, "--type", "folder", "--yes"},
			DefaultAs: defaultAs,
		}, clie2e.RetryOptions{})
		clie2e.ReportCleanupFailure(parentT, "delete drive folder "+folderToken, deleteResult, deleteErr)
	})

	return folderToken
}
