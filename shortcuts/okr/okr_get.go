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

// GetOKR batch-gets OKR details by ID(s).
var GetOKR = common.Shortcut{
	Service:     "okr",
	Command:     "+get",
	Description: "get OKR details by ID(s)",
	Risk:        "read",
	Scopes:      []string{"okr:okr:readonly"},
	AuthTypes:   []string{"user"},
	HasFormat:   true,

	Flags: []common.Flag{
		{Name: "okr-ids", Desc: "comma-separated OKR IDs (max 10)", Required: true},
		{Name: "lang", Desc: "language: zh_cn or en_us (default zh_cn)", Default: "zh_cn", Enum: []string{"zh_cn", "en_us"}},
	},

	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		ids := splitIDs(runtime.Str("okr-ids"))
		if len(ids) == 0 {
			return fmt.Errorf("--okr-ids is required")
		}
		if len(ids) > 10 {
			return fmt.Errorf("--okr-ids cannot contain more than 10 IDs (got %d)", len(ids))
		}
		return nil
	},

	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		ids := splitIDs(runtime.Str("okr-ids"))
		params := map[string]interface{}{
			"okr_ids":      ids,
			"user_id_type": "open_id",
			"lang":         runtime.Str("lang"),
		}
		return common.NewDryRunAPI().
			GET("/open-apis/okr/v1/okrs/batch_get").
			Params(params)
	},

	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		ids := splitIDs(runtime.Str("okr-ids"))

		queryParams := make(larkcore.QueryParams)
		queryParams.Set("user_id_type", "open_id")
		queryParams.Set("lang", runtime.Str("lang"))
		for _, id := range ids {
			queryParams.Add("okr_ids", id)
		}

		apiResp, err := runtime.DoAPI(&larkcore.ApiReq{
			HttpMethod:  http.MethodGet,
			ApiPath:     "/open-apis/okr/v1/okrs/batch_get",
			QueryParams: queryParams,
		})

		var result map[string]interface{}
		if err == nil {
			if parseErr := json.Unmarshal(apiResp.RawBody, &result); parseErr != nil {
				return WrapOkrError(ErrCodeOkrInternalError, fmt.Sprintf("failed to parse response: %v", parseErr), "parse OKR batch get response")
			}
		}

		data, err := HandleOkrApiResult(result, err, "batch get OKRs")
		if err != nil {
			return err
		}

		okrList, _ := data["okr_list"].([]interface{})

		outData := map[string]interface{}{
			"okr_list": okrList,
		}

		runtime.OutFormat(outData, nil, func(w io.Writer) {
			if len(okrList) == 0 {
				fmt.Fprintln(w, "No OKRs found for the given IDs.")
				return
			}

			for i, item := range okrList {
				okr, ok := item.(map[string]interface{})
				if !ok {
					continue
				}
				okrID, _ := okr["id"].(string)
				name, _ := okr["name"].(string)
				periodID, _ := okr["period_id"].(string)

				fmt.Fprintf(w, "[%d] OKR: %s\n", i+1, name)
				fmt.Fprintf(w, "    OKR ID: %s\n", okrID)
				fmt.Fprintf(w, "    Period ID: %s\n", periodID)

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
