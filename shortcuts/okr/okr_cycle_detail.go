// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package okr

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/larksuite/cli/shortcuts/common"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
)

// OKRCycleDetail lists all objectives and their key results under a given OKR cycle.
var OKRCycleDetail = common.Shortcut{
	Service:     "okr",
	Command:     "+cycle-detail",
	Description: "List objectives and key results under an OKR cycle",
	Risk:        "read",
	Scopes:      []string{"okr:okr.content:readonly"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "cycle-id", Desc: "OKR cycle id (int64)", Required: true},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		cycleID := runtime.Str("cycle-id")
		if cycleID == "" {
			return common.FlagErrorf("--cycle-id is required")
		}
		if id, err := strconv.ParseInt(cycleID, 10, 64); err != nil || id <= 0 {
			return common.FlagErrorf("--cycle-id must be a positive int64")
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		cycleID := runtime.Str("cycle-id")
		params := map[string]interface{}{
			"page_size": 100,
		}
		return common.NewDryRunAPI().
			GET("/open-apis/okr/v2/cycles/:cycle_id/objectives").
			Params(params).
			Set("cycle_id", cycleID).
			Desc("Auto-paginates objectives in the cycle, then calls GET /open-apis/okr/v2/objectives/:objective_id/key_results for each objective to fetch key results")
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		cycleID := runtime.Str("cycle-id")

		// Paginate objectives under the cycle.
		queryParams := make(larkcore.QueryParams)
		queryParams.Set("page_size", "100")

		var objectives []Objective
		page := 0
		for {
			if err := ctx.Err(); err != nil {
				return err
			}
			if page > 0 {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(500 * time.Millisecond):
				}
			}
			page++

			path := fmt.Sprintf("/open-apis/okr/v2/cycles/%s/objectives", cycleID)
			data, err := runtime.DoAPIJSON("GET", path, queryParams, nil)
			if err != nil {
				return err
			}

			itemsRaw, _ := data["items"].([]interface{})
			for _, item := range itemsRaw {
				raw, err := json.Marshal(item)
				if err != nil {
					continue
				}
				var obj Objective
				if err := json.Unmarshal(raw, &obj); err != nil {
					continue
				}
				objectives = append(objectives, obj)
			}

			hasMore, pageToken := common.PaginationMeta(data)
			if !hasMore || pageToken == "" {
				break
			}
			queryParams.Set("page_token", pageToken)
		}

		// For each objective, paginate key results and convert to response format.
		respObjectives := make([]*RespObjective, 0, len(objectives))
		for i := range objectives {
			if err := ctx.Err(); err != nil {
				return err
			}
			obj := &objectives[i]

			krQuery := make(larkcore.QueryParams)
			krQuery.Set("page_size", "100")

			var keyResults []KeyResult
			krPage := 0
			for {
				if err := ctx.Err(); err != nil {
					return err
				}
				if krPage > 0 {
					select {
					case <-ctx.Done():
						return ctx.Err()
					case <-time.After(500 * time.Millisecond):
					}
				}
				krPage++

				path := fmt.Sprintf("/open-apis/okr/v2/objectives/%s/key_results", obj.ID)
				data, err := runtime.DoAPIJSON("GET", path, krQuery, nil)
				if err != nil {
					return err
				}

				itemsRaw, _ := data["items"].([]interface{})
				for _, item := range itemsRaw {
					raw, err := json.Marshal(item)
					if err != nil {
						continue
					}
					var kr KeyResult
					if err := json.Unmarshal(raw, &kr); err != nil {
						continue
					}
					keyResults = append(keyResults, kr)
				}

				hasMore, pageToken := common.PaginationMeta(data)
				if !hasMore || pageToken == "" {
					break
				}
				krQuery.Set("page_token", pageToken)
			}

			respObj := obj.ToResp()
			if respObj == nil {
				continue
			}
			respKRs := make([]RespKeyResult, 0, len(keyResults))
			for j := range keyResults {
				if r := keyResults[j].ToResp(); r != nil {
					respKRs = append(respKRs, *r)
				}
			}
			respObj.KeyResults = respKRs
			respObjectives = append(respObjectives, respObj)
		}

		result := map[string]interface{}{
			"cycle_id":   cycleID,
			"objectives": respObjectives,
			"total":      len(respObjectives),
		}

		runtime.OutFormat(result, nil, func(w io.Writer) {
			fmt.Fprintf(w, "Cycle %s: %d objective(s)\n", cycleID, len(respObjectives))
			for _, o := range respObjectives {
				fmt.Fprintf(w, "Objective [%s]: %s \n Notes: %s \n score=%.2f weight=%.2f\n", o.ID, ptrStr(o.Content), ptrStr(o.Notes), ptrFloat64(o.Score), ptrFloat64(o.Weight))
				for _, kr := range o.KeyResults {
					fmt.Fprintf(w, "  - KR [%s]: %s \n score=%.2f weight=%.2f\n", kr.ID, ptrStr(kr.Content), ptrFloat64(kr.Score), ptrFloat64(kr.Weight))
				}
			}
		})
		return nil
	},
}
