// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"encoding/json"
	"sort"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
)

// TestPivotPlacementWarn pins the advisory that fires only on the risky
// +pivot-create combination — an explicit placement sheet with no offset —
// and stays silent (or only conditionally reminds) everywhere else.
func TestPivotPlacementWarn(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		raw  map[string]interface{}
		want string // "" none | "definite" names the sheet | "conditional" generic reminder
	}{
		{"no placement target → silent (default sub-sheet)",
			map[string]interface{}{"source": "'Sheet1'!A1:D100"}, ""},
		{"target-position offset → silent",
			map[string]interface{}{"target-sheet-name": "Sheet1", "source": "'Sheet1'!A1:D100", "target-position": "H1"}, ""},
		{"range offset → silent",
			map[string]interface{}{"target-sheet-id": "sht_x", "range": "H1"}, ""},
		{"target name == source sheet, no offset → definite",
			map[string]interface{}{"target-sheet-name": "Sheet1", "source": "'Sheet1'!A1:D100"}, "definite"},
		{"case-insensitive name match → definite",
			map[string]interface{}{"target-sheet-name": "sheet1", "source": "'Sheet1'!A1:D100"}, "definite"},
		{"target name != source sheet → silent (distinct sheet is safe)",
			map[string]interface{}{"target-sheet-name": "PivotOut", "source": "'Sheet1'!A1:D100"}, ""},
		{"target by id, no offset → conditional",
			map[string]interface{}{"target-sheet-id": "sht_abc", "source": "'Sheet1'!A1:D100"}, "conditional"},
		{"target name but source lacks prefix → conditional",
			map[string]interface{}{"target-sheet-name": "Sheet1", "source": "A1:D100"}, "conditional"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := pivotPlacementWarn(mapFlagView{raw: tc.raw})
			switch tc.want {
			case "":
				if got != "" {
					t.Errorf("expected no warning, got %q", got)
				}
			case "definite":
				if !strings.Contains(got, "--target-sheet-name") {
					t.Errorf("expected definite warning citing --target-sheet-name, got %q", got)
				}
			case "conditional":
				if !strings.Contains(got, "a placement sheet is set") {
					t.Errorf("expected conditional reminder, got %q", got)
				}
			}
		})
	}
}

// TestSheetNameFromA1 covers the source-sheet extraction used by the placement
// warning: prefix detection, single-quote stripping, and the no-prefix case.
func TestSheetNameFromA1(t *testing.T) {
	t.Parallel()
	tests := []struct{ in, want string }{
		{"'Sheet1'!A1:D100", "Sheet1"},
		{"Data!A1", "Data"},
		{"'My Sheet'!A1:B2", "My Sheet"},
		{"A1:D100", ""},
		{"", ""},
		{"  'X'!A1  ", "X"},
	}
	for _, tc := range tests {
		if got := sheetNameFromA1(tc.in); got != tc.want {
			t.Errorf("sheetNameFromA1(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestObjectCRUDShortcuts_DryRun walks the create / update / delete trio
// for each object skill. Together these cover all 21 CRUD shortcuts plus
// the per-object id flag renames (rule-id, group-id, view-id, etc.).
func TestObjectCRUDShortcuts_DryRun(t *testing.T) {
	t.Parallel()

	type spec struct {
		name      string
		sc        common.Shortcut
		args      []string
		toolName  string
		wantInput map[string]interface{}
	}

	tests := []spec{
		// chart
		{
			name:     "+chart-create",
			sc:       ChartCreate,
			args:     []string{"--url", testURL, "--sheet-id", testSheetID, "--properties", `{"type":"line","position":{"row":0,"col":"A"},"size":{"width":400,"height":300}}`},
			toolName: "manage_chart_object",
			wantInput: map[string]interface{}{
				"excel_id":  testToken,
				"sheet_id":  testSheetID,
				"operation": "create",
				"properties": map[string]interface{}{
					"type":     "line",
					"position": map[string]interface{}{"row": float64(0), "col": "A"},
					"size":     map[string]interface{}{"width": float64(400), "height": float64(300)},
				},
			},
		},
		{
			name:     "+chart-update",
			sc:       ChartUpdate,
			args:     []string{"--url", testURL, "--sheet-id", testSheetID, "--chart-id", "chartXYZ", "--properties", `{"type":"bar","position":{"row":0,"col":"A"},"size":{"width":400,"height":300}}`},
			toolName: "manage_chart_object",
			wantInput: map[string]interface{}{
				"excel_id":  testToken,
				"sheet_id":  testSheetID,
				"operation": "update",
				"chart_id":  "chartXYZ",
				"properties": map[string]interface{}{
					"type":     "bar",
					"position": map[string]interface{}{"row": float64(0), "col": "A"},
					"size":     map[string]interface{}{"width": float64(400), "height": float64(300)},
				},
			},
		},
		// pivot — has extra create flags incl. required --source.
		// --target-sheet-id is the placement target (where the pivot lands);
		// the placement selector is renamed from the generic --sheet-id /
		// --sheet-name to --target-sheet-id / --target-sheet-name to keep
		// it semantically distinct from the data-source sheet (which is
		// encoded inside --source as `'SheetName'!Range`).
		// pivotSpec.allowEmptySheetSelectorOnCreate lets both target
		// selectors be omitted so the backend auto-creates a sub-sheet —
		// covered separately in the +pivot-create empty-selector / mutex
		// tests below.
		{
			name: "+pivot-create with placement / source / target-position flags",
			sc:   PivotCreate,
			args: []string{
				"--url", testURL, "--target-sheet-id", testSheetID,
				"--properties", `{"rows":[{"field":"A"}]}`,
				"--source", "Sheet1!A1:F1000",
				"--target-position", "B5",
			},
			toolName: "manage_pivot_table_object",
			wantInput: map[string]interface{}{
				"excel_id":  testToken,
				"sheet_id":  testSheetID,
				"operation": "create",
				"properties": map[string]interface{}{
					"rows":   []interface{}{map[string]interface{}{"field": "A"}},
					"source": "Sheet1!A1:F1000",
					// --target-position 映射到 properties.range。
					"range": "B5",
				},
			},
		},
		// +pivot-create accepts both target selectors empty — backend
		// auto-creates a placement sub-sheet.
		{
			name: "+pivot-create empty --target-sheet-id / --target-sheet-name omits sheet from input",
			sc:   PivotCreate,
			args: []string{
				"--url", testURL,
				"--properties", `{"rows":[{"field":"A"}]}`,
				"--source", "Sheet1!A1:F1000",
			},
			toolName: "manage_pivot_table_object",
			wantInput: map[string]interface{}{
				"excel_id":  testToken,
				"operation": "create",
				"properties": map[string]interface{}{
					"rows":   []interface{}{map[string]interface{}{"field": "A"}},
					"source": "Sheet1!A1:F1000",
				},
			},
		},
		{
			name:     "+pivot-delete",
			sc:       PivotDelete,
			args:     []string{"--url", testURL, "--sheet-id", testSheetID, "--pivot-table-id", "ptA"},
			toolName: "manage_pivot_table_object",
			wantInput: map[string]interface{}{
				"excel_id":       testToken,
				"sheet_id":       testSheetID,
				"operation":      "delete",
				"pivot_table_id": "ptA",
			},
		},
		// cond-format — --rule-id rename + --rule-type / --ranges hoist.
		// rule_type lives at properties.rule_type (flat string), not nested
		// under a `rule` object; enum vocabulary matches server schema
		// (cellIs / duplicateValues / ... — see mcp-tools.json
		// manage_conditional_format_object.properties.rule_type).
		{
			name: "+cond-format-update id rename + rule-type/ranges",
			sc:   CondFormatUpdate,
			args: []string{
				"--url", testURL, "--sheet-id", testSheetID,
				"--rule-id", "ruleA",
				"--properties", `{"attrs":[{"compare_type":"greaterThan","value":"100"}],"style":{"back_color":"#FFD7D7"}}`,
				"--rule-type", "cellIs",
				"--ranges", `["A1:A100"]`,
			},
			toolName: "manage_conditional_format_object",
			wantInput: map[string]interface{}{
				"excel_id":              testToken,
				"sheet_id":              testSheetID,
				"operation":             "update",
				"conditional_format_id": "ruleA",
				"properties": map[string]interface{}{
					"rule_type": "cellIs",
					"attrs":     []interface{}{map[string]interface{}{"compare_type": "greaterThan", "value": "100"}},
					"style":     map[string]interface{}{"back_color": "#FFD7D7"},
					"ranges":    []interface{}{"A1:A100"},
				},
			},
		},
		// filter — special, no id flag
		{
			name:     "+filter-create without --properties sends properties.range only",
			sc:       FilterCreate,
			args:     []string{"--url", testURL, "--sheet-id", testSheetID, "--range", "A1:F1000", "--properties", `{"rules":[]}`},
			toolName: "manage_filter_object",
			wantInput: map[string]interface{}{
				"excel_id":  testToken,
				"sheet_id":  testSheetID,
				"operation": "create",
				"properties": map[string]interface{}{
					"range": "A1:F1000",
					"rules": []interface{}{},
				},
			},
		},
		{
			name:     "+filter-create with --properties merges rules",
			sc:       FilterCreate,
			args:     []string{"--url", testURL, "--sheet-id", testSheetID, "--range", "A1:F1000", "--properties", `{"rules":[{"column_index":"B","conditions":[{"type":"text","compare_type":"contains","values":["x"]}]}]}`},
			toolName: "manage_filter_object",
			wantInput: map[string]interface{}{
				"properties": map[string]interface{}{
					"range": "A1:F1000",
					"rules": []interface{}{map[string]interface{}{
						"column_index": "B",
						"conditions": []interface{}{map[string]interface{}{
							"type":         "text",
							"compare_type": "contains",
							"values":       []interface{}{"x"},
						}},
					}},
				},
			},
		},
		{
			// +filter-delete has no separate --filter-id flag because the
			// server contract sets filter_id === sheet_id; the translator
			// auto-injects filter_id from --sheet-id. update/delete fail
			// hard when only --sheet-name is given (no mid-call lookup).
			name:     "+filter-delete (sheet-scoped, auto-injects filter_id=sheet_id)",
			sc:       FilterDelete,
			args:     []string{"--url", testURL, "--sheet-id", testSheetID},
			toolName: "manage_filter_object",
			wantInput: map[string]interface{}{
				"excel_id":  testToken,
				"sheet_id":  testSheetID,
				"filter_id": testSheetID,
				"operation": "delete",
			},
		},
		{
			// +filter-update auto-injects filter_id from sheet_id, hoists
			// --range out of properties, and merges properties.rules.
			name: "+filter-update auto-injects filter_id, hoists --range",
			sc:   FilterUpdate,
			args: []string{
				"--url", testURL, "--sheet-id", testSheetID,
				"--range", "A1:F1000",
				"--properties", `{"rules":[{"column_index":"B","conditions":[{"type":"text","compare_type":"contains","values":["x"]}]}]}`,
			},
			toolName: "manage_filter_object",
			wantInput: map[string]interface{}{
				"excel_id":  testToken,
				"sheet_id":  testSheetID,
				"filter_id": testSheetID,
				"operation": "update",
				"properties": map[string]interface{}{
					"range": "A1:F1000",
					"rules": []interface{}{map[string]interface{}{
						"column_index": "B",
						"conditions": []interface{}{map[string]interface{}{
							"type":         "text",
							"compare_type": "contains",
							"values":       []interface{}{"x"},
						}},
					}},
				},
			},
		},
		// filter-view CRUD (cli-only via callTool)
		{
			name:     "+filter-view-create",
			sc:       FilterViewCreate,
			args:     []string{"--url", testURL, "--sheet-id", testSheetID, "--range", "A1:Z100", "--properties", `{"view_name":"v1"}`},
			toolName: "manage_filter_view_object",
			wantInput: map[string]interface{}{
				"excel_id":   testToken,
				"sheet_id":   testSheetID,
				"operation":  "create",
				"properties": map[string]interface{}{"view_name": "v1", "range": "A1:Z100"},
			},
		},
		{
			name:     "+filter-view-update with --view-id",
			sc:       FilterViewUpdate,
			args:     []string{"--url", testURL, "--sheet-id", testSheetID, "--view-id", "vABC", "--properties", `{"view_name":"renamed"}`},
			toolName: "manage_filter_view_object",
			wantInput: map[string]interface{}{
				"view_id":   "vABC",
				"operation": "update",
			},
		},
		// sparkline --group-id
		{
			name:     "+sparkline-update --group-id → group_id",
			sc:       SparklineUpdate,
			args:     []string{"--url", testURL, "--sheet-id", testSheetID, "--group-id", "grpA", "--properties", `{"type":"line"}`},
			toolName: "manage_sparkline_object",
			wantInput: map[string]interface{}{
				"group_id":   "grpA",
				"operation":  "update",
				"properties": map[string]interface{}{"type": "line"},
			},
		},
		{
			// happy path for the new sparkline_id check: each
			// properties.sparklines[i] carries sparkline_id, so the
			// validator passes through cleanly.
			name: "+sparkline-update properties.sparklines[] with sparkline_id passes",
			sc:   SparklineUpdate,
			args: []string{
				"--url", testURL, "--sheet-id", testSheetID, "--group-id", "grpA",
				"--properties", `{"sparklines":[{"sparkline_id":"sl1","source":"Sheet1!A1:A10"}]}`,
			},
			toolName: "manage_sparkline_object",
			wantInput: map[string]interface{}{
				"group_id":  "grpA",
				"operation": "update",
				"properties": map[string]interface{}{
					"sparklines": []interface{}{
						map[string]interface{}{"sparkline_id": "sl1", "source": "Sheet1!A1:A10"},
					},
				},
			},
		},
		// float-image — fully hoisted to flat flags
		{
			name: "+float-image-create with image-token + position/size",
			sc:   FloatImageCreate,
			args: []string{
				"--url", testURL, "--sheet-id", testSheetID,
				"--image-name", "logo.png",
				"--image-token", "tok_xyz",
				"--position-row", "2", "--position-col", "D",
				"--size-width", "300", "--size-height", "200",
			},
			toolName: "manage_float_image_object",
			wantInput: map[string]interface{}{
				"excel_id":  testToken,
				"sheet_id":  testSheetID,
				"operation": "create",
				"properties": map[string]interface{}{
					"image_name":  "logo.png",
					"image_token": "tok_xyz",
					"position":    map[string]interface{}{"row": float64(2), "col": "D"},
					"size":        map[string]interface{}{"width": float64(300), "height": float64(200)},
				},
			},
		},
		{
			// patch mode: position + size with no image source. The image
			// fields are omitted so the server keeps the current image; only
			// image_name (server-mandated) and the changed geometry are sent.
			// This is the shape that used to be rejected CLI-side.
			name: "+float-image-update patch position+size, no image source",
			sc:   FloatImageUpdate,
			args: []string{
				"--url", testURL, "--sheet-id", testSheetID,
				"--float-image-id", "imgABC", "--image-name", "logo.png",
				"--position-row", "10", "--position-col", "I",
				"--size-width", "90", "--size-height", "70",
			},
			toolName: "manage_float_image_object",
			wantInput: map[string]interface{}{
				"excel_id":       testToken,
				"sheet_id":       testSheetID,
				"operation":      "update",
				"float_image_id": "imgABC",
				"properties": map[string]interface{}{
					"image_name": "logo.png",
					"position":   map[string]interface{}{"row": float64(10), "col": "I"},
					"size":       map[string]interface{}{"width": float64(90), "height": float64(70)},
				},
			},
		},
		{
			// swap the image: an explicit --image-token rides alongside the
			// mandatory core (image_name + position + size).
			name: "+float-image-update swap image via image-token",
			sc:   FloatImageUpdate,
			args: []string{
				"--url", testURL, "--sheet-id", testSheetID,
				"--float-image-id", "imgABC",
				"--image-name", "new.png", "--image-token", "tok_new",
				"--position-row", "2", "--position-col", "B",
				"--size-width", "300", "--size-height", "200",
			},
			toolName: "manage_float_image_object",
			wantInput: map[string]interface{}{
				"excel_id":       testToken,
				"sheet_id":       testSheetID,
				"operation":      "update",
				"float_image_id": "imgABC",
				"properties": map[string]interface{}{
					"image_name":  "new.png",
					"image_token": "tok_new",
					"position":    map[string]interface{}{"row": float64(2), "col": "B"},
					"size":        map[string]interface{}{"width": float64(300), "height": float64(200)},
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

// TestPivotCreate_SheetSelectorSemantics locks in the "at most one"
// semantics for +pivot-create (and only +pivot-create): both
// --target-sheet-id and --target-sheet-name may be omitted (backend
// auto-creates a placement sub-sheet), but passing both is rejected.
//
// Companion regression — TestObjectCreate_RequiresSheetSelector below —
// confirms every other *-create still rejects empty selector.
func TestPivotCreate_SheetSelectorSemantics(t *testing.T) {
	t.Parallel()

	t.Run("both empty is accepted", func(t *testing.T) {
		t.Parallel()
		body := parseDryRunBody(t, PivotCreate, []string{
			"--url", testURL,
			"--properties", `{"rows":[{"field":"A"}]}`,
			"--source", "Sheet1!A1:F1000",
		})
		input := decodeToolInput(t, body, "manage_pivot_table_object")
		if _, ok := input["sheet_id"]; ok {
			t.Errorf("expected no sheet_id in input; got %v", input["sheet_id"])
		}
		if _, ok := input["sheet_name"]; ok {
			t.Errorf("expected no sheet_name in input; got %v", input["sheet_name"])
		}
	})

	t.Run("both set is rejected", func(t *testing.T) {
		t.Parallel()
		_, _, err := runShortcutCapturingErr(t, PivotCreate, []string{
			"--url", testURL,
			"--target-sheet-id", testSheetID,
			"--target-sheet-name", "Sheet1",
			"--properties", `{"rows":[{"field":"A"}]}`,
			"--source", "Sheet1!A1:F1000",
		})
		ve := requireValidation(t, err, "mutually exclusive")
		// 错误信息必须用真实的 flag 名（target-*），否则模型按消息提示去
		// 改 --sheet-id 还是错的。
		if !strings.Contains(ve.Message, "--target-sheet-id") {
			t.Errorf("expected error to quote --target-sheet-id flag name; got message=%q", ve.Message)
		}
	})

	t.Run("only target-sheet-id is accepted", func(t *testing.T) {
		t.Parallel()
		body := parseDryRunBody(t, PivotCreate, []string{
			"--url", testURL,
			"--target-sheet-id", testSheetID,
			"--properties", `{"rows":[{"field":"A"}]}`,
			"--source", "Sheet1!A1:F1000",
		})
		input := decodeToolInput(t, body, "manage_pivot_table_object")
		if got, _ := input["sheet_id"].(string); got != testSheetID {
			t.Errorf("sheet_id = %q, want %q", got, testSheetID)
		}
	})
}

// TestPivotCreate_TargetPositionRangeMutex regresses the "--target-position
// and --range cannot both be set" guardrail on +pivot-create. They map to
// the same wire field (properties.range), so two non-default values are
// ambiguous; the CLI rejects up front (mirrors the --target-sheet-id /
// --target-sheet-name mutex). --target-position=A1 is the documented default
// and is treated as "not set" — pairing it with --range still works.
func TestPivotCreate_TargetPositionRangeMutex(t *testing.T) {
	t.Parallel()

	t.Run("both non-default values rejected", func(t *testing.T) {
		t.Parallel()
		_, _, err := runShortcutCapturingErr(t, PivotCreate, []string{
			"--url", testURL,
			"--target-sheet-id", testSheetID,
			"--properties", `{"rows":[{"field":"A"}]}`,
			"--source", "Sheet1!A1:F1000",
			"--target-position", "B5",
			"--range", "F1",
		})
		ve := requireValidation(t, err, "mutually exclusive")
		if !strings.Contains(ve.Message, "--target-position") || !strings.Contains(ve.Message, "--range") {
			t.Errorf("expected error to quote both --target-position and --range; got message=%q", ve.Message)
		}
	})

	t.Run("default A1 with --range is accepted (range wins)", func(t *testing.T) {
		t.Parallel()
		body := parseDryRunBody(t, PivotCreate, []string{
			"--url", testURL,
			"--target-sheet-id", testSheetID,
			"--properties", `{"rows":[{"field":"A"}]}`,
			"--source", "Sheet1!A1:F1000",
			"--target-position", "A1",
			"--range", "F1",
		})
		input := decodeToolInput(t, body, "manage_pivot_table_object")
		props, _ := input["properties"].(map[string]interface{})
		if got, _ := props["range"].(string); got != "F1" {
			t.Errorf("properties.range = %q, want %q", got, "F1")
		}
	})
}

// TestPivotCreate_SchemaValidates exercises the schema-driven
// validator wired into objectCreateInput. The pivot create schema
// doesn't constrain rows/columns/values to be present (the backend
// just creates an empty shell), but it does pin types and enums —
// confirm both kinds of misuse are surfaced as CLI-side errors and
// that schema-conformant input is accepted.
func TestPivotCreate_SchemaValidates(t *testing.T) {
	t.Parallel()

	t.Run("rejects wrong type for rows", func(t *testing.T) {
		t.Parallel()
		_, _, err := runShortcutCapturingErr(t, PivotCreate, []string{
			"--url", testURL,
			"--properties", `{"rows":"not-an-array"}`,
			"--source", "Sheet1!A1:F1000",
			"--dry-run",
		})
		ve := requireValidation(t, err, "rows")
		if !strings.Contains(ve.Message, "array") {
			t.Errorf("expected error to mention array; got message=%q", ve.Message)
		}
	})

	t.Run("rejects out-of-enum summarize_by", func(t *testing.T) {
		t.Parallel()
		_, _, err := runShortcutCapturingErr(t, PivotCreate, []string{
			"--url", testURL,
			"--properties", `{"values":[{"field":"A","summarize_by":"BOGUS"}]}`,
			"--source", "Sheet1!A1:F1000",
			"--dry-run",
		})
		requireValidation(t, err, "summarize_by")
	})

	t.Run("schema-conformant input is accepted", func(t *testing.T) {
		t.Parallel()
		body := parseDryRunBody(t, PivotCreate, []string{
			"--url", testURL,
			"--properties", `{"values":[{"field":"A","summarize_by":"sum"}]}`,
			"--source", "Sheet1!A1:F1000",
		})
		decodeToolInput(t, body, "manage_pivot_table_object")
	})
}

// TestObjectCreate_RequiresSheetSelector regresses the non-pivot create
// shortcuts: pivot-create is the only one whose spec sets
// allowEmptySheetSelectorOnCreate=true. Every other *-create must still
// reject empty --sheet-id / --sheet-name (this is the guardrail that
// keeps the change minimally scoped).
func TestObjectCreate_RequiresSheetSelector(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		sc   common.Shortcut
		args []string // omit sheet selector flags on purpose
	}{
		{"chart", ChartCreate, []string{"--url", testURL, "--properties", `{"type":"line","position":{"row":0,"col":"A"},"size":{"width":400,"height":300}}`}},
		{"cond-format", CondFormatCreate, []string{"--url", testURL, "--properties", `{"attrs":[]}`, "--rule-type", "cellIs", "--ranges", `["A1:A10"]`}},
		{"sparkline", SparklineCreate, []string{"--url", testURL, "--properties", `{"sparklines":[]}`}},
		{"filter-view", FilterViewCreate, []string{"--url", testURL, "--properties", `{}`, "--range", "A1:F10"}},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, _, err := runShortcutCapturingErr(t, tt.sc, tt.args)
			requireValidation(t, err, "specify at least one of --sheet-id or --sheet-name")
		})
	}
}

// TestSparklineUpdate_MissingSparklineID confirms the standalone-path
// pre-check fires: +sparkline-update with properties.sparklines[] but no
// per-item sparkline_id must fail CLI-side with a pointer to
// +sparkline-list, before any server call goes out.
func TestSparklineUpdate_MissingSparklineID(t *testing.T) {
	t.Parallel()
	_, _, err := runShortcutCapturingErr(t, SparklineUpdate, []string{
		"--url", testURL, "--sheet-id", testSheetID, "--group-id", "grpA",
		"--properties", `{"sparklines":[{"source":"Sheet1!A1:A10"}]}`,
	})
	ve := requireValidation(t, err, "missing sparkline_id")
	if !strings.Contains(ve.Message, "+sparkline-list") {
		t.Errorf("expected error to point at +sparkline-list; got message=%q", ve.Message)
	}
}

// TestCondFormatAttrs_ShapeMatchesRuleType regresses the cross-field
// guard that rejects attrs whose shape doesn't match the sibling
// rule_type — the gap behind the "缺 color 的 colorScale 脏数据导致表格
// 打不开" report: a colorScale rule fed cellIs-shaped attrs
// ({compare_type,value}, no color) passed both the CLI's per-entry oneOf
// schema check and the tool, writing a color-less segment that crashed
// the frontend on open. The check covers create and update symmetrically.
func TestCondFormatAttrs_ShapeMatchesRuleType(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		sc      common.Shortcut
		args    []string
		wantErr bool
		wantMsg string // substring expected in the error, when wantErr
	}{
		{
			name: "colorScale fed cellIs-shaped attrs (missing color) is rejected",
			sc:   CondFormatCreate,
			args: []string{
				"--url", testURL, "--sheet-id", testSheetID,
				"--rule-type", "colorScale", "--ranges", `["C1:C10"]`,
				"--properties", `{"style":{},"attrs":[{"compare_type":"greaterThan","value":"0"},{"compare_type":"lessThan","value":"100"}]}`, "--dry-run",
			},
			wantErr: true,
			wantMsg: "colorScale",
		},
		{
			name: "colorScale with empty color string is rejected",
			sc:   CondFormatCreate,
			args: []string{
				"--url", testURL, "--sheet-id", testSheetID,
				"--rule-type", "colorScale", "--ranges", `["C1:C10"]`,
				"--properties", `{"style":{},"attrs":[{"value_type":"minValue","color":""},{"value_type":"maxValue","color":"#FF0000"}]}`, "--dry-run",
			},
			wantErr: true,
			wantMsg: `"color"`,
		},
		{
			name: "well-formed colorScale attrs pass",
			sc:   CondFormatCreate,
			args: []string{
				"--url", testURL, "--sheet-id", testSheetID,
				"--rule-type", "colorScale", "--ranges", `["C1:C10"]`,
				"--properties", `{"style":{},"attrs":[{"value_type":"minValue","color":"#FFFFFF"},{"value_type":"maxValue","color":"#FF0000"}]}`, "--dry-run",
			},
			wantErr: false,
		},
		{
			name: "update path is guarded too (colorScale + cellIs attrs)",
			sc:   CondFormatUpdate,
			args: []string{
				"--url", testURL, "--sheet-id", testSheetID, "--rule-id", "ruleA",
				"--rule-type", "colorScale", "--ranges", `["C1:C10"]`,
				"--properties", `{"style":{},"attrs":[{"compare_type":"greaterThan","value":"0"}]}`, "--dry-run",
			},
			wantErr: true,
			wantMsg: "colorScale",
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, stderr, err := runShortcutCapturingErr(t, tt.sc, tt.args)
			if tt.wantErr {
				requireValidation(t, err, tt.wantMsg)
				return
			}
			if err != nil {
				t.Fatalf("expected acceptance (dry-run); got err=%v stderr=%s", err, stderr)
			}
		})
	}
}

// TestCondFormatAttrsRequired_MatchesSchemaOneOf guards against drift
// between the hand-maintained condFormatAttrsRequired table (the source
// validateCondFormatAttrs enforces) and the embedded flag-schemas.json
// attrs oneOf (the authoritative shape contract synced from the spec
// repo). The cross-field validator only works if its per-rule_type
// required keys mirror the schema branches; if a future schema sync adds
// or drops a required key on any branch without updating the table, the
// CLI would silently under- or over-validate. They share no compile-time
// link, so this test is the only thing pinning them together.
//
// The schema oneOf branches are NOT labeled by rule_type (that's the whole
// point — rule_type is a sibling field the per-entry oneOf can't see), so
// we can't match branch→rule_type. We instead compare the *multiset* of
// required-key sets: every branch's required array must appear as some
// table entry's value and vice versa. This catches any added/dropped
// required key (real drift); it tolerates only a relabeling between two
// branches that happen to share an identical required set (dataBar and
// colorScale both require {color,value_type}), which is harmless here.
func TestCondFormatAttrsRequired_MatchesSchemaOneOf(t *testing.T) {
	t.Parallel()

	// multiset key: required keys sorted + joined, so order within a
	// branch's required array doesn't matter.
	keyOf := func(req []string) string {
		s := append([]string(nil), req...)
		sort.Strings(s)
		return strings.Join(s, "+")
	}

	tableMS := map[string]int{}
	for _, req := range condFormatAttrsRequired {
		tableMS[keyOf(req)]++
	}

	schemaMS := func(t *testing.T, command string) map[string]int {
		idx, err := loadFlagSchemas()
		if err != nil {
			t.Fatalf("loadFlagSchemas: %v", err)
		}
		raw, ok := idx.Flags[command]["properties"]
		if !ok {
			t.Fatalf("no embedded schema for %s --properties", command)
		}
		var schema map[string]interface{}
		if err := json.Unmarshal(raw, &schema); err != nil {
			t.Fatalf("unmarshal %s properties schema: %v", command, err)
		}
		dig := func(m map[string]interface{}, key string) map[string]interface{} {
			next, _ := m[key].(map[string]interface{})
			if next == nil {
				t.Fatalf("%s: missing %q while navigating to attrs oneOf", command, key)
			}
			return next
		}
		attrs := dig(dig(schema, "properties"), "attrs")
		items := dig(attrs, "items")
		oneOf, ok := items["oneOf"].([]interface{})
		if !ok || len(oneOf) == 0 {
			t.Fatalf("%s: attrs.items.oneOf is missing or empty", command)
		}
		ms := map[string]int{}
		for i, branchRaw := range oneOf {
			branch, ok := branchRaw.(map[string]interface{})
			if !ok {
				t.Fatalf("%s: oneOf[%d] is not an object", command, i)
			}
			reqRaw, _ := branch["required"].([]interface{})
			req := make([]string, 0, len(reqRaw))
			for _, r := range reqRaw {
				if s, ok := r.(string); ok {
					req = append(req, s)
				}
			}
			ms[keyOf(req)]++
		}
		return ms
	}

	for _, command := range []string{"+cond-format-create", "+cond-format-update"} {
		got := schemaMS(t, command)
		if len(got) != len(tableMS) {
			t.Errorf("%s: schema oneOf has %d distinct required-sets, table has %d", command, len(got), len(tableMS))
		}
		for k, n := range tableMS {
			if got[k] != n {
				t.Errorf("%s: required-set %q appears %d× in schema but %d× in condFormatAttrsRequired — table drifted from schema; re-sync the table", command, k, got[k], n)
			}
		}
		for k, n := range got {
			if tableMS[k] != n {
				t.Errorf("%s: schema branch with required-set %q (×%d) has no matching condFormatAttrsRequired entry — add it to the table", command, k, n)
			}
		}
	}
}

// Note: +float-image-update's image_name / position / size are cobra-required
// (flag-defs.json), so the standalone path is gated by the flag layer — its
// "required flag(s) … not set" wording is framework-owned and intentionally not
// re-asserted here. The CLI-side enforcement that matters is on the
// +batch-update sub-op path (no cobra layer); that is covered by
// TestBatchOp_RejectsBadSubOpInput in batch_op_contract_test.go.

// TestFloatImageCreate_RequiresImageSource guards the asymmetry with update:
// create still mandates one of --image / --image-token / --image-uri.
func TestFloatImageCreate_RequiresImageSource(t *testing.T) {
	t.Parallel()
	_, _, err := runShortcutCapturingErr(t, FloatImageCreate, []string{
		"--url", testURL, "--sheet-id", testSheetID,
		"--image-name", "x.png",
		"--position-row", "0", "--position-col", "A",
		"--size-width", "10", "--size-height", "10",
	})
	requireValidation(t, err, "one of --image, --image-token, or --image-uri is required")
}

// TestObjectDelete_AllHighRisk asserts every delete shortcut blocks
// without --yes (framework-enforced).
func TestObjectDelete_AllHighRisk(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		sc   common.Shortcut
		args []string
	}{
		{"chart", ChartDelete, []string{"--url", testURL, "--sheet-id", testSheetID, "--chart-id", "x"}},
		{"pivot", PivotDelete, []string{"--url", testURL, "--sheet-id", testSheetID, "--pivot-table-id", "x"}},
		{"cond-format", CondFormatDelete, []string{"--url", testURL, "--sheet-id", testSheetID, "--rule-id", "x"}},
		{"filter", FilterDelete, []string{"--url", testURL, "--sheet-id", testSheetID}},
		{"filter-view", FilterViewDelete, []string{"--url", testURL, "--sheet-id", testSheetID, "--view-id", "x"}},
		{"sparkline", SparklineDelete, []string{"--url", testURL, "--sheet-id", testSheetID, "--group-id", "x"}},
		{"float-image", FloatImageDelete, []string{"--url", testURL, "--sheet-id", testSheetID, "--float-image-id", "x"}},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, _, err := runShortcutCapturingErr(t, tt.sc, tt.args)
			requireProblem(t, err, errs.CategoryConfirmation, errs.SubtypeConfirmationRequired, "")
		})
	}
}
