// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package okr

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
)

// parseTimeRange parses a "YYYY-MM--YYYY-MM" string into two time.Time values.
// The start is the first moment of the start month; the end is the last moment of the end month.
func parseTimeRange(s string) (start, end time.Time, err error) {
	parts := strings.SplitN(s, "--", 2)
	if len(parts) != 2 {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid time-range format %q, expected YYYY-MM--YYYY-MM", s)
	}
	start, err = time.Parse("2006-01", strings.TrimSpace(parts[0]))
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid start month %q: %w", parts[0], err)
	}
	end, err = time.Parse("2006-01", strings.TrimSpace(parts[1]))
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid end month %q: %w", parts[1], err)
	}
	// end is the last moment of the end month
	end = end.AddDate(0, 1, 0).Add(-time.Millisecond)
	if start.After(end) {
		return time.Time{}, time.Time{}, fmt.Errorf("start month %s is after end month %s", parts[0], parts[1])
	}
	return start, end, nil
}

// cycleOverlaps checks whether a cycle's [startMs, endMs] overlaps with [rangeStart, rangeEnd].
func cycleOverlaps(cycle *Cycle, rangeStart, rangeEnd time.Time) bool {
	startMs, err1 := strconv.ParseInt(cycle.StartTime, 10, 64)
	endMs, err2 := strconv.ParseInt(cycle.EndTime, 10, 64)
	if err1 != nil || err2 != nil {
		return false
	}
	cycleStart := time.UnixMilli(startMs)
	cycleEnd := time.UnixMilli(endMs)
	// Two ranges overlap iff one starts before the other ends
	return !cycleStart.After(rangeEnd) && !cycleEnd.Before(rangeStart)
}

var OKRListCycles = common.Shortcut{
	Service:     "okr",
	Command:     "+cycle-list",
	Description: "List okr cycles of a certain user",
	Risk:        "read",
	Scopes:      []string{"okr:okr.period:readonly"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "user-id", Desc: "user ID", Required: true},
		{Name: "user-id-type", Default: "open_id", Desc: "user ID type: open_id | union_id | user_id"},
		{Name: "time-range", Desc: "specify time range. Use Format as YYYY-MM--YYYY-MM. leave empty to fetch all user cycles."},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		idType := runtime.Str("user-id-type")
		if idType != "open_id" && idType != "union_id" && idType != "user_id" {
			return common.FlagErrorf("--user-id-type must be one of: open_id | union_id | user_id")
		}
		userID := runtime.Str("user-id")
		if err := validate.RejectControlChars(userID, "user-id"); err != nil {
			return err
		}

		tr := runtime.Str("time-range")
		if tr != "" {
			if err := validate.RejectControlChars(tr, "time-range"); err != nil {
				return err
			}
			if _, _, err := parseTimeRange(tr); err != nil {
				return common.FlagErrorf("--time-range: %s", err)
			}
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		params := map[string]interface{}{
			"user_id":      runtime.Str("user-id"),
			"user_id_type": runtime.Str("user-id-type"),
			"page_size":    100,
		}
		return common.NewDryRunAPI().
			GET("/open-apis/okr/v2/cycles").
			Params(params).
			Desc("List OKR cycles for user, paginated at 100 per page, filtered by time-range")
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		userID := runtime.Str("user-id")
		userIDType := runtime.Str("user-id-type")
		timeRange := runtime.Str("time-range")

		// Parse time range for filtering
		var rangeStart, rangeEnd time.Time
		var hasRange bool
		if timeRange != "" {
			var err error
			rangeStart, rangeEnd, err = parseTimeRange(timeRange)
			if err != nil {
				return common.FlagErrorf("--time-range: %s", err)
			}
			hasRange = true
		}

		// Paginated fetch of all cycles
		queryParams := make(larkcore.QueryParams)
		queryParams.Set("user_id", userID)
		queryParams.Set("user_id_type", userIDType)
		queryParams.Set("page_size", "100")

		var allCycles []Cycle
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

			data, err := runtime.DoAPIJSON("GET", "/open-apis/okr/v2/cycles", queryParams, nil)
			if err != nil {
				return err
			}

			itemsRaw, _ := data["items"].([]interface{})
			for _, item := range itemsRaw {
				raw, err := json.Marshal(item)
				if err != nil {
					continue
				}
				var cycle Cycle
				if err := json.Unmarshal(raw, &cycle); err != nil {
					continue
				}
				allCycles = append(allCycles, cycle)
			}

			hasMore, pageToken := common.PaginationMeta(data)
			if !hasMore || pageToken == "" {
				break
			}
			queryParams.Set("page_token", pageToken)
		}

		// Filter by time-range overlap
		var filtered []Cycle
		for i := range allCycles {
			if !hasRange || cycleOverlaps(&allCycles[i], rangeStart, rangeEnd) {
				filtered = append(filtered, allCycles[i])
			}
		}

		// Convert to response format
		respCycles := make([]*RespCycle, 0, len(filtered))
		for i := range filtered {
			respCycles = append(respCycles, filtered[i].ToResp())
		}

		runtime.OutFormat(map[string]interface{}{
			"cycles": respCycles,
			"total":  len(respCycles),
		}, nil, func(w io.Writer) {
			fmt.Fprintf(w, "Found %d cycle(s)\n", len(respCycles))
			for _, c := range respCycles {
				fmt.Fprintf(w, "  [%s] %s ~ %s (status: %s)\n", c.ID, c.StartTime, c.EndTime, ptrStr(c.CycleStatus))
			}
		})
		return nil
	},
}
