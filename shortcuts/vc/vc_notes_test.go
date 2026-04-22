// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package vc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
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
				_, err := rctx.CallAPI("GET", "/open-apis/test/v1/warm", nil, nil)
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

func artifactsStub(token string) *httpmock.Stub {
	return &httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/minutes/v1/minutes/" + token + "/artifacts",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"summary":         "Test summary content",
				"minute_todos":    []interface{}{map[string]interface{}{"content": "Buy milk"}},
				"minute_chapters": []interface{}{map[string]interface{}{"title": "Intro", "summary_content": "Opening"}},
			},
		},
	}
}

func emptyArtifactsStub(token string) *httpmock.Stub {
	return &httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/minutes/v1/minutes/" + token + "/artifacts",
		Body:   map[string]interface{}{"code": 0, "msg": "ok", "data": map[string]interface{}{}},
	}
}

func transcriptStub(token string) *httpmock.Stub {
	return &httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/minutes/v1/minutes/" + token + "/transcript",
		Body:   map[string]interface{}{"code": 0, "msg": "ok", "data": map[string]interface{}{}},
	}
}

// transcriptRawStub returns an actual transcript body so downloadTranscriptFile
// writes a file to disk. Used by path-layout tests.
func transcriptRawStub(token string, body []byte) *httpmock.Stub {
	return &httpmock.Stub{
		Method:  "GET",
		URL:     "/open-apis/minutes/v1/minutes/" + token + "/transcript",
		RawBody: body,
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

func TestParseArtifactType(t *testing.T) {
	tests := []struct {
		input any
		want  int
	}{
		{float64(1), 1},
		{float64(2), 2},
		{json.Number("3"), 3},
		{"unknown", 0},
		{nil, 0},
	}
	for _, tt := range tests {
		got := parseArtifactType(tt.input)
		if got != tt.want {
			t.Errorf("parseArtifactType(%v) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestExtractArtifactTokens(t *testing.T) {
	artifacts := []any{
		map[string]any{"doc_token": "main_doc", "artifact_type": float64(1)},
		map[string]any{"doc_token": "verbatim_doc", "artifact_type": float64(2)},
		map[string]any{"doc_token": "unknown_doc", "artifact_type": float64(99)},
		nil,
	}
	noteDoc, verbatimDoc := extractArtifactTokens(artifacts)
	if noteDoc != "main_doc" {
		t.Errorf("noteDoc = %q, want %q", noteDoc, "main_doc")
	}
	if verbatimDoc != "verbatim_doc" {
		t.Errorf("verbatimDoc = %q, want %q", verbatimDoc, "verbatim_doc")
	}
}

func TestExtractArtifactTokens_Empty(t *testing.T) {
	noteDoc, verbatimDoc := extractArtifactTokens(nil)
	if noteDoc != "" || verbatimDoc != "" {
		t.Errorf("expected empty tokens for nil input, got %q, %q", noteDoc, verbatimDoc)
	}
}

func TestExtractDocTokens(t *testing.T) {
	refs := []any{
		map[string]any{"doc_token": "shared1"},
		map[string]any{"doc_token": "shared2"},
		map[string]any{"doc_token": ""},
		map[string]any{},
		nil,
	}
	tokens := extractDocTokens(refs)
	if len(tokens) != 2 || tokens[0] != "shared1" || tokens[1] != "shared2" {
		t.Errorf("extractDocTokens = %v, want [shared1 shared2]", tokens)
	}
}

func TestExtractDocTokens_Empty(t *testing.T) {
	tokens := extractDocTokens(nil)
	if tokens != nil {
		t.Errorf("expected nil for nil input, got %v", tokens)
	}
}

// ---------------------------------------------------------------------------
// Integration tests: +notes with mocked HTTP
// ---------------------------------------------------------------------------

func TestNotes_Validation_ExactlyOne(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, defaultConfig())

	err := mountAndRun(t, VCNotes, []string{"+notes", "--as", "user"}, f, nil)
	if err == nil {
		t.Fatal("expected validation error for no flags")
	}

	err = mountAndRun(t, VCNotes, []string{"+notes", "--meeting-ids", "m1", "--minute-tokens", "t1", "--as", "user"}, f, nil)
	if err == nil {
		t.Fatal("expected validation error for two flags")
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

func TestParseArtifactType_AllBranches(t *testing.T) {
	// cover json.Number branch
	if got := parseArtifactType(json.Number("1")); got != 1 {
		t.Errorf("json.Number: got %d, want 1", got)
	}
	// cover float64 branch
	if got := parseArtifactType(float64(2)); got != 2 {
		t.Errorf("float64: got %d, want 2", got)
	}
	// cover default branch
	if got := parseArtifactType("str"); got != 0 {
		t.Errorf("default: got %d, want 0", got)
	}
	// cover nil
	if got := parseArtifactType(nil); got != 0 {
		t.Errorf("nil: got %d, want 0", got)
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
	reg.Register(emptyArtifactsStub("tok001"))
	reg.Register(transcriptRawStub("tok001", []byte("speaker1: hello world\n")))

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
	reg.Register(emptyArtifactsStub("tok001"))
	reg.Register(transcriptRawStub("tok001", []byte("content")))

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
