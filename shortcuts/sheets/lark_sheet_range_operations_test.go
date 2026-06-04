// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
)

func TestAnnotateEmbeddedBlockClearErr(t *testing.T) {
	t.Parallel()

	t.Run("adds pivot-delete hint on embedded-block error", func(t *testing.T) {
		in := &output.ExitError{Code: output.ExitAPI, Detail: &output.ErrDetail{
			Type:    "api",
			Message: `tool "clear_cell_range" failed: [500] can not find embedded block`,
		}}
		var ee *output.ExitError
		if !errors.As(annotateEmbeddedBlockClearErr(in), &ee) || ee.Detail == nil {
			t.Fatal("expected ExitError with detail")
		}
		if !strings.Contains(ee.Detail.Hint, "+pivot-delete") {
			t.Errorf("hint should point at +pivot-delete, got %q", ee.Detail.Hint)
		}
	})

	t.Run("appends to existing hint", func(t *testing.T) {
		in := &output.ExitError{Code: output.ExitAPI, Detail: &output.ErrDetail{
			Message: "embedded block missing", Hint: "preexisting",
		}}
		out := annotateEmbeddedBlockClearErr(in).(*output.ExitError)
		if !strings.HasPrefix(out.Detail.Hint, "preexisting; ") {
			t.Errorf("existing hint should be preserved and appended, got %q", out.Detail.Hint)
		}
	})

	t.Run("passes through unrelated ExitError untouched", func(t *testing.T) {
		in := &output.ExitError{Code: output.ExitAPI, Detail: &output.ErrDetail{Message: "some other failure"}}
		out := annotateEmbeddedBlockClearErr(in).(*output.ExitError)
		if out.Detail.Hint != "" {
			t.Errorf("unrelated error should not gain a hint, got %q", out.Detail.Hint)
		}
	})

	t.Run("passes through non-ExitError untouched", func(t *testing.T) {
		in := errors.New("can not find embedded block")
		if out := annotateEmbeddedBlockClearErr(in); out != in {
			t.Error("plain (non-ExitError) error should be returned as-is")
		}
	})
}

func TestRangeOperationsShortcuts_DryRun(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		sc        common.Shortcut
		args      []string
		toolName  string
		wantInput map[string]interface{}
	}{
		{
			name:     "+cells-clear scope=content → clear_type=contents",
			sc:       CellsClear,
			args:     []string{"--url", testURL, "--sheet-id", testSheetID, "--range", "A1:C5", "--scope", "content"},
			toolName: "clear_cell_range",
			wantInput: map[string]interface{}{
				"excel_id":   testToken,
				"sheet_id":   testSheetID,
				"range":      "A1:C5",
				"clear_type": "contents",
			},
		},
		{
			name:     "+cells-clear scope=all passthrough",
			sc:       CellsClear,
			args:     []string{"--url", testURL, "--sheet-id", testSheetID, "--range", "A1:C5", "--scope", "all"},
			toolName: "clear_cell_range",
			wantInput: map[string]interface{}{
				"clear_type": "all",
			},
		},
		{
			name:     "+cells-merge with merge-type",
			sc:       CellsMerge,
			args:     []string{"--url", testURL, "--sheet-id", testSheetID, "--range", "A1:B2", "--merge-type", "rows"},
			toolName: "merge_cells",
			wantInput: map[string]interface{}{
				"excel_id":   testToken,
				"sheet_id":   testSheetID,
				"range":      "A1:B2",
				"operation":  "merge",
				"merge_type": "rows",
			},
		},
		{
			name:     "+cells-unmerge (no merge-type flag)",
			sc:       CellsUnmerge,
			args:     []string{"--url", testURL, "--sheet-id", testSheetID, "--range", "A1:B2"},
			toolName: "merge_cells",
			wantInput: map[string]interface{}{
				"excel_id":  testToken,
				"sheet_id":  testSheetID,
				"range":     "A1:B2",
				"operation": "unmerge",
			},
		},
		{
			name:     "+rows-resize --range 1:5 pixel 200",
			sc:       RowsResize,
			args:     []string{"--url", testURL, "--sheet-id", testSheetID, "--range", "1:5", "--type", "pixel", "--size", "200"},
			toolName: "resize_range",
			wantInput: map[string]interface{}{
				"excel_id": testToken,
				"sheet_id": testSheetID,
				"range":    "1:5",
				"resize_height": map[string]interface{}{
					"type":  "pixel",
					"value": float64(200),
				},
			},
		},
		{
			name:     "+rows-resize single row \"1\" expands to \"1:1\"",
			sc:       RowsResize,
			args:     []string{"--url", testURL, "--sheet-id", testSheetID, "--range", "1", "--type", "auto"},
			toolName: "resize_range",
			wantInput: map[string]interface{}{
				"range":         "1:1",
				"resize_height": map[string]interface{}{"type": "auto"},
			},
		},
		{
			name:     "+cols-resize --range B:D standard",
			sc:       ColsResize,
			args:     []string{"--url", testURL, "--sheet-id", testSheetID, "--range", "B:D", "--type", "standard"},
			toolName: "resize_range",
			wantInput: map[string]interface{}{
				"excel_id": testToken,
				"sheet_id": testSheetID,
				"range":    "B:D",
				"resize_width": map[string]interface{}{
					"type": "standard",
				},
			},
		},
		{
			name:     "+cols-resize --range A:C pixel 120",
			sc:       ColsResize,
			args:     []string{"--url", testURL, "--sheet-id", testSheetID, "--range", "A:C", "--type", "pixel", "--size", "120"},
			toolName: "resize_range",
			wantInput: map[string]interface{}{
				"range": "A:C",
				"resize_width": map[string]interface{}{
					"type":  "pixel",
					"value": float64(120),
				},
			},
		},
		{
			name:     "+cols-resize single column \"C\" expands to \"C:C\"",
			sc:       ColsResize,
			args:     []string{"--url", testURL, "--sheet-id", testSheetID, "--range", "C", "--type", "standard"},
			toolName: "resize_range",
			wantInput: map[string]interface{}{
				"range":        "C:C",
				"resize_width": map[string]interface{}{"type": "standard"},
			},
		},
		{
			name:     "+range-move cross-sheet",
			sc:       RangeMove,
			args:     []string{"--url", testURL, "--sheet-id", testSheetID, "--source-range", "A1:C5", "--target-range", "D1", "--target-sheet-id", testSheetID2},
			toolName: "transform_range",
			wantInput: map[string]interface{}{
				"excel_id":             testToken,
				"sheet_id":             testSheetID,
				"operation":            "move",
				"range":                "A1:C5",
				"destination_range":    "D1",
				"destination_sheet_id": testSheetID2,
			},
		},
		{
			name:     "+range-copy paste-type values → value_only",
			sc:       RangeCopy,
			args:     []string{"--url", testURL, "--sheet-id", testSheetID, "--source-range", "A1:C5", "--target-range", "E1", "--paste-type", "values"},
			toolName: "transform_range",
			wantInput: map[string]interface{}{
				"excel_id":          testToken,
				"sheet_id":          testSheetID,
				"operation":         "copy",
				"range":             "A1:C5",
				"destination_range": "E1",
				"paste_type":        "value_only",
			},
		},
		{
			name:     "+range-copy paste-type all → field omitted",
			sc:       RangeCopy,
			args:     []string{"--url", testURL, "--sheet-id", testSheetID, "--source-range", "A1:C5", "--target-range", "E1"},
			toolName: "transform_range",
			wantInput: map[string]interface{}{
				"excel_id":          testToken,
				"sheet_id":          testSheetID,
				"operation":         "copy",
				"range":             "A1:C5",
				"destination_range": "E1",
			},
		},
		{
			name:     "+range-fill series=copy → copyCells",
			sc:       RangeFill,
			args:     []string{"--url", testURL, "--sheet-id", testSheetID, "--source-range", "A1:A3", "--target-range", "A4:A10", "--series-type", "copy"},
			toolName: "transform_range",
			wantInput: map[string]interface{}{
				"excel_id":          testToken,
				"sheet_id":          testSheetID,
				"operation":         "fill",
				"range":             "A1:A3",
				"destination_range": "A4:A10",
				"fill_type":         "copyCells",
			},
		},
		{
			name:     "+range-fill series=linear → fillSeries",
			sc:       RangeFill,
			args:     []string{"--url", testURL, "--sheet-id", testSheetID, "--source-range", "A1:A3", "--target-range", "A4:A10", "--series-type", "linear"},
			toolName: "transform_range",
			wantInput: map[string]interface{}{
				"fill_type": "fillSeries",
			},
		},
		{
			name:     "+range-sort multi-key with header",
			sc:       RangeSort,
			args:     []string{"--url", testURL, "--sheet-id", testSheetID, "--range", "A1:E100", "--has-header", "--sort-keys", `[{"column":"B","ascending":true},{"column":"D","ascending":false}]`},
			toolName: "transform_range",
			wantInput: map[string]interface{}{
				"excel_id":   testToken,
				"sheet_id":   testSheetID,
				"operation":  "sort",
				"range":      "A1:E100",
				"has_header": true,
				"sort_conditions": []interface{}{
					map[string]interface{}{"column": "B", "ascending": true},
					map[string]interface{}{"column": "D", "ascending": false},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			body := parseDryRunBody(t, tt.sc, tt.args)
			got := decodeToolInput(t, body, tt.toolName)
			assertInputEquals(t, got, tt.wantInput)
		})
	}
}

// TestRangeSort_RejectsMalformedKeys verifies the schema-driven check
// that each --sort-keys entry has both `column` (string) and
// `ascending` (bool). The schema validator (loaded from
// data/flag-schemas.json) reports the offending JSON path; previously
// the CLI passed any JSON through and the server bounced with a terse
// "required property X missing" that didn't name the bad entry.
func TestRangeSort_RejectsMalformedKeys(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		keys string
		want string
	}{
		{"missing column", `[{"ascending":true}]`, `required property "column" is missing at [0]`},
		{"missing ascending", `[{"column":"B"}]`, `required property "ascending" is missing at [0]`},
		{"old vocab col/order", `[{"col":"B","order":"asc"}]`, `required property "column" is missing at [0]`},
		{"non-object item", `["B"]`, `[0]: expected type "object"`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			stdout, stderr, err := runShortcutCapturingErr(t, RangeSort, []string{
				"--url", testURL, "--sheet-id", testSheetID,
				"--range", "A1:E10", "--sort-keys", c.keys, "--dry-run",
			})
			if err == nil {
				t.Fatalf("expected validation error; stdout=%s stderr=%s", stdout, stderr)
			}
			if !strings.Contains(stdout+stderr+err.Error(), c.want) {
				t.Errorf("want substring %q in error; got stdout=%s stderr=%s err=%v", c.want, stdout, stderr, err)
			}
		})
	}
}

func TestResize_TypeAndSizeGuards(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		sc   common.Shortcut
		args []string
		want string
	}{
		{
			name: "+rows-resize --type pixel without --size",
			sc:   RowsResize,
			args: []string{"--url", testURL, "--sheet-id", testSheetID, "--range", "1:5", "--type", "pixel"},
			want: "--type pixel requires --size",
		},
		{
			name: "+rows-resize --type standard with --size",
			sc:   RowsResize,
			args: []string{"--url", testURL, "--sheet-id", testSheetID, "--range", "1:5", "--type", "standard", "--size", "30"},
			want: "--size is only valid with --type pixel",
		},
		{
			name: "+cols-resize rejects --type auto",
			sc:   ColsResize,
			args: []string{"--url", testURL, "--sheet-id", testSheetID, "--range", "A:C", "--type", "auto"},
			want: "auto", // cobra Enum gate kicks first with "valid values are: pixel, standard"
		},
		{
			name: "+rows-resize given column range",
			sc:   RowsResize,
			args: []string{"--url", testURL, "--sheet-id", testSheetID, "--range", "A:C", "--type", "standard"},
			want: "+rows-resize expects row numbers",
		},
		{
			name: "+cols-resize given row range",
			sc:   ColsResize,
			args: []string{"--url", testURL, "--sheet-id", testSheetID, "--range", "1:5", "--type", "standard"},
			want: "+cols-resize expects column letters",
		},
		{
			name: "+rows-resize end < start",
			sc:   RowsResize,
			args: []string{"--url", testURL, "--sheet-id", testSheetID, "--range", "5:3", "--type", "standard"},
			want: "end position is before start",
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			stdout, stderr, err := runShortcutCapturingErr(t, tt.sc, append(tt.args, "--dry-run"))
			if err == nil {
				t.Fatalf("expected validation error; stdout=%s stderr=%s", stdout, stderr)
			}
			if !strings.Contains(stdout+stderr+err.Error(), tt.want) {
				t.Errorf("expected %q; got=%s|%s|%v", tt.want, stdout, stderr, err)
			}
		})
	}
}
