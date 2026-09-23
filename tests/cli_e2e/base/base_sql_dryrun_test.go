// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
)

func TestBaseSQLQueryDryRun(t *testing.T) {
	setBaseDryRunConfigEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	sql := " SELECT rec_id FROM \"任务\" WHERE \"金额\" > 20.0\n"
	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      []string{"base", "+sql-query", "--base-token", "app_x", "--sql", sql, "--dry-run"},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	require.Equal(t, "POST", clie2e.DryRunGet(result.Stdout, "api.0.method").String())
	require.Equal(t, "/open-apis/base/v3/bases/app_x/basesql/query", clie2e.DryRunGet(result.Stdout, "api.0.url").String())
	require.Equal(t, sql, clie2e.DryRunGet(result.Stdout, "api.0.body.sql").String())
	require.Len(t, clie2e.DryRunGet(result.Stdout, "api.0.body").Map(), 1)
}

func TestBaseSQLQueryRejectsRemovedFlag(t *testing.T) {
	setBaseDryRunConfigEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      []string{"base", "+sql-query", "--base-token", "app_x", "--sql", "SELECT rec_id FROM t", "--params", "[]", "--dry-run"},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	require.NotZero(t, result.ExitCode)
	var envelope struct {
		Error struct {
			Type   string `json:"type"`
			Params []struct {
				Name string `json:"name"`
			} `json:"params"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal([]byte(result.Stderr), &envelope))
	require.Equal(t, "validation", envelope.Error.Type)
	require.Len(t, envelope.Error.Params, 1)
	require.Equal(t, "--params", envelope.Error.Params[0].Name)
}
