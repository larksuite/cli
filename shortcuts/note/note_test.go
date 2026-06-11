// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package note

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
)

// These tests were relocated from shortcuts/vc/vc_notes_test.go together with
// the note-detail parsing helpers they cover.

func TestParseLooseInt(t *testing.T) {
	tests := []struct {
		input any
		want  int
	}{
		{float64(1), 1},
		{float64(2), 2},
		{float64(1.9), 0},
		{json.Number("3"), 3},
		{"unknown", 0},
		{nil, 0},
	}
	for _, tt := range tests {
		got := parseLooseInt(tt.input)
		if got != tt.want {
			t.Errorf("parseLooseInt(%v) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestParseLooseCursorID(t *testing.T) {
	tests := []struct {
		name string
		in   any
		want string
		ok   bool
	}{
		{name: "string", in: "7648924766078847940", want: "7648924766078847940", ok: true},
		{name: "trim string", in: " 123 ", want: "123", ok: true},
		{name: "empty string", in: "", ok: false},
		{name: "zero string", in: "0", ok: false},
		{name: "json number", in: json.Number("123"), want: "123", ok: true},
		{name: "float safe integer", in: float64(123), want: "123", ok: true},
		{name: "float unsafe integer", in: float64(1<<53 + 1), ok: false},
		{name: "float fractional", in: float64(1.5), ok: false},
		{name: "negative", in: -1, ok: false},
		{name: "nil", in: nil, ok: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parseLooseCursorID(tt.in)
			if got != tt.want || ok != tt.ok {
				t.Fatalf("parseLooseCursorID(%v) = (%q, %v), want (%q, %v)", tt.in, got, ok, tt.want, tt.ok)
			}
		})
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

func TestDetailToMap(t *testing.T) {
	detail := &Detail{
		NoteID:           "note_1",
		CreatorID:        "creator_1",
		CreateTime:       "2026-06-09 12:00:00",
		DisplayType:      "unified",
		NoteDocToken:     "note_doc",
		VerbatimDocToken: "verbatim_doc",
		SharedDocTokens:  []string{"shared_1", "shared_2"},
	}

	got := detail.ToMap()
	want := map[string]any{
		"note_id":            "note_1",
		"creator_id":         "creator_1",
		"create_time":        "2026-06-09 12:00:00",
		"note_display_type":  "unified",
		"note_doc_token":     "note_doc",
		"verbatim_doc_token": "verbatim_doc",
		"shared_doc_tokens":  []string{"shared_1", "shared_2"},
	}
	for key, wantValue := range want {
		gotValue, ok := got[key]
		if !ok {
			t.Fatalf("ToMap missing key %q in %#v", key, got)
		}
		if !valuesEqual(gotValue, wantValue) {
			t.Fatalf("ToMap[%q] = %#v, want %#v", key, gotValue, wantValue)
		}
	}
}

func TestDetailToMap_OmitsEmptySharedDocTokens(t *testing.T) {
	got := (&Detail{NoteID: "note_1"}).ToMap()
	if _, ok := got["shared_doc_tokens"]; ok {
		t.Fatalf("ToMap should omit empty shared_doc_tokens, got %#v", got)
	}
}

func TestMapNoteError_NoReadPermission(t *testing.T) {
	err := &errs.PermissionError{
		Problem: errs.Problem{
			Category: errs.CategoryAuthorization,
			Subtype:  errs.SubtypePermissionDenied,
			Code:     NoNoteReadPermissionCode,
			Message:  "upstream permission denied",
			LogID:    "log_1",
		},
		MissingScopes: []string{"vc:note:read"},
		Identity:      "user",
	}

	got := mapNoteError(err)
	problem, ok := errs.ProblemOf(got)
	if !ok {
		t.Fatalf("mapNoteError returned %T, want typed problem", got)
	}
	if problem.Code != NoNoteReadPermissionCode {
		t.Fatalf("mapped code = %d, want %d", problem.Code, NoNoteReadPermissionCode)
	}
	if !strings.Contains(problem.Message, "no read permission for this note") || !strings.Contains(problem.Message, "upstream permission denied") {
		t.Fatalf("mapped message = %q, want note permission guidance with upstream message", problem.Message)
	}
	if !errors.Is(got, err) {
		t.Fatal("mapped error should preserve the original typed error as cause")
	}
	originalProblem, _ := errs.ProblemOf(err)
	if originalProblem.Message != "upstream permission denied" {
		t.Fatalf("original message was mutated to %q", originalProblem.Message)
	}
	var gotPerm *errs.PermissionError
	if !errors.As(got, &gotPerm) {
		t.Fatalf("mapped error = %T, want PermissionError", got)
	}
	if gotPerm.LogID != "log_1" {
		t.Fatalf("LogID = %q, want preserved log_1", gotPerm.LogID)
	}
	if len(gotPerm.MissingScopes) != 1 || gotPerm.MissingScopes[0] != "vc:note:read" {
		t.Fatalf("MissingScopes = %#v, want preserved vc:note:read", gotPerm.MissingScopes)
	}
	if gotPerm.Identity != "user" {
		t.Fatalf("Identity = %q, want preserved user", gotPerm.Identity)
	}
}

func TestMapNoteError_NormalizesNonPermissionTypedError(t *testing.T) {
	err := &errs.APIError{
		Problem: errs.Problem{
			Category: errs.CategoryAPI,
			Subtype:  errs.SubtypeUnknown,
			Code:     NoNoteReadPermissionCode,
			Message:  "upstream api error",
			LogID:    "log_2",
		},
	}

	got := mapNoteError(err)
	var gotPerm *errs.PermissionError
	if !errors.As(got, &gotPerm) {
		t.Fatalf("mapped error = %T, want PermissionError", got)
	}
	if gotPerm.Category != errs.CategoryAuthorization || gotPerm.Subtype != errs.SubtypePermissionDenied {
		t.Fatalf("mapped category/subtype = %q/%q, want authorization/permission_denied", gotPerm.Category, gotPerm.Subtype)
	}
	if !strings.Contains(gotPerm.Message, "no read permission for this note") || !strings.Contains(gotPerm.Message, "upstream api error") {
		t.Fatalf("mapped message = %q, want note permission guidance with upstream message", gotPerm.Message)
	}
	if gotPerm.Hint == "" {
		t.Fatal("mapped hint should not be empty")
	}
	if gotPerm.LogID != "log_2" {
		t.Fatalf("LogID = %q, want preserved log_2", gotPerm.LogID)
	}
	if !errors.Is(got, err) {
		t.Fatal("mapped error should preserve the original typed error as cause")
	}
}

func TestMapNoteError_Passthrough(t *testing.T) {
	err := errors.New("boom")
	if got := mapNoteError(err); got != err {
		t.Fatalf("mapNoteError passthrough = %v, want original", got)
	}
}

func TestShortcuts(t *testing.T) {
	shortcuts := Shortcuts()
	if len(shortcuts) != 2 {
		t.Fatalf("Shortcuts len = %d, want 2", len(shortcuts))
	}
	if shortcuts[0].Command != "+detail" || shortcuts[1].Command != "+transcript" {
		t.Fatalf("Shortcuts commands = %q, %q", shortcuts[0].Command, shortcuts[1].Command)
	}
}

func valuesEqual(a, b any) bool {
	ab, _ := json.Marshal(a)
	bb, _ := json.Marshal(b)
	return string(ab) == string(bb)
}
