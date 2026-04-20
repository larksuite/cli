// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package calendar

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
)

const (
	suggestionPath = "/open-apis/calendar/v4/freebusy/suggestion"

	flagStart           = "start"
	flagEnd             = "end"
	flagAttendees       = "attendee-ids"
	flagEventRrule      = "event-rrule"
	flagDurationMinutes = "duration-minutes"
	flagTimezone        = "timezone"
	flagExclude         = "exclude"
)

type OpenAPIResponse[T any] struct {
	Code int    `json:"code,omitempty"`
	Msg  string `json:"msg,omitempty"`
	Data T      `json:"data,omitempty"`
}

type SuggestionRequest struct {
	SearchStartTime    string       `json:"search_start_time,omitempty"`
	SearchEndTime      string       `json:"search_end_time,omitempty"`
	Timezone           string       `json:"timezone,omitempty"`
	EventRrule         string       `json:"event_rrule,omitempty"`
	DurationMinutes    int          `json:"duration_minutes,omitempty"`
	AttendeeUserIds    []string     `json:"attendee_user_ids,omitempty"`
	AttendeeChatIds    []string     `json:"attendee_chat_ids,omitempty"`
	ExcludedEventTimes []*EventTime `json:"excluded_event_times,omitempty"`
}

type EventTime struct {
	EventStartTime  string `json:"event_start_time,omitempty"`
	EventEndTime    string `json:"event_end_time,omitempty"`
	RecommendReason string `json:"recommend_reason,omitempty"`
}

type SuggestionResponse struct {
	Suggestions      []*EventTime `json:"suggestions,omitempty"`
	AiActionGuidance string       `json:"ai_action_guidance,omitempty"`
}

func buildSuggestionRequest(runtime *common.RuntimeContext) (*SuggestionRequest, error) {
	req := &SuggestionRequest{}

	// resolve start and end times specifically for suggestion (default to current time to end of today)
	startInput := runtime.Str(flagStart)
	if startInput == "" {
		startInput = time.Now().Format(time.RFC3339)
	}

	timeMin, err := common.ParseTime(startInput)
	if err != nil {
		return nil, output.ErrValidation("invalid --start: %v", err)
	}
	minSec, err := strconv.ParseInt(timeMin, 10, 64)
	if err != nil {
		return nil, output.ErrValidation("invalid start timestamp: %v", err)
	}
	startTime := time.Unix(minSec, 0)

	endInput := runtime.Str(flagEnd)
	if endInput == "" {
		// end of start time's day
		endOfStartDay := time.Date(startTime.Year(), startTime.Month(), startTime.Day(), 23, 59, 59, 0, startTime.Location())
		endInput = endOfStartDay.Format(time.RFC3339)
	}

	timeMax, err := common.ParseTime(endInput, "end")
	if err != nil {
		return nil, output.ErrValidation("invalid --end: %v", err)
	}
	// Convert Unix timestamp string back to RFC3339 since the API requires RFC3339
	maxSec, err := strconv.ParseInt(timeMax, 10, 64)
	if err != nil {
		return nil, output.ErrValidation("invalid end timestamp: %v", err)
	}
	req.SearchStartTime = startTime.Format(time.RFC3339)
	req.SearchEndTime = time.Unix(maxSec, 0).Format(time.RFC3339)

	// Parse combined attendees (auto-split by prefix oc_ for chats)
	attendeesStr := runtime.Str(flagAttendees)
	if attendeesStr != "" {
		parts := strings.Split(attendeesStr, ",")
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			if strings.HasPrefix(p, "oc_") {
				req.AttendeeChatIds = append(req.AttendeeChatIds, p)
			} else {
				req.AttendeeUserIds = append(req.AttendeeUserIds, p)
			}
		}
	}

	// Fallback joining strategy for current user
	if !runtime.IsBot() {
		userOpenId := runtime.UserOpenId()
		found := false
		for _, id := range req.AttendeeUserIds {
			if id == userOpenId {
				found = true
				break
			}
		}
		if !found && userOpenId != "" {
			req.AttendeeUserIds = append(req.AttendeeUserIds, userOpenId)
		}
	}

	eventRrule := runtime.Str(flagEventRrule)
	if eventRrule != "" {
		req.EventRrule = eventRrule
	}

	durationMinutes := runtime.Int(flagDurationMinutes)
	if durationMinutes > 0 {
		req.DurationMinutes = durationMinutes
	}

	timezone := runtime.Str(flagTimezone)
	if timezone != "" {
		req.Timezone = timezone
	}

	excludeStr := runtime.Str(flagExclude)
	if excludeStr != "" {
		excludeStr = strings.TrimSpace(excludeStr)
		var excludedTimes []*EventTime

		ranges := strings.Split(excludeStr, ",")
		for _, r := range ranges {
			r = strings.TrimSpace(r)
			if r == "" {
				continue
			}
			parts := strings.Split(r, "~")
			if len(parts) != 2 {
				return nil, output.ErrValidation("invalid --exclude format %q, expected 'start~end'", r)
			}
			startTsStr, err := common.ParseTime(parts[0])
			if err != nil {
				return nil, output.ErrValidation("invalid start time in --exclude: %q (%v)", parts[0], err)
			}
			endTsStr, err := common.ParseTime(parts[1], "end")
			if err != nil {
				return nil, output.ErrValidation("invalid end time in --exclude: %q (%v)", parts[1], err)
			}
			startSec, err := strconv.ParseInt(startTsStr, 10, 64)
			if err != nil {
				return nil, output.ErrValidation("invalid start timestamp in --exclude: %v", err)
			}
			endSec, err := strconv.ParseInt(endTsStr, 10, 64)
			if err != nil {
				return nil, output.ErrValidation("invalid end timestamp in --exclude: %v", err)
			}
			excludedTimes = append(excludedTimes, &EventTime{
				EventStartTime: time.Unix(startSec, 0).Format(time.RFC3339),
				EventEndTime:   time.Unix(endSec, 0).Format(time.RFC3339),
			})
		}

		req.ExcludedEventTimes = excludedTimes
	}

	return req, nil
}

var CalendarSuggestion = common.Shortcut{
	Service:     "calendar",
	Command:     "+suggestion",
	Description: "Intelligently suggest available time blocks based on unclear time ranges",
	Risk:        "read",
	Scopes:      []string{"calendar:calendar.free_busy:read"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: flagStart, Type: "string", Desc: "search start time (ISO 8601, default: current time)"},
		{Name: flagEnd, Type: "string", Desc: "search end time (ISO 8601, default: end of start day)"},
		{Name: flagAttendees, Type: "string", Desc: "attendee IDs, comma-separated (supports user (open_id) ou_xxx, or chat oc_xxx) ids"},
		{Name: flagEventRrule, Type: "string", Desc: "event recurrence rules"},
		{Name: flagDurationMinutes, Type: "int", Desc: "duration (minutes)"},
		{Name: flagTimezone, Type: "string", Desc: "current time zone"},
		{Name: flagExclude, Type: "string", Desc: "excluded event times (ISO 8601, e.g. '2026-03-19T10:00:00+08:00~2026-03-19T11:00:00+08:00'), comma-separated"},
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		req, err := buildSuggestionRequest(runtime)
		if err != nil {
			return common.NewDryRunAPI().Set("error", err.Error())
		}
		return common.NewDryRunAPI().
			POST(suggestionPath).
			Body(req)
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if err := rejectCalendarAutoBotFallback(runtime); err != nil {
			return err
		}
		durationMinutes := runtime.Int(flagDurationMinutes)
		if durationMinutes != 0 && (durationMinutes < 1 || durationMinutes > 1440) {
			return output.ErrValidation("--duration-minutes must be between 1 and 1440")
		}

		for _, flag := range []string{flagEventRrule, flagTimezone} {
			if val := runtime.Str(flag); val != "" {
				if err := common.RejectDangerousChars("--"+flag, val); err != nil {
					return output.ErrValidation(err.Error())
				}
			}
		}

		if attendeesStr := runtime.Str(flagAttendees); attendeesStr != "" {
			for _, id := range strings.Split(attendeesStr, ",") {
				id = strings.TrimSpace(id)
				if id == "" {
					continue
				}
				if !strings.HasPrefix(id, "ou_") && !strings.HasPrefix(id, "oc_") {
					return output.ErrValidation("invalid attendee id format %q: should start with 'ou_' or 'oc_'", id)
				}
			}
		}

		startInput := runtime.Str(flagStart)
		if startInput != "" {
			if _, err := common.ParseTime(startInput); err != nil {
				return output.ErrValidation("invalid start time: %v", err)
			}
		}

		endInput := runtime.Str(flagEnd)
		if endInput != "" {
			if _, err := common.ParseTime(endInput, "end"); err != nil {
				return output.ErrValidation("invalid end time: %v", err)
			}
		}

		excludeStr := runtime.Str(flagExclude)
		if excludeStr != "" {
			excludeStr = strings.TrimSpace(excludeStr)
			ranges := strings.Split(excludeStr, ",")
			for _, r := range ranges {
				r = strings.TrimSpace(r)
				if r == "" {
					continue
				}
				parts := strings.Split(r, "~")
				if len(parts) != 2 {
					return output.ErrValidation("invalid range format in --exclude: %q, expect start~end", r)
				}
				if _, err := common.ParseTime(parts[0]); err != nil {
					return output.ErrValidation("invalid start time in --exclude: %q (%v)", parts[0], err)
				}
				if _, err := common.ParseTime(parts[1], "end"); err != nil {
					return output.ErrValidation("invalid end time in --exclude: %q (%v)", parts[1], err)
				}
			}
		}

		return nil
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		req, err := buildSuggestionRequest(runtime)
		if err != nil {
			return err
		}

		apiResp, err := runtime.DoAPI(&larkcore.ApiReq{
			HttpMethod: "POST",
			ApiPath:    suggestionPath,
			Body:       req,
		})
		if err != nil {
			return err
		}

		if apiResp.StatusCode < http.StatusOK || apiResp.StatusCode >= http.StatusMultipleChoices {
			return output.ErrAPI(apiResp.StatusCode, "", string(apiResp.RawBody))
		}

		var resp = &OpenAPIResponse[*SuggestionResponse]{}
		if err := json.Unmarshal(apiResp.RawBody, &resp); err != nil {
			return output.ErrWithHint(output.ExitInternal, "validation", "unmarshal response fail", err.Error())
		}

		if resp.Code != 0 {
			return output.ErrAPI(resp.Code, resp.Msg, resp.Data)
		}

		data := resp.Data
		var suggestions []*EventTime
		var aiGuidance string
		if data != nil {
			suggestions = data.Suggestions
			aiGuidance = data.AiActionGuidance
		}
		runtime.OutFormat(data, &output.Meta{Count: len(suggestions)}, func(w io.Writer) {
			if len(suggestions) == 0 {
				fmt.Fprintln(w, "No suggestions available.")
			} else {
				var rows []map[string]interface{}
				for _, item := range suggestions {
					rows = append(rows, map[string]interface{}{
						"start":  item.EventStartTime,
						"end":    item.EventEndTime,
						"reason": item.RecommendReason,
					})
				}
				output.PrintTable(w, rows)
				fmt.Fprintf(w, "\n%d suggestion(s) found\n", len(suggestions))
			}

			if aiGuidance != "" {
				fmt.Fprintf(w, "\nAction Guidance: %s\n", aiGuidance)
			}
		})
		return nil
	},
}
