// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"encoding/json"

	"github.com/larksuite/cli/shortcuts/common"
)

var BaseFormQuestionsDelete = common.Shortcut{
	Service:     "base",
	Command:     "+form-questions-delete",
	Description: "Delete questions from a form in a Base table",
	Risk:        "high-risk-write",
	Scopes:      []string{"base:form:update"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "base-token", Desc: "Base token (base_token)", Required: true},
		{Name: "table-id", Desc: "table ID", Required: true},
		{Name: "form-id", Desc: "form ID", Required: true},
		{Name: "question-ids", Desc: `JSON array of question IDs (field IDs) to remove from the form, max 10 items. Default behavior also deletes the underlying fields and their record data; add --keep-field to preserve fields. E.g. '["q_001","q_002"]'`, Required: true},
		{Name: "keep-field", Type: "bool", Desc: "Only remove/hide the questions from the form; keep the underlying fields and existing record data so they can be added back later with +form-questions-create using use_existing_field=true and field_id. Default false deletes fields and data."},
	},
	Tips: []string{
		"Run +form-questions-list first and use returned question IDs; question IDs are field IDs.",
		"Default behavior is destructive: it deletes the underlying fields and all record data in those fields.",
		"Use --keep-field when you only want to remove questions from the form while preserving fields and data; those fields can be added back with +form-questions-create using use_existing_field=true and field_id.",
		baseHighRiskYesTip,
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		_, _, err := buildFormQuestionsDeleteBody(runtime)
		return err
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		body, _, err := buildFormQuestionsDeleteBody(runtime)
		if err != nil {
			return common.NewDryRunAPI().Set("error", err.Error())
		}
		return common.NewDryRunAPI().
			DELETE("/open-apis/base/v3/bases/:base_token/tables/:table_id/forms/:form_id/questions").
			Set("base_token", runtime.Str("base-token")).
			Set("table_id", runtime.Str("table-id")).
			Set("form_id", runtime.Str("form-id")).
			Body(body)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		baseToken := runtime.Str("base-token")
		tableId := runtime.Str("table-id")
		formId := runtime.Str("form-id")

		body, questionIds, err := buildFormQuestionsDeleteBody(runtime)
		if err != nil {
			return err
		}

		_, err = baseV3Call(runtime, "DELETE",
			baseV3Path("bases", baseToken, "tables", tableId, "forms", formId, "questions"),
			nil, body)
		if err != nil {
			return err
		}

		runtime.Out(map[string]interface{}{
			"deleted":      true,
			"question_ids": questionIds,
			"keep_field":   runtime.Bool("keep-field"),
		}, nil)
		return nil
	},
}

func buildFormQuestionsDeleteBody(runtime *common.RuntimeContext) (map[string]interface{}, []string, error) {
	var questionIds []string
	if err := json.Unmarshal([]byte(runtime.Str("question-ids")), &questionIds); err != nil {
		return nil, nil, baseValidationErrorf("--question-ids must be a valid JSON array of strings: %s", err)
	}
	if len(questionIds) == 0 {
		return nil, nil, baseValidationErrorf("--question-ids must contain at least 1 item")
	}
	if len(questionIds) > 10 {
		return nil, nil, baseValidationErrorf("--question-ids must contain at most 10 items")
	}
	for i, id := range questionIds {
		if id == "" {
			return nil, nil, baseValidationErrorf("--question-ids item %d must be a non-empty string", i+1)
		}
	}
	body := map[string]interface{}{"question_ids": questionIds}
	if runtime.Bool("keep-field") {
		body["keep_field"] = true
	}
	return body, questionIds, nil
}
