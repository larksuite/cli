// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package task

import (
	"strings"
	"testing"
)

func TestSplitAndTrimCSV(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{name: "trim blanks", input: " a, ,b , c ", want: []string{"a", "b", "c"}},
		{name: "empty input", input: "", want: []string{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitAndTrimCSV(tt.input)
			if len(got) != len(tt.want) {
				t.Fatalf("len(splitAndTrimCSV(%q)) = %d, want %d", tt.input, len(got), len(tt.want))
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("splitAndTrimCSV(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestOutputTaskSummary(t *testing.T) {
	tests := []struct {
		name string
		task map[string]interface{}
	}{
		{
			name: "with timestamps and due",
			task: map[string]interface{}{
				"guid":       "task-123",
				"summary":    "summary",
				"url":        "https://example.com/task-123&suite_entity_num=t1",
				"created_at": "1775174400000",
				"due": map[string]interface{}{
					"timestamp": "1775174400000",
				},
			},
		},
		{
			name: "with completed and updated",
			task: map[string]interface{}{
				"guid":         "task-456",
				"summary":      "done",
				"url":          "https://example.com/task-456",
				"completed_at": "1775174400000",
				"updated_at":   "1775174400000",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := outputTaskSummary(tt.task)
			if got["guid"] != tt.task["guid"] || got["summary"] != tt.task["summary"] {
				t.Fatalf("unexpected summary output: %#v", got)
			}
			if got["url"] == "" {
				t.Fatalf("expected url in output, got %#v", got)
			}
		})
	}
}

func TestParseTimeRangeMillisAndRequireSearchFilter(t *testing.T) {
	timeTests := []struct {
		name      string
		input     string
		wantErr   bool
		wantStart string
		wantEnd   string
	}{
		{name: "empty input", input: "", wantStart: "", wantEnd: ""},
		{name: "invalid input", input: "bad-time", wantErr: true},
		{name: "range input", input: "-1d,+1d", wantStart: "non-empty", wantEnd: "non-empty"},
		{name: "reversed range fails fast", input: "+1d,-1d", wantErr: true},
	}
	for _, tt := range timeTests {
		t.Run("parse:"+tt.name, func(t *testing.T) {
			start, end, err := parseTimeRangeMillis(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseTimeRangeMillis(%q) expected error, got nil", tt.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseTimeRangeMillis(%q) error = %v", tt.input, err)
			}
			if tt.wantStart == "" && start != "" {
				t.Fatalf("start = %q, want empty", start)
			}
			if tt.wantEnd == "" && end != "" {
				t.Fatalf("end = %q, want empty", end)
			}
			if tt.wantStart == "non-empty" && start == "" {
				t.Fatalf("start should not be empty")
			}
			if tt.wantEnd == "non-empty" && end == "" {
				t.Fatalf("end should not be empty")
			}
		})
	}

	filterTests := []struct {
		name    string
		query   string
		filter  map[string]interface{}
		wantErr bool
	}{
		{name: "missing query and filter", query: "", filter: map[string]interface{}{}, wantErr: true},
		{name: "query only", query: "query", filter: map[string]interface{}{}, wantErr: false},
		{name: "filter only", query: "", filter: map[string]interface{}{"creator_ids": []string{"ou_1"}}, wantErr: false},
	}
	for _, tt := range filterTests {
		t.Run("filter:"+tt.name, func(t *testing.T) {
			err := requireSearchFilter(tt.query, tt.filter, "search")
			if tt.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestOutputRelatedTaskAndTimeRangeFilter(t *testing.T) {
	outputTests := []struct {
		name string
		task map[string]interface{}
	}{
		{
			name: "full related task",
			task: map[string]interface{}{
				"guid":          "task-123",
				"summary":       "Related Task",
				"description":   "desc",
				"status":        "todo",
				"source":        1,
				"mode":          2,
				"subtask_count": 0,
				"tasklists":     []interface{}{},
				"url":           "https://example.com/task-123&suite_entity_num=t1",
				"creator":       map[string]interface{}{"id": "ou_1"},
				"members":       []interface{}{map[string]interface{}{"id": "ou_2", "role": "follower"}},
				"created_at":    "1775174400000",
				"completed_at":  "1775174400000",
			},
		},
		{
			name: "minimal related task",
			task: map[string]interface{}{
				"guid":    "task-456",
				"summary": "Minimal",
				"url":     "https://example.com/task-456",
			},
		},
	}
	for _, tt := range outputTests {
		t.Run("output:"+tt.name, func(t *testing.T) {
			got := outputRelatedTask(tt.task)
			if got["guid"] != tt.task["guid"] || got["summary"] != tt.task["summary"] {
				t.Fatalf("unexpected related task output: %#v", got)
			}
		})
	}

	rangeTests := []struct {
		name    string
		start   string
		end     string
		wantNil bool
	}{
		{name: "empty range", start: "", end: "", wantNil: true},
		{name: "full range", start: "1", end: "2", wantNil: false},
	}
	for _, tt := range rangeTests {
		t.Run("range:"+tt.name, func(t *testing.T) {
			got := buildTimeRangeFilter("due_time", tt.start, tt.end)
			if tt.wantNil && got != nil {
				t.Fatalf("expected nil, got %#v", got)
			}
			if !tt.wantNil && got == nil {
				t.Fatalf("expected range filter, got nil")
			}
		})
	}
}

func TestRenderRelatedTasksPretty(t *testing.T) {
	tests := []struct {
		name      string
		items     []map[string]interface{}
		hasMore   bool
		pageToken string
		wantParts []string
	}{
		{
			name: "includes next token",
			items: []map[string]interface{}{
				{"guid": "task-123", "summary": "Related Task", "url": "https://example.com/task-123"},
			},
			hasMore:   true,
			pageToken: "pt_123",
			wantParts: []string{"Related Task", "Next page token: pt_123"},
		},
		{
			name: "without next token",
			items: []map[string]interface{}{
				{"guid": "task-456", "summary": "Another Task"},
			},
			hasMore:   false,
			pageToken: "",
			wantParts: []string{"Another Task"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := renderRelatedTasksPretty(tt.items, tt.hasMore, tt.pageToken)
			for _, want := range tt.wantParts {
				if !strings.Contains(out, want) {
					t.Fatalf("output missing %q: %s", want, out)
				}
			}
		})

		t.Run("parseTimeRangeRFC3339", func(t *testing.T) {
			timeTests := []struct {
				name      string
				input     string
				wantErr   bool
				wantStart string
				wantEnd   string
			}{
				{name: "empty input", input: "", wantStart: "", wantEnd: ""},
				{name: "invalid input", input: "bad-time", wantErr: true},
				{name: "range input", input: "-1d,+1d", wantStart: "rfc3339", wantEnd: "rfc3339"},
				{name: "reversed range fails fast", input: "+1d,-1d", wantErr: true},
			}

			for _, tt := range timeTests {
				t.Run(tt.name, func(t *testing.T) {
					start, end, err := parseTimeRangeRFC3339(tt.input)
					if tt.wantErr {
						if err == nil {
							t.Fatal("expected error, got nil")
						}
						return
					}
					if err != nil {
						t.Fatalf("parseTimeRangeRFC3339() error = %v", err)
					}
					if tt.wantStart == "rfc3339" {
						if !strings.Contains(start, "T") || !strings.Contains(start, ":") {
							t.Fatalf("expected RFC3339 start, got %q", start)
						}
					} else if start != tt.wantStart {
						t.Fatalf("unexpected start: %q", start)
					}
					if tt.wantEnd == "rfc3339" {
						if !strings.Contains(end, "T") || !strings.Contains(end, ":") {
							t.Fatalf("expected RFC3339 end, got %q", end)
						}
					} else if end != tt.wantEnd {
						t.Fatalf("unexpected end: %q", end)
					}
				})
			}
		})
	}
}
