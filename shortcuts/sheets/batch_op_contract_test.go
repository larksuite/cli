// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/larksuite/cli/shortcuts/common"
)

// TestBatchOp_BodyMatchesStandalone is the core contract: for every batchable
// shortcut, the MCP body produced inside +batch-update must be byte-for-byte
// identical to the body the same shortcut produces when invoked standalone
// (both observed via --dry-run, comparing tool_name + decoded input). This is
// what guarantees "a sub-op behaves exactly like the standalone command", and
// it is the regression guard for the whole flag→body translator reuse.
//
// Each case provides the standalone CLI args and the equivalent sub-op input
// object (same CLI flag names, minus the spreadsheet locator which the batch
// supplies at the top level).
func TestBatchOp_BodyMatchesStandalone(t *testing.T) {
	t.Parallel()

	cases := []struct {
		shortcut string
		sc       common.Shortcut
		// standalone args (excluding --url, which every case shares)
		args []string
		// sub-op input object as JSON (CLI flag names; no excel_id/url)
		subInput string
	}{
		{
			shortcut: "+cells-set",
			sc:       CellsSet,
			args:     []string{"--sheet-id", "sh1", "--range", "A1:B1", "--cells", `[[{"value":"x"},{"value":"y"}]]`},
			subInput: `{"sheet-id":"sh1","range":"A1:B1","cells":[[{"value":"x"},{"value":"y"}]]}`,
		},
		{
			shortcut: "+cells-clear",
			sc:       CellsClear,
			args:     []string{"--sheet-id", "sh1", "--range", "A1:C3", "--scope", "formats"},
			subInput: `{"sheet-id":"sh1","range":"A1:C3","scope":"formats"}`,
		},
		{
			shortcut: "+cells-replace",
			sc:       CellsReplace,
			args:     []string{"--sheet-id", "sh1", "--find", "foo", "--replacement", "bar", "--match-case"},
			subInput: `{"sheet-id":"sh1","find":"foo","replacement":"bar","match-case":true}`,
		},
		{
			shortcut: "+csv-put",
			sc:       CsvPut,
			args:     []string{"--sheet-id", "sh1", "--csv", "a,b\n1,2", "--start-cell", "B2"},
			subInput: `{"sheet-id":"sh1","csv":"a,b\n1,2","start-cell":"B2"}`,
		},
		{
			shortcut: "+cells-merge",
			sc:       CellsMerge,
			args:     []string{"--sheet-id", "sh1", "--range", "A1:C1", "--merge-type", "rows"},
			subInput: `{"sheet-id":"sh1","range":"A1:C1","merge-type":"rows"}`,
		},
		{
			shortcut: "+cells-unmerge",
			sc:       CellsUnmerge,
			args:     []string{"--sheet-id", "sh1", "--range", "A1:C1"},
			subInput: `{"sheet-id":"sh1","range":"A1:C1"}`,
		},
		{
			shortcut: "+dim-insert",
			sc:       DimInsert,
			args:     []string{"--sheet-id", "sh1", "--dimension", "row", "--start", "10", "--end", "12", "--inherit-style", "before"},
			subInput: `{"sheet-id":"sh1","dimension":"row","start":10,"end":12,"inherit-style":"before"}`,
		},
		{
			shortcut: "+dim-delete",
			sc:       DimDelete,
			args:     []string{"--sheet-id", "sh1", "--dimension", "column", "--start", "2", "--end", "4"},
			subInput: `{"sheet-id":"sh1","dimension":"column","start":2,"end":4}`,
		},
		{
			shortcut: "+dim-hide",
			sc:       DimHide,
			args:     []string{"--sheet-id", "sh1", "--dimension", "row", "--start", "1", "--end", "3"},
			subInput: `{"sheet-id":"sh1","dimension":"row","start":1,"end":3}`,
		},
		{
			shortcut: "+dim-freeze",
			sc:       DimFreeze,
			args:     []string{"--sheet-id", "sh1", "--dimension", "row", "--count", "2"},
			subInput: `{"sheet-id":"sh1","dimension":"row","count":2}`,
		},
		{
			shortcut: "+dim-group",
			sc:       DimGroup,
			args:     []string{"--sheet-id", "sh1", "--dimension", "row", "--start", "1", "--end", "5", "--group-state", "fold"},
			subInput: `{"sheet-id":"sh1","dimension":"row","start":1,"end":5,"group-state":"fold"}`,
		},
		{
			shortcut: "+rows-resize",
			sc:       RowsResize,
			args:     []string{"--sheet-id", "sh1", "--start", "0", "--end", "0", "--type", "pixel", "--size", "30"},
			subInput: `{"sheet-id":"sh1","start":0,"end":0,"type":"pixel","size":30}`,
		},
		{
			shortcut: "+cols-resize",
			sc:       ColsResize,
			args:     []string{"--sheet-id", "sh1", "--start", "1", "--end", "3", "--type", "standard"},
			subInput: `{"sheet-id":"sh1","start":1,"end":3,"type":"standard"}`,
		},
		{
			shortcut: "+range-move",
			sc:       RangeMove,
			args:     []string{"--sheet-id", "sh1", "--source-range", "A1:C5", "--target-range", "D1"},
			subInput: `{"sheet-id":"sh1","source-range":"A1:C5","target-range":"D1"}`,
		},
		{
			shortcut: "+range-copy",
			sc:       RangeCopy,
			args:     []string{"--sheet-id", "sh1", "--source-range", "A1:B2", "--target-range", "A10", "--paste-type", "values"},
			subInput: `{"sheet-id":"sh1","source-range":"A1:B2","target-range":"A10","paste-type":"values"}`,
		},
		{
			shortcut: "+range-fill",
			sc:       RangeFill,
			args:     []string{"--sheet-id", "sh1", "--source-range", "A1:A2", "--target-range", "A1:A10", "--series-type", "linear"},
			subInput: `{"sheet-id":"sh1","source-range":"A1:A2","target-range":"A1:A10","series-type":"linear"}`,
		},
		{
			shortcut: "+range-sort",
			sc:       RangeSort,
			args:     []string{"--sheet-id", "sh1", "--range", "A1:D10", "--sort-keys", `[{"col":"B","order":"asc"}]`, "--has-header"},
			subInput: `{"sheet-id":"sh1","range":"A1:D10","sort-keys":[{"col":"B","order":"asc"}],"has-header":true}`,
		},
		{
			shortcut: "+sheet-create",
			sc:       SheetCreate,
			args:     []string{"--title", "New", "--index", "2"},
			subInput: `{"title":"New","index":2}`,
		},
		{
			shortcut: "+sheet-delete",
			sc:       SheetDelete,
			args:     []string{"--sheet-id", "sh1"},
			subInput: `{"sheet-id":"sh1"}`,
		},
		{
			shortcut: "+sheet-rename",
			sc:       SheetRename,
			args:     []string{"--sheet-id", "sh1", "--title", "Renamed"},
			subInput: `{"sheet-id":"sh1","title":"Renamed"}`,
		},
		{
			shortcut: "+sheet-copy",
			sc:       SheetCopy,
			args:     []string{"--sheet-id", "sh1", "--title", "Copy"},
			subInput: `{"sheet-id":"sh1","title":"Copy"}`,
		},
		{
			shortcut: "+sheet-hide",
			sc:       SheetHide,
			args:     []string{"--sheet-id", "sh1"},
			subInput: `{"sheet-id":"sh1"}`,
		},
		{
			shortcut: "+sheet-unhide",
			sc:       SheetUnhide,
			args:     []string{"--sheet-id", "sh1"},
			subInput: `{"sheet-id":"sh1"}`,
		},
		{
			shortcut: "+sheet-set-tab-color",
			sc:       SheetSetTabColor,
			args:     []string{"--sheet-id", "sh1", "--color", "#FF0000"},
			subInput: `{"sheet-id":"sh1","color":"#FF0000"}`,
		},
		{
			shortcut: "+dropdown-set",
			sc:       DropdownSet,
			args:     []string{"--sheet-id", "sh1", "--range", "A2:A4", "--options", `["x","y"]`, "--multiple"},
			subInput: `{"sheet-id":"sh1","range":"A2:A4","options":["x","y"],"multiple":true}`,
		},
		{
			shortcut: "+chart-create",
			sc:       ChartCreate,
			args:     []string{"--sheet-id", "sh1", "--properties", `{"position":{"start":"A1"}}`},
			subInput: `{"sheet-id":"sh1","properties":{"position":{"start":"A1"}}}`,
		},
		{
			shortcut: "+chart-update",
			sc:       ChartUpdate,
			args:     []string{"--sheet-id", "sh1", "--chart-id", "c1", "--properties", `{"title":"T"}`},
			subInput: `{"sheet-id":"sh1","chart-id":"c1","properties":{"title":"T"}}`,
		},
		{
			shortcut: "+chart-delete",
			sc:       ChartDelete,
			args:     []string{"--sheet-id", "sh1", "--chart-id", "c1"},
			subInput: `{"sheet-id":"sh1","chart-id":"c1"}`,
		},
		{
			shortcut: "+pivot-create",
			sc:       PivotCreate,
			args:     []string{"--sheet-id", "sh1", "--properties", `{"rows":[]}`, "--source", "Sheet1!A1:D100"},
			subInput: `{"sheet-id":"sh1","properties":{"rows":[]},"source":"Sheet1!A1:D100"}`,
		},
		{
			shortcut: "+cond-format-create",
			sc:       CondFormatCreate,
			args:     []string{"--sheet-id", "sh1", "--properties", `{"style":{}}`, "--rule-type", "duplicateValues", "--ranges", `["A1:A100"]`},
			subInput: `{"sheet-id":"sh1","properties":{"style":{}},"rule-type":"duplicateValues","ranges":["A1:A100"]}`,
		},
		{
			shortcut: "+filter-create",
			sc:       FilterCreate,
			args:     []string{"--sheet-id", "sh1", "--range", "A1:F1000", "--properties", `{"rules":[]}`},
			subInput: `{"sheet-id":"sh1","range":"A1:F1000","properties":{"rules":[]}}`,
		},
		{
			shortcut: "+filter-update",
			sc:       FilterUpdate,
			args:     []string{"--sheet-id", "sh1", "--range", "A1:F1000", "--properties", `{"rules":[]}`},
			subInput: `{"sheet-id":"sh1","range":"A1:F1000","properties":{"rules":[]}}`,
		},
		{
			shortcut: "+filter-delete",
			sc:       FilterDelete,
			args:     []string{"--sheet-id", "sh1"},
			subInput: `{"sheet-id":"sh1"}`,
		},
		{
			shortcut: "+filter-view-create",
			sc:       FilterViewCreate,
			args:     []string{"--sheet-id", "sh1", "--range", "A1:Z100", "--view-name", "v1", "--properties", `{"rules":[]}`},
			subInput: `{"sheet-id":"sh1","range":"A1:Z100","view-name":"v1","properties":{"rules":[]}}`,
		},
		{
			shortcut: "+sparkline-create",
			sc:       SparklineCreate,
			args:     []string{"--sheet-id", "sh1", "--properties", `{"type":"line","data_range":"A2:F2","target_range":"G2"}`},
			subInput: `{"sheet-id":"sh1","properties":{"type":"line","data_range":"A2:F2","target_range":"G2"}}`,
		},
		{
			shortcut: "+sparkline-delete",
			sc:       SparklineDelete,
			args:     []string{"--sheet-id", "sh1", "--group-id", "g1"},
			subInput: `{"sheet-id":"sh1","group-id":"g1"}`,
		},
		{
			shortcut: "+float-image-create",
			sc:       FloatImageCreate,
			args:     []string{"--sheet-id", "sh1", "--image-name", "logo.png", "--image-token", "tok", "--position-row", "0", "--position-col", "A", "--size-width", "100", "--size-height", "50"},
			subInput: `{"sheet-id":"sh1","image-name":"logo.png","image-token":"tok","position-row":0,"position-col":"A","size-width":100,"size-height":50}`,
		},
		{
			shortcut: "+float-image-delete",
			sc:       FloatImageDelete,
			args:     []string{"--sheet-id", "sh1", "--float-image-id", "fi1"},
			subInput: `{"sheet-id":"sh1","float-image-id":"fi1"}`,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.shortcut, func(t *testing.T) {
			t.Parallel()

			mapping, ok := batchOpDispatch[tc.shortcut]
			if !ok {
				t.Fatalf("%s not in batchOpDispatch", tc.shortcut)
			}

			// Standalone body via the shortcut's own dry-run.
			standaloneBody := decodeToolInput(t, parseDryRunBody(t, tc.sc, append([]string{"--url", testURL}, tc.args...)), mapping.mcpToolName)

			// Batch body via the +batch-update translator.
			var subInput map[string]interface{}
			if err := json.Unmarshal([]byte(tc.subInput), &subInput); err != nil {
				t.Fatalf("bad subInput JSON: %v", err)
			}
			fv := newMapFlagViewForCommand(tc.shortcut, subInput)
			sid := subInput["sheet-id"]
			sname := subInput["sheet-name"]
			sidStr, _ := sid.(string)
			snameStr, _ := sname.(string)
			batchBody, err := mapping.translate(fv, testToken, sidStr, snameStr)
			if err != nil {
				t.Fatalf("batch translate failed: %v", err)
			}

			// Round-trip the batch body through JSON so number types match the
			// standalone path (which is decoded from a JSON string).
			batchBody = jsonRoundTrip(t, batchBody)

			if !reflect.DeepEqual(standaloneBody, batchBody) {
				t.Errorf("%s: batch body != standalone body\n standalone=%#v\n batch     =%#v", tc.shortcut, standaloneBody, batchBody)
			}
		})
	}
}

func jsonRoundTrip(t *testing.T, m map[string]interface{}) map[string]interface{} {
	t.Helper()
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out map[string]interface{}
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return out
}

// TestBatchOp_ErrorEquivalence is the second half of the contract: for the
// same bad input, the standalone shortcut Validate and the +batch-update
// sub-op translator must emit the same friendly CLI error. Previously a
// sub-op that omitted --sheet-id (or another required flag) slipped through
// to the server and surfaced as "sheet undefined not found"; with the
// validation pushed down into the xxxInput builders both paths now stop the
// request before the API call.
//
// Scope: this test covers checks that cobra cannot enforce — XOR pairs
// (sheet selector, image token/uri), range relationships, enum-bound rules,
// pixel/size cross-flag coupling. cobra's own MarkFlagRequired catches the
// single-required cases on the standalone path with its own
// "required flag(s) \"X\" not set" wording; the batch path now catches the
// same situations with our friendlier "--X is required" wording — those are
// asserted by TestBatchOp_RejectsBadSubOpInput below.
func TestBatchOp_ErrorEquivalence(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		// shortcut & standalone args. --url is supplied by the runner. Args
		// satisfy every cobra-required flag so cobra doesn't short-circuit
		// before our shared validator runs.
		shortcut common.Shortcut
		args     []string
		// matching sub-op input; reach the same failing check.
		subShortcut string
		subInput    string
		// substring expected in both errors. We assert *contains* rather than
		// equality because the batch path wraps the inner error with
		// "operations[i] (<name>): " context — the inner message must match.
		wantContains string
	}{
		{
			name:         "+cells-set missing sheet selector",
			shortcut:     CellsSet,
			args:         []string{"--range", "A1", "--cells", `[[{"value":"x"}]]`},
			subShortcut:  "+cells-set",
			subInput:     `{"range":"A1","cells":[[{"value":"x"}]]}`,
			wantContains: "specify at least one of --sheet-id or --sheet-name",
		},
		{
			name:         "+cells-set both sheet-id and sheet-name",
			shortcut:     CellsSet,
			args:         []string{"--sheet-id", "sh1", "--sheet-name", "Sheet1", "--range", "A1", "--cells", `[[{"value":"x"}]]`},
			subShortcut:  "+cells-set",
			subInput:     `{"sheet-id":"sh1","sheet-name":"Sheet1","range":"A1","cells":[[{"value":"x"}]]}`,
			wantContains: "mutually exclusive",
		},
		{
			name:         "+dim-insert missing sheet selector",
			shortcut:     DimInsert,
			args:         []string{"--dimension", "row", "--start", "0", "--end", "1"},
			subShortcut:  "+dim-insert",
			subInput:     `{"dimension":"row","start":0,"end":1}`,
			wantContains: "specify at least one of --sheet-id or --sheet-name",
		},
		{
			name:         "+dim-insert --end <= --start",
			shortcut:     DimInsert,
			args:         []string{"--sheet-id", "sh1", "--dimension", "row", "--start", "5", "--end", "3"},
			subShortcut:  "+dim-insert",
			subInput:     `{"sheet-id":"sh1","dimension":"row","start":5,"end":3}`,
			wantContains: "must be greater than --start",
		},
		{
			name:         "+rows-resize --type pixel without --size",
			shortcut:     RowsResize,
			args:         []string{"--sheet-id", "sh1", "--start", "0", "--end", "1", "--type", "pixel"},
			subShortcut:  "+rows-resize",
			subInput:     `{"sheet-id":"sh1","start":0,"end":1,"type":"pixel"}`,
			wantContains: "--type pixel requires --size",
		},
		{
			name:         "+sheet-delete missing sheet selector",
			shortcut:     SheetDelete,
			args:         []string{},
			subShortcut:  "+sheet-delete",
			subInput:     `{}`,
			wantContains: "specify at least one of --sheet-id or --sheet-name",
		},
		{
			name:         "+float-image-create both image-token and image-uri",
			shortcut:     FloatImageCreate,
			args:         []string{"--sheet-id", "sh1", "--image-name", "x.png", "--image-token", "t", "--image-uri", "u", "--position-row", "0", "--position-col", "A", "--size-width", "100", "--size-height", "50"},
			subShortcut:  "+float-image-create",
			subInput:     `{"sheet-id":"sh1","image-name":"x.png","image-token":"t","image-uri":"u","position-row":0,"position-col":"A","size-width":100,"size-height":50}`,
			wantContains: "mutually exclusive",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Standalone path: run the shortcut with --dry-run + bad args.
			// Validate runs before DryRun, so we expect it to fail there.
			_, _, standaloneErr := runShortcutCapturingErr(
				t, tc.shortcut,
				append([]string{"--url", testURL, "--dry-run"}, tc.args...),
			)
			if standaloneErr == nil {
				t.Fatalf("standalone Validate accepted bad input — expected error containing %q", tc.wantContains)
			}
			if !strings.Contains(standaloneErr.Error(), tc.wantContains) {
				t.Errorf("standalone error = %q, want substring %q", standaloneErr.Error(), tc.wantContains)
			}

			// Batch path: translate the matching sub-op. The translator wraps
			// the inner error with "operations[i] (<shortcut>): " — assert the
			// inner message survives the wrap.
			var subInput map[string]interface{}
			if err := json.Unmarshal([]byte(tc.subInput), &subInput); err != nil {
				t.Fatalf("bad subInput JSON: %v", err)
			}
			rawOp := map[string]interface{}{
				"shortcut": tc.subShortcut,
				"input":    subInput,
			}
			_, batchErr := translateBatchOp(rawOp, testToken, 0)
			if batchErr == nil {
				t.Fatalf("batch translator accepted bad input — expected error containing %q", tc.wantContains)
			}
			if !strings.Contains(batchErr.Error(), tc.wantContains) {
				t.Errorf("batch error = %q, want substring %q (operations[i] prefix is fine)", batchErr.Error(), tc.wantContains)
			}
			// And the wrap context must include the sub-op index + shortcut
			// name so error reports stay actionable in multi-op batches.
			wrapHint := "operations[0] (" + tc.subShortcut + "):"
			if !strings.Contains(batchErr.Error(), wrapHint) {
				t.Errorf("batch error %q missing context prefix %q", batchErr.Error(), wrapHint)
			}
		})
	}
}

// TestBatchOp_RejectsBadSubOpInput pins down the secondary guard: for
// inputs that cobra's MarkFlagRequired catches on the standalone path,
// the +batch-update sub-op (which has no cobra layer) must still reject
// CLI-side with its own friendly error before issuing any API call. This
// closes the original bug — a sub-op missing --sheet-id used to slip
// through and surface as "sheet undefined not found" only after a
// network round-trip.
func TestBatchOp_RejectsBadSubOpInput(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name         string
		subShortcut  string
		subInput     string
		wantContains string
	}{
		{
			"+cells-set missing --range",
			"+cells-set",
			`{"sheet-id":"sh1","cells":[[{"value":"x"}]]}`,
			"--range is required",
		},
		{
			"+dim-insert missing --dimension",
			"+dim-insert",
			`{"sheet-id":"sh1","start":0,"end":1}`,
			"--dimension is required",
		},
		{
			"+rows-resize missing --type",
			"+rows-resize",
			`{"sheet-id":"sh1","start":0,"end":0}`,
			"--type is required",
		},
		{
			"+range-copy missing --target-range",
			"+range-copy",
			`{"sheet-id":"sh1","source-range":"A1:B2"}`,
			"--target-range is required",
		},
		{
			"+sheet-rename missing --title",
			"+sheet-rename",
			`{"sheet-id":"sh1"}`,
			"--title is required",
		},
		{
			"+chart-update missing --chart-id",
			"+chart-update",
			`{"sheet-id":"sh1","properties":{"title":"T"}}`,
			"--chart-id is required",
		},
		{
			"+filter-create missing --range",
			"+filter-create",
			`{"sheet-id":"sh1"}`,
			"--range is required",
		},
		{
			"+float-image-update missing --float-image-id",
			"+float-image-update",
			`{"sheet-id":"sh1","image-name":"x.png","image-token":"t","position-row":0,"position-col":"A","size-width":100,"size-height":50}`,
			"--float-image-id is required",
		},
		// +filter-{update,delete} need sheet-id (not sheet-name) because
		// server contract: filter_id === sheet_id, and we can't resolve
		// sheet-name → sheet-id mid-batch.
		{
			"+filter-update with --sheet-name only (filter_id must equal sheet_id)",
			"+filter-update",
			`{"sheet-name":"Sheet1","range":"A1:F1000","properties":{"rules":[]}}`,
			"+filter-update requires --sheet-id",
		},
		{
			"+filter-delete with --sheet-name only (filter_id must equal sheet_id)",
			"+filter-delete",
			`{"sheet-name":"Sheet1"}`,
			"+filter-delete requires --sheet-id",
		},
		// +sparkline-update requires sparkline_id on every
		// properties.sparklines[i] (server contract). CLI surfaces this
		// with a pointer to +sparkline-list so the agent doesn't have to
		// guess the id from an opaque server-side rejection.
		{
			"+sparkline-update item missing sparkline_id",
			"+sparkline-update",
			`{"sheet-id":"sh1","group-id":"g1","properties":{"sparklines":[{"position":{"row":0,"col":"A"}}]}}`,
			"missing sparkline_id",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var subInput map[string]interface{}
			if err := json.Unmarshal([]byte(tc.subInput), &subInput); err != nil {
				t.Fatalf("bad subInput JSON: %v", err)
			}
			rawOp := map[string]interface{}{
				"shortcut": tc.subShortcut,
				"input":    subInput,
			}
			_, err := translateBatchOp(rawOp, testToken, 0)
			if err == nil {
				t.Fatalf("translator accepted bad input — expected error containing %q", tc.wantContains)
			}
			if !strings.Contains(err.Error(), tc.wantContains) {
				t.Errorf("error = %q, want substring %q", err.Error(), tc.wantContains)
			}
		})
	}
}

// TestBatchOp_DispatchCoversReportedBugs is a focused guard for the two
// originally reported failures: +range-copy and +rows-resize sub-ops must
// translate to the correct MCP body (not a near-passthrough that drops
// required fields).
func TestBatchOp_DispatchCoversReportedBugs(t *testing.T) {
	t.Parallel()

	// +range-copy → transform_range with range / destination_range (not the
	// raw source_range / target_range that used to leak through).
	body := parseDryRunBody(t, BatchUpdate, []string{
		"--url", testURL,
		"--operations", `[{"shortcut":"+range-copy","input":{"sheet-id":"sh1","source-range":"A1:B2","target-range":"A10","paste-type":"all"}}]`,
		"--yes",
	})
	ops := decodeToolInput(t, body, "batch_update")["operations"].([]interface{})
	copyIn := ops[0].(map[string]interface{})["input"].(map[string]interface{})
	if copyIn["range"] != "A1:B2" || copyIn["destination_range"] != "A10" {
		t.Errorf("+range-copy sub-op body wrong: %#v", copyIn)
	}
	if copyIn["operation"] != "copy" {
		t.Errorf("+range-copy operation = %v, want copy", copyIn["operation"])
	}

	// +rows-resize → resize_range with range + resize_height (not raw start/end).
	body = parseDryRunBody(t, BatchUpdate, []string{
		"--url", testURL,
		"--operations", `[{"shortcut":"+rows-resize","input":{"sheet-id":"sh1","start":22,"end":22,"type":"pixel","size":40}}]`,
		"--yes",
	})
	ops = decodeToolInput(t, body, "batch_update")["operations"].([]interface{})
	resizeIn := ops[0].(map[string]interface{})["input"].(map[string]interface{})
	if resizeIn["range"] != "23:23" {
		t.Errorf("+rows-resize single-row range = %v, want 23:23", resizeIn["range"])
	}
	rh, _ := resizeIn["resize_height"].(map[string]interface{})
	if rh == nil || rh["type"] != "pixel" {
		t.Errorf("+rows-resize resize_height wrong: %#v", resizeIn)
	}
}
