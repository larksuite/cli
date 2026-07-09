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
			args:     []string{"--sheet-id", "sh1", "--position", "11", "--count", "2", "--inherit-style", "before"},
			subInput: `{"sheet-id":"sh1","position":"11","count":2,"inherit-style":"before"}`,
		},
		{
			shortcut: "+dim-delete",
			sc:       DimDelete,
			args:     []string{"--sheet-id", "sh1", "--range", "C:D"},
			subInput: `{"sheet-id":"sh1","range":"C:D"}`,
		},
		{
			shortcut: "+dim-hide",
			sc:       DimHide,
			args:     []string{"--sheet-id", "sh1", "--range", "2:3"},
			subInput: `{"sheet-id":"sh1","range":"2:3"}`,
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
			args:     []string{"--sheet-id", "sh1", "--range", "2:5", "--group-state", "fold"},
			subInput: `{"sheet-id":"sh1","range":"2:5","group-state":"fold"}`,
		},
		{
			shortcut: "+rows-resize",
			sc:       RowsResize,
			args:     []string{"--sheet-id", "sh1", "--range", "1", "--type", "pixel", "--size", "30"},
			subInput: `{"sheet-id":"sh1","range":"1","type":"pixel","size":30}`,
		},
		{
			shortcut: "+cols-resize",
			sc:       ColsResize,
			args:     []string{"--sheet-id", "sh1", "--range", "B:D", "--type", "standard"},
			subInput: `{"sheet-id":"sh1","range":"B:D","type":"standard"}`,
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
			args:     []string{"--sheet-id", "sh1", "--range", "A1:D10", "--sort-keys", `[{"column":"B","ascending":true}]`, "--has-header"},
			subInput: `{"sheet-id":"sh1","range":"A1:D10","sort-keys":[{"column":"B","ascending":true}],"has-header":true}`,
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
			shortcut: "+sheet-show-gridline",
			sc:       SheetShowGridline,
			args:     []string{"--sheet-id", "sh1"},
			subInput: `{"sheet-id":"sh1"}`,
		},
		{
			shortcut: "+sheet-hide-gridline",
			sc:       SheetHideGridline,
			args:     []string{"--sheet-id", "sh1"},
			subInput: `{"sheet-id":"sh1"}`,
		},
		{
			shortcut: "+dropdown-set",
			sc:       DropdownSet,
			args:     []string{"--sheet-id", "sh1", "--range", "A2:A4", "--options", `["x","y"]`, "--multiple"},
			subInput: `{"sheet-id":"sh1","range":"A2:A4","options":["x","y"],"multiple":true}`,
		},
		{
			// --highlight=false explicitly opts out of the server's new
			// enable_highlight=true default. Covers the tri-state Changed()
			// branch in buildDropdownValidation: standalone reads the cobra
			// "Changed" bit; sub-op reads the key's presence in the map.
			shortcut: "+dropdown-set",
			sc:       DropdownSet,
			args:     []string{"--sheet-id", "sh1", "--range", "A2:A4", "--options", `["x","y"]`, "--highlight=false"},
			subInput: `{"sheet-id":"sh1","range":"A2:A4","options":["x","y"],"highlight":false}`,
		},
		{
			shortcut: "+chart-create",
			sc:       ChartCreate,
			args:     []string{"--sheet-id", "sh1", "--properties", `{"position":{"row":0,"col":"A"},"size":{"width":400,"height":300}}`},
			subInput: `{"sheet-id":"sh1","properties":{"position":{"row":0,"col":"A"},"size":{"width":400,"height":300}}}`,
		},
		{
			shortcut: "+chart-update",
			sc:       ChartUpdate,
			args:     []string{"--sheet-id", "sh1", "--chart-id", "c1", "--properties", `{"position":{"row":0,"col":"A"},"size":{"width":400,"height":300}}`},
			subInput: `{"sheet-id":"sh1","chart-id":"c1","properties":{"position":{"row":0,"col":"A"},"size":{"width":400,"height":300}}}`,
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
			// +pivot-create renamed --sheet-id / --sheet-name → --target-sheet-id /
			// --target-sheet-name to flag the placement-sheet semantics (the data
			// source is in --source). Both standalone args and the +batch-update
			// sub-op input must use the new names.
			args:     []string{"--target-sheet-id", "sh1", "--properties", `{"rows":[]}`, "--source", "Sheet1!A1:D100"},
			subInput: `{"target-sheet-id":"sh1","properties":{"rows":[]},"source":"Sheet1!A1:D100"}`,
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
			// Match what translateBatchOp does — read the sheet selector
			// via the shortcut-specific flag names so +pivot-create
			// (target-sheet-id / target-sheet-name) and the rest
			// (sheet-id / sheet-name) both resolve correctly.
			sidFlag, snameFlag := sheetSelectorFlagsForSubOp(tc.shortcut)
			sidStr, _ := subInput[sidFlag].(string)
			snameStr, _ := subInput[snameFlag].(string)
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
			args:         []string{"--position", "1", "--count", "1"},
			subShortcut:  "+dim-insert",
			subInput:     `{"position":"1","count":1}`,
			wantContains: "specify at least one of --sheet-id or --sheet-name",
		},
		{
			name:         "+dim-insert count <= 0",
			shortcut:     DimInsert,
			args:         []string{"--sheet-id", "sh1", "--position", "5", "--count", "0"},
			subShortcut:  "+dim-insert",
			subInput:     `{"sheet-id":"sh1","position":"5","count":0}`,
			wantContains: "--count must be > 0",
		},
		{
			name:         "+rows-resize --type pixel without --size",
			shortcut:     RowsResize,
			args:         []string{"--sheet-id", "sh1", "--range", "1:2", "--type", "pixel"},
			subShortcut:  "+rows-resize",
			subInput:     `{"sheet-id":"sh1","range":"1:2","type":"pixel"}`,
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
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Standalone path: run the shortcut with --dry-run + bad args.
			// Validate runs before DryRun, so we expect it to fail there.
			_, _, standaloneErr := runShortcutCapturingErr(
				t, tc.shortcut,
				append([]string{"--url", testURL, "--dry-run"}, tc.args...),
			)
			requireValidation(t, standaloneErr, tc.wantContains)

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
			batchVE := requireValidation(t, batchErr, tc.wantContains)
			// And the wrap context must include the sub-op index + shortcut
			// name so error reports stay actionable in multi-op batches.
			wrapHint := "operations[0] (" + tc.subShortcut + "):"
			if !strings.Contains(batchVE.Message, wrapHint) {
				t.Errorf("batch error %q missing context prefix %q", batchVE.Message, wrapHint)
			}
		})
	}
}

// TestBatchOp_RejectsWrongScalarType locks the type-check that closes the
// silent-coercion gap: `operations` skips parse-time schema validation, and
// mapFlagView coerces a mismatched scalar to its zero value, so a sub-op field
// whose JSON type contradicts its flag-defs type must be rejected up front
// rather than landing as 0 / false in the wrong place.
func TestBatchOp_RejectsWrongScalarType(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name         string
		subShortcut  string
		subInput     string
		wantContains string
	}{
		{
			name:         "int flag given a string",
			subShortcut:  "+sheet-move",
			subInput:     `{"sheet-id":"sh1","source-index":2,"index":"abc"}`,
			wantContains: "--index must be a number",
		},
		{
			name:         "int flag given a boolean",
			subShortcut:  "+sheet-move",
			subInput:     `{"sheet-id":"sh1","source-index":true,"index":0}`,
			wantContains: "--source-index must be a number",
		},
		{
			// Standalone cobra rejects 1.5 for an int flag at parse time;
			// mapFlagView.Int would silently truncate it to 1, so the batch
			// path must reject it too instead of executing on a floored index.
			name:         "int flag given a non-integer number",
			subShortcut:  "+sheet-move",
			subInput:     `{"sheet-id":"sh1","source-index":2,"index":1.5}`,
			wantContains: "--index must be an integer",
		},
		{
			name:         "bool flag given a string",
			subShortcut:  "+cells-set",
			subInput:     `{"sheet-id":"sh1","range":"A1","cells":[[{"value":1}]],"allow-overwrite":"true"}`,
			wantContains: "--allow-overwrite must be a boolean",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var subInput map[string]interface{}
			if err := json.Unmarshal([]byte(tc.subInput), &subInput); err != nil {
				t.Fatalf("bad subInput JSON: %v", err)
			}
			rawOp := map[string]interface{}{"shortcut": tc.subShortcut, "input": subInput}
			_, err := translateBatchOp(rawOp, testToken, 0)
			requireValidation(t, err, tc.wantContains)
		})
	}
}

// TestBatchOp_GuardsBeyondCobra locks the two batch sub-ops whose standalone
// required-flag enforcement lives OUTSIDE the shared *Input builder — so it is
// invisible to TestBatchOp_ErrorEquivalence and was missed by the refactor:
//   - +csv-put: standalone requires one-of(start-cell, range) via cobra's
//     MarkFlagsOneRequired (PostMount); a batch sub-op never runs cobra.
//   - +sheet-move: standalone requires --index (>=0) and source-index>=0 in
//     SheetMove.Validate; the batch path uses a dedicated builder.
//
// Without an explicit guard, mapFlagView's flag-default fallback silently wins
// (start-cell→"A1", index→0), so the batch sub-op diverges from the standalone
// contract instead of failing.
func TestBatchOp_GuardsBeyondCobra(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name         string
		subShortcut  string
		subInput     string
		wantContains string
	}{
		{
			name:         "+csv-put without start-cell or range",
			subShortcut:  "+csv-put",
			subInput:     `{"sheet-id":"sh1","csv":"a,b"}`,
			wantContains: "--start-cell or --range is required",
		},
		{
			name:         "+sheet-move without index",
			subShortcut:  "+sheet-move",
			subInput:     `{"sheet-id":"sh1","source-index":2}`,
			wantContains: "requires index",
		},
		{
			name:         "+sheet-move negative index",
			subShortcut:  "+sheet-move",
			subInput:     `{"sheet-id":"sh1","source-index":2,"index":-1}`,
			wantContains: "--index must be >= 0",
		},
		{
			name:         "+sheet-move negative source-index",
			subShortcut:  "+sheet-move",
			subInput:     `{"sheet-id":"sh1","source-index":-1,"index":0}`,
			wantContains: "--source-index must be >= 0",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var subInput map[string]interface{}
			if err := json.Unmarshal([]byte(tc.subInput), &subInput); err != nil {
				t.Fatalf("bad subInput JSON: %v", err)
			}
			rawOp := map[string]interface{}{"shortcut": tc.subShortcut, "input": subInput}
			_, err := translateBatchOp(rawOp, testToken, 0)
			requireValidation(t, err, tc.wantContains)
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
			"+dim-insert missing --position",
			"+dim-insert",
			`{"sheet-id":"sh1","count":1}`,
			"--position is required",
		},
		{
			"+rows-resize missing --type",
			"+rows-resize",
			`{"sheet-id":"sh1","range":"1:1"}`,
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
		// +float-image-update's core (image_name / position / size) is mandatory
		// on update too — the tool rejects without them and +float-image-list
		// can't backfill image_name. cobra gates these on the standalone path;
		// the batch sub-op must reject them here. The image source stays optional
		// (omitting it keeps the current image), so these inputs omit it.
		{
			"+float-image-update missing --image-name",
			"+float-image-update",
			`{"sheet-id":"sh1","float-image-id":"fi1","position-row":0,"position-col":"A","size-width":100,"size-height":50}`,
			"--image-name is required",
		},
		{
			"+float-image-update missing position",
			"+float-image-update",
			`{"sheet-id":"sh1","float-image-id":"fi1","image-name":"x.png","size-width":100,"size-height":50}`,
			"--position-row and --position-col are required",
		},
		{
			"+float-image-update missing size",
			"+float-image-update",
			`{"sheet-id":"sh1","float-image-id":"fi1","image-name":"x.png","position-row":0,"position-col":"A"}`,
			"--size-width and --size-height are required",
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
			requireValidation(t, err, tc.wantContains)
		})
	}
}

// TestBatchOp_SchemaValidatesSubOps confirms the schema-driven
// validator fires on +batch-update sub-operations the same way it
// fires on standalone shortcuts. mapFlagView.Command() returns the
// sub-op's shortcut name, so validateInputAgainstSchema (called at
// each input builder's tail) routes through the same (command, flag)
// lookup pipeline a standalone invocation would. This regression
// pins that wiring — without it, agents could slip past CLI-side
// schema checks by wrapping a bad input in +batch-update.
func TestBatchOp_SchemaValidatesSubOps(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name         string
		subShortcut  string
		subInput     string
		wantContains string
	}{
		// +pivot-create properties.values items enforce summarize_by
		// enum — schema rejects an out-of-enum value as a sub-op too.
		{
			"+pivot-create summarize_by out of enum",
			"+pivot-create",
			`{"sheet-id":"sh1","source":"Sheet1!A1:D100","properties":{"values":[{"field":"A","summarize_by":"BOGUS"}]}}`,
			"summarize_by",
		},
		// +chart-create properties.position.row has minimum:0 — P0
		// addition; validator must catch -1 even in the batch path.
		{
			"+chart-create position.row below minimum",
			"+chart-create",
			`{"sheet-id":"sh1","properties":{"position":{"row":-1,"col":"A"},"size":{"width":400,"height":300}}}`,
			"below minimum",
		},
		// +cells-set --cells is a 2D array of objects per the
		// upstream-fixed schema; sub-op passing an object must be
		// rejected at the schema layer (not "expected JSON array").
		{
			"+cells-set cells wrong shape",
			"+cells-set",
			`{"sheet-id":"sh1","range":"A1","cells":{"foo":"bar"}}`,
			`expected type "array"`,
		},
	}

	for _, tc := range cases {
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
			requireValidation(t, err, tc.wantContains)
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

	// +rows-resize → resize_range with range + resize_height. The CLI's single
	// "23" input must be expanded to "23:23" because resize_range rejects
	// bare single-element ranges.
	body = parseDryRunBody(t, BatchUpdate, []string{
		"--url", testURL,
		"--operations", `[{"shortcut":"+rows-resize","input":{"sheet-id":"sh1","range":"23","type":"pixel","size":40}}]`,
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

// TestBatchOp_RequiredFlagParity is the systematic standalone-vs-batch parity
// contract: for EVERY batchable shortcut, a +batch-update sub-op that satisfies
// the sheet locator but omits all of the shortcut's business-required flags must
// fail in translateBatchOp — never silently fall back to a default. The earlier
// cases (TestBatchOp_ErrorEquivalence / GuardsBeyondCobra) cover hand-picked
// shortcuts; this one is data-driven over batchOpDispatch + flag-defs, so it
// guards the whole surface and auto-covers any shortcut added later. If a future
// refactor moves a required check out of the shared *Input builder (the exact
// failure mode behind the csv-put / sheet-move gaps), the corresponding sub-op
// would start accepting missing args and this test fails.
func TestBatchOp_RequiredFlagParity(t *testing.T) {
	t.Parallel()
	defs, err := loadFlagDefs()
	if err != nil {
		t.Fatalf("loadFlagDefs: %v", err)
	}
	// Flags supplied by the +batch-update top level (url/token), or that form the
	// sub-op's own sheet selector, are context — not "business" inputs.
	locator := map[string]bool{
		"url": true, "spreadsheet-token": true,
		"sheet-id": true, "sheet-name": true,
		"target-sheet-id": true, "target-sheet-name": true,
	}
	// How each command expresses its sheet locator in a sub-op, so the error we
	// trigger is the business one, not a missing-locator error.
	sheetSel := func(cmd string) map[string]interface{} {
		switch cmd {
		case "+sheet-create": // create needs no existing-sheet anchor
			return map[string]interface{}{}
		case "+pivot-create": // placement selector is target-sheet-*; data source is --source
			return map[string]interface{}{"target-sheet-id": "sh1"}
		default:
			return map[string]interface{}{"sheet-id": "sh1"}
		}
	}
	for cmd := range batchOpDispatch {
		spec, ok := defs[cmd]
		if !ok {
			t.Errorf("%s is in batchOpDispatch but has no flag-defs entry", cmd)
			continue
		}
		var business []string
		for _, fl := range spec.Flags {
			if fl.Kind == "system" || locator[fl.Name] {
				continue
			}
			if fl.Required == "required" || fl.Required == "xor" {
				business = append(business, fl.Name)
			}
		}
		if len(business) == 0 {
			continue // only-locator commands (sheet-delete/hide/unhide/copy/filter-delete): nothing to omit
		}
		t.Run(cmd, func(t *testing.T) {
			t.Parallel()
			rawOp := map[string]interface{}{"shortcut": cmd, "input": sheetSel(cmd)}
			_, err := translateBatchOp(rawOp, testToken, 0)
			if err == nil {
				t.Errorf("%s: a sub-op omitting business-required %v was accepted; want an error "+
					"(batch must reject missing required flags, not silently default)", cmd, business)
				return
			}
			// The sub-op DID supply a sheet selector, so a missing-locator error
			// would mean the fixture is wrong and the business-required check never
			// actually ran — reject that shape so the parity check stays honest.
			if strings.Contains(err.Error(), "specify at least one of") {
				t.Errorf("%s: got a missing-locator error, not a business-required one (fixture bug): %v", cmd, err)
			}
		})
	}
}
