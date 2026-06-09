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
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/shortcuts/common"
)

// targetTypeAllowed values for --target-type flag
var targetTypeAllowed = map[string]int{
	"objective":  2,
	"key_result": 3,
}

// createProgressRecordParams holds the parsed parameters for creating a progress.
type createProgressRecordParams struct {
	ContentV1    *ContentBlockV1
	TargetID     string
	TargetType   int
	SourceTitle  string
	SourceURL    string
	ProgressRate *ProgressRateV1
	UserIDType   string
}

// parseCreateProgressRecordParams parses and validates flags from runtime into request-ready parameters.
func parseCreateProgressRecordParams(runtime *common.RuntimeContext) (*createProgressRecordParams, error) {
	content := runtime.Str("content")
	var cb ContentBlock
	if err := json.Unmarshal([]byte(content), &cb); err != nil {
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--content must be valid ContentBlock JSON: %s", err).WithParam("--content").WithCause(err)
	}
	contentV1 := cb.ToV1()

	targetType := runtime.Str("target-type")
	targetTypeVal := targetTypeAllowed[targetType]

	sourceTitle := runtime.Str("source-title")
	if sourceTitle == "" {
		sourceTitle = "created by lark-cli"
	}

	sourceURL := runtime.Str("source-url")
	if sourceURL == "" {
		sourceURL = core.ResolveOpenBaseURL(runtime.Config.Brand) + "/app"
	}

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

	return &createProgressRecordParams{
		ContentV1:    contentV1,
		TargetID:     runtime.Str("target-id"),
		TargetType:   targetTypeVal,
		SourceTitle:  sourceTitle,
		SourceURL:    sourceURL,
		ProgressRate: progressRate,
		UserIDType:   runtime.Str("user-id-type"),
	}, nil
}

// OKRCreateProgressRecord creates a progress.
var OKRCreateProgressRecord = common.Shortcut{
	Service:     "okr",
	Command:     "+progress-create",
	Description: "Create an OKR progress",
	Risk:        "write",
	Scopes:      []string{"okr:okr.progress:writeonly"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "content", Desc: "progress content in ContentBlock JSON format", Required: true, Input: []string{common.File, common.Stdin}},
		{Name: "target-id", Desc: "target ID (objective or key result ID)", Required: true},
		{Name: "target-type", Desc: "target type: objective | key_result", Required: true, Enum: []string{"objective", "key_result"}},
		{Name: "progress-percent", Desc: "progress percentage"},
		{Name: "progress-status", Desc: "progress status: normal | overdue | done. must provided with --progress-percent", Enum: []string{"normal", "overdue", "done"}},
		{Name: "source-title", Default: "created by lark-cli", Desc: "source title for display"},
		{Name: "source-url", Desc: "source URL for display (defaults to open platform URL based on brand)"},
		{Name: "user-id-type", Default: "open_id", Desc: "user ID type: open_id | union_id | user_id"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		content := runtime.Str("content")
		if content == "" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--content is required").WithParam("--content")
		}
		if err := common.RejectDangerousCharsTyped("--content", content); err != nil {
			return err
		}
		// Validate content is valid JSON and can be parsed as ContentBlock
		var cb ContentBlock
		if err := json.Unmarshal([]byte(content), &cb); err != nil {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--content must be valid ContentBlock JSON: %s", err).WithParam("--content").WithCause(err)
		}

		targetID := runtime.Str("target-id")
		if targetID == "" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--target-id is required").WithParam("--target-id")
		}
		if err := common.RejectDangerousCharsTyped("--target-id", targetID); err != nil {
			return err
		}
		if id, err := strconv.ParseInt(targetID, 10, 64); err != nil || id <= 0 {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--target-id must be a positive int64").WithParam("--target-id")
		}

		targetType := runtime.Str("target-type")
		if _, ok := targetTypeAllowed[targetType]; !ok {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--target-type must be one of: objective | key_result").WithParam("--target-type")
		}

		if v := runtime.Str("source-title"); v != "" {
			if err := common.RejectDangerousCharsTyped("--source-title", v); err != nil {
				return err
			}
		}
		if v := runtime.Str("source-url"); v != "" {
			if err := common.RejectDangerousCharsTyped("--source-url", v); err != nil {
				return err
			}
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
		p, _ := parseCreateProgressRecordParams(runtime)
		params := map[string]interface{}{
			"user_id_type": p.UserIDType,
		}
		body := map[string]interface{}{
			"content":      p.ContentV1,
			"target_id":    p.TargetID,
			"target_type":  p.TargetType,
			"source_title": p.SourceTitle,
			"source_url":   p.SourceURL,
		}
		if p.ProgressRate != nil {
			body["progress_rate"] = p.ProgressRate
		}
		return common.NewDryRunAPI().
			POST("/open-apis/okr/v1/progress_records/").
			Params(params).
			Body(body).
			Desc(fmt.Sprintf("Create OKR progress for %s", runtime.Str("target-type")))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		p, err := parseCreateProgressRecordParams(runtime)
		if err != nil {
			return err
		}

		body := map[string]interface{}{
			"content":      p.ContentV1,
			"target_id":    p.TargetID,
			"target_type":  p.TargetType,
			"source_title": p.SourceTitle,
			"source_url":   p.SourceURL,
		}
		if p.ProgressRate != nil {
			body["progress_rate"] = p.ProgressRate
		}

		queryParams := map[string]interface{}{"user_id_type": p.UserIDType}

		data, err := runtime.CallAPITyped("POST", "/open-apis/okr/v1/progress_records/", queryParams, body)
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
			fmt.Fprintf(w, "Created Progress [%s]\n", resp.ID)
			fmt.Fprintf(w, "  ModifyTime: %s\n", resp.ModifyTime)
			if resp.ProgressRate != nil && resp.ProgressRate.Percent != nil {
				fmt.Fprintf(w, "  ProgressRate: %.1f%%\n", *resp.ProgressRate.Percent)
			}
			if resp.Content != nil {
				fmt.Fprintf(w, "  Content: %s\n", *resp.Content)
			}
		})
		return nil
	},
}
