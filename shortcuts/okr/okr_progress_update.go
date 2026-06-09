// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package okr

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"strconv"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
)

// updateProgressRecordParams holds the parsed parameters for updating a progress.
type updateProgressRecordParams struct {
	ProgressID   string
	ContentV1    *ContentBlockV1
	ProgressRate *ProgressRateV1
	UserIDType   string
}

// parseUpdateProgressRecordParams parses and validates flags from runtime into request-ready parameters.
func parseUpdateProgressRecordParams(runtime *common.RuntimeContext) (*updateProgressRecordParams, error) {
	content := runtime.Str("content")
	var cb ContentBlock
	if err := json.Unmarshal([]byte(content), &cb); err != nil {
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--content must be valid ContentBlock JSON: %s", err).WithParam("--content").WithCause(err)
	}
	contentV1 := cb.ToV1()

	var progressRate *ProgressRateV1
	if v := runtime.Str("progress-percent"); v != "" {
		percent, err := strconv.ParseFloat(v, 64)
		if err != nil || math.IsNaN(percent) || math.IsInf(percent, 0) || percent < -99999999999 || percent > 99999999999 {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--progress-percent must be a number between -99999999999 and 99999999999").WithParam("--progress-percent")
		}
		progressRate = &ProgressRateV1{Percent: &percent}
		if s := runtime.Str("progress-status"); s != "" {
			status, ok := ParseProgressStatus(s)
			if !ok {
				return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--progress-status must be one of: normal | overdue | done").WithParam("--progress-status")
			}
			progressRate.Status = int32Ptr(int32(status))
		}
	}

	return &updateProgressRecordParams{
		ProgressID:   runtime.Str("progress-id"),
		ContentV1:    contentV1,
		ProgressRate: progressRate,
		UserIDType:   runtime.Str("user-id-type"),
	}, nil
}

// OKRUpdateProgressRecord updates a progress.
var OKRUpdateProgressRecord = common.Shortcut{
	Service:     "okr",
	Command:     "+progress-update",
	Description: "Update an OKR progress",
	Risk:        "write",
	Scopes:      []string{"okr:okr.progress:writeonly"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "progress-id", Desc: "progress ID (int64)", Required: true},
		{Name: "content", Desc: "progress content in ContentBlock JSON format", Required: true, Input: []string{common.File, common.Stdin}},
		{Name: "progress-percent", Desc: "progress percentage"},
		{Name: "progress-status", Desc: "progress status: normal | overdue | done", Enum: []string{"normal", "overdue", "done"}},
		{Name: "user-id-type", Default: "open_id", Desc: "user ID type: open_id | union_id | user_id"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		progressID := runtime.Str("progress-id")
		if progressID == "" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--progress-id is required").WithParam("--progress-id")
		}
		if id, err := strconv.ParseInt(progressID, 10, 64); err != nil || id <= 0 {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--progress-id must be a positive int64").WithParam("--progress-id")
		}

		content := runtime.Str("content")
		if content == "" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--content is required").WithParam("--content")
		}
		if err := common.RejectDangerousCharsTyped("--content", content); err != nil {
			return err
		}
		var cb ContentBlock
		if err := json.Unmarshal([]byte(content), &cb); err != nil {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--content must be valid ContentBlock JSON: %s", err).WithParam("--content").WithCause(err)
		}

		if v := runtime.Str("progress-percent"); v != "" {
			percent, err := strconv.ParseFloat(v, 64)
			if err != nil || math.IsNaN(percent) || math.IsInf(percent, 0) || percent < -99999999999 || percent > 99999999999 {
				return errs.NewValidationError(errs.SubtypeInvalidArgument, "--progress-percent must be a number between -99999999999 and 99999999999").WithParam("--progress-percent")
			}
		}
		if v := runtime.Str("progress-status"); v != "" {
			if _, ok := ParseProgressStatus(v); !ok {
				return errs.NewValidationError(errs.SubtypeInvalidArgument, "--progress-status must be one of: normal | overdue | done").WithParam("--progress-status")
			}
			if v := runtime.Str("progress-percent"); v == "" {
				return errs.NewValidationError(errs.SubtypeInvalidArgument, "--progress-percent must provided with --progress-status").WithParam("--progress-percent")
			}
		}

		idType := runtime.Str("user-id-type")
		if idType != "open_id" && idType != "union_id" && idType != "user_id" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--user-id-type must be one of: open_id | union_id | user_id").WithParam("--user-id-type")
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		p, _ := parseUpdateProgressRecordParams(runtime)
		params := map[string]interface{}{
			"user_id_type": p.UserIDType,
		}
		body := map[string]interface{}{
			"content": p.ContentV1,
		}
		if p.ProgressRate != nil {
			body["progress_rate"] = p.ProgressRate
		}
		return common.NewDryRunAPI().
			PUT("/open-apis/okr/v1/progress_records/:progress_id").
			Params(params).
			Body(body).
			Set("progress_id", p.ProgressID).
			Desc("Update OKR progress")
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		p, err := parseUpdateProgressRecordParams(runtime)
		if err != nil {
			return err
		}

		body := map[string]interface{}{
			"content": p.ContentV1,
		}
		if p.ProgressRate != nil {
			body["progress_rate"] = p.ProgressRate
		}

		queryParams := map[string]interface{}{"user_id_type": p.UserIDType}

		path := fmt.Sprintf("/open-apis/okr/v1/progress_records/%s", p.ProgressID)
		data, err := runtime.CallAPITyped("PUT", path, queryParams, body)
		if err != nil {
			return err
		}

		record, err := parseProgressRecord(data)
		if err != nil {
			return err
		}

		resp := record.ToResp()
		result := map[string]interface{}{
			"progress": resp,
		}

		runtime.OutFormat(result, nil, func(w io.Writer) {
			fmt.Fprintf(w, "Updated Progress [%s]\n", resp.ID)
			fmt.Fprintf(w, "  ModifyTime: %s\n", resp.ModifyTime)
			if resp.ProgressRate != nil && resp.ProgressRate.Percent != nil {
				fmt.Fprintf(w, "  Progress: %.1f%%\n", *resp.ProgressRate.Percent)
			}
			if resp.Content != nil {
				fmt.Fprintf(w, "  Content: %s\n", *resp.Content)
			}
		})
		return nil
	},
}
