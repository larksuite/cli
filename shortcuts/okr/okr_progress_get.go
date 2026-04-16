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

// GetProgress gets a progress record by ID.
var GetProgress = common.Shortcut{
	Service:     "okr",
	Command:     "+progress-get",
	Description: "get a progress record by ID",
	Risk:        "read",
	Scopes:      []string{"okr:okr.progress:readonly"},
	AuthTypes:   []string{"user"},
	HasFormat:   true,

	Flags: []common.Flag{
		{Name: "progress-id", Desc: "progress record ID", Required: true},
	},

	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		progressID := runtime.Str("progress-id")
		return common.NewDryRunAPI().
			GET("/open-apis/okr/v1/progress_records/" + progressID).
			Params(map[string]interface{}{"user_id_type": "open_id"})
	},

	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		progressID := runtime.Str("progress-id")

		queryParams := make(larkcore.QueryParams)
		queryParams.Set("user_id_type", "open_id")

		apiResp, err := runtime.DoAPI(&larkcore.ApiReq{
			HttpMethod:  http.MethodGet,
			ApiPath:     "/open-apis/okr/v1/progress_records/" + progressID,
			QueryParams: queryParams,
		})

		var result map[string]interface{}
		if err == nil {
			if parseErr := json.Unmarshal(apiResp.RawBody, &result); parseErr != nil {
				return WrapOkrError(ErrCodeOkrInternalError, fmt.Sprintf("failed to parse response: %v", parseErr), "parse progress response")
			}
		}

		data, err := HandleOkrApiResult(result, err, "get progress record")
		if err != nil {
			return err
		}

		runtime.OutFormat(data, nil, func(w io.Writer) {
			pID, _ := data["progress_id"].(string)
			modifyTime, _ := data["modify_time"].(string)
			targetID, _ := data["target_id"].(string)
			targetType, _ := data["target_type"].(string)

			targetTypeLabel := "unknown"
			switch targetType {
			case "2":
				targetTypeLabel = "objective"
			case "3":
				targetTypeLabel = "key_result"
			}

			fmt.Fprintf(w, "Progress ID: %s\n", pID)
			fmt.Fprintf(w, "Target: %s (%s)\n", targetID, targetTypeLabel)
			if modifyTime != "" {
				fmt.Fprintf(w, "Modified: %s\n", formatTimestampMs(modifyTime))
			}

			// Extract text from rich text content
			if content, ok := data["content"].(map[string]interface{}); ok {
				blocks, _ := content["blocks"].([]interface{})
				for _, block := range blocks {
					b, ok := block.(map[string]interface{})
					if !ok {
						continue
					}
					if para, ok := b["paragraph"].(map[string]interface{}); ok {
						elements, _ := para["elements"].([]interface{})
						for _, elem := range elements {
							e, ok := elem.(map[string]interface{})
							if !ok {
								continue
							}
							if textRun, ok := e["textRun"].(map[string]interface{}); ok {
								text, _ := textRun["text"].(string)
								if text != "" {
									fmt.Fprintf(w, "Content: %s\n", text)
								}
							}
						}
					}
				}
			}
		})

		return nil
	},
}
