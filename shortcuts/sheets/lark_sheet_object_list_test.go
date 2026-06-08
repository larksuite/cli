// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"testing"

	"github.com/larksuite/cli/shortcuts/common"
)

// TestObjectListShortcuts_DryRun covers all 7 object-list shortcuts.
// Each spec asserts the tool name + that the optional filter flag maps
// to the right tool field (including the --rule-id → conditional_format_id
// rename).
func TestObjectListShortcuts_DryRun(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		sc        common.Shortcut
		args      []string
		toolName  string
		wantInput map[string]interface{}
	}{
		{
			name:     "+chart-list no filter",
			sc:       ChartList,
			args:     []string{"--url", testURL, "--sheet-id", testSheetID},
			toolName: "get_chart_objects",
			wantInput: map[string]interface{}{
				"excel_id": testToken,
				"sheet_id": testSheetID,
			},
		},
		{
			name:     "+chart-list with filter",
			sc:       ChartList,
			args:     []string{"--url", testURL, "--sheet-id", testSheetID, "--chart-id", "chartXYZ"},
			toolName: "get_chart_objects",
			wantInput: map[string]interface{}{
				"excel_id": testToken,
				"sheet_id": testSheetID,
				"chart_id": "chartXYZ",
			},
		},
		{
			name:     "+pivot-list filter",
			sc:       PivotList,
			args:     []string{"--url", testURL, "--sheet-id", testSheetID, "--pivot-table-id", "ptA"},
			toolName: "get_pivot_table_objects",
			wantInput: map[string]interface{}{
				"pivot_table_id": "ptA",
			},
		},
		{
			name:     "+cond-format-list --rule-id → conditional_format_id",
			sc:       CondFormatList,
			args:     []string{"--url", testURL, "--sheet-id", testSheetID, "--rule-id", "ruleA"},
			toolName: "get_conditional_format_objects",
			wantInput: map[string]interface{}{
				"conditional_format_id": "ruleA",
			},
		},
		{
			name:     "+filter-list (no filter flag) by sheet-name",
			sc:       FilterList,
			args:     []string{"--url", testURL, "--sheet-name", "Sheet1"},
			toolName: "get_filter_objects",
			wantInput: map[string]interface{}{
				"excel_id":   testToken,
				"sheet_name": "Sheet1",
			},
		},
		{
			name:     "+filter-view-list cli-only via callTool",
			sc:       FilterViewList,
			args:     []string{"--url", testURL, "--sheet-id", testSheetID, "--view-id", "viewABC"},
			toolName: "get_filter_view_objects",
			wantInput: map[string]interface{}{
				"view_id": "viewABC",
			},
		},
		{
			name:     "+sparkline-list --group-id",
			sc:       SparklineList,
			args:     []string{"--url", testURL, "--sheet-id", testSheetID, "--group-id", "grpA"},
			toolName: "get_sparkline_objects",
			wantInput: map[string]interface{}{
				"group_id": "grpA",
			},
		},
		{
			name:     "+float-image-list",
			sc:       FloatImageList,
			args:     []string{"--url", testURL, "--sheet-id", testSheetID, "--float-image-id", "imgA"},
			toolName: "get_float_image_objects",
			wantInput: map[string]interface{}{
				"float_image_id": "imgA",
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
