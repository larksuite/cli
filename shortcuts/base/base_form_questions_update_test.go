// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"encoding/json"
	"testing"
)

func TestNormalizeUpdateFormQuestionAttachments(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantJSON string
	}{
		{
			name:     "attachment without attachment config defaults to all",
			input:    `[{"type":"attachment","title":"Upload"}]`,
			wantJSON: `[{"attachment":{"file_types":["all"]},"title":"Upload","type":"attachment"}]`,
		},
		{
			name:     "attachment with explicit file_types preserved",
			input:    `[{"type":"attachment","title":"Upload","attachment":{"file_types":["image","pdf"]}}]`,
			wantJSON: `[{"attachment":{"file_types":["image","pdf"]},"title":"Upload","type":"attachment"}]`,
		},
		{
			name:     "attachment with null file_types defaults to all",
			input:    `[{"type":"attachment","title":"Upload","attachment":{"file_types":null}}]`,
			wantJSON: `[{"attachment":{"file_types":["all"]},"title":"Upload","type":"attachment"}]`,
		},
		{
			name:     "non-attachment type unchanged",
			input:    `[{"type":"text","title":"Name"}]`,
			wantJSON: `[{"title":"Name","type":"text"}]`,
		},
		{
			name:     "mixed types only normalizes attachment",
			input:    `[{"type":"text","title":"Name"},{"type":"attachment","title":"Upload"}]`,
			wantJSON: `[{"title":"Name","type":"text"},{"attachment":{"file_types":["all"]},"title":"Upload","type":"attachment"}]`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseUpdateFormQuestions(tt.input)
			if err != nil {
				t.Fatalf("parseUpdateFormQuestions error: %v", err)
			}
			gotBytes, _ := json.Marshal(got)
			var gotMap, wantMap []map[string]interface{}
			_ = json.Unmarshal(gotBytes, &gotMap)
			_ = json.Unmarshal([]byte(tt.wantJSON), &wantMap)
			gotBytes2, _ := json.Marshal(gotMap)
			wantBytes2, _ := json.Marshal(wantMap)
			if string(gotBytes2) != string(wantBytes2) {
				t.Errorf("got %s, want %s", gotBytes2, wantBytes2)
			}
		})
	}
}

func TestParseUpdateFormQuestionsInvalidJSON(t *testing.T) {
	_, err := parseUpdateFormQuestions(`not-json`)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}
