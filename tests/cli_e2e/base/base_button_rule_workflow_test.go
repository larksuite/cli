// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestBaseButtonRuleWorkflow(t *testing.T) {
	clie2e.SkipWithoutTenantAccessToken(t)
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	t.Cleanup(cancel)

	suffix := clie2e.GenerateSuffix()
	baseToken := createBaseWithRetry(t, ctx, "lark-cli-e2e-button-rule-"+suffix)
	tableName := "Button Rule " + suffix
	tableID, _, _ := createTableWithRetry(
		t,
		parentT,
		ctx,
		baseToken,
		tableName,
		`[{"name":"Name","type":"text"}]`,
		`{"name":"Main","type":"grid"}`,
	)

	buttonName := "Run workflow"
	buttonJSON, err := json.Marshal(map[string]interface{}{
		"name": buttonName,
		"type": "button",
		"button_config": map[string]interface{}{
			"title": buttonName,
		},
	})
	require.NoError(t, err)

	var fieldID string
	if !t.Run("create button field", func(t *testing.T) {
		result, runErr := clie2e.RunCmdWithRetry(ctx, clie2e.Request{
			Args: []string{
				"base", "+field-create",
				"--base-token", baseToken,
				"--table-id", tableID,
				"--json", string(buttonJSON),
			},
			DefaultAs: "bot",
		}, clie2e.RetryOptions{})
		require.NoError(t, runErr)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		fieldID = gjson.Get(result.Stdout, "data.field.id").String()
		require.True(t, strings.HasPrefix(fieldID, "fld"), "stdout:\n%s", result.Stdout)
		require.Equal(t, buttonName, gjson.Get(result.Stdout, "data.field.name").String(), "stdout:\n%s", result.Stdout)
	}) {
		t.FailNow()
	}

	workflowBody, err := json.Marshal(map[string]interface{}{
		"client_token": "lark-cli-e2e-button-rule-" + suffix,
		"title":        "Button Rule " + suffix,
		"steps": []map[string]interface{}{
			{
				"id":    "button_trigger",
				"type":  "ButtonTrigger",
				"title": "Run from button",
				"next":  "delay_action",
				"data": map[string]interface{}{
					"button_type": "buttonField",
					"table_name":  tableName,
				},
			},
			{
				"id":    "delay_action",
				"type":  "Delay",
				"title": "No-op delay",
				"next":  nil,
				"data": map[string]interface{}{
					"duration": 1,
				},
			},
		},
	})
	require.NoError(t, err)

	var workflowID string
	if !t.Run("create disabled workflow", func(t *testing.T) {
		result, runErr := clie2e.RunCmdWithRetry(ctx, clie2e.Request{
			Args: []string{
				"base", "+workflow-create",
				"--base-token", baseToken,
				"--json", string(workflowBody),
			},
			DefaultAs: "bot",
		}, clie2e.RetryOptions{})
		require.NoError(t, runErr)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		workflowID = gjson.Get(result.Stdout, "data.workflow_id").String()
		require.True(t, strings.HasPrefix(workflowID, "wkf"), "stdout:\n%s", result.Stdout)
	}) {
		t.FailNow()
	}

	if !t.Run("bind by name and read back target by ID", func(t *testing.T) {
		bindResult, runErr := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"base", "+button-rule-bind",
				"--base-token", baseToken,
				"--table-id", tableID,
				"--field-id", buttonName,
				"--workflow-id", workflowID,
			},
			DefaultAs: "bot",
		})
		require.NoError(t, runErr)
		bindResult.AssertExitCode(t, 0)
		bindResult.AssertStdoutStatus(t, true)

		getResult, runErr := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"base", "+button-rule-get",
				"--base-token", baseToken,
				"--table-id", tableID,
				"--field-id", fieldID,
			},
			DefaultAs: "bot",
		})
		require.NoError(t, runErr)
		getResult.AssertExitCode(t, 0)
		getResult.AssertStdoutStatus(t, true)
		require.True(t, gjson.Get(getResult.Stdout, "data.bound").Bool(), "stdout:\n%s", getResult.Stdout)
		require.Equal(t, "workflow", gjson.Get(getResult.Stdout, "data.target.type").String(), "stdout:\n%s", getResult.Stdout)
		require.Equal(t, workflowID, gjson.Get(getResult.Stdout, "data.target.id").String(), "stdout:\n%s", getResult.Stdout)
	}) {
		t.FailNow()
	}

	assertUnbound := func(t *testing.T) {
		t.Helper()
		getResult, runErr := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"base", "+button-rule-get",
				"--base-token", baseToken,
				"--table-id", tableID,
				"--field-id", fieldID,
			},
			DefaultAs: "bot",
		})
		require.NoError(t, runErr)
		getResult.AssertExitCode(t, 0)
		getResult.AssertStdoutStatus(t, true)
		require.False(t, gjson.Get(getResult.Stdout, "data.bound").Bool(), "stdout:\n%s", getResult.Stdout)
		require.Equal(t, "null", gjson.Get(getResult.Stdout, "data.target").Raw, "stdout:\n%s", getResult.Stdout)
	}

	for _, step := range []string{"unbind", "repeat unbind is idempotent"} {
		if !t.Run(step, func(t *testing.T) {
			unbindResult, runErr := clie2e.RunCmd(ctx, clie2e.Request{
				Args: []string{
					"base", "+button-rule-unbind",
					"--base-token", baseToken,
					"--table-id", tableID,
					"--field-id", fieldID,
				},
				DefaultAs: "bot",
			})
			require.NoError(t, runErr)
			unbindResult.AssertExitCode(t, 0)
			unbindResult.AssertStdoutStatus(t, true)
			assertUnbound(t)
		}) {
			t.FailNow()
		}
	}
}
