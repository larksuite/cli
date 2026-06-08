// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package okr

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
)

// OKRListProgress lists progress for an objective or key result.
var OKRListProgress = common.Shortcut{
	Service:     "okr",
	Command:     "+progress-list",
	Description: "List progress for an objective or key result",
	Risk:        "read",
	Scopes:      []string{"okr:okr.progress:readonly"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "target-id", Desc: "target ID (objective or key result ID)", Required: true},
		{Name: "target-type", Desc: "target type: objective | key_result", Required: true, Enum: []string{"objective", "key_result"}},
		{Name: "user-id-type", Default: "open_id", Desc: "user ID type: open_id | union_id | user_id"},
		{Name: "department-id-type", Default: "open_department_id", Desc: "department ID type: department_id | open_department_id"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
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

		idType := runtime.Str("user-id-type")
		if idType != "open_id" && idType != "union_id" && idType != "user_id" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--user-id-type must be one of: open_id | union_id | user_id").WithParam("--user-id-type")
		}

		deptIDType := runtime.Str("department-id-type")
		if deptIDType != "department_id" && deptIDType != "open_department_id" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--department-id-type must be one of: department_id | open_department_id").WithParam("--department-id-type")
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		targetID := runtime.Str("target-id")
		targetType := runtime.Str("target-type")
		params := map[string]interface{}{
			"user_id_type":       runtime.Str("user-id-type"),
			"department_id_type": runtime.Str("department-id-type"),
			"page_size":          100,
		}

		switch targetType {
		case "objective":
			return common.NewDryRunAPI().
				GET("/open-apis/okr/v2/objectives/:objective_id/progresses").
				Params(params).
				Set("objective_id", targetID).
				Desc("List progresses for objective")
		case "key_result":
			return common.NewDryRunAPI().
				GET("/open-apis/okr/v2/key_results/:key_result_id/progresses").
				Params(params).
				Set("key_result_id", targetID).
				Desc("List progresses for key result")
		}
		return nil
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		targetID := runtime.Str("target-id")
		targetType := runtime.Str("target-type")
		userIDType := runtime.Str("user-id-type")
		deptIDType := runtime.Str("department-id-type")

		queryParams := map[string]interface{}{
			"user_id_type":       userIDType,
			"department_id_type": deptIDType,
			"page_size":          "100",
		}

		var apiPath string
		switch targetType {
		case "objective":
			apiPath = fmt.Sprintf("/open-apis/okr/v2/objectives/%s/progresses", targetID)
		case "key_result":
			apiPath = fmt.Sprintf("/open-apis/okr/v2/key_results/%s/progresses", targetID)
		}

		var allProgress []*Progress
		for {
			if err := ctx.Err(); err != nil {
				return err
			}

			data, err := runtime.CallAPITyped("GET", apiPath, queryParams, nil)
			if err != nil {
				return err
			}

			itemsRaw, _ := data["items"].([]interface{})
			for _, item := range itemsRaw {
				raw, err := json.Marshal(item)
				if err != nil {
					continue
				}
				var progress Progress
				if err := json.Unmarshal(raw, &progress); err != nil {
					continue
				}
				allProgress = append(allProgress, &progress)
			}

			hasMore, pageToken := common.PaginationMeta(data)
			if !hasMore || pageToken == "" {
				break
			}
			queryParams["page_token"] = pageToken
		}

		// Convert to response format
		respProgress := make([]*RespProgress, 0, len(allProgress))
		for _, p := range allProgress {
			respProgress = append(respProgress, p.ToResp())
		}

		runtime.OutFormat(map[string]interface{}{
			"progress_list": respProgress,
			"total":         len(respProgress),
		}, nil, func(w io.Writer) {
			fmt.Fprintf(w, "Found %d progress(es)\n", len(respProgress))
			for _, p := range respProgress {
				fmt.Fprintf(w, "  [%s] , %s", p.ID, p.ModifyTime)
				if p.ProgressRate != nil && p.ProgressRate.Percent != nil {
					fmt.Fprintf(w, " (%.2f%%", *p.ProgressRate.Percent)
					if p.ProgressRate.Status != nil {
						fmt.Fprintf(w, ", %s", *p.ProgressRate.Status)
					}
					fmt.Fprintf(w, ")\n")
					if p.Content != nil {
						fmt.Fprintf(w, "  Content: %s\n", *p.Content)
					}
				}
				fmt.Fprintln(w)
			}
		})
		return nil
	},
}
