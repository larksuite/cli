// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/validate"
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
// pivot opts into allowEmptySheetSelectorOnCreate=true so the backend can
// auto-create a placement sub-sheet when neither --sheet-id nor --sheet-name
// is given; it also exposes optional --target-position on create. filter is
// special-cased further down (no separate id flag — filter_id is implicit
// per sheet — and --range is a first-class create flag, not buried in --data).

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
	// enforce *cross-field, update-only* constraints JSON Schema can't
	// express (e.g. sparkline requires properties.sparklines[i] to
	// carry sparkline_id on update — same schema is shared with create
	// where the id is server-assigned). Type / enum / required /
	// nested-shape checks are not handled here: they run automatically
	// against data/flag-schemas.json in objectCreateInput /
	// objectUpdateInput via validatePropertiesAgainstSchema.
	validateUpdateInput func(input map[string]interface{}) error
	// allowEmptySheetSelectorOnCreate, when true, makes the *create*
	// shortcut accept empty --sheet-id / --sheet-name (backend then picks
	// the placement target — e.g. manage_pivot_table_object auto-creates
	// a sub-sheet to host the pivot). Both flags being set is still
	// rejected. Update/delete continue to require an explicit selector.
	// Today only pivotSpec opts in.
	allowEmptySheetSelectorOnCreate bool
	// createSheetIDFlag / createSheetNameFlag override the default
	// `sheet-id` / `sheet-name` flag names on the *create* shortcut and
	// its +batch-update sub-op. Used by pivot to expose
	// `target-sheet-id` / `target-sheet-name` — the placement target,
	// semantically distinct from the data-source sheet (which is encoded
	// in --source as `'SheetName'!Range`). Empty = default names.
	// Update/delete continue to use `sheet-id` / `sheet-name`.
	createSheetIDFlag   string
	createSheetNameFlag string
	// createTips, when set, populates the create shortcut's --help TIPS
	// section. Used by pivot to make "omit --target-* → backend auto-creates
	// a sub-sheet, zero overwrite" a hard, can't-miss note at the point of
	// use (the most-stepped-on #REF! trap in real trajectories).
	createTips []string
	// createWarn, when set, is evaluated on the create shortcut's dry-run and
	// execute paths; a non-empty return is surfaced as a `placement_warning`
	// field in the output. Used by pivot to flag a likely source-data overwrite
	// before it happens, without blocking the call. Local-only (no network), so
	// it stays safe to call from dry-run.
	createWarn func(rt flagView) string
}

// sheetIDFlagOnCreate / sheetNameFlagOnCreate return the cobra flag name
// used to read the placement-sheet selector on this spec's create
// shortcut. Defaults to `sheet-id` / `sheet-name`.
func (s objectCRUDSpec) sheetIDFlagOnCreate() string {
	if s.createSheetIDFlag != "" {
		return s.createSheetIDFlag
	}
	return "sheet-id"
}

func (s objectCRUDSpec) sheetNameFlagOnCreate() string {
	if s.createSheetNameFlag != "" {
		return s.createSheetNameFlag
	}
	return "sheet-name"
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
		Tips:        spec.createTips,
		Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
			token, err := resolveSpreadsheetToken(runtime)
			if err != nil {
				return err
			}
			sheetID := strings.TrimSpace(runtime.Str(spec.sheetIDFlagOnCreate()))
			sheetName := strings.TrimSpace(runtime.Str(spec.sheetNameFlagOnCreate()))
			_, err = objectCreateInput(runtime, token, sheetID, sheetName, spec)
			return err
		},
		DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
			token, _ := resolveSpreadsheetToken(runtime)
			sheetID := strings.TrimSpace(runtime.Str(spec.sheetIDFlagOnCreate()))
			sheetName := strings.TrimSpace(runtime.Str(spec.sheetNameFlagOnCreate()))
			input, _ := objectCreateInput(runtime, token, sheetID, sheetName, spec)
			dr := invokeToolDryRun(token, ToolKindWrite, spec.toolName, input)
			if spec.createWarn != nil {
				if w := spec.createWarn(runtime); w != "" {
					dr = dr.Set("placement_warning", w)
				}
			}
			return dr
		},
		Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
			token, err := resolveSpreadsheetToken(runtime)
			if err != nil {
				return err
			}
			sheetID := strings.TrimSpace(runtime.Str(spec.sheetIDFlagOnCreate()))
			sheetName := strings.TrimSpace(runtime.Str(spec.sheetNameFlagOnCreate()))
			input, err := objectCreateInput(runtime, token, sheetID, sheetName, spec)
			if err != nil {
				return err
			}
			out, err := callTool(ctx, runtime, token, ToolKindWrite, spec.toolName, input)
			if err != nil {
				return err
			}
			if spec.createWarn != nil {
				if w := spec.createWarn(runtime); w != "" {
					if m, ok := out.(map[string]interface{}); ok {
						m["placement_warning"] = w
					}
				}
			}
			runtime.Out(out, nil)
			return nil
		},
	}
}

func objectCreateInput(runtime flagView, token, sheetID, sheetName string, spec objectCRUDSpec) (map[string]interface{}, error) {
	var err error
	if spec.allowEmptySheetSelectorOnCreate {
		err = optionalSheetSelector(sheetID, sheetName, spec.sheetIDFlagOnCreate(), spec.sheetNameFlagOnCreate())
	} else {
		err = requireSheetSelector(sheetID, sheetName)
	}
	if err != nil {
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
	if err := validateInputAgainstSchema(runtime, input); err != nil {
		return nil, err
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
		return nil, sheetsValidationForFlag(spec.idFlag, "--%s is required", spec.idFlag)
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
	if err := validateInputAgainstSchema(runtime, input); err != nil {
		return nil, err
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
		return nil, sheetsValidationForFlag(spec.idFlag, "--%s is required", spec.idFlag)
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

// pivot — create exposes --target-position (top-level of the tool input)
// plus --source / --range hoisted from properties. --sheet-id / --sheet-name
// are the placement target (where the pivot table lands); the backend
// auto-creates a new sub-sheet when both are omitted, so create opts into
// allowEmptySheetSelectorOnCreate.
var pivotSpec = objectCRUDSpec{
	commandPrefix:                   "+pivot",
	toolName:                        "manage_pivot_table_object",
	idFlag:                          "pivot-table-id",
	idField:                         "pivot_table_id",
	allowEmptySheetSelectorOnCreate: true,
	createSheetIDFlag:               "target-sheet-id",
	createSheetNameFlag:             "target-sheet-name",
	createTips: []string{
		"Placement: omit --target-sheet-id / --target-sheet-name and the backend auto-creates a fresh sub-sheet for the pivot — zero overwrite risk. This is the default and the recommended path.",
		"Only pass --target-sheet-id/-name to land in an existing sheet; if that sheet holds the source data you MUST set --target-position (or --range) outside the data, else the pivot overwrites it and the anchor shows #REF!.",
		"Removing a stray pivot is +pivot-delete (get its id from +pivot-list); +cells-clear / +cells-batch-clear only clear cell values/formats and cannot delete the pivot object.",
	},
	createWarn: pivotPlacementWarn,
	enhanceCreateInput: func(rt flagView, input map[string]interface{}) {
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

// pivotPlacementWarn flags the one +pivot-create combination that silently
// overwrites data: an explicit placement sheet (--target-sheet-id/-name) with
// no offset (--target-position unset or A1, and no --range), so the pivot lands
// at A1 of an existing sheet. When that sheet is demonstrably the source-data
// sheet — target given by name, source carries a sheet prefix, names match —
// the warning is definite. When placement is by id (or the source has no
// prefix) the two can't be compared without a workbook lookup, which dry-run
// must avoid, so a conditional reminder is emitted instead. Returns "" when
// placement is safe (no target, or an offset was given). Advisory only: it is
// surfaced as placement_warning and never blocks the call.
func pivotPlacementWarn(rt flagView) string {
	tgtID := strings.TrimSpace(rt.Str("target-sheet-id"))
	tgtName := strings.TrimSpace(rt.Str("target-sheet-name"))
	if tgtID == "" && tgtName == "" {
		return "" // default path — backend auto-creates a sub-sheet, zero overwrite.
	}
	if pos := strings.TrimSpace(rt.Str("target-position")); pos != "" && pos != "A1" {
		return "" // caller steered the pivot off A1.
	}
	if strings.TrimSpace(rt.Str("range")) != "" {
		return "" // --range offset given.
	}
	srcSheet := sheetNameFromA1(rt.Str("source"))
	if tgtName != "" && srcSheet != "" {
		if strings.EqualFold(tgtName, srcSheet) {
			return fmt.Sprintf("--target-sheet-name %q is the source-data sheet and no --target-position is set: "+
				"the pivot lands at A1 and overwrites the source (the anchor then shows #REF!). Set --target-position "+
				"to a blank cell outside the data, or omit --target-* to auto-create a sub-sheet.", tgtName)
		}
		return "" // distinct named sheet — safe.
	}
	return "a placement sheet is set without --target-position: if it is the source-data sheet, the pivot lands " +
		"at A1 and overwrites the source (the anchor then shows #REF!). Set --target-position to a blank cell " +
		"outside the data, or omit --target-* to auto-create a sub-sheet."
}

// sheetNameFromA1 extracts the sheet name from a sheet-prefixed A1 reference,
// stripping the single quotes Lark wraps around names that contain spaces:
// "'Sheet 1'!A1:D100" → "Sheet 1", "Data!A1" → "Data". Returns "" when there
// is no sheet prefix. (splitSheetPrefixedRange keeps the quotes; this one drops
// them, which is what name comparison needs.)
func sheetNameFromA1(ref string) string {
	ref = strings.TrimSpace(ref)
	idx := strings.Index(ref, "!")
	if idx <= 0 {
		return ""
	}
	name := strings.TrimSpace(ref[:idx])
	if len(name) >= 2 && strings.HasPrefix(name, "'") && strings.HasSuffix(name, "'") {
		name = name[1 : len(name)-1]
	}
	return name
}

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
		return sheetsValidationForFlag("properties", "+sparkline-update properties.sparklines must be an array")
	}
	for i, item := range arr {
		m, _ := item.(map[string]interface{})
		if m == nil {
			return sheetsValidationForFlag("properties", "+sparkline-update properties.sparklines[%d] must be an object", i)
		}
		id, _ := m["sparkline_id"].(string)
		if strings.TrimSpace(id) == "" {
			return sheetsValidationForFlag("properties", "+sparkline-update properties.sparklines[%d] missing sparkline_id (run `+sparkline-list --group-id <id>` first to read sparkline_id for each item, then echo each id back on the corresponding update entry)", i)
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

// floatImageUploadPlaceholder is the stand-in image_token shown in
// Validate/DryRun for the --image (local upload) path, before the real
// file_token is known. Execute replaces it with the uploaded token.
const floatImageUploadPlaceholder = "<file_token>"

// floatImageName resolves the image name: explicit --image-name wins,
// otherwise fall back to the basename of a local --image path.
func floatImageName(runtime flagView) string {
	if n := strings.TrimSpace(runtime.Str("image-name")); n != "" {
		return n
	}
	if img := strings.TrimSpace(runtime.Str("image")); img != "" {
		return filepath.Base(img)
	}
	return ""
}

// floatImageProperties assembles the tool's properties object from the flat
// flags. The manage_float_image tool requires image_name, position and size on
// both create and update; the only difference is the image source:
//   - create (requireImageSource=true): exactly one of --image / --image-token
//     / --image-uri must be set.
//   - update (requireImageSource=false): the image source is optional — omit
//     all three to keep the current image; when given it stays mutually
//     exclusive. Despite the "patch" framing, the tool still rejects an update
//     missing image_name, position or size, and +float-image-list does not
//     return image_name for the CLI to backfill, so the caller must supply the
//     full core set.
//
// image_name, position and size are cobra-required on both create and update,
// so the standalone path is already gated by the flag layer; the explicit
// checks below are what enforces them on the +batch-update sub-op path, which
// has no cobra layer (mirrors the --float-image-id check in floatImageWriteInput).
//
// uploadedImageToken, when non-empty, is the file_token obtained by uploading a
// local --image (Execute only); in Validate/DryRun it is "" and a placeholder
// token stands in.
func floatImageProperties(runtime flagView, uploadedImageToken string, requireImageSource bool) (map[string]interface{}, error) {
	img := strings.TrimSpace(runtime.Str("image"))
	token := strings.TrimSpace(runtime.Str("image-token"))
	uri := strings.TrimSpace(runtime.Str("image-uri"))
	set := 0
	for _, v := range []string{img, token, uri} {
		if v != "" {
			set++
		}
	}
	if set == 0 && requireImageSource {
		return nil, common.ValidationErrorf("one of --image, --image-token, or --image-uri is required").WithParams(sheetsInvalidParam("image", "required; specify one"), sheetsInvalidParam("image-token", "required; specify one"), sheetsInvalidParam("image-uri", "required; specify one"))
	}
	if set > 1 {
		params := make([]errs.InvalidParam, 0, 3)
		if img != "" {
			params = append(params, sheetsInvalidParam("image", "mutually exclusive"))
		}
		if token != "" {
			params = append(params, sheetsInvalidParam("image-token", "mutually exclusive"))
		}
		if uri != "" {
			params = append(params, sheetsInvalidParam("image-uri", "mutually exclusive"))
		}
		return nil, common.ValidationErrorf("--image, --image-token, and --image-uri are mutually exclusive").WithParams(params...)
	}
	name := floatImageName(runtime)
	if name == "" {
		return nil, sheetsValidationForFlag("image-name", "--image-name is required")
	}
	if !runtime.Changed("position-row") || !runtime.Changed("position-col") {
		params := make([]errs.InvalidParam, 0, 2)
		if !runtime.Changed("position-row") {
			params = append(params, sheetsInvalidParam("position-row", "required"))
		}
		if !runtime.Changed("position-col") {
			params = append(params, sheetsInvalidParam("position-col", "required"))
		}
		return nil, common.ValidationErrorf("--position-row and --position-col are required").WithParams(params...)
	}
	if !runtime.Changed("size-width") || !runtime.Changed("size-height") {
		params := make([]errs.InvalidParam, 0, 2)
		if !runtime.Changed("size-width") {
			params = append(params, sheetsInvalidParam("size-width", "required"))
		}
		if !runtime.Changed("size-height") {
			params = append(params, sheetsInvalidParam("size-height", "required"))
		}
		return nil, common.ValidationErrorf("--size-width and --size-height are required").WithParams(params...)
	}
	props := map[string]interface{}{
		"image_name": name,
		"position": map[string]interface{}{
			"row": runtime.Int("position-row"),
			"col": strings.TrimSpace(runtime.Str("position-col")),
		},
		"size": map[string]interface{}{
			"width":  runtime.Int("size-width"),
			"height": runtime.Int("size-height"),
		},
	}
	switch {
	case img != "":
		// Local file: validate path safety here so --dry-run also rejects
		// unsafe paths; Execute uploads it and passes the real token in.
		if _, err := validate.SafeLocalFlagPath("--image", img); err != nil {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "%s", err).
				WithParam("--image").
				WithCause(err)
		}
		if uploadedImageToken != "" {
			props["image_token"] = uploadedImageToken
		} else {
			props["image_token"] = floatImageUploadPlaceholder
		}
	case token != "":
		props["image_token"] = token
	case uri != "":
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
			// uploadedImageToken="": Validate never uploads; floatImageProperties
			// still validates the --image path and the source XOR.
			_, err = floatImageWriteInput(runtime, token, sheetID, sheetName, op, withIDFlag, "")
			return err
		},
		DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
			token, _ := resolveSpreadsheetToken(runtime)
			sheetID, sheetName, _ := resolveSheetSelector(runtime)
			input, _ := floatImageWriteInput(runtime, token, sheetID, sheetName, op, withIDFlag, "")
			// With a local --image, Execute first uploads the file; surface that
			// extra step in the preview (mirrors +cells-set-image's dry-run).
			if img := strings.TrimSpace(runtime.Str("image")); img != "" {
				manageBody, _ := buildToolBody("manage_float_image_object", input)
				return common.NewDryRunAPI().
					POST("/open-apis/drive/v1/medias/upload_all").
					Desc("upload local image to drive (parent_type=sheet_image)").
					Body(map[string]interface{}{
						"file_name":   floatImageName(runtime),
						"parent_type": "sheet_image",
						"parent_node": token,
						"size":        "<file_size>",
						"file":        "@" + img,
					}).
					POST(toolInvokePath(token, ToolKindWrite)).
					Desc("create float image referencing the uploaded file_token").
					Body(manageBody)
			}
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
			// If a local --image was given, upload it first (parent_type=
			// sheet_image) and embed the returned file_token; otherwise this
			// returns "" and the token/uri flags are used as-is.
			uploadedImageToken, err := uploadFloatImageIfLocal(runtime, token)
			if err != nil {
				return err
			}
			input, err := floatImageWriteInput(runtime, token, sheetID, sheetName, op, withIDFlag, uploadedImageToken)
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

// uploadFloatImageIfLocal uploads a local --image (when set) as a sheet_image
// and returns its file_token. Returns ("", nil) when --image is not set (the
// token/uri source flags are used instead, e.g. on +float-image-update which
// does not register --image).
func uploadFloatImageIfLocal(runtime *common.RuntimeContext, spreadsheetToken string) (string, error) {
	img := strings.TrimSpace(runtime.Str("image"))
	if img == "" {
		return "", nil
	}
	info, err := runtime.FileIO().Stat(img)
	if err != nil {
		return "", sheetsInputStatError("image", err)
	}
	return common.UploadDriveMediaAllTyped(runtime, common.DriveMediaUploadAllConfig{
		FilePath:   img,
		FileName:   floatImageName(runtime),
		FileSize:   info.Size(),
		ParentType: "sheet_image",
		ParentNode: &spreadsheetToken,
	})
}

func floatImageWriteInput(runtime flagView, token, sheetID, sheetName, op string, withIDFlag bool, uploadedImageToken string) (map[string]interface{}, error) {
	if err := requireSheetSelector(sheetID, sheetName); err != nil {
		return nil, err
	}
	if withIDFlag && strings.TrimSpace(runtime.Str("float-image-id")) == "" {
		return nil, sheetsValidationForFlag("float-image-id", "--float-image-id is required")
	}
	props, err := floatImageProperties(runtime, uploadedImageToken, op == "create")
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
	"Create a floating image (from a local --image path, or an existing --image-token / --image-uri).",
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
		return nil, sheetsValidationForFlag("range", "--range is required")
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
	if err := validateInputAgainstSchema(runtime, input); err != nil {
		return nil, err
	}
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
		return nil, sheetsValidationForFlag("sheet-id", "+filter-update requires --sheet-id (filter_id must equal sheet_id; --sheet-name needs a network lookup unavailable here — call +workbook-info first or pass --sheet-id directly)")
	}
	if strings.TrimSpace(runtime.Str("range")) == "" {
		return nil, sheetsValidationForFlag("range", "--range is required")
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
	if err := validateInputAgainstSchema(runtime, input); err != nil {
		return nil, err
	}
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
		return nil, sheetsValidationForFlag("sheet-id", "+filter-delete requires --sheet-id (filter_id must equal sheet_id; --sheet-name needs a network lookup unavailable here — call +workbook-info first or pass --sheet-id directly)")
	}
	input := map[string]interface{}{
		"excel_id":  token,
		"operation": "delete",
		"filter_id": sheetID, // server contract: filter_id === sheet_id
	}
	sheetSelectorForToolInput(input, sheetID, sheetName)
	return input, nil
}
