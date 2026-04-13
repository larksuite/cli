// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package okr

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/shortcuts/common"
)

// QueryReview queries OKR reviews for given users and period.
var QueryReview = common.Shortcut{
	Service:     "okr",
	Command:     "+review",
	Description: "query OKR reviews",
	Risk:        "read",
	Scopes:      []string{"okr:okr.review:readonly"},
	AuthTypes:   []string{"user"},
	HasFormat:   true,

	Flags: []common.Flag{
		{Name: "user-ids", Desc: "comma-separated user open_ids (max 5)", Required: true},
		{Name: "period-id", Desc: "OKR period ID", Required: true},
	},

	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		ids := splitIDs(runtime.Str("user-ids"))
		if len(ids) == 0 {
			return fmt.Errorf("--user-ids is required")
		}
		if len(ids) > 5 {
			return fmt.Errorf("--user-ids cannot contain more than 5 IDs (got %d)", len(ids))
		}
		return nil
	},

	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		params := map[string]interface{}{
			"user_ids":     splitIDs(runtime.Str("user-ids")),
			"period_ids":   runtime.Str("period-id"),
			"user_id_type": "open_id",
		}
		return common.NewDryRunAPI().
			GET("/open-apis/okr/v1/reviews/query").
			Params(params)
	},

	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		userIDs := splitIDs(runtime.Str("user-ids"))
		periodID := runtime.Str("period-id")

		queryParams := make(larkcore.QueryParams)
		queryParams.Set("user_id_type", "open_id")
		for _, uid := range userIDs {
			queryParams.Add("user_ids", uid)
		}
		queryParams.Add("period_ids", periodID)

		apiResp, err := runtime.DoAPI(&larkcore.ApiReq{
			HttpMethod:  http.MethodGet,
			ApiPath:     "/open-apis/okr/v1/reviews/query",
			QueryParams: queryParams,
		})

		var result map[string]interface{}
		if err == nil {
			if parseErr := json.Unmarshal(apiResp.RawBody, &result); parseErr != nil {
				return WrapOkrError(ErrCodeOkrInternalError, fmt.Sprintf("failed to parse response: %v", parseErr), "parse review response")
			}
		}

		data, err := HandleOkrApiResult(result, err, "query OKR reviews")
		if err != nil {
			return err
		}

		reviewList, _ := data["review_list"].([]interface{})

		outData := map[string]interface{}{
			"review_list": reviewList,
		}

		runtime.OutFormat(outData, nil, func(w io.Writer) {
			if len(reviewList) == 0 {
				fmt.Fprintln(w, "No OKR reviews found.")
				return
			}

			for i, item := range reviewList {
				review, ok := item.(map[string]interface{})
				if !ok {
					continue
				}

				userObj, _ := review["user_id"].(map[string]interface{})
				openID, _ := userObj["open_id"].(string)

				fmt.Fprintf(w, "[%d] User: %s\n", i+1, openID)

				periodList, _ := review["review_period_list"].([]interface{})
				for _, pItem := range periodList {
					rp, ok := pItem.(map[string]interface{})
					if !ok {
						continue
					}
					rpPeriodID, _ := rp["period_id"].(string)
					fmt.Fprintf(w, "    Period: %s\n", rpPeriodID)

					cycleReviews, _ := rp["cycle_review_list"].([]interface{})
					for _, cr := range cycleReviews {
						crMap, ok := cr.(map[string]interface{})
						if !ok {
							continue
						}
						url, _ := crMap["url"].(string)
						createTime, _ := crMap["create_time"].(string)
						fmt.Fprintf(w, "    Cycle Review: %s (created: %s)\n", url, formatTimestampMs(createTime))
					}

					progressReports, _ := rp["progress_report_list"].([]interface{})
					for _, pr := range progressReports {
						prMap, ok := pr.(map[string]interface{})
						if !ok {
							continue
						}
						url, _ := prMap["url"].(string)
						createTime, _ := prMap["create_time"].(string)
						fmt.Fprintf(w, "    Progress Report: %s (created: %s)\n", url, formatTimestampMs(createTime))
					}
				}
				fmt.Fprintln(w)
			}
		})

		return nil
	},
}
