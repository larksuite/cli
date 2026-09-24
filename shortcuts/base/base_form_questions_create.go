// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

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
		{Name: "questions", Desc: `questions JSON array, max 10 items. Supports two shapes: create a new field question with "title"(field title) and "type"(text/number/select/datetime/user/attachment/location), or add an existing field as a question with "use_existing_field":true and "field_id"(field ID/name). Optional form fields: "description"(plain text or markdown link like [text](https://example.com)),"required","option_display_mode"(0=dropdown/1=vertical/2=horizontal,select only),"visible_rule"(display condition; same shape as view filter {"logic":"and","conditions":[["前序题目","==","是"]]}, field references another question's title/id, empty/absent = always shown), "input_range_limit"(fill-range rule; object keyed by "type": text_length{min_length?,max_length?} / number_range{min_value?,max_value?} / date_range{earliest?,latest?} where each bound is {"fixed_ms":int} or {"dynamic":{"anchor":today|monday|friday|saturday|sunday|month_start|month_end,"offset_days"?:int}} (mutually exclusive); field names are snake_case only; pass null to clear). New field questions also support "multiple","options","style". E.g. '[{"type":"text","title":"Your name","required":true}]' or '[{"use_existing_field":true,"field_id":"fldEmail","title":"Email"}]'`, Required: true},
	},
	Tips: []string{
		"If the form may already contain questions and has not been checked, run +form-questions-list for the same --base-token, --table-id, and --form-id. A verified empty form can create directly.",
		"New field questions create fields in the form's table; question IDs are field IDs. Use use_existing_field=true with field_id to add an existing field without creating another field.",
		"Unless the user explicitly requests a separate same-title question, update an existing title with +form-questions-update instead of creating a duplicate.",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		_, err := parseFormQuestionsCreate(runtime.Str("questions"))
		return err
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		questions, _ := parseFormQuestionsCreate(runtime.Str("questions"))
		return common.NewDryRunAPI().
			POST("/open-apis/base/v3/bases/:base_token/tables/:table_id/forms/:form_id/questions").
			Set("base_token", runtime.Str("base-token")).
			Set("table_id", runtime.Str("table-id")).
			Set("form_id", runtime.Str("form-id")).
			Body(map[string]interface{}{"questions": questions})
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		baseToken := runtime.Str("base-token")
		tableId := runtime.Str("table-id")
		formId := runtime.Str("form-id")
		questionsJSON := runtime.Str("questions")

		questions, err := parseFormQuestionsCreate(questionsJSON)
		if err != nil {
			return err
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

// formQuestionsMaxItems 单次提交的题目条数上限，create 与 update 共用同一限制。
const formQuestionsMaxItems = 10

func parseFormQuestionsCreate(raw string) ([]interface{}, error) {
	var questions []interface{}
	if err := json.Unmarshal([]byte(raw), &questions); err != nil {
		return nil, baseValidationErrorf("--questions must be a valid JSON array: %s", err)
	}
	if questions == nil {
		return nil, baseValidationErrorf("--questions must be a non-null JSON array")
	}
	if len(questions) > formQuestionsMaxItems {
		return nil, baseValidationErrorf("--questions must contain at most %d items", formQuestionsMaxItems)
	}
	for i, question := range questions {
		item, ok := question.(map[string]interface{})
		if !ok {
			return nil, baseValidationErrorf("--questions item %d must be an object", i+1)
		}
		if useExistingField, _ := item["use_existing_field"].(bool); useExistingField {
			fieldID, ok := item["field_id"].(string)
			if !ok || strings.TrimSpace(fieldID) == "" {
				return nil, baseValidationErrorf("--questions item %d with use_existing_field must include a non-empty string \"field_id\"", i+1)
			}
			continue
		}
		title, ok := item["title"].(string)
		if !ok || strings.TrimSpace(title) == "" {
			return nil, baseValidationErrorf("--questions item %d must include a non-empty string \"title\"", i+1)
		}
		questionType, ok := item["type"].(string)
		if !ok || strings.TrimSpace(questionType) == "" {
			return nil, baseValidationErrorf("--questions item %d must include a non-empty string \"type\"", i+1)
		}
	}
	if err := validateQuestionsInputRangeLimit(questions); err != nil {
		return nil, err
	}
	return questions, nil
}

// validateQuestionsInputRangeLimit 对每个题目上的 input_range_limit 做结构校验。
// create 与 update 共用同一套规则结构，传 null 清除限制两条命令都接受。
func validateQuestionsInputRangeLimit(questions []interface{}) error {
	for i, question := range questions {
		item, ok := question.(map[string]interface{})
		if !ok {
			continue
		}
		raw, ok := item["input_range_limit"]
		if !ok {
			continue
		}
		if err := validateInputRangeLimit(fmt.Sprintf("--questions item %d", i+1), raw); err != nil {
			return err
		}
	}
	return nil
}
