// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
)

type buttonRuleFieldIdentity struct {
	ID      string `json:"id"`
	FieldID string `json:"field_id"`
}

func validateButtonRuleLocator(runtime *common.RuntimeContext) error {
	if strings.TrimSpace(runtime.Str("base-token")) == "" {
		return baseFlagErrorf("--base-token must not be blank")
	}
	if strings.TrimSpace(runtime.Str("table-id")) == "" {
		return baseFlagErrorf("--table-id must not be blank")
	}
	if strings.TrimSpace(runtime.Str("field-id")) == "" {
		return baseFlagErrorf("--field-id must not be blank")
	}
	return nil
}

func validateButtonRuleWorkflowID(runtime *common.RuntimeContext) error {
	if err := validateButtonRuleLocator(runtime); err != nil {
		return err
	}
	workflowID := strings.TrimSpace(runtime.Str("workflow-id"))
	if workflowID == "" {
		return baseFlagErrorf("--workflow-id must not be blank")
	}
	if !strings.HasPrefix(workflowID, "wkf") {
		return baseFlagErrorf("--workflow-id must be a public wkf workflow ID, not an internal numeric ID")
	}
	return nil
}

func resolveButtonRuleFieldID(runtime *common.RuntimeContext) (string, error) {
	fieldRef := strings.TrimSpace(runtime.Str("field-id"))
	data, err := baseV3Call(
		runtime,
		"GET",
		baseV3Path(
			"bases", runtime.Str("base-token"),
			"tables", baseTableID(runtime),
			"fields", fieldRef,
		),
		nil,
		nil,
	)
	if err != nil {
		return "", err
	}

	raw, err := json.Marshal(data)
	if err != nil {
		return "", errs.NewInternalError(errs.SubtypeSDKError, "failed to project resolved field identity: %v", err).WithCause(err)
	}
	var identity buttonRuleFieldIdentity
	if err := json.Unmarshal(raw, &identity); err != nil {
		return "", errs.NewInternalError(errs.SubtypeSDKError, "failed to decode resolved field identity: %v", err).WithCause(err)
	}
	fieldID := strings.TrimSpace(identity.ID)
	if fieldID == "" {
		fieldID = strings.TrimSpace(identity.FieldID)
	}
	if fieldID == "" {
		return "", errs.NewInternalError(errs.SubtypeInvalidResponse, "field resolution response is missing canonical field ID")
	}
	if !strings.HasPrefix(fieldID, "fld") {
		return "", errs.NewInternalError(errs.SubtypeInvalidResponse, "field resolution response returned invalid canonical field ID %q", fieldID)
	}
	return fieldID, nil
}

func buttonRulePath(runtime *common.RuntimeContext, fieldID string) string {
	return baseV3Path(
		"bases", runtime.Str("base-token"),
		"tables", baseTableID(runtime),
		"fields", fieldID,
		"button_rule",
	)
}

func buttonRuleDryRun(runtime *common.RuntimeContext, method, workflowID string) *common.DryRunAPI {
	dryRun := common.NewDryRunAPI().
		GET("/open-apis/base/v3/bases/:base_token/tables/:table_id/fields/:field_ref").
		Desc("Resolve --field-id as a field ID or name")
	if method == "PUT" {
		dryRun.PUT("/open-apis/base/v3/bases/:base_token/tables/:table_id/fields/:resolved_field_id/button_rule").
			Desc("Use the canonical field ID returned by step 1").
			Body(map[string]interface{}{"workflow_id": workflowID})
	} else {
		dryRun.GET("/open-apis/base/v3/bases/:base_token/tables/:table_id/fields/:resolved_field_id/button_rule").
			Desc("Use the canonical field ID returned by step 1")
	}
	return dryRun.
		Set("base_token", runtime.Str("base-token")).
		Set("table_id", baseTableID(runtime)).
		Set("field_ref", strings.TrimSpace(runtime.Str("field-id"))).
		Set("resolved_field_id", "<resolved_field_id>")
}

var BaseButtonRuleBind = common.Shortcut{
	Service:     "base",
	Command:     "+button-rule-bind",
	Description: "Bind a button field to a workflow",
	Risk:        "write",
	Scopes:      []string{"base:field:read", "base:field:update"},
	AuthTypes:   authTypes(),
	Flags: []common.Flag{
		baseTokenFlag(true),
		tableRefFlag(true),
		fieldRefFlag(true),
		{Name: "workflow-id", Desc: "public workflow ID returned by +workflow-create or +workflow-list (wkf prefix)", Required: true},
	},
	Tips: []string{
		"Use this after +workflow-create and +field-create; do not put workflow_id in the field JSON.",
		"workflow-id must be the public wkf ID returned by workflow commands; never pass an internal numeric workflow ID.",
		"Binding is independent from workflow enablement. Query with +button-rule-get, then call +workflow-enable only if the user wants it active.",
		"If binding fails after workflow and field creation, keep both IDs and retry this command instead of recreating them.",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return validateButtonRuleWorkflowID(runtime)
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		return buttonRuleDryRun(runtime, "PUT", strings.TrimSpace(runtime.Str("workflow-id")))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		fieldID, err := resolveButtonRuleFieldID(runtime)
		if err != nil {
			return err
		}
		body := map[string]interface{}{"workflow_id": strings.TrimSpace(runtime.Str("workflow-id"))}
		data, err := baseV3Call(runtime, "PUT", buttonRulePath(runtime, fieldID), nil, body)
		if err != nil {
			return err
		}
		runtime.Out(data, nil)
		return nil
	},
}

var BaseButtonRuleGet = common.Shortcut{
	Service:     "base",
	Command:     "+button-rule-get",
	Description: "Get the target bound to a button field",
	Risk:        "read",
	Scopes:      []string{"base:field:read"},
	AuthTypes:   authTypes(),
	Flags: []common.Flag{
		baseTokenFlag(true),
		tableRefFlag(true),
		fieldRefFlag(true),
	},
	Tips: []string{
		"Returns bound=false and target=null when the button field has no binding.",
		"When target.type is workflow, target.id is a public wkf ID suitable for +workflow-get, +workflow-enable, and +button-rule-bind.",
		"Use this after +button-rule-bind before enabling a newly created workflow.",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return validateButtonRuleLocator(runtime)
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		return buttonRuleDryRun(runtime, "GET", "")
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		fieldID, err := resolveButtonRuleFieldID(runtime)
		if err != nil {
			return err
		}
		data, err := baseV3Call(runtime, "GET", buttonRulePath(runtime, fieldID), nil, nil)
		if err != nil {
			return err
		}
		runtime.Out(data, nil)
		return nil
	},
}

var BaseButtonRuleUnbind = common.Shortcut{
	Service:     "base",
	Command:     "+button-rule-unbind",
	Description: "Remove the workflow binding from a button field",
	Risk:        "write",
	Scopes:      []string{"base:field:read", "base:field:update"},
	AuthTypes:   authTypes(),
	Flags: []common.Flag{
		baseTokenFlag(true),
		tableRefFlag(true),
		fieldRefFlag(true),
	},
	Tips: []string{
		"Unbind removes only the ButtonRule relation; it does not delete the field or workflow.",
		"Repeat unbind is safe and should leave the button field with bound=false.",
		"Use +button-rule-get after unbind when the agent needs readback evidence.",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return validateButtonRuleLocator(runtime)
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		return buttonRuleDryRun(runtime, "PUT", "")
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		fieldID, err := resolveButtonRuleFieldID(runtime)
		if err != nil {
			return err
		}
		data, err := baseV3Call(runtime, "PUT", buttonRulePath(runtime, fieldID), nil, map[string]interface{}{"workflow_id": ""})
		if err != nil {
			return err
		}
		runtime.Out(data, nil)
		return nil
	},
}
