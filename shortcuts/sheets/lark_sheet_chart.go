// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"context"
	"encoding/json"
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/larksuite/cli/shortcuts/common"
	"github.com/spf13/cobra"
)

var chartHexColorPattern = regexp.MustCompile(`^#?[0-9A-Fa-f]{6}([0-9A-Fa-f]{2})?$`)

// Sheet currently accepts at most 50 value series in one chart. Keep the
// client-side guard aligned with the tool-layer contract so an obviously wide
// source fails before a write call and tells the agent how to recover.
const chartSeriesMaxCount = 50

type chartRoleIndex struct {
	flag  string
	role  string
	index int
}

var bubbleRoleIndexFlags = []struct {
	flag string
	role string
}{
	{flag: "x-index", role: "x"},
	{flag: "y-index", role: "y"},
	{flag: "group-index", role: "group"},
	{flag: "size-index", role: "size"},
}

func resolveBubbleRoleIndexes(rt flagView, chartType string, dimensionCount int) (int, []chartRoleIndex, bool, error) {
	semantic := rt.Changed("key-index")
	for _, item := range bubbleRoleIndexFlags {
		semantic = semantic || rt.Changed(item.flag)
	}
	if !semantic {
		return 0, nil, false, nil
	}
	if chartType != "" && chartType != "bubble" {
		return 0, nil, false, sheetsValidationForFlag("key-index", "bubble role indexes are only valid with --chart-type bubble")
	}
	if rt.Changed("dim1-index") || rt.Changed("dim2-indexes") {
		return 0, nil, false, sheetsValidationForFlag("key-index", "bubble role indexes must not be combined with --dim1-index or --dim2-indexes")
	}
	if !rt.Changed("x-index") || !rt.Changed("y-index") {
		return 0, nil, false, sheetsValidationForFlag("x-index", "--x-index and --y-index must be provided together for a bubble chart")
	}

	keyIndex := 1
	if rt.Changed("key-index") {
		keyIndex = rt.Int("key-index")
	}
	if keyIndex < 1 || (dimensionCount > 0 && keyIndex > dimensionCount) {
		return 0, nil, false, sheetsValidationForFlag("key-index", "--key-index must be between 1 and %d for the selected --data-range", dimensionCount)
	}

	seen := map[int]string{keyIndex: "key"}
	series := make([]chartRoleIndex, 0, len(bubbleRoleIndexFlags))
	for _, item := range bubbleRoleIndexFlags {
		if !rt.Changed(item.flag) {
			continue
		}
		index := rt.Int(item.flag)
		if index < 1 || (dimensionCount > 0 && index > dimensionCount) {
			return 0, nil, false, sheetsValidationForFlag(item.flag, "--%s must be between 1 and %d for the selected --data-range", item.flag, dimensionCount)
		}
		if existingRole, exists := seen[index]; exists {
			return 0, nil, false, sheetsValidationForFlag(item.flag, "--%s must not reuse index %d already assigned to %s", item.flag, index, existingRole)
		}
		seen[index] = item.role
		series = append(series, chartRoleIndex{flag: item.flag, role: item.role, index: index})
	}
	return keyIndex, series, true, nil
}

var chartSemanticConfigFlags = []string{
	"title",
	"subtitle",
	"legend-position",
	"x-axis-title",
	"y-axis-title",
	"secondary-y-axis-title",
	"x-axis-label-angle",
	"y-axis-label-angle",
	"x-axis-min",
	"x-axis-max",
	"y-axis-min",
	"y-axis-max",
	"data-labels",
	"data-label-position",
	"stack",
	"color-palette",
}

// ChartCreateBasic creates a complete server-side chart snapshot from a chart
// type and a rectangular source range. The CLI only forwards semantic input;
// it deliberately does not own or duplicate the full chart snapshot template.
var ChartCreateBasic = common.Shortcut{
	Service:     "sheets",
	Command:     "+chart-create-basic",
	Description: "Create a basic chart from a chart type and data range; the server builds and validates the full snapshot.",
	Risk:        "write",
	Scopes:      []string{"sheets:spreadsheet:write_only"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags:       flagsFor("+chart-create-basic"),
	PostMount:   configureChartSemanticCommand,
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token, err := resolveSpreadsheetToken(runtime)
		if err != nil {
			return err
		}
		sheetID, sheetName, err := resolveSheetSelector(runtime)
		if err != nil {
			return err
		}
		_, err = chartCreateBasicInput(runtime, token, sheetID, sheetName)
		return err
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, _ := resolveSpreadsheetToken(runtime)
		sheetID, sheetName, _ := resolveSheetSelector(runtime)
		input, _ := chartCreateBasicInput(runtime, token, sheetID, sheetName)
		return invokeToolDryRun(token, ToolKindWrite, "manage_chart_object", input)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token, err := resolveSpreadsheetTokenExec(runtime)
		if err != nil {
			return err
		}
		sheetID, sheetName, err := resolveSheetSelector(runtime)
		if err != nil {
			return err
		}
		input, err := chartCreateBasicInput(runtime, token, sheetID, sheetName)
		if err != nil {
			return err
		}
		out, err := callTool(ctx, runtime, token, ToolKindWrite, "manage_chart_object", input)
		if err != nil {
			return err
		}
		runtime.Out(out, nil)
		return nil
	},
}

// ChartConfigUpdate updates the common chart settings that repeatedly caused
// full-snapshot retries in eval traces. Advanced per-series and marker styling
// remains on +chart-update --properties.
var ChartConfigUpdate = common.Shortcut{
	Service:     "sheets",
	Command:     "+chart-config-update",
	Description: "Update common chart titles, axes, legend, labels, stacking, smoothing, or chart-level colors without sending a full snapshot.",
	Risk:        "write",
	Scopes:      []string{"sheets:spreadsheet:read", "sheets:spreadsheet:write_only"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags:       flagsFor("+chart-config-update"),
	Tips: []string{
		"--dry-run validates the request shape only; execution reads the current chart snapshot, so X-axis bounds can still be rejected unless the existing bottom X axis is continuous (valueType=linear).",
	},
	PostMount: configureChartSemanticCommand,
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token, err := resolveSpreadsheetToken(runtime)
		if err != nil {
			return err
		}
		sheetID, sheetName, err := resolveSheetSelector(runtime)
		if err != nil {
			return err
		}
		_, err = chartConfigUpdateInput(runtime, token, sheetID, sheetName)
		return err
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, _ := resolveSpreadsheetToken(runtime)
		sheetID, sheetName, _ := resolveSheetSelector(runtime)
		input, _ := chartConfigUpdateInput(runtime, token, sheetID, sheetName)
		return invokeToolDryRun(token, ToolKindWrite, "manage_chart_object", input)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token, err := resolveSpreadsheetTokenExec(runtime)
		if err != nil {
			return err
		}
		sheetID, sheetName, err := resolveSheetSelector(runtime)
		if err != nil {
			return err
		}
		snapshot, err := fetchChartSnapshot(ctx, runtime, token, sheetID, sheetName, runtime.Str("chart-id"))
		if err != nil {
			return err
		}
		input, viewModel, err := chartConfigUpdateInputFromSnapshot(runtime, token, sheetID, sheetName, snapshot)
		if err != nil {
			return err
		}
		out, err := callTool(ctx, runtime, token, ToolKindWrite, "manage_chart_object", input)
		if err != nil {
			return err
		}
		if runtime.Changed("last-point-label") {
			updatedSnapshot, readErr := fetchChartSnapshot(
				ctx, runtime, token, sheetID, sheetName, runtime.Str("chart-id"),
			)
			if readErr != nil {
				return readErr
			}
			viewModel = chartViewModel(updatedSnapshot)
		}
		runtime.Out(withChartShortcutResult(out, "viewModel", viewModel), nil)
		return nil
	},
}

// ChartDataUpdate rebinds an existing chart to a new source range. The CLI
// reads the current snapshot, rebuilds its data mapping, and preserves the
// chart's layout and visual configuration.
var ChartDataUpdate = common.Shortcut{
	Service:     "sheets",
	Command:     "+chart-data-update",
	Description: "Update an existing chart's data range or direction while preserving its layout and visual configuration.",
	Risk:        "write",
	Scopes:      []string{"sheets:spreadsheet:read", "sheets:spreadsheet:write_only"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags:       flagsFor("+chart-data-update"),
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token, err := resolveSpreadsheetToken(runtime)
		if err != nil {
			return err
		}
		sheetID, sheetName, err := resolveSheetSelector(runtime)
		if err != nil {
			return err
		}
		_, err = chartDataUpdateInput(runtime, token, sheetID, sheetName)
		return err
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, _ := resolveSpreadsheetToken(runtime)
		sheetID, sheetName, _ := resolveSheetSelector(runtime)
		input, _ := chartDataUpdateInput(runtime, token, sheetID, sheetName)
		return invokeToolDryRun(token, ToolKindWrite, "manage_chart_object", input)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token, err := resolveSpreadsheetTokenExec(runtime)
		if err != nil {
			return err
		}
		sheetID, sheetName, err := resolveSheetSelector(runtime)
		if err != nil {
			return err
		}
		snapshot, err := fetchChartSnapshot(ctx, runtime, token, sheetID, sheetName, runtime.Str("chart-id"))
		if err != nil {
			return err
		}
		input, data, ranges, notice, err := chartDataUpdateInputFromSnapshot(runtime, token, sheetID, sheetName, snapshot)
		if err != nil {
			return err
		}
		out, err := callTool(ctx, runtime, token, ToolKindWrite, "manage_chart_object", input)
		if err != nil {
			return err
		}
		result := withChartShortcutResult(out, "data", data)
		result["normalized_data_ranges"] = ranges
		if notice != "" {
			result["normalization_notice"] = notice
		}
		runtime.Out(result, nil)
		return nil
	},
}

func chartCreateBasicInput(rt flagView, token, sheetID, sheetName string) (map[string]interface{}, error) {
	if err := requireSheetSelector(sheetID, sheetName); err != nil {
		return nil, err
	}
	chartType := strings.TrimSpace(rt.Str("chart-type"))
	if chartType == "" {
		return nil, sheetsValidationForFlag("chart-type", "--chart-type is required")
	}
	dataRange := strings.TrimSpace(rt.Str("data-range"))
	if dataRange == "" {
		return nil, sheetsValidationForFlag("data-range", "--data-range is required")
	}
	direction := rt.Str("data-direction")
	if direction == "" {
		direction = "column"
	}
	normalizedDataRange, dimensionCount, dataPointCount, _, err := normalizeBasicChartDataRanges(dataRange, direction)
	if err != nil {
		return nil, err
	}
	if dimensionCount < 2 || dataPointCount < 2 {
		if direction == "column" && dataPointCount < 2 && dimensionCount >= 2 {
			return nil, sheetsValidationForFlag(
				"data-range",
				"--data-range has one row and multiple columns; if categories run horizontally, include the category and value rows in --data-range and use --data-direction row",
			)
		}
		return nil, sheetsValidationForFlag("data-range", "--data-range must provide at least 2 data points and 2 dimensions")
	}

	bubbleKeyIndex, bubbleSeries, useBubbleRoles, err := resolveBubbleRoleIndexes(rt, chartType, dimensionCount)
	if err != nil {
		return nil, err
	}
	dim1Index := 1
	if useBubbleRoles {
		dim1Index = bubbleKeyIndex
	} else if rt.Changed("dim1-index") {
		dim1Index = rt.Int("dim1-index")
	}
	if dim1Index < 1 || dim1Index > dimensionCount {
		return nil, sheetsValidationForFlag(
			"dim1-index",
			"--dim1-index must be between 1 and %d for the selected --data-range",
			dimensionCount,
		)
	}
	dim2Indexes := make([]int, 0, dimensionCount-1)
	if useBubbleRoles {
		for _, item := range bubbleSeries {
			dim2Indexes = append(dim2Indexes, item.index)
		}
	} else {
		for index := 1; index <= dimensionCount; index++ {
			if index != dim1Index {
				dim2Indexes = append(dim2Indexes, index)
			}
		}
	}
	if chartType == "pie" || chartType == "pareto" {
		dim2Indexes = dim2Indexes[:1]
	}
	if rt.Changed("dim2-indexes") {
		dim2Indexes, err = parseChartDim2Indexes(rt.Str("dim2-indexes"))
		if err != nil {
			return nil, sheetsValidationForFlag("dim2-indexes", "%v", err)
		}
	}
	for _, index := range dim2Indexes {
		if index > dimensionCount {
			return nil, sheetsValidationForFlag(
				"dim2-indexes",
				"--dim2-indexes must contain only indexes between 1 and %d for the selected --data-range",
				dimensionCount,
			)
		}
		if index == dim1Index {
			return nil, sheetsValidationForFlag(
				"dim2-indexes",
				"--dim2-indexes must not contain the dim1 index %d",
				dim1Index,
			)
		}
	}
	if chartType == "pie" && len(dim2Indexes) != 1 {
		return nil, sheetsValidationForFlag("dim2-indexes", "pie charts require exactly one --dim2-indexes value")
	}
	if chartType == "combo" && len(dim2Indexes) < 2 {
		return nil, sheetsValidationForFlag("dim2-indexes", "combo charts require at least two --dim2-indexes values")
	}
	if chartType != "combo" && (rt.Changed("series-types") || rt.Changed("series-y-axes")) {
		return nil, sheetsValidationForFlag("series-types", "--series-types and --series-y-axes are only valid for combo charts")
	}
	var seriesTypes []string
	if rt.Changed("series-types") {
		seriesTypes, err = parseChartEnumList(rt.Str("series-types"), "series-types", []string{"column", "line", "area"})
		if err != nil {
			return nil, err
		}
		if len(seriesTypes) != len(dim2Indexes) {
			return nil, sheetsValidationForFlag("series-types", "--series-types must contain one value per selected value series")
		}
	}
	var seriesYAxes []string
	if rt.Changed("series-y-axes") {
		seriesYAxes, err = parseChartEnumList(rt.Str("series-y-axes"), "series-y-axes", []string{"left", "right"})
		if err != nil {
			return nil, err
		}
		if len(seriesYAxes) != len(dim2Indexes) {
			return nil, sheetsValidationForFlag("series-y-axes", "--series-y-axes must contain one value per selected value series")
		}
	}
	if chartType == "bubble" && (len(dim2Indexes) < 2 || len(dim2Indexes) > 4) {
		return nil, sheetsValidationForFlag("dim2-indexes", "bubble charts require x and y plus optional group and size indexes")
	}
	if chartType == "pareto" && len(dim2Indexes) != 1 {
		return nil, sheetsValidationForFlag("dim2-indexes", "pareto charts require exactly one --dim2-indexes value")
	}
	if len(dim2Indexes) > chartSeriesMaxCount {
		flagName := "data-range"
		if rt.Changed("dim2-indexes") {
			flagName = "dim2-indexes"
		}
		return nil, sheetsValidationForFlag(
			flagName,
			"the selected dimensions create %d series, over the current limit of %d; provide at most %d --dim2-indexes values or build a compact summary table",
			len(dim2Indexes),
			chartSeriesMaxCount,
			chartSeriesMaxCount,
		)
	}

	xAxisNumbersAs := strings.TrimSpace(rt.Str("x-axis-numbers-as"))
	if xAxisNumbersAs == "" {
		xAxisNumbersAs = "text"
	}
	if xAxisNumbersAs != "text" && xAxisNumbersAs != "values" {
		return nil, sheetsValidationForFlag(
			"x-axis-numbers-as",
			"--x-axis-numbers-as must be text or values",
		)
	}
	if err := validateChartAxisBounds(rt, "x", xAxisNumbersAs == "values"); err != nil {
		return nil, err
	}
	if err := validateChartAxisBounds(rt, "y", true); err != nil {
		return nil, err
	}
	basic := map[string]interface{}{
		"chart_type":        chartType,
		"data_range":        normalizedDataRange,
		"x_axis_numbers_as": xAxisNumbersAs,
	}
	if rt.Changed("header-range") {
		headerRange := strings.TrimSpace(rt.Str("header-range"))
		if err := validateChartRangeListFlag("header-range", headerRange); err != nil {
			return nil, err
		}
		if err := validateChartHeaderRangeDirection(headerRange, direction); err != nil {
			return nil, err
		}
		if _, err := buildChartHeaderRefs(headerRange, direction, dimensionCount); err != nil {
			return nil, err
		}
		basic["header_range"] = headerRange
	}
	if rt.Changed("data-direction") {
		basic["data_direction"] = rt.Str("data-direction")
	}
	if useBubbleRoles {
		basic["key_index"] = dim1Index
		for _, item := range bubbleSeries {
			basic[strings.ReplaceAll(item.flag, "-", "_")] = item.index
		}
	} else if rt.Changed("dim1-index") {
		basic["dim1_index"] = dim1Index
	}
	if !useBubbleRoles && rt.Changed("dim2-indexes") {
		basic["dim2_indexes"] = dim2Indexes
	}
	if rt.Changed("series-types") {
		basic["series_types"] = seriesTypes
	}
	if rt.Changed("series-y-axes") {
		basic["series_y_axes"] = seriesYAxes
	}
	if err := validateChartColorFlags(rt); err != nil {
		return nil, err
	}
	if err := validateChartSemanticEnums(rt); err != nil {
		return nil, err
	}
	addChartSemanticConfig(rt, basic)

	if rt.Changed("anchor-cell") {
		anchor := strings.TrimSpace(rt.Str("anchor-cell"))
		_, row, ok := splitCellRef(anchor)
		if !ok {
			return nil, sheetsValidationForFlag("anchor-cell", "--anchor-cell must be a single A1 cell such as F2")
		}
		colEnd := 0
		for colEnd < len(anchor) && ((anchor[colEnd] >= 'A' && anchor[colEnd] <= 'Z') || (anchor[colEnd] >= 'a' && anchor[colEnd] <= 'z')) {
			colEnd++
		}
		basic["position"] = map[string]interface{}{"row": row, "col": strings.ToUpper(anchor[:colEnd])}
	}
	widthChanged := rt.Changed("width")
	heightChanged := rt.Changed("height")
	if widthChanged != heightChanged {
		return nil, common.ValidationErrorf("--width and --height must be provided together").WithParams(
			sheetsInvalidParam("width", "must be paired with --height"),
			sheetsInvalidParam("height", "must be paired with --width"),
		)
	}
	if widthChanged {
		if rt.Int("width") < 10 || rt.Int("height") < 10 {
			return nil, common.ValidationErrorf("--width and --height must be at least 10")
		}
		basic["size"] = map[string]interface{}{"width": rt.Int("width"), "height": rt.Int("height")}
	}

	input := map[string]interface{}{"excel_id": token, "operation": "create", "basic_chart": basic}
	sheetSelectorForToolInput(input, sheetID, sheetName)
	if err := validateInputAgainstSchema(rt, input); err != nil {
		return nil, err
	}
	return input, nil
}

func chartConfigUpdateInput(rt flagView, token, sheetID, sheetName string) (map[string]interface{}, error) {
	if err := requireSheetSelector(sheetID, sheetName); err != nil {
		return nil, err
	}
	chartID := strings.TrimSpace(rt.Str("chart-id"))
	if chartID == "" {
		return nil, sheetsValidationForFlag("chart-id", "--chart-id is required")
	}
	updates := map[string]interface{}{}
	if err := validateChartColorFlags(rt); err != nil {
		return nil, err
	}
	if err := validateChartSemanticEnums(rt); err != nil {
		return nil, err
	}
	if err := validateChartAxisBounds(rt, "x", true); err != nil {
		return nil, err
	}
	if err := validateChartAxisBounds(rt, "y", true); err != nil {
		return nil, err
	}
	addChartSemanticConfig(rt, updates)
	if len(updates) == 0 && !rt.Changed("last-point-label") {
		return nil, common.ValidationErrorf("at least one chart configuration flag is required")
	}
	patch, _ := applyChartConfigPatch(map[string]interface{}{}, updates)
	input := map[string]interface{}{
		"excel_id":  token,
		"operation": "update",
		"chart_id":  chartID,
		"properties": map[string]interface{}{
			"snapshot": patch,
		},
	}
	if rt.Changed("last-point-label") {
		input["properties"].(map[string]interface{})["last_point_label"] = rt.Bool("last-point-label")
	}
	sheetSelectorForToolInput(input, sheetID, sheetName)
	if err := validateInputAgainstSchema(rt, input); err != nil {
		return nil, err
	}
	return input, nil
}

func chartDataUpdateInput(rt flagView, token, sheetID, sheetName string) (map[string]interface{}, error) {
	if err := requireSheetSelector(sheetID, sheetName); err != nil {
		return nil, err
	}
	chartID := strings.TrimSpace(rt.Str("chart-id"))
	if chartID == "" {
		return nil, sheetsValidationForFlag("chart-id", "--chart-id is required")
	}
	dataRange := strings.TrimSpace(rt.Str("data-range"))
	if dataRange == "" {
		return nil, sheetsValidationForFlag("data-range", "--data-range is required")
	}
	ranges, err := splitChartDataRanges(dataRange)
	if err != nil {
		return nil, sheetsValidationForFlag("data-range", "invalid --data-range %q: %v", dataRange, err)
	}
	for _, value := range ranges {
		_, parseErr := parseChartDataRange(value)
		if parseErr != nil {
			return nil, sheetsValidationForFlag("data-range", "invalid --data-range item %q: %v", value, parseErr)
		}
	}

	updates := map[string]interface{}{"data_range": dataRange}
	if rt.Changed("header-range") {
		headerRange := strings.TrimSpace(rt.Str("header-range"))
		if err := validateChartRangeListFlag("header-range", headerRange); err != nil {
			return nil, err
		}
		updates["header_range"] = headerRange
	}
	if rt.Changed("data-direction") {
		updates["data_direction"] = rt.Str("data-direction")
	}
	dim1Index := 1
	if rt.Changed("dim1-index") {
		dim1Index = rt.Int("dim1-index")
		if dim1Index < 1 {
			return nil, sheetsValidationForFlag("dim1-index", "--dim1-index must be a positive 1-based index")
		}
		updates["dim1_index"] = dim1Index
	}
	if rt.Changed("dim2-indexes") {
		dim2Indexes, parseErr := parseChartDim2Indexes(rt.Str("dim2-indexes"))
		if parseErr != nil {
			return nil, sheetsValidationForFlag("dim2-indexes", "%v", parseErr)
		}
		for _, index := range dim2Indexes {
			if index == dim1Index {
				return nil, sheetsValidationForFlag(
					"dim2-indexes",
					"--dim2-indexes must not contain the dim1 index %d",
					dim1Index,
				)
			}
		}
		if len(dim2Indexes) > chartSeriesMaxCount {
			return nil, sheetsValidationForFlag(
				"dim2-indexes",
				"--dim2-indexes selects %d series, over the current limit of %d; select fewer series",
				len(dim2Indexes),
				chartSeriesMaxCount,
			)
		}
		updates["dim2_indexes"] = dim2Indexes
	}
	bubbleKeyIndex, bubbleSeries, useBubbleRoles, err := resolveBubbleRoleIndexes(rt, "", 0)
	if err != nil {
		return nil, err
	}
	if useBubbleRoles {
		updates["key_index"] = bubbleKeyIndex
		for _, item := range bubbleSeries {
			updates[strings.ReplaceAll(item.flag, "-", "_")] = item.index
		}
	}
	dryRunData, err := chartDataDryRunPatch(updates)
	if err != nil {
		return nil, err
	}
	input := map[string]interface{}{
		"excel_id":  token,
		"operation": "update",
		"chart_id":  chartID,
		"properties": map[string]interface{}{
			"snapshot": map[string]interface{}{
				"data": dryRunData,
			},
		},
	}
	sheetSelectorForToolInput(input, sheetID, sheetName)
	if err := validateInputAgainstSchema(rt, input); err != nil {
		return nil, err
	}
	return input, nil
}

func chartConfigUpdateInputFromSnapshot(
	rt flagView,
	token, sheetID, sheetName string,
	snapshot map[string]interface{},
) (map[string]interface{}, map[string]interface{}, error) {
	if _, err := chartConfigUpdateInput(rt, token, sheetID, sheetName); err != nil {
		return nil, nil, err
	}
	if err := validateChartConfigSnapshot(rt, snapshot); err != nil {
		return nil, nil, err
	}
	updates := map[string]interface{}{}
	addChartSemanticConfig(rt, updates)
	patch, viewModel := applyChartConfigPatch(snapshot, updates)
	input := map[string]interface{}{
		"excel_id":  token,
		"operation": "update",
		"chart_id":  strings.TrimSpace(rt.Str("chart-id")),
		"properties": map[string]interface{}{
			"snapshot": patch,
		},
	}
	if rt.Changed("last-point-label") {
		input["properties"].(map[string]interface{})["last_point_label"] = rt.Bool("last-point-label")
	}
	sheetSelectorForToolInput(input, sheetID, sheetName)
	if err := validateInputAgainstSchema(rt, input); err != nil {
		return nil, nil, err
	}
	return input, viewModel, nil
}

func chartDataUpdateInputFromSnapshot(
	rt flagView,
	token, sheetID, sheetName string,
	snapshot map[string]interface{},
) (map[string]interface{}, map[string]interface{}, []string, string, error) {
	if _, err := chartDataUpdateInput(rt, token, sheetID, sheetName); err != nil {
		return nil, nil, nil, "", err
	}
	currentData, _ := snapshot["data"].(map[string]interface{})
	if static, _ := currentData["isStaticData"].(bool); static {
		return nil, nil, nil, "", sheetsValidationForFlag("chart-id", "+chart-data-update does not support static-data charts")
	}
	chartType := chartTypeFromSnapshot(snapshot)
	if !isBasicChartType(chartType) {
		return nil, nil, nil, "", sheetsValidationForFlag(
			"chart-id",
			"+chart-data-update does not support chart type %q",
			chartType,
		)
	}

	direction := "column"
	if value, ok := currentData["direction"].(string); ok && value != "" {
		direction = value
	}
	if rt.Changed("data-direction") {
		direction = rt.Str("data-direction")
	}
	dataRange := strings.TrimSpace(rt.Str("data-range"))
	normalized, dimensionCount, _, merged, err := normalizeBasicChartDataRanges(dataRange, direction)
	if err != nil {
		return nil, nil, nil, "", err
	}

	bubbleKeyIndex, bubbleSeries, useBubbleRoles, err := resolveBubbleRoleIndexes(rt, chartType, dimensionCount)
	if err != nil {
		return nil, nil, nil, "", err
	}
	dim1Index := 1
	if useBubbleRoles {
		dim1Index = bubbleKeyIndex
	} else if rt.Changed("dim1-index") {
		dim1Index = rt.Int("dim1-index")
	}
	if dim1Index < 1 || dim1Index > dimensionCount {
		return nil, nil, nil, "", sheetsValidationForFlag(
			"dim1-index",
			"--dim1-index must be between 1 and %d for the selected --data-range",
			dimensionCount,
		)
	}
	dim2Indexes := make([]int, 0, dimensionCount-1)
	if useBubbleRoles {
		for _, item := range bubbleSeries {
			dim2Indexes = append(dim2Indexes, item.index)
		}
	} else {
		for index := 1; index <= dimensionCount; index++ {
			if index != dim1Index {
				dim2Indexes = append(dim2Indexes, index)
			}
		}
	}
	if (chartType == "pie" || chartType == "pareto") && len(dim2Indexes) > 1 {
		dim2Indexes = dim2Indexes[:1]
	}
	if rt.Changed("dim2-indexes") {
		dim2Indexes, err = parseChartDim2Indexes(rt.Str("dim2-indexes"))
		if err != nil {
			return nil, nil, nil, "", sheetsValidationForFlag("dim2-indexes", "%v", err)
		}
	}
	for _, index := range dim2Indexes {
		if index > dimensionCount {
			return nil, nil, nil, "", sheetsValidationForFlag(
				"dim2-indexes",
				"--dim2-indexes must contain only indexes between 1 and %d for the selected --data-range",
				dimensionCount,
			)
		}
		if index == dim1Index {
			return nil, nil, nil, "", sheetsValidationForFlag(
				"dim2-indexes",
				"--dim2-indexes must not contain the dim1 index %d",
				dim1Index,
			)
		}
	}
	if chartType == "pie" && len(dim2Indexes) != 1 {
		return nil, nil, nil, "", sheetsValidationForFlag("dim2-indexes", "pie charts require exactly one --dim2-indexes value")
	}
	if chartType == "combo" && len(dim2Indexes) < 2 {
		return nil, nil, nil, "", sheetsValidationForFlag("dim2-indexes", "combo charts require at least two --dim2-indexes values")
	}
	if chartType == "bubble" && (len(dim2Indexes) < 2 || len(dim2Indexes) > 4) {
		return nil, nil, nil, "", sheetsValidationForFlag("dim2-indexes", "bubble charts require x and y plus optional group and size indexes")
	}
	if chartType == "pareto" && len(dim2Indexes) != 1 {
		return nil, nil, nil, "", sheetsValidationForFlag("dim2-indexes", "pareto charts require exactly one --dim2-indexes value")
	}
	if len(dim2Indexes) > chartSeriesMaxCount {
		return nil, nil, nil, "", sheetsValidationForFlag(
			"dim2-indexes",
			"--dim2-indexes selects %d series, over the current limit of %d; select fewer series",
			len(dim2Indexes),
			chartSeriesMaxCount,
		)
	}

	headerRefs := existingChartHeaderRefs(currentData)
	detached := currentData["headerMode"] == "detached"
	if rt.Changed("header-range") {
		headerRefs, err = buildChartHeaderRefs(strings.TrimSpace(rt.Str("header-range")), direction, dimensionCount)
		if err != nil {
			return nil, nil, nil, "", err
		}
		detached = true
	}
	if detached {
		for _, index := range append([]int{dim1Index}, dim2Indexes...) {
			if headerRefs[index] == "" {
				return nil, nil, nil, "", sheetsValidationForFlag(
					"header-range",
					"--header-range is required because the detached header for dimension %d is missing",
					index,
				)
			}
		}
	}

	rangeValues, _ := splitChartDataRanges(normalized)
	refs := make([]interface{}, 0, len(rangeValues))
	for _, value := range rangeValues {
		refs = append(refs, map[string]interface{}{"value": value})
	}
	dim1 := map[string]interface{}{"index": dim1Index}
	if detached {
		dim1["nameRef"] = headerRefs[dim1Index]
	}
	series := make([]interface{}, 0, len(dim2Indexes))
	for seriesIndex, index := range dim2Indexes {
		item := map[string]interface{}{"index": index}
		if chartType == "bubble" {
			role := []string{"x", "y", "group", "size"}[seriesIndex]
			if useBubbleRoles {
				role = bubbleSeries[seriesIndex].role
			}
			item["role"] = role
		}
		if detached {
			item["nameRef"] = headerRefs[index]
		}
		series = append(series, item)
	}
	data := map[string]interface{}{
		"isStaticData": false,
		"direction":    direction,
		"refs":         refs,
		"dim1":         map[string]interface{}{"serie": dim1},
		"dim2":         map[string]interface{}{"series": series},
	}
	if detached {
		data["headerMode"] = "detached"
	}
	patch := map[string]interface{}{"data": data}
	if chartType == "combo" {
		patch["plotArea"] = comboPlotAreaForIndexes(snapshot, dim2Indexes)
	}
	input := map[string]interface{}{
		"excel_id":  token,
		"operation": "update",
		"chart_id":  strings.TrimSpace(rt.Str("chart-id")),
		"properties": map[string]interface{}{
			"snapshot": patch,
		},
	}
	sheetSelectorForToolInput(input, sheetID, sheetName)
	if err := validateInputAgainstSchema(rt, input); err != nil {
		return nil, nil, nil, "", err
	}
	notice := ""
	if merged {
		notice = "Multiple data ranges were not aligned and were merged into the smallest enclosing rectangular range; review normalized_data_ranges before continuing."
	}
	return input, data, rangeValues, notice, nil
}

func chartDataDryRunPatch(updates map[string]interface{}) (map[string]interface{}, error) {
	data := map[string]interface{}{}
	dataRange, _ := updates["data_range"].(string)
	direction, _ := updates["data_direction"].(string)
	if direction == "" {
		direction = "column"
	}
	normalized, dimensionCount, _, _, err := normalizeBasicChartDataRanges(dataRange, direction)
	if err != nil {
		return nil, err
	}
	if dataRange != "" {
		ranges, err := splitChartDataRanges(normalized)
		if err != nil {
			return nil, sheetsValidationForFlag("data-range", "invalid --data-range %q: %v", normalized, err)
		}
		refs := make([]interface{}, 0, len(ranges))
		for _, item := range ranges {
			refs = append(refs, map[string]interface{}{"value": item})
		}
		data["refs"] = refs
	}
	if value, ok := updates["data_direction"]; ok {
		data["direction"] = value
	}
	dim1Index := 1
	if value, ok := chartInt(updates["key_index"]); ok {
		dim1Index = value
	} else if value, ok := chartInt(updates["dim1_index"]); ok {
		dim1Index = value
	}
	dim2Indexes := make([]int, 0, max(0, dimensionCount-1))
	for index := 1; index <= dimensionCount; index++ {
		if index != dim1Index {
			dim2Indexes = append(dim2Indexes, index)
		}
	}
	if values, ok := updates["dim2_indexes"].([]int); ok {
		dim2Indexes = values
	}
	headerRefs := map[int]string{}
	if headerRange, ok := updates["header_range"].(string); ok {
		headerRefs, err = buildChartHeaderRefs(headerRange, direction, dimensionCount)
		if err != nil {
			return nil, err
		}
		data["headerMode"] = "detached"
	}
	dim1 := map[string]interface{}{"index": dim1Index}
	if headerRefs[dim1Index] != "" {
		dim1["nameRef"] = headerRefs[dim1Index]
	}
	data["dim1"] = map[string]interface{}{"serie": dim1}
	semanticRoles := make([]chartRoleIndex, 0, len(bubbleRoleIndexFlags))
	for _, item := range bubbleRoleIndexFlags {
		if value, ok := chartInt(updates[strings.ReplaceAll(item.flag, "-", "_")]); ok {
			semanticRoles = append(semanticRoles, chartRoleIndex{flag: item.flag, role: item.role, index: value})
		}
	}
	if len(semanticRoles) > 0 {
		dim2Indexes = dim2Indexes[:0]
		for _, item := range semanticRoles {
			dim2Indexes = append(dim2Indexes, item.index)
		}
	}
	series := make([]interface{}, 0, len(dim2Indexes))
	for seriesIndex, value := range dim2Indexes {
		item := map[string]interface{}{"index": value}
		if len(semanticRoles) > 0 {
			item["role"] = semanticRoles[seriesIndex].role
		}
		if headerRefs[value] != "" {
			item["nameRef"] = headerRefs[value]
		}
		series = append(series, item)
	}
	data["dim2"] = map[string]interface{}{"series": series}
	return data, nil
}

func fetchChartSnapshot(
	ctx context.Context,
	runtime *common.RuntimeContext,
	token, sheetID, sheetName, chartID string,
) (map[string]interface{}, error) {
	input := map[string]interface{}{
		"excel_id": token,
		"chart_id": strings.TrimSpace(chartID),
	}
	sheetSelectorForToolInput(input, sheetID, sheetName)
	out, err := callTool(ctx, runtime, token, ToolKindRead, "get_chart_objects", input)
	if err != nil {
		return nil, err
	}
	root, _ := out.(map[string]interface{})
	if data, ok := root["data"].(map[string]interface{}); ok {
		root = data
	}
	sheets, _ := root["sheets"].([]interface{})
	for _, rawSheet := range sheets {
		sheet, _ := rawSheet.(map[string]interface{})
		charts, _ := sheet["charts"].([]interface{})
		for _, rawChart := range charts {
			chart, _ := rawChart.(map[string]interface{})
			if id, _ := chart["chart_id"].(string); id != strings.TrimSpace(chartID) {
				continue
			}
			details, _ := chart["details"].(map[string]interface{})
			snapshot, _ := details["snapshot"].(map[string]interface{})
			if snapshot == nil {
				return nil, sheetsValidationForFlag("chart-id", "chart %q has no editable snapshot", chartID)
			}
			return snapshot, nil
		}
	}
	return nil, sheetsValidationForFlag("chart-id", "chart %q was not found on the selected sheet", chartID)
}

func withChartShortcutResult(out interface{}, key string, value interface{}) map[string]interface{} {
	result := map[string]interface{}{}
	if object, ok := out.(map[string]interface{}); ok {
		for name, item := range object {
			result[name] = item
		}
	} else if out != nil {
		result["tool_output"] = out
	}
	result[key] = value
	return result
}

func applyChartConfigPatch(
	current map[string]interface{},
	updates map[string]interface{},
) (map[string]interface{}, map[string]interface{}) {
	next := cloneChartMap(current)
	patch := map[string]interface{}{}
	plotChanged := false

	if value, ok := updates["title"].(string); ok {
		title := chartMap(next["title"])
		title["text"] = value
		next["title"] = title
		patch["title"] = title
	}
	if value, ok := updates["subtitle"].(string); ok {
		subtitle := chartMap(next["subTitle"])
		subtitle["text"] = value
		next["subTitle"] = subtitle
		patch["subTitle"] = subtitle
	}
	if value, ok := updates["legend_position"].(string); ok {
		if value == "hidden" {
			next["legend"] = false
			patch["legend"] = false
		} else {
			legend := chartMap(next["legend"])
			legend["position"] = value
			next["legend"] = legend
			patch["legend"] = legend
		}
	}

	plotArea := chartMap(next["plotArea"])
	plot := chartMap(plotArea["plot"])
	plotArea["plot"] = plot
	next["plotArea"] = plotArea
	for _, item := range []struct {
		key      string
		axisType string
		position string
		title    bool
	}{
		{"x_axis_title", "x", "bottom", true},
		{"y_axis_title", "y", "left", true},
		{"secondary_y_axis_title", "y", "right", true},
		{"x_axis_label_angle", "x", "bottom", false},
		{"y_axis_label_angle", "y", "left", false},
	} {
		value, ok := updates[item.key]
		if !ok {
			continue
		}
		axis := ensureChartAxisMap(plotArea, item.axisType, item.position)
		if item.title {
			axis["title"] = map[string]interface{}{"text": value}
		} else {
			label := chartMap(axis["label"])
			label["angle"] = value
			axis["label"] = label
		}
		plotChanged = true
	}
	for _, item := range []struct {
		key      string
		axisType string
		position string
		axisProp string
	}{
		{"x_axis_min", "x", "bottom", "min"},
		{"x_axis_max", "x", "bottom", "max"},
		{"y_axis_min", "y", "left", "min"},
		{"y_axis_max", "y", "left", "max"},
	} {
		value, ok := updates[item.key]
		if !ok {
			continue
		}
		axis := ensureChartAxisMap(plotArea, item.axisType, item.position)
		axis[item.axisProp] = value
		plotChanged = true
	}

	if value, ok := updates["data_labels"].(string); ok {
		if value == "none" {
			delete(plot, "labels")
		} else {
			labels := map[string]interface{}{
				"series":     value == "series",
				"category":   strings.Contains(value, "category"),
				"value":      strings.Contains(value, "value"),
				"percentage": strings.Contains(value, "percentage"),
			}
			if position, ok := updates["data_label_position"]; ok {
				labels["position"] = position
			}
			plot["labels"] = labels
		}
		plotChanged = true
	} else if position, ok := updates["data_label_position"]; ok {
		labels, exists := plot["labels"].(map[string]interface{})
		if exists {
			labels["position"] = position
			plot["labels"] = labels
			plotChanged = true
		}
	}
	if value, ok := updates["stack"].(string); ok {
		extra := chartMap(plot["extra"])
		if value == "none" {
			chartType, _ := plot["type"].(string)
			if chartType == "" || chartType == "waterfall" {
				extra["stack"] = map[string]interface{}{"enabled": false}
			} else {
				delete(extra, "stack")
			}
		} else {
			extra["stack"] = map[string]interface{}{"percentage": value == "percent"}
		}
		plot["extra"] = extra
		plotChanged = true
	}
	if value, ok := updates["smooth"].(bool); ok {
		extra := chartMap(plot["extra"])
		extra["smooth"] = value
		plot["extra"] = extra
		plotChanged = true
	}
	var colorTheme []interface{}
	if value, ok := updates["color_palette"].(string); ok {
		colorTheme = []interface{}{value}
	}
	if values, ok := updates["colors"].([]string); ok {
		colorTheme = make([]interface{}, 0, len(values))
		for _, value := range values {
			colorTheme = append(colorTheme, value)
		}
	}
	if colorTheme != nil {
		style := chartMap(next["style"])
		style["colorTheme"] = colorTheme
		next["style"] = style
		patch["style"] = map[string]interface{}{"colorTheme": colorTheme}
	}
	if plotChanged {
		patch["plotArea"] = plotArea
	}
	return patch, chartViewModel(next)
}

func chartViewModel(snapshot map[string]interface{}) map[string]interface{} {
	viewModel := cloneChartMap(snapshot)
	delete(viewModel, "data")
	return viewModel
}

func cloneChartMap(value map[string]interface{}) map[string]interface{} {
	if value == nil {
		return map[string]interface{}{}
	}
	raw, _ := json.Marshal(value)
	out := map[string]interface{}{}
	_ = json.Unmarshal(raw, &out)
	return out
}

func chartMap(value interface{}) map[string]interface{} {
	if object, ok := value.(map[string]interface{}); ok {
		return object
	}
	return map[string]interface{}{}
}

func ensureChartAxisMap(plotArea map[string]interface{}, axisType, position string) map[string]interface{} {
	if axis := findChartAxisMap(plotArea, axisType, position); axis != nil {
		return axis
	}
	axes, _ := plotArea["axes"].([]interface{})
	axis := map[string]interface{}{"type": axisType, "position": position}
	plotArea["axes"] = append(axes, axis)
	return axis
}

func findChartAxisMap(plotArea map[string]interface{}, axisType, position string) map[string]interface{} {
	axes, _ := plotArea["axes"].([]interface{})
	for _, raw := range axes {
		axis, _ := raw.(map[string]interface{})
		if axis["type"] == axisType && axis["position"] == position {
			return axis
		}
	}
	return nil
}

func chartTypeFromSnapshot(snapshot map[string]interface{}) string {
	plotArea := chartMap(snapshot["plotArea"])
	plot := chartMap(plotArea["plot"])
	value, _ := plot["type"].(string)
	return value
}

func isBasicChartType(value string) bool {
	switch value {
	case "column", "bar", "line", "area", "pie", "scatter", "combo", "radar", "bubble", "waterfall", "pareto":
		return true
	default:
		return false
	}
}

func existingChartHeaderRefs(data map[string]interface{}) map[int]string {
	out := map[int]string{}
	if dim1 := chartMap(chartMap(data["dim1"])["serie"]); dim1 != nil {
		index, _ := chartInt(dim1["index"])
		name, _ := dim1["nameRef"].(string)
		if index > 0 && name != "" {
			out[index] = name
		}
	}
	series, _ := chartMap(data["dim2"])["series"].([]interface{})
	for _, raw := range series {
		item := chartMap(raw)
		index, _ := chartInt(item["index"])
		name, _ := item["nameRef"].(string)
		if index > 0 && name != "" {
			out[index] = name
		}
	}
	return out
}

func buildChartHeaderRefs(value, direction string, dimensionCount int) (map[int]string, error) {
	ranges, err := splitChartDataRanges(value)
	if err != nil {
		return nil, sheetsValidationForFlag("header-range", "invalid --header-range %q: %v", value, err)
	}
	var refs []string
	for _, rangeValue := range ranges {
		item, parseErr := parseChartHeaderRange(rangeValue)
		if parseErr != nil {
			return nil, sheetsValidationForFlag("header-range", "invalid --header-range item %q: %v", rangeValue, parseErr)
		}
		prefix := ""
		if item.sheet != "" {
			prefix = item.sheet + "!"
		}
		if direction == "row" {
			if item.colCount != 1 {
				return nil, sheetsValidationForFlag("header-range", "row-oriented --header-range must be one column")
			}
			for row := item.row; row < item.row+item.rowCount; row++ {
				refs = append(refs, prefix+columnIndexToLetter(item.col)+strconv.Itoa(row+1))
			}
		} else {
			if item.rowCount != 1 {
				return nil, sheetsValidationForFlag("header-range", "column-oriented --header-range must be one row")
			}
			for col := item.col; col < item.col+item.colCount; col++ {
				refs = append(refs, prefix+columnIndexToLetter(col)+strconv.Itoa(item.row+1))
			}
		}
	}
	if len(refs) != dimensionCount {
		return nil, sheetsValidationForFlag(
			"header-range",
			"--header-range provides %d headers but --data-range has %d dimensions",
			len(refs),
			dimensionCount,
		)
	}
	out := make(map[int]string, len(refs))
	for index, value := range refs {
		out[index+1] = value
	}
	return out, nil
}

func comboPlotAreaForIndexes(snapshot map[string]interface{}, indexes []int) map[string]interface{} {
	plotArea := chartMap(cloneChartMap(snapshot)["plotArea"])
	plot := chartMap(plotArea["plot"])
	currentSeries, _ := plot["series"].([]interface{})
	byIndex := map[int]map[string]interface{}{}
	for _, raw := range currentSeries {
		item := chartMap(raw)
		if index, ok := chartInt(item["index"]); ok {
			byIndex[index] = item
		}
	}
	nextSeries := make([]interface{}, 0, len(indexes))
	for position, index := range indexes {
		item := cloneChartMap(byIndex[index])
		if len(item) == 0 {
			item["comboType"] = "line"
			item["yAxisPosition"] = "right"
			if position == 0 {
				item["comboType"] = "column"
				item["yAxisPosition"] = "left"
			}
		}
		item["index"] = index
		nextSeries = append(nextSeries, item)
	}
	plot["series"] = nextSeries
	plotArea["plot"] = plot
	return plotArea
}

func chartInt(value interface{}) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, true
	case int64:
		return int(typed), true
	case float64:
		return int(typed), typed == float64(int(typed))
	default:
		return 0, false
	}
}

func validateChartRangeListFlag(flagName, value string) error {
	if value == "" {
		return sheetsValidationForFlag(flagName, "--%s must not be empty", flagName)
	}
	ranges, err := splitChartDataRanges(value)
	if err != nil {
		return sheetsValidationForFlag(flagName, "invalid --%s %q: %v", flagName, value, err)
	}
	for _, rangeValue := range ranges {
		_, parseErr := parseChartHeaderRange(rangeValue)
		if parseErr != nil {
			return sheetsValidationForFlag(flagName, "invalid --%s item %q: %v", flagName, rangeValue, parseErr)
		}
	}
	return nil
}

func validateChartHeaderRangeDirection(value, direction string) error {
	ranges, err := splitChartDataRanges(value)
	if err != nil {
		return sheetsValidationForFlag("header-range", "invalid --header-range %q: %v", value, err)
	}
	for _, rangeValue := range ranges {
		item, parseErr := parseChartHeaderRange(rangeValue)
		if parseErr != nil {
			return sheetsValidationForFlag("header-range", "invalid --header-range item %q: %v", rangeValue, parseErr)
		}
		if direction == "row" && item.colCount != 1 {
			return sheetsValidationForFlag(
				"header-range",
				"row-oriented --header-range must be one column of dimension/series names; this horizontal range looks like a category row, so include it in --data-range and omit --header-range",
			)
		}
		if direction == "column" && item.rowCount != 1 {
			return sheetsValidationForFlag(
				"header-range",
				"column-oriented --header-range must be one row of dimension/series names",
			)
		}
	}
	return nil
}

func parseChartHeaderRange(value string) (chartDataRange, error) {
	item := chartDataRange{}
	ref := strings.TrimSpace(value)
	if bang := strings.LastIndex(ref, "!"); bang >= 0 {
		item.sheet = strings.TrimSpace(ref[:bang])
		ref = strings.TrimSpace(ref[bang+1:])
	}
	if strings.Contains(ref, ":") {
		return parseChartDataRange(value)
	}
	col, row, ok := splitCellRef(ref)
	if !ok {
		return item, common.ValidationErrorf("expected an A1 cell or rectangular range such as A1 or A1:C1")
	}
	item.row, item.col = row, col
	item.rowCount, item.colCount = 1, 1
	return item, nil
}

func parseChartDim2Indexes(raw string) ([]int, error) {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "[") {
		var indexes []int
		if err := json.Unmarshal([]byte(raw), &indexes); err != nil {
			return nil, common.ValidationErrorf("--dim2-indexes must be a comma-separated list or an array of positive 1-based indexes")
		}
		return validateChartDim2Indexes(indexes)
	}
	parts := strings.Split(raw, ",")
	indexes := make([]int, 0, len(parts))
	for _, part := range parts {
		value := strings.TrimSpace(part)
		if value == "" {
			return nil, common.ValidationErrorf("--dim2-indexes must be a comma-separated list of positive 1-based indexes")
		}
		index, err := strconv.Atoi(value)
		if err != nil || index < 1 {
			return nil, common.ValidationErrorf("--dim2-indexes must contain only positive 1-based indexes")
		}
		indexes = append(indexes, index)
	}
	return validateChartDim2Indexes(indexes)
}

func parseChartEnumList(raw, flagName string, allowed []string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	values := []string{}
	if strings.HasPrefix(raw, "[") {
		if err := json.Unmarshal([]byte(raw), &values); err != nil {
			return nil, sheetsValidationForFlag(flagName, "--%s must be a comma-separated list or a JSON string array", flagName)
		}
	} else {
		values = strings.Split(raw, ",")
	}
	if len(values) == 0 {
		return nil, sheetsValidationForFlag(flagName, "--%s must not be empty", flagName)
	}
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, value := range allowed {
		allowedSet[value] = struct{}{}
	}
	for index := range values {
		values[index] = strings.TrimSpace(values[index])
		if _, ok := allowedSet[values[index]]; !ok {
			return nil, sheetsValidationForFlag(
				flagName,
				"--%s value %q is invalid; expected one of %s",
				flagName,
				values[index],
				strings.Join(allowed, ", "),
			)
		}
	}
	return values, nil
}

func validateChartDim2Indexes(indexes []int) ([]int, error) {
	if len(indexes) == 0 {
		return nil, common.ValidationErrorf("--dim2-indexes must contain at least one positive 1-based index")
	}
	seen := make(map[int]struct{}, len(indexes))
	for _, index := range indexes {
		if index < 1 {
			return nil, common.ValidationErrorf("--dim2-indexes must contain only positive 1-based indexes")
		}
		if _, exists := seen[index]; exists {
			return nil, common.ValidationErrorf("--dim2-indexes must not contain duplicate index %d", index)
		}
		seen[index] = struct{}{}
	}
	return indexes, nil
}

type chartDataRange struct {
	sheet              string
	qualifier          string
	row, col           int
	rowCount, colCount int
}

func normalizeBasicChartDataRanges(dataRange, direction string) (normalized string, dimensionCount, dataPointCount int, merged bool, err error) {
	ranges, err := splitChartDataRanges(dataRange)
	if err != nil {
		return "", 0, 0, false, sheetsValidationForFlag("data-range", "invalid --data-range %q: %v", dataRange, err)
	}
	parsed := make([]chartDataRange, 0, len(ranges))
	for _, value := range ranges {
		item, parseErr := parseChartDataRange(value)
		if parseErr != nil {
			return "", 0, 0, false, sheetsValidationForFlag("data-range", "invalid --data-range item %q: %v", value, parseErr)
		}
		parsed = append(parsed, item)
	}
	first := parsed[0]
	spansBySheet := make(map[string][][2]int)
	aligned := true
	minRow, minCol := first.row, first.col
	maxRow, maxCol := first.row+first.rowCount, first.col+first.colCount
	for _, item := range parsed {
		if direction == "row" {
			if item.col != first.col || item.colCount != first.colCount {
				aligned = false
			}
			dimensionCount += item.rowCount
			spansBySheet[item.sheet] = append(spansBySheet[item.sheet], [2]int{item.row, item.row + item.rowCount})
		} else {
			if item.row != first.row || item.rowCount != first.rowCount {
				aligned = false
			}
			dimensionCount += item.colCount
			spansBySheet[item.sheet] = append(spansBySheet[item.sheet], [2]int{item.col, item.col + item.colCount})
		}
		minRow = min(minRow, item.row)
		minCol = min(minCol, item.col)
		maxRow = max(maxRow, item.row+item.rowCount)
		maxCol = max(maxCol, item.col+item.colCount)
	}
	overlapping := false
	for _, spans := range spansBySheet {
		for i, current := range spans {
			for j := 0; j < i; j++ {
				if current[0] < spans[j][1] && spans[j][0] < current[1] {
					overlapping = true
				}
			}
		}
	}
	normalized = strings.Join(ranges, ",")
	if len(ranges) > 1 && (!aligned || overlapping) {
		if len(spansBySheet) > 1 {
			return "", 0, 0, false, sheetsValidationForFlag(
				"data-range",
				"cross-sheet --data-range items must align along the data-point axis and must not overlap within the same sheet",
			)
		}
		prefix := ""
		if first.qualifier != "" {
			prefix = first.qualifier
		}
		normalized = prefix + columnIndexToLetter(minCol) + strconv.Itoa(minRow+1) + ":" + columnIndexToLetter(maxCol-1) + strconv.Itoa(maxRow)
		dimensionCount = maxCol - minCol
		dataPointCount = maxRow - minRow
		if direction == "row" {
			dimensionCount, dataPointCount = dataPointCount, dimensionCount
		}
		return normalized, dimensionCount, dataPointCount, true, nil
	}
	if direction == "row" {
		dataPointCount = first.colCount
	} else {
		dataPointCount = first.rowCount
	}
	return normalized, dimensionCount, dataPointCount, false, nil
}

func splitChartDataRanges(value string) ([]string, error) {
	var ranges []string
	start := 0
	inQuote := false
	for i := 0; i <= len(value); i++ {
		if i < len(value) && value[i] == '\'' {
			if inQuote && i+1 < len(value) && value[i+1] == '\'' {
				i++
			} else {
				inQuote = !inQuote
			}
		}
		if i == len(value) || (value[i] == ',' && !inQuote) {
			part := strings.TrimSpace(value[start:i])
			if part == "" {
				return nil, common.ValidationErrorf("range list contains an empty item")
			}
			ranges = append(ranges, part)
			start = i + 1
		}
	}
	if inQuote {
		return nil, common.ValidationErrorf("unterminated quoted sheet name")
	}
	return ranges, nil
}

func parseChartDataRange(value string) (chartDataRange, error) {
	item := chartDataRange{}
	ref := strings.TrimSpace(value)
	if sheet, end, ok := scanSheetQualifier(ref); ok {
		item.sheet = sheet
		item.qualifier = strings.TrimSpace(ref[:end])
		ref = strings.TrimSpace(ref[end:])
	}
	parts := strings.SplitN(ref, ":", 2)
	if len(parts) != 2 {
		return item, common.ValidationErrorf("expected a rectangular A1 range such as A1:C10")
	}
	startCol, startRow, startOK := splitCellRef(parts[0])
	endCol, endRow, endOK := splitCellRef(parts[1])
	if !startOK || !endOK || endCol < startCol || endRow < startRow {
		return item, common.ValidationErrorf("expected a rectangular A1 range such as A1:C10")
	}
	item.row, item.col = startRow, startCol
	item.rowCount, item.colCount = endRow-startRow+1, endCol-startCol+1
	return item, nil
}

func configureChartSemanticCommand(cmd *cobra.Command) {
	if cmd.Flags().Lookup("stacked") == nil {
		cmd.Flags().Bool("stacked", false, "compatibility alias for --stack normal")
		_ = cmd.Flags().MarkHidden("stacked")
	}
	originalArgs := cmd.Args
	cmd.Args = func(cmd *cobra.Command, args []string) error {
		if len(args) == 1 && cmd.Flags().Changed("smooth") && (args[0] == "true" || args[0] == "false") {
			return cmd.Flags().Set("smooth", args[0])
		}
		return originalArgs(cmd, args)
	}
	cmd.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		message := err.Error()
		if strings.Contains(message, "unknown flag: --stacked") {
			return sheetsValidationForFlag("stacked", "--stacked is not supported; use --stack normal (or --stack percent for 100%% stacking)")
		}
		return err
	})
}

func addChartSemanticConfig(rt flagView, out map[string]interface{}) {
	for _, flag := range chartSemanticConfigFlags {
		if !rt.Changed(flag) {
			continue
		}
		key := strings.ReplaceAll(flag, "-", "_")
		if flag == "x-axis-label-angle" || flag == "y-axis-label-angle" {
			out[key] = rt.Int(flag)
		} else if flag == "x-axis-min" || flag == "x-axis-max" || flag == "y-axis-min" || flag == "y-axis-max" {
			out[key] = rt.Float64(flag)
		} else {
			out[key] = rt.Str(flag)
		}
	}
	if rt.Changed("stacked") {
		if rt.Bool("stacked") {
			out["stack"] = "normal"
		} else {
			out["stack"] = "none"
		}
	}
	if rt.Changed("smooth") {
		out["smooth"] = rt.Bool("smooth")
	}
	if rt.Changed("colors") {
		out["colors"] = normalizedChartColors(rt)
	}
}

func validateChartSemanticEnums(rt flagView) error {
	if rt.Changed("stack") && rt.Changed("stacked") {
		return common.ValidationErrorf("--stack and --stacked are mutually exclusive").WithParams(
			sheetsInvalidParam("stack", "cannot be used with --stacked"),
			sheetsInvalidParam("stacked", "cannot be used with --stack"),
		)
	}
	return nil
}

func validateChartConfigSnapshot(rt flagView, snapshot map[string]interface{}) error {
	if rt.Changed("data-label-position") && !rt.Changed("data-labels") {
		plotArea := chartMap(snapshot["plotArea"])
		plot := chartMap(plotArea["plot"])
		if _, exists := plot["labels"].(map[string]interface{}); !exists {
			return sheetsValidationForFlag(
				"data-label-position",
				"--data-label-position requires existing data labels or --data-labels in the same update",
			)
		}
	}
	if !rt.Changed("x-axis-min") && !rt.Changed("x-axis-max") {
		return nil
	}
	plotArea := chartMap(snapshot["plotArea"])
	xAxis := findChartAxisMap(plotArea, "x", "bottom")
	if valueType, _ := xAxis["valueType"].(string); valueType == "linear" {
		return nil
	}
	flag := "x-axis-min"
	if !rt.Changed(flag) {
		flag = "x-axis-max"
	}
	return sheetsValidationForFlag(
		flag,
		"--%s requires the existing bottom X axis to be a continuous numeric X axis (valueType=linear)",
		flag,
	)
}

func validateChartAxisBounds(rt flagView, axis string, numericAxis bool) error {
	minFlag := axis + "-axis-min"
	maxFlag := axis + "-axis-max"
	hasMin := rt.Changed(minFlag)
	hasMax := rt.Changed(maxFlag)
	if !hasMin && !hasMax {
		return nil
	}
	if !numericAxis {
		return sheetsValidationForFlag(
			"x-axis-numbers-as",
			"--%s and --%s require --x-axis-numbers-as values",
			minFlag,
			maxFlag,
		)
	}
	if hasMin {
		value := rt.Float64(minFlag)
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return sheetsValidationForFlag(minFlag, "--%s must be a finite number", minFlag)
		}
	}
	if hasMax {
		value := rt.Float64(maxFlag)
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return sheetsValidationForFlag(maxFlag, "--%s must be a finite number", maxFlag)
		}
	}
	if hasMin && hasMax && rt.Float64(minFlag) >= rt.Float64(maxFlag) {
		return sheetsValidationForFlag(minFlag, "--%s must be less than --%s", minFlag, maxFlag)
	}
	return nil
}

func validateChartColorFlags(rt flagView) error {
	if rt.Changed("color-palette") && rt.Changed("colors") {
		return common.ValidationErrorf("--color-palette and --colors are mutually exclusive").WithParams(
			sheetsInvalidParam("color-palette", "cannot be used with --colors"),
			sheetsInvalidParam("colors", "cannot be used with --color-palette"),
		)
	}
	if rt.Changed("colors") {
		colors := normalizedChartColors(rt)
		if len(colors) < 2 {
			return sheetsValidationForFlag("colors", "--colors must contain at least two hex colors")
		}
		for _, color := range colors {
			if !chartHexColorPattern.MatchString(color) {
				return sheetsValidationForFlag("colors", "--colors contains invalid hex color %q", color)
			}
		}
	}
	return nil
}

func normalizedChartColors(rt flagView) []string {
	raw := rt.StrSlice("colors")
	colors := make([]string, len(raw))
	for i := range raw {
		colors[i] = strings.TrimSpace(raw[i])
	}
	return colors
}
