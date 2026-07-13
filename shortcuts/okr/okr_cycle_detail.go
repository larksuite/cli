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

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
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
		{Name: "style", Default: "simple", Desc: "output style: simple (semi-plain text JSON) | richtext (ContentBlock JSON)", Enum: []string{"simple", "richtext"}},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		cycleID := runtime.Str("cycle-id")
		if cycleID == "" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--cycle-id is required").WithParam("--cycle-id")
		}
		if id, err := strconv.ParseInt(cycleID, 10, 64); err != nil || id <= 0 {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--cycle-id must be a positive int64").WithParam("--cycle-id")
		}
		style := runtime.Str("style")
		if style != "simple" && style != "richtext" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--style must be one of: simple | richtext").WithParam("--style")
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
		style := runtime.Str("style")

		// Paginate objectives under the cycle.
		queryParams := map[string]interface{}{"page_size": "100"}

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
			data, err := runtime.CallAPITyped("GET", path, queryParams, nil)
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
			queryParams["page_token"] = pageToken
		}

		// For each objective, paginate key results and convert to response format.
		if style == "simple" {
			respObjectives := make([]*RespObjectiveSimple, 0, len(objectives))
			for i := range objectives {
				if err := ctx.Err(); err != nil {
					return err
				}
				obj := &objectives[i]

				keyResults, err := fetchKeyResults(ctx, runtime, obj.ID)
				if err != nil {
					return err
				}

				respObj := obj.ToSimple()
				if respObj == nil {
					continue
				}
				respKRs := make([]RespKeyResultSimple, 0, len(keyResults))
				for j := range keyResults {
					if r := keyResults[j].ToSimple(); r != nil {
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
				"style":      style,
			}

			runtime.OutFormat(result, nil, func(w io.Writer) {
				fmt.Fprintf(w, "Cycle %s: %d objective(s) (style: %s)\n", cycleID, len(respObjectives), style)
				for _, o := range respObjectives {
					contentText := ""
					if o.Content != nil {
						contentText = o.Content.Text
					}
					notesText := ""
					if o.Notes != nil {
						notesText = o.Notes.Text
					}
					fmt.Fprintf(w, "Objective [%s]: %s \n Notes: %s \n score=%.2f weight=%.2f\n", o.ID, contentText, notesText, ptrFloat64(o.Score), ptrFloat64(o.Weight))
					for _, kr := range o.KeyResults {
						krText := ""
						if kr.Content != nil {
							krText = kr.Content.Text
						}
						fmt.Fprintf(w, "  - KR [%s]: %s \n score=%.2f weight=%.2f\n", kr.ID, krText, ptrFloat64(kr.Score), ptrFloat64(kr.Weight))
					}
				}
			})
		} else {
			// richtext mode
			respObjectives := make([]*RespObjective, 0, len(objectives))
			for i := range objectives {
				if err := ctx.Err(); err != nil {
					return err
				}
				obj := &objectives[i]

				keyResults, err := fetchKeyResults(ctx, runtime, obj.ID)
				if err != nil {
					return err
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
				"style":      style,
			}

			runtime.OutFormat(result, nil, func(w io.Writer) {
				fmt.Fprintf(w, "Cycle %s: %d objective(s) (style: %s)\n", cycleID, len(respObjectives), style)
				for _, o := range respObjectives {
					fmt.Fprintf(w, "Objective [%s]: %s \n Notes: %s \n score=%.2f weight=%.2f\n", o.ID, ptrStr(o.Content), ptrStr(o.Notes), ptrFloat64(o.Score), ptrFloat64(o.Weight))
					for _, kr := range o.KeyResults {
						fmt.Fprintf(w, "  - KR [%s]: %s \n score=%.2f weight=%.2f\n", kr.ID, ptrStr(kr.Content), ptrFloat64(kr.Score), ptrFloat64(kr.Weight))
					}
				}
			})
		}
		return nil
	},
}
