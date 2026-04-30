// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

const cleanupTimeout = 30 * time.Second

func reportCleanupFailure(parentT *testing.T, prefix string, result *clie2e.Result, err error) {
	parentT.Helper()

	if err != nil {
		parentT.Errorf("%s: %v", prefix, err)
		return
	}
	if result == nil {
		parentT.Errorf("%s: nil result", prefix)
		return
	}
	if isCleanupSuppressedResult(result) {
		return
	}

	parentT.Errorf("%s failed: exit=%d stdout=%s stderr=%s", prefix, result.ExitCode, result.Stdout, result.Stderr)
}

func cleanupContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), cleanupTimeout)
}

func isCleanupSuppressedResult(result *clie2e.Result) bool {
	if result == nil {
		return false
	}

	raw := strings.TrimSpace(result.Stdout)
	if raw == "" {
		raw = strings.TrimSpace(result.Stderr)
	}
	if raw == "" {
		return false
	}

	start := strings.LastIndex(raw, "\n{")
	if start >= 0 {
		start++
	} else {
		start = strings.Index(raw, "{")
	}
	if start < 0 {
		return false
	}

	payload := raw[start:]
	if !gjson.Valid(payload) {
		return false
	}

	if gjson.Get(payload, "error.type").String() != "api_error" {
		return false
	}

	if gjson.Get(payload, "error.detail.type").String() == "not_found" ||
		strings.Contains(strings.ToLower(gjson.Get(payload, "error.message").String()), "not found") {
		return true
	}

	return gjson.Get(payload, "error.code").Int() == 800004135 ||
		strings.Contains(strings.ToLower(gjson.Get(payload, "error.message").String()), " limited")
}

func createBaseWithRetry(t *testing.T, ctx context.Context, name string) string {
	t.Helper()

	result, err := clie2e.RunCmdWithRetry(ctx, clie2e.Request{
		Args:      []string{"base", "+base-create", "--name", name, "--time-zone", "Asia/Shanghai"},
		DefaultAs: "bot",
	}, clie2e.RetryOptions{})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	result.AssertStdoutStatus(t, true)

	baseToken := gjson.Get(result.Stdout, "data.base.app_token").String()
	if baseToken == "" {
		baseToken = gjson.Get(result.Stdout, "data.base.base_token").String()
	}
	require.NotEmpty(t, baseToken, "stdout:\n%s", result.Stdout)
	return baseToken
}

func createTableWithRetry(t *testing.T, parentT *testing.T, ctx context.Context, baseToken string, name string, fieldsJSON string, viewJSON string) (tableID string, primaryFieldID string, primaryViewID string) {
	t.Helper()

	args := []string{"base", "+table-create", "--base-token", baseToken, "--name", name}
	if fieldsJSON != "" {
		args = append(args, "--fields", fieldsJSON)
	}
	if viewJSON != "" {
		args = append(args, "--view", viewJSON)
	}

	result, err := clie2e.RunCmdWithRetry(ctx, clie2e.Request{
		Args:      args,
		DefaultAs: "bot",
	}, clie2e.RetryOptions{})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	result.AssertStdoutStatus(t, true)

	tableID = gjson.Get(result.Stdout, "data.table.id").String()
	if tableID == "" {
		tableID = gjson.Get(result.Stdout, "data.table.table_id").String()
	}
	require.NotEmpty(t, tableID, "stdout:\n%s", result.Stdout)

	primaryFieldID = gjson.Get(result.Stdout, "data.fields.0.id").String()
	if primaryFieldID == "" {
		primaryFieldID = gjson.Get(result.Stdout, "data.fields.0.field_id").String()
	}

	primaryViewID = gjson.Get(result.Stdout, "data.views.0.id").String()
	if primaryViewID == "" {
		primaryViewID = gjson.Get(result.Stdout, "data.views.0.view_id").String()
	}

	parentT.Cleanup(func() {
		cleanupCtx, cancel := cleanupContext()
		defer cancel()

		deleteResult, deleteErr := clie2e.RunCmd(cleanupCtx, clie2e.Request{
			Args:      []string{"base", "+table-delete", "--base-token", baseToken, "--table-id", tableID, "--yes"},
			DefaultAs: "bot",
		})
		if deleteErr != nil || deleteResult.ExitCode != 0 {
			reportCleanupFailure(parentT, "delete table "+tableID, deleteResult, deleteErr)
		}
	})

	return tableID, primaryFieldID, primaryViewID
}

func createRole(t *testing.T, ctx context.Context, baseToken string, body string) string {
	t.Helper()

	result, err := clie2e.RunCmdWithRetry(ctx, clie2e.Request{
		Args:      []string{"base", "+role-create", "--base-token", baseToken, "--json", body},
		DefaultAs: "bot",
	}, clie2e.RetryOptions{})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	result.AssertStdoutStatus(t, true)

	return gjson.Get(result.Stdout, "data.role_id").String()
}

func findBaseTableByID(t *testing.T, ctx context.Context, baseToken string, tableID string) gjson.Result {
	t.Helper()

	require.NotEmpty(t, baseToken, "base token is required")
	require.NotEmpty(t, tableID, "table ID is required")

	const pageLimit = 50

	offset := 0
	seenOffsets := map[int]struct{}{}
	for {
		if _, seen := seenOffsets[offset]; seen {
			t.Fatalf("base table list pagination loop detected for base %q, repeated offset %d", baseToken, offset)
		}
		seenOffsets[offset] = struct{}{}

		args := []string{
			"base", "+table-list",
			"--base-token", baseToken,
			"--limit", strconv.Itoa(pageLimit),
			"--offset", strconv.Itoa(offset),
		}

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      args,
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		table := gjson.Get(result.Stdout, `data.tables.#(id=="`+tableID+`")`)
		if table.Exists() {
			return table
		}

		tables := gjson.Get(result.Stdout, "data.tables").Array()
		if len(tables) == 0 || len(tables) < pageLimit {
			t.Fatalf("table %q not found in listed pages, last stdout:\n%s", tableID, result.Stdout)
		}
		offset += len(tables)
	}
}
