// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestBase_BasicWorkflow(t *testing.T) {
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	t.Cleanup(cancel)

	baseName := "lark-cli-e2e-base-basic-" + clie2e.GenerateSuffix()
	baseToken := createBaseWithRetry(t, ctx, baseName)

	t.Run("get base as bot", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"base", "+base-get", "--base-token", baseToken},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		returnedBaseToken := gjson.Get(result.Stdout, "data.base.app_token").String()
		if returnedBaseToken == "" {
			returnedBaseToken = gjson.Get(result.Stdout, "data.base.base_token").String()
		}
		assert.Equal(t, baseToken, returnedBaseToken, "stdout:\n%s", result.Stdout)
		assert.NotEmpty(t, gjson.Get(result.Stdout, "data.base.name").String(), "stdout:\n%s", result.Stdout)
	})

	tableName := "lark-cli-e2e-table-basic-" + clie2e.GenerateSuffix()
	tableID, primaryFieldID, primaryViewID := createTableWithRetry(
		t,
		parentT,
		ctx,
		baseToken,
		tableName,
		`[{"name":"Name","type":"text"}]`,
		`{"name":"Main","type":"grid"}`,
	)

	t.Run("get table as bot", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"base", "+table-get", "--base-token", baseToken, "--table-id", tableID},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		assert.Equal(t, tableID, gjson.Get(result.Stdout, "data.table.id").String())
		assert.Equal(t, tableName, gjson.Get(result.Stdout, "data.table.name").String())
	})

	t.Run("list tables and find created table as bot", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"base", "+table-list", "--base-token", baseToken},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		assert.True(t, gjson.Get(result.Stdout, `data.tables.#(id=="`+tableID+`")`).Exists(), "stdout:\n%s", result.Stdout)
	})

	require.NotEmpty(t, primaryFieldID)
	require.NotEmpty(t, primaryViewID)
}
