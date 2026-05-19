// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"strings"
	"testing"

	"github.com/larksuite/cli/shortcuts/common"
)

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
			name:     "+rows-resize --type pixel --size 200",
			sc:       RowsResize,
			args:     []string{"--url", testURL, "--sheet-id", testSheetID, "--start", "0", "--end", "4", "--type", "pixel", "--size", "200"},
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
			name:     "+rows-resize --type auto omits --size",
			sc:       RowsResize,
			args:     []string{"--url", testURL, "--sheet-id", testSheetID, "--start", "0", "--end", "0", "--type", "auto"},
			toolName: "resize_range",
			wantInput: map[string]interface{}{
				"range":         "1",
				"resize_height": map[string]interface{}{"type": "auto"},
			},
		},
		{
			name:     "+cols-resize --type standard (reset to default)",
			sc:       ColsResize,
			args:     []string{"--url", testURL, "--sheet-id", testSheetID, "--start", "1", "--end", "3", "--type", "standard"},
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
			name:     "+cols-resize --type pixel --size 120",
			sc:       ColsResize,
			args:     []string{"--url", testURL, "--sheet-id", testSheetID, "--start", "0", "--end", "2", "--type", "pixel", "--size", "120"},
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
			args:     []string{"--url", testURL, "--sheet-id", testSheetID, "--range", "A1:E100", "--has-header", "--sort-keys", `[{"col":"B","order":"asc"},{"col":"D","order":"desc"}]`},
			toolName: "transform_range",
			wantInput: map[string]interface{}{
				"excel_id":   testToken,
				"sheet_id":   testSheetID,
				"operation":  "sort",
				"range":      "A1:E100",
				"has_header": true,
				"sort_conditions": []interface{}{
					map[string]interface{}{"col": "B", "order": "asc"},
					map[string]interface{}{"col": "D", "order": "desc"},
				},
			},
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			body := parseDryRunBody(t, tt.sc, tt.args)
			got := decodeToolInput(t, body, tt.toolName)
			assertInputEquals(t, got, tt.wantInput)
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
			args: []string{"--url", testURL, "--sheet-id", testSheetID, "--start", "0", "--end", "3", "--type", "pixel"},
			want: "--type pixel requires --size",
		},
		{
			name: "+rows-resize --type standard with --size",
			sc:   RowsResize,
			args: []string{"--url", testURL, "--sheet-id", testSheetID, "--start", "0", "--end", "3", "--type", "standard", "--size", "30"},
			want: "--size is only valid with --type pixel",
		},
		{
			name: "+cols-resize rejects --type auto",
			sc:   ColsResize,
			args: []string{"--url", testURL, "--sheet-id", testSheetID, "--start", "0", "--end", "2", "--type", "auto"},
			want: "auto", // cobra Enum gate kicks first with "valid values are: pixel, standard"
		},
		{
			name: "--end < --start",
			sc:   RowsResize,
			args: []string{"--url", testURL, "--sheet-id", testSheetID, "--start", "5", "--end", "3", "--type", "standard"},
			want: "must be >= --start",
		},
	}
	for _, tt := range cases {
		tt := tt
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
