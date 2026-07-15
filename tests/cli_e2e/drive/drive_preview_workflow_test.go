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

// TestDrive_PreviewAndCoverWorkflow verifies preview and cover shortcuts against
// a live Drive workflow, skipping when required bot scopes are unavailable.
func TestDrive_PreviewAndCoverWorkflow(t *testing.T) {
	clie2e.SkipWithoutTenantAccessToken(t)
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	t.Cleanup(cancel)

	suffix := clie2e.GenerateSuffix()
	folderName := "lark-cli-e2e-drive-preview-" + suffix
	folderToken := createDriveFolderOrSkipPermission(t, parentT, ctx, folderName)

	workDir := t.TempDir()
	sourceRelPath := "fixture/report.txt"
	sourceContent := "drive preview and cover workflow\n"
	writePreviewFixture(t, workDir, sourceRelPath, sourceContent)

	fileToken := uploadPreviewFixture(t, parentT, ctx, workDir, folderToken, sourceRelPath, "report.txt")

	t.Run("preview list and download", func(t *testing.T) {
		listResult, err := clie2e.RunCmdWithRetry(ctx, clie2e.Request{
			Args: []string{
				"drive", "+preview",
				"--file-token", fileToken,
				"--list-only",
			},
			DefaultAs: "bot",
		}, clie2e.RetryOptions{
			Attempts:        8,
			InitialDelay:    2 * time.Second,
			MaxDelay:        8 * time.Second,
			BackoffMultiple: 2,
			ShouldRetry: func(result *clie2e.Result) bool {
				if result == nil || result.ExitCode != 0 {
					return true
				}
				return !previewListContainsReadyType(result.Stdout, "text")
			},
		})
		require.NoError(t, err)
		listResult.AssertExitCode(t, 0)
		listResult.AssertStdoutStatus(t, true)

		if !previewListContainsReadyType(listResult.Stdout, "text") {
			t.Fatalf("preview list did not expose downloadable text preview\nstdout:\n%s", listResult.Stdout)
		}

		downloadDir := t.TempDir()
		downloadResult, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"drive", "+preview",
				"--file-token", fileToken,
				"--type", "text",
				"--output", "./artifacts/report-preview",
			},
			WorkDir:   downloadDir,
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		downloadResult.AssertExitCode(t, 0)
		downloadResult.AssertStdoutStatus(t, true)

		stdout := downloadResult.Stdout
		if got := gjson.Get(stdout, "data.selected_type").String(); got != "text" {
			t.Fatalf("selected_type=%q, want text\nstdout:\n%s", got, stdout)
		}
		outputPath := gjson.Get(stdout, "data.output_path").String()
		require.NotEmpty(t, outputPath, "preview download should return output_path")
		if ext := filepath.Ext(outputPath); ext != ".txt" {
			t.Fatalf("preview output extension=%q, want .txt\nstdout:\n%s", ext, stdout)
		}
		data, readErr := os.ReadFile(outputPath)
		require.NoError(t, readErr)
		if !strings.Contains(string(data), "drive preview and cover workflow") {
			t.Fatalf("preview artifact content mismatch: %q", string(data))
		}
	})

	t.Run("cover list and download", func(t *testing.T) {
		listResult, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"drive", "+cover",
				"--file-token", fileToken,
				"--list-only",
			},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		listResult.AssertExitCode(t, 0)
		listResult.AssertStdoutStatus(t, true)

		if !gjson.Get(listResult.Stdout, `data.candidates.#(spec=="default")`).Exists() {
			t.Fatalf("cover list missing default spec\nstdout:\n%s", listResult.Stdout)
		}

		downloadDir := t.TempDir()
		coverResult, err := clie2e.RunCmdWithRetry(ctx, clie2e.Request{
			Args: []string{
				"drive", "+cover",
				"--file-token", fileToken,
				"--spec", "default",
				"--output", "./artifacts/report-cover",
			},
			WorkDir:   downloadDir,
			DefaultAs: "bot",
		}, clie2e.RetryOptions{
			Attempts:        8,
			InitialDelay:    2 * time.Second,
			MaxDelay:        8 * time.Second,
			BackoffMultiple: 2,
			ShouldRetry: func(result *clie2e.Result) bool {
				if result == nil {
					return true
				}
				if result.ExitCode == 0 {
					return false
				}
				return false
			},
		})
		require.NoError(t, err)
		coverResult.AssertExitCode(t, 0)
		coverResult.AssertStdoutStatus(t, true)

		stdout := coverResult.Stdout
		if got := gjson.Get(stdout, "data.selected_spec").String(); got != "default" {
			t.Fatalf("selected_spec=%q, want default\nstdout:\n%s", got, stdout)
		}
		outputPath := gjson.Get(stdout, "data.output_path").String()
		require.NotEmpty(t, outputPath, "cover download should return output_path")
		if ext := filepath.Ext(outputPath); ext != ".png" && ext != ".jpg" && ext != ".jpeg" && ext != ".webp" {
			t.Fatalf("cover output extension=%q, want image extension\nstdout:\n%s", ext, stdout)
		}
		info, statErr := os.Stat(outputPath)
		require.NoError(t, statErr)
		if info.Size() <= 0 {
			t.Fatalf("cover artifact should not be empty: %s", outputPath)
		}
	})
}

// writePreviewFixture writes a local fixture file used by the live workflow.
func writePreviewFixture(t *testing.T, workDir, relPath, content string) {
	t.Helper()

	fullPath := filepath.Join(workDir, relPath)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatalf("mkdir fixture parent: %v", err)
	}
	if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
}

// uploadPreviewFixture uploads a fixture into Drive and registers cleanup for
// the created file token.
func uploadPreviewFixture(t *testing.T, parentT *testing.T, ctx context.Context, workDir, folderToken, relPath, uploadName string) string {
	t.Helper()

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
	result.AssertExitCode(t, 0)
	result.AssertStdoutStatus(t, true)

	fileToken := gjson.Get(result.Stdout, "data.file_token").String()
	require.NotEmpty(t, fileToken, "uploaded file should have a token")

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

// previewListContainsReadyType reports whether a preview list response contains
// a downloadable candidate for the requested type.
func previewListContainsReadyType(stdout, wantType string) bool {
	for _, candidate := range gjson.Get(stdout, "data.candidates").Array() {
		if candidate.Get("type").String() != wantType {
			continue
		}
		if candidate.Get("downloadable").Bool() {
			return true
		}
	}
	return false
}

// createDriveFolderOrSkipPermission creates a Drive folder for the live
// workflow and skips when the bot lacks required folder scopes.
func createDriveFolderOrSkipPermission(t *testing.T, parentT *testing.T, ctx context.Context, name string) string {
	t.Helper()

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      []string{"drive", "+create-folder", "--name", name},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	if result.ExitCode != 0 {
		combinedOutput := strings.ToLower(result.Stdout + "\n" + result.Stderr)
		if strings.Contains(combinedOutput, "app scope not enabled") ||
			strings.Contains(combinedOutput, "space:folder:create") ||
			strings.Contains(combinedOutput, "99991672") {
			t.Skipf("skip drive preview/cover workflow due to missing bot scope space:folder:create: %s", strings.TrimSpace(result.Stdout+"\n"+result.Stderr))
		}
	}
	result.AssertExitCode(t, 0)
	result.AssertStdoutStatus(t, true)

	folderToken := gjson.Get(result.Stdout, "data.folder_token").String()
	require.NotEmpty(t, folderToken, "drive folder token should not be empty")

	parentT.Cleanup(func() {
		cleanupCtx, cancel := clie2e.CleanupContext()
		defer cancel()

		deleteResult, deleteErr := clie2e.RunCmdWithRetry(cleanupCtx, clie2e.Request{
			Args:      []string{"drive", "+delete", "--file-token", folderToken, "--type", "folder", "--yes"},
			DefaultAs: "bot",
		}, clie2e.RetryOptions{})
		clie2e.ReportCleanupFailure(parentT, "delete drive folder "+folderToken, deleteResult, deleteErr)
	})

	return folderToken
}
