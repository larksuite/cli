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

var BaseFormQuestionsUpdate = common.Shortcut{
	Service:     "base",
	Command:     "+form-questions-update",
	Description: "Update questions of a form in a Base table",
	Risk:        "write",
	Scopes:      []string{"base:form:update"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "base-token", Desc: "Base token (base_token)", Required: true},
		{Name: "table-id", Desc: "table ID", Required: true},
		{Name: "form-id", Desc: "form ID", Required: true},
		{Name: "questions", Desc: `questions JSON array, max 10 items, each item must include "id". Update uses full question overwrite semantics: omitted/empty fields are written as defaults/empty, so run +form-questions-list first and include existing values you want to keep. Supported fields: "id"(required),"title","description"(plain text or markdown link like [text](https://example.com)),"required","option_display_mode"(0=dropdown,1=vertical,2=horizontal,select only),"visible_rule"(display condition; same shape as view filter {"logic":"and","conditions":[["前序题目","==","是"]]}, field references another question's title/id; pass null or omit to clear). E.g. '[{"id":"q_001","title":"Updated?","required":true}]'`, Required: true},
	},
	Tips: []string{
		"Update uses full question overwrite semantics, not a patch.",
		"Run +form-questions-list first and include existing title/description/required/option_display_mode/visible_rule values you want to keep.",
		"Omitted fields reset to defaults; empty strings, null, and empty arrays are written as empty/clear when accepted by the API.",
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		api := common.NewDryRunAPI().
			PATCH("/open-apis/base/v3/bases/:base_token/tables/:table_id/forms/:form_id/questions").
			Set("base_token", runtime.Str("base-token")).
			Set("table_id", runtime.Str("table-id")).
			Set("form_id", runtime.Str("form-id"))
		// Transcribe the questions body verbatim so the preview shows exactly
		// what would be sent (including optional fields like visible_rule).
		var questions []interface{}
		if err := json.Unmarshal([]byte(runtime.Str("questions")), &questions); err == nil {
			api.Body(map[string]interface{}{"questions": questions})
		}
		return api
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

		data, err := baseV3Call(runtime, "PATCH",
			baseV3Path("bases", baseToken, "tables", tableId, "forms", formId, "questions"),
			nil, map[string]interface{}{"questions": questions})
		if err != nil {
			return err
		}

		items, _ := data["items"].([]interface{})
		if len(items) == 0 {
			items, _ = data["questions"].([]interface{})
		}
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
			fmt.Fprintf(w, "\n%d question(s) updated\n", len(items))
		})
		return nil
	},
}
