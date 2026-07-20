// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"encoding/json"
	"runtime"
	"strings"
	"testing"
)

// These benchmarks back the memory review of the sheets fan-out paths. They
// measure the cell matrices materialized by range-based shortcuts and table IO.
//
//   1. fillCellsMatrix — fan-out shortcuts (+cells-set-style, +dropdown-set,
//      +cells-batch-set-style, +dropdown-update) expand one A1 range into a
//      rows×cols matrix of per-cell maps. A tiny input string ("A1:Z100000")
//      explodes into millions of heap maps with no upper bound.
// Run: go test ./shortcuts/sheets -run XXX -bench 'FillCellsMatrix|BuildSheetMatrix' -benchmem

var styleProto = map[string]interface{}{
	"cell_styles":   map[string]interface{}{"bold": true, "fg_color": "#FF0000"},
	"border_styles": map[string]interface{}{"top": map[string]interface{}{"style": "solid"}},
}

func benchFillCellsMatrix(b *testing.B, rows, cols int) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m := fillCellsMatrix(rows, cols, styleProto)
		if len(m) != rows {
			b.Fatalf("bad matrix")
		}
	}
}

func BenchmarkFillCellsMatrix_100(b *testing.B)   { benchFillCellsMatrix(b, 10, 10) }     // A1:J10
func BenchmarkFillCellsMatrix_10K(b *testing.B)   { benchFillCellsMatrix(b, 1000, 10) }   // A1:J1000
func BenchmarkFillCellsMatrix_100K(b *testing.B)  { benchFillCellsMatrix(b, 10000, 10) }  // A1:J10000
func BenchmarkFillCellsMatrix_2600K(b *testing.B) { benchFillCellsMatrix(b, 100000, 26) } // A1:Z100000

// TestFanoutMatrixPeakMemory reports the concrete resident-heap delta of
// materializing a large fan-out matrix, so the review doc can quote real MB.
// Not an assertion — it prints numbers under `go test -v -run PeakMemory`.
func TestFanoutMatrixPeakMemory(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping memory probe in -short")
	}
	cases := []struct {
		name       string
		rows, cols int
	}{
		{"A1:Z10000 (260K cells)", 10000, 26},
		{"A1:Z100000 (2.6M cells)", 100000, 26},
	}
	for _, c := range cases {
		var before, after runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&before)
		m := fillCellsMatrix(c.rows, c.cols, styleProto)
		runtime.ReadMemStats(&after)
		runtime.KeepAlive(m)
		t.Logf("%-26s heap +%6.1f MB  (%d total allocs)",
			c.name,
			float64(after.HeapAlloc-before.HeapAlloc)/(1024*1024),
			after.Mallocs-before.Mallocs)
	}
}

// --- +table-put / +workbook-create matrix materialization (sibling #1 path) ---
//
// buildSheetMatrix turns the caller's --sheets/--values into a rows×cols matrix
// of per-cell maps, the same unbounded blow-up as fillCellsMatrix but on the
// table-put ingress (tablePutMaxCellsPerWrite only slices the *write*, not this
// in-memory build). checkCellBudget rejects oversized payloads before this runs.

func makeTypelessSpec(rows, cols int) *tableSheetSpec {
	c := make([]tableColumnSpec, cols)
	r := make([][]interface{}, rows)
	for i := range r {
		row := make([]interface{}, cols)
		for j := range row {
			row[j] = "x"
		}
		r[i] = row
	}
	return &tableSheetSpec{Columns: c, Rows: r}
}

func benchBuildSheetMatrix(b *testing.B, rows, cols int) {
	spec := makeTypelessSpec(rows, cols)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m, err := buildSheetMatrix(spec, true)
		if err != nil || len(m) != rows+1 {
			b.Fatalf("bad matrix")
		}
	}
}

func BenchmarkBuildSheetMatrix_100K(b *testing.B)  { benchBuildSheetMatrix(b, 10000, 10) }  // 100K cells
func BenchmarkBuildSheetMatrix_2600K(b *testing.B) { benchBuildSheetMatrix(b, 100000, 26) } // 2.6M cells

// TestTablePutMatrixPeakMemory reports the resident-heap delta of materializing
// a large table-put matrix (the cost checkCellBudget now prevents), so the
// review doc can quote real MB. Not an assertion — prints under -v -run PeakMemory.
func TestTablePutMatrixPeakMemory(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping memory probe in -short")
	}
	for _, c := range []struct {
		name       string
		rows, cols int
	}{
		{"100000×26 (2.6M cells)", 100000, 26},
	} {
		spec := makeTypelessSpec(c.rows, c.cols)
		var before, after runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&before)
		m, _ := buildSheetMatrix(spec, true)
		runtime.ReadMemStats(&after)
		runtime.KeepAlive(m)
		t.Logf("%-24s buildSheetMatrix heap +%6.1f MB  (%d total allocs)",
			c.name,
			float64(after.HeapAlloc-before.HeapAlloc)/(1024*1024),
			after.Mallocs-before.Mallocs)
	}
}

// --- fan-out cell-budget cap (fix for the unbounded matrix blow-up) ---

func TestStampMatrixBudgetCap(t *testing.T) {
	// 199992 cells (7692×26) sits just under the 200000 cap → allowed.
	if err := checkStampMatrixBudget("range", "A1:Z7692", 7692, 26); err != nil {
		t.Fatalf("199992 cells should pass, got: %v", err)
	}
	// Exactly at the cap → allowed.
	if err := checkStampMatrixBudget("range", "A1:A200000", 200000, 1); err != nil {
		t.Fatalf("200000 cells (== cap) should pass, got: %v", err)
	}
	// Just over the cap → rejected.
	if err := checkStampMatrixBudget("range", "A1:A200001", 200001, 1); err == nil {
		t.Fatal("200001 cells should be rejected")
	}
	// The pathological case from the review (2.6M cells) → rejected.
	if err := checkStampMatrixBudget("ranges", "Sheet1!A1:Z100000", 100000, 26); err == nil {
		t.Fatal("2.6M-cell fan-out should be rejected")
	}
}

// --- sibling cap gaps: +table-put/+workbook-create payload, batch aggregate,
//     batch-update operation count (follow-up to the single fan-out cap) ---

// TestTablePutCellBudgetCap covers the --sheets/--values materialization cap:
// buildSheetMatrix builds the whole matrix in memory, so the total cell count is
// bounded before that allocation, summed across all sheets.
func TestTablePutCellBudgetCap(t *testing.T) {
	// 1000×1000 = 1,000,000 == cap → allowed.
	atCap := &tablePayload{Sheets: []tableSheetSpec{{
		Columns: make([]tableColumnSpec, 1000),
		Rows:    make([][]interface{}, 1000),
	}}}
	if err := atCap.checkCellBudget(); err != nil {
		t.Fatalf("1,000,000 cells (== cap) should pass, got: %v", err)
	}
	// 1000×1001 = 1,001,000 > cap → rejected.
	over := &tablePayload{Sheets: []tableSheetSpec{{
		Columns: make([]tableColumnSpec, 1000),
		Rows:    make([][]interface{}, 1001),
	}}}
	if err := over.checkCellBudget(); err == nil {
		t.Fatal("1,001,000 cells should be rejected")
	}
	// Budget is summed across sheets, not per-sheet: 600k + 600k = 1.2M > cap.
	twoSheets := &tablePayload{Sheets: []tableSheetSpec{
		{Columns: make([]tableColumnSpec, 1000), Rows: make([][]interface{}, 600)},
		{Columns: make([]tableColumnSpec, 1000), Rows: make([][]interface{}, 600)},
	}}
	if err := twoSheets.checkCellBudget(); err == nil {
		t.Fatal("1.2M cells across two sheets should be rejected")
	}
}

func TestTablePutCellBudgetIncludesStylePadding(t *testing.T) {
	payload := &tablePayload{Sheets: []tableSheetSpec{{
		Columns: make([]tableColumnSpec, 1),
		Rows:    make([][]interface{}, 1),
	}}}
	styles := &workbookCreateSheetStyles{ByIndex: []*workbookCreateStylePayload{{
		CellStyles: []workbookCreateCellStyleOp{{Range: "A1:AX25000"}},
	}}}
	if err := payload.checkCellBudgetWithStyles(styles); err == nil {
		t.Fatal("1x1 data padded by styles to 1.25M cells should be rejected before allocation")
	}

	twoSheets := &tablePayload{Sheets: []tableSheetSpec{
		{Columns: make([]tableColumnSpec, 1), Rows: make([][]interface{}, 1)},
		{Columns: make([]tableColumnSpec, 1), Rows: make([][]interface{}, 1)},
	}}
	style := &workbookCreateStylePayload{CellStyles: []workbookCreateCellStyleOp{{Range: "A1:Z20000"}}}
	if err := twoSheets.checkCellBudgetWithStyles(&workbookCreateSheetStyles{ByIndex: []*workbookCreateStylePayload{style, style}}); err == nil {
		t.Fatal("style-padded cells should be summed across sheets")
	}
}

// TestBatchStampAggregateCap covers the batch fan-out aggregate budget — the
// per-range cap can't stop many ranges from summing past the matrix ceiling.
func TestBatchStampAggregateCap(t *testing.T) {
	if err := checkBatchStampBudget(maxStampMatrixCells); err != nil {
		t.Fatalf("aggregate == cap should pass, got: %v", err)
	}
	if err := checkBatchStampBudget(maxStampMatrixCells + 1); err == nil {
		t.Fatal("aggregate over cap should be rejected")
	}
}

// TestBatchFanoutRangeCountCap drives a fan-out shortcut with > maxBatchRanges
// ranges and expects the shared validateDropdownRanges cap to reject it.
func TestBatchFanoutRangeCountCap(t *testing.T) {
	ranges := make([]string, maxBatchRanges+1)
	for i := range ranges {
		ranges[i] = "sheet1!A1"
	}
	rangesJSON, _ := json.Marshal(ranges)
	_, _, err := runShortcutCapturingErr(t, CellsBatchSetStyle, []string{
		"--url", testURL,
		"--ranges", string(rangesJSON),
		"--font-weight", "bold",
		"--dry-run",
	})
	requireValidation(t, err, "at most")
}

// TestBatchOperationsCountCap covers the +batch-update sub-operation count cap.
func TestBatchOperationsCountCap(t *testing.T) {
	ops := make([]interface{}, maxBatchOperations+1)
	for i := range ops {
		ops[i] = map[string]interface{}{"shortcut": "+cells-set", "input": map[string]interface{}{}}
	}
	_, err := translateBatchOperations(ops, testURL)
	if err == nil || !strings.Contains(err.Error(), "at most") {
		t.Fatalf("expected operations count cap error, got: %v", err)
	}
}

// BenchmarkStampBudget_RejectsOversized is the "after" side of the fix: the same
// A1:Z100000 input that BenchmarkFillCellsMatrix_2600K shows costing ~917MB /
// 5.3M allocs is now rejected up front, allocating only the error string.
func BenchmarkStampBudget_RejectsOversized(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if err := checkStampMatrixBudget("range", "A1:Z100000", 100000, 26); err == nil {
			b.Fatal("expected rejection")
		}
	}
}
