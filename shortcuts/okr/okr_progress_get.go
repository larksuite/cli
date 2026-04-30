// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package okr

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"

	"github.com/larksuite/cli/shortcuts/common"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
)

// OKRGetProgressRecord gets a progress by ID.
var OKRGetProgressRecord = common.Shortcut{
	Service:     "okr",
	Command:     "+progress-get",
	Description: "Get an OKR progress by ID",
	Risk:        "read",
	Scopes:      []string{"okr:okr.progress:readonly"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "progress-id", Desc: "progress ID (int64)", Required: true},
		{Name: "user-id-type", Default: "open_id", Desc: "user ID type: open_id | union_id | user_id"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		progressID := runtime.Str("progress-id")
		if progressID == "" {
			return common.FlagErrorf("--progress-id is required")
		}
		if id, err := strconv.ParseInt(progressID, 10, 64); err != nil || id <= 0 {
			return common.FlagErrorf("--progress-id must be a positive int64")
		}
		idType := runtime.Str("user-id-type")
		if idType != "open_id" && idType != "union_id" && idType != "user_id" {
			return common.FlagErrorf("--user-id-type must be one of: open_id | union_id | user_id")
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		progressID := runtime.Str("progress-id")
		params := map[string]interface{}{
			"user_id_type": runtime.Str("user-id-type"),
		}
		return common.NewDryRunAPI().
			GET("/open-apis/okr/v1/progress_records/:progress_id").
			Params(params).
			Set("progress_id", progressID).
			Desc("Get OKR progress")
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		progressID := runtime.Str("progress-id")
		userIDType := runtime.Str("user-id-type")

		queryParams := make(larkcore.QueryParams)
		queryParams.Set("user_id_type", userIDType)

		path := fmt.Sprintf("/open-apis/okr/v1/progress_records/%s", progressID)
		data, err := runtime.DoAPIJSON("GET", path, queryParams, nil)
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
			fmt.Fprintf(w, "Progress [%s]\n", resp.ID)
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

// parseProgressRecord parses a single progress from API response data.
func parseProgressRecord(data map[string]any) (*ProgressV1, error) {
	raw, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	var record ProgressV1
	if err := json.Unmarshal(raw, &record); err != nil {
		return nil, err
	}
	return &record, nil
}
