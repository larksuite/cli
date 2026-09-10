// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"strings"

	"github.com/larksuite/cli/shortcuts/common"
)

const formSettingsScope = "base:form:update"

var formSettingsIdentityFlags = []common.Flag{
	baseTokenFlag(true),
	{Name: "table-id", Desc: "table ID", Required: true},
	{Name: "form-id", Desc: "form ID", Required: true},
}

var BaseFormSubmissionSettingsGet = newFormSettingsGetShortcut(
	"+form-submission-settings-get",
	"Get form submission settings",
	"submission-settings",
)

var BaseFormNotificationSettingsGet = newFormSettingsGetShortcut(
	"+form-notification-settings-get",
	"Get form notification settings",
	"notifications",
)

var BaseFormPostSubmitSettingsGet = newFormSettingsGetShortcut(
	"+form-post-submit-settings-get",
	"Get form post-submit settings",
	"submit-actions",
)

var BaseFormLotterySettingsGet = newFormSettingsGetShortcut(
	"+form-lottery-settings-get",
	"Get form lottery settings",
	"lottery",
)

var BaseFormSubmissionSettingsUpdate = common.Shortcut{
	Service:     "base",
	Command:     "+form-submission-settings-update",
	Description: "Update exactly one form submission setting",
	Risk:        "write",
	Scopes:      []string{formSettingsScope},
	AuthTypes:   authTypes(),
	Flags: append(formSettingsFlags(),
		jsonSettingsFlag("submit-period", "submit period JSON"),
		jsonSettingsFlag("per-user-submit-limit", "per-user submit limit JSON"),
		jsonSettingsFlag("global-submit-limit", "global submit limit JSON"),
		jsonSettingsFlag("allow-modify-submission", "allow-modify-submission JSON"),
		jsonSettingsFlag("ai-voice-input", "AI voice input JSON"),
	),
	Tips: []string{
		"Run +form-submission-settings-get first, then update exactly one setting.",
		"Boolean values use presence-aware JSON; {\"enabled\":false} explicitly disables without clearing saved parameters.",
	},
	Validate: validateFormSubmissionSettingsUpdate,
	DryRun: func(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		body, _ := buildFormSubmissionSettingsBody(runtime)
		return formSettingsDryRun(runtime, "submission-settings", body)
	},
	Execute: func(_ context.Context, runtime *common.RuntimeContext) error {
		body, err := buildFormSubmissionSettingsBody(runtime)
		if err != nil {
			return err
		}
		return executeFormSettingsUpdate(runtime, "submission-settings", body)
	},
}

var BaseFormNotificationSettingsUpdate = common.Shortcut{
	Service:     "base",
	Command:     "+form-notification-settings-update",
	Description: "Update exactly one form notification setting",
	Risk:        "write",
	Scopes:      []string{formSettingsScope},
	AuthTypes:   authTypes(),
	Flags: append(formSettingsFlags(),
		common.Flag{Name: "locale", Desc: "shared notification locale, for example zh_cn or en_us"},
		jsonSettingsFlag("submit-notification", "submit notification JSON"),
		jsonSettingsFlag("scheduled-notification", "scheduled notification JSON"),
	),
	Tips: []string{
		"Run +form-notification-settings-get first, then update locale or one notification.",
		"Notification targets accept user open_id values only; chat_id and internal IDs are rejected locally.",
	},
	Validate: validateFormNotificationSettingsUpdate,
	DryRun: func(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		body, _ := buildFormNotificationSettingsBody(runtime)
		return formSettingsDryRun(runtime, "notifications", body)
	},
	Execute: func(_ context.Context, runtime *common.RuntimeContext) error {
		body, err := buildFormNotificationSettingsBody(runtime)
		if err != nil {
			return err
		}
		return executeFormSettingsUpdate(runtime, "notifications", body)
	},
}

var BaseFormPostSubmitSettingsUpdate = common.Shortcut{
	Service:     "base",
	Command:     "+form-post-submit-settings-update",
	Description: "Update exactly one form post-submit setting",
	Risk:        "write",
	Scopes:      []string{formSettingsScope},
	AuthTypes:   authTypes(),
	Flags: append(formSettingsFlags(),
		common.Flag{Name: "revision", Type: "int", Desc: "current form revision", Required: true},
		jsonSettingsFlag("submit-result-page", "submit result page JSON"),
		jsonSettingsFlag("redirect-after-submit", "redirect-after-submit JSON"),
	),
	Tips: []string{
		"Run +form-post-submit-settings-get first and pass its revision.",
		"Update the result page or redirect in one call, never both.",
	},
	Validate: validateFormPostSubmitSettingsUpdate,
	DryRun: func(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		body, _ := buildFormPostSubmitSettingsBody(runtime)
		return formSettingsDryRun(runtime, "submit-actions", body)
	},
	Execute: func(_ context.Context, runtime *common.RuntimeContext) error {
		body, err := buildFormPostSubmitSettingsBody(runtime)
		if err != nil {
			return err
		}
		return executeFormSettingsUpdate(runtime, "submit-actions", body)
	},
}

var BaseFormLotterySettingsUpdate = common.Shortcut{
	Service:     "base",
	Command:     "+form-lottery-settings-update",
	Description: "Enable, disable, update, or relink form lottery settings",
	Risk:        "write",
	Scopes:      []string{formSettingsScope},
	AuthTypes:   authTypes(),
	Flags: append(formSettingsFlags(),
		common.Flag{Name: "action", Desc: "lottery action", Required: true, Enum: []string{"enable", "disable", "update", "relink_winning_table"}},
		common.Flag{Name: "version", Type: "int", Desc: "current lottery version; required for update"},
		common.Flag{Name: "probability", Type: "int", Desc: "winning probability in ten-thousandths, from 0 to 10000"},
		jsonSettingsFlag("awarder-info", "public awarder information JSON"),
		jsonSettingsFlag("awards", "public awards JSON array"),
	),
	Tips: []string{
		"Run +form-lottery-settings-get first; action=update requires its version.",
		"icon_token is private and is rejected in award input and never printed by this command.",
	},
	Validate: validateFormLotterySettingsUpdate,
	DryRun: func(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		body, _ := buildFormLotterySettingsBody(runtime)
		return formSettingsActionDryRun(runtime, "lottery/actions", body)
	},
	Execute: func(_ context.Context, runtime *common.RuntimeContext) error {
		body, err := buildFormLotterySettingsBody(runtime)
		if err != nil {
			return err
		}
		return executeFormSettingsAction(runtime, "lottery/actions", body)
	},
}

func newFormSettingsGetShortcut(command, description, resource string) common.Shortcut {
	return common.Shortcut{
		Service:     "base",
		Command:     command,
		Description: description,
		Risk:        "read",
		Scopes:      []string{formSettingsScope},
		AuthTypes:   authTypes(),
		Flags:       formSettingsFlags(),
		DryRun: func(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
			return common.NewDryRunAPI().
				GET(formSettingsPathTemplate(resource)).
				Set("base_token", runtime.Str("base-token")).
				Set("table_id", runtime.Str("table-id")).
				Set("form_id", runtime.Str("form-id"))
		},
		Execute: func(_ context.Context, runtime *common.RuntimeContext) error {
			data, err := baseV3Call(runtime, "GET", formSettingsPath(runtime, resource), nil, nil)
			if err != nil {
				return err
			}
			runtime.Out(data, nil)
			return nil
		},
	}
}

func formSettingsFlags() []common.Flag {
	return append([]common.Flag(nil), formSettingsIdentityFlags...)
}

func jsonSettingsFlag(name, description string) common.Flag {
	return common.Flag{Name: name, Desc: description, Input: []string{common.File, common.Stdin}}
}

func formSettingsPathTemplate(resource string) string {
	return "/open-apis/base/v3/bases/:base_token/tables/:table_id/forms/:form_id/" + resource
}

func formSettingsPath(runtime *common.RuntimeContext, resource string) string {
	parts := []string{
		"bases", runtime.Str("base-token"),
		"tables", runtime.Str("table-id"),
		"forms", runtime.Str("form-id"),
	}
	parts = append(parts, strings.Split(resource, "/")...)
	return baseV3Path(parts...)
}

func formSettingsDryRun(runtime *common.RuntimeContext, resource string, body map[string]interface{}) *common.DryRunAPI {
	return common.NewDryRunAPI().
		PATCH(formSettingsPathTemplate(resource)).
		Body(body).
		Set("base_token", runtime.Str("base-token")).
		Set("table_id", runtime.Str("table-id")).
		Set("form_id", runtime.Str("form-id"))
}

func formSettingsActionDryRun(runtime *common.RuntimeContext, resource string, body map[string]interface{}) *common.DryRunAPI {
	return common.NewDryRunAPI().
		POST(formSettingsPathTemplate(resource)).
		Body(body).
		Set("base_token", runtime.Str("base-token")).
		Set("table_id", runtime.Str("table-id")).
		Set("form_id", runtime.Str("form-id"))
}

func executeFormSettingsUpdate(runtime *common.RuntimeContext, resource string, body map[string]interface{}) error {
	data, err := baseV3Call(runtime, "PATCH", formSettingsPath(runtime, resource), nil, body)
	if err != nil {
		return err
	}
	runtime.Out(data, nil)
	return nil
}

func executeFormSettingsAction(runtime *common.RuntimeContext, resource string, body map[string]interface{}) error {
	data, err := baseV3Call(runtime, "POST", formSettingsPath(runtime, resource), nil, body)
	if err != nil {
		return err
	}
	runtime.Out(data, nil)
	return nil
}
