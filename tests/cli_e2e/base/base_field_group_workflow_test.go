// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/larksuite/cli/tests/cli_e2e/drive"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// TestBaseFieldGroupCreateWorkflow creates a throwaway Base with a table,
// groups one field, and asserts the returned group ID. Field groups have no
// delete API; cleanup removes the whole Base via drive. Runs as user because
// the tenant token of a first-party app typically lacks base:app:create.
func TestBaseFieldGroupCreateWorkflow(t *testing.T) {
	clie2e.SkipWithoutUserToken(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	t.Cleanup(cancel)

	suffix := clie2e.GenerateSuffix()
	baseName := "lark-cli-e2e-field-group-" + suffix
	var baseToken string
	t.Run("create base", func(t *testing.T) {
		result, err := clie2e.RunCmdWithRetry(ctx, clie2e.Request{
			Args:      []string{"base", "+base-create", "--name", baseName, "--time-zone", "Asia/Shanghai"},
			DefaultAs: "user",
		}, clie2e.RetryOptions{})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		baseToken = gjson.Get(result.Stdout, "data.base.app_token").String()
		if baseToken == "" {
			baseToken = gjson.Get(result.Stdout, "data.base.base_token").String()
		}
		require.NotEmpty(t, baseToken, "stdout:\n%s", result.Stdout)
	})
	if baseToken == "" {
		t.FailNow()
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := clie2e.CleanupContext()
		defer cleanupCancel()
		deleteResult, deleteErr := drive.DeleteDriveResourceAndVerify(cleanupCtx, baseToken, "bitable", "user")
		clie2e.ReportCleanupFailure(t, "delete base "+baseToken, deleteResult, deleteErr)
	})

	var tableID string
	t.Run("create table", func(t *testing.T) {
		result, err := clie2e.RunCmdWithRetry(ctx, clie2e.Request{
			Args: []string{
				"base", "+table-create",
				"--base-token", baseToken,
				"--name", "Field Group " + suffix,
				"--fields", `[{"name":"Name","type":"text"},{"name":"Status","type":"text"}]`,
			},
			DefaultAs: "user",
		}, clie2e.RetryOptions{})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		tableID = gjson.Get(result.Stdout, "data.table.id").String()
		if tableID == "" {
			tableID = gjson.Get(result.Stdout, "data.table.table_id").String()
		}
		require.NotEmpty(t, tableID, "stdout:\n%s", result.Stdout)
	})
	if tableID == "" {
		t.FailNow()
	}

	var primaryFieldID string
	t.Run("list fields", func(t *testing.T) {
		result, err := clie2e.RunCmdWithRetry(ctx, clie2e.Request{
			Args: []string{
				"base", "+field-list",
				"--base-token", baseToken,
				"--table-id", tableID,
			},
			DefaultAs: "user",
		}, clie2e.RetryOptions{})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		primaryFieldID = gjson.Get(result.Stdout, "data.fields.0.id").String()
		if primaryFieldID == "" {
			primaryFieldID = gjson.Get(result.Stdout, "data.fields.0.field_id").String()
		}
		require.True(t, strings.HasPrefix(primaryFieldID, "fld"), "stdout:\n%s", result.Stdout)
	})
	if primaryFieldID == "" {
		t.FailNow()
	}

	groupJSON := fmt.Sprintf(
		`{"field_groups":[{"name":"Details %s","children":[{"type":"field","id":"%s"}]}]}`,
		suffix,
		primaryFieldID,
	)

	t.Run("create field group", func(t *testing.T) {
		result, err := clie2e.RunCmdWithRetry(ctx, clie2e.Request{
			Args: []string{
				"base", "+field-group-create",
				"--base-token", baseToken,
				"--table-id", tableID,
				"--json", groupJSON,
			},
			DefaultAs: "user",
		}, clie2e.RetryOptions{})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		out := result.Stdout
		groupID := gjson.Get(out, "data.field_groups.0.id").String()
		require.True(t, strings.HasPrefix(groupID, "fg"), "stdout:\n%s", out)
		require.Equal(t, "Details "+suffix, gjson.Get(out, "data.field_groups.0.name").String(), "stdout:\n%s", out)
	})

	t.Run("duplicate membership is rejected", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"base", "+field-group-create",
				"--base-token", baseToken,
				"--table-id", tableID,
				"--json", groupJSON,
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		require.NotEqual(t, 0, result.ExitCode, "stdout:\n%s", result.Stdout)
		require.Contains(t, result.Stderr, "1254122", "stderr:\n%s", result.Stderr)
	})
}
