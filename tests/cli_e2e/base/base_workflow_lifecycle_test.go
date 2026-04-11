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

func TestBase_WorkflowLifecycle(t *testing.T) {
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	t.Cleanup(cancel)

	baseToken := createBase(t, ctx, "lark-cli-e2e-base-workflow-"+testSuffix())
	tableName := "lark-cli-e2e-workflow-table-" + testSuffix()
	_, _, _ = createTable(t, parentT, ctx, baseToken, tableName, `[{"name":"Name","type":"text"}]`, "")
	workflowID := createWorkflow(t, ctx, baseToken, `{"client_token":"`+testSuffix()+`","title":"My Workflow","steps":[{"id":"trigger_1","type":"AddRecordTrigger","title":"Watch New Records","next":"action_1","data":{"table_name":"`+tableName+`","watched_field_name":"Name"}},{"id":"action_1","type":"Delay","title":"Wait Briefly","next":null,"data":{"duration":1}}]}`)

	t.Run("list", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"base", "+workflow-list", "--base-token", baseToken},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		if result.ExitCode != 0 {
			skipIfBaseUnavailable(t, result, "requires bot workflow list capability")
		}
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		assert.True(t, gjson.Get(result.Stdout, "data.items.#(workflow_id==\""+workflowID+"\")").Exists(), "stdout:\n%s", result.Stdout)
	})

	t.Run("get", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"base", "+workflow-get", "--base-token", baseToken, "--workflow-id", workflowID},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		if result.ExitCode != 0 {
			skipIfBaseUnavailable(t, result, "requires bot workflow get capability")
		}
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		assert.Equal(t, workflowID, gjson.Get(result.Stdout, "data.workflow_id").String())
	})

	t.Run("update", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"base", "+workflow-update", "--base-token", baseToken, "--workflow-id", workflowID, "--json", `{"title":"My Workflow Updated","steps":[{"id":"trigger_1","type":"AddRecordTrigger","title":"Watch New Records","next":"action_1","data":{"table_name":"` + tableName + `","watched_field_name":"Name"}},{"id":"action_1","type":"Delay","title":"Wait Briefly","next":null,"data":{"duration":2}}]}`},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		if result.ExitCode != 0 {
			skipIfBaseUnavailable(t, result, "requires bot workflow update capability")
		}
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		assert.Equal(t, "My Workflow Updated", gjson.Get(result.Stdout, "data.title").String())
	})

	t.Run("enable", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"base", "+workflow-enable", "--base-token", baseToken, "--workflow-id", workflowID},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		if result.ExitCode != 0 {
			skipIfBaseUnavailable(t, result, "requires bot workflow enable capability")
		}
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
	})

	t.Run("disable", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"base", "+workflow-disable", "--base-token", baseToken, "--workflow-id", workflowID},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		if result.ExitCode != 0 {
			skipIfBaseUnavailable(t, result, "requires bot workflow disable capability")
		}
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
	})
}
