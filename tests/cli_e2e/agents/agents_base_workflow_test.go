// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package agents

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/larksuite/cli/tests/cli_e2e/drive"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestBaseAssistantLiveWorkflow(t *testing.T) {
	if os.Getenv("LARK_BASE_AGENT_E2E") != "1" {
		t.Skip("set LARK_BASE_AGENT_E2E=1 after the Base Assistant A2A backend is deployed to run the live workflow")
	}
	clie2e.SkipWithoutUserToken(t)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)

	baseToken := createAgentTestBase(t, ctx)
	tableID := firstAgentTestTable(t, ctx, baseToken)

	card, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      []string{"agents", "card", "base:assistant", "--as", "user"},
		DefaultAs: "user",
	})
	require.NoError(t, err)
	card.AssertExitCode(t, 0)
	card.AssertStdoutStatus(t, true)
	require.Equal(t, "base_assistant", gjson.Get(card.Stdout, "data.skills.0.id").String(), card.Stdout)
	require.True(t, gjson.Get(card.Stdout, "data.capabilities.input_required").Bool(), card.Stdout)

	sendContract, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      []string{"agents", "card", "base:assistant", "--operation", "send", "--as", "user"},
		DefaultAs: "user",
	})
	require.NoError(t, err)
	sendContract.AssertExitCode(t, 0)
	sendContract.AssertStdoutStatus(t, true)
	require.Equal(t, "base_token", gjson.Get(sendContract.Stdout, "data.parameters.0.name").String(), sendContract.Stdout)

	send, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"agents", "send", "base:assistant",
			"--text", "概括这个多维表格的当前结构，并说明包含多少张数据表。",
			"--param", "base_token=" + baseToken,
			"--param", "active_table_id=" + tableID,
			"--as", "user",
		},
		DefaultAs: "user",
	})
	require.NoError(t, err)
	send.AssertExitCode(t, 0)
	send.AssertStdoutStatus(t, true)

	taskID := gjson.Get(send.Stdout, "data.task_id").String()
	contextID := gjson.Get(send.Stdout, "data.context_id").String()
	require.NotEmpty(t, taskID, send.Stdout)
	require.NotEmpty(t, contextID, send.Stdout)
	registerAgentContextCleanup(t, baseToken, contextID, taskID)

	task, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"agents", "task", "get", "base:assistant", taskID,
			"--watch", "--timeout", "90s",
			"--param", "base_token=" + baseToken,
			"--param", "context_id=" + contextID,
			"--as", "user",
		},
		DefaultAs: "user",
	})
	require.NoError(t, err)
	task.AssertExitCode(t, 0)
	task.AssertStdoutStatus(t, true)
	require.Equal(t, taskID, gjson.Get(task.Stdout, "data.task_id").String(), task.Stdout)
	require.Contains(t, []string{"submitted", "working", "completed", "input_required", "auth_required"},
		gjson.Get(task.Stdout, "data.state").String(), task.Stdout)

	contextResult, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"agents", "context", "get", "base:assistant", contextID,
			"--param", "base_token=" + baseToken,
			"--as", "user",
		},
		DefaultAs: "user",
	})
	require.NoError(t, err)
	contextResult.AssertExitCode(t, 0)
	contextResult.AssertStdoutStatus(t, true)
	require.Equal(t, contextID, gjson.Get(contextResult.Stdout, "data.context_id").String(), contextResult.Stdout)
	require.GreaterOrEqual(t, gjson.Get(contextResult.Stdout, "data.task_count").Int(), int64(1), contextResult.Stdout)
}

func createAgentTestBase(t *testing.T, ctx context.Context) string {
	t.Helper()

	name := fmt.Sprintf("lark-cli-e2e-base-assistant-%d", time.Now().UnixNano())
	result, err := clie2e.RunCmdWithRetry(ctx, clie2e.Request{
		Args:      []string{"base", "+base-create", "--name", name, "--time-zone", "Asia/Shanghai"},
		DefaultAs: "user",
	}, clie2e.RetryOptions{})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	result.AssertStdoutStatus(t, true)

	baseToken := gjson.Get(result.Stdout, "data.base.app_token").String()
	if baseToken == "" {
		baseToken = gjson.Get(result.Stdout, "data.base.base_token").String()
	}
	require.NotEmpty(t, baseToken, result.Stdout)

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := clie2e.CleanupContext()
		defer cleanupCancel()
		deleteResult, deleteErr := drive.DeleteDriveResourceAndVerify(cleanupCtx, baseToken, "bitable", "user")
		clie2e.ReportCleanupFailure(t, "delete Base "+baseToken, deleteResult, deleteErr)
	})
	return baseToken
}

func firstAgentTestTable(t *testing.T, ctx context.Context, baseToken string) string {
	t.Helper()

	result, err := clie2e.RunCmdWithRetry(ctx, clie2e.Request{
		Args:      []string{"base", "+table-list", "--base-token", baseToken, "--limit", "20"},
		DefaultAs: "user",
	}, clie2e.RetryOptions{})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	result.AssertStdoutStatus(t, true)

	tableID := gjson.Get(result.Stdout, "data.tables.0.id").String()
	if tableID == "" {
		tableID = gjson.Get(result.Stdout, "data.tables.0.table_id").String()
	}
	require.NotEmpty(t, tableID, result.Stdout)
	return tableID
}

func registerAgentContextCleanup(t *testing.T, baseToken, contextID, taskID string) {
	t.Helper()

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := clie2e.CleanupContext()
		defer cleanupCancel()

		cancelResult, cancelErr := clie2e.RunCmd(cleanupCtx, clie2e.Request{
			Args: []string{
				"agents", "task", "cancel", "base:assistant", taskID,
				"--param", "base_token=" + baseToken,
			},
			DefaultAs: "user",
		})
		if cancelErr != nil || (cancelResult.ExitCode != 0 && !isExpectedAgentCleanupFailure(cancelResult)) {
			clie2e.ReportCleanupFailure(t, "cancel Base Assistant task "+taskID, cancelResult, cancelErr)
		}

		deleteResult, deleteErr := clie2e.RunCmd(cleanupCtx, clie2e.Request{
			Args: []string{
				"agents", "context", "delete", "base:assistant", contextID,
				"--param", "base_token=" + baseToken,
			},
			DefaultAs: "user",
			Yes:       true,
		})
		clie2e.ReportCleanupFailure(t, "delete Base Assistant context "+contextID, deleteResult, deleteErr)
	})
}

func isExpectedAgentCleanupFailure(result *clie2e.Result) bool {
	if result == nil {
		return false
	}
	raw := strings.ToLower(result.Stdout + "\n" + result.Stderr)
	return strings.Contains(raw, "failed_precondition") ||
		strings.Contains(raw, "not_found") ||
		strings.Contains(raw, "already completed") ||
		strings.Contains(raw, "already canceled")
}
