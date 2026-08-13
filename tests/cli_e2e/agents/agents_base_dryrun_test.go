// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package agents

import (
	"context"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestBaseAgentSendDryRun(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("LARKSUITE_CLI_APP_ID", "agents_dryrun_test")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "agents_dryrun_secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"agents", "send", "base:assistant",
			"--text", "汇总当前表格",
			"--param", "base_token=basc_dryrun",
			"--param", "active_table_id=tbl_dryrun",
			"--as", "user",
			"--dry-run",
		},
		DefaultAs: "user",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	out := result.Stdout
	require.True(t, gjson.Get(out, "data.dry_run").Bool(), out)
	require.Equal(t, "base:assistant", gjson.Get(out, "data.would_send.agent_ref").String(), out)
	require.Equal(t, "汇总当前表格", gjson.Get(out, "data.would_send.text").String(), out)
	require.Equal(t, "basc_dryrun", gjson.Get(out, "data.would_send.params.base_token").String(), out)
	require.Equal(t, "tbl_dryrun", gjson.Get(out, "data.would_send.params.active_table_id").String(), out)
}

func TestBaseAgentComplexConstructionDryRun(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("LARKSUITE_CLI_APP_ID", "agents_dryrun_test")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "agents_dryrun_secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"agents", "send", "base:assistant",
			"--text", "新建一张订单表，字段为订单号、客户、金额和状态",
			"--param", "base_token=basc_dryrun",
			"--as", "user",
			"--dry-run",
		},
		DefaultAs: "user",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	out := result.Stdout
	require.True(t, gjson.Get(out, "data.dry_run").Bool(), out)
	require.Equal(t, "base:assistant", gjson.Get(out, "data.would_send.agent_ref").String(), out)
	require.Equal(t, "新建一张订单表，字段为订单号、客户、金额和状态", gjson.Get(out, "data.would_send.text").String(), out)
	require.Equal(t, "basc_dryrun", gjson.Get(out, "data.would_send.params.base_token").String(), out)
	require.False(t, gjson.Get(out, "data.would_send.params.active_table_id").Exists(), out)
}

// TestBaseAgentAnswerDryRun covers the input_required reply mode: --answer with
// the pending group's context/task. dry-run runs before the provider handler,
// so it only previews the normalized answers map and context/task ids — the wire
// DataPart and deterministic id are asserted in the adapter call-capture unit
// test (agents/base), not here.
func TestBaseAgentAnswerDryRun(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("LARKSUITE_CLI_APP_ID", "agents_dryrun_test")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "agents_dryrun_secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"agents", "send", "base:assistant",
			"--context-id", "ctx-1",
			"--task-id", "task-1",
			"--answer", "q_scene=opt_1",
			"--answer", "q_metric=opt_a",
			"--answer", "q_metric=opt_b",
			"--answer", "q_note.text=group by calendar month",
			"--param", "base_token=basc_dryrun",
			"--as", "user",
			"--dry-run",
		},
		DefaultAs: "user",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	out := result.Stdout
	require.True(t, gjson.Get(out, "data.dry_run").Bool(), out)
	require.Equal(t, "ctx-1", gjson.Get(out, "data.would_send.context_id").String(), out)
	require.Equal(t, "task-1", gjson.Get(out, "data.would_send.task_id").String(), out)
	require.Equal(t, "opt_1", gjson.Get(out, "data.would_send.answers.q_scene.0").String(), out)
	var metric []string
	for _, v := range gjson.Get(out, "data.would_send.answers.q_metric").Array() {
		metric = append(metric, v.String())
	}
	require.Equal(t, []string{"opt_a", "opt_b"}, metric, out)
	require.Equal(t, "group by calendar month", gjson.Get(out, `data.would_send.answers.q_note\.text.0`).String(), out)
}
