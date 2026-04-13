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

// ListPeriods lists OKR periods.
var ListPeriods = common.Shortcut{
	Service:     "okr",
	Command:     "+periods",
	Description: "list OKR periods",
	Risk:        "read",
	Scopes:      []string{"okr:okr.period:readonly"},
	AuthTypes:   []string{"user"},
	HasFormat:   true,

	Flags: []common.Flag{
		{Name: "page-token", Desc: "pagination token"},
		{Name: "page-size", Type: "int", Default: "10", Desc: "page size (default 10)"},
	},

	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		params := map[string]interface{}{
			"page_size": runtime.Int("page-size"),
		}
		if pt := runtime.Str("page-token"); pt != "" {
			params["page_token"] = pt
		}
		return common.NewDryRunAPI().
			GET("/open-apis/okr/v1/periods").
			Params(params)
	},

	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		queryParams := make(larkcore.QueryParams)
		queryParams.Set("page_size", fmt.Sprintf("%d", runtime.Int("page-size")))
		if pt := runtime.Str("page-token"); pt != "" {
			queryParams.Set("page_token", pt)
		}

		apiResp, err := runtime.DoAPI(&larkcore.ApiReq{
			HttpMethod:  http.MethodGet,
			ApiPath:     "/open-apis/okr/v1/periods",
			QueryParams: queryParams,
		})

		var result map[string]interface{}
		if err == nil {
			if parseErr := json.Unmarshal(apiResp.RawBody, &result); parseErr != nil {
				return WrapOkrError(ErrCodeOkrInternalError, fmt.Sprintf("failed to parse response: %v", parseErr), "parse periods response")
			}
		}

		data, err := HandleOkrApiResult(result, err, "list periods")
		if err != nil {
			return err
		}

		items, _ := data["items"].([]interface{})
		pageToken, _ := data["page_token"].(string)
		hasMore, _ := data["has_more"].(bool)

		outData := map[string]interface{}{
			"items":      items,
			"page_token": pageToken,
			"has_more":   hasMore,
		}

		runtime.OutFormat(outData, nil, func(w io.Writer) {
			if len(items) == 0 {
				fmt.Fprintln(w, "No OKR periods found.")
				return
			}

			for i, item := range items {
				period, ok := item.(map[string]interface{})
				if !ok {
					continue
				}
				id, _ := period["id"].(string)
				zhName, _ := period["zh_name"].(string)
				enName, _ := period["en_name"].(string)

				name := zhName
				if name == "" {
					name = enName
				}

				startStr, _ := period["period_start_time"].(string)
				endStr, _ := period["period_end_time"].(string)
				start := formatTimestampMs(startStr)
				end := formatTimestampMs(endStr)

				currentTag := ""
				if findCurrentPeriod([]interface{}{item}) != nil {
					currentTag = " [current]"
				}

				fmt.Fprintf(w, "[%d] %s%s\n", i+1, name, currentTag)
				fmt.Fprintf(w, "    ID: %s\n", id)
				if start != "" && end != "" {
					fmt.Fprintf(w, "    Period: %s ~ %s\n", start, end)
				}
				fmt.Fprintln(w)
			}

			if hasMore && pageToken != "" {
				fmt.Fprintf(w, "More periods available. Use --page-token %s to see next page.\n", pageToken)
			}
		})

		return nil
	},
}
