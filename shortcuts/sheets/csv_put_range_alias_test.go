// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"testing"
)

// +csv-put locates with --start-cell, while +csv-get / +cells-set locate with
// --range. Agents routinely carry --range over to +csv-put and hit a guaranteed
// first-try failure. csvPutInput now accepts --range as an alias for
// --start-cell; a range value collapses to its top-left cell.
func TestCsvPutInput_RangeAliasForStartCell(t *testing.T) {
	tests := []struct {
		name       string
		raw        map[string]interface{}
		wantAnchor string
	}{
		{"start-cell direct (unchanged)", map[string]interface{}{"csv": "a,b", "start-cell": "B2"}, "B2"},
		{"range alias, single cell", map[string]interface{}{"csv": "a,b", "range": "B2"}, "B2"},
		{"range alias collapses to top-left", map[string]interface{}{"csv": "a,b", "range": "A1:H17"}, "A1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fv := newMapFlagViewForCommand("+csv-put", tt.raw)
			input, err := csvPutInput(fv, "tok", "sid", "")
			if err != nil {
				t.Fatalf("csvPutInput returned error: %v", err)
			}
			got, _ := input["start_cell"].(string)
			if got != tt.wantAnchor {
				t.Errorf("start_cell = %q, want %q", got, tt.wantAnchor)
			}
		})
	}
}

func TestCsvPutInput_RejectsStartCellAndRangeTogether(t *testing.T) {
	fv := newMapFlagViewForCommand("+csv-put", map[string]interface{}{
		"csv":        "a,b",
		"start-cell": "C3",
		"range":      "A1:H17",
	})
	_, err := csvPutInput(fv, "tok", "sid", "")
	requireValidation(t, err, "--start-cell and --range are mutually exclusive")
}

// With neither --start-cell nor --range explicitly set, csvPutInput rejects the
// call instead of silently anchoring at the "A1" flag default. Standalone never
// reaches this path — cobra's MarkFlagsOneRequired(start-cell, range) catches it
// first — but a +batch-update sub-op skips cobra, so the guard must live in the
// shared builder too. Otherwise a batch +csv-put with no anchor silently pastes
// at A1, diverging from the standalone contract.
func TestCsvPutInput_RequiresStartCellOrRange(t *testing.T) {
	fv := newMapFlagViewForCommand("+csv-put", map[string]interface{}{"csv": "a,b"})
	_, err := csvPutInput(fv, "tok", "sid", "")
	requireValidation(t, err, "--start-cell or --range is required")
}

// csvPutWriteRangeFromInput surfaces the real paste footprint so agents can see
// how far a CSV reaches from its anchor — it auto-expands to the CSV's own size,
// not to any user-set range.
func TestCsvPutWriteRangeFromInput(t *testing.T) {
	tests := []struct {
		name  string
		input map[string]interface{}
		want  string
		ok    bool
	}{
		{"3x3 at B2", map[string]interface{}{"start_cell": "B2", "csv": "a,b,c\n1,2,3\n4,5,6"}, "B2:D4", true},
		{"single cell at A1", map[string]interface{}{"start_cell": "A1", "csv": "x"}, "A1:A1", true},
		{"1 row 3 cols at C3", map[string]interface{}{"start_cell": "C3", "csv": "a,b,c"}, "C3:E3", true},
		{"ragged rows use max width", map[string]interface{}{"start_cell": "A1", "csv": "a,b\nc,d,e"}, "A1:C2", true},
		{"missing csv", map[string]interface{}{"start_cell": "A1"}, "", false},
		{"non-single anchor", map[string]interface{}{"start_cell": "A1:B2", "csv": "x"}, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := csvPutWriteRangeFromInput(tt.input)
			if ok != tt.ok || got != tt.want {
				t.Errorf("got (%q, %v), want (%q, %v)", got, ok, tt.want, tt.ok)
			}
		})
	}
}
