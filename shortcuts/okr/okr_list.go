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

// ListOKR lists OKRs for a user (defaults to current logged-in user).
var ListOKR = common.Shortcut{
	Service:     "okr",
	Command:     "+list",
	Description: "list OKRs for a user (defaults to current user)",
	Risk:        "read",
	Scopes:      []string{"okr:okr:readonly"},
	AuthTypes:   []string{"user"},
	HasFormat:   true,

	Flags: []common.Flag{
		{Name: "user-id", Desc: "user open_id (defaults to current logged-in user)"},
		{Name: "period-id", Desc: "OKR period ID (if omitted, auto-detects current period)"},
		{Name: "lang", Desc: "language: zh_cn or en_us (default zh_cn)", Default: "zh_cn", Enum: []string{"zh_cn", "en_us"}},
		{Name: "offset", Type: "int", Default: "0", Desc: "pagination offset"},
		{Name: "limit", Type: "int", Default: "10", Desc: "page size (default 10, max 10)"},
	},

	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		userID := runtime.Str("user-id")
		if userID == "" {
			userID = runtime.Config.UserOpenId
		}
		params := map[string]interface{}{
			"user_id_type": "open_id",
			"offset":       runtime.Int("offset"),
			"limit":        runtime.Int("limit"),
			"lang":         runtime.Str("lang"),
		}
		if pid := runtime.Str("period-id"); pid != "" {
			params["period_ids"] = pid
		}
		return common.NewDryRunAPI().
			GET("/open-apis/okr/v1/users/" + userID + "/okrs").
			Params(params)
	},

	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		userID := runtime.Str("user-id")
		if userID == "" && runtime.Config.UserOpenId == "" {
			return fmt.Errorf("--user-id is required (or login first with: lark-cli auth login --domain okr)")
		}
		limit := runtime.Int("limit")
		if limit < 1 {
			return fmt.Errorf("--limit must be at least 1")
		}
		if limit > 10 {
			return fmt.Errorf("--limit cannot exceed 10 (API maximum)")
		}
		return nil
	},

	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		userID := runtime.Str("user-id")
		if userID == "" {
			userID = runtime.Config.UserOpenId
		}

		periodID := runtime.Str("period-id")

		// Auto-detect current period if not specified
		if periodID == "" {
			pid, err := detectCurrentPeriod(runtime)
			if err != nil {
				return err
			}
			periodID = pid
		}

		queryParams := make(larkcore.QueryParams)
		queryParams.Set("user_id_type", "open_id")
		queryParams.Set("offset", fmt.Sprintf("%d", runtime.Int("offset")))
		queryParams.Set("limit", fmt.Sprintf("%d", runtime.Int("limit")))
		queryParams.Set("lang", runtime.Str("lang"))
		if periodID != "" {
			queryParams.Set("period_ids", periodID)
		}

		apiResp, err := runtime.DoAPI(&larkcore.ApiReq{
			HttpMethod:  http.MethodGet,
			ApiPath:     "/open-apis/okr/v1/users/" + userID + "/okrs",
			QueryParams: queryParams,
		})

		var result map[string]interface{}
		if err == nil {
			if parseErr := json.Unmarshal(apiResp.RawBody, &result); parseErr != nil {
				return WrapOkrError(ErrCodeOkrInternalError, fmt.Sprintf("failed to parse response: %v", parseErr), "parse OKR list response")
			}
		}

		data, err := HandleOkrApiResult(result, err, "list OKRs")
		if err != nil {
			return err
		}

		okrList, _ := data["okr_list"].([]interface{})
		total, _ := data["total"]

		outData := map[string]interface{}{
			"okr_list": okrList,
			"total":    total,
		}

		runtime.OutFormat(outData, nil, func(w io.Writer) {
			if len(okrList) == 0 {
				fmt.Fprintln(w, "No OKRs found for this period.")
				return
			}

			for i, item := range okrList {
				okr, ok := item.(map[string]interface{})
				if !ok {
					continue
				}
				okrID, _ := okr["id"].(string)
				name, _ := okr["name"].(string)

				fmt.Fprintf(w, "[%d] OKR: %s\n", i+1, name)
				fmt.Fprintf(w, "    OKR ID: %s\n", okrID)

				objectives, _ := okr["objective_list"].([]interface{})
				for j, obj := range objectives {
					objective, ok := obj.(map[string]interface{})
					if !ok {
						continue
					}
					content, _ := objective["content"].(string)
					objID, _ := objective["id"].(string)

					progressStr := ""
					if pr, ok := objective["progress_rate"].(map[string]interface{}); ok {
						progressStr = " " + formatProgressPercent(pr)
					}

					fmt.Fprintf(w, "\n    O%d: %s%s\n", j+1, content, progressStr)
					fmt.Fprintf(w, "        Objective ID: %s\n", objID)

					krList, _ := objective["kr_list"].([]interface{})
					for k, kr := range krList {
						keyResult, ok := kr.(map[string]interface{})
						if !ok {
							continue
						}
						krContent, _ := keyResult["content"].(string)
						krID, _ := keyResult["id"].(string)

						krProgressStr := ""
						if pr, ok := keyResult["progress_rate"].(map[string]interface{}); ok {
							krProgressStr = " " + formatProgressPercent(pr)
						}

						fmt.Fprintf(w, "        KR%d: %s%s\n", k+1, krContent, krProgressStr)
						fmt.Fprintf(w, "            KR ID: %s\n", krID)
					}
				}
				fmt.Fprintln(w)
			}
		})

		return nil
	},
}

// detectCurrentPeriod fetches periods and returns the current active period ID.
func detectCurrentPeriod(runtime *common.RuntimeContext) (string, error) {
	queryParams := make(larkcore.QueryParams)
	queryParams.Set("page_size", "20")

	apiResp, err := runtime.DoAPI(&larkcore.ApiReq{
		HttpMethod:  http.MethodGet,
		ApiPath:     "/open-apis/okr/v1/periods",
		QueryParams: queryParams,
	})

	var result map[string]interface{}
	if err == nil {
		if parseErr := json.Unmarshal(apiResp.RawBody, &result); parseErr != nil {
			return "", WrapOkrError(ErrCodeOkrInternalError, fmt.Sprintf("failed to parse periods response: %v", parseErr), "detect current period")
		}
	}

	data, err := HandleOkrApiResult(result, err, "list periods for auto-detection")
	if err != nil {
		return "", err
	}

	items, _ := data["items"].([]interface{})
	current := findCurrentPeriod(items)
	if current == nil {
		return "", nil
	}

	id, _ := current["id"].(string)
	return id, nil
}
