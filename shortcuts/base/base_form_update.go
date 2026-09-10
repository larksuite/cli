// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"io"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
)

const (
	formDisplayModeList = "list"
	formDisplayModeStep = "step"
)

var BaseFormUpdate = common.Shortcut{
	Service:     "base",
	Command:     "+form-update",
	Description: "Update a form in a Base table",
	Risk:        "write",
	Scopes:      []string{"base:form:update"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "base-token", Desc: "Base token (base_token)", Required: true},
		{Name: "table-id", Desc: "table ID", Required: true},
		{Name: "form-id", Desc: "form ID", Required: true},
		{Name: "name", Desc: "new form name"},
		{Name: "description", Desc: "new form description (plain text or markdown link like [text](https://example.com))"},
		{Name: "display-mode", Desc: "form display mode: list (traditional) or step (one question per page)", Enum: []string{formDisplayModeList, formDisplayModeStep}},
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		return common.NewDryRunAPI().
			PATCH("/open-apis/base/v3/bases/:base_token/tables/:table_id/forms/:form_id").
			Body(buildFormUpdateBody(runtime)).
			Set("base_token", runtime.Str("base-token")).
			Set("table_id", runtime.Str("table-id")).
			Set("form_id", runtime.Str("form-id"))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		baseToken := runtime.Str("base-token")
		tableId := runtime.Str("table-id")
		formId := runtime.Str("form-id")
		body := buildFormUpdateBody(runtime)

		data, err := baseV3Call(runtime, "PATCH",
			baseV3Path("bases", baseToken, "tables", tableId, "forms", formId), nil, body)
		if err != nil {
			return err
		}
		form, err := decodeBaseFormResponse(data)
		if err != nil {
			return err
		}

		runtime.OutFormat(form, nil, func(w io.Writer) {
			output.PrintTable(w, []map[string]interface{}{baseFormTableRow(form)})
		})
		return nil
	},
}

func buildFormUpdateBody(runtime *common.RuntimeContext) baseFormUpdateRequest {
	body := baseFormUpdateRequest{}
	if name := runtime.Str("name"); name != "" {
		body.Name = name
	}
	if description := runtime.Str("description"); description != "" {
		body.Description = description
	}
	if runtime.Changed("display-mode") {
		var displayMode int
		switch runtime.Str("display-mode") {
		case formDisplayModeStep:
			displayMode = 2
		case formDisplayModeList:
			displayMode = 1
		}
		body.DisplayMode = &displayMode
	}
	return body
}
