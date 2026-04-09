// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
)

// TestDrive_FileStatisticsWorkflow tests the file.statistics get resource command.
// Workflow: upload a file -> get file statistics.
func TestDrive_FileStatisticsWorkflow(t *testing.T) {
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	suffix := time.Now().UTC().Format("20060102-150405")

	fileToken := uploadTestFile(t, parentT, ctx, "file-statistics-"+suffix)
	require.NotEmpty(t, fileToken)

	t.Run("get - get file statistics", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"drive", "file.statistics", "get"},
			Params: map[string]any{
				"file_token": fileToken,
				"file_type":  "file",
			},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, 0)
	})
}

// TestDrive_FileViewRecordsWorkflow tests the file.view_records list resource command.
// Workflow: upload a file -> list view records.
func TestDrive_FileViewRecordsWorkflow(t *testing.T) {
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	suffix := time.Now().UTC().Format("20060102-150405")

	fileToken := uploadTestFile(t, parentT, ctx, "file-view-records-"+suffix)
	require.NotEmpty(t, fileToken)

	t.Run("list - get file view records", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"drive", "file.view_records", "list"},
			Params: map[string]any{
				"file_token": fileToken,
				"file_type":  "file",
				"page_size":  10,
			},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, 0)
	})
}