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

func validateButtonRuleBind(runtime *common.RuntimeContext) error {
	if err := validateButtonRuleLocator(runtime); err != nil {
		return err
	}
	workflowProvided := runtime.Changed("workflow-id")
	targetProvided := runtime.Changed("target-json")
	if workflowProvided == targetProvided {
		return baseFlagErrorf("exactly one of --workflow-id and --target-json is required")
	}
	if targetProvided {
		_, err := parseButtonRuleTarget(runtime.Str("target-json"))
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

func buttonRuleBindBody(runtime *common.RuntimeContext) (map[string]interface{}, error) {
	if runtime.Changed("target-json") {
		target, err := parseButtonRuleTarget(runtime.Str("target-json"))
		if err != nil {
			return nil, err
		}
		return map[string]interface{}{"target": target}, nil
	}
	return map[string]interface{}{"workflow_id": strings.TrimSpace(runtime.Str("workflow-id"))}, nil
}

func buttonRuleDryRun(runtime *common.RuntimeContext, method string, body map[string]interface{}) *common.DryRunAPI {
	dryRun := common.NewDryRunAPI().
		GET("/open-apis/base/v3/bases/:base_token/tables/:table_id/fields/:field_ref").
		Desc("Resolve --field-id as a field ID or name")
	if method == "PUT" {
		dryRun.PUT("/open-apis/base/v3/bases/:base_token/tables/:table_id/fields/:resolved_field_id/button_rule").
			Desc("Use the canonical field ID returned by step 1").
			Body(body)
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

func addButtonRuleVerificationHint(data map[string]interface{}) map[string]interface{} {
	if data == nil {
		data = make(map[string]interface{})
	}
	data["verification_hint"] = "run +button-rule-get for a fresh persisted readback"
	return data
}

var BaseButtonRuleBind = common.Shortcut{
	Service:     "base",
	Command:     "+button-rule-bind",
	Description: "Set the action target of a button field",
	Risk:        "write",
	Scopes:      []string{"base:field:read", "base:field:update"},
	AuthTypes:   authTypes(),
	Flags: []common.Flag{
		baseTokenFlag(true),
		tableRefFlag(true),
		fieldRefFlag(true),
		{Name: "workflow-id", Desc: "legacy workflow target: public workflow ID returned by workflow commands (wkf prefix)"},
		{Name: "target-json", Desc: "strict ButtonTarget JSON; workflow, open_record, open_link, or open_form", Input: []string{common.File, common.Stdin}},
	},
	Tips: []string{
		"Pass exactly one of --workflow-id and --target-json.",
		"Button appearance belongs to +field-create/update; actions belong to this command.",
		"workflow-id must be the public wkf ID returned by workflow commands; never pass an internal numeric workflow ID.",
		"Binding is independent from workflow enablement. Query with +button-rule-get, then call +workflow-enable only if the user wants it active.",
		"A successful PUT is request acceptance; use +button-rule-get for persisted readback.",
	},
	Validate: func(_ context.Context, runtime *common.RuntimeContext) error {
		return validateButtonRuleBind(runtime)
	},
	DryRun: func(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		body, _ := buttonRuleBindBody(runtime)
		return buttonRuleDryRun(runtime, "PUT", body)
	},
	Execute: func(_ context.Context, runtime *common.RuntimeContext) error {
		fieldID, err := resolveButtonRuleFieldID(runtime)
		if err != nil {
			return err
		}
		body, err := buttonRuleBindBody(runtime)
		if err != nil {
			return err
		}
		data, err := baseV3Call(runtime, "PUT", buttonRulePath(runtime, fieldID), nil, body)
		if err != nil {
			return err
		}
		runtime.Out(addButtonRuleVerificationHint(data), nil)
		return nil
	},
}

var BaseButtonRuleGet = common.Shortcut{
	Service:     "base",
	Command:     "+button-rule-get",
	Description: "Get the action target of a button field",
	Risk:        "read",
	Scopes:      []string{"base:field:read"},
	AuthTypes:   authTypes(),
	Flags: []common.Flag{
		baseTokenFlag(true),
		tableRefFlag(true),
		fieldRefFlag(true),
	},
	Tips: []string{
		"Returns bound=false and target=null when no action is configured.",
		"When target.type is workflow, target.id is a public wkf ID suitable for workflow commands and +button-rule-bind.",
		"Unknown targets are returned as read-only raw data and cannot be passed to +button-rule-bind.",
	},
	Validate: func(_ context.Context, runtime *common.RuntimeContext) error {
		return validateButtonRuleLocator(runtime)
	},
	DryRun: func(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		return buttonRuleDryRun(runtime, "GET", nil)
	},
	Execute: func(_ context.Context, runtime *common.RuntimeContext) error {
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
	Description: "Clear the action target of a button field",
	Risk:        "write",
	Scopes:      []string{"base:field:read", "base:field:update"},
	AuthTypes:   authTypes(),
	Flags: []common.Flag{
		baseTokenFlag(true),
		tableRefFlag(true),
		fieldRefFlag(true),
	},
	Tips: []string{
		"Unbind clears any direct action or workflow target without deleting the field, form, view, or workflow.",
		"Repeat unbind is safe; use +button-rule-get for persisted readback.",
	},
	Validate: func(_ context.Context, runtime *common.RuntimeContext) error {
		return validateButtonRuleLocator(runtime)
	},
	DryRun: func(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		return buttonRuleDryRun(runtime, "PUT", map[string]interface{}{"workflow_id": ""})
	},
	Execute: func(_ context.Context, runtime *common.RuntimeContext) error {
		fieldID, err := resolveButtonRuleFieldID(runtime)
		if err != nil {
			return err
		}
		data, err := baseV3Call(runtime, "PUT", buttonRulePath(runtime, fieldID), nil, map[string]interface{}{"workflow_id": ""})
		if err != nil {
			return err
		}
		runtime.Out(addButtonRuleVerificationHint(data), nil)
		return nil
	},
}
