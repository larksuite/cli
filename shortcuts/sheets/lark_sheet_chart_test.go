// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"reflect"
	"strings"
	"testing"

	"github.com/larksuite/cli/shortcuts/common"
)

func chartDryRunSnapshot(t *testing.T, input map[string]interface{}) map[string]interface{} {
	t.Helper()
	properties, ok := input["properties"].(map[string]interface{})
	if !ok {
		t.Fatalf("input.properties = %#v, want object", input["properties"])
	}
	snapshot, ok := properties["snapshot"].(map[string]interface{})
	if !ok {
		t.Fatalf("input.properties.snapshot = %#v, want object", properties["snapshot"])
	}
	return snapshot
}

func TestChartCreateBasic_AllTypes(t *testing.T) {
	t.Parallel()

	types := []string{"column", "bar", "line", "area", "pie", "scatter", "combo", "radar"}
	for _, chartType := range types {
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
			if basic["x_axis_numbers_as"] != "text" {
				t.Fatalf("x_axis_numbers_as = %v, want text", basic["x_axis_numbers_as"])
			}
		})
	}
}

func TestChartCreateBasic_XAxisNumbersAsValues(t *testing.T) {
	t.Parallel()
	body := parseDryRunBody(t, ChartCreateBasic, []string{
		"--url", testURL,
		"--sheet-id", testSheetID,
		"--chart-type", "scatter",
		"--data-range", "A1:C10",
		"--x-axis-numbers-as", "values",
		"--x-axis-min", "237",
		"--x-axis-max", "239",
		"--y-axis-min", "0.3",
		"--y-axis-max", "3.8",
	})
	basic := decodeToolInput(t, body, "manage_chart_object")["basic_chart"].(map[string]interface{})
	if basic["x_axis_numbers_as"] != "values" {
		t.Fatalf("x_axis_numbers_as = %v, want values", basic["x_axis_numbers_as"])
	}
	if basic["x_axis_min"] != float64(237) || basic["x_axis_max"] != float64(239) {
		t.Fatalf("x axis bounds = %v..%v, want 237..239", basic["x_axis_min"], basic["x_axis_max"])
	}
	if basic["y_axis_min"] != float64(0.3) || basic["y_axis_max"] != float64(3.8) {
		t.Fatalf("y axis bounds = %v..%v, want 0.3..3.8", basic["y_axis_min"], basic["y_axis_max"])
	}
}

func TestChartCreateBasic_XAxisBoundsRequireValues(t *testing.T) {
	t.Parallel()
	_, err := chartCreateBasicInput(newMapFlagViewForCommand("+chart-create-basic", map[string]interface{}{
		"sheet-id":   testSheetID,
		"chart-type": "scatter",
		"data-range": "A1:C10",
		"x-axis-min": 237,
	}), "token", testSheetID, "")
	if err == nil || !strings.Contains(err.Error(), "require --x-axis-numbers-as values") {
		t.Fatalf("error = %v, want numeric X-axis validation", err)
	}
}

func TestChartCreateBasic_XAxisBoundsRejectInvalidRange(t *testing.T) {
	t.Parallel()
	_, err := chartCreateBasicInput(newMapFlagViewForCommand("+chart-create-basic", map[string]interface{}{
		"sheet-id":          testSheetID,
		"chart-type":        "scatter",
		"data-range":        "A1:C10",
		"x-axis-numbers-as": "values",
		"x-axis-min":        239,
		"x-axis-max":        237,
	}), "token", testSheetID, "")
	if err == nil || !strings.Contains(err.Error(), "--x-axis-min must be less than --x-axis-max") {
		t.Fatalf("error = %v, want X-axis bounds validation", err)
	}
}

func TestChartCreateBasic_YAxisBoundsRejectInvalidRange(t *testing.T) {
	t.Parallel()
	_, err := chartCreateBasicInput(newMapFlagViewForCommand("+chart-create-basic", map[string]interface{}{
		"sheet-id":   testSheetID,
		"chart-type": "scatter",
		"data-range": "A1:C10",
		"y-axis-min": 4,
		"y-axis-max": 1,
	}), "token", testSheetID, "")
	if err == nil || !strings.Contains(err.Error(), "--y-axis-min must be less than --y-axis-max") {
		t.Fatalf("error = %v, want Y-axis bounds validation", err)
	}
}

func TestChartCreateBasic_XAxisNumbersAsRejectsInvalidBatchValue(t *testing.T) {
	t.Parallel()
	_, err := chartCreateBasicInput(newMapFlagViewForCommand("+chart-create-basic", map[string]interface{}{
		"sheet-id":          testSheetID,
		"chart-type":        "scatter",
		"data-range":        "A1:C10",
		"x-axis-numbers-as": "auto",
	}), "token", testSheetID, "")
	if err == nil || !strings.Contains(err.Error(), "--x-axis-numbers-as must be text or values") {
		t.Fatalf("error = %v, want x-axis-numbers-as validation", err)
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

func TestChartCreateBasic_DetachedHeaderRange(t *testing.T) {
	t.Parallel()
	dataRange := "'Sheet1'!A2:A10,'Sheet1'!K2:L10"
	headerRange := "'Sheet1'!A1,'Sheet1'!K1:L1"
	body := parseDryRunBody(t, ChartCreateBasic, []string{
		"--url", testURL,
		"--sheet-id", testSheetID,
		"--chart-type", "line",
		"--data-range", dataRange,
		"--header-range", headerRange,
	})
	input := decodeToolInput(t, body, "manage_chart_object")
	basic := input["basic_chart"].(map[string]interface{})
	if basic["data_range"] != dataRange || basic["header_range"] != headerRange {
		t.Fatalf("basic_chart = %#v", basic)
	}
}

func TestChartCreateBasic_MergesMisalignedOrOverlappingRanges(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{name: "separated rows", input: "'Sheet1'!A1:M1,'Sheet1'!A3:M3", expected: "'Sheet1'!A1:M3"},
		{name: "overlapping columns", input: "A1:B10,B1:C10", expected: "A1:C10"},
		{name: "mixed qualifier spelling", input: "Data!A1:B10,'Data'!B1:C10", expected: "Data!A1:C10"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			body := parseDryRunBody(t, ChartCreateBasic, []string{
				"--url", testURL,
				"--sheet-id", testSheetID,
				"--chart-type", "line",
				"--data-range", tt.input,
			})
			input := decodeToolInput(t, body, "manage_chart_object")
			basic := input["basic_chart"].(map[string]interface{})
			if basic["data_range"] != tt.expected {
				t.Fatalf("basic_chart.data_range = %v, want %q", basic["data_range"], tt.expected)
			}
		})
	}
}

func TestChartCreateBasic_PreservesAlignedCrossSheetRanges(t *testing.T) {
	t.Parallel()

	body := parseDryRunBody(t, ChartCreateBasic, []string{
		"--url", testURL,
		"--sheet-id", testSheetID,
		"--chart-type", "line",
		"--data-range", "'A'!A1:A10,'B'!A1:B10",
	})
	input := decodeToolInput(t, body, "manage_chart_object")
	basic := input["basic_chart"].(map[string]interface{})
	if got, want := basic["data_range"], "'A'!A1:A10,'B'!A1:B10"; got != want {
		t.Fatalf("basic_chart.data_range = %v, want %q", got, want)
	}
}

func TestChartSemanticShortcuts_InDedicatedBatch(t *testing.T) {
	body := parseDryRunBody(t, BatchChartCreate, []string{
		"--url", testURL,
		"--operations", `[
			{"sheet-id":"sh1","chart-type":"column","data-range":"A1:C10","title":"Sales"},
			{"sheet-id":"sh1","chart-type":"line","data-range":"E1:G10","title":"Trend"}
		]`,
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
		if chartInput["basic_chart"].(map[string]interface{})["x_axis_numbers_as"] != "text" {
			t.Fatalf("operations[%d].input.basic_chart.x_axis_numbers_as = %#v", i, chartInput["basic_chart"])
		}
	}
}

func TestBatchChartCreate_LegacyWrappedInputStillAccepted(t *testing.T) {
	t.Parallel()
	body := parseDryRunBody(t, BatchChartCreate, []string{
		"--url", testURL,
		"--operations", `[{
			"shortcut":"+chart-create-basic",
			"input":{"sheet_id":"sh1","chart_type":"line","data_range":"A1:C10"}
		}]`,
	})
	input := decodeToolInput(t, body, "batch_update")
	ops := input["operations"].([]interface{})
	if len(ops) != 1 || ops[0].(map[string]interface{})["tool_name"] != "manage_chart_object" {
		t.Fatalf("legacy wrapped operation was not translated: %#v", ops)
	}
}

func TestChartBatches_IgnoredLocatorWarns(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		shortcut common.Shortcut
		command  string
		ops      string
	}{
		{
			name:     "create flat operation",
			shortcut: BatchChartCreate,
			command:  "+batch-chart-create",
			ops:      `[{"sheet_id":"sh1","chart_type":"line","data_range":"A1:C10","url":"https://example.invalid/sheets/shtWRONG"}]`,
		},
		{
			name:     "update wrapped operation",
			shortcut: BatchChartUpdate,
			command:  "+batch-chart-update",
			ops:      `[{"shortcut":"+chart-config-update","input":{"sheet_id":"sh1","chart_id":"chart-1","title":"Sales","spreadsheet_token":"shtWRONG"}}]`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			warning := dryRunWarning(t, tc.shortcut, []string{
				"--url", testURL,
				"--operations", tc.ops,
			})
			for _, want := range []string{"operations[0]", tc.command + " --url/--spreadsheet-token locator is authoritative"} {
				if !strings.Contains(warning, want) {
					t.Errorf("locator warning should contain %q, got %q", want, warning)
				}
			}
		})
	}
}

func TestChartBatches_RejectDuplicateUpdateTargets(t *testing.T) {
	t.Parallel()

	operations := `[
		{"shortcut":"+chart-config-update","input":{"sheet_id":"sh1","chart_id":"chart-1","title":"Sales"}},
		{"shortcut":"+chart-data-update","input":{"sheet_id":"sh1","chart_id":"chart-1","data_range":"A1:C10"}}
	]`
	for _, tc := range []struct {
		name     string
		shortcut common.Shortcut
		args     []string
	}{
		{
			name:     "dedicated chart batch",
			shortcut: BatchChartUpdate,
			args:     []string{"--url", testURL, "--operations", operations},
		},
		{
			name:     "general batch",
			shortcut: BatchUpdate,
			args:     []string{"--url", testURL, "--operations", operations, "--yes"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, _, err := runShortcutCapturingErr(t, tc.shortcut, append(tc.args, "--dry-run"))
			requireValidation(t, err, "both target chart \"chart-1\"")
			if !strings.Contains(err.Error(), "separate batch calls") {
				t.Fatalf("duplicate-target error is not actionable: %v", err)
			}
		})
	}
}

func TestChartConfigUpdate_TipsDescribeSnapshotValidation(t *testing.T) {
	t.Parallel()

	tips := strings.Join(ChartConfigUpdate.Tips, "\n")
	for _, want := range []string{"--dry-run validates the request shape only", "valueType=linear"} {
		if !strings.Contains(tips, want) {
			t.Errorf("tips should contain %q, got %q", want, tips)
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
	snapshot := chartDryRunSnapshot(t, input)
	plotArea := snapshot["plotArea"].(map[string]interface{})
	plot := plotArea["plot"].(map[string]interface{})
	extra := plot["extra"].(map[string]interface{})
	if extra["smooth"] != false || extra["stack"].(map[string]interface{})["percentage"] != true {
		t.Errorf("plot extra = %#v", extra)
	}
	style, _ := snapshot["style"].(map[string]interface{})
	colors, _ := style["colorTheme"].([]interface{})
	if len(colors) != 2 || colors[0] != "#112233" || colors[1] != "#445566" {
		t.Errorf("snapshot.style.colorTheme = %#v", style["colorTheme"])
	}
	axes := plotArea["axes"].([]interface{})
	if axes[0].(map[string]interface{})["title"].(map[string]interface{})["text"] != "Revenue" {
		t.Errorf("axes = %#v", axes)
	}
}

func TestChartConfigUpdate_StackNonePreservesExplicitFalse(t *testing.T) {
	t.Parallel()
	body := parseDryRunBody(t, ChartConfigUpdate, []string{
		"--url", testURL,
		"--sheet-id", testSheetID,
		"--chart-id", "chart-1",
		"--stack", "none",
	})
	plot := chartDryRunSnapshot(t, decodeToolInput(t, body, "manage_chart_object"))["plotArea"].(map[string]interface{})["plot"].(map[string]interface{})
	stack := plot["extra"].(map[string]interface{})["stack"].(map[string]interface{})
	if stack["enabled"] != false {
		t.Fatalf("stack = %#v, want enabled=false", stack)
	}
}

func TestApplyChartConfigPatch_StackNoneIsExplicitForWaterfallOnly(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		chartType string
		explicit  bool
	}{
		{chartType: "waterfall", explicit: true},
		{chartType: "column", explicit: false},
	} {
		t.Run(tc.chartType, func(t *testing.T) {
			current := map[string]interface{}{
				"plotArea": map[string]interface{}{
					"plot": map[string]interface{}{
						"type":  tc.chartType,
						"extra": map[string]interface{}{"stack": map[string]interface{}{"percentage": false}},
					},
				},
			}
			patch, _ := applyChartConfigPatch(current, map[string]interface{}{"stack": "none"})
			plot := patch["plotArea"].(map[string]interface{})["plot"].(map[string]interface{})
			stack, ok := plot["extra"].(map[string]interface{})["stack"].(map[string]interface{})
			if tc.explicit {
				if !ok || stack["enabled"] != false {
					t.Fatalf("stack = %#v, want enabled=false", stack)
				}
			} else if ok {
				t.Fatalf("stack = %#v, want removed", stack)
			}
		})
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
	plot := chartDryRunSnapshot(t, input)["plotArea"].(map[string]interface{})["plot"].(map[string]interface{})
	if plot["extra"].(map[string]interface{})["smooth"] != false {
		t.Fatalf("snapshot smooth = %v, want false", plot)
	}
}

func TestChartConfigUpdate_XAxisBounds(t *testing.T) {
	t.Parallel()
	body := parseDryRunBody(t, ChartConfigUpdate, []string{
		"--url", testURL,
		"--sheet-id", testSheetID,
		"--chart-id", "chart-1",
		"--x-axis-min", "237",
		"--x-axis-max", "239",
		"--y-axis-min", "0.3",
		"--y-axis-max", "3.8",
	})
	snapshot := chartDryRunSnapshot(t, decodeToolInput(t, body, "manage_chart_object"))
	axes := snapshot["plotArea"].(map[string]interface{})["axes"].([]interface{})
	xAxis := axes[0].(map[string]interface{})
	if xAxis["min"] != float64(237) || xAxis["max"] != float64(239) {
		t.Fatalf("x axis = %#v, want min=237 max=239", xAxis)
	}
	yAxis := axes[1].(map[string]interface{})
	if yAxis["min"] != float64(0.3) || yAxis["max"] != float64(3.8) {
		t.Fatalf("y axis = %#v, want min=0.3 max=3.8", yAxis)
	}
}

func TestChartConfigUpdate_XAxisBoundsRequireContinuousExistingAxis(t *testing.T) {
	t.Parallel()
	fv := newMapFlagViewForCommand("+chart-config-update", map[string]interface{}{
		"sheet-id":   testSheetID,
		"chart-id":   "chart-1",
		"x-axis-min": 237,
	})

	ordinal := map[string]interface{}{
		"plotArea": map[string]interface{}{
			"axes": []interface{}{
				map[string]interface{}{"type": "x", "position": "bottom", "valueType": "ordinal"},
			},
		},
	}
	if _, _, err := chartConfigUpdateInputFromSnapshot(fv, "token", testSheetID, "", ordinal); err == nil ||
		!strings.Contains(err.Error(), "continuous numeric X axis") {
		t.Fatalf("error = %v, want existing continuous X-axis validation", err)
	}

	linear := map[string]interface{}{
		"plotArea": map[string]interface{}{
			"axes": []interface{}{
				map[string]interface{}{"type": "x", "position": "bottom", "valueType": "linear"},
			},
		},
	}
	input, _, err := chartConfigUpdateInputFromSnapshot(fv, "token", testSheetID, "", linear)
	if err != nil {
		t.Fatalf("linear X axis rejected: %v", err)
	}
	xAxis := chartDryRunSnapshot(t, input)["plotArea"].(map[string]interface{})["axes"].([]interface{})[0].(map[string]interface{})
	if xAxis["min"] != float64(237) {
		t.Fatalf("x axis = %#v, want min=237", xAxis)
	}
}

func TestChartSemanticShortcuts_CompatibleAliases(t *testing.T) {
	t.Parallel()
	chartCreateBasic := shortcutFromRegistry(t, "+chart-create-basic")
	chartConfigUpdate := shortcutFromRegistry(t, "+chart-config-update")
	body := parseDryRunBody(t, chartCreateBasic, []string{
		"--url", testURL,
		"--sheet-id", testSheetID,
		"--type", "line",
		"--range", "A1:C10",
		"--x-axis", "Month",
		"--y-axis", "Revenue",
	})
	basic := decodeToolInput(t, body, "manage_chart_object")["basic_chart"].(map[string]interface{})
	if basic["chart_type"] != "line" || basic["data_range"] != "A1:C10" ||
		basic["x_axis_title"] != "Month" || basic["y_axis_title"] != "Revenue" {
		t.Fatalf("chart create aliases = %#v", basic)
	}
	body = parseDryRunBody(t, chartConfigUpdate, []string{
		"--url", testURL, "--sheet-id", testSheetID, "--chart-id", "chart-1", "--stacked",
	})
	snapshot := chartDryRunSnapshot(t, decodeToolInput(t, body, "manage_chart_object"))
	extra := snapshot["plotArea"].(map[string]interface{})["plot"].(map[string]interface{})["extra"].(map[string]interface{})
	if extra["stack"].(map[string]interface{})["percentage"] != false {
		t.Fatalf("--stacked normalized stack = %#v, want non-percentage stack", extra["stack"])
	}
	body = parseDryRunBody(t, chartConfigUpdate, []string{
		"--url", testURL,
		"--sheet-id", testSheetID,
		"--chart-id", "chart-1",
		"--x-axis", "Month",
		"--y-axis", "Revenue",
		"--data-labels", "percentage,value",
	})
	snapshot = chartDryRunSnapshot(t, decodeToolInput(t, body, "manage_chart_object"))
	plotArea := snapshot["plotArea"].(map[string]interface{})
	axes := plotArea["axes"].([]interface{})
	labels := plotArea["plot"].(map[string]interface{})["labels"].(map[string]interface{})
	if axes[0].(map[string]interface{})["title"].(map[string]interface{})["text"] != "Month" ||
		axes[1].(map[string]interface{})["title"].(map[string]interface{})["text"] != "Revenue" ||
		labels["value"] != true || labels["percentage"] != true {
		t.Fatalf("chart config aliases = %#v", snapshot)
	}
}

func TestChartSemanticShortcuts_StackedFalseDisablesStacking(t *testing.T) {
	t.Parallel()

	body := parseDryRunBody(t, ChartConfigUpdate, []string{
		"--url", testURL,
		"--sheet-id", testSheetID,
		"--chart-id", "chart-1",
		"--stacked=false",
	})
	plot := chartDryRunSnapshot(t, decodeToolInput(t, body, "manage_chart_object"))["plotArea"].(map[string]interface{})["plot"].(map[string]interface{})
	stack := plot["extra"].(map[string]interface{})["stack"].(map[string]interface{})
	if stack["enabled"] != false {
		t.Fatalf("--stacked=false stack = %#v, want enabled=false", stack)
	}

	body = parseDryRunBody(t, BatchChartUpdate, []string{
		"--url", testURL,
		"--operations", `[{"shortcut":"+chart-config-update","input":{"sheet_id":"sh1","chart_id":"chart-1","stacked":false}}]`,
	})
	input := decodeToolInput(t, body, "batch_update")
	operation := input["operations"].([]interface{})[0].(map[string]interface{})
	plot = chartDryRunSnapshot(t, operation["input"].(map[string]interface{}))["plotArea"].(map[string]interface{})["plot"].(map[string]interface{})
	stack = plot["extra"].(map[string]interface{})["stack"].(map[string]interface{})
	if stack["enabled"] != false {
		t.Fatalf("batch stacked=false stack = %#v, want enabled=false", stack)
	}
}

func TestChartSemanticShortcuts_DataLabelCombinations(t *testing.T) {
	t.Parallel()
	chartConfigUpdate := shortcutFromRegistry(t, "+chart-config-update")
	tests := []struct {
		input      string
		category   bool
		value      bool
		percentage bool
	}{
		{"value", false, true, false},
		{"category", true, false, false},
		{"percentage", false, false, true},
		{"value_category", true, true, false},
		{"value_percentage", false, true, true},
		{"category_percentage", true, false, true},
		{"value_category_percentage", true, true, true},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			body := parseDryRunBody(t, chartConfigUpdate, []string{
				"--url", testURL,
				"--sheet-id", testSheetID,
				"--chart-id", "chart-1",
				"--data-labels", tc.input,
			})
			snapshot := chartDryRunSnapshot(t, decodeToolInput(t, body, "manage_chart_object"))
			labels := snapshot["plotArea"].(map[string]interface{})["plot"].(map[string]interface{})["labels"].(map[string]interface{})
			if labels["category"] != tc.category || labels["value"] != tc.value || labels["percentage"] != tc.percentage {
				t.Fatalf("%s labels = %#v, want category=%t value=%t percentage=%t", tc.input, labels, tc.category, tc.value, tc.percentage)
			}
		})
	}
}

func TestChartConfigUpdate_DataLabelPositionDoesNotEnableLabels(t *testing.T) {
	t.Parallel()
	current := map[string]interface{}{
		"plotArea": map[string]interface{}{
			"plot": map[string]interface{}{"type": "line"},
		},
	}
	patch, viewModel := applyChartConfigPatch(current, map[string]interface{}{"data_label_position": "top"})
	if len(patch) != 0 {
		t.Fatalf("patch = %#v, want no-op when labels are absent", patch)
	}
	plot := viewModel["plotArea"].(map[string]interface{})["plot"].(map[string]interface{})
	if _, ok := plot["labels"]; ok {
		t.Fatalf("labels = %#v, position-only update must not enable labels", plot["labels"])
	}
	fv := newMapFlagViewForCommand("+chart-config-update", map[string]interface{}{
		"sheet-id":            testSheetID,
		"chart-id":            "chart-1",
		"data-label-position": "top",
	})
	if _, _, err := chartConfigUpdateInputFromSnapshot(fv, "token", testSheetID, "", current); err == nil ||
		!strings.Contains(err.Error(), "requires existing data labels") {
		t.Fatalf("error = %v, want position-only update to reject a chart without labels", err)
	}
	withLabels := map[string]interface{}{
		"plotArea": map[string]interface{}{
			"plot": map[string]interface{}{
				"type":   "line",
				"labels": map[string]interface{}{"value": true},
			},
		},
	}
	patch, _ = applyChartConfigPatch(withLabels, map[string]interface{}{"data_label_position": "top"})
	labels := patch["plotArea"].(map[string]interface{})["plot"].(map[string]interface{})["labels"].(map[string]interface{})
	if labels["value"] != true || labels["position"] != "top" {
		t.Fatalf("labels = %#v, want existing content preserved with position=top", labels)
	}
}

func TestChartSemanticShortcuts_LastPointLabel(t *testing.T) {
	t.Parallel()
	chartConfigUpdate := shortcutFromRegistry(t, "+chart-config-update")
	for _, tc := range []struct {
		arg  string
		want bool
	}{
		{arg: "true", want: true},
		{arg: "false", want: false},
	} {
		body := parseDryRunBody(t, chartConfigUpdate, []string{
			"--url", testURL,
			"--sheet-id", testSheetID,
			"--chart-id", "chart-1",
			"--last-point-label=" + tc.arg,
		})
		input := decodeToolInput(t, body, "manage_chart_object")
		if _, ok := input["last_point_label"]; ok {
			t.Fatalf("--last-point-label=%s must not be written at the tool input root: %#v", tc.arg, input)
		}
		properties := input["properties"].(map[string]interface{})
		if properties["last_point_label"] != tc.want {
			t.Fatalf("--last-point-label=%s input = %#v, want %t", tc.arg, input, tc.want)
		}
	}
}

func TestChartSemanticShortcuts_CompatibleAliasesInBatch(t *testing.T) {
	t.Parallel()
	body := parseDryRunBody(t, BatchChartUpdate, []string{
		"--url", testURL,
		"--operations", `[{"shortcut":"+chart-config-update","input":{"sheet_id":"sh1","chart_id":"chart-1","stacked":true,"x_axis":"Month","y_axis":"Revenue","data_labels":"value,percentage","smooth":false}}]`,
	})
	input := decodeToolInput(t, body, "batch_update")
	ops := input["operations"].([]interface{})
	chartInput := ops[0].(map[string]interface{})["input"].(map[string]interface{})
	snapshot := chartDryRunSnapshot(t, chartInput)
	plotArea := snapshot["plotArea"].(map[string]interface{})
	plot := plotArea["plot"].(map[string]interface{})
	labels := plot["labels"].(map[string]interface{})
	extra := plot["extra"].(map[string]interface{})
	if labels["value"] != true || labels["percentage"] != true || extra["smooth"] != false ||
		extra["stack"].(map[string]interface{})["percentage"] != false {
		t.Fatalf("batch config patch = %#v", snapshot)
	}
}

func TestChartSemanticShortcuts_RejectsSingleCustomColor(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		shortcut common.Shortcut
		args     []string
	}{
		{
			name:     "standalone create",
			shortcut: ChartCreateBasic,
			args: []string{
				"--url", testURL,
				"--sheet-id", testSheetID,
				"--chart-type", "line",
				"--data-range", "A1:C10",
				"--colors", "#112233",
			},
		},
		{
			name:     "standalone config update",
			shortcut: ChartConfigUpdate,
			args: []string{
				"--url", testURL,
				"--sheet-id", testSheetID,
				"--chart-id", "chart-1",
				"--colors", "#223344",
			},
		},
		{
			name:     "batch create",
			shortcut: BatchChartCreate,
			args: []string{
				"--url", testURL,
				"--operations", `[{"sheet_id":"sh1","type":"line","range":"A1:C10","colors":["#445566"]}]`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, _, err := runShortcutCapturingErr(t, tt.shortcut, tt.args)
			requireValidation(t, err, "at least two hex colors")
		})
	}
}

func TestChartCreateBasic_RejectsMoreThanFiftySeries(t *testing.T) {
	t.Parallel()
	_, _, err := runShortcutCapturingErr(t, ChartCreateBasic, []string{
		"--url", testURL,
		"--sheet-id", testSheetID,
		"--chart-type", "line",
		"--data-range", "A2:DV7",
		"--data-direction", "column",
	})
	requireValidation(t, err, "create 125 series")
	if !strings.Contains(err.Error(), "current limit of 50") ||
		!strings.Contains(err.Error(), "compact summary table") {
		t.Fatalf("series limit error is not actionable: %v", err)
	}
}

func TestChartCreateBasic_SelectsDimensionsAtCreation(t *testing.T) {
	t.Parallel()
	body := parseDryRunBody(t, ChartCreateBasic, []string{
		"--url", testURL,
		"--sheet-id", testSheetID,
		"--chart-type", "line",
		"--data-range", "A1:DV7",
		"--dim1-index", "3",
		"--dim2-indexes", "2,6,8",
	})
	basic := decodeToolInput(t, body, "manage_chart_object")["basic_chart"].(map[string]interface{})
	if basic["dim1_index"] != float64(3) {
		t.Fatalf("basic_chart.dim1_index = %#v", basic["dim1_index"])
	}
	indexes := basic["dim2_indexes"].([]interface{})
	if len(indexes) != 3 || indexes[0] != float64(2) || indexes[1] != float64(6) || indexes[2] != float64(8) {
		t.Fatalf("basic_chart.dim2_indexes = %#v", indexes)
	}
}

func TestChartCreateBasic_ConfiguresComboSeriesSemantically(t *testing.T) {
	t.Parallel()
	body := parseDryRunBody(t, ChartCreateBasic, []string{
		"--url", testURL,
		"--sheet-id", testSheetID,
		"--chart-type", "combo",
		"--data-range", "A1:D7",
		"--dim2-indexes", "2,3,4",
		"--series-types", "column,column,line",
		"--series-y-axes", "left,left,right",
	})
	basic := decodeToolInput(t, body, "manage_chart_object")["basic_chart"].(map[string]interface{})
	if got := basic["series_types"]; !reflect.DeepEqual(got, []interface{}{"column", "column", "line"}) {
		t.Fatalf("basic_chart.series_types = %#v", got)
	}
	if got := basic["series_y_axes"]; !reflect.DeepEqual(got, []interface{}{"left", "left", "right"}) {
		t.Fatalf("basic_chart.series_y_axes = %#v", got)
	}
}

func TestChartCreateBasic_ConfiguresComboSeriesSemanticallyInBatch(t *testing.T) {
	t.Parallel()
	body := parseDryRunBody(t, BatchChartCreate, []string{
		"--url", testURL,
		"--operations", `[{"sheet_id":"sh1","chart_type":"combo","data_range":"A1:D7","dim2_indexes":[2,3,4],"series_types":["column","column","line"],"series_y_axes":["left","left","right"]}]`,
	})
	input := decodeToolInput(t, body, "batch_update")
	ops := input["operations"].([]interface{})
	basic := ops[0].(map[string]interface{})["input"].(map[string]interface{})["basic_chart"].(map[string]interface{})
	if got := basic["series_types"]; !reflect.DeepEqual(got, []interface{}{"column", "column", "line"}) {
		t.Fatalf("batch basic_chart.series_types = %#v", got)
	}
	if got := basic["series_y_axes"]; !reflect.DeepEqual(got, []interface{}{"left", "left", "right"}) {
		t.Fatalf("batch basic_chart.series_y_axes = %#v", got)
	}
}

func TestChartCreateBasic_ValidatesComboSeriesSemantics(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "non combo",
			args: []string{"--chart-type", "line", "--data-range", "A1:C7", "--series-y-axes", "left,right"},
			want: "only valid for combo charts",
		},
		{
			name: "type count",
			args: []string{"--chart-type", "combo", "--data-range", "A1:D7", "--series-types", "column,line"},
			want: "one value per selected value series",
		},
		{
			name: "invalid axis",
			args: []string{"--chart-type", "combo", "--data-range", "A1:C7", "--series-y-axes", "left,secondary"},
			want: "expected one of left, right",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			args := append([]string{"--url", testURL, "--sheet-id", testSheetID}, tc.args...)
			_, _, err := runShortcutCapturingErr(t, ChartCreateBasic, args)
			requireValidation(t, err, tc.want)
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want substring %q", err, tc.want)
			}
		})
	}
}

func TestChartCreateBasic_SelectsDimensionsInBatch(t *testing.T) {
	t.Parallel()
	body := parseDryRunBody(t, BatchChartCreate, []string{
		"--url", testURL,
		"--operations", `[{
			"sheet_id":"sh1","chart_type":"line","data_range":"A1:DV7","dim1_index":3,"dim2_indexes":[2,6,8]
		}]`,
	})
	input := decodeToolInput(t, body, "batch_update")
	ops := input["operations"].([]interface{})
	basic := ops[0].(map[string]interface{})["input"].(map[string]interface{})["basic_chart"].(map[string]interface{})
	if basic["dim1_index"] != float64(3) {
		t.Fatalf("batch basic_chart.dim1_index = %#v", basic["dim1_index"])
	}
	indexes := basic["dim2_indexes"].([]interface{})
	if len(indexes) != 3 || indexes[0] != float64(2) || indexes[1] != float64(6) || indexes[2] != float64(8) {
		t.Fatalf("batch basic_chart.dim2_indexes = %#v", indexes)
	}
}

func TestChartCreateBasic_BubbleRoleIndexes(t *testing.T) {
	t.Parallel()
	body := parseDryRunBody(t, ChartCreateBasic, []string{
		"--url", testURL,
		"--sheet-id", testSheetID,
		"--chart-type", "bubble",
		"--data-range", "A1:E10",
		"--key-index", "2",
		"--x-index", "1",
		"--y-index", "3",
		"--size-index", "5",
	})
	basic := decodeToolInput(t, body, "manage_chart_object")["basic_chart"].(map[string]interface{})
	for name, want := range map[string]float64{
		"key_index":  2,
		"x_index":    1,
		"y_index":    3,
		"size_index": 5,
	} {
		if got := basic[name]; got != want {
			t.Fatalf("basic_chart.%s = %#v, want %v", name, got, want)
		}
	}
	if _, exists := basic["group_index"]; exists {
		t.Fatalf("basic_chart unexpectedly contains group_index: %#v", basic)
	}
	if _, exists := basic["dim2_indexes"]; exists {
		t.Fatalf("basic_chart unexpectedly contains dim2_indexes: %#v", basic)
	}
}

func TestChartCreateBasic_BubbleRoleIndexesInBatch(t *testing.T) {
	t.Parallel()
	body := parseDryRunBody(t, BatchChartCreate, []string{
		"--url", testURL,
		"--operations", `[{"sheet_id":"sh1","chart_type":"bubble","data_range":"A1:E10","key_index":1,"x_index":2,"y_index":3,"group_index":4,"size_index":5}]`,
	})
	input := decodeToolInput(t, body, "batch_update")
	ops := input["operations"].([]interface{})
	basic := ops[0].(map[string]interface{})["input"].(map[string]interface{})["basic_chart"].(map[string]interface{})
	if basic["x_index"] != float64(2) || basic["size_index"] != float64(5) {
		t.Fatalf("batch bubble roles = %#v", basic)
	}
}

func TestChartCreateBasic_RejectsHorizontalHeaderForRowDirection(t *testing.T) {
	t.Parallel()
	_, _, err := runShortcutCapturingErr(t, ChartCreateBasic, []string{
		"--url", testURL,
		"--sheet-id", testSheetID,
		"--chart-type", "line",
		"--data-range", "'Sheet1'!A3:M3,'Sheet1'!A5:M5",
		"--header-range", "'Sheet1'!A1:M1",
		"--data-direction", "row",
	})
	requireValidation(t, err, "looks like a category row")
	if !strings.Contains(err.Error(), "include it in --data-range") {
		t.Fatalf("header-range error is not actionable: %v", err)
	}
}

func TestChartCreateBasic_RejectsHeaderCountMismatch(t *testing.T) {
	t.Parallel()
	_, _, err := runShortcutCapturingErr(t, ChartCreateBasic, []string{
		"--url", testURL,
		"--sheet-id", testSheetID,
		"--chart-type", "line",
		"--data-range", "A2:C4",
		"--header-range", "A1:B1",
	})
	requireValidation(t, err, "provides 2 headers but --data-range has 3 dimensions")
}

func TestChartCreateBasic_SuggestsRowDirectionForHorizontalCategories(t *testing.T) {
	t.Parallel()
	_, _, err := runShortcutCapturingErr(t, ChartCreateBasic, []string{
		"--url", testURL,
		"--sheet-id", testSheetID,
		"--chart-type", "line",
		"--data-range", "'Sheet1'!A1:M1",
	})
	requireValidation(t, err, "--data-direction row")
}

func TestChartDataUpdate_MapsToPartialProperties(t *testing.T) {
	t.Parallel()
	body := parseDryRunBody(t, ChartDataUpdate, []string{
		"--url", testURL,
		"--sheet-id", testSheetID,
		"--chart-id", "chart-1",
		"--data-range", "'Sheet1'!A1:M6",
	})
	input := decodeToolInput(t, body, "manage_chart_object")
	if input["operation"] != "update" || input["chart_id"] != "chart-1" {
		t.Fatalf("input = %#v", input)
	}
	data := chartDryRunSnapshot(t, input)["data"].(map[string]interface{})
	refs := data["refs"].([]interface{})
	if refs[0].(map[string]interface{})["value"] != "'Sheet1'!A1:M6" {
		t.Errorf("data patch = %#v", data)
	}
	if _, ok := data["direction"]; ok {
		t.Errorf("omitted --data-direction must be resolved from the current snapshot during execution: %#v", data)
	}
}

func TestChartDataUpdate_ExplicitDirectionAndMultipleRanges(t *testing.T) {
	t.Parallel()
	dataRange := "'Sheet1'!A1:A10,'Sheet2'!A1:B10"
	body := parseDryRunBody(t, ChartDataUpdate, []string{
		"--url", testURL,
		"--sheet-id", testSheetID,
		"--chart-id", "chart-1",
		"--data-range", dataRange,
		"--data-direction", "column",
	})
	input := decodeToolInput(t, body, "manage_chart_object")
	data := chartDryRunSnapshot(t, input)["data"].(map[string]interface{})
	refs := data["refs"].([]interface{})
	if data["direction"] != "column" || len(refs) != 2 {
		t.Errorf("data patch = %#v", data)
	}
	if refs[0].(map[string]interface{})["value"] != "'Sheet1'!A1:A10" ||
		refs[1].(map[string]interface{})["value"] != "'Sheet2'!A1:B10" {
		t.Errorf("cross-sheet refs = %#v, want %q", refs, dataRange)
	}
}

func TestChartDataUpdate_RejectsHeaderDirectionDuringDryRun(t *testing.T) {
	t.Parallel()
	_, _, err := runShortcutCapturingErr(t, ChartDataUpdate, []string{
		"--url", testURL,
		"--sheet-id", testSheetID,
		"--chart-id", "chart-1",
		"--data-range", "A2:C4",
		"--data-direction", "row",
		"--header-range", "A1:M1",
	})
	requireValidation(t, err, "row-oriented --header-range must be one column")
}

func TestChartDataUpdate_RejectsInvalidRangeNormalizationDuringDryRun(t *testing.T) {
	t.Parallel()
	_, _, err := runShortcutCapturingErr(t, ChartDataUpdate, []string{
		"--url", testURL,
		"--sheet-id", testSheetID,
		"--chart-id", "chart-1",
		"--data-range", "'Sheet1'!A1:A10,'Sheet2'!B2:B10",
		"--data-direction", "column",
	})
	requireValidation(t, err, "cross-sheet --data-range items must align")
}

func TestChartDataUpdate_NormalizationNoticeOnlyWhenRangesMerge(t *testing.T) {
	t.Parallel()

	snapshot := map[string]interface{}{
		"plotArea": map[string]interface{}{"plot": map[string]interface{}{"type": "line"}},
	}
	for _, tc := range []struct {
		name       string
		dataRange  string
		wantNotice bool
	}{
		{name: "spacing only", dataRange: "A1:B10, D1:E10", wantNotice: false},
		{name: "unaligned ranges", dataRange: "A1:B10,C2:D10", wantNotice: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fv := newMapFlagViewForCommand("+chart-data-update", map[string]interface{}{
				"sheet_id":       "sh1",
				"chart_id":       "chart-1",
				"data_range":     tc.dataRange,
				"data_direction": "column",
			})
			_, _, _, notice, err := chartDataUpdateInputFromSnapshot(fv, testToken, "sh1", "", snapshot)
			if err != nil {
				t.Fatalf("chart data update input failed: %v", err)
			}
			if got := notice != ""; got != tc.wantNotice {
				t.Fatalf("notice = %q, want present=%t", notice, tc.wantNotice)
			}
		})
	}
}

func TestChartDataUpdate_DetachedHeaderRange(t *testing.T) {
	t.Parallel()
	dataRange := "'Sheet1'!A2:A10,'Sheet1'!K2:L10"
	headerRange := "'Sheet1'!A1,'Sheet1'!K1:L1"
	body := parseDryRunBody(t, ChartDataUpdate, []string{
		"--url", testURL,
		"--sheet-id", testSheetID,
		"--chart-id", "chart-1",
		"--data-range", dataRange,
		"--header-range", headerRange,
	})
	input := decodeToolInput(t, body, "manage_chart_object")
	data := chartDryRunSnapshot(t, input)["data"].(map[string]interface{})
	if data["headerMode"] != "detached" {
		t.Fatalf("data patch = %#v", data)
	}
	dim1 := data["dim1"].(map[string]interface{})["serie"].(map[string]interface{})
	if dim1["nameRef"] != "'Sheet1'!A1" {
		t.Fatalf("detached dim1 = %#v", dim1)
	}
}

func TestChartDataUpdate_ExplicitSeriesIndexes(t *testing.T) {
	t.Parallel()
	body := parseDryRunBody(t, ChartDataUpdate, []string{
		"--url", testURL,
		"--sheet-id", testSheetID,
		"--chart-id", "chart-1",
		"--data-range", "'Sheet1'!A1:M6",
		"--dim1-index", "1",
		"--dim2-indexes", "4, 8",
	})
	input := decodeToolInput(t, body, "manage_chart_object")
	data := chartDryRunSnapshot(t, input)["data"].(map[string]interface{})
	dim1 := data["dim1"].(map[string]interface{})["serie"].(map[string]interface{})
	if dim1["index"] != float64(1) {
		t.Errorf("data.dim1 = %#v", dim1)
	}
	series := data["dim2"].(map[string]interface{})["series"].([]interface{})
	if len(series) != 2 || series[0].(map[string]interface{})["index"] != float64(4) ||
		series[1].(map[string]interface{})["index"] != float64(8) {
		t.Errorf("data.dim2 = %#v", series)
	}
}

func TestChartDataUpdate_BubbleRoleIndexes(t *testing.T) {
	t.Parallel()
	body := parseDryRunBody(t, ChartDataUpdate, []string{
		"--url", testURL,
		"--sheet-id", testSheetID,
		"--chart-id", "chart-1",
		"--data-range", "A1:E10",
		"--key-index", "1",
		"--x-index", "2",
		"--y-index", "3",
		"--size-index", "5",
	})
	data := chartDryRunSnapshot(t, decodeToolInput(t, body, "manage_chart_object"))["data"].(map[string]interface{})
	series := data["dim2"].(map[string]interface{})["series"].([]interface{})
	if len(series) != 3 || series[2].(map[string]interface{})["role"] != "size" ||
		series[2].(map[string]interface{})["index"] != float64(5) {
		t.Fatalf("bubble data update roles = %#v", series)
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
		{name: "colors cannot be empty", args: []string{"--url", testURL, "--sheet-id", testSheetID, "--chart-type", "line", "--data-range", "A1:C4", "--colors", ""}},
		{name: "palette and colors are exclusive", args: []string{"--url", testURL, "--sheet-id", testSheetID, "--chart-type", "line", "--data-range", "A1:C4", "--color-palette", "brandColorSeries@v2", "--colors", "#112233,#445566"}},
		{name: "size must be paired", args: []string{"--url", testURL, "--sheet-id", testSheetID, "--chart-type", "line", "--data-range", "A1:C4", "--width", "640"}},
		{name: "misaligned cross-sheet ranges", args: []string{"--url", testURL, "--sheet-id", testSheetID, "--chart-type", "line", "--data-range", "'A'!A1:A4,'B'!B2:C4"}},
		{name: "bubble roles on non-bubble chart", args: []string{"--url", testURL, "--sheet-id", testSheetID, "--chart-type", "line", "--data-range", "A1:C4", "--x-index", "2", "--y-index", "3"}},
		{name: "bubble roles require x and y", args: []string{"--url", testURL, "--sheet-id", testSheetID, "--chart-type", "bubble", "--data-range", "A1:C4", "--x-index", "2"}},
		{name: "bubble roles cannot mix generic indexes", args: []string{"--url", testURL, "--sheet-id", testSheetID, "--chart-type", "bubble", "--data-range", "A1:C4", "--dim1-index", "1", "--x-index", "2", "--y-index", "3"}},
		{name: "bubble roles must be distinct", args: []string{"--url", testURL, "--sheet-id", testSheetID, "--chart-type", "bubble", "--data-range", "A1:C4", "--x-index", "2", "--y-index", "2"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := runShortcutCapturingErr(t, ChartCreateBasic, tt.args)
			if err == nil {
				t.Fatal("expected validation error")
			}
		})
	}

	body := parseDryRunBody(t, ChartCreateBasic, []string{
		"--url", testURL,
		"--sheet-id", testSheetID,
		"--chart-type", "line",
		"--data-range", "A2:C4",
		"--header-range", "'A'!A1,'B'!B1:C1",
	})
	input := decodeToolInput(t, body, "manage_chart_object")
	basic := input["basic_chart"].(map[string]interface{})
	if got, want := basic["header_range"], "'A'!A1,'B'!B1:C1"; got != want {
		t.Fatalf("basic_chart.header_range = %v, want %q", got, want)
	}

	_, _, err := runShortcutCapturingErr(t, ChartConfigUpdate, []string{
		"--url", testURL,
		"--sheet-id", testSheetID,
		"--chart-id", "chart-1",
	})
	if err == nil {
		t.Fatal("expected config update with no changed field to fail")
	}

	for _, args := range [][]string{
		{"--url", testURL, "--sheet-id", testSheetID, "--data-range", "A1:C4"},
		{"--url", testURL, "--sheet-id", testSheetID, "--chart-id", "chart-1"},
		{"--url", testURL, "--sheet-id", testSheetID, "--chart-id", "chart-1", "--data-range", "A1:C4", "--data-direction", "horizontal"},
		{"--url", testURL, "--sheet-id", testSheetID, "--chart-id", "chart-1", "--data-range", "A1:C4", "--dim1-index", "0"},
		{"--url", testURL, "--sheet-id", testSheetID, "--chart-id", "chart-1", "--data-range", "A1:C4", "--dim2-indexes", "2,2"},
		{"--url", testURL, "--sheet-id", testSheetID, "--chart-id", "chart-1", "--data-range", "A1:C4", "--dim2-indexes", "1,2"},
	} {
		_, _, err = runShortcutCapturingErr(t, ChartDataUpdate, args)
		if err == nil {
			t.Fatalf("expected chart data update validation error for args %#v", args)
		}
	}
}
