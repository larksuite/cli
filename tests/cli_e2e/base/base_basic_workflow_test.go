// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"fmt"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestBase_BasicWorkflow(t *testing.T) {
	clie2e.SkipWithoutTenantAccessToken(t)
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
	tableID, _, primaryViewID := createTableWithRetry(
		t,
		parentT,
		ctx,
		baseToken,
		tableName,
		`[{"name":"Name","type":"text"}]`,
		`{"name":"Main","type":"grid"}`,
	)

	t.Run("resolve table URL as bot", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"base", "+url-resolve",
				"--url", fmt.Sprintf("https://example.larkoffice.com/base/%s?table=%s&view=%s", baseToken, tableID, primaryViewID),
			},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		assert.Equal(t, baseToken, gjson.Get(result.Stdout, "data.base_token").String(), "stdout:\n%s", result.Stdout)
		assert.Equal(t, tableID, gjson.Get(result.Stdout, "data.block_id").String(), "stdout:\n%s", result.Stdout)
		assert.Equal(t, "table", gjson.Get(result.Stdout, "data.block_type").String(), "stdout:\n%s", result.Stdout)
		assert.Equal(t, tableID, gjson.Get(result.Stdout, "data.table_id").String(), "stdout:\n%s", result.Stdout)
		assert.Equal(t, primaryViewID, gjson.Get(result.Stdout, "data.view_id").String(), "stdout:\n%s", result.Stdout)
	})

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
		table := findBaseTableByID(t, ctx, baseToken, tableID)
		assert.Equal(t, tableID, table.Get("id").String())
		assert.Equal(t, tableName, table.Get("name").String())
	})
}

func TestBaseWorkflowUpdateDryRun(t *testing.T) {
	result := runBaseDryRun(t, 0,
		"base", "+workflow-update",
		"--base-token", "app_x",
		"--workflow-id", "wkf_x",
		"--json", `{"title":"Reminder","steps":[{"type":"LarkMessageAction","data":{"receiver":[{"value_type":"user","value":{"id":"ou_x"}}],"content":[{"value_type":"text","value":"Review the request"}]}}]}`,
	)

	out := result.Stdout
	require.Equal(t, "/open-apis/base/v3/bases/app_x/workflows/wkf_x", clie2e.DryRunGet(out, "api.0.url").String(), out)
	require.Equal(t, "PUT", clie2e.DryRunGet(out, "api.0.method").String(), out)
	require.Equal(t, "Reminder", clie2e.DryRunGet(out, "api.0.body.title").String(), out)
	require.Equal(t, "LarkMessageAction", clie2e.DryRunGet(out, "api.0.body.steps.0.type").String(), out)
	require.Equal(t, "ou_x", clie2e.DryRunGet(out, "api.0.body.steps.0.data.receiver.0.value.id").String(), out)
	require.Equal(t, "Review the request", clie2e.DryRunGet(out, "api.0.body.steps.0.data.content.0.value").String(), out)
	require.False(t, clie2e.DryRunGet(out, "api.0.body.steps.0.data.btn_list").Exists(), out)
}

func TestBaseWorkflowCreateDryRun(t *testing.T) {
	result := runBaseDryRun(t, 0,
		"base", "+workflow-create",
		"--base-token", "app_x",
		"--json", `{"title":"Reminder","client_token":"create_1","steps":[{"type":"LarkMessageAction","data":{"receiver":[{"value_type":"user","value":{"id":"ou_x"}}],"content":[{"value_type":"text","value":"Review the request"}]}}]}`,
	)

	out := result.Stdout
	require.Equal(t, "/open-apis/base/v3/bases/app_x/workflows", clie2e.DryRunGet(out, "api.0.url").String(), out)
	require.Equal(t, "POST", clie2e.DryRunGet(out, "api.0.method").String(), out)
	require.Equal(t, "create_1", clie2e.DryRunGet(out, "api.0.body.client_token").String(), out)
	require.Equal(t, "ou_x", clie2e.DryRunGet(out, "api.0.body.steps.0.data.receiver.0.value.id").String(), out)
	require.Equal(t, "Review the request", clie2e.DryRunGet(out, "api.0.body.steps.0.data.content.0.value").String(), out)
	require.False(t, clie2e.DryRunGet(out, "api.0.body.steps.0.data.btn_list").Exists(), out)
}

func TestBaseWorkflowDryRunRejectsInvalidDefinitions(t *testing.T) {
	for _, tt := range []struct {
		name        string
		args        []string
		wantSubtype string
		wantPath    string
	}{
		{
			name: "missing message content",
			args: []string{
				"base", "+workflow-update",
				"--base-token", "app_x",
				"--workflow-id", "wkf_x",
				"--json", `{"title":"Reminder","steps":[{"type":"LarkMessageAction","data":{"receiver":[{"value_type":"user","value":{"id":"ou_x"}}]}}]}`,
			},
			wantSubtype: "invalid_argument",
			wantPath:    "content",
		},
		{
			name: "invalid optional message field",
			args: []string{
				"base", "+workflow-create",
				"--base-token", "app_x",
				"--json", `{"title":"Reminder","client_token":"create_1","steps":[{"type":"LarkMessageAction","data":{"receiver":[{"value_type":"user","value":{"id":"ou_x"}}],"content":[{"value_type":"text","value":"Review the request"}],"send_to_everyone":"yes"}}]}`,
			},
			wantSubtype: "invalid_argument",
			wantPath:    "send_to_everyone",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			result := runBaseDryRun(t, 2, tt.args...)
			require.Equal(t, "validation", gjson.Get(result.Stderr, "error.type").String(), result.Stderr)
			require.Equal(t, tt.wantSubtype, gjson.Get(result.Stderr, "error.subtype").String(), result.Stderr)
			require.Equal(t, "--json", gjson.Get(result.Stderr, "error.param").String(), result.Stderr)
			require.Contains(t, gjson.Get(result.Stderr, "error.message").String(), tt.wantPath, result.Stderr)
			require.Empty(t, result.Stdout)
		})
	}
}

func readWorkflowReference(t *testing.T, name string) string {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{"skills", "read", "lark-base", "references/" + name},
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	return result.Stdout
}

func TestWorkflowSkillDeliveryRegression(t *testing.T) {
	guide := readWorkflowReference(t, "lark-base-workflow-guide.md")
	for _, contract := range []string{
		"创建/更新时重点构造 `title` 和 `steps`",
		"`status` 通过 `+workflow-enable` 或 `+workflow-disable` 单独管理",
	} {
		require.Contains(t, guide, contract)
	}
	require.NotContains(t, guide, "创建/更新时重点构造 `title`、`status` 和 `steps`")
}

func TestWorkflowSchemaDeliveryRegression(t *testing.T) {
	schema := readWorkflowReference(t, "lark-base-workflow-schema.md")
	for _, contract := range []string{
		"| `receiver` | 是 | 非空 ValueInfo[] |",
		"| `content` | 是 | 非空 TextRefItem[] 消息内容 |",
		"| `send_to_everyone` | 否 | boolean；",
		"| `btn_list` | 否 | ButtonConfig[]；",
		"`DeleteRecordTrigger` 当前不可用，不要构造或提交该 type",
		"`--dry-run` 只预览最终请求，不能证明服务端支持",
		"先说明能力边界并停止写入",
		"任何会改变触发条件、数据结构或业务结果的替代方案都只能作为建议",
		"只有用户明确选择该替代方案后才能执行相关写入",
		"先前请求失败、用户保持沉默，或要求减少追问，都不构成改变原始语义的授权",
	} {
		require.Contains(t, schema, contract)
	}
}
