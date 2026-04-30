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

	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
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
		return nil, common.FlagErrorf("--content must be valid ContentBlock JSON: %s", err)
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
			return nil, common.FlagErrorf("--progress-percent must be a number between -99999999999 and 99999999999")
		}
		progressRate = &ProgressRateV1{Percent: &percent}
		if s := runtime.Str("progress-status"); s != "" {
			status, ok := ParseProgressStatus(s)
			if !ok {
				return nil, common.FlagErrorf("--progress-status must be one of: normal | overdue | done")
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
			return common.FlagErrorf("--content is required")
		}
		if err := validate.RejectControlChars(content, "content"); err != nil {
			return err
		}
		// Validate content is valid JSON and can be parsed as ContentBlock
		var cb ContentBlock
		if err := json.Unmarshal([]byte(content), &cb); err != nil {
			return common.FlagErrorf("--content must be valid ContentBlock JSON: %s", err)
		}

		targetID := runtime.Str("target-id")
		if targetID == "" {
			return common.FlagErrorf("--target-id is required")
		}
		if err := validate.RejectControlChars(targetID, "target-id"); err != nil {
			return err
		}
		if id, err := strconv.ParseInt(targetID, 10, 64); err != nil || id <= 0 {
			return common.FlagErrorf("--target-id must be a positive int64")
		}

		targetType := runtime.Str("target-type")
		if _, ok := targetTypeAllowed[targetType]; !ok {
			return common.FlagErrorf("--target-type must be one of: objective | key_result")
		}

		if v := runtime.Str("source-title"); v != "" {
			if err := validate.RejectControlChars(v, "source-title"); err != nil {
				return err
			}
		}
		if v := runtime.Str("source-url"); v != "" {
			if err := validate.RejectControlChars(v, "source-url"); err != nil {
				return err
			}
		}

		if v := runtime.Str("progress-percent"); v != "" {
			percent, err := strconv.ParseFloat(v, 64)
			if err != nil || math.IsNaN(percent) || math.IsInf(percent, 0) || percent < -99999999999 || percent > 99999999999 {
				return common.FlagErrorf("--progress-percent must be a number between -99999999999 and 99999999999")
			}
		}
		if v := runtime.Str("progress-status"); v != "" {
			if _, ok := ParseProgressStatus(v); !ok {
				return common.FlagErrorf("--progress-status must be one of: normal | overdue | done")
			}
			if v := runtime.Str("progress-percent"); v == "" {
				return common.FlagErrorf("--progress-percent must provided with --progress-status")
			}
		}

		idType := runtime.Str("user-id-type")
		if idType != "open_id" && idType != "union_id" && idType != "user_id" {
			return common.FlagErrorf("--user-id-type must be one of: open_id | union_id | user_id")
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

		queryParams := make(larkcore.QueryParams)
		queryParams.Set("user_id_type", p.UserIDType)

		data, err := runtime.DoAPIJSON("POST", "/open-apis/okr/v1/progress_records/", queryParams, body)
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
