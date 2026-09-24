// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"

	"github.com/larksuite/cli/shortcuts/common"
)

const formDisplaySettingsSegment = "display-settings"

// 展示模式对外一律是字符串枚举。SDK 内部另有一套数字枚举，
// 不要把它透出到命令输出或文档里。
var formDisplayModeEnums = []string{"traditional", "one_question_per_page"}

var BaseFormDisplaySettingsGet = common.Shortcut{
	Service:     "base",
	Command:     "+form-display-settings-get",
	Description: "Get the display settings (page layout) of a form",
	Risk:        "read",
	Scopes:      []string{"base:form:read"},
	AuthTypes:   authTypes(),
	Flags:       formRefFlags(),
	Tips: []string{
		"The response is {display_mode, revision}. Report display_mode as the returned string; never translate it into a numeric enum.",
		"revision is the form's current version; pass it back to +form-display-settings-update --revision when a concurrent edit must be detected.",
	},
	DryRun: func(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		return setFormRefDryRun(
			common.NewDryRunAPI().GET(formSubDryRunPath(formDisplaySettingsSegment)),
			runtime,
		)
	},
	Execute: func(_ context.Context, runtime *common.RuntimeContext) error {
		data, err := baseV3Call(runtime, "GET", formSubPath(runtime, formDisplaySettingsSegment), nil, nil)
		if err != nil {
			return err
		}
		runtime.Out(data, nil)
		return nil
	},
}

var BaseFormDisplaySettingsUpdate = common.Shortcut{
	Service:     "base",
	Command:     "+form-display-settings-update",
	Description: "Update the display settings (page layout) of a form",
	Risk:        "write",
	Scopes:      []string{"base:form:update"},
	AuthTypes:   authTypes(),
	Flags: append(formRefFlags(),
		common.Flag{
			Name:     "display-mode",
			Desc:     "page layout: traditional (all questions on one page) or one_question_per_page",
			Required: true,
			Enum:     formDisplayModeEnums,
		},
		common.Flag{
			Name: "revision",
			Type: "int",
			Desc: "expected current form revision; when supplied it must match, otherwise the API returns a conflict",
		},
	),
	Tips: []string{
		"display_mode is a string enum (traditional / one_question_per_page). Do not emit the SDK's numeric enum values in output or explanations.",
		"This command only switches the page layout. Theme color and background are not supported by the CLI; change the form title or description with +form-update.",
		"--revision is optional. When supplied it must match the form's current revision; a mismatch returns a conflict, so re-read with +form-display-settings-get and retry with the revision it returns.",
		"After a successful update, run +form-display-settings-get as a separate call to confirm the new display_mode and revision instead of trusting the write response alone.",
	},
	Validate: func(_ context.Context, runtime *common.RuntimeContext) error {
		if runtime.Changed("revision") && runtime.Int("revision") < 0 {
			return baseFlagErrorf("--revision must not be negative; omit it to skip the version check")
		}
		return nil
	},
	DryRun: func(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		return setFormRefDryRun(
			common.NewDryRunAPI().
				PATCH(formSubDryRunPath(formDisplaySettingsSegment)).
				Body(buildFormDisplaySettingsUpdateBody(runtime)),
			runtime,
		)
	},
	Execute: func(_ context.Context, runtime *common.RuntimeContext) error {
		data, err := baseV3Call(runtime, "PATCH", formSubPath(runtime, formDisplaySettingsSegment),
			nil, buildFormDisplaySettingsUpdateBody(runtime))
		if err != nil {
			return err
		}
		runtime.Out(data, nil)
		return nil
	},
}

// buildFormDisplaySettingsUpdateBody 组装 PATCH 请求体。display_mode 是必填
// flag 所以总是携带；revision 走 PATCH 语义——只有显式传入时才带上，否则
// 不发送该字段，避免把默认零值当成"期望版本 0"提交上去。
func buildFormDisplaySettingsUpdateBody(runtime *common.RuntimeContext) map[string]interface{} {
	body := map[string]interface{}{
		"display_mode": runtime.Str("display-mode"),
	}
	if runtime.Changed("revision") {
		body["revision"] = runtime.Int("revision")
	}
	return body
}
