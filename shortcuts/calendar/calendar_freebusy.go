// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package calendar

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
)

// parseFreebusyTimeRange parses --start/--end into RFC3339.
func parseFreebusyTimeRange(runtime *common.RuntimeContext) (string, string, error) {
	startInput, endInput := resolveStartEnd(runtime)

	startTs, err := common.ParseTime(startInput)
	if err != nil {
		return "", "", output.ErrValidation("--start: %v", err)
	}
	endTs, err := common.ParseTime(endInput, "end")
	if err != nil {
		return "", "", output.ErrValidation("--end: %v", err)
	}

	startSec, err := strconv.ParseInt(startTs, 10, 64)
	if err != nil {
		return "", "", output.ErrValidation("invalid start timestamp: %v", err)
	}
	endSec, err := strconv.ParseInt(endTs, 10, 64)
	if err != nil {
		return "", "", output.ErrValidation("invalid end timestamp: %v", err)
	}

	timeMin := time.Unix(startSec, 0).Format(time.RFC3339)
	timeMax := time.Unix(endSec, 0).Format(time.RFC3339)
	return timeMin, timeMax, nil
}

var CalendarFreebusy = common.Shortcut{
	Service:     "calendar",
	Command:     "+freebusy",
	Description: "Query user free/busy and RSVP status",
	Risk:        "read",
	Scopes:      []string{"calendar:calendar.free_busy:read"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "start", Desc: "start time (ISO 8601, default: today)"},
		{Name: "end", Desc: "end time (ISO 8601, default: end of start day)"},
		{Name: "user-id", Desc: "target user open_id (ou_ prefix, default: current user)"},
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		userId := runtime.Str("user-id")
		if userId == "" {
			userId = runtime.UserOpenId()
		}
		timeMin, timeMax, err := parseFreebusyTimeRange(runtime)
		if err != nil {
			return common.NewDryRunAPI().Set("error", err.Error())
		}
		return common.NewDryRunAPI().
			POST("/open-apis/calendar/v4/freebusy/list").
			Body(map[string]interface{}{"time_min": timeMin, "time_max": timeMax, "user_id": userId, "need_rsvp_status": true})
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if err := rejectCalendarAutoBotFallback(runtime); err != nil {
			return err
		}
		userId := runtime.Str("user-id")
		if userId == "" && runtime.IsBot() {
			return common.FlagErrorf("--user-id is required for bot identity")
		}
		if userId == "" && runtime.UserOpenId() == "" {
			return common.FlagErrorf("cannot determine user ID, specify --user-id or ensure you are logged in")
		}
		if userId != "" {
			if _, err := common.ValidateUserID(userId); err != nil {
				return err
			}
		}
		return nil
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		userId := runtime.Str("user-id")
		if userId == "" {
			userId = runtime.UserOpenId()
		}

		timeMin, timeMax, err := parseFreebusyTimeRange(runtime)
		if err != nil {
			return output.ErrValidation("--start/--end: %v", err)
		}

		data, err := runtime.CallAPI("POST", "/open-apis/calendar/v4/freebusy/list", nil, map[string]interface{}{
			"time_min":         timeMin,
			"time_max":         timeMax,
			"user_id":          userId,
			"need_rsvp_status": true,
		})
		err = wrapPredefinedError(err)
		if err != nil {
			return err
		}
		items, _ := data["freebusy_list"].([]interface{})

		runtime.OutFormat(items, &output.Meta{Count: len(items)}, func(w io.Writer) {
			if len(items) == 0 {
				fmt.Fprintln(w, "No busy periods in this time range.")
				return
			}

			var rows []map[string]interface{}
			for _, item := range items {
				m, ok := item.(map[string]interface{})
				if !ok {
					continue
				}
				rows = append(rows, map[string]interface{}{
					"start": m["start_time"],
					"end":   m["end_time"],
				})
			}
			output.PrintTable(w, rows)
			fmt.Fprintf(w, "\n%d busy period(s) total\n", len(items))
		})
		return nil
	},
}
