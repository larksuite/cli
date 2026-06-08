// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT
//
// vc +notes — query meeting notes
//
// Three mutually exclusive input modes (only one allowed per invocation):
//   meeting-ids:        meeting.get → note_id → note detail API
//   minute-tokens:      minutes API → note detail + AI artifacts (transcript inlined)
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
	"regexp"
	"strings"
	"time"

	"github.com/larksuite/cli/errs"
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
		"vc:note:read",
		"vc:record:readonly",
	}
	scopesMinuteTokens = []string{
		"minutes:minutes:readonly",
		"minutes:minutes.artifacts:read",
	}
	scopesCalendarEventIDs = []string{
		"calendar:calendar:read",
		"calendar:calendar.event:read",
		"vc:meeting.meetingevent:read",
		"vc:record:readonly",
	}
)

// artifact type enum from note detail API
const (
	artifactTypeMainDoc  = 1 // main note document
	artifactTypeVerbatim = 2 // verbatim transcript
)

const logPrefix = "[vc +notes]"

const (
	minutesNoReadPermissionCode = 2091005

	// recording API specific error codes (used to surface meeting minute_token state).
	recordingNotFoundCode     = 121004 // 该会议没有妙记文件
	recordingNoPermissionCode = 121005 // 非会议参与者无权查看
	recordingGeneratingCode   = 124002 // 录制/妙记文件仍在生成中

	// note detail API specific error code.
	noteNoPermissionCode = 121005 // 调用者没有该纪要的阅读权限
)

func minutesReadError(err error, minuteToken string) error {
	p, ok := errs.ProblemOf(err)
	if !ok || p.Code != minutesNoReadPermissionCode {
		return err
	}
	p.Message = fmt.Sprintf("No read permission for minute %s: cannot query the minute.", minuteToken)
	p.Hint = "Ask the minute owner for minute file read permission"
	return err
}

// validMinuteToken matches the server's minute-token format and blocks any
// user-supplied token from reaching filesystem paths unsanitized.
var validMinuteToken = regexp.MustCompile(`^[a-z0-9]+$`)

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
	data, err := runtime.CallAPITyped(http.MethodPost, "/open-apis/calendar/v4/calendars/primary", nil, nil)
	if err != nil {
		return "", err
	}
	calendars, _ := data["calendars"].([]any)
	if len(calendars) == 0 {
		return "", errs.NewValidationError(errs.SubtypeInvalidArgument, "primary calendar not found")
	}
	first, _ := calendars[0].(map[string]any)
	cal, _ := first["calendar"].(map[string]any)
	calID, _ := cal["calendar_id"].(string)
	if calID == "" {
		return "", errs.NewValidationError(errs.SubtypeInvalidArgument, "primary calendar ID is empty")
	}
	return calID, nil
}

// eventRelationInfo holds the resolved relation info from mget_instance_relation_info API.
type eventRelationInfo struct {
	MeetingIDs   []string // meeting IDs (one event may spawn multiple meetings)
	MeetingNotes []string // user-bound meeting note doc tokens
}

// resolveMeetingIDsFromCalendarEvent resolves a calendar event instance to its
// associated meeting IDs and optionally note doc tokens via the mget_instance_relation_info API.
// When needNotes is true, meeting_notes are also requested.
// Shared by +notes and +recording for the --calendar-event-ids path.
func resolveMeetingIDsFromCalendarEvent(runtime *common.RuntimeContext, instanceID string, calendarID string, needNotes bool) (*eventRelationInfo, error) {
	body := map[string]any{
		"instance_ids":              []string{instanceID},
		"need_meeting_instance_ids": true,
	}
	if needNotes {
		body["need_meeting_notes"] = true
	}
	data, err := runtime.CallAPITyped(http.MethodPost,
		fmt.Sprintf("/open-apis/calendar/v4/calendars/%s/events/mget_instance_relation_info", validate.EncodePathSegment(calendarID)),
		nil,
		body)
	if err != nil {
		return nil, err
	}

	infos, _ := data["instance_relation_infos"].([]any)
	if len(infos) == 0 {
		return nil, errs.NewValidationError(errs.SubtypeFailedPrecondition, "no event relation info found")
	}
	info, _ := infos[0].(map[string]any)

	rawIDs, _ := info["meeting_instance_ids"].([]any)
	if len(rawIDs) == 0 {
		return nil, errs.NewValidationError(errs.SubtypeFailedPrecondition, "no associated video meeting for this event")
	}

	result := &eventRelationInfo{}
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
		result.MeetingIDs = append(result.MeetingIDs, meetingID)
	}

	result.MeetingNotes = extractStringSlice(info, "meeting_notes")

	return result, nil
}

// extractStringSlice extracts a []string from a JSON array field in a map.
func extractStringSlice(m map[string]any, key string) []string {
	raw, _ := m[key].([]any)
	var out []string
	for _, v := range raw {
		if s, ok := v.(string); ok && s != "" {
			out = append(out, s)
		}
	}
	return out
}

// fetchNoteByCalendarEventID queries notes via calendar event instance ID.
// Two sources of doc tokens are collected and deduplicated:
//   - mget_instance_relation_info: meeting_notes (user-bound note doc tokens)
//   - meeting_id chain: meeting.get → note detail (note_doc_token, verbatim_doc_token, shared_doc_tokens)
func fetchNoteByCalendarEventID(ctx context.Context, runtime *common.RuntimeContext, instanceID string, calendarID string) map[string]any {
	errOut := runtime.IO().ErrOut

	relInfo, err := resolveMeetingIDsFromCalendarEvent(runtime, instanceID, calendarID, true)
	if err != nil {
		return map[string]any{"calendar_event_id": instanceID, "error": err.Error()}
	}

	result := map[string]any{"calendar_event_id": instanceID}

	// source 1: user-bound meeting note doc tokens from mget_instance_relation_info
	if len(relInfo.MeetingNotes) > 0 {
		result["meeting_notes"] = relInfo.MeetingNotes
	}

	// source 2: meeting_id → meeting.get → note detail (for shared_doc_tokens etc.)
	if len(relInfo.MeetingIDs) > 1 {
		fmt.Fprintf(errOut, "%s event %s has %d meetings, trying each\n", logPrefix, sanitizeLogValue(instanceID), len(relInfo.MeetingIDs))
	}

	for _, meetingID := range relInfo.MeetingIDs {
		fmt.Fprintf(errOut, "%s event %s → meeting_id=%s\n", logPrefix, sanitizeLogValue(instanceID), sanitizeLogValue(meetingID))
		noteResult := fetchNoteByMeetingID(ctx, runtime, meetingID)
		// success means note detail was retrieved, regardless of whether the
		// recording API (minute_token) call succeeded — minute_token failures
		// surface as part of the merged `error` string for downstream visibility.
		if _, ok := noteResult["note_doc_token"].(string); ok {
			for k, v := range noteResult {
				result[k] = v
			}
			deduplicateDocTokens(result)
			return result
		}
		fmt.Fprintf(errOut, "%s meeting_id=%s: %s, trying next\n", logPrefix, sanitizeLogValue(meetingID), noteResult["error"])
	}

	// meeting chain failed, but still succeed if relation info returned note tokens
	if len(relInfo.MeetingNotes) > 0 {
		return result
	}
	result["error"] = "no notes found in any associated meeting"
	return result
}

// deduplicateDocTokens removes meeting_notes entries that duplicate note detail fields.
func deduplicateDocTokens(result map[string]any) {
	seen := map[string]bool{}
	if v, _ := result["note_doc_token"].(string); v != "" {
		seen[v] = true
	}
	if v, _ := result["verbatim_doc_token"].(string); v != "" {
		seen[v] = true
	}
	for _, tok := range asStringSlice(result["shared_doc_tokens"]) {
		seen[tok] = true
	}

	var filtered []string
	for _, tok := range asStringSlice(result["meeting_notes"]) {
		if !seen[tok] {
			filtered = append(filtered, tok)
		}
	}
	if len(filtered) > 0 {
		result["meeting_notes"] = filtered
	} else {
		delete(result, "meeting_notes")
	}
}

// asStringSlice casts v to []string; returns nil for non-[]string or nil values.
func asStringSlice(v any) []string {
	ss, _ := v.([]string)
	return ss
}

// fetchMeetingMinuteToken queries the recording API of a meeting and returns
// the associated minute_token (parsed from the recording URL) and an
// optional human-friendly error message. On success token is non-empty and
// errMsg is empty; on failure token is empty and errMsg describes the cause:
//   - 121004: meeting has no minute file
//   - 121005: caller has no permission for the meeting recording
//   - 124002: recording / minute file is still being generated
//
// Other failures fall back to the raw API error description so Agents can
// still parse the underlying cause.
func fetchMeetingMinuteToken(runtime *common.RuntimeContext, meetingID string) (token, errMsg string) {
	data, err := runtime.CallAPITyped(http.MethodGet,
		fmt.Sprintf("/open-apis/vc/v1/meetings/%s/recording", validate.EncodePathSegment(meetingID)),
		nil, nil)
	if err != nil {
		if p, ok := errs.ProblemOf(err); ok {
			switch p.Code {
			case recordingNotFoundCode:
				return "", "no minute file for this meeting"
			case recordingNoPermissionCode:
				return "", "no permission to access this meeting's minute; ask the meeting owner to share the minute"
			case recordingGeneratingCode:
				return "", "minute file is still being generated; please retry later"
			}
		}
		return "", fmt.Sprintf("failed to query recording: %v", err)
	}

	recording, _ := data["recording"].(map[string]any)
	if recording == nil {
		return "", "no recording available for this meeting"
	}
	recordingURL, _ := recording["url"].(string)
	if t := extractMinuteToken(recordingURL); t != "" {
		return t, ""
	}
	return "", "no minute_token found in recording URL"
}

// fetchNoteByMeetingID queries notes via meeting_id and additionally fetches
// the meeting's minute_token via the recording API. The two paths are queried
// independently; their failures are merged into a single `error` field
// (semicolon-separated) so Agents always see all causes at once. The
// `minute_token` field is only populated on success.
func fetchNoteByMeetingID(ctx context.Context, runtime *common.RuntimeContext, meetingID string) map[string]any {
	data, err := runtime.CallAPITyped(http.MethodGet, fmt.Sprintf("/open-apis/vc/v1/meetings/%s", validate.EncodePathSegment(meetingID)),
		map[string]interface{}{"with_participants": "false", "query_mode": "0"}, nil)
	if err != nil {
		return map[string]any{"meeting_id": meetingID, "error": fmt.Sprintf("failed to query meeting: %v", err)}
	}

	meeting, _ := data["meeting"].(map[string]any)
	if meeting == nil {
		return map[string]any{"meeting_id": meetingID, "error": "meeting not found"}
	}

	// Always attempt to query the meeting's minute_token via the recording API,
	// regardless of whether the meeting has a note_id, so callers always see
	// minute state for follow-up calls (e.g. `vc +notes --minute-tokens=...`).
	minuteToken, minuteErr := fetchMeetingMinuteToken(runtime, meetingID)

	var result map[string]any
	var noteErr string
	if noteID, _ := meeting["note_id"].(string); noteID != "" {
		result = fetchNoteDetail(ctx, runtime, noteID)
		if msg, _ := result["error"].(string); msg != "" {
			noteErr = msg
			delete(result, "error")
		}
	} else {
		result = map[string]any{}
		noteErr = "no notes available for this meeting"
	}

	result["meeting_id"] = meetingID
	if minuteToken != "" {
		result["minute_token"] = minuteToken
	}
	if combined := joinErrors(noteErr, minuteErr); combined != "" {
		result["error"] = combined
	}
	return result
}

// joinErrors merges multiple non-empty error messages with "; " so Agents can
// see all causes at once when both note and minute paths fail.
func joinErrors(msgs ...string) string {
	parts := make([]string, 0, len(msgs))
	for _, m := range msgs {
		if m != "" {
			parts = append(parts, m)
		}
	}
	return strings.Join(parts, "; ")
}

// hasNotesPayload reports whether a result map carries any usable note or
// minute payload, irrespective of partial failures surfaced via `error`.
func hasNotesPayload(m map[string]any) bool {
	if m == nil {
		return false
	}
	for _, k := range []string{"note_doc_token", "verbatim_doc_token", "minute_token", "meeting_notes", "shared_doc_tokens", "artifacts"} {
		if v, ok := m[k]; ok && v != nil && v != "" {
			return true
		}
	}
	return false
}

// fetchNoteByMinuteToken queries notes via minute_token.
// Fetches both note detail (doc tokens) and AI artifacts (summary/todos/chapters inline +
// transcript to file) independently, merging into a single result map for Agent consumption.
func fetchNoteByMinuteToken(ctx context.Context, runtime *common.RuntimeContext, minuteToken string) map[string]any {
	errOut := runtime.IO().ErrOut

	data, err := runtime.CallAPITyped(http.MethodGet, fmt.Sprintf("/open-apis/minutes/v1/minutes/%s", validate.EncodePathSegment(minuteToken)), nil, nil)
	if err != nil {
		err = minutesReadError(err, minuteToken)
		result := map[string]any{"minute_token": minuteToken, "error": err.Error()}
		if p, ok := errs.ProblemOf(err); ok && p.Hint != "" {
			result["hint"] = p.Hint
		}
		return result
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

	// AI artifacts + transcript come from the same /artifacts endpoint.
	artifacts := map[string]any{}
	fetchInlineArtifacts(runtime, minuteToken, title, artifacts)
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

// fetchInlineArtifacts fetches summary/todos/chapters/keywords and transcript from the
// /artifacts API, persists transcript to disk, and exposes the path as transcript_file.
func fetchInlineArtifacts(runtime *common.RuntimeContext, minuteToken string, title string, result map[string]any) {
	errOut := runtime.IO().ErrOut
	fmt.Fprintf(errOut, "%s fetching AI artifacts...\n", logPrefix)
	data, err := runtime.CallAPITyped(http.MethodGet, fmt.Sprintf("/open-apis/minutes/v1/minutes/%s/artifacts", validate.EncodePathSegment(minuteToken)), nil, nil)
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
	if keywords, ok := data["keywords"].([]any); ok && len(keywords) > 0 {
		result["keywords"] = keywords
	}
	if transcript, ok := data["transcript"].(string); ok && transcript != "" {
		if path := saveTranscriptToFile(runtime, minuteToken, title, []byte(transcript)); path != "" {
			result["transcript_file"] = path
		}
	}
}

// saveTranscriptToFile persists transcript bytes to the canonical artifact path
// for the given minute_token. Returns the file path on success (or when the
// file already exists and --overwrite is not set), empty string on any failure.
func saveTranscriptToFile(runtime *common.RuntimeContext, minuteToken, title string, content []byte) string {
	errOut := runtime.IO().ErrOut

	// With no --output-dir the default layout shares the directory with
	// `minutes +download`. Legacy layout is preserved when the flag is set.
	var dirName string
	if outDir := runtime.Str("output-dir"); outDir != "" {
		dirName = filepath.Join(outDir, sanitizeDirName(title, minuteToken))
	} else {
		dirName = common.DefaultMinuteArtifactDir(minuteToken)
	}
	transcriptPath := filepath.Join(dirName, common.DefaultTranscriptFileName)

	if !runtime.Bool("overwrite") {
		if _, statErr := runtime.FileIO().Stat(transcriptPath); statErr == nil {
			fmt.Fprintf(errOut, "%s transcript already exists: %s (use --overwrite to replace)\n", logPrefix, transcriptPath)
			return transcriptPath
		}
	}

	fmt.Fprintf(errOut, "%s writing transcript: %s\n", logPrefix, transcriptPath)
	if _, err := runtime.FileIO().Save(transcriptPath, fileio.SaveOptions{}, bytes.NewReader(content)); err != nil {
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
	data, err := runtime.CallAPITyped(http.MethodGet, fmt.Sprintf("/open-apis/vc/v1/notes/%s", validate.EncodePathSegment(noteID)), nil, nil)
	if err != nil {
		if p, ok := errs.ProblemOf(err); ok && p.Code == noteNoPermissionCode {
			return map[string]any{"error": fmt.Sprintf("[%v]: no read permission for this meeting note", p.Code)}
		}
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
		{Name: "output-dir", Desc: "output directory for artifact files (default: ./minutes/{minute_token}/)"},
		{Name: "overwrite", Type: "bool", Desc: "overwrite existing artifact files"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if err := common.ExactlyOneTyped(runtime, "meeting-ids", "minute-tokens", "calendar-event-ids"); err != nil {
			return err
		}
		// batch input size limit
		const maxBatchSize = 50
		for _, flag := range []string{"meeting-ids", "minute-tokens", "calendar-event-ids"} {
			if v := runtime.Str(flag); v != "" {
				if ids := common.SplitCSV(v); len(ids) > maxBatchSize {
					return errs.NewValidationError(errs.SubtypeInvalidArgument, "--%s: too many IDs (%d), maximum is %d", flag, len(ids), maxBatchSize).WithParam("--" + flag)
				}
			}
		}
		if outDir := runtime.Str("output-dir"); outDir != "" {
			if err := common.ValidateSafePathTyped(runtime.FileIO(), outDir); err != nil {
				return err
			}
		}
		// Reject malformed minute tokens before they flow into filesystem paths.
		if v := runtime.Str("minute-tokens"); v != "" {
			for _, token := range common.SplitCSV(v) {
				if !validMinuteToken.MatchString(token) {
					return errs.NewValidationError(errs.SubtypeInvalidArgument, "invalid minute token %q: must contain only lowercase alphanumeric characters", token).WithParam("--minute-tokens")
				}
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
		if ids := runtime.Str("meeting-ids"); ids != "" {
			return common.NewDryRunAPI().
				GET("/open-apis/vc/v1/meetings/{meeting_id}").
				GET("/open-apis/vc/v1/notes/{note_id}").
				GET("/open-apis/vc/v1/meetings/{meeting_id}/recording").
				Set("meeting_ids", common.SplitCSV(ids)).
				Set("steps", "meeting.get → note_id → note detail API + recording API → minute_token")
		}
		if tokens := runtime.Str("minute-tokens"); tokens != "" {
			return common.NewDryRunAPI().
				GET("/open-apis/minutes/v1/minutes/{minute_token}").
				GET("/open-apis/vc/v1/notes/{note_id}").
				GET("/open-apis/minutes/v1/minutes/{minute_token}/artifacts").
				Set("minute_tokens", common.SplitCSV(tokens)).
				Set("steps", "minutes API → note detail + AI artifacts (incl. transcript)")
		}
		ids := runtime.Str("calendar-event-ids")
		return common.NewDryRunAPI().
			POST("/open-apis/calendar/v4/calendars/primary").
			POST("/open-apis/calendar/v4/calendars/{calendar_id}/events/mget_instance_relation_info").
			GET("/open-apis/vc/v1/meetings/{meeting_id}").
			GET("/open-apis/vc/v1/notes/{note_id}").
			GET("/open-apis/vc/v1/meetings/{meeting_id}/recording").
			Set("calendar_event_ids", common.SplitCSV(ids)).
			Set("steps", "primary calendar → mget_instance_relation_info → meeting_id → meeting.get → note detail API + recording API → minute_token")
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

		// count results: a result counts as "successful" when it carries any
		// note/minute payload, even if the merged `error` field surfaces a
		// partial failure (e.g. note ok but minute_token lookup failed).
		successCount := 0
		for _, r := range results {
			m, _ := r.(map[string]any)
			if hasNotesPayload(m) {
				successCount++
			}
		}
		fmt.Fprintf(errOut, "%s done: %d total, %d succeeded, %d failed\n", logPrefix, len(results), successCount, len(results)-successCount)

		// all failed → return structured error
		if successCount == 0 && len(results) > 0 {
			return runtime.OutPartialFailure(map[string]any{"notes": results}, &output.Meta{Count: len(results)})
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
				if id == "" {
					id, _ = m["calendar_event_id"].(string)
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
					if v := asStringSlice(m["meeting_notes"]); len(v) > 0 {
						row["meeting_notes"] = strings.Join(v, ", ")
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
