// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"strings"

	"github.com/larksuite/cli/shortcuts/common"
)

func validateWorkflowIDFlag(runtime *common.RuntimeContext) error {
	workflowID := strings.TrimSpace(runtime.Str("workflow-id"))
	if workflowID == "" {
		return baseFlagErrorf("--workflow-id must not be blank")
	}
	if !strings.HasPrefix(workflowID, "wkf") {
		return baseFlagErrorf("--workflow-id must be an OpenAPI workflow ID with wkf prefix; internal numeric workflow IDs are not accepted")
	}
	return nil
}

func dryRunFieldButtonBind(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	return common.NewDryRunAPI().
		POST("/open-apis/base/v3/bases/:base_token/tables/:table_id/fields/:field_id/button-workflow:bind").
		Body(map[string]interface{}{"workflow_id": runtime.Str("workflow-id")}).
		Set("base_token", runtime.Str("base-token")).
		Set("table_id", baseTableID(runtime)).
		Set("field_id", runtime.Str("field-id"))
}

func dryRunFieldButtonBindingGet(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	return common.NewDryRunAPI().
		GET("/open-apis/base/v3/bases/:base_token/tables/:table_id/fields/:field_id/button-workflow").
		Set("base_token", runtime.Str("base-token")).
		Set("table_id", baseTableID(runtime)).
		Set("field_id", runtime.Str("field-id"))
}

func dryRunFieldButtonUnbind(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	return common.NewDryRunAPI().
		POST("/open-apis/base/v3/bases/:base_token/tables/:table_id/fields/:field_id/button-workflow:unbind").
		Set("base_token", runtime.Str("base-token")).
		Set("table_id", baseTableID(runtime)).
		Set("field_id", runtime.Str("field-id"))
}

func dryRunWorkflowButtonFields(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	return common.NewDryRunAPI().
		GET("/open-apis/base/v3/bases/:base_token/workflows/:workflow_id/button-fields").
		Set("base_token", runtime.Str("base-token")).
		Set("workflow_id", runtime.Str("workflow-id"))
}

func executeFieldButtonBind(runtime *common.RuntimeContext) error {
	data, err := baseV3Call(runtime, "POST",
		baseV3Path("bases", runtime.Str("base-token"), "tables", baseTableID(runtime), "fields", runtime.Str("field-id"), "button-workflow:bind"),
		nil,
		map[string]interface{}{"workflow_id": runtime.Str("workflow-id")},
	)
	if err != nil {
		return err
	}
	runtime.Out(data, nil)
	return nil
}

func executeFieldButtonBindingGet(runtime *common.RuntimeContext) error {
	data, err := baseV3Call(runtime, "GET",
		baseV3Path("bases", runtime.Str("base-token"), "tables", baseTableID(runtime), "fields", runtime.Str("field-id"), "button-workflow"),
		nil,
		nil,
	)
	if err != nil {
		return err
	}
	runtime.Out(data, nil)
	return nil
}

func executeFieldButtonUnbind(runtime *common.RuntimeContext) error {
	data, err := baseV3Call(runtime, "POST",
		baseV3Path("bases", runtime.Str("base-token"), "tables", baseTableID(runtime), "fields", runtime.Str("field-id"), "button-workflow:unbind"),
		nil,
		nil,
	)
	if err != nil {
		return err
	}
	runtime.Out(data, nil)
	return nil
}

func executeWorkflowButtonFields(runtime *common.RuntimeContext) error {
	data, err := baseV3Call(runtime, "GET",
		baseV3Path("bases", runtime.Str("base-token"), "workflows", runtime.Str("workflow-id"), "button-fields"),
		nil,
		nil,
	)
	if err != nil {
		return err
	}
	runtime.Out(data, nil)
	return nil
}
