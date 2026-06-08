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
		{Name: "question-ids", Desc: `JSON array of question IDs to delete, max 10 items, e.g. '["q_001","q_002"]'`, Required: true},
	},
	Tips: []string{
		baseHighRiskYesTip,
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		return common.NewDryRunAPI().
			DELETE("/open-apis/base/v3/bases/:base_token/tables/:table_id/forms/:form_id/questions").
			Set("base_token", runtime.Str("base-token")).
			Set("table_id", runtime.Str("table-id")).
			Set("form_id", runtime.Str("form-id"))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		baseToken := runtime.Str("base-token")
		tableId := runtime.Str("table-id")
		formId := runtime.Str("form-id")
		questionIdsJSON := runtime.Str("question-ids")

		var questionIds []string
		if err := json.Unmarshal([]byte(questionIdsJSON), &questionIds); err != nil {
			return baseValidationErrorf("--question-ids must be a valid JSON array of strings: %s", err)
		}

		_, err := baseV3Call(runtime, "DELETE",
			baseV3Path("bases", baseToken, "tables", tableId, "forms", formId, "questions"),
			nil, map[string]interface{}{"question_ids": questionIds})
		if err != nil {
			return err
		}

		runtime.Out(map[string]interface{}{
			"deleted":      true,
			"question_ids": questionIds,
		}, nil)
		return nil
	},
}
