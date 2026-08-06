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

// TestDrive_ImportWorkflow exercises the complete Drive import workflow against the live API.
func TestDrive_ImportWorkflow(t *testing.T) {
	clie2e.SkipWithoutTenantAccessToken(t)

	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	t.Cleanup(cancel)

	workDir := t.TempDir()
	fileName := "import-" + clie2e.GenerateSuffix() + ".md"
	if err := os.WriteFile(filepath.Join(workDir, fileName), []byte("# lark-cli import e2e\n"), 0o644); err != nil {
		t.Fatalf("write import fixture: %v", err)
	}

	var importedToken string
	importedType := "docx"
	parentT.Cleanup(func() {
		if importedToken == "" {
			return
		}
		cleanupCtx, cleanupCancel := clie2e.CleanupContext()
		defer cleanupCancel()
		deleteResult, deleteErr := DeleteDriveResourceAndVerify(cleanupCtx, importedToken, importedType, "bot")
		clie2e.ReportCleanupFailure(parentT, "delete imported document "+importedToken, deleteResult, deleteErr)
	})

	importCtx, importCancel := context.WithTimeout(ctx, 90*time.Second)
	result, err := clie2e.RunCmd(importCtx, clie2e.Request{
		Args:      []string{"drive", "+import", "--file", fileName, "--type", "docx"},
		WorkDir:   workDir,
		DefaultAs: "bot",
	})
	importCancel()
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	result.AssertStdoutStatus(t, true)

	ticket := gjson.Get(result.Stdout, "data.ticket").String()
	require.NotEmpty(t, ticket, "import should return a task ticket, stdout:\n%s", result.Stdout)
	if got := gjson.Get(result.Stdout, "data.type").String(); got != "" {
		importedType = got
	}
	importedToken = gjson.Get(result.Stdout, "data.token").String()
	if importedToken == "" {
		importedToken, importedType = waitDriveImportReady(t, ctx, ticket, importedType)
	}
	require.NotEmpty(t, importedToken, "ready import should return a document token")

	for _, reportOnlyField := range []string{"data.file_scene", "data.scene", "data.operation"} {
		if gjson.Get(result.Stdout, reportOnlyField).Exists() {
			t.Fatalf("report-only field %q leaked into import stdout:\n%s", reportOnlyField, result.Stdout)
		}
	}
}

// waitDriveImportReady polls an import task until it returns a document token or
// the workflow timeout expires.
func waitDriveImportReady(t *testing.T, ctx context.Context, ticket, fallbackType string) (string, string) {
	t.Helper()
	deadline := time.NewTimer(90 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"drive", "+task_result", "--scenario", "import", "--ticket", ticket},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		if gjson.Get(result.Stdout, "data.failed").Bool() {
			t.Fatalf("import task failed: %s", result.Stdout)
		}
		if gjson.Get(result.Stdout, "data.ready").Bool() {
			docType := gjson.Get(result.Stdout, "data.type").String()
			if docType == "" {
				docType = fallbackType
			}
			return gjson.Get(result.Stdout, "data.token").String(), docType
		}

		select {
		case <-ctx.Done():
			t.Fatalf("wait for import task %s: %v", ticket, ctx.Err())
		case <-deadline.C:
			t.Fatalf("import task %s did not become ready within 90s", ticket)
		case <-ticker.C:
		}
	}
}
