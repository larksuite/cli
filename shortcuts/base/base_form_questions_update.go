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
		{Name: "questions", Desc: `questions JSON array, max 10 items, each item must include "id". Supported fields: "id"(required),"title","description"(plain text or markdown link like [text](https://example.com)),"required","option_display_mode"(0=dropdown,1=vertical,2=horizontal,select only),"attachment"({"file_types":["all"]},attachment only). E.g. '[{"id":"q_001","title":"Updated?","required":true}]'`, Required: true},
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		dr := common.NewDryRunAPI().
			PATCH("/open-apis/base/v3/bases/:base_token/tables/:table_id/forms/:form_id/questions").
			Set("base_token", runtime.Str("base-token")).
			Set("table_id", runtime.Str("table-id")).
			Set("form_id", runtime.Str("form-id"))
		if questions, err := parseUpdateFormQuestions(runtime.Str("questions")); err == nil {
			dr.Body(map[string]interface{}{"questions": questions})
		}
		return dr
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		baseToken := runtime.Str("base-token")
		tableId := runtime.Str("table-id")
		formId := runtime.Str("form-id")
		questionsJSON := runtime.Str("questions")

		questions, err := parseUpdateFormQuestions(questionsJSON)
		if err != nil {
			return output.Errorf(output.ExitValidation, "invalid_json", "--questions must be a valid JSON array: %s", err)
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

func parseUpdateFormQuestions(questionsJSON string) ([]interface{}, error) {
	var questions []interface{}
	if err := json.Unmarshal([]byte(questionsJSON), &questions); err != nil {
		return nil, err
	}
	normalizeUpdateFormQuestionAttachments(questions)
	return questions, nil
}

func normalizeUpdateFormQuestionAttachments(questions []interface{}) {
	for _, question := range questions {
		q, ok := question.(map[string]interface{})
		if !ok || q["type"] != "attachment" {
			continue
		}
		attachment, ok := q["attachment"].(map[string]interface{})
		if !ok {
			q["attachment"] = map[string]interface{}{"file_types": []interface{}{"all"}}
			continue
		}
		if fileTypes, ok := attachment["file_types"]; !ok || fileTypes == nil {
			attachment["file_types"] = []interface{}{"all"}
		}
	}
}
