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

func TestDriveVersionWorkflow(t *testing.T) {
	clie2e.SkipWithoutTenantAccessToken(t)
	if os.Getenv("LARK_DRIVE_VERSION_E2E") == "" {
		t.Skip("set LARK_DRIVE_VERSION_E2E=1 to run drive version live workflow")
	}

	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)

	suffix := clie2e.GenerateSuffix()
	fileName := "lark-cli-version-workflow-" + suffix + ".md"

	createResult, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"markdown", "+create",
			"--name", fileName,
			"--content", "# v1\n",
		},
		DefaultAs:  "bot",
		BinaryPath: "../../../lark-cli",
	})
	require.NoError(t, err)
	createResult.AssertExitCode(t, 0)
	createResult.AssertStdoutStatus(t, true)

	fileToken := gjson.Get(createResult.Stdout, "data.file_token").String()
	require.NotEmpty(t, fileToken, "stdout:\n%s", createResult.Stdout)

	parentT.Cleanup(func() {
		cleanupCtx, cleanupCancel := clie2e.CleanupContext()
		defer cleanupCancel()

		deleteResult, deleteErr := clie2e.RunCmd(cleanupCtx, clie2e.Request{
			Args: []string{
				"drive", "+delete",
				"--file-token", fileToken,
				"--type", "file",
				"--yes",
			},
			DefaultAs:  "bot",
			BinaryPath: "../../../lark-cli",
		})
		clie2e.ReportCleanupFailure(parentT, "delete version workflow file "+fileToken, deleteResult, deleteErr)
	})

	overwriteResult, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"markdown", "+overwrite",
			"--file-token", fileToken,
			"--content", "# v2\n",
		},
		DefaultAs:  "bot",
		BinaryPath: "../../../lark-cli",
	})
	require.NoError(t, err)
	overwriteResult.AssertExitCode(t, 0)
	overwriteResult.AssertStdoutStatus(t, true)

	overwriteResult, err = clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"markdown", "+overwrite",
			"--file-token", fileToken,
			"--content", "# v3\n",
		},
		DefaultAs:  "bot",
		BinaryPath: "../../../lark-cli",
	})
	require.NoError(t, err)
	overwriteResult.AssertExitCode(t, 0)
	overwriteResult.AssertStdoutStatus(t, true)

	historyResult, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"drive", "+version-history",
			"--file-token", fileToken,
		},
		DefaultAs:  "bot",
		BinaryPath: "../../../lark-cli",
	})
	require.NoError(t, err)
	historyResult.AssertExitCode(t, 0)
	historyResult.AssertStdoutStatus(t, true)

	versions := gjson.Get(historyResult.Stdout, "data.versions").Array()
	require.GreaterOrEqual(t, len(versions), 3, "stdout:\n%s", historyResult.Stdout)

	var (
		versionToDownload string
		versionV1         string
		versionV2         string
	)
	for _, version := range versions {
		versionID := version.Get("version").String()
		if versionID == "" {
			continue
		}
		downloadDir := t.TempDir()
		downloadResult, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"drive", "+version-get",
				"--file-token", fileToken,
				"--version", versionID,
			},
			DefaultAs:  "bot",
			BinaryPath: "../../../lark-cli",
			WorkDir:    downloadDir,
		})
		require.NoError(t, err)
		downloadResult.AssertExitCode(t, 0)
		downloadResult.AssertStdoutStatus(t, true)

		downloadedPath := filepath.Join(downloadDir, fileName)
		body, err := os.ReadFile(downloadedPath)
		require.NoError(t, err)

		switch string(body) {
		case "# v1\n":
			versionV1 = versionID
		case "# v2\n":
			versionV2 = versionID
		}
		if versionToDownload == "" {
			versionToDownload = versionID
		}
	}
	require.NotEmpty(t, versionToDownload, "stdout:\n%s", historyResult.Stdout)
	require.NotEmpty(t, versionV1, "stdout:\n%s", historyResult.Stdout)
	require.NotEmpty(t, versionV2, "stdout:\n%s", historyResult.Stdout)

	downloadDir := t.TempDir()
	downloadResult, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"drive", "+version-get",
			"--file-token", fileToken,
			"--version", versionToDownload,
		},
		DefaultAs:  "bot",
		BinaryPath: "../../../lark-cli",
		WorkDir:    downloadDir,
	})
	require.NoError(t, err)
	downloadResult.AssertExitCode(t, 0)
	downloadResult.AssertStdoutStatus(t, true)

	downloadedPath := filepath.Join(downloadDir, fileName)
	if _, err := os.Stat(downloadedPath); err != nil {
		t.Fatalf("expected downloaded version at %q: %v", downloadedPath, err)
	}

	revertResult, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"drive", "+version-revert",
			"--file-token", fileToken,
			"--version", versionV1,
		},
		DefaultAs:  "bot",
		BinaryPath: "../../../lark-cli",
	})
	require.NoError(t, err)
	revertResult.AssertExitCode(t, 0)
	revertResult.AssertStdoutStatus(t, true)

	fetchResult, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"markdown", "+fetch",
			"--file-token", fileToken,
		},
		DefaultAs:  "bot",
		BinaryPath: "../../../lark-cli",
	})
	require.NoError(t, err)
	fetchResult.AssertExitCode(t, 0)
	fetchResult.AssertStdoutStatus(t, true)
	if got := gjson.Get(fetchResult.Stdout, "data.content").String(); got != "# v1\n" {
		t.Fatalf("markdown content after revert = %q, want %q\nstdout:\n%s", got, "# v1\n", fetchResult.Stdout)
	}

	deleteResult, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"drive", "+version-delete",
			"--file-token", fileToken,
			"--version", versionV2,
		},
		DefaultAs:  "bot",
		BinaryPath: "../../../lark-cli",
		Yes:        true,
	})
	require.NoError(t, err)
	deleteResult.AssertExitCode(t, 0)
	deleteResult.AssertStdoutStatus(t, true)

	historyAfterDelete, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"drive", "+version-history",
			"--file-token", fileToken,
		},
		DefaultAs:  "bot",
		BinaryPath: "../../../lark-cli",
	})
	require.NoError(t, err)
	historyAfterDelete.AssertExitCode(t, 0)
	historyAfterDelete.AssertStdoutStatus(t, true)

	foundDeletedVersion := false
	for _, version := range gjson.Get(historyAfterDelete.Stdout, "data.versions").Array() {
		if version.Get("version").String() != versionV2 {
			continue
		}
		foundDeletedVersion = true
		if !version.Get("is_deleted").Bool() {
			t.Fatalf("version %s should be marked deleted after +version-delete\nstdout:\n%s", versionV2, historyAfterDelete.Stdout)
		}
	}
	if !foundDeletedVersion {
		t.Fatalf("deleted version %s not found in history after delete\nstdout:\n%s", versionV2, historyAfterDelete.Stdout)
	}
}
