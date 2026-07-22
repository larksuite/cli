// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import "testing"

func TestChartCreateBasic_AllTypes(t *testing.T) {
	t.Parallel()

	types := []string{"column", "bar", "line", "area", "pie", "scatter", "combo", "radar"}
	for _, chartType := range types {
		chartType := chartType
		t.Run(chartType, func(t *testing.T) {
			t.Parallel()
			rangeValue := "A1:C4"
			if chartType == "combo" {
				rangeValue = "A1:D4"
			}
			body := parseDryRunBody(t, ChartCreateBasic, []string{
				"--url", testURL,
				"--sheet-id", testSheetID,
				"--chart-type", chartType,
				"--data-range", rangeValue,
			})
			input := decodeToolInput(t, body, "manage_chart_object")
			if input["operation"] != "create" {
				t.Fatalf("operation = %v, want create", input["operation"])
			}
			if _, ok := input["properties"]; ok {
				t.Fatal("semantic create must not send properties")
			}
			basic, _ := input["basic_chart"].(map[string]interface{})
			if basic["chart_type"] != chartType || basic["data_range"] != rangeValue {
				t.Fatalf("basic_chart = %#v", basic)
			}
		})
	}
}

func TestChartCreateBasic_ConfigAndPlacement(t *testing.T) {
	t.Parallel()
	body := parseDryRunBody(t, ChartCreateBasic, []string{
		"--url", testURL,
		"--sheet-id", testSheetID,
		"--chart-type", "line",
		"--data-range", "A1:C4",
		"--anchor-cell", "f2",
		"--width", "640",
		"--height", "360",
		"--title", "Trend",
		"--legend-position", "bottom",
		"--smooth=false",
		"--data-direction", "row",
		"--color-palette", "brandColorSeries@v2",
	})
	input := decodeToolInput(t, body, "manage_chart_object")
	basic, _ := input["basic_chart"].(map[string]interface{})
	position, _ := basic["position"].(map[string]interface{})
	size, _ := basic["size"].(map[string]interface{})
	if position["col"] != "F" || position["row"] != float64(1) {
		t.Errorf("position = %#v, want F2 as zero-based row 1", position)
	}
	if size["width"] != float64(640) || size["height"] != float64(360) {
		t.Errorf("size = %#v", size)
	}
	if basic["title"] != "Trend" || basic["legend_position"] != "bottom" || basic["smooth"] != false ||
		basic["data_direction"] != "row" || basic["color_palette"] != "brandColorSeries@v2" {
		t.Errorf("semantic config = %#v", basic)
	}
}

func TestChartCreateBasic_MultipleAlignedRanges(t *testing.T) {
	t.Parallel()
	rangeValue := "'Data, 2026'!A1:A10,'Data, 2026'!K1:L10"
	body := parseDryRunBody(t, ChartCreateBasic, []string{
		"--url", testURL,
		"--sheet-id", testSheetID,
		"--chart-type", "line",
		"--data-range", rangeValue,
	})
	input := decodeToolInput(t, body, "manage_chart_object")
	basic := input["basic_chart"].(map[string]interface{})
	if basic["data_range"] != rangeValue {
		t.Fatalf("basic_chart.data_range = %v, want %q", basic["data_range"], rangeValue)
	}
}

func TestChartSemanticShortcuts_InBatchUpdate(t *testing.T) {
	body := parseDryRunBody(t, BatchUpdate, []string{
		"--url", testURL,
		"--operations", `[
			{"shortcut":"+chart-create-basic","input":{"sheet-id":"sh1","chart-type":"column","data-range":"A1:C10","title":"Sales"}},
			{"shortcut":"+chart-create-basic","input":{"sheet-id":"sh1","chart-type":"line","data-range":"E1:G10","title":"Trend"}}
		]`,
		"--yes",
	})
	input := decodeToolInput(t, body, "batch_update")
	ops := input["operations"].([]interface{})
	if len(ops) != 2 {
		t.Fatalf("operations len = %d, want 2", len(ops))
	}
	for i, op := range ops {
		item := op.(map[string]interface{})
		if item["tool_name"] != "manage_chart_object" {
			t.Fatalf("operations[%d].tool_name = %v", i, item["tool_name"])
		}
		chartInput := item["input"].(map[string]interface{})
		if chartInput["operation"] != "create" {
			t.Fatalf("operations[%d].input.operation = %v", i, chartInput["operation"])
		}
		if _, ok := chartInput["basic_chart"].(map[string]interface{}); !ok {
			t.Fatalf("operations[%d].input.basic_chart = %#v", i, chartInput["basic_chart"])
		}
	}
}

func TestChartConfigUpdate_PartialFields(t *testing.T) {
	t.Parallel()
	body := parseDryRunBody(t, ChartConfigUpdate, []string{
		"--url", testURL,
		"--sheet-id", testSheetID,
		"--chart-id", "chart-1",
		"--y-axis-title", "Revenue",
		"--stack", "percent",
		"--smooth=false",
		"--colors", "#112233,#445566",
	})
	input := decodeToolInput(t, body, "manage_chart_object")
	if input["operation"] != "update" || input["chart_id"] != "chart-1" {
		t.Fatalf("input = %#v", input)
	}
	if _, ok := input["properties"]; ok {
		t.Fatal("semantic update must not send properties")
	}
	updates, _ := input["config_updates"].(map[string]interface{})
	if updates["y_axis_title"] != "Revenue" || updates["stack"] != "percent" || updates["smooth"] != false {
		t.Errorf("config_updates = %#v", updates)
	}
	colors, _ := updates["colors"].([]interface{})
	if len(colors) != 2 || colors[0] != "#112233" || colors[1] != "#445566" {
		t.Errorf("config_updates.colors = %#v", updates["colors"])
	}
}

func TestChartConfigUpdate_SpacedSmoothBool(t *testing.T) {
	t.Parallel()
	body := parseDryRunBody(t, ChartConfigUpdate, []string{
		"--url", testURL,
		"--sheet-id", testSheetID,
		"--chart-id", "chart-1",
		"--smooth", "false",
	})
	input := decodeToolInput(t, body, "manage_chart_object")
	updates := input["config_updates"].(map[string]interface{})
	if updates["smooth"] != false {
		t.Fatalf("config_updates.smooth = %v, want false", updates["smooth"])
	}
}

func TestChartSemanticShortcuts_CompatibleAliases(t *testing.T) {
	t.Parallel()
	body := parseDryRunBody(t, ChartConfigUpdate, []string{
		"--url", testURL, "--sheet-id", testSheetID, "--chart-id", "chart-1", "--stacked",
	})
	updates := decodeToolInput(t, body, "manage_chart_object")["config_updates"].(map[string]interface{})
	if updates["stack"] != "normal" {
		t.Fatalf("--stacked normalized stack = %v, want normal", updates["stack"])
	}
	body = parseDryRunBody(t, ChartConfigUpdate, []string{
		"--url", testURL, "--sheet-id", testSheetID, "--chart-id", "chart-1", "--data-labels", "category_percentage",
	})
	updates = decodeToolInput(t, body, "manage_chart_object")["config_updates"].(map[string]interface{})
	if updates["data_labels"] != "value_percentage" {
		t.Fatalf("data-labels normalized value = %v, want value_percentage", updates["data_labels"])
	}
}

func TestChartSemanticShortcuts_CompatibleAliasesInBatch(t *testing.T) {
	t.Parallel()
	body := parseDryRunBody(t, BatchUpdate, []string{
		"--url", testURL,
		"--operations", `[{"shortcut":"+chart-config-update","input":{"sheet_id":"sh1","chart_id":"chart-1","stacked":true,"data_labels":"category_percentage","smooth":false}}]`,
		"--yes",
	})
	input := decodeToolInput(t, body, "batch_update")
	ops := input["operations"].([]interface{})
	chartInput := ops[0].(map[string]interface{})["input"].(map[string]interface{})
	updates := chartInput["config_updates"].(map[string]interface{})
	if updates["stack"] != "normal" || updates["data_labels"] != "value_percentage" || updates["smooth"] != false {
		t.Fatalf("batch config_updates = %#v", updates)
	}
}

func TestChartSemanticShortcuts_Validation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
	}{
		{name: "unsupported type", args: []string{"--url", testURL, "--sheet-id", testSheetID, "--chart-type", "donut", "--data-range", "A1:C4"}},
		{name: "invalid semantic enum", args: []string{"--url", testURL, "--sheet-id", testSheetID, "--chart-type", "line", "--data-range", "A1:C4", "--legend-position", "diagonal"}},
		{name: "range too small", args: []string{"--url", testURL, "--sheet-id", testSheetID, "--chart-type", "line", "--data-range", "A1:A4"}},
		{name: "combo needs two series", args: []string{"--url", testURL, "--sheet-id", testSheetID, "--chart-type", "combo", "--data-range", "A1:B4"}},
		{name: "invalid direction", args: []string{"--url", testURL, "--sheet-id", testSheetID, "--chart-type", "line", "--data-range", "A1:C4", "--data-direction", "horizontal"}},
		{name: "colors need two values", args: []string{"--url", testURL, "--sheet-id", testSheetID, "--chart-type", "line", "--data-range", "A1:C4", "--colors", "#112233"}},
		{name: "palette and colors are exclusive", args: []string{"--url", testURL, "--sheet-id", testSheetID, "--chart-type", "line", "--data-range", "A1:C4", "--color-palette", "brandColorSeries@v2", "--colors", "#112233,#445566"}},
		{name: "size must be paired", args: []string{"--url", testURL, "--sheet-id", testSheetID, "--chart-type", "line", "--data-range", "A1:C4", "--width", "640"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := runShortcutCapturingErr(t, ChartCreateBasic, tt.args)
			if err == nil {
				t.Fatal("expected validation error")
			}
		})
	}

	_, _, err := runShortcutCapturingErr(t, ChartConfigUpdate, []string{
		"--url", testURL,
		"--sheet-id", testSheetID,
		"--chart-id", "chart-1",
	})
	if err == nil {
		t.Fatal("expected config update with no changed field to fail")
	}
}
