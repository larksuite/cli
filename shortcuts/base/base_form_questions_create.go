// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
)

var BaseFormQuestionsCreate = common.Shortcut{
	Service:     "base",
	Command:     "+form-questions-create",
	Description: "Create questions for a form in a Base table",
	Risk:        "write",
	Scopes:      []string{"base:form:update"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "base-token", Desc: "Base token (base_token)", Required: true},
		{Name: "table-id", Desc: "table ID", Required: true},
		{Name: "form-id", Desc: "form ID", Required: true},
		{Name: "questions", Desc: `questions JSON array, max 10 items. Each item requires "title"(field title) and "type"(text/number/select/datetime/user/attachment/location). Optional fields: "description"(plain text or markdown link like [text](https://example.com)),"required","option_display_mode"(0=dropdown/1=vertical/2=horizontal,select only),"multiple"(bool,select/user),"options"([{"name":"opt","hue":"Blue"}],select only),"style"({"type":"plain/phone/url/email/barcode/rating","precision":2,"format":"yyyy/MM/dd","icon":"star","min":1,"max":5}). E.g. '[{"type":"text","title":"Your name","required":true}]'`, Required: true},
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		return common.NewDryRunAPI().
			POST("/open-apis/base/v3/bases/:base_token/tables/:table_id/forms/:form_id/questions").
			Set("base_token", runtime.Str("base-token")).
			Set("table_id", runtime.Str("table-id")).
			Set("form_id", runtime.Str("form-id"))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		baseToken := runtime.Str("base-token")
		tableId := runtime.Str("table-id")
		formId := runtime.Str("form-id")
		questionsJSON := runtime.Str("questions")

		var questions []interface{}
		if err := json.Unmarshal([]byte(questionsJSON), &questions); err != nil {
			return baseValidationErrorf("--questions must be a valid JSON array: %s", err)
		}

		data, err := baseV3Call(runtime, "POST",
			baseV3Path("bases", baseToken, "tables", tableId, "forms", formId, "questions"),
			nil, map[string]interface{}{"questions": questions})
		if err != nil {
			return err
		}

		items, _ := data["questions"].([]interface{})
		outData := map[string]interface{}{"questions": items}

		runtime.OutFormat(outData, nil, func(w io.Writer) {
			var rows []map[string]interface{}
			for _, item := range items {
				m, _ := item.(map[string]interface{})
				rows = append(rows, map[string]interface{}{
					"id":       m["id"],
					"title":    m["title"],
					"required": m["required"],
				})
			}
			output.PrintTable(w, rows)
			fmt.Fprintf(w, "\n%d question(s) created\n", len(items))
		})
		return nil
	},
}
