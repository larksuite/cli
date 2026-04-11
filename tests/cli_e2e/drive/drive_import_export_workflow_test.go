// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// TestDrive_ImportExportWorkflow tests the import and export shortcut methods.
// Workflow: import to drive as docx -> export docx -> verify exported file.
func TestDrive_ImportExportWorkflow(t *testing.T) {
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)

	suffix := time.Now().UTC().Format("20060102-150405")
	testContent := "# Lark CLI E2E Test\n\nThis is a test document created by drive import-export workflow test.\nTimestamp: " + suffix

	docToken := importTestDoc(t, parentT, ctx, "import-export", testContent)
	require.NotEmpty(t, docToken)

	t.Run("export", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"drive", "+export",
				"--token", docToken,
				"--doc-type", "docx",
				"--file-extension", "pdf",
			},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)

		// For PDF export, check if file was directly saved or if polling needed
		savedPath := gjson.Get(result.Stdout, "data.saved_path").String()
		if savedPath == "" {
			// Poll for completion if ticket returned (async case)
			exportTicket := gjson.Get(result.Stdout, "data.ticket").String()
			if exportTicket != "" {
				exportResult, exportErr := clie2e.RunCmd(ctx, clie2e.Request{
					Args: []string{"drive", "+task_result", "--ticket", exportTicket, "--scenario", "export", "--file-token", docToken},
				})
				require.NoError(t, exportErr)
				exportResult.AssertExitCode(t, 0)
				exportResult.AssertStdoutStatus(t, true)
			}
		}
	})
}