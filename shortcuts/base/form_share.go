// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"

	"github.com/larksuite/cli/shortcuts/common"
)

var formShareUpdateFlagNames = []string{
	"enabled",
	"access-scope",
	"allow-anonymous",
	"require-login",
}

var BaseFormShareGet = common.Shortcut{
	Service:     "base",
	Command:     "+form-share-get",
	Description: "Get form share status and settings",
	Risk:        "read",
	Scopes:      []string{"base:form:update"},
	AuthTypes:   authTypes(),
	Flags: []common.Flag{
		baseTokenFlag(true),
		{Name: "table-id", Desc: "table ID", Required: true},
		{Name: "form-id", Desc: "form ID", Required: true},
	},
	DryRun: func(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		return common.NewDryRunAPI().
			GET("/open-apis/base/v3/bases/:base_token/tables/:table_id/forms/:form_id/share").
			Set("base_token", runtime.Str("base-token")).
			Set("table_id", runtime.Str("table-id")).
			Set("form_id", runtime.Str("form-id"))
	},
	Execute: func(_ context.Context, runtime *common.RuntimeContext) error {
		data, err := baseV3Call(runtime, "GET", baseV3Path(
			"bases", runtime.Str("base-token"), "tables", runtime.Str("table-id"), "forms", runtime.Str("form-id"), "share",
		), nil, nil)
		if err != nil {
			return err
		}
		runtime.Out(data, nil)
		return nil
	},
}

var BaseFormShareUpdate = common.Shortcut{
	Service:     "base",
	Command:     "+form-share-update",
	Description: "Update form share status and settings",
	Risk:        "write",
	Scopes:      []string{"base:form:update"},
	AuthTypes:   authTypes(),
	Flags: []common.Flag{
		baseTokenFlag(true),
		{Name: "table-id", Desc: "table ID", Required: true},
		{Name: "form-id", Desc: "form ID", Required: true},
		{Name: "enabled", Type: "bool", Desc: "enable or disable form sharing"},
		{Name: "access-scope", Desc: "share access scope", Enum: shareAccessScopeEnums},
		{Name: "allow-anonymous", Type: "bool", Desc: "anonymize the submitter identity"},
		{Name: "require-login", Type: "bool", Desc: "require submitters to sign in before submitting"},
	},
	Tips: []string{
		"Boolean settings use PATCH semantics: pass --allow-anonymous=false or another boolean flag with =false to explicitly turn it off.",
		"--allow-anonymous controls submitter identity and --require-login controls sign-in; run separate update commands to change both settings.",
		"Update exactly one field per invocation; run separate commands to change multiple share fields.",
	},
	Validate: func(_ context.Context, runtime *common.RuntimeContext) error {
		return validateFormShareUpdate(runtime)
	},
	DryRun: func(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		return common.NewDryRunAPI().
			PATCH("/open-apis/base/v3/bases/:base_token/tables/:table_id/forms/:form_id/share").
			Body(buildFormShareUpdateBody(runtime)).
			Set("base_token", runtime.Str("base-token")).
			Set("table_id", runtime.Str("table-id")).
			Set("form_id", runtime.Str("form-id"))
	},
	Execute: func(_ context.Context, runtime *common.RuntimeContext) error {
		data, err := baseV3Call(runtime, "PATCH", baseV3Path(
			"bases", runtime.Str("base-token"), "tables", runtime.Str("table-id"), "forms", runtime.Str("form-id"), "share",
		), nil, buildFormShareUpdateBody(runtime))
		if err != nil {
			return err
		}
		runtime.Out(data, nil)
		return nil
	},
}

func validateFormShareUpdate(runtime *common.RuntimeContext) error {
	return validateSingleShareUpdate(runtime, formShareUpdateFlagNames...)
}

func buildFormShareUpdateBody(runtime *common.RuntimeContext) map[string]interface{} {
	body := map[string]interface{}{}
	addCommonShareUpdateFields(runtime, body)

	settings := map[string]interface{}{}
	if runtime.Changed("allow-anonymous") {
		settings["allow_anonymous"] = runtime.Bool("allow-anonymous")
	}
	if runtime.Changed("require-login") {
		settings["require_login"] = runtime.Bool("require-login")
	}

	if len(settings) > 0 {
		body["settings"] = settings
	}
	return body
}
