// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"fmt"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestBaseButtonFieldWorkflow(t *testing.T) {
	clie2e.SkipWithoutTenantAccessToken(t)
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	t.Cleanup(cancel)

	baseToken := createBaseWithRetry(t, ctx, "lark-cli-e2e-button-field-"+clie2e.GenerateSuffix())
	tableName := "Leads " + clie2e.GenerateSuffix()
	tableID, _, _ := createTableWithRetry(
		t,
		parentT,
		ctx,
		baseToken,
		tableName,
		`[{"name":"Lead Name","type":"text"}]`,
		`{"name":"Main","type":"grid"}`,
	)

	workflowBody := func(title string) string {
		return fmt.Sprintf(`{"client_token":"%s","title":"%s","steps":[{"id":"step_button_trigger","type":"ButtonTrigger","title":"Click sync button","next":"step_delay","data":{"button_type":"buttonField","table_name":"%s"}},{"id":"step_delay","type":"Delay","title":"Wait briefly","next":null,"data":{"duration":1}}]}`, clie2e.GenerateSuffix(), title, tableName)
	}

	createWorkflow := func(title string) string {
		t.Helper()
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"base", "+workflow-create",
				"--base-token", baseToken,
				"--json", workflowBody(title),
			},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		workflowID := gjson.Get(result.Stdout, "data.workflow_id").String()
		require.NotEmpty(t, workflowID, "stdout:\n%s", result.Stdout)
		return workflowID
	}

	getWorkflow := func(workflowID string) gjson.Result {
		t.Helper()
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"base", "+workflow-get",
				"--base-token", baseToken,
				"--workflow-id", workflowID,
			},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		return gjson.Parse(result.Stdout)
	}

	getField := func(fieldID string) gjson.Result {
		t.Helper()
		var payload gjson.Result
		err := clie2e.WaitForCondition(ctx, clie2e.WaitOptions{
			Timeout:  20 * time.Second,
			Interval: time.Second,
		}, func() (bool, error) {
			result, runErr := clie2e.RunCmd(ctx, clie2e.Request{
				Args: []string{
					"base", "+field-get",
					"--base-token", baseToken,
					"--table-id", tableID,
					"--field-id", fieldID,
				},
				DefaultAs: "bot",
			})
			if runErr != nil {
				return false, runErr
			}
			if result.ExitCode != 0 {
				return false, nil
			}
			payload = gjson.Parse(result.Stdout)
			return payload.Get("status").Bool(), nil
		})
		require.NoError(t, err)
		return payload
	}

	firstWorkflowID := createWorkflow("Sync to CRM A")
	firstWorkflow := getWorkflow(firstWorkflowID)
	require.Equal(t, firstWorkflowID, firstWorkflow.Get("data.workflow_id").String(), firstWorkflow.Raw)
	require.Equal(t, tableName, firstWorkflow.Get("data.steps.0.data.table_name").String(), firstWorkflow.Raw)
	require.Equal(t, "buttonField", firstWorkflow.Get("data.steps.0.data.button_type").String(), firstWorkflow.Raw)

	createFieldResult, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"base", "+field-create",
			"--base-token", baseToken,
			"--table-id", tableID,
			"--json", fmt.Sprintf(`{"name":"Sync to CRM","type":"button","button":{"title":"Sync to CRM","color":0},"trigger":{"type":"automation","workflow_id":"%s"}}`, firstWorkflowID),
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	createFieldResult.AssertExitCode(t, 0)
	createFieldResult.AssertStdoutStatus(t, true)
	require.True(t, gjson.Get(createFieldResult.Stdout, "data.field_get_recommended").Bool(), createFieldResult.Stdout)
	require.Equal(t, "field_get", gjson.Get(createFieldResult.Stdout, "data.next_step").String(), createFieldResult.Stdout)

	fieldID := gjson.Get(createFieldResult.Stdout, "data.field.id").String()
	if fieldID == "" {
		fieldID = gjson.Get(createFieldResult.Stdout, "data.field.field_id").String()
	}
	require.NotEmpty(t, fieldID, "stdout:\n%s", createFieldResult.Stdout)

	fieldPayload := getField(fieldID)
	require.Equal(t, fieldID, fieldPayload.Get("data.field.id").String(), fieldPayload.Raw)
	require.Equal(t, "Button", fieldPayload.Get("data.field.fieldUIType").String(), fieldPayload.Raw)
	require.Equal(t, "Sync to CRM", fieldPayload.Get("data.field.property.button.title").String(), fieldPayload.Raw)
	require.Equal(t, int64(0), fieldPayload.Get("data.field.property.button.color").Int(), fieldPayload.Raw)
	require.Equal(t, int64(1), fieldPayload.Get("data.field.property.trigger.type").Int(), fieldPayload.Raw)
	require.Equal(t, firstWorkflowID, fieldPayload.Get("data.field.property.trigger.config.id").String(), fieldPayload.Raw)

	secondWorkflowID := createWorkflow("Sync to CRM B")
	secondWorkflow := getWorkflow(secondWorkflowID)
	require.Equal(t, secondWorkflowID, secondWorkflow.Get("data.workflow_id").String(), secondWorkflow.Raw)
	require.Equal(t, tableName, secondWorkflow.Get("data.steps.0.data.table_name").String(), secondWorkflow.Raw)

	updateFieldResult, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"base", "+field-update",
			"--base-token", baseToken,
			"--table-id", tableID,
			"--field-id", fieldID,
			"--json", fmt.Sprintf(`{"name":"Sync to CRM","type":"button","button":{"title":"Sync to CRM","color":0},"trigger":{"type":"automation","workflow_id":"%s"}}`, secondWorkflowID),
			"--yes",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	updateFieldResult.AssertExitCode(t, 0)
	updateFieldResult.AssertStdoutStatus(t, true)
	require.True(t, gjson.Get(updateFieldResult.Stdout, "data.field_get_recommended").Bool(), updateFieldResult.Stdout)
	require.Equal(t, "field_get", gjson.Get(updateFieldResult.Stdout, "data.next_step").String(), updateFieldResult.Stdout)

	updatedFieldPayload := getField(fieldID)
	require.Equal(t, "Button", updatedFieldPayload.Get("data.field.fieldUIType").String(), updatedFieldPayload.Raw)
	require.Equal(t, secondWorkflowID, updatedFieldPayload.Get("data.field.property.trigger.config.id").String(), updatedFieldPayload.Raw)
}
