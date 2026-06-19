// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package vc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/larksuite/cli/shortcuts/note"
)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

var warmOnce sync.Once

func warmTokenCache(t *testing.T) {
	t.Helper()
	warmOnce.Do(func() {
		f, _, _, reg := cmdutil.TestFactory(t, defaultConfig())
		reg.Register(&httpmock.Stub{
			URL:  "/open-apis/test/v1/warm",
			Body: map[string]interface{}{"code": 0, "msg": "ok", "data": map[string]interface{}{}},
		})
		s := common.Shortcut{
			Service:   "test",
			Command:   "+warm",
			AuthTypes: []string{"bot"},
			Execute: func(_ context.Context, rctx *common.RuntimeContext) error {
				_, err := rctx.CallAPITyped("GET", "/open-apis/test/v1/warm", nil, nil)
				return err
			},
		}
		parent := &cobra.Command{Use: "test"}
		s.Mount(parent, f)
		parent.SetArgs([]string{"+warm"})
		parent.SilenceErrors = true
		parent.SilenceUsage = true
		parent.Execute()
	})
}

func mountAndRun(t *testing.T, s common.Shortcut, args []string, f *cmdutil.Factory, stdout *bytes.Buffer) error {
	t.Helper()
	warmTokenCache(t)
	parent := &cobra.Command{Use: "vc"}
	s.Mount(parent, f)
	parent.SetArgs(args)
	parent.SilenceErrors = true
	parent.SilenceUsage = true
	if stdout != nil {
		stdout.Reset()
	}
	return parent.Execute()
}

func defaultConfig() *core.CliConfig {
	return &core.CliConfig{
		AppID: "test-app", AppSecret: "test-secret", Brand: core.BrandFeishu,
		UserOpenId: "ou_testuser",
	}
}

func meetingGetStub(meetingID, noteID string) *httpmock.Stub {
	meeting := map[string]interface{}{
		"id":    meetingID,
		"topic": "Test Meeting",
	}
	if noteID != "" {
		meeting["note_id"] = noteID
	}
	return &httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/vc/v1/meetings/" + meetingID,
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{"meeting": meeting},
		},
	}
}

func noteDetailStub(noteID string) *httpmock.Stub {
	return &httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/vc/v1/notes/" + noteID,
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"note": map[string]interface{}{
					"creator_id":  "ou_creator",
					"create_time": "1700000000",
					"artifacts": []interface{}{
						map[string]interface{}{"doc_token": "doc_main", "artifact_type": 1},
						map[string]interface{}{"doc_token": "doc_verbatim", "artifact_type": 2},
					},
					"references": []interface{}{
						map[string]interface{}{"doc_token": "doc_shared1"},
					},
				},
			},
		},
	}
}

func noteDetailDisplayOnlyStub(noteID string, displayType int) *httpmock.Stub {
	return &httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/vc/v1/notes/" + noteID,
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"note": map[string]interface{}{
					"note_display_type": displayType,
				},
			},
		},
	}
}

func artifactsStub(token, transcript string) *httpmock.Stub {
	data := map[string]interface{}{
		"summary":         "Test summary content",
		"minute_todos":    []interface{}{map[string]interface{}{"content": "Buy milk"}},
		"minute_chapters": []interface{}{map[string]interface{}{"title": "Intro", "summary_content": "Opening"}},
		"keywords":        []interface{}{"budget", "roadmap"},
	}
	if transcript != "" {
		data["transcript"] = transcript
	}
	return &httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/minutes/v1/minutes/" + token + "/artifacts",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": data,
		},
	}
}

func minuteGetStub(token, noteID, title string) *httpmock.Stub {
	minute := map[string]interface{}{"title": title}
	if noteID != "" {
		minute["note_id"] = noteID
	}
	return &httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/minutes/v1/minutes/" + token,
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{"minute": minute},
		},
	}
}

// ---------------------------------------------------------------------------
// Unit tests for pure functions
// ---------------------------------------------------------------------------

func TestSanitizeDirName(t *testing.T) {
	tests := []struct {
		title, token, want string
	}{
		{"", "abc123", "artifact-abc123"},
		{"会议纪要", "abc", "artifact-会议纪要-abc"},
		{"a/b\\c:d", "tok", "artifact-a_b_c_d-tok"},
		{"   ", "tok", "artifact-tok"},
		{"ok title", "tok", "artifact-ok title-tok"},
		{"..hidden", "tok", "artifact-hidden-tok"},
		{"a\nb", "tok", "artifact-a_b-tok"},
	}
	for _, tt := range tests {
		got := sanitizeDirName(tt.title, tt.token)
		if got != tt.want {
			t.Errorf("sanitizeDirName(%q, %q) = %q, want %q", tt.title, tt.token, got, tt.want)
		}
	}
}

// Note-detail parsing helpers (parseArtifactType/extractArtifactTokens/
// extractDocTokens) moved to the note domain; their tests live in
// shortcuts/note/note_test.go.

// ---------------------------------------------------------------------------
// Integration tests: +notes with mocked HTTP
// ---------------------------------------------------------------------------

func TestNotes_Validation_ExactlyOne(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, defaultConfig())

	err := mountAndRun(t, VCNotes, []string{"+notes", "--as", "user"}, f, nil)
	if err == nil {
		t.Fatal("expected validation error for no flags")
	}
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected *errs.ValidationError, got %T: %v", err, err)
	}
	if ve.Subtype != errs.SubtypeInvalidArgument {
		t.Fatalf("Subtype = %q, want SubtypeInvalidArgument", ve.Subtype)
	}

	err = mountAndRun(t, VCNotes, []string{"+notes", "--meeting-ids", "m1", "--minute-tokens", "t1", "--as", "user"}, f, nil)
	if err == nil {
		t.Fatal("expected validation error for two flags")
	}
	ve = nil
	if !errors.As(err, &ve) {
		t.Fatalf("expected *errs.ValidationError, got %T: %v", err, err)
	}
	if ve.Subtype != errs.SubtypeInvalidArgument {
		t.Fatalf("Subtype = %q, want SubtypeInvalidArgument", ve.Subtype)
	}
}

func TestNotes_DryRun_MeetingIDs(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, defaultConfig())
	err := mountAndRun(t, VCNotes, []string{"+notes", "--meeting-ids", "m001", "--dry-run", "--as", "user"}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), "meeting.get") {
		t.Errorf("dry-run should show meeting.get step, got: %s", stdout.String())
	}
}

func TestNotes_DryRun_MinuteTokens(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, defaultConfig())
	err := mountAndRun(t, VCNotes, []string{"+notes", "--minute-tokens", "tok001", "--dry-run", "--as", "user"}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), "minutes API") {
		t.Errorf("dry-run should show minutes API step, got: %s", stdout.String())
	}
}

func TestNotes_DryRun_CalendarEventIDs(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, defaultConfig())
	err := mountAndRun(t, VCNotes, []string{"+notes", "--calendar-event-ids", "evt001", "--dry-run", "--as", "user"}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), "mget_instance_relation_info") {
		t.Errorf("dry-run should show mget step, got: %s", stdout.String())
	}
}

// ---------------------------------------------------------------------------
// Additional unit tests for coverage
// ---------------------------------------------------------------------------

func TestSanitizeDirName_Truncate(t *testing.T) {
	long := strings.Repeat("a", 300)
	got := sanitizeDirName(long, "tok")
	if len(got) > 250 { // artifact- prefix + 200 chars + - + tok
		t.Errorf("expected truncated dir name, got len=%d", len(got))
	}
	if !strings.Contains(got, "tok") {
		t.Errorf("expected minute_token in dir name, got %q", got)
	}
}

func TestSanitizeDirName_LeadingDots(t *testing.T) {
	got := sanitizeDirName("...hidden", "tok")
	if strings.Contains(got, "artifact-...") {
		t.Errorf("expected dots stripped, got %q", got)
	}
}

func TestSanitizeLogValue(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"normal", "normal"},
		{"line1\nline2", "line1 line2"},
		{"has\rCR", "has CR"},
		{"ansi\x1b[31mred\x1b[0m", "ansired"},
		{"", ""},
	}
	for _, tt := range tests {
		got := sanitizeLogValue(tt.input)
		if got != tt.want {
			t.Errorf("sanitizeLogValue(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestNotes_BatchLimit(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, defaultConfig())
	// generate 51 IDs (over limit of 50)
	ids := make([]string, 51)
	for i := range ids {
		ids[i] = fmt.Sprintf("m%d", i)
	}
	err := mountAndRun(t, VCNotes, []string{"+notes", "--meeting-ids", strings.Join(ids, ","), "--as", "user"}, f, nil)
	if err == nil {
		t.Fatal("expected batch limit error")
	}
	if !strings.Contains(err.Error(), "too many IDs") {
		t.Errorf("expected 'too many IDs' error, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Unit tests for new calendar-to-notes functions
// ---------------------------------------------------------------------------

func TestExtractStringSlice(t *testing.T) {
	m := map[string]any{
		"tokens":  []any{"a", "b", "", "c"},
		"empty":   []any{},
		"missing": nil,
		"mixed":   []any{"x", float64(123), nil, "y"},
	}
	if got := extractStringSlice(m, "tokens"); len(got) != 3 || got[0] != "a" || got[1] != "b" || got[2] != "c" {
		t.Errorf("tokens: got %v, want [a b c]", got)
	}
	if got := extractStringSlice(m, "empty"); got != nil {
		t.Errorf("empty: got %v, want nil", got)
	}
	if got := extractStringSlice(m, "missing"); got != nil {
		t.Errorf("missing: got %v, want nil", got)
	}
	if got := extractStringSlice(m, "nonexistent"); got != nil {
		t.Errorf("nonexistent: got %v, want nil", got)
	}
	if got := extractStringSlice(m, "mixed"); len(got) != 2 || got[0] != "x" || got[1] != "y" {
		t.Errorf("mixed: got %v, want [x y]", got)
	}
}

func TestAsStringSlice(t *testing.T) {
	if got := asStringSlice(nil); got != nil {
		t.Errorf("nil: got %v, want nil", got)
	}
	if got := asStringSlice([]string{"a", "b"}); len(got) != 2 || got[0] != "a" {
		t.Errorf("[]string: got %v", got)
	}
	if got := asStringSlice("not a slice"); got != nil {
		t.Errorf("string: got %v, want nil", got)
	}
}

func TestDeduplicateDocTokens(t *testing.T) {
	// case 1: meeting_notes overlap with note_doc_token
	result := map[string]any{
		"note_doc_token":     "doc_main",
		"verbatim_doc_token": "doc_verb",
		"shared_doc_tokens":  []string{"doc_shared"},
		"meeting_notes":      []string{"doc_main", "unique_note"},
	}
	deduplicateDocTokens(result)
	mn := asStringSlice(result["meeting_notes"])
	if len(mn) != 1 || mn[0] != "unique_note" {
		t.Errorf("meeting_notes: got %v, want [unique_note]", mn)
	}

	// case 2: no overlap
	result2 := map[string]any{
		"note_doc_token": "doc_a",
		"meeting_notes":  []string{"doc_b"},
	}
	deduplicateDocTokens(result2)
	mn2 := asStringSlice(result2["meeting_notes"])
	if len(mn2) != 1 || mn2[0] != "doc_b" {
		t.Errorf("no overlap: got %v, want [doc_b]", mn2)
	}

	// case 3: empty meeting_notes
	result3 := map[string]any{
		"note_doc_token": "doc_a",
	}
	deduplicateDocTokens(result3)
	if _, exists := result3["meeting_notes"]; exists {
		t.Errorf("should not have meeting_notes key")
	}

	// case 4: all meeting_notes are duplicates
	result4 := map[string]any{
		"note_doc_token":    "doc_a",
		"shared_doc_tokens": []string{"doc_b"},
		"meeting_notes":     []string{"doc_a", "doc_b"},
	}
	deduplicateDocTokens(result4)
	if _, exists := result4["meeting_notes"]; exists {
		t.Errorf("case4: meeting_notes should be removed (all duplicates), got %v", result4["meeting_notes"])
	}
}

// ---------------------------------------------------------------------------
// Integration: calendar-event-ids path with meeting_notes + dedup
// ---------------------------------------------------------------------------

func calendarRelationStub(calendarID, instanceID string, meetingIDs []string, meetingNotes []string) *httpmock.Stub {
	infos := map[string]interface{}{
		"instance_id": instanceID,
	}
	mIDs := make([]interface{}, len(meetingIDs))
	for i, id := range meetingIDs {
		mIDs[i] = id
	}
	infos["meeting_instance_ids"] = mIDs
	if len(meetingNotes) > 0 {
		notes := make([]interface{}, len(meetingNotes))
		for i, n := range meetingNotes {
			notes[i] = n
		}
		infos["meeting_notes"] = notes
	}
	return &httpmock.Stub{
		Method: "POST",
		URL:    fmt.Sprintf("/open-apis/calendar/v4/calendars/%s/events/mget_instance_relation_info", calendarID),
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"instance_relation_infos": []interface{}{infos},
			},
		},
	}
}

func primaryCalendarStub(calendarID string) *httpmock.Stub {
	return &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/calendar/v4/calendars/primary",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"calendars": []interface{}{
					map[string]interface{}{
						"calendar": map[string]interface{}{
							"calendar_id": calendarID,
						},
					},
				},
			},
		},
	}
}

func TestNotes_CalendarPath_MeetingNotesDedup(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())

	calID := "cal_test"
	reg.Register(primaryCalendarStub(calID))
	// mget returns meeting_notes=["doc_main","unique_note"], doc_main overlaps with note_doc_token
	reg.Register(calendarRelationStub(calID, "evt_001", []string{"m001"}, []string{"doc_main", "unique_note"}))
	reg.Register(meetingGetStub("m001", "note_001"))
	reg.Register(noteDetailStub("note_001"))

	err := mountAndRun(t, VCNotes, []string{"+notes", "--calendar-event-ids", "evt_001", "--as", "user"}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var resp map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse output: %v", err)
	}
	data, _ := resp["data"].(map[string]any)
	notes, _ := data["notes"].([]any)
	if len(notes) != 1 {
		t.Fatalf("expected 1 note, got %d", len(notes))
	}
	note, _ := notes[0].(map[string]any)

	// doc_main should be deduplicated (exists in note_doc_token)
	// only "unique_note" should remain in meeting_notes
	mn, _ := note["meeting_notes"].([]any)
	if len(mn) != 1 {
		t.Fatalf("meeting_notes: expected 1 after dedup, got %d: %v", len(mn), mn)
	}
	if mn[0] != "unique_note" {
		t.Errorf("meeting_notes[0] = %v, want unique_note", mn[0])
	}
}

func TestNotes_CalendarPath_FallbackWhenMeetingChainFails(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())

	calID := "cal_test"
	reg.Register(primaryCalendarStub(calID))
	// mget returns note tokens but meeting chain will fail
	reg.Register(calendarRelationStub(calID, "evt_002", []string{"m_bad"}, []string{"fallback_note"}))
	// meeting.get returns error
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/vc/v1/meetings/m_bad",
		Body:   map[string]interface{}{"code": 121004, "msg": "data not found"},
	})

	err := mountAndRun(t, VCNotes, []string{"+notes", "--calendar-event-ids", "evt_002", "--as", "user"}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var resp map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse output: %v", err)
	}
	data, _ := resp["data"].(map[string]any)
	notes, _ := data["notes"].([]any)
	if len(notes) != 1 {
		t.Fatalf("expected 1 note, got %d", len(notes))
	}
	note, _ := notes[0].(map[string]any)

	// should succeed via fallback (meeting chain failed but mget had tokens)
	if _, hasErr := note["error"]; hasErr {
		t.Errorf("expected no error (fallback), got error: %v", note["error"])
	}
	mn, _ := note["meeting_notes"].([]any)
	if len(mn) != 1 || mn[0] != "fallback_note" {
		t.Errorf("meeting_notes: got %v, want [fallback_note]", mn)
	}
}

func TestNotes_CalendarPath_KeepsNoteIDOnlyDetail(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())

	calID := "cal_test"
	reg.Register(primaryCalendarStub(calID))
	reg.Register(calendarRelationStub(calID, "evt_note_only", []string{"m_note_only"}, nil))
	reg.Register(meetingGetStub("m_note_only", "note_only"))
	reg.Register(noteDetailDisplayOnlyStub("note_only", 2))
	reg.Register(recordingErrStub("m_note_only", 121004, "not found"))

	err := mountAndRun(t, VCNotes, []string{"+notes", "--calendar-event-ids", "evt_note_only", "--as", "user"}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	note := extractFirstNote(t, stdout)
	if got := note["note_id"]; got != "note_only" {
		t.Fatalf("note_id = %v, want note_only; note=%#v", got, note)
	}
	if got := note["note_display_type"]; got != "unified" {
		t.Fatalf("note_display_type = %v, want unified; note=%#v", got, note)
	}
	if got := note["calendar_event_id"]; got != "evt_note_only" {
		t.Fatalf("calendar_event_id = %v, want evt_note_only; note=%#v", got, note)
	}
}

func TestNotes_CalendarPath_NeedNotes_RequestBody(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, defaultConfig())
	warmTokenCache(t)

	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/calendar/v4/calendars/cal_001/events/mget_instance_relation_info",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"instance_relation_infos": []interface{}{
					map[string]interface{}{
						"meeting_instance_ids": []interface{}{"m001"},
					},
				},
			},
		},
	}
	reg.Register(stub)

	s := common.Shortcut{
		Service:   "test",
		Command:   "+need-notes-test",
		AuthTypes: []string{"bot"},
		Execute: func(_ context.Context, rctx *common.RuntimeContext) error {
			_, err := resolveMeetingIDsFromCalendarEvent(rctx, "evt_001", "cal_001", true)
			return err
		},
	}
	parent := &cobra.Command{Use: "vc"}
	s.Mount(parent, f)
	parent.SetArgs([]string{"+need-notes-test"})
	parent.SilenceErrors = true
	parent.SilenceUsage = true
	if err := parent.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(stub.CapturedBody) == 0 {
		t.Fatal("request body was not captured")
	}
	var body map[string]any
	if err := json.Unmarshal(stub.CapturedBody, &body); err != nil {
		t.Fatalf("failed to parse captured body: %v", err)
	}
	if v, ok := body["need_meeting_notes"]; !ok || v != true {
		t.Errorf("need_meeting_notes: got %v, want true", v)
	}
	if _, ok := body["need_ai_meeting_notes"]; ok {
		t.Errorf("need_ai_meeting_notes should not be requested")
	}
}

func TestNotes_TableOutputIncludesNoteRoutingFields(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())

	reg.Register(meetingGetStub("m_table", "note_table"))
	reg.Register(noteDetailDisplayOnlyStub("note_table", 2))
	reg.Register(recordingErrStub("m_table", 121004, "not found"))

	err := mountAndRun(t, VCNotes, []string{"+notes", "--meeting-ids", "m_table", "--format", "table", "--as", "user"}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "note_table") {
		t.Fatalf("table output missing note_id:\n%s", out)
	}
	if !strings.Contains(out, "unified") {
		t.Fatalf("table output missing note_display_type:\n%s", out)
	}
}

// ---------------------------------------------------------------------------
// Transcript path layout tests (unified ./minutes/{token}/ default)
// ---------------------------------------------------------------------------

// chdirForTest switches cwd to a temp dir for the test; restored on cleanup.
func chdirForTest(t *testing.T) string {
	t.Helper()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { os.Chdir(orig) })
	return dir
}

func TestNotes_TranscriptDefaultLayout(t *testing.T) {
	chdirForTest(t)

	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(minuteGetStub("tok001", "", "Meeting Title"))
	reg.Register(artifactsStub("tok001", "speaker1: hello world\n"))

	err := mountAndRun(t, VCNotes, []string{
		"+notes", "--minute-tokens", "tok001", "--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wantPath := "minutes/tok001/transcript.txt"
	data, err := os.ReadFile(wantPath)
	if err != nil {
		t.Fatalf("expected file at %s: %v", wantPath, err)
	}
	if string(data) != "speaker1: hello world\n" {
		t.Errorf("content mismatch: %q", string(data))
	}

	if _, err := os.Stat("artifact-Meeting Title-tok001"); err == nil {
		t.Errorf("legacy artifact dir should not appear under default layout")
	}
}

func TestNotes_TranscriptExplicitOutputDir_PreservesLegacyLayout(t *testing.T) {
	chdirForTest(t)

	f, _, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(minuteGetStub("tok001", "", "Meeting Title"))
	reg.Register(artifactsStub("tok001", "content"))

	if err := os.MkdirAll("out", 0755); err != nil {
		t.Fatalf("setup: %v", err)
	}

	err := mountAndRun(t, VCNotes, []string{
		"+notes", "--minute-tokens", "tok001", "--output-dir", "out", "--as", "user",
	}, f, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wantPath := filepath.Join("out", "artifact-Meeting Title-tok001", "transcript.txt")
	if _, err := os.Stat(wantPath); err != nil {
		t.Errorf("expected legacy path %s preserved, got err: %v", wantPath, err)
	}
	if _, err := os.Stat("minutes"); err == nil {
		t.Errorf("minutes/ should not be created when --output-dir is explicit")
	}
}

// ---------------------------------------------------------------------------
// Tests for joinErrors / hasNotesPayload (pure helpers)
// ---------------------------------------------------------------------------

func TestJoinErrors(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want string
	}{
		{"all empty", []string{"", "", ""}, ""},
		{"single", []string{"only"}, "only"},
		{"two non-empty", []string{"a", "b"}, "a; b"},
		{"skip empties", []string{"", "a", "", "b", ""}, "a; b"},
		{"three", []string{"x", "y", "z"}, "x; y; z"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := joinErrors(tt.in...); got != tt.want {
				t.Errorf("joinErrors(%v) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestHasNotesPayload(t *testing.T) {
	tests := []struct {
		name string
		in   map[string]any
		want bool
	}{
		{"nil", nil, false},
		{"empty", map[string]any{}, false},
		{"only meta", map[string]any{"meeting_id": "m1", "error": "fail"}, false},
		{"empty values", map[string]any{"note_doc_token": "", "minute_token": "", "note_id": ""}, false},
		{"only note_id", map[string]any{"note_id": "note1"}, true},
		{"note_id with display type", map[string]any{"note_id": "note1", "note_display_type": "unified", "note_doc_token": ""}, true},
		{"has note_doc_token", map[string]any{"note_doc_token": "doc1"}, true},
		{"has verbatim_doc_token", map[string]any{"verbatim_doc_token": "v1"}, true},
		{"has minute_token", map[string]any{"minute_token": "obc"}, true},
		{"has meeting_notes", map[string]any{"meeting_notes": []string{"d1"}}, true},
		{"has shared_doc_tokens", map[string]any{"shared_doc_tokens": []string{"s1"}}, true},
		{"has artifacts", map[string]any{"artifacts": map[string]any{"summary": "s"}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hasNotesPayload(tt.in); got != tt.want {
				t.Errorf("hasNotesPayload(%v) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Tests for fetchMeetingMinuteToken — recording API → minute_token mapping
// ---------------------------------------------------------------------------

// recordingStub is a small helper for shaping `/v1/meetings/{id}/recording` responses.
func recordingStub(meetingID string, body map[string]any) *httpmock.Stub {
	return &httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/vc/v1/meetings/" + meetingID + "/recording",
		Body:   body,
	}
}

func recordingErrStub(meetingID string, code int, msg string) *httpmock.Stub {
	return recordingStub(meetingID, map[string]any{"code": code, "msg": msg})
}

func recordingOKStub(meetingID, url string) *httpmock.Stub {
	return recordingStub(meetingID, map[string]any{
		"code": 0, "msg": "ok",
		"data": map[string]any{
			"recording": map[string]any{"url": url},
		},
	})
}

func TestFetchMeetingMinuteToken_Success(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, _, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(recordingOKStub("m_ok", "https://meetings.feishu.cn/minutes/obctoken_ok"))

	if err := botExec(t, "fmmt-ok", f, func(_ context.Context, rctx *common.RuntimeContext) error {
		token, msg := fetchMeetingMinuteToken(rctx, "m_ok")
		if token != "obctoken_ok" {
			t.Errorf("token = %q, want obctoken_ok", token)
		}
		if msg != "" {
			t.Errorf("errMsg = %q, want empty", msg)
		}
		return nil
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFetchMeetingMinuteToken_KnownErrorCodes(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	cases := []struct {
		name      string
		meetingID string
		code      int
		wantMsg   string
	}{
		{"121004 not found", "m_121004", 121004, "no minute file for this meeting"},
		{"121005 no permission", "m_121005", 121005, "no permission to access this meeting's minute"},
		{"124002 generating", "m_124002", 124002, "minute file is still being generated"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			f, _, _, reg := cmdutil.TestFactory(t, defaultConfig())
			reg.Register(recordingErrStub(tt.meetingID, tt.code, "err"))

			if err := botExec(t, "fmmt-"+tt.meetingID, f, func(_ context.Context, rctx *common.RuntimeContext) error {
				token, msg := fetchMeetingMinuteToken(rctx, tt.meetingID)
				if token != "" {
					t.Errorf("token = %q, want empty on error", token)
				}
				if !strings.Contains(msg, tt.wantMsg) {
					t.Errorf("errMsg = %q, want contains %q", msg, tt.wantMsg)
				}
				return nil
			}); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestFetchMeetingMinuteToken_GenericAPIError(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, _, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(recordingErrStub("m_other", 99999, "weird"))

	if err := botExec(t, "fmmt-generic", f, func(_ context.Context, rctx *common.RuntimeContext) error {
		token, msg := fetchMeetingMinuteToken(rctx, "m_other")
		if token != "" {
			t.Errorf("token = %q, want empty", token)
		}
		if !strings.Contains(msg, "failed to query recording") {
			t.Errorf("errMsg = %q, want contains 'failed to query recording'", msg)
		}
		return nil
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFetchMeetingMinuteToken_NoRecording(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, _, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(recordingStub("m_norec", map[string]any{
		"code": 0, "msg": "ok",
		"data": map[string]any{},
	}))

	if err := botExec(t, "fmmt-norec", f, func(_ context.Context, rctx *common.RuntimeContext) error {
		token, msg := fetchMeetingMinuteToken(rctx, "m_norec")
		if token != "" {
			t.Errorf("token = %q, want empty", token)
		}
		if !strings.Contains(msg, "no recording available") {
			t.Errorf("errMsg = %q, want contains 'no recording available'", msg)
		}
		return nil
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFetchMeetingMinuteToken_URLWithoutToken(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, _, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(recordingOKStub("m_notok", "https://example.com/no/minute/path"))

	if err := botExec(t, "fmmt-notok", f, func(_ context.Context, rctx *common.RuntimeContext) error {
		token, msg := fetchMeetingMinuteToken(rctx, "m_notok")
		if token != "" {
			t.Errorf("token = %q, want empty", token)
		}
		if !strings.Contains(msg, "no minute_token found") {
			t.Errorf("errMsg = %q, want contains 'no minute_token found'", msg)
		}
		return nil
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Integration: fetchNoteByMeetingID — note + minute_token combined behavior
// ---------------------------------------------------------------------------

// extractFirstNote runs +notes via --meeting-ids and returns the single result map.
func extractFirstNote(t *testing.T, stdout *bytes.Buffer) map[string]any {
	t.Helper()
	var resp map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse output: %v\n%s", err, stdout.String())
	}
	data, _ := resp["data"].(map[string]any)
	notes, _ := data["notes"].([]any)
	if len(notes) != 1 {
		t.Fatalf("expected 1 note, got %d (%v)", len(notes), notes)
	}
	note, _ := notes[0].(map[string]any)
	return note
}

// assertNoteError verifies the result map's `error` field contains every
// substring in wantSubstrs (order-independent). Pass an empty slice to assert
// the field is absent. Centralized here so tests don't have to repeat the same
// "for each substring, Contains + Errorf" pattern.
func assertNoteError(t *testing.T, note map[string]any, wantSubstrs ...string) {
	t.Helper()
	errMsg, _ := note["error"].(string)
	if len(wantSubstrs) == 0 {
		if e, has := note["error"]; has {
			t.Errorf("error should be absent, got %v", e)
		}
		return
	}
	for _, sub := range wantSubstrs {
		if !strings.Contains(errMsg, sub) {
			t.Errorf("error %q missing substring %q", errMsg, sub)
		}
	}
}

// assertNoteFieldAbsent fails the test if any of the named fields is present.
func assertNoteFieldAbsent(t *testing.T, note map[string]any, fields ...string) {
	t.Helper()
	for _, f := range fields {
		if v, has := note[f]; has {
			t.Errorf("%s should be absent, got %v", f, v)
		}
	}
}

func TestNotes_MeetingPath_NoteAndMinuteBothOK(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(meetingGetStub("m_both", "note_both"))
	reg.Register(noteDetailStub("note_both"))
	reg.Register(recordingOKStub("m_both", "https://meetings.feishu.cn/minutes/obc_both"))

	if err := mountAndRun(t, VCNotes, []string{"+notes", "--meeting-ids", "m_both", "--as", "user"}, f, stdout); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	note := extractFirstNote(t, stdout)
	if got := note["note_doc_token"]; got != "doc_main" {
		t.Errorf("note_doc_token = %v, want doc_main", got)
	}
	if got := note["minute_token"]; got != "obc_both" {
		t.Errorf("minute_token = %v, want obc_both", got)
	}
	assertNoteError(t, note)
}

func TestNotes_MeetingPath_OnlyMinuteFails_PartialSuccess(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(meetingGetStub("m_minfail", "note_minfail"))
	reg.Register(noteDetailStub("note_minfail"))
	reg.Register(recordingErrStub("m_minfail", 121005, "no permission"))

	if err := mountAndRun(t, VCNotes, []string{"+notes", "--meeting-ids", "m_minfail", "--as", "user"}, f, stdout); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	note := extractFirstNote(t, stdout)
	if got := note["note_doc_token"]; got != "doc_main" {
		t.Errorf("note_doc_token = %v, want doc_main", got)
	}
	assertNoteFieldAbsent(t, note, "minute_token")
	assertNoteError(t, note, "no permission to access this meeting's minute")
}

func TestNotes_MeetingPath_NoNote_ButMinuteOK(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	// note_id missing on the meeting object → no notes, but minute_token present
	reg.Register(meetingGetStub("m_nonote", ""))
	reg.Register(recordingOKStub("m_nonote", "https://meetings.feishu.cn/minutes/obc_nonote"))

	if err := mountAndRun(t, VCNotes, []string{"+notes", "--meeting-ids", "m_nonote", "--as", "user"}, f, stdout); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	note := extractFirstNote(t, stdout)
	if got := note["minute_token"]; got != "obc_nonote" {
		t.Errorf("minute_token = %v, want obc_nonote", got)
	}
	assertNoteError(t, note, "no notes available for this meeting")
}

func TestNotes_MeetingPath_BothFail_ErrorJoinedWithSemicolon(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	// no note_id → "no notes available..."; recording 121004 → "no minute file..."
	reg.Register(meetingGetStub("m_bothfail", ""))
	reg.Register(recordingErrStub("m_bothfail", 121004, "data not found"))

	// Two-path failure with no payload should make the batch return OutPartialFailure.
	err := mountAndRun(t, VCNotes, []string{"+notes", "--meeting-ids", "m_bothfail", "--as", "user"}, f, stdout)
	if err == nil {
		t.Fatalf("expected batch failure error, got nil")
	}
	var pfErr *output.PartialFailureError
	if !errors.As(err, &pfErr) {
		t.Fatalf("expected *output.PartialFailureError, got %T: %v", err, err)
	}
	if pfErr.Code != output.ExitAPI {
		t.Errorf("PartialFailureError.Code = %d, want ExitAPI (%d)", pfErr.Code, output.ExitAPI)
	}

	note := extractFirstNote(t, stdout)
	assertNoteFieldAbsent(t, note, "minute_token")
	assertNoteError(t, note,
		"no notes available for this meeting",
		"no minute file for this meeting",
		"; ", // causes joined with semicolon
	)
}

// noteDetailErrStub returns a stub that emits an error response from
// /open-apis/vc/v1/notes/{note_id}.
func noteDetailErrStub(noteID string, code int, msg string) *httpmock.Stub {
	return &httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/vc/v1/notes/" + noteID,
		Body:   map[string]any{"code": code, "msg": msg},
	}
}

func TestNotes_MeetingPath_NoteNoPermission_FriendlyHint(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	// note 接口返回 121005 → 阅读权限不足；同时 recording 也返回 121005，
	// 用以验证两路错误都会被合并到顶层 error 字段（用 "; " 拼接）。
	reg.Register(meetingGetStub("m_noteperm", "note_noperm"))
	reg.Register(noteDetailErrStub("note_noperm", 121005, "no permission"))
	reg.Register(recordingErrStub("m_noteperm", 121005, "no permission"))

	err := mountAndRun(t, VCNotes, []string{"+notes", "--meeting-ids", "m_noteperm", "--as", "user"}, f, stdout)
	if err == nil {
		t.Fatalf("expected batch failure error, got nil")
	}
	var pfErr *output.PartialFailureError
	if !errors.As(err, &pfErr) {
		t.Fatalf("expected *output.PartialFailureError, got %T: %v", err, err)
	}
	if pfErr.Code != output.ExitAPI {
		t.Errorf("PartialFailureError.Code = %d, want ExitAPI (%d)", pfErr.Code, output.ExitAPI)
	}

	note := extractFirstNote(t, stdout)
	assertNoteFieldAbsent(t, note, "note_doc_token", "minute_token")
	assertNoteError(t, note,
		"[121005]",
		"no read permission for this meeting note",
		"; ", // note + minute causes joined with semicolon
	)
}

// ---------------------------------------------------------------------------
// Typed-error lock: errs.ValidationError assertions
// ---------------------------------------------------------------------------

func TestNotes_BatchLimit_TypedValidationError(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, defaultConfig())
	ids := make([]string, 51)
	for i := range ids {
		ids[i] = fmt.Sprintf("m%d", i)
	}
	err := mountAndRun(t, VCNotes, []string{"+notes", "--meeting-ids", strings.Join(ids, ","), "--as", "user"}, f, nil)
	if err == nil {
		t.Fatal("expected batch limit error")
	}
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected *errs.ValidationError, got %T: %v", err, err)
	}
	if ve.Subtype != errs.SubtypeInvalidArgument {
		t.Errorf("Subtype = %q, want SubtypeInvalidArgument", ve.Subtype)
	}
	if !strings.HasPrefix(ve.Param, "--") {
		t.Errorf("Param = %q, want prefix '--'", ve.Param)
	}
}

func TestNotes_InvalidMinuteToken_TypedValidationError(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, defaultConfig())
	err := mountAndRun(t, VCNotes, []string{"+notes", "--minute-tokens", "INVALID_TOKEN!", "--as", "user"}, f, nil)
	if err == nil {
		t.Fatal("expected validation error for invalid minute token")
	}
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected *errs.ValidationError, got %T: %v", err, err)
	}
	if ve.Subtype != errs.SubtypeInvalidArgument {
		t.Errorf("Subtype = %q, want SubtypeInvalidArgument", ve.Subtype)
	}
	if ve.Param != "--minute-tokens" {
		t.Errorf("Param = %q, want --minute-tokens", ve.Param)
	}
}

func TestResolveMeetingIDs_NoRelationInfo_TypedValidationError(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, _, _, reg := cmdutil.TestFactory(t, defaultConfig())
	// mget returns empty instance_relation_infos
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/calendar/v4/calendars/cal_x/events/mget_instance_relation_info",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"instance_relation_infos": []interface{}{},
			},
		},
	})

	if err := botExec(t, "no-rel-info", f, func(_ context.Context, rctx *common.RuntimeContext) error {
		_, err := resolveMeetingIDsFromCalendarEvent(rctx, "evt_x", "cal_x", false)
		if err == nil {
			t.Fatal("expected error for empty instance_relation_infos")
		}
		var ve *errs.ValidationError
		if !errors.As(err, &ve) {
			t.Fatalf("expected *errs.ValidationError, got %T: %v", err, err)
		}
		if ve.Subtype != errs.SubtypeFailedPrecondition {
			t.Errorf("Subtype = %q, want SubtypeFailedPrecondition", ve.Subtype)
		}
		if !strings.Contains(ve.Error(), "no event relation info found") {
			t.Errorf("message = %q, want contains 'no event relation info found'", ve.Error())
		}
		return nil
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResolveMeetingIDs_NoMeetingIDs_TypedValidationError(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, _, _, reg := cmdutil.TestFactory(t, defaultConfig())
	// mget returns one info entry but with no meeting_instance_ids
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/calendar/v4/calendars/cal_y/events/mget_instance_relation_info",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"instance_relation_infos": []interface{}{
					map[string]interface{}{
						"instance_id":          "evt_y",
						"meeting_instance_ids": []interface{}{},
					},
				},
			},
		},
	})

	if err := botExec(t, "no-meeting-ids", f, func(_ context.Context, rctx *common.RuntimeContext) error {
		_, err := resolveMeetingIDsFromCalendarEvent(rctx, "evt_y", "cal_y", false)
		if err == nil {
			t.Fatal("expected error for empty meeting_instance_ids")
		}
		var ve *errs.ValidationError
		if !errors.As(err, &ve) {
			t.Fatalf("expected *errs.ValidationError, got %T: %v", err, err)
		}
		if ve.Subtype != errs.SubtypeFailedPrecondition {
			t.Errorf("Subtype = %q, want SubtypeFailedPrecondition", ve.Subtype)
		}
		if !strings.Contains(ve.Error(), "no associated video meeting for this event") {
			t.Errorf("message = %q, want contains 'no associated video meeting for this event'", ve.Error())
		}
		return nil
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Typed-error lock: enrichment via errs.ProblemOf
// ---------------------------------------------------------------------------

// minuteGetErrStub returns an error stub for the minutes API.
func minuteGetErrStub(token string, code int, msg string) *httpmock.Stub {
	return &httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/minutes/v1/minutes/" + token,
		Body:   map[string]any{"code": code, "msg": msg},
	}
}

// TestMinutesReadError_ProblemOf_EnrichesMessage pins that minutesReadError
// mutates the typed error's Message and Hint in-place via errs.ProblemOf when
// the server returns code 2091005 (minutes no-read-permission).
func TestMinutesReadError_ProblemOf_EnrichesMessage(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(minuteGetErrStub("tokperm", minutesNoReadPermissionCode, "no permission"))
	// artifactsStub not needed: we never reach it on error

	// A single minute-token that fails on a no-read-permission code still
	// produces a note carrying minute_token, so the batch exits 0 with the
	// enriched error surfaced inline rather than becoming an all-fail.
	if err := mountAndRun(t, VCNotes, []string{"+notes", "--minute-tokens", "tokperm", "--as", "user"}, f, stdout); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// stdout carries the note with the enriched error/hint
	var resp map[string]any
	if parseErr := json.Unmarshal(stdout.Bytes(), &resp); parseErr != nil {
		t.Fatalf("unmarshal stdout: %v\n%s", parseErr, stdout.String())
	}
	data, _ := resp["data"].(map[string]any)
	notes, _ := data["notes"].([]any)
	if len(notes) != 1 {
		t.Fatalf("expected 1 note, got %d", len(notes))
	}
	note, _ := notes[0].(map[string]any)

	errMsg, _ := note["error"].(string)
	if !strings.Contains(errMsg, "No read permission for minute tokperm") {
		t.Errorf("error message not enriched: %q", errMsg)
	}
	hint, _ := note["hint"].(string)
	if !strings.Contains(hint, "minute file read permission") {
		t.Errorf("hint not surfaced: %q", hint)
	}
}

// TestFetchNoteDetail_NoteNoPermission_ProblemOf pins that fetchNoteDetail
// returns a friendly error map when CallAPITyped returns code 121005 and
// ProblemOf can extract it.
func TestFetchNoteDetail_NoteNoPermission_ProblemOf(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())

	// meeting.get returns note_id, note detail returns 121005
	reg.Register(meetingGetStub("m_noteperm2", "note_perm2"))
	reg.Register(noteDetailErrStub("note_perm2", note.NoNoteReadPermissionCode, "no permission"))
	reg.Register(recordingOKStub("m_noteperm2", "https://meetings.feishu.cn/minutes/obcpermtest"))

	// note fails but minute_token succeeds → partial success (hasNotesPayload=true)
	err := mountAndRun(t, VCNotes, []string{"+notes", "--meeting-ids", "m_noteperm2", "--as", "user"}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error (expected partial success): %v", err)
	}
	note := extractFirstNote(t, stdout)

	// minute_token succeeded so hasNotesPayload=true; note error still surfaced
	if got := note["minute_token"]; got != "obcpermtest" {
		t.Errorf("minute_token = %v, want obcpermtest", got)
	}
	errMsg, _ := note["error"].(string)
	if !strings.Contains(errMsg, "[121005]") || !strings.Contains(errMsg, "no read permission for this meeting note") {
		t.Errorf("fetchNoteDetail permission error = %q; want contains '[121005]: no read permission for this meeting note'", errMsg)
	}
}

func TestFetchNoteDetail_EmptyDetailKeepsLegacyError(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, _, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/vc/v1/notes/note_empty_detail",
		Body: map[string]any{
			"code": 0,
			"data": map[string]any{},
		},
	})

	if err := botExec(t, "empty-note-detail", f, func(ctx context.Context, rctx *common.RuntimeContext) error {
		got := fetchNoteDetail(ctx, rctx, "note_empty_detail")
		if got["error"] != "note detail is empty" {
			t.Fatalf("error = %#v, want legacy empty-detail text", got["error"])
		}
		return nil
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestNotes_AllFailed_OutPartialFailure pins that when every item in the batch
// fails (successCount == 0), Execute returns *output.PartialFailureError with
// ExitAPI code, and stdout still carries the ok:false envelope with notes data.
func TestNotes_AllFailed_OutPartialFailure(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())

	// Both meetings have no note_id and recording returns 121004 (no minute file)
	// → hasNotesPayload == false for both → successCount == 0
	reg.Register(meetingGetStub("m_fail1", ""))
	reg.Register(recordingErrStub("m_fail1", 121004, "not found"))
	reg.Register(meetingGetStub("m_fail2", ""))
	reg.Register(recordingErrStub("m_fail2", 121004, "not found"))

	err := mountAndRun(t, VCNotes, []string{"+notes", "--meeting-ids", "m_fail1,m_fail2", "--as", "user"}, f, stdout)
	if err == nil {
		t.Fatal("expected batch failure error, got nil")
	}

	// typed partial-failure exit signal
	var pfErr *output.PartialFailureError
	if !errors.As(err, &pfErr) {
		t.Fatalf("expected *output.PartialFailureError, got %T: %v", err, err)
	}
	if pfErr.Code != output.ExitAPI {
		t.Errorf("PartialFailureError.Code = %d, want ExitAPI (%d)", pfErr.Code, output.ExitAPI)
	}

	// stdout carries ok:false envelope with both failed notes
	var env struct {
		OK   bool                   `json:"ok"`
		Data map[string]interface{} `json:"data"`
	}
	if parseErr := json.Unmarshal(stdout.Bytes(), &env); parseErr != nil {
		t.Fatalf("unmarshal stdout: %v\n%s", parseErr, stdout.String())
	}
	if env.OK {
		t.Errorf("ok must be false on all-fail, got ok:true")
	}
	notes, _ := env.Data["notes"].([]interface{})
	if len(notes) != 2 {
		t.Fatalf("expected 2 notes in data, got %d\nstdout: %s", len(notes), stdout.String())
	}
}
