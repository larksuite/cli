// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"strings"

	"github.com/larksuite/cli/shortcuts/common"
)

var BaseButtonBind = common.Shortcut{
	Service:     "base",
	Command:     "+button-bind",
	Description: "Bind a button field to a workflow",
	Risk:        "write",
	Scopes:      []string{"base:field:update", "base:workflow:update"},
	AuthTypes:   authTypes(),
	Flags: []common.Flag{
		baseTokenFlag(true),
		tableRefFlag(true),
		fieldRefFlag(true),
		{Name: "workflow-id", Desc: "workflow ID (wkf... prefix)", Required: true},
	},
	Tips: []string{
		"Create the button-trigger workflow first, then create the button field, then bind them with this command.",
		"Button field JSON must not include workflow_id; binding is managed only through button_rule APIs.",
		"workflow-id must start with wkf; do not pass a tbl table ID or raw internal automation ID.",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return validateButtonBind(runtime)
	},
	DryRun: dryRunButtonBind,
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeButtonBind(runtime)
	},
}

var BaseButtonGet = common.Shortcut{
	Service:     "base",
	Command:     "+button-get",
	Description: "Get the workflow bound to a button field",
	Risk:        "read",
	Scopes:      []string{"base:field:read", "base:workflow:read"},
	AuthTypes:   authTypes(),
	Flags: []common.Flag{
		baseTokenFlag(true),
		tableRefFlag(true),
		fieldRefFlag(true),
	},
	Tips: []string{
		"Returns the button_rule binding for a button field; use +workflow-get for workflow details.",
		"The binding workflow_id is the public wkf-prefixed workflow ID.",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return validateButtonRuleLocator(runtime)
	},
	DryRun: dryRunButtonGet,
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeButtonGet(runtime)
	},
}

var BaseButtonUnbind = common.Shortcut{
	Service:     "base",
	Command:     "+button-unbind",
	Description: "Unbind a button field from its workflow",
	Risk:        "high-risk-write",
	Scopes:      []string{"base:field:update", "base:workflow:update"},
	AuthTypes:   authTypes(),
	Flags: []common.Flag{
		baseTokenFlag(true),
		tableRefFlag(true),
		fieldRefFlag(true),
	},
	Tips: []string{
		"Unbind only removes the button_rule relation; it does not delete the button field or workflow.",
		"Agent guidance: for high-risk writes, explain the exact target and pass --yes without asking again when the user has already asked you to perform this action.",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return validateButtonRuleLocator(runtime)
	},
	DryRun: dryRunButtonUnbind,
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeButtonUnbind(runtime)
	},
}

func validateButtonRuleLocator(runtime *common.RuntimeContext) error {
	if strings.TrimSpace(runtime.Str("base-token")) == "" {
		return baseFlagErrorf("--base-token must not be blank")
	}
	if strings.TrimSpace(baseTableID(runtime)) == "" {
		return baseFlagErrorf("--table-id must not be blank")
	}
	if strings.TrimSpace(runtime.Str("field-id")) == "" {
		return baseFlagErrorf("--field-id must not be blank")
	}
	return nil
}

func validateButtonBind(runtime *common.RuntimeContext) error {
	if err := validateButtonRuleLocator(runtime); err != nil {
		return err
	}
	workflowID := strings.TrimSpace(runtime.Str("workflow-id"))
	if workflowID == "" {
		return baseFlagErrorf("--workflow-id must not be blank")
	}
	if !strings.HasPrefix(workflowID, "wkf") {
		return baseFlagErrorf("--workflow-id must be a public workflow ID with wkf prefix")
	}
	return nil
}

func buttonRulePath(runtime *common.RuntimeContext) string {
	return baseV3Path(
		"bases", runtime.Str("base-token"),
		"tables", baseTableID(runtime),
		"fields", runtime.Str("field-id"),
		"button_rule",
	)
}

func dryRunButtonBind(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	body := map[string]interface{}{"workflow_id": runtime.Str("workflow-id")}
	return common.NewDryRunAPI().
		PUT("/open-apis/base/v3/bases/:base_token/tables/:table_id/fields/:field_id/button_rule").
		Body(body).
		Set("base_token", runtime.Str("base-token")).
		Set("table_id", baseTableID(runtime)).
		Set("field_id", runtime.Str("field-id"))
}

func dryRunButtonGet(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	return common.NewDryRunAPI().
		GET("/open-apis/base/v3/bases/:base_token/tables/:table_id/fields/:field_id/button_rule").
		Set("base_token", runtime.Str("base-token")).
		Set("table_id", baseTableID(runtime)).
		Set("field_id", runtime.Str("field-id"))
}

func dryRunButtonUnbind(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	return common.NewDryRunAPI().
		DELETE("/open-apis/base/v3/bases/:base_token/tables/:table_id/fields/:field_id/button_rule").
		Set("base_token", runtime.Str("base-token")).
		Set("table_id", baseTableID(runtime)).
		Set("field_id", runtime.Str("field-id"))
}

func executeButtonBind(runtime *common.RuntimeContext) error {
	body := map[string]interface{}{"workflow_id": runtime.Str("workflow-id")}
	data, err := baseV3CallAny(runtime, "PUT", buttonRulePath(runtime), nil, body)
	if err != nil {
		return err
	}
	runtime.Out(map[string]interface{}{"button_rule": data, "bound": true}, nil)
	return nil
}

func executeButtonGet(runtime *common.RuntimeContext) error {
	data, err := baseV3CallAny(runtime, "GET", buttonRulePath(runtime), nil, nil)
	if err != nil {
		return err
	}
	runtime.Out(map[string]interface{}{"button_rule": data}, nil)
	return nil
}

func executeButtonUnbind(runtime *common.RuntimeContext) error {
	data, err := baseV3CallAny(runtime, "DELETE", buttonRulePath(runtime), nil, nil)
	if err != nil {
		return err
	}
	runtime.Out(map[string]interface{}{"button_rule": data, "unbound": true}, nil)
	return nil
}
