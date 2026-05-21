// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"context"
	"strings"

	"github.com/larksuite/cli/shortcuts/common"
)

// ─── object CRUD shortcuts ────────────────────────────────────────────
//
// Six object skills (chart / pivot table / conditional format / filter /
// filter view / sparkline / float image) each expose a uniform create /
// update / delete trio backed by their manage_<obj>_object tool.
//
// Shared shape:
//   excel_id + sheet_id|sheet_name + operation + [<obj>_id] + [properties]
//
// CLI `--data` is passed through as the tool's `properties` payload as-is —
// callers shape it per the spec doc for each object (which is what makes
// the surface narrow even though everything funnels through one tool).
//
// Five of the seven objects share the factory below (newObjectCRUDShortcuts).
// pivot adds optional --target-sheet-id / --target-position on create,
// declared with extraCreateFlags. filter is special-cased further down
// (no separate id flag — filter_id is implicit per sheet — and --range is
// a first-class create flag, not buried in --data).

// objectCRUDSpec describes a 3-shortcut create/update/delete cluster.
// idFlag / idField empty → no per-object id flag (only filter uses that
// today, and it has its own bespoke shortcuts further down).
type objectCRUDSpec struct {
	commandPrefix string // e.g. "+chart" → +chart-create / -update / -delete
	toolName      string // e.g. "manage_chart_object"
	idFlag        string // e.g. "chart-id"
	idField       string // e.g. "chart_id"
	// enhanceCreateInput / enhanceUpdateInput, when set, mutate the tool
	// input after the standard fields are written. Used to inject
	// shortcut-specific flat flags into the input (typically into the
	// properties map). The callback is responsible for navigating to the
	// right nesting level.
	enhanceCreateInput func(rt flagView, input map[string]interface{})
	enhanceUpdateInput func(rt flagView, input map[string]interface{})
	// validateUpdateInput, when set, runs after enhanceUpdateInput to
	// enforce constraints that span across input fields (e.g. sparkline
	// requires properties.sparklines[i] to carry sparkline_id on update —
	// a server contract the CLI now surfaces with a pointer to
	// +sparkline-list instead of letting the caller hit an opaque
	// server-side rejection).
	validateUpdateInput func(input map[string]interface{}) error
}

func newObjectCreateShortcut(spec objectCRUDSpec) common.Shortcut {
	flags := flagsFor(spec.commandPrefix + "-create")
	return common.Shortcut{
		Service:     "sheets",
		Command:     spec.commandPrefix + "-create",
		Description: "Create a " + strings.TrimPrefix(spec.commandPrefix, "+") + " object via the manage_*_object tool.",
		Risk:        "write",
		Scopes:      []string{"sheets:spreadsheet:write_only"},
		AuthTypes:   []string{"user", "bot"},
		HasFormat:   true,
		Flags:       flags,
		Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
			token, err := resolveSpreadsheetToken(runtime)
			if err != nil {
				return err
			}
			sheetID := strings.TrimSpace(runtime.Str("sheet-id"))
			sheetName := strings.TrimSpace(runtime.Str("sheet-name"))
			_, err = objectCreateInput(runtime, token, sheetID, sheetName, spec)
			return err
		},
		DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
			token, _ := resolveSpreadsheetToken(runtime)
			sheetID, sheetName, _ := resolveSheetSelector(runtime)
			input, _ := objectCreateInput(runtime, token, sheetID, sheetName, spec)
			return invokeToolDryRun(token, ToolKindWrite, spec.toolName, input)
		},
		Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
			token, err := resolveSpreadsheetToken(runtime)
			if err != nil {
				return err
			}
			sheetID, sheetName, err := resolveSheetSelector(runtime)
			if err != nil {
				return err
			}
			input, err := objectCreateInput(runtime, token, sheetID, sheetName, spec)
			if err != nil {
				return err
			}
			out, err := callTool(ctx, runtime, token, ToolKindWrite, spec.toolName, input)
			if err != nil {
				return err
			}
			runtime.Out(out, nil)
			return nil
		},
	}
}

func objectCreateInput(runtime flagView, token, sheetID, sheetName string, spec objectCRUDSpec) (map[string]interface{}, error) {
	if err := requireSheetSelector(sheetID, sheetName); err != nil {
		return nil, err
	}
	props, err := requireJSONObject(runtime, "properties")
	if err != nil {
		return nil, err
	}
	input := map[string]interface{}{
		"excel_id":   token,
		"operation":  "create",
		"properties": props,
	}
	sheetSelectorForToolInput(input, sheetID, sheetName)
	if spec.enhanceCreateInput != nil {
		spec.enhanceCreateInput(runtime, input)
	}
	return input, nil
}

func newObjectUpdateShortcut(spec objectCRUDSpec) common.Shortcut {
	flags := flagsFor(spec.commandPrefix + "-update")
	return common.Shortcut{
		Service:     "sheets",
		Command:     spec.commandPrefix + "-update",
		Description: "Update an existing " + strings.TrimPrefix(spec.commandPrefix, "+") + " object (read-modify-write; consult --list first).",
		Risk:        "write",
		Scopes:      []string{"sheets:spreadsheet:write_only"},
		AuthTypes:   []string{"user", "bot"},
		HasFormat:   true,
		Flags:       flags,
		Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
			token, err := resolveSpreadsheetToken(runtime)
			if err != nil {
				return err
			}
			sheetID := strings.TrimSpace(runtime.Str("sheet-id"))
			sheetName := strings.TrimSpace(runtime.Str("sheet-name"))
			_, err = objectUpdateInput(runtime, token, sheetID, sheetName, spec)
			return err
		},
		DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
			token, _ := resolveSpreadsheetToken(runtime)
			sheetID, sheetName, _ := resolveSheetSelector(runtime)
			input, _ := objectUpdateInput(runtime, token, sheetID, sheetName, spec)
			return invokeToolDryRun(token, ToolKindWrite, spec.toolName, input)
		},
		Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
			token, err := resolveSpreadsheetToken(runtime)
			if err != nil {
				return err
			}
			sheetID, sheetName, err := resolveSheetSelector(runtime)
			if err != nil {
				return err
			}
			input, err := objectUpdateInput(runtime, token, sheetID, sheetName, spec)
			if err != nil {
				return err
			}
			out, err := callTool(ctx, runtime, token, ToolKindWrite, spec.toolName, input)
			if err != nil {
				return err
			}
			runtime.Out(out, nil)
			return nil
		},
	}
}

func objectUpdateInput(runtime flagView, token, sheetID, sheetName string, spec objectCRUDSpec) (map[string]interface{}, error) {
	if err := requireSheetSelector(sheetID, sheetName); err != nil {
		return nil, err
	}
	if spec.idFlag != "" && strings.TrimSpace(runtime.Str(spec.idFlag)) == "" {
		return nil, common.FlagErrorf("--%s is required", spec.idFlag)
	}
	props, err := requireJSONObject(runtime, "properties")
	if err != nil {
		return nil, err
	}
	input := map[string]interface{}{
		"excel_id":   token,
		"operation":  "update",
		"properties": props,
	}
	sheetSelectorForToolInput(input, sheetID, sheetName)
	if spec.idFlag != "" {
		input[spec.idField] = strings.TrimSpace(runtime.Str(spec.idFlag))
	}
	if spec.enhanceUpdateInput != nil {
		spec.enhanceUpdateInput(runtime, input)
	}
	if spec.validateUpdateInput != nil {
		if err := spec.validateUpdateInput(input); err != nil {
			return nil, err
		}
	}
	return input, nil
}

func newObjectDeleteShortcut(spec objectCRUDSpec) common.Shortcut {
	flags := flagsFor(spec.commandPrefix + "-delete")
	return common.Shortcut{
		Service:     "sheets",
		Command:     spec.commandPrefix + "-delete",
		Description: "Delete a " + strings.TrimPrefix(spec.commandPrefix, "+") + " object (irreversible).",
		Risk:        "high-risk-write",
		Scopes:      []string{"sheets:spreadsheet:write_only"},
		AuthTypes:   []string{"user", "bot"},
		HasFormat:   true,
		Flags:       flags,
		Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
			token, err := resolveSpreadsheetToken(runtime)
			if err != nil {
				return err
			}
			sheetID := strings.TrimSpace(runtime.Str("sheet-id"))
			sheetName := strings.TrimSpace(runtime.Str("sheet-name"))
			_, err = objectDeleteInput(runtime, token, sheetID, sheetName, spec)
			return err
		},
		DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
			token, _ := resolveSpreadsheetToken(runtime)
			sheetID, sheetName, _ := resolveSheetSelector(runtime)
			input, _ := objectDeleteInput(runtime, token, sheetID, sheetName, spec)
			return invokeToolDryRun(token, ToolKindWrite, spec.toolName, input)
		},
		Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
			token, err := resolveSpreadsheetToken(runtime)
			if err != nil {
				return err
			}
			sheetID, sheetName, err := resolveSheetSelector(runtime)
			if err != nil {
				return err
			}
			input, err := objectDeleteInput(runtime, token, sheetID, sheetName, spec)
			if err != nil {
				return err
			}
			out, err := callTool(ctx, runtime, token, ToolKindWrite, spec.toolName, input)
			if err != nil {
				return err
			}
			runtime.Out(out, nil)
			return nil
		},
	}
}

func objectDeleteInput(runtime flagView, token, sheetID, sheetName string, spec objectCRUDSpec) (map[string]interface{}, error) {
	if err := requireSheetSelector(sheetID, sheetName); err != nil {
		return nil, err
	}
	if spec.idFlag != "" && strings.TrimSpace(runtime.Str(spec.idFlag)) == "" {
		return nil, common.FlagErrorf("--%s is required", spec.idFlag)
	}
	input := map[string]interface{}{
		"excel_id":  token,
		"operation": "delete",
	}
	sheetSelectorForToolInput(input, sheetID, sheetName)
	if spec.idFlag != "" {
		input[spec.idField] = strings.TrimSpace(runtime.Str(spec.idFlag))
	}
	return input, nil
}

// ─── per-object instantiations ────────────────────────────────────────

// chart
var chartSpec = objectCRUDSpec{
	commandPrefix: "+chart",
	toolName:      "manage_chart_object",
	idFlag:        "chart-id",
	idField:       "chart_id",
}
var ChartCreate = newObjectCreateShortcut(chartSpec)
var ChartUpdate = newObjectUpdateShortcut(chartSpec)
var ChartDelete = newObjectDeleteShortcut(chartSpec)

// pivot — create exposes --target-sheet-id / --target-position (top-level
// of the tool input) plus --source / --range hoisted from properties.
var pivotSpec = objectCRUDSpec{
	commandPrefix: "+pivot",
	toolName:      "manage_pivot_table_object",
	idFlag:        "pivot-table-id",
	idField:       "pivot_table_id",
	enhanceCreateInput: func(rt flagView, input map[string]interface{}) {
		if v := strings.TrimSpace(rt.Str("target-sheet-id")); v != "" {
			input["target_sheet_id"] = v
		}
		if v := strings.TrimSpace(rt.Str("target-position")); v != "" && v != "A1" {
			input["target_position"] = v
		}
		props, _ := input["properties"].(map[string]interface{})
		if props == nil {
			return
		}
		if v := strings.TrimSpace(rt.Str("source")); v != "" {
			props["source"] = v
		}
		if v := strings.TrimSpace(rt.Str("range")); v != "" {
			props["range"] = v
		}
	},
}
var PivotCreate = newObjectCreateShortcut(pivotSpec)
var PivotUpdate = newObjectUpdateShortcut(pivotSpec)
var PivotDelete = newObjectDeleteShortcut(pivotSpec)

// conditional format — CLI surface uses --rule-id (short), wired to the
// tool's conditional_format_id on the wire. --rule-type and --ranges are
// hoisted out of properties (both required, set on every CRUD write).
//
// Wire shape matches manage_conditional_format_object.properties — the
// enum value lives at properties.rule_type (flat string, NOT nested under
// a `rule` object), and ranges is a sibling array. Earlier CLI builds
// wrote properties.rule.type which the server silently dropped — both
// path and enum vocabulary are now aligned with the server schema (see
// sheet-skill-spec/canonical-spec/tool-schemas/mcp-tools.json line
// 3305-3324).
var condFormatEnhance = func(rt flagView, input map[string]interface{}) {
	props, _ := input["properties"].(map[string]interface{})
	if props == nil {
		return
	}
	if ruleType := strings.TrimSpace(rt.Str("rule-type")); ruleType != "" {
		props["rule_type"] = ruleType
	}
	if rt.Str("ranges") != "" {
		if arr, err := requireJSONArray(rt, "ranges"); err == nil {
			props["ranges"] = arr
		}
	}
}

var condFormatSpec = objectCRUDSpec{
	commandPrefix:      "+cond-format",
	toolName:           "manage_conditional_format_object",
	idFlag:             "rule-id",
	idField:            "conditional_format_id",
	enhanceCreateInput: condFormatEnhance,
	enhanceUpdateInput: condFormatEnhance,
}
var CondFormatCreate = newObjectCreateShortcut(condFormatSpec)
var CondFormatUpdate = newObjectUpdateShortcut(condFormatSpec)
var CondFormatDelete = newObjectDeleteShortcut(condFormatSpec)

// sparkline — CLI uses --group-id (higher level) as the object selector.
// Two-layer ID model: --group-id picks the sparkline group; individual
// items inside properties.sparklines[] are addressed by sparkline_id.
// On update the server requires sparkline_id on every item (it's how
// the server maps each entry back to an existing sparkline);
// validateSparklineUpdateItems surfaces that requirement CLI-side with
// a pointer to +sparkline-list instead of letting the caller hit a
// server-side rejection that doesn't mention sparkline_id at all.
//
// (sparkline-delete is intentionally not pre-checked here:
// objectDeleteInput doesn't pass properties through, so the partial-
// delete branch — properties.sparklines: [{sparkline_id}] — silently
// degrades to whole-group delete today. Surfacing that gap is a
// separate fix; this validator stays scoped to update.)
func validateSparklineUpdateItems(input map[string]interface{}) error {
	props, _ := input["properties"].(map[string]interface{})
	if props == nil {
		return nil
	}
	raw, ok := props["sparklines"]
	if !ok {
		return nil // config-only update — fine
	}
	arr, ok := raw.([]interface{})
	if !ok {
		return common.FlagErrorf("+sparkline-update properties.sparklines must be an array")
	}
	for i, item := range arr {
		m, _ := item.(map[string]interface{})
		if m == nil {
			return common.FlagErrorf("+sparkline-update properties.sparklines[%d] must be an object", i)
		}
		id, _ := m["sparkline_id"].(string)
		if strings.TrimSpace(id) == "" {
			return common.FlagErrorf("+sparkline-update properties.sparklines[%d] missing sparkline_id (run `+sparkline-list --group-id <id>` first to read sparkline_id for each item, then echo each id back on the corresponding update entry)", i)
		}
	}
	return nil
}

var sparklineSpec = objectCRUDSpec{
	commandPrefix:       "+sparkline",
	toolName:            "manage_sparkline_object",
	idFlag:              "group-id",
	idField:             "group_id",
	validateUpdateInput: validateSparklineUpdateItems,
}
var SparklineCreate = newObjectCreateShortcut(sparklineSpec)
var SparklineUpdate = newObjectUpdateShortcut(sparklineSpec)
var SparklineDelete = newObjectDeleteShortcut(sparklineSpec)

// float image — fully hoisted to 10 flat flags. No --properties flag;
// the tool's properties is composed entirely from the position / size /
// offset / image_token / image_uri / z_index flat flags.

// floatImageProperties assembles the tool's properties object from the
// 10 flat flags. Caller is responsible for marking required flags via
// cobra Required:true; this function only enforces the image_token XOR
// image_uri pair (one must be set).
func floatImageProperties(runtime flagView) (map[string]interface{}, error) {
	token := strings.TrimSpace(runtime.Str("image-token"))
	uri := strings.TrimSpace(runtime.Str("image-uri"))
	if token == "" && uri == "" {
		return nil, common.FlagErrorf("either --image-token or --image-uri is required")
	}
	if token != "" && uri != "" {
		return nil, common.FlagErrorf("--image-token and --image-uri are mutually exclusive")
	}
	props := map[string]interface{}{
		"image_name": strings.TrimSpace(runtime.Str("image-name")),
		"position": map[string]interface{}{
			"row": runtime.Int("position-row"),
			"col": strings.TrimSpace(runtime.Str("position-col")),
		},
		"size": map[string]interface{}{
			"width":  runtime.Int("size-width"),
			"height": runtime.Int("size-height"),
		},
	}
	if token != "" {
		props["image_token"] = token
	} else {
		props["image_uri"] = uri
	}
	if runtime.Changed("offset-row") || runtime.Changed("offset-col") {
		offset := map[string]interface{}{}
		if runtime.Changed("offset-row") {
			offset["row_offset"] = runtime.Int("offset-row")
		}
		if runtime.Changed("offset-col") {
			offset["col_offset"] = runtime.Int("offset-col")
		}
		props["offset"] = offset
	}
	if runtime.Changed("z-index") {
		props["z_index"] = runtime.Int("z-index")
	}
	return props, nil
}

func newFloatImageWriteShortcut(command, description, op string, withIDFlag, isHighRisk bool) common.Shortcut {
	risk := "write"
	if isHighRisk {
		risk = "high-risk-write"
	}
	flags := flagsFor(command)
	return common.Shortcut{
		Service:     "sheets",
		Command:     command,
		Description: description,
		Risk:        risk,
		Scopes:      []string{"sheets:spreadsheet:write_only"},
		AuthTypes:   []string{"user", "bot"},
		HasFormat:   true,
		Flags:       flags,
		Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
			token, err := resolveSpreadsheetToken(runtime)
			if err != nil {
				return err
			}
			sheetID := strings.TrimSpace(runtime.Str("sheet-id"))
			sheetName := strings.TrimSpace(runtime.Str("sheet-name"))
			_, err = floatImageWriteInput(runtime, token, sheetID, sheetName, op, withIDFlag)
			return err
		},
		DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
			token, _ := resolveSpreadsheetToken(runtime)
			sheetID, sheetName, _ := resolveSheetSelector(runtime)
			input, _ := floatImageWriteInput(runtime, token, sheetID, sheetName, op, withIDFlag)
			return invokeToolDryRun(token, ToolKindWrite, "manage_float_image_object", input)
		},
		Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
			token, err := resolveSpreadsheetToken(runtime)
			if err != nil {
				return err
			}
			sheetID, sheetName, err := resolveSheetSelector(runtime)
			if err != nil {
				return err
			}
			input, err := floatImageWriteInput(runtime, token, sheetID, sheetName, op, withIDFlag)
			if err != nil {
				return err
			}
			out, err := callTool(ctx, runtime, token, ToolKindWrite, "manage_float_image_object", input)
			if err != nil {
				return err
			}
			runtime.Out(out, nil)
			return nil
		},
	}
}

func floatImageWriteInput(runtime flagView, token, sheetID, sheetName, op string, withIDFlag bool) (map[string]interface{}, error) {
	if err := requireSheetSelector(sheetID, sheetName); err != nil {
		return nil, err
	}
	if withIDFlag && strings.TrimSpace(runtime.Str("float-image-id")) == "" {
		return nil, common.FlagErrorf("--float-image-id is required")
	}
	props, err := floatImageProperties(runtime)
	if err != nil {
		return nil, err
	}
	input := map[string]interface{}{
		"excel_id":   token,
		"operation":  op,
		"properties": props,
	}
	sheetSelectorForToolInput(input, sheetID, sheetName)
	if withIDFlag {
		input["float_image_id"] = strings.TrimSpace(runtime.Str("float-image-id"))
	}
	return input, nil
}

var FloatImageCreate = newFloatImageWriteShortcut(
	"+float-image-create",
	"Create a floating image (referenced by --image-token or --image-uri).",
	"create", false, false,
)
var FloatImageUpdate = newFloatImageWriteShortcut(
	"+float-image-update",
	"Update an existing floating image (target by --float-image-id; provide the full set of flat flags).",
	"update", true, false,
)

// FloatImageDelete uses the standard CRUD delete factory since it only
// needs --float-image-id + --yes.
var floatImageDeleteSpec = objectCRUDSpec{
	commandPrefix: "+float-image",
	toolName:      "manage_float_image_object",
	idFlag:        "float-image-id",
	idField:       "float_image_id",
}
var FloatImageDelete = newObjectDeleteShortcut(floatImageDeleteSpec)

// filter view — cli_status: cli-only but the tool is in mcp-tools.json so
// it dispatches via the same One-OpenAPI endpoint as every other shortcut.
// --view-name and --range are hoisted out of properties (optional on both
// create and update; they always win over properties.{view_name, range}).
var filterViewEnhance = func(rt flagView, input map[string]interface{}) {
	props, _ := input["properties"].(map[string]interface{})
	if props == nil {
		return
	}
	if v := strings.TrimSpace(rt.Str("range")); v != "" {
		props["range"] = v
	}
	if v := strings.TrimSpace(rt.Str("view-name")); v != "" {
		props["view_name"] = v
	}
}

var filterViewSpec = objectCRUDSpec{
	commandPrefix:      "+filter-view",
	toolName:           "manage_filter_view_object",
	idFlag:             "view-id",
	idField:            "view_id",
	enhanceCreateInput: filterViewEnhance,
	enhanceUpdateInput: filterViewEnhance,
}
var FilterViewCreate = newObjectCreateShortcut(filterViewSpec)
var FilterViewUpdate = newObjectUpdateShortcut(filterViewSpec)
var FilterViewDelete = newObjectDeleteShortcut(filterViewSpec)

// ─── filter (sheet-scoped, no separate filter_id) ─────────────────────
//
// At most one filter per sheet, so filter_id is implicit (the tool treats
// filter_id and sheet_id as the same value). create requires --range
// (covering the header) and an optional --data with conditions; update
// patches conditions / range; delete drops the entire filter.

// FilterCreate creates a sheet-level filter. --range covers the data
// (header inclusive). --data is optional — empty filter is valid.
var FilterCreate = common.Shortcut{
	Service:     "sheets",
	Command:     "+filter-create",
	Description: "Create a sheet-level filter (one per sheet).",
	Risk:        "write",
	Scopes:      []string{"sheets:spreadsheet:write_only"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags:       flagsFor("+filter-create"),
	Validate:    validateViaInput(filterCreateInput),
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, _ := resolveSpreadsheetToken(runtime)
		sheetID, sheetName, _ := resolveSheetSelector(runtime)
		input, _ := filterCreateInput(runtime, token, sheetID, sheetName)
		return invokeToolDryRun(token, ToolKindWrite, "manage_filter_object", input)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token, err := resolveSpreadsheetToken(runtime)
		if err != nil {
			return err
		}
		sheetID, sheetName, err := resolveSheetSelector(runtime)
		if err != nil {
			return err
		}
		input, err := filterCreateInput(runtime, token, sheetID, sheetName)
		if err != nil {
			return err
		}
		out, err := callTool(ctx, runtime, token, ToolKindWrite, "manage_filter_object", input)
		if err != nil {
			return err
		}
		runtime.Out(out, nil)
		return nil
	},
}

func filterCreateInput(runtime flagView, token, sheetID, sheetName string) (map[string]interface{}, error) {
	if err := requireSheetSelector(sheetID, sheetName); err != nil {
		return nil, err
	}
	if strings.TrimSpace(runtime.Str("range")) == "" {
		return nil, common.FlagErrorf("--range is required")
	}
	props := map[string]interface{}{
		"range": strings.TrimSpace(runtime.Str("range")),
	}
	if runtime.Str("properties") != "" {
		extra, err := requireJSONObject(runtime, "properties")
		if err != nil {
			return nil, err
		}
		for k, v := range extra {
			if k == "range" {
				continue // --range wins
			}
			props[k] = v
		}
	}
	input := map[string]interface{}{
		"excel_id":   token,
		"operation":  "create",
		"properties": props,
	}
	sheetSelectorForToolInput(input, sheetID, sheetName)
	return input, nil
}

// FilterUpdate patches the sheet-level filter. --properties carries the
// rules; --range is first-class and overrides any properties.range.
// filter_id is implicit (sheet-scoped).
var FilterUpdate = common.Shortcut{
	Service:     "sheets",
	Command:     "+filter-update",
	Description: "Update the sheet-level filter (overwrite rules + range).",
	Risk:        "write",
	Scopes:      []string{"sheets:spreadsheet:write_only"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags:       flagsFor("+filter-update"),
	Validate:    validateViaInput(filterUpdateInput),
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, _ := resolveSpreadsheetToken(runtime)
		sheetID, sheetName, _ := resolveSheetSelector(runtime)
		input, _ := filterUpdateInput(runtime, token, sheetID, sheetName)
		return invokeToolDryRun(token, ToolKindWrite, "manage_filter_object", input)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token, err := resolveSpreadsheetToken(runtime)
		if err != nil {
			return err
		}
		sheetID, sheetName, err := resolveSheetSelector(runtime)
		if err != nil {
			return err
		}
		input, err := filterUpdateInput(runtime, token, sheetID, sheetName)
		if err != nil {
			return err
		}
		out, err := callTool(ctx, runtime, token, ToolKindWrite, "manage_filter_object", input)
		if err != nil {
			return err
		}
		runtime.Out(out, nil)
		return nil
	},
}

func filterUpdateInput(runtime flagView, token, sheetID, sheetName string) (map[string]interface{}, error) {
	if err := requireSheetSelector(sheetID, sheetName); err != nil {
		return nil, err
	}
	if sheetID == "" {
		return nil, common.FlagErrorf("+filter-update requires --sheet-id (filter_id must equal sheet_id; --sheet-name needs a network lookup unavailable here — call +workbook-info first or pass --sheet-id directly)")
	}
	if strings.TrimSpace(runtime.Str("range")) == "" {
		return nil, common.FlagErrorf("--range is required")
	}
	props, err := requireJSONObject(runtime, "properties")
	if err != nil {
		return nil, err
	}
	// --range wins over any properties.range
	props["range"] = strings.TrimSpace(runtime.Str("range"))
	input := map[string]interface{}{
		"excel_id":   token,
		"operation":  "update",
		"filter_id":  sheetID, // server contract: filter_id === sheet_id for sheet-scoped filters
		"properties": props,
	}
	sheetSelectorForToolInput(input, sheetID, sheetName)
	return input, nil
}

// FilterDelete drops the sheet-level filter entirely. high-risk-write.
var FilterDelete = common.Shortcut{
	Service:     "sheets",
	Command:     "+filter-delete",
	Description: "Remove the sheet-level filter (irreversible).",
	Risk:        "high-risk-write",
	Scopes:      []string{"sheets:spreadsheet:write_only"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags:       flagsFor("+filter-delete"),
	Validate:    validateViaInput(filterDeleteInput),
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, _ := resolveSpreadsheetToken(runtime)
		sheetID, sheetName, _ := resolveSheetSelector(runtime)
		input, _ := filterDeleteInput(runtime, token, sheetID, sheetName)
		return invokeToolDryRun(token, ToolKindWrite, "manage_filter_object", input)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token, err := resolveSpreadsheetToken(runtime)
		if err != nil {
			return err
		}
		sheetID, sheetName, err := resolveSheetSelector(runtime)
		if err != nil {
			return err
		}
		input, err := filterDeleteInput(runtime, token, sheetID, sheetName)
		if err != nil {
			return err
		}
		out, err := callTool(ctx, runtime, token, ToolKindWrite, "manage_filter_object", input)
		if err != nil {
			return err
		}
		runtime.Out(out, nil)
		return nil
	},
}

// filterDeleteInput mirrors the standalone +filter-delete body for batch
// sub-op reuse. Server contract: filter_id === sheet_id, and update/delete
// must populate filter_id (per manage_filter_object schema). The CLI has no
// separate --filter-id flag because the value is fully derived from sheet_id;
// only --sheet-id is accepted (not --sheet-name, since there's no mid-call
// network lookup to resolve it).
func filterDeleteInput(runtime flagView, token, sheetID, sheetName string) (map[string]interface{}, error) {
	if err := requireSheetSelector(sheetID, sheetName); err != nil {
		return nil, err
	}
	if sheetID == "" {
		return nil, common.FlagErrorf("+filter-delete requires --sheet-id (filter_id must equal sheet_id; --sheet-name needs a network lookup unavailable here — call +workbook-info first or pass --sheet-id directly)")
	}
	input := map[string]interface{}{
		"excel_id":  token,
		"operation": "delete",
		"filter_id": sheetID, // server contract: filter_id === sheet_id
	}
	sheetSelectorForToolInput(input, sheetID, sheetName)
	return input, nil
}
