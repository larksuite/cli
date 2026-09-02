// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package wiki

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	drivee2e "github.com/larksuite/cli/tests/cli_e2e/drive"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// TestDrive_DownloadPreviewWikiSourceWorkflow exercises the live wiki-source
// path added to drive +download / +preview end to end: it uploads a fixture
// file, moves it into Wiki to produce a node whose get_node resolves to
// obj_type=="file", then downloads and previews that file through both a
// --wiki-token and a /wiki/ --url. It also pins the contract that a wiki node
// wrapping a document (the docx parent host) is rejected with a typed
// validation error pointing at drive +export. The whole flow is self-contained
// and skips when the bot lacks the required Drive/Wiki scopes.
func TestDrive_DownloadPreviewWikiSourceWorkflow(t *testing.T) {
	clie2e.SkipWithoutTenantAccessToken(t)
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	t.Cleanup(cancel)

	suffix := clie2e.GenerateSuffix()
	fixtureContent := "drive download/preview wiki source workflow " + suffix + "\n"

	// 1) Isolated Drive folder to stage the fixture (skip on missing scope).
	folderToken := createDriveFolderOrSkipWikiSource(t, parentT, ctx, "lark-cli-e2e-drive-wiki-src-"+suffix)

	// 2) Upload the fixture file we will later reach through a wiki node.
	fileToken := uploadWikiSourceFixture(t, parentT, ctx, folderToken, "wiki-source.txt", fixtureContent)

	// 3) Parent wiki node (a docx origin node) that both hosts the moved file
	//    and doubles as the document-node rejection fixture. Its cleanup deletes
	//    child nodes recursively, so the moved file node is reclaimed with it.
	_, parentNode := createWikiNodeUnderAnyHost(t, parentT, ctx, "lark-cli-e2e-drive-wiki-src-node-"+suffix)
	spaceID := parentNode.Get("space_id").String()
	docNodeToken := parentNode.Get("node_token").String()
	require.NotEmpty(t, spaceID, "parent wiki node must expose space_id; node:\n%s", parentNode.Raw)
	require.NotEmpty(t, docNodeToken, "parent wiki node must expose node_token; node:\n%s", parentNode.Raw)

	// 4) Move the Drive file into Wiki -> a wiki node whose get_node resolves to
	//    obj_type=="file". This is exactly the shape drive +download / +preview
	//    must accept.
	fileNodeToken := moveDriveFileIntoWikiOrSkip(t, ctx, spaceID, docNodeToken, fileToken)
	wikiURL := "https://example.feishu.cn/wiki/" + fileNodeToken

	t.Run("download via wiki token", func(t *testing.T) {
		assertWikiSourceDownload(t, ctx, []string{"--wiki-token", fileNodeToken}, fileNodeToken, fixtureContent)
	})

	t.Run("download via wiki url", func(t *testing.T) {
		assertWikiSourceDownload(t, ctx, []string{"--url", wikiURL}, fileNodeToken, fixtureContent)
	})

	t.Run("preview source file via wiki token", func(t *testing.T) {
		workDir := t.TempDir()
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"drive", "+preview",
				"--wiki-token", fileNodeToken,
				"--type", "source_file",
				"--output", "./artifacts/wiki-preview",
			},
			WorkDir:   workDir,
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		if result.ExitCode != 0 && isWikiSourceScopeSkip(result) {
			t.Skipf("skip wiki preview due to missing scope:\n%s", strings.TrimSpace(result.Stderr))
		}
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		stdout := result.Stdout
		require.Equal(t, "source_file", gjson.Get(stdout, "data.selected_type").String(), "stdout:\n%s", stdout)
		require.Equal(t, fileNodeToken, gjson.Get(stdout, "data.wiki_token").String(), "stdout:\n%s", stdout)
		require.Equal(t, "file", gjson.Get(stdout, "data.wiki_node.obj_type").String(), "stdout:\n%s", stdout)
		require.NotEmpty(t, gjson.Get(stdout, "data.wiki_node.obj_token").String(), "stdout:\n%s", stdout)

		outputPath := gjson.Get(stdout, "data.output_path").String()
		require.NotEmpty(t, outputPath, "preview source file should return output_path\nstdout:\n%s", stdout)
		data, readErr := os.ReadFile(outputPath)
		require.NoError(t, readErr)
		require.Equal(t, fixtureContent, string(data), "preview source content mismatch")
	})

	t.Run("rejects wiki node that wraps a document", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"drive", "+download",
				"--wiki-token", docNodeToken,
			},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		if result.ExitCode != 0 && isWikiSourceScopeSkip(result) {
			t.Skipf("skip document-node rejection due to missing scope:\n%s", strings.TrimSpace(result.Stderr))
		}

		result.AssertExitCode(t, 2)
		if strings.TrimSpace(result.Stdout) != "" {
			t.Fatalf("stdout must stay empty on validation failure, got:\n%s", result.Stdout)
		}
		require.Equal(t, "validation", gjson.Get(result.Stderr, "error.type").String(), "stderr:\n%s", result.Stderr)
		require.Equal(t, "invalid_argument", gjson.Get(result.Stderr, "error.subtype").String(), "stderr:\n%s", result.Stderr)
		require.Equal(t, "--wiki-token", gjson.Get(result.Stderr, "error.param").String(), "stderr:\n%s", result.Stderr)
		require.NotEmpty(t, gjson.Get(result.Stderr, "error.message").String(), "stderr:\n%s", result.Stderr)
		require.Contains(t, gjson.Get(result.Stderr, "error.hint").String(), "+export", "stderr:\n%s", result.Stderr)
	})
}

// assertWikiSourceDownload runs drive +download for a wiki source (token or URL)
// and verifies the resolution annotation plus the downloaded bytes.
func assertWikiSourceDownload(t *testing.T, ctx context.Context, sourceArgs []string, wantWikiToken, wantContent string) {
	t.Helper()

	workDir := t.TempDir()
	args := append([]string{"drive", "+download"}, sourceArgs...)
	args = append(args, "--output", "downloaded.txt", "--overwrite")

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      args,
		WorkDir:   workDir,
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	if result.ExitCode != 0 && isWikiSourceScopeSkip(result) {
		t.Skipf("skip wiki download due to missing scope:\n%s", strings.TrimSpace(result.Stderr))
	}
	result.AssertExitCode(t, 0)
	result.AssertStdoutStatus(t, true)

	stdout := result.Stdout
	require.Equal(t, wantWikiToken, gjson.Get(stdout, "data.wiki_token").String(), "stdout:\n%s", stdout)
	require.Equal(t, "file", gjson.Get(stdout, "data.wiki_node.obj_type").String(), "stdout:\n%s", stdout)
	require.NotEmpty(t, gjson.Get(stdout, "data.wiki_node.obj_token").String(), "stdout:\n%s", stdout)

	require.NotEmpty(t, gjson.Get(stdout, "data.saved_path").String(), "download should return saved_path\nstdout:\n%s", stdout)
	data, readErr := os.ReadFile(filepath.Join(workDir, "downloaded.txt"))
	require.NoError(t, readErr)
	require.Equal(t, wantContent, string(data), "downloaded content mismatch")
}

// moveDriveFileIntoWikiOrSkip moves an uploaded Drive file under a wiki parent
// node and returns the resulting wiki node token (obj_type=="file"). It handles
// both the immediate and async move responses and skips when scopes are missing.
func moveDriveFileIntoWikiOrSkip(t *testing.T, ctx context.Context, spaceID, parentNodeToken, fileToken string) string {
	t.Helper()

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"wiki", "+move",
			"--obj-type", "file",
			"--obj-token", fileToken,
			"--target-space-id", spaceID,
			"--target-parent-token", parentNodeToken,
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	if result.ExitCode != 0 && isWikiSourceScopeSkip(result) {
		t.Skipf("skip wiki source workflow: move file into wiki needs scopes:\n%s", strings.TrimSpace(result.Stderr))
	}
	result.AssertExitCode(t, 0)
	result.AssertStdoutStatus(t, true)

	nodeToken := gjson.Get(result.Stdout, "data.node_token").String()
	if nodeToken != "" {
		return nodeToken
	}

	taskID := gjson.Get(result.Stdout, "data.task_id").String()
	require.NotEmpty(t, taskID, "wiki move must return node_token or task_id\nstdout:\n%s", result.Stdout)
	return waitWikiMoveNodeReady(t, ctx, taskID)
}

// waitWikiMoveNodeReady polls drive +task_result until the async docs-to-wiki
// move finishes and returns the resulting wiki node token.
func waitWikiMoveNodeReady(t *testing.T, ctx context.Context, taskID string) string {
	t.Helper()

	var nodeToken string
	err := clie2e.WaitForCondition(ctx, clie2e.WaitOptions{
		Timeout:  90 * time.Second,
		Interval: 3 * time.Second,
		TimeoutError: func() error {
			return fmt.Errorf("wiki move task %s did not finish", taskID)
		},
	}, func() (bool, error) {
		result, runErr := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"drive", "+task_result",
				"--scenario", "wiki_move",
				"--task-id", taskID,
			},
			DefaultAs: "bot",
		})
		if runErr != nil {
			return false, runErr
		}
		if result.ExitCode != 0 {
			return false, fmt.Errorf(
				"query wiki move task failed: exit=%d stdout=%s stderr=%s",
				result.ExitCode, result.Stdout, result.Stderr,
			)
		}
		parsed := gjson.Parse(result.Stdout)
		if parsed.Get("data.failed").Bool() {
			return false, fmt.Errorf("wiki move task %s failed: %s", taskID, parsed.Get("data.status_msg").String())
		}
		if parsed.Get("data.ready").Bool() {
			nodeToken = parsed.Get("data.node_token").String()
			return true, nil
		}
		return false, nil
	})
	require.NoError(t, err)
	require.NotEmpty(t, nodeToken, "async wiki move must yield a node_token")
	return nodeToken
}

// createDriveFolderOrSkipWikiSource creates a staging Drive folder for the wiki
// source workflow, skipping when the bot lacks the folder-create scope.
func createDriveFolderOrSkipWikiSource(t *testing.T, parentT *testing.T, ctx context.Context, name string) string {
	t.Helper()

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      []string{"drive", "+create-folder", "--name", name},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	if result.ExitCode != 0 && isWikiSourceScopeSkip(result) {
		t.Skipf("skip wiki source workflow: create folder needs scope:\n%s", strings.TrimSpace(result.Stderr))
	}
	result.AssertExitCode(t, 0)
	result.AssertStdoutStatus(t, true)

	folderToken := gjson.Get(result.Stdout, "data.folder_token").String()
	require.NotEmpty(t, folderToken, "drive folder token should not be empty\nstdout:\n%s", result.Stdout)

	parentT.Cleanup(func() {
		cleanupCtx, cancel := clie2e.CleanupContext()
		defer cancel()

		deleteResult, deleteErr := drivee2e.DeleteDriveResourceAndVerify(cleanupCtx, folderToken, "folder", "bot")
		clie2e.ReportCleanupFailure(parentT, "delete drive folder "+folderToken, deleteResult, deleteErr)
	})

	return folderToken
}

// uploadWikiSourceFixture uploads a text fixture into Drive and registers
// cleanup for the underlying file token. After the file is moved into Wiki the
// delete resolves as already-gone, which DeleteDriveResourceAndVerify tolerates,
// so this cleanup also covers the skip path where the move never happens.
func uploadWikiSourceFixture(t *testing.T, parentT *testing.T, ctx context.Context, folderToken, uploadName, content string) string {
	t.Helper()

	workDir := t.TempDir()
	relPath := "wiki-fixture.txt"
	if err := os.WriteFile(filepath.Join(workDir, relPath), []byte(content), 0o644); err != nil {
		t.Fatalf("write wiki source fixture: %v", err)
	}

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"drive", "+upload",
			"--file", relPath,
			"--folder-token", folderToken,
			"--name", uploadName,
		},
		WorkDir:   workDir,
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	if result.ExitCode != 0 && isWikiSourceScopeSkip(result) {
		t.Skipf("skip wiki source workflow: upload needs scope:\n%s", strings.TrimSpace(result.Stderr))
	}
	result.AssertExitCode(t, 0)
	result.AssertStdoutStatus(t, true)

	fileToken := gjson.Get(result.Stdout, "data.file_token").String()
	require.NotEmpty(t, fileToken, "uploaded file should have a token\nstdout:\n%s", result.Stdout)

	parentT.Cleanup(func() {
		cleanupCtx, cancel := clie2e.CleanupContext()
		defer cancel()

		deleteResult, deleteErr := drivee2e.DeleteDriveResourceAndVerify(cleanupCtx, fileToken, "file", "bot")
		clie2e.ReportCleanupFailure(parentT, "delete drive file "+fileToken, deleteResult, deleteErr)
	})

	return fileToken
}

// isWikiSourceScopeSkip reports whether a failed result was caused by a missing
// bot scope, so the live workflow can skip instead of failing on tenants that
// have not granted the required Drive/Wiki permissions.
func isWikiSourceScopeSkip(result *clie2e.Result) bool {
	if result == nil {
		return false
	}
	combined := strings.ToLower(result.Stdout + "\n" + result.Stderr)
	for _, marker := range []string{
		"app scope not enabled",
		"99991672",
		"missing_scopes",
		"wiki:node:move",
		"wiki:node:read",
		"wiki:space:read",
		"wiki:node:retrieve",
		"space:folder:create",
		"drive:file:upload",
	} {
		if strings.Contains(combined, marker) {
			return true
		}
	}
	return false
}
