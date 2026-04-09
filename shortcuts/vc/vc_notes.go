// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT
//
// vc +notes — query meeting notes
//
// Three mutually exclusive input modes (only one allowed per invocation):
//   meeting-ids:        meeting.get → note_id → note detail API
//   minute-tokens:      minutes API → note detail + AI artifacts + transcript
//   calendar-event-ids: primary calendar → mget_instance_relation_info → meeting_id → meeting.get → note_id

package vc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/credential"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

// per-flag additional scope requirements for +notes (vc:note:read is checked by framework)
var (
	scopesMeetingIDs = []string{
		"vc:meeting.meetingevent:read",
	}
	scopesMinuteTokens = []string{
		"minutes:minutes:readonly",
		"minutes:minutes.artifacts:read",
		"minutes:minutes.transcript:export",
	}
	scopesCalendarEventIDs = []string{
		"calendar:calendar:read",
		"calendar:calendar.event:read",
		"vc:meeting.meetingevent:read",
	}
)

// artifact type enum from note detail API
const (
	artifactTypeMainDoc  = 1 // main note document
	artifactTypeVerbatim = 2 // verbatim transcript
)

const logPrefix = "[vc +notes]"

// sanitizeLogValue strips newlines and ANSI escape sequences from user input for safe logging.
func sanitizeLogValue(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	// strip ANSI escape sequences (ESC[...)
	for i := strings.Index(s, "\x1b["); i >= 0; i = strings.Index(s, "\x1b[") {
		end := strings.IndexByte(s[i+2:], 'm')
		if end < 0 {
			s = s[:i]
			break
		}
		s = s[:i] + s[i+2+end+1:]
	}
	return s
}

// getPrimaryCalendarID retrieves the current user's primary calendar ID.
func getPrimaryCalendarID(runtime *common.RuntimeContext) (string, error) {
	data, err := runtime.DoAPIJSON(http.MethodPost, "/open-apis/calendar/v4/calendars/primary", nil, nil)
	if err != nil {
		return "", err // preserve original API error (with lark error code)
	}
	calendars, _ := data["calendars"].([]any)
	if len(calendars) == 0 {
		return "", output.ErrValidation("primary calendar not found")
	}
	first, _ := calendars[0].(map[string]any)
	cal, _ := first["calendar"].(map[string]any)
	calID, _ := cal["calendar_id"].(string)
	if calID == "" {
		return "", output.ErrValidation("primary calendar ID is empty")
	}
	return calID, nil
}

// resolveMeetingIDsFromCalendarEvent resolves a calendar event instance to its
// associated meeting IDs via the mget_instance_relation_info API.
// Shared by +notes and +recording for the --calendar-event-ids path.
func resolveMeetingIDsFromCalendarEvent(runtime *common.RuntimeContext, instanceID string, calendarID string) ([]string, error) {
	data, err := runtime.DoAPIJSON(http.MethodPost,
		fmt.Sprintf("/open-apis/calendar/v4/calendars/%s/events/mget_instance_relation_info", validate.EncodePathSegment(calendarID)),
		nil,
		map[string]any{
			"instance_ids":              []string{instanceID},
			"need_meeting_instance_ids": true,
		})
	if err != nil {
		return nil, fmt.Errorf("failed to query event relation info: %w", err)
	}

	infos, _ := data["instance_relation_infos"].([]any)
	if len(infos) == 0 {
		return nil, fmt.Errorf("no event relation info found")
	}
	info, _ := infos[0].(map[string]any)

	rawIDs, _ := info["meeting_instance_ids"].([]any)
	if len(rawIDs) == 0 {
		return nil, fmt.Errorf("no associated video meeting for this event")
	}

	var ids []string
	for _, mid := range rawIDs {
		if mid == nil {
			continue
		}
		var meetingID string
		switch v := mid.(type) {
		case float64:
			meetingID = fmt.Sprintf("%.0f", v)
		case string:
			meetingID = v
		default:
			meetingID = fmt.Sprintf("%v", v)
		}
		ids = append(ids, meetingID)
	}
	return ids, nil
}

// fetchNoteByCalendarEventID queries notes via calendar event instance ID.
// Chain: primary calendar → mget_instance_relation_info → meeting_id → meeting.get → note_id
func fetchNoteByCalendarEventID(ctx context.Context, runtime *common.RuntimeContext, instanceID string, calendarID string) map[string]any {
	errOut := runtime.IO().ErrOut

	meetingIDs, err := resolveMeetingIDsFromCalendarEvent(runtime, instanceID, calendarID)
	if err != nil {
		return map[string]any{"calendar_event_id": instanceID, "error": err.Error()}
	}

	if len(meetingIDs) > 1 {
		fmt.Fprintf(errOut, "%s event %s has %d meetings, trying each\n", logPrefix, sanitizeLogValue(instanceID), len(meetingIDs))
	}

	// try each associated meeting until one has notes
	for _, meetingID := range meetingIDs {
		fmt.Fprintf(errOut, "%s event %s → meeting_id=%s\n", logPrefix, sanitizeLogValue(instanceID), sanitizeLogValue(meetingID))
		result := fetchNoteByMeetingID(ctx, runtime, meetingID)
		if result["error"] == nil {
			return result
		}
		fmt.Fprintf(errOut, "%s meeting_id=%s: %s, trying next\n", logPrefix, sanitizeLogValue(meetingID), result["error"])
	}
	return map[string]any{"calendar_event_id": instanceID, "error": "no notes found in any associated meeting"}
}

// fetchNoteByMeetingID queries notes via meeting_id.
func fetchNoteByMeetingID(ctx context.Context, runtime *common.RuntimeContext, meetingID string) map[string]any {
	data, err := runtime.DoAPIJSON(http.MethodGet, fmt.Sprintf("/open-apis/vc/v1/meetings/%s", validate.EncodePathSegment(meetingID)),
		larkcore.QueryParams{"with_participants": []string{"false"}, "query_mode": []string{"0"}}, nil)
	if err != nil {
		return map[string]any{"meeting_id": meetingID, "error": fmt.Sprintf("failed to query meeting: %v", err)}
	}

	meeting, _ := data["meeting"].(map[string]any)
	if meeting == nil {
		return map[string]any{"meeting_id": meetingID, "error": "meeting not found"}
	}

	noteID, _ := meeting["note_id"].(string)
	if noteID == "" {
		return map[string]any{"meeting_id": meetingID, "error": "no notes available for this meeting"}
	}

	result := fetchNoteDetail(ctx, runtime, noteID)
	result["meeting_id"] = meetingID
	return result
}

// fetchNoteByMinuteToken queries notes via minute_token.
// Fetches both note detail (doc tokens) and AI artifacts (summary/todos/chapters inline +
// transcript to file) independently, merging into a single result map for Agent consumption.
func fetchNoteByMinuteToken(ctx context.Context, runtime *common.RuntimeContext, minuteToken string) map[string]any {
	errOut := runtime.IO().ErrOut

	data, err := runtime.DoAPIJSON(http.MethodGet, fmt.Sprintf("/open-apis/minutes/v1/minutes/%s", validate.EncodePathSegment(minuteToken)), nil, nil)
	if err != nil {
		return map[string]any{"minute_token": minuteToken, "error": fmt.Sprintf("failed to query minutes: %v", err)}
	}

	minute, _ := data["minute"].(map[string]any)
	if minute == nil {
		return map[string]any{"minute_token": minuteToken, "error": "minutes not found"}
	}

	result := map[string]any{"minute_token": minuteToken}
	title, _ := minute["title"].(string)
	if title != "" {
		result["title"] = title
	}

	// path 1: note detail (doc tokens) — fetch when note_id exists
	noteID, _ := minute["note_id"].(string)
	if noteID != "" {
		noteResult := fetchNoteDetail(ctx, runtime, noteID)
		if errMsg, _ := noteResult["error"].(string); errMsg != "" {
			fmt.Fprintf(errOut, "%s note detail failed: %s\n", logPrefix, errMsg)
		} else {
			// merge note detail fields into result
			for k, v := range noteResult {
				result[k] = v
			}
		}
	}

	// path 2 & 3: AI 产物统一归到 artifacts 字段下
	artifacts := map[string]any{}
	fetchInlineArtifacts(runtime, minuteToken, artifacts)
	transcriptPath := downloadTranscriptFile(runtime, minuteToken, title)
	if transcriptPath != "" {
		artifacts["transcript_file"] = transcriptPath
	}
	if len(artifacts) > 0 {
		result["artifacts"] = artifacts
	}

	return result
}

// sanitizeDirName generates a safe directory name using title and minuteToken for uniqueness.
func sanitizeDirName(title, minuteToken string) string {
	const maxLen = 200
	replacer := strings.NewReplacer(
		"/", "_", "\\", "_", ":", "_", "*", "_", "?", "_",
		"\"", "_", "<", "_", ">", "_", "|", "_",
		"\n", "_", "\r", "_", "\t", "_", "\x00", "_",
	)
	safe := replacer.Replace(strings.TrimSpace(title))
	safe = strings.Trim(safe, ".") // remove leading/trailing dots
	if len(safe) > maxLen {
		safe = safe[:maxLen]
	}
	if safe == "" {
		return fmt.Sprintf("artifact-%s", minuteToken)
	}
	return fmt.Sprintf("artifact-%s-%s", safe, minuteToken)
}

// downloadTranscriptFile downloads transcript to a local file and returns the file path (empty on failure).
func downloadTranscriptFile(runtime *common.RuntimeContext, minuteToken string, title string) string {
	errOut := runtime.IO().ErrOut

	base := "."
	if outDir := runtime.Str("output-dir"); outDir != "" {
		base = outDir
	}
	dirName := filepath.Join(base, sanitizeDirName(title, minuteToken))
	transcriptPath := filepath.Join(dirName, "transcript.txt")

	// Overwrite check via FileIO.Stat
	if !runtime.Bool("overwrite") {
		if _, statErr := runtime.FileIO().Stat(transcriptPath); statErr == nil {
			fmt.Fprintf(errOut, "%s transcript already exists: %s (use --overwrite to replace)\n", logPrefix, transcriptPath)
			return transcriptPath
		}
	}

	fmt.Fprintf(errOut, "%s downloading transcript: %s\n", logPrefix, transcriptPath)
	apiResp, err := runtime.DoAPI(&larkcore.ApiReq{
		HttpMethod: http.MethodGet,
		ApiPath:    fmt.Sprintf("/open-apis/minutes/v1/minutes/%s/transcript", validate.EncodePathSegment(minuteToken)),
		QueryParams: larkcore.QueryParams{
			"need_speaker":   []string{"true"},
			"need_timestamp": []string{"true"},
			"file_format":    []string{"txt"},
		},
	}, larkcore.WithFileDownload())
	if err != nil {
		fmt.Fprintf(errOut, "%s failed to download transcript: %v\n", logPrefix, err)
		return ""
	}
	if apiResp.StatusCode >= 400 {
		fmt.Fprintf(errOut, "%s failed to download transcript: HTTP %d\n", logPrefix, apiResp.StatusCode)
		return ""
	}
	if len(apiResp.RawBody) == 0 {
		fmt.Fprintf(errOut, "%s transcript is empty (not available for this minute)\n", logPrefix)
		return ""
	}
	if _, err := runtime.FileIO().Save(transcriptPath, fileio.SaveOptions{}, bytes.NewReader(apiResp.RawBody)); err != nil {
		var me *fileio.MkdirError
		switch {
		case errors.Is(err, fileio.ErrPathValidation):
			fmt.Fprintf(errOut, "%s invalid transcript path: %v\n", logPrefix, err)
		case errors.As(err, &me):
			fmt.Fprintf(errOut, "%s failed to create directory: %v\n", logPrefix, err)
		default:
			fmt.Fprintf(errOut, "%s failed to write transcript: %v\n", logPrefix, err)
		}
		return ""
	}
	return transcriptPath
}

// fetchInlineArtifacts fetches summary/todos/chapters from artifacts API and writes them inline into result map.
func fetchInlineArtifacts(runtime *common.RuntimeContext, minuteToken string, result map[string]any) {
	errOut := runtime.IO().ErrOut
	fmt.Fprintf(errOut, "%s fetching AI artifacts...\n", logPrefix)
	data, err := runtime.DoAPIJSON(http.MethodGet, fmt.Sprintf("/open-apis/minutes/v1/minutes/%s/artifacts", validate.EncodePathSegment(minuteToken)), nil, nil)
	if err != nil {
		fmt.Fprintf(errOut, "%s failed to fetch AI artifacts: %v\n", logPrefix, err)
		return
	}
	if summary, ok := data["summary"].(string); ok && summary != "" {
		result["summary"] = summary
	}
	if todos, ok := data["minute_todos"].([]any); ok && len(todos) > 0 {
		result["todos"] = todos
	}
	if chapters, ok := data["minute_chapters"].([]any); ok && len(chapters) > 0 {
		result["chapters"] = chapters
	}
}

// parseArtifactType extracts artifact_type as int from varying JSON number representations.
func parseArtifactType(v any) int {
	switch n := v.(type) {
	case json.Number:
		i, _ := n.Int64()
		return int(i)
	case float64:
		return int(n)
	default:
		return 0
	}
}

// extractArtifactTokens picks main-doc and verbatim-doc tokens from the artifacts list.
func extractArtifactTokens(artifacts []any) (noteDoc, verbatimDoc string) {
	for _, a := range artifacts {
		artifact, _ := a.(map[string]any)
		if artifact == nil {
			continue
		}
		docToken, _ := artifact["doc_token"].(string)
		switch parseArtifactType(artifact["artifact_type"]) {
		case artifactTypeMainDoc:
			noteDoc = docToken
		case artifactTypeVerbatim:
			verbatimDoc = docToken
		default:
			// ignore unknown artifact types
		}
	}
	return
}

// extractDocTokens collects doc_token values from a list of reference objects.
func extractDocTokens(refs []any) []string {
	var tokens []string
	for _, s := range refs {
		source, _ := s.(map[string]any)
		if source == nil {
			continue
		}
		if docToken, _ := source["doc_token"].(string); docToken != "" {
			tokens = append(tokens, docToken)
		}
	}
	return tokens
}

// fetchNoteDetail retrieves note document tokens via note_id.
func fetchNoteDetail(_ context.Context, runtime *common.RuntimeContext, noteID string) map[string]any {
	data, err := runtime.DoAPIJSON(http.MethodGet, fmt.Sprintf("/open-apis/vc/v1/notes/%s", validate.EncodePathSegment(noteID)), nil, nil)
	if err != nil {
		return map[string]any{"error": fmt.Sprintf("failed to query note detail: %v", err)}
	}

	note, _ := data["note"].(map[string]any)
	if note == nil {
		return map[string]any{"error": "note detail is empty"}
	}

	creatorID, _ := note["creator_id"].(string)
	createTime := common.FormatTime(note["create_time"])
	noteDocToken, verbatimDocToken := extractArtifactTokens(common.GetSlice(note, "artifacts"))
	sharedDocTokens := extractDocTokens(common.GetSlice(note, "references"))

	result := map[string]any{
		"creator_id":         creatorID,
		"create_time":        createTime,
		"note_doc_token":     noteDocToken,
		"verbatim_doc_token": verbatimDocToken,
	}
	if len(sharedDocTokens) > 0 {
		result["shared_doc_tokens"] = sharedDocTokens
	}
	return result
}

// VCNotes queries meeting notes via meeting-ids, minute-tokens, or calendar-event-ids.
var VCNotes = common.Shortcut{
	Service:     "vc",
	Command:     "+notes",
	Description: "Query meeting notes (via meeting-ids, minute-tokens, or calendar-event-ids)",
	Risk:        "read",
	Scopes:      []string{"vc:note:read"}, // minimum scope; additional per-flag scopes checked in Validate
	AuthTypes:   []string{"user"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "meeting-ids", Desc: "meeting IDs, comma-separated for batch"},
		{Name: "minute-tokens", Desc: "minute tokens, comma-separated for batch"},
		{Name: "calendar-event-ids", Desc: "calendar event instance IDs, comma-separated for batch"},
		{Name: "output-dir", Desc: "output directory for artifact files (default: current dir)"},
		{Name: "overwrite", Type: "bool", Desc: "overwrite existing artifact files"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if err := common.ExactlyOne(runtime, "meeting-ids", "minute-tokens", "calendar-event-ids"); err != nil {
			return err
		}
		// batch input size limit
		const maxBatchSize = 50
		for _, flag := range []string{"meeting-ids", "minute-tokens", "calendar-event-ids"} {
			if v := runtime.Str(flag); v != "" {
				if ids := common.SplitCSV(v); len(ids) > maxBatchSize {
					return output.ErrValidation("--%s: too many IDs (%d), maximum is %d", flag, len(ids), maxBatchSize)
				}
			}
		}
		// output-dir 路径安全校验
		if outDir := runtime.Str("output-dir"); outDir != "" {
			if err := common.ValidateSafeOutputDir(outDir); err != nil {
				return err
			}
		}
		// dynamic scope check based on which flag is provided
		var required []string
		switch {
		case runtime.Str("meeting-ids") != "":
			required = scopesMeetingIDs
		case runtime.Str("minute-tokens") != "":
			required = scopesMinuteTokens
		case runtime.Str("calendar-event-ids") != "":
			required = scopesCalendarEventIDs
		default:
			// unreachable: ExactlyOne already ensures one flag is set
		}
		result, err := runtime.Factory.Credential.ResolveToken(ctx, credential.NewTokenSpec(runtime.As(), runtime.Config.AppID))
		if err == nil && result != nil && result.Scopes != "" {
			if missing := auth.MissingScopes(result.Scopes, required); len(missing) > 0 {
				return output.ErrWithHint(output.ExitAuth, "missing_scope",
					fmt.Sprintf("missing required scope(s): %s", strings.Join(missing, ", ")),
					fmt.Sprintf("run `lark-cli auth login --scope \"%s\"` in the background. It blocks and outputs a verification URL — retrieve the URL and open it in a browser to complete login.", strings.Join(missing, " ")))
			}
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		if ids := runtime.Str("meeting-ids"); ids != "" {
			return common.NewDryRunAPI().
				GET("/open-apis/vc/v1/meetings/{meeting_id}").
				GET("/open-apis/vc/v1/notes/{note_id}").
				Set("meeting_ids", common.SplitCSV(ids)).
				Set("steps", "meeting.get → note_id → note detail API")
		}
		if tokens := runtime.Str("minute-tokens"); tokens != "" {
			return common.NewDryRunAPI().
				GET("/open-apis/minutes/v1/minutes/{minute_token}").
				GET("/open-apis/vc/v1/notes/{note_id}").
				GET("/open-apis/minutes/v1/minutes/{minute_token}/artifacts").
				GET("/open-apis/minutes/v1/minutes/{minute_token}/transcript").
				Set("minute_tokens", common.SplitCSV(tokens)).
				Set("steps", "minutes API → note detail + AI artifacts + transcript")
		}
		ids := runtime.Str("calendar-event-ids")
		return common.NewDryRunAPI().
			POST("/open-apis/calendar/v4/calendars/primary").
			POST("/open-apis/calendar/v4/calendars/{calendar_id}/events/mget_instance_relation_info").
			GET("/open-apis/vc/v1/meetings/{meeting_id}").
			GET("/open-apis/vc/v1/notes/{note_id}").
			Set("calendar_event_ids", common.SplitCSV(ids)).
			Set("steps", "primary calendar → mget_instance_relation_info → meeting_id → meeting.get → note detail API")
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		errOut := runtime.IO().ErrOut
		var results []any

		const batchDelay = 100 * time.Millisecond

		if ids := runtime.Str("meeting-ids"); ids != "" {
			meetingIDs := common.SplitCSV(ids)
			fmt.Fprintf(errOut, "%s querying %d meeting_id(s)\n", logPrefix, len(meetingIDs))
			for i, id := range meetingIDs {
				if err := ctx.Err(); err != nil {
					return err
				}
				if i > 0 {
					time.Sleep(batchDelay)
				}
				fmt.Fprintf(errOut, "%s querying meeting_id=%s ...\n", logPrefix, sanitizeLogValue(id))
				results = append(results, fetchNoteByMeetingID(ctx, runtime, id))
			}
		} else if tokens := runtime.Str("minute-tokens"); tokens != "" {
			minuteTokens := common.SplitCSV(tokens)
			fmt.Fprintf(errOut, "%s querying %d minute_token(s)\n", logPrefix, len(minuteTokens))
			for i, token := range minuteTokens {
				if err := ctx.Err(); err != nil {
					return err
				}
				if i > 0 {
					time.Sleep(batchDelay)
				}
				fmt.Fprintf(errOut, "%s querying minute_token=%s ...\n", logPrefix, sanitizeLogValue(token))
				results = append(results, fetchNoteByMinuteToken(ctx, runtime, token))
			}
		} else {
			instanceIDs := common.SplitCSV(runtime.Str("calendar-event-ids"))
			fmt.Fprintf(errOut, "%s querying %d calendar_event_id(s)\n", logPrefix, len(instanceIDs))
			calendarID, err := getPrimaryCalendarID(runtime)
			if err != nil {
				return err
			}
			fmt.Fprintf(errOut, "%s primary calendar: %s\n", logPrefix, calendarID)
			for i, id := range instanceIDs {
				if err := ctx.Err(); err != nil {
					return err
				}
				if i > 0 {
					time.Sleep(batchDelay)
				}
				fmt.Fprintf(errOut, "%s querying calendar_event_id=%s ...\n", logPrefix, sanitizeLogValue(id))
				results = append(results, fetchNoteByCalendarEventID(ctx, runtime, id, calendarID))
			}
		}

		// count results
		successCount := 0
		for _, r := range results {
			m, _ := r.(map[string]any)
			if m["error"] == nil {
				successCount++
			}
		}
		fmt.Fprintf(errOut, "%s done: %d total, %d succeeded, %d failed\n", logPrefix, len(results), successCount, len(results)-successCount)

		// all failed → return structured error
		if successCount == 0 && len(results) > 0 {
			outData := map[string]any{"notes": results}
			runtime.OutFormat(outData, &output.Meta{Count: len(results)}, nil)
			return output.ErrAPI(0, fmt.Sprintf("all %d queries failed", len(results)), nil)
		}

		// output
		outData := map[string]any{"notes": results}
		runtime.OutFormat(outData, &output.Meta{Count: len(results)}, func(w io.Writer) {
			var rows []map[string]interface{}
			for _, r := range results {
				m, _ := r.(map[string]any)
				id, _ := m["meeting_id"].(string)
				if id == "" {
					id, _ = m["minute_token"].(string)
				}
				row := map[string]interface{}{"id": id}
				if errMsg, _ := m["error"].(string); errMsg != "" {
					row["status"] = "FAIL"
					row["error"] = errMsg
				} else {
					row["status"] = "OK"
					if v, _ := m["note_doc_token"].(string); v != "" {
						row["note_doc"] = v
					}
					if v, _ := m["verbatim_doc_token"].(string); v != "" {
						row["verbatim_doc"] = v
					}
					if v, _ := m["shared_doc_tokens"].([]string); len(v) > 0 {
						row["shared_docs"] = strings.Join(v, ", ")
					}
					if v, _ := m["source"].(string); v != "" {
						row["source"] = v
					}
					if v, _ := m["create_time"].(string); v != "" {
						row["create_time"] = v
					}
					if arts, _ := m["artifacts"].(map[string]any); arts != nil {
						if v, _ := arts["transcript_file"].(string); v != "" {
							row["transcript"] = v
						}
					}
				}
				rows = append(rows, row)
			}
			output.PrintTable(w, rows)
			fmt.Fprintf(w, "\n%d note(s), %d succeeded, %d failed\n", len(results), successCount, len(results)-successCount)
		})
		return nil
	},
}
