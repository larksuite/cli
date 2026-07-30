// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestBaseTableCopyWorkflow(t *testing.T) {
	if os.Getenv("LARK_CLI_E2E_BASE_TABLE_COPY_READY") != "1" {
		t.Skip("set LARK_CLI_E2E_BASE_TABLE_COPY_READY=1 after the table-copy OpenAPI is deployed")
	}
	clie2e.SkipWithoutTenantAccessToken(t)

	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	t.Cleanup(cancel)

	baseToken := createBaseWithRetry(t, ctx, "lark-cli-e2e-table-copy-"+clie2e.GenerateSuffix())
	sourceTableID, _, _ := createTableWithRetry(
		t,
		parentT,
		ctx,
		baseToken,
		"Copy source "+clie2e.GenerateSuffix(),
		`[{"name":"Name","type":"text"}]`,
		`{"name":"Main","type":"grid"}`,
	)

	seed, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"base", "+record-batch-create",
			"--base-token", baseToken,
			"--table-id", sourceTableID,
			"--json", `{"fields":["Name"],"rows":[["copied record"]]}`,
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	seed.AssertExitCode(t, 0)
	seed.AssertStdoutStatus(t, true)

	cleanupCopiedTable := func(tableID string) {
		t.Helper()
		require.NotEmpty(t, tableID)
		t.Cleanup(func() {
			cleanupCtx, cleanupCancel := cleanupContext()
			defer cleanupCancel()
			result, cleanupErr := clie2e.RunCmd(cleanupCtx, clie2e.Request{
				Args:      []string{"base", "+table-delete", "--base-token", baseToken, "--table-id", tableID, "--yes"},
				DefaultAs: "bot",
			})
			if cleanupErr != nil || result == nil || result.ExitCode != 0 {
				reportCleanupFailure(parentT, "delete copied table "+tableID, result, cleanupErr)
			}
		})
	}
	countRecords := func(tableID string) int64 {
		t.Helper()
		result, runErr := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"base", "+record-list", "--base-token", baseToken, "--table-id", tableID, "--limit", "10"},
			DefaultAs: "bot",
		})
		require.NoError(t, runErr)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		return gjson.Get(result.Stdout, "data.record_id_list.#").Int()
	}

	t.Run("schema default", func(t *testing.T) {
		result, runErr := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"base", "+table-copy",
				"--base-token", baseToken,
				"--table-id", sourceTableID,
				"--name", "Schema copy " + clie2e.GenerateSuffix(),
			},
			DefaultAs: "bot",
		})
		require.NoError(t, runErr)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		require.Equal(t, "schema", gjson.Get(result.Stdout, "data.range").String(), result.Stdout)
		require.Equal(t, "success", gjson.Get(result.Stdout, "data.state").String(), result.Stdout)
		targetID := gjson.Get(result.Stdout, "data.table.id").String()
		cleanupCopiedTable(targetID)
		require.Equal(t, int64(0), countRecords(targetID))
	})

	t.Run("all no-wait and status", func(t *testing.T) {
		result, runErr := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"base", "+table-copy",
				"--base-token", baseToken,
				"--table-id", sourceTableID,
				"--name", "Async copy " + clie2e.GenerateSuffix(),
				"--range", "all",
			},
			DefaultAs: "bot",
		})
		require.NoError(t, runErr)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		targetID := gjson.Get(result.Stdout, "data.table.id").String()
		taskID := gjson.Get(result.Stdout, "data.task_id").String()
		cleanupCopiedTable(targetID)
		require.NotEmpty(t, taskID, result.Stdout)

		var lastStatus string
		waitErr := clie2e.WaitForCondition(ctx, clie2e.WaitOptions{
			Timeout:  2 * time.Minute,
			Interval: 3 * time.Second,
			TimeoutError: func() error {
				return fmt.Errorf("table copy still %s", lastStatus)
			},
		}, func() (bool, error) {
			status, statusErr := clie2e.RunCmd(ctx, clie2e.Request{
				Args:      []string{"base", "+table-copy-status", "--base-token", baseToken, "--task-id", taskID},
				DefaultAs: "bot",
			})
			if statusErr != nil {
				return false, statusErr
			}
			if status.ExitCode != 0 {
				return false, fmt.Errorf("status failed: stdout=%s stderr=%s", status.Stdout, status.Stderr)
			}
			lastStatus = gjson.Get(status.Stdout, "data.state").String()
			if lastStatus == "failed" {
				return false, fmt.Errorf("copy task failed: %s", status.Stdout)
			}
			return lastStatus == "success", nil
		})
		require.NoError(t, waitErr)
		require.Equal(t, int64(1), countRecords(targetID))
	})

	t.Run("all wait", func(t *testing.T) {
		result, runErr := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"base", "+table-copy",
				"--base-token", baseToken,
				"--table-id", sourceTableID,
				"--name", "Wait copy " + clie2e.GenerateSuffix(),
				"--range", "all",
				"--wait",
				"--timeout", "2m",
			},
			DefaultAs: "bot",
		})
		require.NoError(t, runErr)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		require.Equal(t, "success", gjson.Get(result.Stdout, "data.state").String(), result.Stdout)
		targetID := gjson.Get(result.Stdout, "data.table.id").String()
		cleanupCopiedTable(targetID)
		require.Equal(t, int64(1), countRecords(targetID))
	})
}
