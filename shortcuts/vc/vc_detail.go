// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT
//
// vc +detail — get meeting details including note_id and minute_token

package vc

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/credential"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

const detailLogPrefix = "[vc +detail]"

var scopesDetailMeetingIDs = []string{
	"vc:meeting.meetingevent:read",
	"vc:record:readonly",
}

// meetingDetailItem represents a single meeting detail result.
type meetingDetailItem struct {
	MeetingID   string `json:"meeting_id"`
	MeetingNo   string `json:"meeting_no,omitempty"`
	Topic       string `json:"topic"`
	StartTime   string `json:"start_time,omitempty"`
	EndTime     string `json:"end_time,omitempty"`
	NoteID      string `json:"note_id,omitempty"`
	MinuteToken string `json:"minute_token,omitempty"`
	Error       string `json:"error,omitempty"`
	Hint        string `json:"hint,omitempty"`
}

// fetchMeetingDetail queries meeting.get and recording API to return a
// consolidated view of meeting metadata, note_id, and minute_token.
// Error is only set when an API call actually fails; note_id and minute_token
// are always present (empty string when not available).
func fetchMeetingDetail(ctx context.Context, runtime *common.RuntimeContext, meetingID string) *meetingDetailItem {
	result := &meetingDetailItem{MeetingID: meetingID}

	// Step 1: query meeting detail
	data, err := runtime.CallAPITyped(http.MethodGet,
		fmt.Sprintf("/open-apis/vc/v1/meetings/%s", validate.EncodePathSegment(meetingID)),
		map[string]interface{}{"with_participants": "false", "query_mode": "0"}, nil)
	if err != nil {
		result.Error = fmt.Sprintf("failed to query meeting detail: %v", err)
		return result
	}

	meeting, _ := data["meeting"].(map[string]any)
	if meeting == nil {
		result.Error = "meeting not found in response"
		return result
	}

	if v, ok := meeting["meeting_no"].(string); ok {
		result.MeetingNo = v
	}
	if v, ok := meeting["topic"].(string); ok {
		result.Topic = v
	}
	if v := common.FormatTime(meeting["start_time"]); v != "" {
		result.StartTime = v
	}
	if v := common.FormatTime(meeting["end_time"]); v != "" {
		result.EndTime = v
	}
	if v, ok := meeting["note_id"].(string); ok && v != "" {
		result.NoteID = v
	}

	// Step 2: query minute_token via recording API — only meaningful once the
	// meeting has ended. While it is still in progress the note/minute are not
	// generated yet, so skip the recording call and surface an informational
	// hint instead of letting an unclassified recording error fail the command.
	inProgress := meetingInProgress(meeting)
	var minuteHint string
	if inProgress {
		minuteHint = "meeting is still in progress; note and minute are not generated yet"
	} else {
		minuteToken, hint, minuteErr := fetchMeetingMinuteToken(runtime, meetingID)
		minuteHint = hint
		if minuteErr != nil {
			// Recording lookup is a best-effort supplement; step 1 already
			// succeeded, so degrade the failure to a hint rather than failing
			// the whole command.
			minuteHint = fmt.Sprintf("failed to query minutes: %v", minuteErr)
		}
		if minuteToken != "" {
			result.MinuteToken = minuteToken
		}
	}

	// Add hints for empty resources (not errors, just informational). For an
	// in-progress meeting the "not found" wording is noise, so we only emit the
	// single in-progress hint below.
	if !inProgress {
		var emptyFields []string
		if result.NoteID == "" {
			emptyFields = append(emptyFields, "note_id")
		}
		if result.MinuteToken == "" && minuteHint == "" {
			emptyFields = append(emptyFields, "minute_token")
		}
		if len(emptyFields) > 0 {
			result.Hint = fmt.Sprintf("%s not found for this meeting", strings.Join(emptyFields, ", "))
		}
	}
	if minuteHint != "" {
		if result.Hint != "" {
			result.Hint += "; " + minuteHint
		} else {
			result.Hint = minuteHint
		}
	}

	return result
}

// meetingTimeField reads a meeting time field as a string regardless of whether
// the API returned it as a JSON string or number. VC serializes int64
// timestamps as strings, but coercing via %v keeps parsing robust either way;
// float64(0) renders as "0", which parseFlexibleTime treats as "absent".
func meetingTimeField(meeting map[string]any, key string) string {
	v, ok := meeting[key]
	if !ok || v == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprintf("%v", v))
}

// meetingInProgress reports whether a meeting is still ongoing, using the same
// start/end heuristic as +meeting-events (meetingEventsMeetingFromPayload): a
// meeting is ongoing when it has a start time but no end time, or its end time
// is not after its start time. It reads the RAW timestamp fields, not the
// FormatTime-rendered result strings, because parseFlexibleTime only accepts
// Unix timestamps or RFC3339. Empty or "0" values are treated as absent.
func meetingInProgress(meeting map[string]any) bool {
	start, hasStart := parseFlexibleTime(meetingTimeField(meeting, "start_time"))
	end, hasEnd := parseFlexibleTime(meetingTimeField(meeting, "end_time"))
	if !hasStart {
		return false
	}
	if !hasEnd {
		return true
	}
	return !end.After(start)
}

// VCDetail gets meeting details including note_id and minute_token.
var VCDetail = common.Shortcut{
	Service:     "vc",
	Command:     "+detail",
	Description: "Get meeting details including note_id and minute_token by meeting IDs",
	Risk:        "read",
	Scopes:      []string{"vc:meeting.meetingevent:read", "vc:record:readonly"},
	AuthTypes:   []string{"user"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "meeting-ids", Desc: "meeting IDs, comma-separated for batch", Required: true},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		ids := common.SplitCSV(runtime.Str("meeting-ids"))
		const maxBatchSize = 50
		if len(ids) > maxBatchSize {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--meeting-ids: too many IDs (%d), maximum is %d", len(ids), maxBatchSize).WithParam("--meeting-ids")
		}
		// dynamic scope check
		result, err := runtime.Factory.Credential.ResolveToken(ctx, credential.NewTokenSpec(runtime.As(), runtime.Config.AppID))
		if err == nil && result != nil && result.Scopes != "" {
			if missing := auth.MissingScopes(result.Scopes, scopesDetailMeetingIDs); len(missing) > 0 {
				return errs.NewPermissionError(errs.SubtypeMissingScope,
					"missing required scope(s): %s", strings.Join(missing, ", ")).
					WithHint("run `lark-cli auth login --scope %q` in the background. It blocks and outputs a verification URL — retrieve the URL and open it in a browser to complete login.", strings.Join(missing, " ")).
					WithMissingScopes(missing...).
					WithIdentity(string(runtime.As()))
			}
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		ids := runtime.Str("meeting-ids")
		return common.NewDryRunAPI().
			GET("/open-apis/vc/v1/meetings/{meeting_id}").
			GET("/open-apis/vc/v1/meetings/{meeting_id}/recording").
			Set("meeting_ids", common.SplitCSV(ids)).
			Set("steps", "meeting.get → note_id + recording API → minute_token")
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		errOut := runtime.IO().ErrOut
		meetingIDs := common.SplitCSV(runtime.Str("meeting-ids"))
		results := make([]*meetingDetailItem, 0, len(meetingIDs))

		const batchDelay = 100 * time.Millisecond
		fmt.Fprintf(errOut, "%s querying %d meeting_id(s)\n", detailLogPrefix, len(meetingIDs))
		for i, id := range meetingIDs {
			if err := ctx.Err(); err != nil {
				return err
			}
			if i > 0 {
				time.Sleep(batchDelay)
			}
			fmt.Fprintf(errOut, "%s querying meeting_id=%s ...\n", detailLogPrefix, sanitizeLogValue(id))
			results = append(results, fetchMeetingDetail(ctx, runtime, id))
		}

		successCount := 0
		for _, r := range results {
			if r.Error == "" {
				successCount++
			}
		}
		fmt.Fprintf(errOut, "%s done: %d total, %d succeeded, %d failed\n", detailLogPrefix, len(results), successCount, len(results)-successCount)

		if successCount == 0 && len(results) > 0 {
			return runtime.OutPartialFailure(map[string]any{"meetings": results}, &output.Meta{Count: len(results)})
		}

		outData := map[string]any{"meetings": results}
		runtime.OutFormat(outData, &output.Meta{Count: len(results)}, func(w io.Writer) {
			if len(results) == 0 {
				fmt.Fprintln(w, "No meetings.")
				return
			}
			var rows []map[string]interface{}
			for _, r := range results {
				row := map[string]interface{}{"meeting_id": r.MeetingID}
				if r.Error != "" {
					row["status"] = "FAIL"
					row["error"] = r.Error
				} else {
					row["status"] = "OK"
				}
				if r.NoteID != "" {
					row["note_id"] = r.NoteID
				}
				if r.MinuteToken != "" {
					row["minute_token"] = r.MinuteToken
				}
				row["topic"] = r.Topic
				if r.Hint != "" {
					row["hint"] = r.Hint
				}
				rows = append(rows, row)
			}
			output.PrintTable(w, rows)
			fmt.Fprintf(w, "\n%d meeting(s), %d succeeded, %d failed\n", len(results), successCount, len(results)-successCount)
		})
		return nil
	},
}
