// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT
//
// vc +recording — query minute_token from meeting-ids or calendar-event-ids
//
// Two mutually exclusive input modes:
//   meeting-ids:        recording API → extract minute_token from URL
//   calendar-event-ids: primary calendar → mget_instance_relation_info → meeting_id → recording API

package vc

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

const recordingLogPrefix = "[vc +recording]"

var (
	scopesRecordingMeetingIDs = []string{
		"vc:record:readonly",
	}
	scopesRecordingCalendarEventIDs = []string{
		"vc:record:readonly",
		"calendar:calendar:read",
		"calendar:calendar.event:read",
	}
)

// extractMinuteToken parses minute_token from a recording URL.
// URL format: https://meetings.feishu.cn/minutes/{minute_token}
func extractMinuteToken(recordingURL string) string {
	u, err := url.Parse(recordingURL)
	if err != nil {
		return ""
	}
	parts := strings.Split(strings.TrimRight(u.Path, "/"), "/")
	for i, p := range parts {
		if p == "minutes" && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return ""
}

// fetchRecordingByMeetingID queries recording info for a single meeting.
func fetchRecordingByMeetingID(_ context.Context, runtime *common.RuntimeContext, meetingID string) map[string]any {
	data, err := runtime.DoAPIJSON(http.MethodGet,
		fmt.Sprintf("/open-apis/vc/v1/meetings/%s/recording", validate.EncodePathSegment(meetingID)),
		nil, nil)
	if err != nil {
		return map[string]any{"meeting_id": meetingID, "error": fmt.Sprintf("failed to query recording: %v", err)}
	}

	recording, _ := data["recording"].(map[string]any)
	if recording == nil {
		return map[string]any{"meeting_id": meetingID, "error": "no recording available for this meeting"}
	}

	recordingURL, _ := recording["url"].(string)
	duration, _ := recording["duration"].(string)

	result := map[string]any{"meeting_id": meetingID}
	if recordingURL != "" {
		result["recording_url"] = recordingURL
	}
	if duration != "" {
		result["duration"] = duration
	}
	if token := extractMinuteToken(recordingURL); token != "" {
		result["minute_token"] = token
	}
	return result
}

// VCRecording gets meeting recording info and extracts minute_token.
var VCRecording = common.Shortcut{
	Service:     "vc",
	Command:     "+recording",
	Description: "Query minute_token from meeting-ids or calendar-event-ids",
	Risk:        "read",
	Scopes:      []string{"vc:record:readonly"},
	AuthTypes:   []string{"user"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "meeting-ids", Desc: "meeting IDs, comma-separated for batch"},
		{Name: "calendar-event-ids", Desc: "calendar event instance IDs, comma-separated for batch"},
	},
	Validate: func(_ context.Context, runtime *common.RuntimeContext) error {
		if err := common.ExactlyOne(runtime, "meeting-ids", "calendar-event-ids"); err != nil {
			return err
		}
		const maxBatchSize = 50
		for _, flag := range []string{"meeting-ids", "calendar-event-ids"} {
			if v := runtime.Str(flag); v != "" {
				if ids := common.SplitCSV(v); len(ids) > maxBatchSize {
					return output.ErrValidation("--%s: too many IDs (%d), maximum is %d", flag, len(ids), maxBatchSize)
				}
			}
		}
		var required []string
		switch {
		case runtime.Str("meeting-ids") != "":
			required = scopesRecordingMeetingIDs
		case runtime.Str("calendar-event-ids") != "":
			required = scopesRecordingCalendarEventIDs
		}
		appID := runtime.Config.AppID
		userOpenID := runtime.UserOpenId()
		if appID != "" && userOpenID != "" {
			stored := auth.GetStoredToken(appID, userOpenID)
			if stored != nil {
				if missing := auth.MissingScopes(stored.Scope, required); len(missing) > 0 {
					return output.ErrWithHint(output.ExitAuth, "missing_scope",
						fmt.Sprintf("missing required scope(s): %s", strings.Join(missing, ", ")),
						fmt.Sprintf("run `lark-cli auth login --scope \"%s\"` in the background. It blocks and outputs a verification URL — retrieve the URL and open it in a browser to complete login.", strings.Join(missing, " ")))
				}
			}
		}
		return nil
	},
	DryRun: func(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		if ids := runtime.Str("meeting-ids"); ids != "" {
			return common.NewDryRunAPI().
				GET("/open-apis/vc/v1/meetings/{meeting_id}/recording").
				Set("meeting_ids", common.SplitCSV(ids)).
				Set("steps", "meeting recording API → extract minute_token from URL")
		}
		ids := runtime.Str("calendar-event-ids")
		return common.NewDryRunAPI().
			POST("/open-apis/calendar/v4/calendars/primary").
			POST("/open-apis/calendar/v4/calendars/{calendar_id}/events/mget_instance_relation_info").
			GET("/open-apis/vc/v1/meetings/{meeting_id}/recording").
			Set("calendar_event_ids", common.SplitCSV(ids)).
			Set("steps", "primary calendar → mget_instance_relation_info → meeting_id → recording API → extract minute_token")
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		errOut := runtime.IO().ErrOut
		var results []any

		const batchDelay = 100 * time.Millisecond

		if ids := runtime.Str("meeting-ids"); ids != "" {
			meetingIDs := common.SplitCSV(ids)
			fmt.Fprintf(errOut, "%s querying %d meeting_id(s)\n", recordingLogPrefix, len(meetingIDs))
			for i, id := range meetingIDs {
				if err := ctx.Err(); err != nil {
					return err
				}
				if i > 0 {
					time.Sleep(batchDelay)
				}
				fmt.Fprintf(errOut, "%s querying meeting_id=%s ...\n", recordingLogPrefix, sanitizeLogValue(id))
				results = append(results, fetchRecordingByMeetingID(ctx, runtime, id))
			}
		} else {
			instanceIDs := common.SplitCSV(runtime.Str("calendar-event-ids"))
			fmt.Fprintf(errOut, "%s querying %d calendar_event_id(s)\n", recordingLogPrefix, len(instanceIDs))
			calendarID, err := getPrimaryCalendarID(runtime)
			if err != nil {
				return err
			}
			fmt.Fprintf(errOut, "%s primary calendar: %s\n", recordingLogPrefix, calendarID)
			for i, instanceID := range instanceIDs {
				if err := ctx.Err(); err != nil {
					return err
				}
				if i > 0 {
					time.Sleep(batchDelay)
				}
				fmt.Fprintf(errOut, "%s resolving calendar_event_id=%s ...\n", recordingLogPrefix, sanitizeLogValue(instanceID))
				relInfo, resolveErr := resolveMeetingIDsFromCalendarEvent(runtime, instanceID, calendarID, false)
				if resolveErr != nil {
					results = append(results, map[string]any{"calendar_event_id": instanceID, "error": resolveErr.Error()})
					continue
				}
				found := false
				for _, meetingID := range relInfo.MeetingIDs {
					fmt.Fprintf(errOut, "%s event %s → meeting_id=%s\n", recordingLogPrefix, sanitizeLogValue(instanceID), sanitizeLogValue(meetingID))
					result := fetchRecordingByMeetingID(ctx, runtime, meetingID)
					if result["error"] == nil {
						result["calendar_event_id"] = instanceID
						results = append(results, result)
						found = true
						break
					}
					fmt.Fprintf(errOut, "%s meeting_id=%s: %s, trying next\n", recordingLogPrefix, sanitizeLogValue(meetingID), result["error"])
				}
				if !found {
					results = append(results, map[string]any{"calendar_event_id": instanceID, "error": "no recording found in any associated meeting"})
				}
			}
		}

		successCount := 0
		for _, r := range results {
			m, _ := r.(map[string]any)
			if m["error"] == nil {
				successCount++
			}
		}
		fmt.Fprintf(errOut, "%s done: %d total, %d succeeded, %d failed\n", recordingLogPrefix, len(results), successCount, len(results)-successCount)

		if successCount == 0 && len(results) > 0 {
			outData := map[string]any{"recordings": results}
			runtime.OutFormat(outData, &output.Meta{Count: len(results)}, nil)
			return output.ErrAPI(0, fmt.Sprintf("all %d queries failed", len(results)), nil)
		}

		outData := map[string]any{"recordings": results}
		runtime.OutFormat(outData, &output.Meta{Count: len(results)}, func(w io.Writer) {
			var rows []map[string]interface{}
			for _, r := range results {
				m, _ := r.(map[string]any)
				meetingID, _ := m["meeting_id"].(string)
				row := map[string]interface{}{}
				if meetingID != "" {
					row["meeting_id"] = meetingID
				}
				if calEventID, _ := m["calendar_event_id"].(string); calEventID != "" {
					row["calendar_event_id"] = calEventID
				}
				if errMsg, _ := m["error"].(string); errMsg != "" {
					row["status"] = "FAIL"
					row["error"] = errMsg
				} else {
					row["status"] = "OK"
					if v, _ := m["minute_token"].(string); v != "" {
						row["minute_token"] = v
					}
					if v, _ := m["duration"].(string); v != "" {
						row["duration"] = v
					}
				}
				rows = append(rows, row)
			}
			output.PrintTable(w, rows)
			fmt.Fprintf(w, "\n%d recording(s), %d succeeded, %d failed\n", len(results), successCount, len(results)-successCount)
		})
		return nil
	},
}
