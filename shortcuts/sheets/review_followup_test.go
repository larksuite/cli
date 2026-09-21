// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/httpmock"
)

// A value that is not a STRING cannot be the placeholder the dtype demotion
// exists for. stringifyCellValue would render an object / array / bool as Go
// debug text ("map[a:1]", "[1 2]", "TRUE") — irreversible corruption under a
// success exit code.
func TestDtypeDemotion_OnlyStringsAreDemoted(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, value string
		wantErr     bool
	}{
		{"placeholder string", `"—"`, false},
		{"unit-suffixed string", `"75分"`, false},
		{"object", `{"a":1}`, true},
		{"array", `[1,2]`, true},
		{"bool", `true`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, _, err := runShortcutCapturingErr(t, shortcutFromRegistry(t, "+workbook-create"), []string{
				"--title", "T", "--dry-run",
				"--sheets", `{"sheets":[{"name":"S","columns":["a"],"dtypes":{"a":"float64"},"data":[[` + tc.value + `]]}]}`,
			})
			if tc.wantErr {
				requireValidation(t, err, "number expects a numeric value")
				return
			}
			if err != nil {
				t.Fatalf("a string placeholder was rejected: %v", err)
			}
		})
	}
}

// dtypes and formats each read only the keys that can mean them. One shared
// "first match wins" list took a format string as a dtype and back.
func TestColumnLabels_ObjectKeysAreFieldScoped(t *testing.T) {
	t.Parallel()
	var in tableSheetIn
	if err := jsonUnmarshalSheet(t, `{"name":"S","columns":["amount"],
	  "dtypes":{"amount":{"format":"#,##0","dtype":"float64"}},
	  "formats":{"amount":{"dtype":"float64","format":"#,##0"}}}`, &in); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got := in.Dtypes.byName["amount"]; got != "float64" {
		t.Errorf("dtypes read %q, want the dtype — a format string here writes the column as text", got)
	}
	if got := in.Formats.byName["amount"]; got != "#,##0" {
		t.Errorf("formats read %q, want the number format", got)
	}
}

// An unknown --dimension must stop the call. The flag is registered
// dynamically, so it is absent from flag-defs and nothing downstream would
// catch "bananas" — least of all on a command that deletes rows.
func TestDimension_UnknownValueIsRefused(t *testing.T) {
	t.Parallel()
	_, _, err := runShortcutCapturingErr(t, shortcutFromRegistry(t, "+dim-delete"), []string{
		"--url", testURL, "--sheet-name", "S",
		"--dimension", "bananas", "--range", "3:5", "--yes", "--dry-run",
	})
	requireValidation(t, err, `invalid value "bananas" for --dimension`)
}

// Two explicit targets that disagree are the caller naming two spans; picking
// one would unmerge a region they never mentioned.
func TestSingletonRanges_ConflictWithRangeIsRefused(t *testing.T) {
	t.Parallel()
	_, _, err := runShortcutCapturingErr(t, shortcutFromRegistry(t, "+cells-unmerge"), []string{
		"--url", testURL, "--sheet-id", testSheetID,
		"--range", "A1:B2", "--ranges", `["D1:E2"]`, "--dry-run",
	})
	requireValidation(t, err, "name different spans")
}

// --sheet-id neither selects a sheet here nor validates against the payload,
// so accepting it would let a write land wherever the payload pointed.
func TestSheetIDStaysRefusedOnPayloadPerSheetCommands(t *testing.T) {
	t.Parallel()
	_, _, err := runShortcutCapturingErr(t, shortcutFromRegistry(t, "+table-put"), []string{
		"--url", testURL, "--sheet-id", "sh_target", "--dry-run",
		"--sheets", `{"sheets":[{"name":"Other","columns":["a"],"data":[["x"]]}]}`,
	})
	requireValidation(t, err, `unknown flag "--sheet-id"`)
}

// The sheet-name check runs in Validate, after the shared runner has resolved
// @file / stdin. From PreRunE it saw "@payload.json" as a literal, failed to
// parse it, and waved the call through to a sheet the caller did not name.
func TestEchoedSheetName_ChecksResolvedPayloads(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := writeTempPayload(t, dir, `{"sheets":[{"name":"Other","columns":["a"],"data":[["x"]]}]}`)
	_, _, err := runShortcutCapturingErr(t, shortcutFromRegistry(t, "+table-put"), []string{
		"--url", testURL, "--sheet-name", "Expected", "--dry-run",
		"--sheets", "@" + path,
	})
	requireValidation(t, err, "does not match any sub-sheet")
}

// The growth cap has to measure the rectangle that EXISTS after padding, not
// just the part prepended: padMatrixForStyles then grows right and down to
// every style's end, unbounded.
func TestStyleGrowth_CapCountsTheWholeBoundingRectangle(t *testing.T) {
	t.Parallel()
	_, _, err := runShortcutCapturingErr(t, shortcutFromRegistry(t, "+workbook-create"), []string{
		"--title", "T", "--dry-run",
		"--sheets", `{"sheets":[{"name":"S","start_cell":"B2","columns":["a"],"data":[["x"]]}]}`,
		"--styles", `{"styles":[{"name":"S","cell_styles":[{"range":"A1:ZZZ1000","font_weight":"bold"}]}]}`,
	})
	if err == nil {
		t.Fatal("a style spanning ~18M cells was accepted; padding would materialize every one")
	}
}

func TestStyleGrowthFits_DegenerateRectangleIsRefused(t *testing.T) {
	t.Parallel()
	// A style-only payload with no columns must not multiply out to zero and
	// pass any cap at all.
	if styleGrowthFits(0, 0, -1, 10) {
		t.Error("a zero-width rectangle passed the cap")
	}
	if styleGrowthFits(0, 0, 10, -1) {
		t.Error("a zero-height rectangle passed the cap")
	}
	if !styleGrowthFits(0, 0, 9, 9) {
		t.Error("a 10x10 rectangle was refused")
	}
}

// Matching rectangles do not prove two merges are the same: on a 3x3 range
// `all` is one cell while `rows` is three.
func TestMergeIdempotence_MergeTypeMustAgreeWithTheAxis(t *testing.T) {
	t.Parallel()
	op := func(mergeType string) interface{} {
		in := map[string]interface{}{"range": "A1:F1"}
		if mergeType != "" {
			in["merge_type"] = mergeType
		}
		return map[string]interface{}{"tool_name": "merge_cells", "input": in}
	}
	oneRow := mergeRect{singleRow: true}
	oneCol := mergeRect{singleCol: true}
	block := mergeRect{}

	for _, tc := range []struct {
		name      string
		mergeType string
		rect      mergeRect
		want      bool
	}{
		{"default on one row", "", oneRow, true},
		{"all on one row", "all", oneRow, true},
		{"rows on one row", "rows", oneRow, true},
		{"columns on one row does nothing", "columns", oneRow, false},
		{"all on one column", "all", oneCol, true},
		{"columns on one column", "columns", oneCol, true},
		{"rows on one column does nothing", "rows", oneCol, false},
		{"all on a block is not rows", "all", block, false},
		{"rows on a block", "rows", block, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := mergeOpIsRedundantFor(op(tc.mergeType), tc.rect); got != tc.want {
				t.Errorf("mergeOpIsRedundantFor(%q, %+v) = %v, want %v", tc.mergeType, tc.rect, got, tc.want)
			}
		})
	}

	// Anything that is not a merge op is never dropped.
	if mergeOpIsRedundantFor(map[string]interface{}{"tool_name": "set_cell_range"}, oneRow) {
		t.Error("a non-merge operation was classified redundant")
	}
}

// A sheet prefix belongs to the later --range rewrite; count expansion must
// not choke on it.
func TestDimDeleteCount_KeepsASheetPrefix(t *testing.T) {
	t.Parallel()
	calls := parseDryRunAPI(t, shortcutFromRegistry(t, "+dim-delete"), []string{
		"--url", testURL, "--position", "Sheet1!V", "--count", "2", "--yes",
	})
	if len(calls) == 0 {
		t.Fatal("a prefixed --position with --count was refused")
	}
	body := joinAny(calls)
	for _, want := range []string{"Sheet1", "V:W"} {
		if !strings.Contains(body, want) {
			t.Errorf("%q missing from the plan: %s", want, body)
		}
	}
}

// Title extraction runs the same loose-JSON repair the real parser does, or
// Validate rejects a payload the parser would go on to accept.
func TestWorkbookTitle_ReadsALooseJSONPayload(t *testing.T) {
	t.Parallel()
	stdout, _, err := runShortcutCapturingErr(t, shortcutFromRegistry(t, "+workbook-create"), []string{
		"--dry-run",
		// Single quotes and a trailing comma: repairLooseJSON's territory.
		"--sheets", `{'title':'季度报表','sheets':[{'name':'S','columns':['a'],'data':[['x']],}],}`,
	})
	if err != nil {
		t.Fatalf("a loose payload carrying a title was rejected: %v", err)
	}
	if !strings.Contains(stdout, "季度报表") {
		t.Errorf("title not taken from the repaired payload:\n%s", stdout)
	}
}

// A sub-op must mean what the standalone command means. --position V --count 2
// deleted V:W on its own while the batch form answered `unknown input key
// "count"`.
func TestBatchSubOp_PositionCountMatchesStandalone(t *testing.T) {
	t.Parallel()
	input := map[string]interface{}{"sheet_id": "sh1", "position": "V", "count": float64(2)}
	if err := normalizeSubOpInputKeys("+dim-delete", input); err != nil {
		t.Fatalf("the pair was refused in a sub-op: %v", err)
	}
	if got := input["range"]; got != "V:W" {
		t.Errorf("range = %v, want V:W (the span standalone produces)", got)
	}
	for _, gone := range []string{"count", "position"} {
		if _, still := input[gone]; still {
			t.Errorf("%q survived the fold: %v", gone, input)
		}
	}
}

func TestBatchSubOp_PositionCountRejectsNonsense(t *testing.T) {
	t.Parallel()
	for _, in := range []map[string]interface{}{
		{"sheet_id": "sh1", "count": float64(2)},                   // no span to count from
		{"sheet_id": "sh1", "position": "V", "count": float64(0)},  // not a count
		{"sheet_id": "sh1", "position": "!!", "count": float64(2)}, // neither row nor column
	} {
		if err := normalizeSubOpInputKeys("+dim-delete", in); err == nil {
			t.Errorf("accepted %v", in)
		}
	}
}

// Both checks that read a --sheets payload go through the same repair the
// real parser does, or they disagree with it about what the payload says.
func TestCanonicalPayloadJSON_SharedByCheckAndParser(t *testing.T) {
	t.Parallel()
	t.Run("sheet-name check sees a loose payload", func(t *testing.T) {
		t.Parallel()
		_, _, err := runShortcutCapturingErr(t, shortcutFromRegistry(t, "+table-put"), []string{
			"--url", testURL, "--sheet-name", "Expected", "--dry-run",
			"--sheets", `{'sheets':[{'name':'Other','columns':['a'],'data':[['x']]}]}`,
		})
		requireValidation(t, err, "does not match any sub-sheet")
	})

	t.Run("sheet-name check ignores sibling sections", func(t *testing.T) {
		t.Parallel()
		_, _, err := runShortcutCapturingErr(t, shortcutFromRegistry(t, "+table-put"), []string{
			"--url", testURL, "--sheet-name", "Expected", "--dry-run",
			"--sheets", `{"sheets":[{"name":"Other","columns":["a"],"data":[["x"]]}],"styles":[{"name":"Expected"}]}`,
		})
		requireValidation(t, err, "does not match any sub-sheet")
	})

	t.Run("malformed sibling cannot disable sheet-name check", func(t *testing.T) {
		t.Parallel()
		_, _, err := runShortcutCapturingErr(t, shortcutFromRegistry(t, "+table-put"), []string{
			"--url", testURL, "--sheet-name", "Expected", "--dry-run",
			"--sheets", `{"sheets":[{"name":"Other","columns":["a"],"data":[["x"]]}],"styles":"junk"}`,
		})
		requireValidation(t, err, "does not match any sub-sheet")
	})

	t.Run("sheet names compare exactly instead of trimmed", func(t *testing.T) {
		t.Parallel()
		_, _, err := runShortcutCapturingErr(t, shortcutFromRegistry(t, "+table-put"), []string{
			"--url", testURL, "--sheet-name", "Expected", "--dry-run",
			"--sheets", `{"sheets":[{"name":" Expected ","columns":["a"],"data":[["x"]]}]}`,
		})
		requireValidation(t, err, "does not match any sub-sheet")
	})

	t.Run("styles check ignores sibling sheets", func(t *testing.T) {
		t.Parallel()
		_, _, err := runShortcutCapturingErr(t, shortcutFromRegistry(t, "+styles-put"), []string{
			"--url", testURL, "--sheet-name", "Expected", "--dry-run",
			"--styles", `{"styles":[{"name":"Other","cell_styles":[{"range":"A1","font_weight":"bold"}]}],"sheets":[{"name":"Expected"}]}`,
		})
		requireValidation(t, err, "does not match any sub-sheet")
	})

	t.Run("styles check reads the same target alias as its parser", func(t *testing.T) {
		t.Parallel()
		_, _, err := runShortcutCapturingErr(t, shortcutFromRegistry(t, "+styles-put"), []string{
			"--url", testURL, "--sheet-name", "Expected", "--dry-run",
			"--styles", `{"styles":[{"sheet_name":"Other","cell_styles":[{"range":"A1","font_weight":"bold"}]}]}`,
		})
		requireValidation(t, err, "does not match any sub-sheet")
	})

	// One document on one flag has to reach all three readers — the title
	// derivation, the sheets parser and the styles section — through the same
	// repair, or a payload in the loose grammar arrives complete to some of
	// them and empty to the rest.
	t.Run("one piped document feeds title, sheets and styles", func(t *testing.T) {
		t.Parallel()
		stdout, _, err := runShortcutCapturingErrWithStdin(t, shortcutFromRegistry(t, "+workbook-create"), []string{
			"--sheets", "-", "--dry-run",
		}, `{'title':'Book','sheets':[{'name':'S','columns':['a'],'data':[['x']],}],'styles':[{'name':'S','cell_styles':[{'range':'A1','font_weight':'bold'}],}],}`)
		if err != nil {
			t.Fatalf("canonicalizable payload was refused: %v", err)
		}
		if !strings.Contains(stdout, "Book") || !strings.Contains(stdout, "font_weight") {
			t.Errorf("payload lost its title or styles:\n%s", stdout)
		}
	})

	t.Run("directly mounted shortcut reads the same one document", func(t *testing.T) {
		t.Parallel()
		stdout, _, err := runShortcutCapturingErrWithStdin(t, WorkbookCreate, []string{
			"--title", "Book", "--sheets", "-", "--dry-run",
		}, `{'sheets':[{'name':'S','columns':['a'],'data':[['x']],}],'styles':[{'name':'S','cell_styles':[{'range':'A1','font_weight':'bold'}],}],}`)
		if err != nil {
			t.Fatalf("directly mounted Sheets shortcut refused the payload: %v", err)
		}
		if !strings.Contains(stdout, "Book") || !strings.Contains(stdout, "font_weight") {
			t.Errorf("directly mounted shortcut lost payload fields:\n%s", stdout)
		}
	})

	t.Run("title derivation sees a mistyped closer", func(t *testing.T) {
		t.Parallel()
		stdout, _, err := runShortcutCapturingErr(t, shortcutFromRegistry(t, "+workbook-create"), []string{
			"--dry-run",
			"--sheets", `{"title":"Book","sheets":[{"name":"S","columns":["a"],"data":[["x"]]}]]`,
		})
		if err != nil {
			t.Fatalf("a repairable payload was refused for a title it carries: %v", err)
		}
		if !strings.Contains(stdout, "Book") {
			t.Errorf("title not taken from the repaired payload:\n%s", stdout)
		}
	})
}

// The per-sheet growth cap and the payload-wide cell budget must measure the
// same rectangle: six sheets each growing 200,000 cells upward passed the
// per-sheet cap and sailed past a 1M budget.
func TestStyleGrowth_CountsIntoTheGlobalCellBudget(t *testing.T) {
	t.Parallel()
	sheets := make([]string, 0, 6)
	styles := make([]string, 0, 6)
	for i := 0; i < 6; i++ {
		sheets = append(sheets, `{"name":"S`+string(rune('0'+i))+`","start_cell":"A199999","columns":["a"],"data":[["x"]]}`)
		styles = append(styles, `{"name":"S`+string(rune('0'+i))+`","cell_styles":[{"range":"A1","font_weight":"bold"}]}`)
	}
	_, _, err := runShortcutCapturingErr(t, shortcutFromRegistry(t, "+workbook-create"), []string{
		"--title", "T", "--dry-run",
		"--sheets", `{"sheets":[` + strings.Join(sheets, ",") + `]}`,
		"--styles", `{"styles":[` + strings.Join(styles, ",") + `]}`,
	})
	requireValidation(t, err, "over the 1000000-cell safety cap")
}

// headerRowCount, not an unconditional +1: a header:false payload sitting
// exactly on the cap is legitimate and must not be refused.
func TestStyleGrowth_HeaderFalseBoundaryIsAccepted(t *testing.T) {
	t.Parallel()
	_, _, err := runShortcutCapturingErr(t, shortcutFromRegistry(t, "+workbook-create"), []string{
		"--title", "T", "--dry-run",
		"--sheets", `{"sheets":[{"name":"S","header":false,"start_cell":"A200000","columns":["a"],"data":[["x"]]}]}`,
		"--styles", `{"styles":[{"name":"S","cell_styles":[{"range":"A1","font_weight":"bold"}]}]}`,
	})
	if err != nil {
		t.Fatalf("a header:false payload exactly on the cap was refused: %v", err)
	}
}

func TestStyleGrowth_RejectsDownwardOnlyAndOverflowingRectangles(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, styleRange, want string
	}{
		{"downward only", "A1:A200001", "over the 200000-cell safety cap"},
		// The payload-wide guard runs before the per-sheet guard; either is safe,
		// but the checked multiplication must not wrap and admit the payload.
		{"overflowing product", "A1:B9223372036854775807", "over the 1000000-cell safety cap"},
		// This column is MaxUint64+1 in base-26; without checked A1 decoding it
		// wraps to A and turns an enormous range into a one-cell range.
		{"overflowing column", "A1:GKGWBYLWRXTLPQ1", "unsupported range form"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, _, err := runShortcutCapturingErr(t, shortcutFromRegistry(t, "+workbook-create"), []string{
				"--title", "T", "--dry-run",
				"--sheets", `{"sheets":[{"name":"S","header":false,"columns":["a"],"data":[["x"]]}]}`,
				"--styles", `{"styles":[{"name":"S","cell_styles":[{"range":"` + tc.styleRange + `","font_weight":"bold"}]}]}`,
			})
			requireValidation(t, err, tc.want)
		})
	}
}

func TestStyleGrowth_EmptyMatrixStillCountsAnchorGap(t *testing.T) {
	t.Parallel()
	_, _, err := runShortcutCapturingErr(t, shortcutFromRegistry(t, "+workbook-create"), []string{
		"--title", "T", "--dry-run",
		"--sheets", `{"sheets":[{"name":"S","header":false,"columns":["a"],"data":[]}]}`,
		"--styles", `{"styles":[{"name":"S","cell_styles":[{"range":"A1000000000","font_weight":"bold"}]}]}`,
	})
	requireValidation(t, err, "over the 1000000-cell safety cap")
}

func TestTablePayloadRejectsEndpointCoordinateOverflow(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, startCell, columns, data string
	}{
		{"column endpoint", "CRPXNLSKVLJFHG1", `["a","b"]`, `[]`},
		{"row endpoint", "A9223372036854775807", `["a"]`, `[["x"]]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, _, err := runShortcutCapturingErr(t, shortcutFromRegistry(t, "+workbook-create"), []string{
				"--title", "T", "--dry-run",
				"--sheets", `{"sheets":[{"name":"S","start_cell":"` + tc.startCell + `","columns":` + tc.columns + `,"data":` + tc.data + `}]}`,
			})
			requireValidation(t, err, "exceeds the supported A1 coordinate range")
		})
	}
}

func TestResolvedAppendBudgetUsesLiveBaseRows(t *testing.T) {
	t.Parallel()
	payload := &tablePayload{Sheets: make([]tableSheetSpec, 6)}
	styles := &workbookCreateSheetStyles{ByIndex: make([]*workbookCreateStylePayload, 6)}
	placements := make([]tableWritePlacement, 6)
	for i := range payload.Sheets {
		payload.Sheets[i] = tableSheetSpec{Name: fmt.Sprintf("S%d", i), Mode: "append",
			Columns: []tableColumnSpec{{Name: "a"}}, Rows: [][]interface{}{{"x"}}}
		styles.ByIndex[i] = &workbookCreateStylePayload{CellStyles: []workbookCreateCellStyleOp{{
			Range: "A1", Style: map[string]interface{}{"font_weight": "bold"},
		}}}
		placements[i] = tableWritePlacement{baseCol: 0, baseRow: 199998}
	}
	requireValidation(t, checkResolvedTableWriteBudget(payload, styles, placements), "over the 1000000-cell safety cap")
}

func TestTablePut_RefreshesLaterAppendPlacementBeforeWrite(t *testing.T) {
	t.Parallel()
	structure := toolOutputStub(testToken, "read", `{"sheets":[`+
		`{"sheet_id":"sh1","sheet_name":"S1","row_count":200,"column_count":20},`+
		`{"sheet_id":"sh2","sheet_name":"S2","row_count":200,"column_count":20}]}`)
	preflightS1 := toolOutputStub(testToken, "read", `{"current_region":"A1:A2"}`)
	preflightS2 := toolOutputStub(testToken, "read", `{"current_region":"A1:A2"}`)
	refreshS1 := toolOutputStub(testToken, "read", `{"current_region":"A1:A2"}`)
	writeS1 := toolOutputStub(testToken, "write", `{"ok":true}`)
	// A concurrent append appears after preflight and before S2's turn.
	refreshS2 := toolOutputStub(testToken, "read", `{"current_region":"A1:A3"}`)
	writeS2 := toolOutputStub(testToken, "write", `{"ok":true}`)

	out, err := runShortcutWithStubs(t, TablePut, []string{
		"--url", testURL,
		"--sheets", `{"sheets":[` +
			`{"name":"S1","mode":"append","header":false,"columns":["a"],"data":[["x"]]},` +
			`{"name":"S2","mode":"append","header":false,"columns":["a"],"data":[["y"]]}]}`,
	}, structure, preflightS1, preflightS2, refreshS1, writeS1, refreshS2, writeS2)
	if err != nil {
		t.Fatalf("execute failed: %v\nout=%s", err, out)
	}
	var wire struct {
		Input string `json:"input"`
	}
	if err := json.Unmarshal(writeS2.CapturedBody, &wire); err != nil {
		t.Fatalf("decode captured S2 request: %v", err)
	}
	var input map[string]interface{}
	if err := json.Unmarshal([]byte(wire.Input), &input); err != nil {
		t.Fatalf("decode captured S2 tool input: %v", err)
	}
	if got := input["range"]; got != "A4:A4" {
		t.Errorf("S2 append range = %v, want A4:A4 from refreshed placement", got)
	}
}

func TestTablePut_AppendDefaultsToNoOverwrite(t *testing.T) {
	t.Parallel()
	structure := toolOutputStub(testToken, "read", `{"sheets":[{"sheet_id":"sh1","sheet_name":"S","row_count":200,"column_count":20}]}`)
	region := toolOutputStub(testToken, "read", `{"current_region":"A1:A2"}`)
	write := toolOutputStub(testToken, "write", `{"ok":true}`)
	out, err := runShortcutWithStubs(t, TablePut, []string{
		"--url", testURL,
		"--sheets", `{"sheets":[{"name":"S","mode":"append","header":false,"columns":["a"],"data":[["x"]]}]}`,
	}, structure, region, write)
	if err != nil {
		t.Fatalf("execute failed: %v\nout=%s", err, out)
	}
	var wire struct {
		Input string `json:"input"`
	}
	if err := json.Unmarshal(write.CapturedBody, &wire); err != nil {
		t.Fatalf("decode captured write request: %v", err)
	}
	var input map[string]interface{}
	if err := json.Unmarshal([]byte(wire.Input), &input); err != nil {
		t.Fatalf("decode captured tool input: %v", err)
	}
	if input["allow_overwrite"] != false {
		t.Errorf("append allow_overwrite = %v, want false by default", input["allow_overwrite"])
	}
}

func TestTablePut_ResolvedAppendBudgetFailsBeforeAnyWrite(t *testing.T) {
	t.Parallel()
	sheetSpecs := make([]string, 0, 6)
	styleSpecs := make([]string, 0, 6)
	structureEntries := make([]string, 0, 6)
	stubs := make([]*httpmock.Stub, 0, 7)
	for i := 0; i < 6; i++ {
		name, id := fmt.Sprintf("S%d", i), fmt.Sprintf("sh%d", i)
		sheetSpecs = append(sheetSpecs, fmt.Sprintf(`{"name":%q,"mode":"append","columns":["a"],"data":[["x"]]}`, name))
		styleSpecs = append(styleSpecs, fmt.Sprintf(`{"name":%q,"cell_styles":[{"range":"A1","font_weight":"bold"}]}`, name))
		structureEntries = append(structureEntries, fmt.Sprintf(`{"sheet_id":%q,"sheet_name":%q,"row_count":200000,"column_count":20}`, id, name))
	}
	stubs = append(stubs, toolOutputStub(testToken, "read", `{"sheets":[`+strings.Join(structureEntries, ",")+`]}`))
	for range 6 {
		stubs = append(stubs, toolOutputStub(testToken, "read", `{"current_region":"A1:A199998"}`))
	}
	// No write stub is registered: reaching create/set_cell_range would fail as
	// an unmatched request instead of the budget error asserted below.
	out, err := runShortcutWithStubs(t, TablePut, []string{
		"--url", testURL,
		"--sheets", `{"sheets":[` + strings.Join(sheetSpecs, ",") + `]}`,
		"--styles", `{"styles":[` + strings.Join(styleSpecs, ",") + `]}`,
	}, stubs...)
	if err == nil {
		t.Fatalf("expected the live append budget to fail before writing: %s", out)
	}
	if !strings.Contains(err.Error(), "over the 1000000-cell safety cap") {
		t.Fatalf("unexpected error: %v\nout=%s", err, out)
	}
}

// The batch surface reads the same scalars the standalone flags accept, and
// the same sheet-qualifier spellings.
func TestBatchSubOp_PositionAcceptsNumbersAndWidePrefixes(t *testing.T) {
	t.Parallel()
	numeric := map[string]interface{}{"sheet_id": "sh1", "position": float64(3), "count": float64(2)}
	if err := normalizeSubOpInputKeys("+dim-delete", numeric); err != nil {
		t.Fatalf("a numeric position with count was refused: %v", err)
	}
	if got := numeric["range"]; got != "3:4" {
		t.Errorf("range = %v, want 3:4", got)
	}
	wide := map[string]interface{}{"position": "Sheet1！V", "count": float64(2)}
	if err := normalizeSubOpInputKeys("+dim-delete", wide); err != nil {
		t.Fatalf("a full-width sheet qualifier was refused: %v", err)
	}
	if got, _ := wide["range"].(string); !strings.HasSuffix(got, "V:W") || !strings.Contains(got, "Sheet1") {
		t.Errorf("range = %v, want the prefix kept and the span expanded", wide["range"])
	}
}
