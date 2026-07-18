// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/internal/output"
)

// ─── pure helpers: date serial, typed cell mapping ────────────────────

func TestTablePut_IsoDateToSerial(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   string
		want int
		ok   bool
	}{
		{"2024-01-15", 45306, true}, // the empirically verified anchor
		{"2024-01-01", 45292, true},
		{"2024-02-29", 45351, true}, // 2024 is a leap year
		{"1899-12-31", 1, true},     // one day after the epoch
		// pandas `df.to_json(orient="split", date_format="iso")` emits full ISO
		// datetimes (`yyyy-mm-ddTHH:MM:SS.fff[±HH:MM]`); df_to_sheet hands those
		// straight to --sheets, so the writer must round-trip them without
		// asking agents to strip the time suffix themselves.
		{"2024-01-15T00:00:00.000", 45306, true},
		{"2024-01-15T08:30:00+08:00", 45306, true},
		{"not-a-date", 0, false},
		{"2024/01/15", 0, false},  // wrong separator
		{"T2024-01-15", 0, false}, // a leading T isn't a valid prefix to strip
		{"2024-15-01", 0, false},  // invalid month/day still rejected after T-strip
	}
	for _, tt := range cases {
		got, err := isoDateToSerial(tt.in)
		if tt.ok {
			if err != nil {
				t.Errorf("isoDateToSerial(%q) unexpected error: %v", tt.in, err)
				continue
			}
			if got != tt.want {
				t.Errorf("isoDateToSerial(%q) = %d, want %d", tt.in, got, tt.want)
			}
		} else if err == nil {
			t.Errorf("isoDateToSerial(%q) = %d, want error", tt.in, got)
		}
	}
}

// TestTablePut_EmptyDatePrescription pins the empty-cell branch: the error
// must name the three ways out (drop rows / fill dates / object dtype)
// instead of the generic "must be ISO" parse failure.
func TestTablePut_EmptyDatePrescription(t *testing.T) {
	t.Parallel()
	for _, in := range []string{"", "   "} {
		_, err := isoDateToSerial(in)
		if err == nil {
			t.Fatalf("isoDateToSerial(%q) should fail", in)
		}
		for _, want := range []string{"empty cell", "object (text)"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("isoDateToSerial(%q) error should contain %q, got %q", in, want, err.Error())
			}
		}
	}
}

// TestTablePut_ColumnKeyHint pins the dtypes/formats unknown-column hint:
// A1-style letter keys get the Excel-habit callout, and the declared column
// names ride inline either way.
func TestTablePut_ColumnKeyHint(t *testing.T) {
	t.Parallel()
	cols := []string{"姓名", "出生日期"}
	got := columnKeyHint("dtypes", "A", cols)
	for _, want := range []string{"not A1-style column letters", `"姓名", "出生日期"`} {
		if !strings.Contains(got, want) {
			t.Errorf("letter-key hint should contain %q, got %q", want, got)
		}
	}
	got = columnKeyHint("formats", "出生 日期", cols)
	if strings.Contains(got, "A1-style") {
		t.Errorf("non-letter key must not get the letter callout, got %q", got)
	}
	if !strings.Contains(got, `"姓名", "出生日期"`) {
		t.Errorf("hint should inline column names, got %q", got)
	}

	many := make([]string, 20)
	for i := range many {
		many[i] = fmt.Sprintf("col%02d", i)
	}
	got = columnKeyHint("dtypes", "X", many)
	if !strings.Contains(got, ", …") || strings.Contains(got, "col19") {
		t.Errorf("hint should truncate long column lists, got %q", got)
	}
}

func TestIsColumnLetterKey(t *testing.T) {
	t.Parallel()
	cases := map[string]bool{
		"A": true, "Z": true, "AA": true, "ABC": true,
		"": false, "ABCD": false, "a": false, "A1": false, "姓名": false,
	}
	for in, want := range cases {
		if got := isColumnLetterKey(in); got != want {
			t.Errorf("isColumnLetterKey(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestTablePut_BuildTypedCell(t *testing.T) {
	t.Parallel()

	t.Run("string keeps literal + text format so digit-like ids survive read-back", func(t *testing.T) {
		t.Parallel()
		cell, err := buildTypedCell(&tableColumnSpec{Name: "id", Type: "string"}, "00123")
		if err != nil {
			t.Fatal(err)
		}
		if cell["value"] != "00123" {
			t.Errorf("value = %#v, want \"00123\"", cell["value"])
		}
		if nf := numberFormatOf(cell); nf != "@" {
			t.Errorf("number_format = %q, want @ (text format so +table-get infers string, not number)", nf)
		}
	})

	t.Run("string stringifies a json.Number without scientific notation", func(t *testing.T) {
		t.Parallel()
		cell, _ := buildTypedCell(&tableColumnSpec{Name: "code", Type: "string"}, json.Number("123456789012345"))
		if cell["value"] != "123456789012345" {
			t.Errorf("value = %#v, want literal digits", cell["value"])
		}
	})

	t.Run("number preserves json.Number", func(t *testing.T) {
		t.Parallel()
		cell, err := buildTypedCell(&tableColumnSpec{Name: "amt", Type: "number", Format: "#,##0"}, json.Number("259874"))
		if err != nil {
			t.Fatal(err)
		}
		if n, ok := cell["value"].(json.Number); !ok || n.String() != "259874" {
			t.Errorf("value = %#v, want json.Number 259874", cell["value"])
		}
		if nf := numberFormatOf(cell); nf != "#,##0" {
			t.Errorf("number_format = %q, want #,##0", nf)
		}
	})

	t.Run("date converts to serial + default format", func(t *testing.T) {
		t.Parallel()
		cell, err := buildTypedCell(&tableColumnSpec{Name: "d", Type: "date"}, "2024-01-15")
		if err != nil {
			t.Fatal(err)
		}
		if cell["value"] != 45306 {
			t.Errorf("value = %#v, want serial 45306", cell["value"])
		}
		if nf := numberFormatOf(cell); nf != "yyyy-mm-dd" {
			t.Errorf("number_format = %q, want default yyyy-mm-dd", nf)
		}
	})

	t.Run("date honors explicit format", func(t *testing.T) {
		t.Parallel()
		cell, _ := buildTypedCell(&tableColumnSpec{Name: "d", Type: "date", Format: "yyyy-mm"}, "2024-01-15")
		if nf := numberFormatOf(cell); nf != "yyyy-mm" {
			t.Errorf("number_format = %q, want yyyy-mm", nf)
		}
	})

	t.Run("bool maps to boolean", func(t *testing.T) {
		t.Parallel()
		cell, err := buildTypedCell(&tableColumnSpec{Name: "b", Type: "bool"}, true)
		if err != nil || cell["value"] != true {
			t.Errorf("value = %#v (err=%v), want true", cell["value"], err)
		}
	})

	t.Run("null is an empty cell that still carries format", func(t *testing.T) {
		t.Parallel()
		cell, err := buildTypedCell(&tableColumnSpec{Name: "d", Type: "date"}, nil)
		if err != nil {
			t.Fatal(err)
		}
		if _, has := cell["value"]; has {
			t.Errorf("null cell should have no value: %#v", cell)
		}
		if nf := numberFormatOf(cell); nf != "yyyy-mm-dd" {
			t.Errorf("null date cell should still carry format, got %q", nf)
		}
	})

	t.Run("type mismatches are rejected", func(t *testing.T) {
		t.Parallel()
		if _, err := buildTypedCell(&tableColumnSpec{Type: "number"}, "abc"); err == nil {
			t.Error("number column accepting a string should error")
		}
		if _, err := buildTypedCell(&tableColumnSpec{Type: "date"}, json.Number("1")); err == nil {
			t.Error("date column accepting a number should error")
		}
		if _, err := buildTypedCell(&tableColumnSpec{Type: "bool"}, "true"); err == nil {
			t.Error("bool column accepting a string should error")
		}
	})
}

// TestDtypeToTypeFormat pins down the pandas-dtype → internal (type, format)
// mapping that drives the writer. Pandas dtype strings come in three flavors —
// lowercase numpy (`int64`), capitalized nullable pandas (`Int64`), and the
// stringified output of tz-aware datetimes (`datetime64[ns, UTC]`) — the table
// below exercises one of each per family so a future pandas release that adds
// (say) `float128` still maps to "number" via the prefix check rather than
// silently falling through to string.
func TestDtypeToTypeFormat(t *testing.T) {
	t.Parallel()
	cases := []struct {
		dtype, wantType, wantFmt string
	}{
		// numeric: all variants → number (no default format; formats[] decides display)
		{"int8", "number", ""}, {"int16", "number", ""}, {"int32", "number", ""}, {"int64", "number", ""},
		{"uint8", "number", ""}, {"uint16", "number", ""}, {"uint32", "number", ""}, {"uint64", "number", ""},
		{"float32", "number", ""}, {"float64", "number", ""},
		{"Int8", "number", ""}, {"Int64", "number", ""}, {"UInt32", "number", ""}, {"Float64", "number", ""}, // nullable
		{"complex64", "number", ""}, {"complex128", "number", ""},
		// booleans: bool (numpy) + boolean (nullable pandas)
		{"bool", "bool", ""}, {"boolean", "bool", ""},
		// dates: every datetime* variant, incl. tz-aware
		{"datetime64[ns]", "date", "yyyy-mm-dd"},
		{"datetime64[ns, UTC]", "date", "yyyy-mm-dd"},
		{"datetime64[ns, Asia/Shanghai]", "date", "yyyy-mm-dd"},
		{"datetime64", "date", "yyyy-mm-dd"},
		// strings / unknown: object, string, category, empty, gibberish → string + @
		{"object", "string", "@"}, {"string", "string", "@"}, {"category", "string", "@"},
		{"", "string", "@"}, {"timestamp", "string", "@"}, {"bigint", "string", "@"},
	}
	for _, tc := range cases {
		t.Run(tc.dtype, func(t *testing.T) {
			t.Parallel()
			gotType, gotFmt := dtypeToTypeFormat(tc.dtype)
			if gotType != tc.wantType || gotFmt != tc.wantFmt {
				t.Errorf("dtypeToTypeFormat(%q) = (%q, %q), want (%q, %q)",
					tc.dtype, gotType, gotFmt, tc.wantType, tc.wantFmt)
			}
		})
	}
}

// TestTypeToDtype pins down the inverse mapping used by +table-get. The dtype
// string each internal type maps to must be one `df.astype(dtypes)` can
// consume without a per-column branch — that's the round-trip contract.
func TestTypeToDtype(t *testing.T) {
	t.Parallel()
	cases := []struct{ typ, want string }{
		{"string", "object"},       // pandas default, astype("object") is a no-op
		{"number", "float64"},      // works for ints, floats, and NaN-containing series
		{"date", "datetime64[ns]"}, // matches ISO yyyy-mm-dd strings we emit
		{"bool", "bool"},           // inferColumnType only picks bool when every cell is bool
		{"", "object"},             // defensive default
		{"surprise", "object"},     // ditto
	}
	for _, tc := range cases {
		if got := typeToDtype(tc.typ); got != tc.want {
			t.Errorf("typeToDtype(%q) = %q, want %q", tc.typ, got, tc.want)
		}
	}
}

// TestNormalize_DefaultsAndFormatOverride covers the wire-shape ergonomics that
// matter for one-line pandas writes:
//   - omitting `dtypes` makes every column a text-format string (so a bare
//     `df.to_json(orient="split")` payload is valid: leading-zero ids survive,
//     digits don't sneak in as numbers);
//   - `formats[col]` overrides the dtype-derived default (so a `float64` column
//     gets `#,##0.00` instead of no format);
//   - explicit `dtypes[col]` wins over the default-when-missing path.
func TestNormalize_DefaultsAndFormatOverride(t *testing.T) {
	t.Parallel()
	in := &tableSheetIn{
		Name:    "S",
		Columns: []string{"id", "amt", "d", "raw"},
		Dtypes:  map[string]string{"amt": "float64", "d": "datetime64[ns]"}, // id, raw left unspecified
		Formats: map[string]string{"amt": "#,##0.00"},                       // override float default ("")
		Data:    [][]interface{}{},
	}
	spec, err := in.normalize(0)
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	want := []tableColumnSpec{
		{Name: "id", Type: "string", Format: "@"},         // unspecified dtype → string + text format
		{Name: "amt", Type: "number", Format: "#,##0.00"}, // float64 + formats override
		{Name: "d", Type: "date", Format: "yyyy-mm-dd"},   // datetime → date + default date format
		{Name: "raw", Type: "string", Format: "@"},        // unspecified → string + text format
	}
	for i, w := range want {
		got := spec.Columns[i]
		if got != w {
			t.Errorf("columns[%d] = %+v, want %+v", i, got, w)
		}
	}
}

// numberFormatOf digs the number_format out of a built cell's cell_styles, or
// "" when absent.
func numberFormatOf(cell map[string]interface{}) string {
	styles, ok := cell["cell_styles"].(map[string]interface{})
	if !ok {
		return ""
	}
	nf, _ := styles["number_format"].(string)
	return nf
}

// ─── payload validation ───────────────────────────────────────────────

func TestTablePut_PayloadValidation(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		json string
		want string
	}{
		{"empty sheets", `{"sheets":[]}`, "at least one sheet"},
		{"missing name", `{"sheets":[{"columns":["a"],"data":[]}]}`, "name is required"},
		{"duplicate name", `{"sheets":[{"name":"S","columns":["a"],"data":[]},{"name":"S","columns":["a"],"data":[]}]}`, "duplicate sheet name"},
		{"no columns", `{"sheets":[{"name":"S","columns":[],"data":[]}]}`, "columns must be non-empty"},
		{"column missing name", `{"sheets":[{"name":"S","columns":[""],"data":[]}]}`, "columns[0] name is required"},
		{"duplicate column", `{"sheets":[{"name":"S","columns":["a","a"],"data":[]}]}`, "duplicate column name"},
		{"dtypes refs unknown column", `{"sheets":[{"name":"S","columns":["a"],"data":[],"dtypes":{"b":"int64"}}]}`, "dtypes references unknown column"},
		{"formats refs unknown column", `{"sheets":[{"name":"S","columns":["a"],"data":[],"formats":{"b":"0.0"}}]}`, "formats references unknown column"},
		{"row width mismatch", `{"sheets":[{"name":"S","columns":["a","b"],"data":[["x"]]}]}`, "column count"},
		{"bad start_cell", `{"sheets":[{"name":"S","start_cell":"A","columns":["a"],"data":[]}]}`, "start_cell"},
		{"bad date value", `{"sheets":[{"name":"S","columns":["d"],"dtypes":{"d":"datetime64[ns]"},"data":[["2025/03/31"]]}]}`, "must be ISO"},
		{"number expects numeric", `{"sheets":[{"name":"S","columns":["n"],"dtypes":{"n":"int64"},"data":[["abc"]]}]}`, "number expects"},
		{"invalid json", `{not json`, "invalid JSON"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := parseTablePutPayload(stubFlagView{"sheets": tt.json})
			requireValidation(t, err, tt.want)
		})
	}
}

// stubFlagView is a minimal flagView backed by a map, for unit-testing the
// payload parser without a cobra command.
type stubFlagView map[string]string

func (s stubFlagView) Str(name string) string        { return s[name] }
func (s stubFlagView) Bool(name string) bool         { return s[name] == "true" }
func (s stubFlagView) Int(name string) int           { return 0 }
func (s stubFlagView) Float64(name string) float64   { return 0 }
func (s stubFlagView) Changed(name string) bool      { _, ok := s[name]; return ok }
func (s stubFlagView) StrArray(name string) []string { return nil }
func (s stubFlagView) StrSlice(name string) []string { return nil }
func (s stubFlagView) Command() string               { return "+table-put" }

// ─── dry-run: create + write rendering ────────────────────────────────

const tablePutSheetsJSON = `{"sheets":[{"name":"月度",` +
	`"columns":["门店","月份","销售额"],` +
	`"dtypes":{"门店":"object","月份":"datetime64[ns]","销售额":"int64"},` +
	`"formats":{"月份":"yyyy-mm","销售额":"#,##0"},` +
	`"data":[["北京","2024-01-15",259874]]}]}`

func TestTablePut_DryRunWrite(t *testing.T) {
	t.Parallel()
	calls := parseDryRunAPI(t, TablePut, []string{"--url", testURL, "--sheets", tablePutSheetsJSON})
	if len(calls) != 1 {
		t.Fatalf("api calls = %d, want 1 (set_cell_range only)", len(calls))
	}
	body, _ := calls[0].(map[string]interface{})["body"].(map[string]interface{})
	input := decodeToolInput(t, body, "set_cell_range")
	if input["excel_id"] != testToken {
		t.Errorf("excel_id = %v, want %s", input["excel_id"], testToken)
	}
	if input["sheet_name"] != "月度" {
		t.Errorf("sheet_name = %v, want 月度", input["sheet_name"])
	}
	if input["range"] != "A1:C2" {
		t.Errorf("range = %v, want A1:C2 (1 header + 1 data row × 3 cols)", input["range"])
	}
	rows := input["cells"].([]interface{})
	header := rows[0].([]interface{})
	if hs := cellStyles(header[0]); len(hs) != 0 {
		t.Errorf("header cell should carry no style now that --header-style is removed, got %#v", header[0])
	}
	data := rows[1].([]interface{})
	// 月份 (date) → serial 45306, number_format yyyy-mm
	if v := cellValue(data[1]); v != float64(45306) {
		t.Errorf("date cell value = %#v, want 45306 serial", v)
	}
	if nf := cellStyles(data[1])["number_format"]; nf != "yyyy-mm" {
		t.Errorf("date number_format = %v, want yyyy-mm", nf)
	}
	// 销售额 (number) → 259874 preserved
	if v := cellValue(data[2]); v != float64(259874) {
		t.Errorf("number cell value = %#v, want 259874", v)
	}
}

func cellValue(c interface{}) interface{} {
	m, _ := c.(map[string]interface{})
	return m["value"]
}

func cellStyles(c interface{}) map[string]interface{} {
	m, _ := c.(map[string]interface{})
	s, _ := m["cell_styles"].(map[string]interface{})
	return s
}

// TestTablePut_DryRunWithStyles confirms --styles flows through the same shared
// path as +workbook-create: cell_styles merge into the matrix, and merges /
// resizes render as their own tool calls after the set_cell_range write.
func TestTablePut_DryRunWithStyles(t *testing.T) {
	t.Parallel()
	calls := parseDryRunAPI(t, TablePut, []string{
		"--url", testURL,
		"--sheets", `{"sheets":[{"name":"数据","columns":["a","b"],"data":[["x","y"]]}]}`,
		"--styles", `{"styles":[{"name":"数据","cell_styles":[{"range":"A1:B1","font_weight":"bold"}],"cell_merges":[{"range":"A1:B1"}],"col_sizes":[{"range":"A:A","type":"pixel","size":120}]}]}`,
	})
	if len(calls) != 3 {
		t.Fatalf("want 3 dry-run calls (set_cell_range + merge + resize), got %d", len(calls))
	}
	body, _ := calls[0].(map[string]interface{})["body"].(map[string]interface{})
	input := decodeToolInput(t, body, "set_cell_range")
	cellsRaw, _ := json.Marshal(input["cells"])
	if !strings.Contains(string(cellsRaw), `"font_weight":"bold"`) {
		t.Errorf("cell_styles should merge into the matrix; cells=%s", cellsRaw)
	}
	allRaw, _ := json.Marshal(calls)
	if !strings.Contains(string(allRaw), "merge_cells") {
		t.Errorf("dry-run should include merge_cells visual op; calls=%s", allRaw)
	}
	if !strings.Contains(string(allRaw), "resize_range") {
		t.Errorf("dry-run should include resize_range visual op; calls=%s", allRaw)
	}
}

// TestTablePut_StylesNameMismatchRejected confirms a styles item whose name
// doesn't match the payload sheet is rejected at Validate, before any write.
func TestTablePut_StylesNameMismatchRejected(t *testing.T) {
	t.Parallel()
	_, _, err := runShortcutCapturingErr(t, TablePut, []string{
		"--url", testURL,
		"--sheets", `{"sheets":[{"name":"数据","columns":["a"],"data":[["x"]]}]}`,
		"--styles", `{"styles":[{"name":"其他","cell_styles":[{"range":"A1","font_weight":"bold"}]}]}`,
		"--dry-run",
	})
	requireValidation(t, err, "must match")
}

func TestTablePut_StylePaddingBudgetRejectedBeforeDryRun(t *testing.T) {
	_, _, err := runShortcutCapturingErr(t, TablePut, []string{
		"--url", testURL,
		"--sheets", `{"sheets":[{"name":"数据","columns":["a"],"data":[["x"]]}]}`,
		"--styles", `{"styles":[{"name":"数据","cell_styles":[{"range":"A1:AX25000","font_weight":"bold"}]}]}`,
		"--dry-run",
	})
	requireValidation(t, err, "over the 1000000-cell safety cap")
}

// TestTablePut_ExecuteWithStyles drives the full write + visual-ops path: the
// set_cell_range write carries the merged cell_styles, then merge_cells /
// resize_range tool calls apply the structural styles in the same call.
func TestTablePut_ExecuteWithStyles(t *testing.T) {
	t.Parallel()
	structure := toolOutputStub(testToken, "read", `{"sheets":[{"sheet_id":"`+testSheetID+`","sheet_name":"数据","index":0}]}`)
	// set_cell_range + merge_cells + resize_range all hit the same write endpoint;
	// mark reusable so the FIFO stub serves each.
	write := toolOutputStub(testToken, "write", `{"ok":true}`)
	write.Reusable = true
	out, err := runShortcutWithStubs(t, TablePut,
		[]string{"--url", testURL,
			"--sheets", `{"sheets":[{"name":"数据","columns":["a","b"],"data":[["x","y"]]}]}`,
			"--styles", `{"styles":[{"name":"数据","cell_styles":[{"range":"A1:B1","font_weight":"bold"}],"cell_merges":[{"range":"A1:B1"}]}]}`},
		structure, write)
	if err != nil {
		t.Fatalf("execute failed: %v\nout=%s", err, out)
	}
	data := decodeEnvelopeData(t, out)
	sheets, _ := data["sheets"].([]interface{})
	if len(sheets) != 1 {
		t.Fatalf("result sheets = %d, want 1: %#v", len(sheets), data)
	}
}

// ─── validation through the cobra surface ─────────────────────────────

func TestTablePut_Validation(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "missing spreadsheet locator rejected",
			args: []string{"--sheets", tablePutSheetsJSON},
			want: "at least one",
		},
		{
			name: "url and token are mutually exclusive",
			args: []string{"--url", testURL, "--spreadsheet-token", testToken, "--sheets", tablePutSheetsJSON},
			want: "mutually exclusive",
		},
		{
			name: "duplicate column name rejected",
			args: []string{"--url", testURL, "--sheets", `{"sheets":[{"name":"S","columns":["a","a"],"data":[]}]}`},
			want: "duplicate column name",
		},
		{
			name: "row width mismatch rejected",
			args: []string{"--url", testURL, "--sheets", `{"sheets":[{"name":"S","columns":["a","b"],"data":[["only-one"]]}]}`},
			want: "column count",
		},
		{
			name: "trailing JSON data after --sheets value rejected",
			args: []string{"--url", testURL, "--sheets", `{"sheets":[{"name":"S","columns":["a"],"data":[]}]} trailing`},
			want: "trailing data after JSON value",
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, _, err := runShortcutCapturingErr(t, TablePut, append(tt.args, "--dry-run"))
			requireValidation(t, err, tt.want)
		})
	}
}

// ─── execute paths with stubbed tools ─────────────────────────────────

// TestTablePut_ExecuteWrite drives the write path: a structure read maps the
// existing sheet by name, then a set_cell_range write fills it.
func TestTablePut_ExecuteWrite(t *testing.T) {
	t.Parallel()
	structure := toolOutputStub(testToken, "read", `{"sheets":[{"sheet_id":"`+testSheetID+`","sheet_name":"数据","index":0}]}`)
	write := toolOutputStub(testToken, "write", `{"updated_cells_count":2}`)
	out, err := runShortcutWithStubs(t, TablePut,
		[]string{"--url", testURL, "--sheets",
			`{"sheets":[{"name":"数据","columns":["a","b"],"dtypes":{"b":"int64"},"data":[["x",1]]}]}`},
		structure, write)
	if err != nil {
		t.Fatalf("execute failed: %v\nout=%s", err, out)
	}
	data := decodeEnvelopeData(t, out)
	sheets, _ := data["sheets"].([]interface{})
	if len(sheets) != 1 {
		t.Fatalf("result sheets = %d, want 1: %#v", len(sheets), data)
	}
	s0, _ := sheets[0].(map[string]interface{})
	if s0["name"] != "数据" || s0["sheet_id"] != testSheetID {
		t.Errorf("sheet summary = %#v, want name=数据 sheet_id=%s", s0, testSheetID)
	}
	if s0["range"] != "A1:B2" {
		t.Errorf("range = %v, want A1:B2", s0["range"])
	}
}

// TestTablePut_ExecuteWriteCreatesMissingSheet covers the branch where the
// named sheet does not yet exist: a create precedes the write.
func TestTablePut_ExecuteWriteCreatesMissingSheet(t *testing.T) {
	t.Parallel()
	// First structure read sees only "Sheet1"; the payload targets "新表", so
	// createSheet runs, and the follow-up read (FIFO: second stub) resolves the
	// newly created sheet's id.
	structBefore := toolOutputStub(testToken, "read", `{"sheets":[{"sheet_id":"`+testSheetID+`","sheet_name":"Sheet1","index":0}]}`)
	structAfter := toolOutputStub(testToken, "read", `{"sheets":[{"sheet_id":"`+testSheetID+`","sheet_name":"Sheet1","index":0},{"sheet_id":"`+testSheetID2+`","sheet_name":"新表","index":1}]}`)
	write := toolOutputStub(testToken, "write", `{"ok":true}`)
	write.Reusable = true // modify_workbook_structure create + set_cell_range
	out, err := runShortcutWithStubs(t, TablePut,
		[]string{"--url", testURL, "--sheets",
			`{"sheets":[{"name":"新表","columns":["a"],"data":[["x"]]}]}`},
		structBefore, structAfter, write)
	if err != nil {
		t.Fatalf("execute failed: %v\nout=%s", err, out)
	}
	data := decodeEnvelopeData(t, out)
	sheets, _ := data["sheets"].([]interface{})
	if len(sheets) != 1 {
		t.Fatalf("result sheets = %d, want 1", len(sheets))
	}
	if s0, _ := sheets[0].(map[string]interface{}); s0["sheet_id"] != testSheetID2 {
		t.Errorf("created sheet id = %v, want %s", s0["sheet_id"], testSheetID2)
	}
}

// TestTablePut_SheetCreateDims checks new-sheet sizing: small tables keep the
// 20×200 floor (unchanged behavior), wide/long tables grow past it (the fix for
// set_cell_range "exceeds sheet bounds"), and start_cell offset + header row are
// accounted for, with columns clamped to the backend's 200 ceiling.
func TestTablePut_SheetCreateDims(t *testing.T) {
	t.Parallel()
	bp := func(b bool) *bool { return &b }
	cols := func(n int) []tableColumnSpec { return make([]tableColumnSpec, n) }
	rows := func(n int) [][]interface{} { return make([][]interface{}, n) }
	cases := []struct {
		name               string
		spec               tableSheetSpec
		styles             *workbookCreateStylePayload
		wantRows, wantCols int
	}{
		{"small table keeps 20x200 floor", tableSheetSpec{Columns: cols(3), Rows: rows(5)}, nil, 200, 20},
		{"wide table grows columns", tableSheetSpec{Columns: cols(37), Rows: rows(22)}, nil, 200, 37},
		{"long table grows rows", tableSheetSpec{Columns: cols(3), Rows: rows(500)}, nil, 501, 20},
		{"start_cell offset adds to both", tableSheetSpec{StartCell: "C5", Columns: cols(40), Rows: rows(5)}, nil, 200, 42},
		{"header:false drops the header row", tableSheetSpec{Header: bp(false), Columns: cols(3), Rows: rows(500)}, nil, 500, 20},
		{"columns clamp at backend max 200", tableSheetSpec{Columns: cols(250), Rows: rows(5)}, nil, 200, 200},
		// Default headerOn() is false for append mode, but writeSheetData forces
		// a header when append hits an empty sheet with no explicit Header
		// choice (so column names aren't lost). sheetCreateDims runs only on
		// brand-new sheets — which are empty by definition — so it has to
		// match: append + Header=nil ⇒ +1 row. Otherwise an append-near-50000
		// payload would be created one row short.
		{"append on new sheet sizes for the forced header row (49999 data rows + 1 header = 50000)",
			tableSheetSpec{Mode: "append", Columns: cols(3), Rows: rows(49999)}, nil, 50000, 20},
		{"append + Header=false (explicit) does NOT add the forced header row",
			tableSheetSpec{Mode: "append", Header: bp(false), Columns: cols(3), Rows: rows(50)}, nil, 200, 20},
		// --styles reaching past the data grows the grid so a cell_styles op on a
		// blank cell (or a merge / resize) still fits after the create.
		{"styles past the data grow the grid",
			tableSheetSpec{Columns: cols(3), Rows: rows(5)},
			&workbookCreateStylePayload{CellStyles: []workbookCreateCellStyleOp{{Range: "A1:Z400"}}}, 400, 26},
		{"styles inside the data don't shrink the grid",
			tableSheetSpec{Columns: cols(3), Rows: rows(5)},
			&workbookCreateStylePayload{CellStyles: []workbookCreateCellStyleOp{{Range: "A1:B2"}}}, 200, 20},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			gotRows, gotCols := sheetCreateDims(&tt.spec, tt.styles)
			if gotRows != tt.wantRows || gotCols != tt.wantCols {
				t.Errorf("sheetCreateDims = (%d rows, %d cols), want (%d, %d)", gotRows, gotCols, tt.wantRows, tt.wantCols)
			}
		})
	}
}

// TestTablePut_ExecuteCreatesWideSheetWithDims is the regression test for the
// wide-table bug: a 25-column payload targeting a not-yet-existing sheet must
// create it with 25 columns (past the 20-column default) so the follow-up
// set_cell_range fits instead of failing with "exceeds sheet bounds".
func TestTablePut_ExecuteCreatesWideSheetWithDims(t *testing.T) {
	t.Parallel()
	structBefore := toolOutputStub(testToken, "read", `{"sheets":[{"sheet_id":"`+testSheetID+`","sheet_name":"Sheet1","index":0}]}`)
	createStub := toolOutputStub(testToken, "write", `{"ok":true}`) // modify_workbook_structure create
	structAfter := toolOutputStub(testToken, "read", `{"sheets":[{"sheet_id":"`+testSheetID+`","sheet_name":"Sheet1","index":0},{"sheet_id":"`+testSheetID2+`","sheet_name":"宽表","index":1}]}`)
	writeStub := toolOutputStub(testToken, "write", `{"ok":true}`) // set_cell_range
	const n = 25
	// Distinct names per column — the new wire shape rejects duplicates at
	// normalize-time, so a repeated "c" would never reach the create call.
	colNames := make([]string, n)
	for i := 0; i < n; i++ {
		colNames[i] = fmt.Sprintf(`"c%d"`, i)
	}
	cols := strings.Join(colNames, ",")
	vals := strings.TrimRight(strings.Repeat(`"x",`, n), ",")
	payload := `{"sheets":[{"name":"宽表","columns":[` + cols + `],"data":[[` + vals + `]]}]}`
	out, err := runShortcutWithStubs(t, TablePut,
		[]string{"--url", testURL, "--sheets", payload},
		structBefore, createStub, structAfter, writeStub)
	if err != nil {
		t.Fatalf("execute failed: %v\nout=%s", err, out)
	}
	var wire map[string]interface{}
	if err := json.Unmarshal(createStub.CapturedBody, &wire); err != nil {
		t.Fatalf("decode create body: %v", err)
	}
	var input map[string]interface{}
	if err := json.Unmarshal([]byte(wire["input"].(string)), &input); err != nil {
		t.Fatalf("decode create tool input: %v", err)
	}
	if input["operation"] != "create" {
		t.Fatalf("first write should be the create op, got %#v", input["operation"])
	}
	if input["columns"] != float64(n) {
		t.Errorf("create columns = %#v, want %d (sized to the wide payload)", input["columns"], n)
	}
	if input["rows"] != float64(200) {
		t.Errorf("create rows = %#v, want 200 (floor)", input["rows"])
	}
}

// TestTablePut_ExecutePartialFailure covers the partial-success error path:
// a set_cell_range write fails mid-import and the structured error surfaces.
// TestTablePut_ExecuteTotalFailure: a single sheet whose write fails landed
// nothing — it must be a plain failure, NOT partial_success.
func TestTablePut_ExecuteTotalFailure(t *testing.T) {
	t.Parallel()
	structure := toolOutputStub(testToken, "read", `{"sheets":[{"sheet_id":"`+testSheetID+`","sheet_name":"数据","index":0}]}`)
	writeErr := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/sheet_ai/v2/spreadsheets/" + testToken + "/tools/invoke_write",
		Body:   map[string]interface{}{"code": 1254000, "msg": "boom"},
	}
	out, err := runShortcutWithStubs(t, TablePut,
		[]string{"--url", testURL, "--sheets",
			`{"sheets":[{"name":"数据","columns":["a"],"data":[["x"]]}]}`},
		structure, writeErr)
	if err == nil {
		t.Fatalf("expected failure; got nil. out=%s", out)
	}
	if strings.Contains(err.Error(), "partially applied") || strings.Contains(out, "partially applied") {
		t.Errorf("single-sheet failure must NOT be partial_success; got err=%v out=%s", err, out)
	}
	if !strings.Contains(err.Error(), "failed") && !strings.Contains(out, "no sheets were written") {
		t.Errorf("expected plain-failure message; got err=%v out=%s", err, out)
	}
}

// TestTablePut_ExecutePartialFailure: first sheet's write lands, second fails →
// partial_success carrying the first sheet in written_sheets.
func TestTablePut_ExecutePartialFailure(t *testing.T) {
	t.Parallel()
	structure := toolOutputStub(testToken, "read",
		`{"sheets":[{"sheet_id":"`+testSheetID+`","sheet_name":"汇总","index":0},{"sheet_id":"`+testSheetID2+`","sheet_name":"明细","index":1}]}`)
	writeOK := toolOutputStub(testToken, "write", `{"updated_cells_count":2}`)
	writeErr := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/sheet_ai/v2/spreadsheets/" + testToken + "/tools/invoke_write",
		Body:   map[string]interface{}{"code": 1254000, "msg": "boom"},
	}
	out, err := runShortcutWithStubs(t, TablePut,
		[]string{"--url", testURL, "--sheets",
			`{"sheets":[{"name":"汇总","columns":["a"],"data":[["x"]]},{"name":"明细","columns":["a"],"data":[["y"]]}]}`},
		structure, writeOK, writeErr)
	if err == nil {
		t.Fatalf("expected partial-failure exit signal; got nil. out=%s", out)
	}
	// Per the typed-error contract, a partial success is a result, not an error
	// envelope: the ok:false result (with written_sheets and the failing sheet's
	// name) lands on stdout, and err is the bare partial-failure exit signal.
	var pfErr *output.PartialFailureError
	if !errors.As(err, &pfErr) {
		t.Errorf("expected *output.PartialFailureError exit signal; got %T %v", err, err)
	}
	if !strings.Contains(out, "written_sheets") {
		t.Errorf("expected written_sheets in the partial-success result; got out=%s", out)
	}
	// The failing sheet is named in the reason; the written one lives in
	// written_sheets — both on stdout.
	if !strings.Contains(out, "明细") {
		t.Errorf("partial_success should name the failed sheet 明细; got out=%s", out)
	}
}

// ─── +workbook-create typed --sheets path ─────────────────────────────

// TestWorkbookCreate_TypedMutualExclusion locks the Validate contract: the typed
// --sheets entry can't be combined with the untyped --headers/--values.
func TestWorkbookCreate_TypedMutualExclusion(t *testing.T) {
	t.Parallel()
	typed := `{"sheets":[{"name":"S","columns":["a"],"data":[["x"]]}]}`
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"sheets+values", []string{"--title", "X", "--sheets", typed, "--values", `[["x"]]`}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, _, err := runShortcutCapturingErr(t, WorkbookCreate, tc.args)
			requireValidation(t, err, "mutually exclusive")
		})
	}
}

// TestWorkbookCreate_EmptySheetsErrors locks the fix for an explicitly-given but
// empty --sheets (e.g. empty stdin / file): it must error, not silently fall
// through to creating an empty workbook.
func TestWorkbookCreate_EmptySheetsErrors(t *testing.T) {
	t.Parallel()
	_, _, err := runShortcutCapturingErr(t, WorkbookCreate, []string{"--title", "X", "--sheets", ""})
	requireValidation(t, err, "empty")
}

// TestWorkbookCreate_TypedAdoptsDefaultSheet covers the one-step typed create:
// the new workbook's default sheet is renamed to the first payload sheet's name
// and reused (no empty Sheet1 left behind), then written type-faithfully (the
// date lands as an Excel serial, not text).
func TestWorkbookCreate_TypedAdoptsDefaultSheet(t *testing.T) {
	t.Parallel()
	create := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/sheets/v3/spreadsheets",
		Body: map[string]interface{}{
			"code": 0, "msg": "success",
			"data": map[string]interface{}{
				"spreadsheet": map[string]interface{}{"spreadsheet_token": "shtTYPED", "title": "Demo"},
			},
		},
	}
	// lookupFirstSheetID and writeTypedSheets' listSheetIDsByName both read the
	// structure; one reusable stub serves both, reporting only the default sheet.
	structure := toolOutputStub("shtTYPED", "read", `{"sheets":[{"sheet_id":"shtDef","sheet_name":"Sheet1","index":0}]}`)
	structure.Reusable = true
	rename := &httpmock.Stub{
		Method:     "POST",
		URL:        "/open-apis/sheet_ai/v2/spreadsheets/shtTYPED/tools/invoke_write",
		BodyFilter: func(b []byte) bool { return strings.Contains(string(b), "modify_workbook_structure") },
		Body:       map[string]interface{}{"code": 0, "msg": "success", "data": map[string]interface{}{"output": `{"ok":true}`}},
	}
	write := &httpmock.Stub{
		Method:     "POST",
		URL:        "/open-apis/sheet_ai/v2/spreadsheets/shtTYPED/tools/invoke_write",
		BodyFilter: func(b []byte) bool { return strings.Contains(string(b), "set_cell_range") },
		Body:       map[string]interface{}{"code": 0, "msg": "success", "data": map[string]interface{}{"output": `{"updated_cells_count":4}`}},
	}
	out, err := runShortcutWithStubs(t, WorkbookCreate, []string{
		"--title", "Demo",
		"--sheets", `{"sheets":[{"name":"Sales","columns":["d","amt"],"dtypes":{"d":"datetime64[ns]","amt":"float64"},"data":[["2024-01-15",1234.5]]}]}`,
	}, create, structure, rename, write)
	if err != nil {
		t.Fatalf("typed create failed: %v\nout=%s", err, out)
	}
	data := decodeEnvelopeData(t, out)
	if ss, _ := data["spreadsheet"].(map[string]interface{}); ss["spreadsheet_token"] != "shtTYPED" {
		t.Errorf("spreadsheet_token = %v, want shtTYPED", data["spreadsheet"])
	}
	if sheets, _ := data["sheets"].([]interface{}); len(sheets) != 1 {
		t.Fatalf("want 1 written sheet, got %#v", data["sheets"])
	}
	// Default sheet adopted: rename targets shtDef → "Sales" (no new sheet, no
	// stray Sheet1).
	renameInput := decodeToolInput(t, decodeRawEnvelopeBody(t, rename.CapturedBody), "modify_workbook_structure")
	if renameInput["operation"] != "rename" || renameInput["sheet_id"] != "shtDef" || renameInput["new_name"] != "Sales" {
		t.Errorf("rename should adopt default shtDef→Sales; got %#v", renameInput)
	}
	// The data write carries the date as serial 45306, proving the type-faithful path.
	writeInput := decodeToolInput(t, decodeRawEnvelopeBody(t, write.CapturedBody), "set_cell_range")
	cellsJSON, _ := json.Marshal(writeInput["cells"])
	if !strings.Contains(string(cellsJSON), "45306") {
		t.Errorf("date 2024-01-15 should be written as serial 45306; cells=%s", cellsJSON)
	}
}

// TestWorkbookCreate_TypedDryRun verifies the dry-run previews create + a typed
// set_cell_range write with the date already converted to a serial.
func TestWorkbookCreate_TypedDryRun(t *testing.T) {
	t.Parallel()
	calls := parseDryRunAPI(t, WorkbookCreate, []string{
		"--title", "Demo",
		"--sheets", `{"sheets":[{"name":"S","columns":["d"],"dtypes":{"d":"datetime64[ns]"},"data":[["2024-01-15"]]}]}`,
	})
	if len(calls) != 2 {
		t.Fatalf("want 2 dry-run calls (create + typed write), got %d", len(calls))
	}
	raw, _ := json.Marshal(calls[1])
	if !strings.Contains(string(raw), "45306") {
		t.Errorf("typed dry-run write should contain serial 45306; got %s", raw)
	}
}

func TestWorkbookCreate_TypedDryRun_MultiSheetStyles(t *testing.T) {
	t.Parallel()
	calls := parseDryRunAPI(t, WorkbookCreate, []string{
		"--title", "Demo",
		"--sheets", `{"sheets":[{"name":"S1","columns":["name"],"data":[["alice"]]},{"name":"S2","columns":["amount"],"dtypes":{"amount":"int64"},"formats":{"amount":"0"},"data":[[12]]}]}`,
		"--styles", `{"styles":[{"name":"S1","cell_styles":[{"range":"A1:A2","background_color":"#f5f5f5"}],"cell_merges":[{"range":"A1:A2"}]},{"name":"S2","cell_styles":[{"range":"A1","font_weight":"bold"},{"range":"A2","font_color":"#0f7b0f"}],"col_sizes":[{"range":"A:A","type":"pixel","size":120}],"row_sizes":[{"range":"1:1","type":"pixel","size":28}]}]}`,
	})
	if len(calls) != 6 {
		t.Fatalf("want 6 dry-run calls (create + two typed writes + merge + two resizes), got %d", len(calls))
	}
	firstBody, _ := calls[1].(map[string]interface{})["body"].(map[string]interface{})
	firstInput := decodeToolInput(t, firstBody, "set_cell_range")
	firstRaw, _ := json.Marshal(firstInput["cells"])
	if !strings.Contains(string(firstRaw), `"background_color":"#f5f5f5"`) {
		t.Errorf("first sheet should carry global style; cells=%s", firstRaw)
	}
	secondBody, _ := calls[3].(map[string]interface{})["body"].(map[string]interface{})
	secondInput := decodeToolInput(t, secondBody, "set_cell_range")
	secondRaw, _ := json.Marshal(secondInput["cells"])
	if !strings.Contains(string(secondRaw), `"font_weight":"bold"`) || !strings.Contains(string(secondRaw), `"font_color":"#0f7b0f"`) {
		t.Errorf("second sheet should carry per-cell styles; cells=%s", secondRaw)
	}
	allRaw, _ := json.Marshal(calls)
	if !strings.Contains(string(allRaw), "merge_cells") {
		t.Errorf("dry-run should include merge_cells visual op; calls=%s", allRaw)
	}
	if got := strings.Count(string(allRaw), "resize_range"); got != 2 {
		t.Errorf("dry-run resize_range count = %d, want 2; calls=%s", got, allRaw)
	}
}

func TestTablePut_StringifyCellValue(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   interface{}
		want string
	}{
		{"plain", "plain"},
		{json.Number("12345678901234"), "12345678901234"},
		{true, "TRUE"},
		{false, "FALSE"},
		{3.5, "3.5"},
	}
	for _, tt := range cases {
		if got := stringifyCellValue(tt.in); got != tt.want {
			t.Errorf("stringifyCellValue(%#v) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestTablePut_DescribeJSONType(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   interface{}
		want string
	}{
		{"x", "a string"},
		{json.Number("1"), "a number"},
		{true, "a boolean"},
		{[]interface{}{}, "an array"},
		{map[string]interface{}{}, "an object"},
		{3.14, "float64"},
	}
	for _, tt := range cases {
		if got := describeJSONType(tt.in); got != tt.want {
			t.Errorf("describeJSONType(%#v) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestTablePut_HeaderAndMode(t *testing.T) {
	t.Parallel()
	bp := func(b bool) *bool { return &b }
	// headerOn: overwrite writes header, append omits it by default, explicit wins
	if !headerOn(&tableSheetSpec{}) {
		t.Error("overwrite default should write header")
	}
	if headerOn(&tableSheetSpec{Mode: "append"}) {
		t.Error("append should omit header by default")
	}
	if !headerOn(&tableSheetSpec{Mode: "append", Header: bp(true)}) {
		t.Error("explicit header:true should override append default")
	}
	if headerOn(&tableSheetSpec{Header: bp(false)}) {
		t.Error("explicit header:false should be honored")
	}
	// writeModeName
	if writeModeName(&tableSheetSpec{}) != "overwrite" || writeModeName(&tableSheetSpec{Mode: "append"}) != "append" {
		t.Error("writeModeName normalization wrong")
	}
	// buildSheetMatrix header toggle
	s := &tableSheetSpec{Columns: []tableColumnSpec{{Name: "a", Type: "string"}}, Rows: [][]interface{}{{"x"}}}
	if m, _ := buildSheetMatrix(s, false); len(m) != 1 {
		t.Errorf("header off → 1 data row, got %d", len(m))
	}
	if m, _ := buildSheetMatrix(s, true); len(m) != 2 {
		t.Errorf("header on → header + 1 data row, got %d", len(m))
	}
}

func TestTablePut_BadModeRejected(t *testing.T) {
	t.Parallel()
	_, err := parseTablePutPayload(stubFlagView{"sheets": `{"sheets":[{"name":"S","mode":"upsert","columns":["a"],"data":[]}]}`})
	requireValidation(t, err, "invalid")
}

// TestTablePut_AppendEmptySheetWritesHeader: appending to an EMPTY sheet still
// writes the header row, so column names aren't lost (and a later +table-get
// won't consume the first data row as the header).
func TestTablePut_AppendEmptySheetWritesHeader(t *testing.T) {
	t.Parallel()
	structure := toolOutputStub(testToken, "read", `{"sheets":[{"sheet_id":"`+testSheetID+`","sheet_name":"新","index":0}]}`)
	region := toolOutputStub(testToken, "read", `{}`) // empty sheet: no current_region → lastRow 0
	write := toolOutputStub(testToken, "write", `{"ok":true}`)
	out, err := runShortcutWithStubs(t, TablePut,
		[]string{"--url", testURL, "--sheets",
			`{"sheets":[{"name":"新","mode":"append","columns":["列A"],"data":[["x"],["y"]]}]}`},
		structure, region, write)
	if err != nil {
		t.Fatalf("execute failed: %v\nout=%s", err, out)
	}
	var wire map[string]interface{}
	if err := json.Unmarshal(write.CapturedBody, &wire); err != nil {
		t.Fatalf("decode captured write body: %v", err)
	}
	var input map[string]interface{}
	if err := json.Unmarshal([]byte(wire["input"].(string)), &input); err != nil {
		t.Fatalf("decode tool input: %v", err)
	}
	cells, _ := input["cells"].([]interface{})
	if len(cells) != 3 {
		t.Fatalf("empty-sheet append should write header + 2 data rows = 3, got %d", len(cells))
	}
	if header, _ := cells[0].([]interface{}); len(header) > 0 {
		if h0, _ := header[0].(map[string]interface{}); h0["value"] != "列A" {
			t.Errorf("first row should be the header 列A; got %#v", h0)
		}
	}
	if input["range"] != "A1:A3" {
		t.Errorf("range = %v, want A1:A3 (header + 2 rows at top of empty sheet)", input["range"])
	}
}

// TestTablePut_ExecuteAppend verifies append placement: data lands below the
// sheet's existing data (current_region A1:B5 → start at row 6) with no repeated
// header.
func TestTablePut_ExecuteAppend(t *testing.T) {
	t.Parallel()
	structure := toolOutputStub(testToken, "read", `{"sheets":[{"sheet_id":"`+testSheetID+`","sheet_name":"日志","index":0}]}`)
	region := toolOutputStub(testToken, "read", `{"current_region":"A1:B5","actual_range":"A1:B5"}`)
	write := toolOutputStub(testToken, "write", `{"ok":true}`)
	out, err := runShortcutWithStubs(t, TablePut,
		[]string{"--url", testURL, "--sheets",
			`{"sheets":[{"name":"日志","mode":"append","columns":["时间","值"],"dtypes":{"值":"int64"},"data":[["t1",1],["t2",2]]}]}`},
		structure, region, write)
	if err != nil {
		t.Fatalf("execute failed: %v\nout=%s", err, out)
	}
	// inspect the set_cell_range request the append produced
	var wire map[string]interface{}
	if err := json.Unmarshal(write.CapturedBody, &wire); err != nil {
		t.Fatalf("decode captured write body: %v", err)
	}
	var input map[string]interface{}
	if err := json.Unmarshal([]byte(wire["input"].(string)), &input); err != nil {
		t.Fatalf("decode tool input: %v", err)
	}
	if input["range"] != "A6:B7" {
		t.Errorf("append range = %v, want A6:B7 (2 rows below last data row 5, no header)", input["range"])
	}
	if cells, _ := input["cells"].([]interface{}); len(cells) != 2 {
		t.Errorf("append should write 2 data rows (no header), got %d", len(cells))
	}
	data := decodeEnvelopeData(t, out)
	if s0, _ := data["sheets"].([]interface{})[0].(map[string]interface{}); s0["mode"] != "append" {
		t.Errorf("summary mode = %v, want append", s0["mode"])
	}
}

// TestTablePut_HeaderFalseAndAllowOverwrite checks header:false drops the
// header row and allow_overwrite:false reaches the tool input.
func TestTablePut_HeaderFalseAndAllowOverwrite(t *testing.T) {
	t.Parallel()
	calls := parseDryRunAPI(t, TablePut, []string{"--url", testURL, "--sheets",
		`{"sheets":[{"name":"S","header":false,"allow_overwrite":false,"columns":["a"],"data":[["x"],["y"]]}]}`})
	body, _ := calls[0].(map[string]interface{})["body"].(map[string]interface{})
	input := decodeToolInput(t, body, "set_cell_range")
	if input["allow_overwrite"] != false {
		t.Errorf("allow_overwrite = %v, want false", input["allow_overwrite"])
	}
	rows, _ := input["cells"].([]interface{})
	if len(rows) != 2 {
		t.Fatalf("header:false → 2 data rows only, got %d", len(rows))
	}
	first, _ := rows[0].([]interface{})[0].(map[string]interface{})
	if first["value"] != "x" {
		t.Errorf("header:false first cell = %v, want data 'x' (no header row)", first["value"])
	}
}

// ─── +table-get ───────────────────────────────────────────────────────

func TestTableGet_SerialRoundTrip(t *testing.T) {
	t.Parallel()
	for _, iso := range []string{"2024-01-15", "2024-02-29", "2000-01-01", "1899-12-31"} {
		s, err := isoDateToSerial(iso)
		if err != nil {
			t.Fatalf("isoDateToSerial(%s): %v", iso, err)
		}
		if back := serialToISO(float64(s)); back != iso {
			t.Errorf("roundtrip %s → %d → %s", iso, s, back)
		}
	}
}

func TestTableGet_IsDateNumberFormat(t *testing.T) {
	t.Parallel()
	for _, nf := range []string{"yyyy-mm-dd", "yyyy-mm", "yyyy/m/d", "YYYY/MM/DD", "yy-mm-dd", "[Red]yyyy-mm-dd", `"At "yyyy`} {
		if !isDateNumberFormat(nf) {
			t.Errorf("%q should be a date format", nf)
		}
	}
	// JPY (and other currency / unit prefixes that happen to contain a lone Y)
	// were previously misread as dates because the scanner saw any lowercase 'y'.
	// Pure numeric formats, escaped/quoted literals, and bracket sections must
	// never trigger the date branch — those rows would have round-tripped
	// integer cells as ISO dates.
	for _, nf := range []string{
		"#,##0", "0.00", "0.00%", "@", "",
		"JPY #,##0", "JPY 0", `"YEN "#,##0`, "[$JPY-411] #,##0",
		`\y\y`, // escaped letters, not a year token
	} {
		if isDateNumberFormat(nf) {
			t.Errorf("%q should NOT be a date format", nf)
		}
	}
}

func TestTableGet_InferColumnType(t *testing.T) {
	t.Parallel()
	mk := func(v interface{}, nf string) map[string]interface{} {
		c := map[string]interface{}{"value": v}
		if nf != "" {
			c["cell_styles"] = map[string]interface{}{"number_format": nf}
		}
		return c
	}
	col := func(cells ...map[string]interface{}) [][]map[string]interface{} {
		rows := make([][]map[string]interface{}, len(cells))
		for i, c := range cells {
			rows[i] = []map[string]interface{}{c}
		}
		return rows
	}
	if typ, f := inferColumnType(col(mk(45306.0, "yyyy-mm-dd")), 0); typ != "date" || f != "yyyy-mm-dd" {
		t.Errorf("date col → %s/%s", typ, f)
	}
	if typ, f := inferColumnType(col(mk(100.0, "#,##0")), 0); typ != "number" || f != "#,##0" {
		t.Errorf("number col → %s/%s", typ, f)
	}
	if typ, _ := inferColumnType(col(mk(true, "")), 0); typ != "bool" {
		t.Errorf("bool col → %s", typ)
	}
	if typ, _ := inferColumnType(col(mk("x", "")), 0); typ != "string" {
		t.Errorf("string col → %s", typ)
	}
	// digit-like value carrying text format (@) infers as string, not number —
	// this is what makes +table-put's string columns (ids/postcodes) survive read-back.
	if typ, _ := inferColumnType(col(mk(123.0, "@")), 0); typ != "string" {
		t.Errorf("@-format numeric-looking col → %s, want string", typ)
	}
	if typ, _ := inferColumnType([][]map[string]interface{}{}, 0); typ != "string" {
		t.Errorf("empty col → %s (want string)", typ)
	}

	// Mixed number+text degrades to string (self-consistent: every value is then
	// a string), so the column round-trips and pandas doesn't choke. Numeric
	// coercion of the dirty cells is left to the caller (pandas to_numeric).
	if typ, _ := inferColumnType(col(mk(100.0, ""), mk("N/A", ""), mk(200.0, "")), 0); typ != "string" {
		t.Errorf("mixed number+text col → %s, want string", typ)
	}
	// A bare number mixed into a date column must NOT stay date (would serial-
	// convert the number into a bogus date) — degrades to string.
	if typ, _ := inferColumnType(col(mk(45306.0, "yyyy-mm-dd"), mk(5.0, "")), 0); typ != "string" {
		t.Errorf("date+bare-number col → %s, want string", typ)
	}
}

func TestTableGet_CellToTyped(t *testing.T) {
	t.Parallel()
	mk := func(v interface{}) map[string]interface{} { return map[string]interface{}{"value": v} }
	if v := cellToTyped(mk(45306.0), "date"); v != "2024-01-15" {
		t.Errorf("date serial → %v, want 2024-01-15", v)
	}
	if v := cellToTyped(mk(100.0), "number"); v != 100.0 {
		t.Errorf("number → %v", v)
	}
	if v := cellToTyped(mk(true), "bool"); v != true {
		t.Errorf("bool → %v", v)
	}
	if v := cellToTyped(mk(""), "string"); v != nil {
		t.Errorf("empty string → %v, want nil", v)
	}
	if v := cellToTyped(nil, "string"); v != nil {
		t.Errorf("nil → %v, want nil", v)
	}
	if v := cellToTyped(mk("hi"), "string"); v != "hi" {
		t.Errorf("string → %v", v)
	}
}

// TestTableGet_DigitStringRoundTrip: a column +table-put wrote as string (text
// format @) reads back as string, not number — so leading-zero ids / postcodes
// survive instead of collapsing to a number.
func TestTableGet_DigitStringRoundTrip(t *testing.T) {
	t.Parallel()
	structure := toolOutputStub(testToken, "read", `{"sheets":[{"sheet_id":"`+testSheetID+`","sheet_name":"S","row_count":200,"column_count":20,"index":0}]}`)
	region := toolOutputStub(testToken, "read", `{"current_region":"A1:A2"}`)
	cells := toolOutputStub(testToken, "read", `{"ranges":[{"cells":[`+
		`[{"value":"邮编"}],`+
		`[{"value":"00123","cell_styles":{"number_format":"@"}}]`+
		`]}]}`)
	out, err := runShortcutWithStubs(t, TableGet,
		[]string{"--url", testURL, "--sheet-name", "S"}, structure, region, cells)
	if err != nil {
		t.Fatalf("execute failed: %v\nout=%s", err, out)
	}
	data := decodeEnvelopeData(t, out)
	sheets, _ := data["sheets"].([]interface{})
	s0, _ := sheets[0].(map[string]interface{})
	cols, _ := s0["columns"].([]interface{})
	if cols[0] != "邮编" {
		t.Errorf("columns[0] = %v, want 邮编", cols[0])
	}
	dtypes, _ := s0["dtypes"].(map[string]interface{})
	if dtypes["邮编"] != "object" {
		t.Errorf("dtypes[邮编] = %v, want object (text-format column round-trips as string)", dtypes["邮编"])
	}
	// The writer paints `@` on string columns so digit-like text survives;
	// surfacing that back as a user-set format would round-trip noisily, so the
	// reader strips it. Hence: no "formats" key at all on an all-string sheet.
	if _, has := s0["formats"]; has {
		t.Errorf("@ is a writer convention, must NOT surface in formats: %#v", s0["formats"])
	}
	rows, _ := s0["data"].([]interface{})
	if r0, _ := rows[0].([]interface{}); r0[0] != "00123" {
		t.Errorf("value = %v, want \"00123\" (leading zero preserved)", r0[0])
	}
}

// TestTableGet_ExecuteRoundTrip reads a sheet back and checks the output is the
// same typed protocol +table-put consumes: date serial → ISO, number preserved,
// types inferred from number_format.
func TestTableGet_ExecuteRoundTrip(t *testing.T) {
	t.Parallel()
	structure := toolOutputStub(testToken, "read", `{"sheets":[{"sheet_id":"`+testSheetID+`","sheet_name":"销售","row_count":200,"column_count":20,"index":0}]}`)
	region := toolOutputStub(testToken, "read", `{"current_region":"A1:C2"}`)
	cells := toolOutputStub(testToken, "read", `{"ranges":[{"cells":[`+
		`[{"value":"门店"},{"value":"月份"},{"value":"销售额"}],`+
		`[{"value":"北京"},{"value":45306,"cell_styles":{"number_format":"yyyy-mm"}},{"value":259874,"cell_styles":{"number_format":"#,##0"}}]`+
		`]}]}`)
	out, err := runShortcutWithStubs(t, TableGet,
		[]string{"--url", testURL, "--sheet-name", "销售"}, structure, region, cells)
	if err != nil {
		t.Fatalf("execute failed: %v\nout=%s", err, out)
	}
	data := decodeEnvelopeData(t, out)
	sheets, _ := data["sheets"].([]interface{})
	if len(sheets) != 1 {
		t.Fatalf("want 1 sheet, got %d", len(sheets))
	}
	s0, _ := sheets[0].(map[string]interface{})
	if s0["name"] != "销售" {
		t.Errorf("name = %v, want 销售", s0["name"])
	}
	cols, _ := s0["columns"].([]interface{})
	if len(cols) != 3 {
		t.Fatalf("want 3 columns, got %d", len(cols))
	}
	if cols[0] != "门店" || cols[1] != "月份" || cols[2] != "销售额" {
		t.Errorf("columns = %#v, want [门店 月份 销售额]", cols)
	}
	dtypes, _ := s0["dtypes"].(map[string]interface{})
	if dtypes["月份"] != "datetime64[ns]" {
		t.Errorf("dtypes[月份] = %v, want datetime64[ns]", dtypes["月份"])
	}
	if dtypes["销售额"] != "float64" {
		t.Errorf("dtypes[销售额] = %v, want float64 (numeric)", dtypes["销售额"])
	}
	if dtypes["门店"] != "object" {
		t.Errorf("dtypes[门店] = %v, want object (string column)", dtypes["门店"])
	}
	formats, _ := s0["formats"].(map[string]interface{})
	if formats["月份"] != "yyyy-mm" {
		t.Errorf("formats[月份] = %v, want yyyy-mm (number_format preserved)", formats["月份"])
	}
	if formats["销售额"] != "#,##0" {
		t.Errorf("formats[销售额] = %v, want #,##0", formats["销售额"])
	}
	rows, _ := s0["data"].([]interface{})
	r0, _ := rows[0].([]interface{})
	if r0[1] != "2024-01-15" {
		t.Errorf("date roundtrip = %v, want 2024-01-15 (serial 45306 → ISO)", r0[1])
	}
	if r0[2] != float64(259874) {
		t.Errorf("number = %v, want 259874", r0[2])
	}
}

// TestTableGet_DuplicateHeaderRejected covers a fangshuyu-flagged round-trip
// hole: a sheet with two columns named the same (e.g. `amount, amount`) used
// to read back successfully — with the dtypes map silently collapsing to a
// single entry — and then fail on the immediate +table-put write because the
// writer rejects duplicate column names. Fail fast at read time with an
// actionable hint to rename or pass --no-header instead.
func TestTableGet_DuplicateHeaderRejected(t *testing.T) {
	t.Parallel()
	structure := toolOutputStub(testToken, "read", `{"sheets":[{"sheet_id":"`+testSheetID+`","sheet_name":"S","row_count":200,"column_count":20,"index":0}]}`)
	region := toolOutputStub(testToken, "read", `{"current_region":"A1:B2"}`)
	cells := toolOutputStub(testToken, "read", `{"ranges":[{"cells":[`+
		`[{"value":"amount"},{"value":"amount"}],`+
		`[{"value":1},{"value":2}]`+
		`]}]}`)
	_, err := runShortcutWithStubs(t, TableGet,
		[]string{"--url", testURL, "--sheet-name", "S"}, structure, region, cells)
	ve := requireValidation(t, err, "duplicate header column name")
	if !strings.Contains(ve.Message, "--no-header") {
		t.Errorf("error should hint about --no-header; got message=%q", ve.Message)
	}
}

// TestTableGet_SheetIDFallbackBackfillsName covers the fallback path: when
// get_workbook_structure cannot enumerate the target row (e.g. selector
// mismatch), the target name was previously left empty — which would later
// fail the +table-put round-trip because the writer requires a non-empty
// sheet name. The target should fall back to using the id as the name.
func TestTableGet_SheetIDFallbackBackfillsName(t *testing.T) {
	t.Parallel()
	// Structure has a different sheet — selector mismatch triggers the fallback.
	structure := toolOutputStub(testToken, "read", `{"sheets":[{"sheet_id":"shtOther","sheet_name":"另一张","row_count":200,"column_count":20,"index":0}]}`)
	region := toolOutputStub(testToken, "read", `{"current_region":"A1:A2"}`)
	cells := toolOutputStub(testToken, "read", `{"ranges":[{"cells":[[{"value":"h"}],[{"value":"x"}]]}]}`)
	out, err := runShortcutWithStubs(t, TableGet,
		[]string{"--url", testURL, "--sheet-id", testSheetID}, structure, region, cells)
	if err != nil {
		t.Fatalf("execute failed: %v\nout=%s", err, out)
	}
	data := decodeEnvelopeData(t, out)
	s0, _ := data["sheets"].([]interface{})[0].(map[string]interface{})
	if s0["name"] != testSheetID {
		t.Errorf("fallback name = %v, want %q (id used as name)", s0["name"], testSheetID)
	}
}

// TestTablePutFullRange_EmptyMatrix covers the dry-run report for the
// header=false + rows=[] case. Previously totalRows=0 produced an invalid
// "A1:C0"; the helper now returns an empty range string instead.
func TestTablePutFullRange_EmptyMatrix(t *testing.T) {
	t.Parallel()
	s := &tableSheetSpec{Name: "S", Columns: []tableColumnSpec{{Name: "a"}, {Name: "b"}, {Name: "c"}}, StartCell: "A1"}
	if got := tablePutFullRange(s, 0); got != "" {
		t.Errorf("tablePutFullRange(_, 0) = %q, want \"\" (no invalid A1:C0)", got)
	}
	// Sanity check: positive totals still produce a real rectangle so we
	// haven't broken the happy path.
	if got := tablePutFullRange(s, 2); got != "A1:C2" {
		t.Errorf("tablePutFullRange(_, 2) = %q, want A1:C2", got)
	}
}

// TestTableGet_OutputRoundTripsBackIntoTablePut is the contract test: the
// output of +table-get must be a payload +table-put accepts. This catches
// dtype/format symmetry breaks early — if the reader ever emits a dtype the
// writer doesn't recognize (or under a key the writer doesn't read), pipe-back
// loops in agent scripts would fail with a confusing JSON error instead of a
// schema error here.
func TestTableGet_OutputRoundTripsBackIntoTablePut(t *testing.T) {
	t.Parallel()
	structure := toolOutputStub(testToken, "read", `{"sheets":[{"sheet_id":"`+testSheetID+`","sheet_name":"S","row_count":200,"column_count":20,"index":0}]}`)
	region := toolOutputStub(testToken, "read", `{"current_region":"A1:D2"}`)
	cells := toolOutputStub(testToken, "read", `{"ranges":[{"cells":[`+
		`[{"value":"city"},{"value":"day"},{"value":"revenue"},{"value":"closed"}],`+
		`[{"value":"BJ"},{"value":45306,"cell_styles":{"number_format":"yyyy-mm-dd"}},{"value":1234.5,"cell_styles":{"number_format":"#,##0.00"}},{"value":true}]`+
		`]}]}`)
	out, err := runShortcutWithStubs(t, TableGet,
		[]string{"--url", testURL, "--sheet-name", "S"}, structure, region, cells)
	if err != nil {
		t.Fatalf("execute failed: %v\nout=%s", err, out)
	}
	data := decodeEnvelopeData(t, out)
	// The reader's "sheets" array is the same key +table-put consumes, so wrap
	// the whole `data` envelope back as a fresh --sheets payload and parse it.
	// A name is mandatory on each sheet, so make sure it survived.
	body, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("marshal reader output: %v", err)
	}
	parsed, err := parseTablePutPayload(stubFlagView{"sheets": string(body)})
	if err != nil {
		t.Fatalf("reader output rejected by writer: %v\npayload=%s", err, body)
	}
	if len(parsed.Sheets) != 1 {
		t.Fatalf("round-trip lost sheets: %#v", parsed.Sheets)
	}
	s := parsed.Sheets[0]
	if s.Name != "S" {
		t.Errorf("name = %q, want S", s.Name)
	}
	wantCols := []tableColumnSpec{
		{Name: "city", Type: "string", Format: "@"},
		{Name: "day", Type: "date", Format: "yyyy-mm-dd"},
		{Name: "revenue", Type: "number", Format: "#,##0.00"},
		{Name: "closed", Type: "bool", Format: ""},
	}
	for i, w := range wantCols {
		if s.Columns[i] != w {
			t.Errorf("columns[%d] after round-trip = %+v, want %+v", i, s.Columns[i], w)
		}
	}
	if len(s.Rows) != 1 || len(s.Rows[0]) != 4 {
		t.Fatalf("rows shape changed: %#v", s.Rows)
	}
}

func TestTableGet_DryRunIncludesCellRead(t *testing.T) {
	t.Parallel()
	calls := parseDryRunAPI(t, TableGet, []string{"--url", testURL, "--sheet-name", "S"})
	found := false
	for _, c := range calls {
		body, _ := c.(map[string]interface{})["body"].(map[string]interface{})
		if body == nil {
			continue
		}
		if tn, _ := body["tool_name"].(string); tn == "get_cell_ranges" {
			found = true
		}
	}
	if !found {
		t.Error("dry-run should include a get_cell_ranges read")
	}
}

// TestTableGet_DryRunMirrorsExecuteSelector pins the dry-run get_cell_ranges
// body to the same selector field Execute will send. Agents validate the
// dry-run shape and then expect the live call to look the same — without this
// the selector field appeared in Execute but not in dry-run, so a request
// shape mismatch would have been invisible.
func TestTableGet_DryRunMirrorsExecuteSelector(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		args       []string
		wantField  string
		wantValue  string
		otherField string // must NOT be present
	}{
		{
			name:       "by --sheet-name",
			args:       []string{"--url", testURL, "--sheet-name", "销售"},
			wantField:  "sheet_name",
			wantValue:  "销售",
			otherField: "sheet_id",
		},
		{
			name:       "by --sheet-id",
			args:       []string{"--url", testURL, "--sheet-id", testSheetID},
			wantField:  "sheet_id",
			wantValue:  testSheetID,
			otherField: "sheet_name",
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			calls := parseDryRunAPI(t, TableGet, tt.args)
			var cellInput map[string]any
			for _, c := range calls {
				body, _ := c.(map[string]any)["body"].(map[string]any)
				if body == nil {
					continue
				}
				if tn, _ := body["tool_name"].(string); tn == "get_cell_ranges" {
					cellInput = decodeToolInput(t, body, "get_cell_ranges")
				}
			}
			if cellInput == nil {
				t.Fatalf("dry-run had no get_cell_ranges body; calls=%#v", calls)
			}
			if got := cellInput[tt.wantField]; got != tt.wantValue {
				t.Errorf("get_cell_ranges body %s = %#v, want %q", tt.wantField, got, tt.wantValue)
			}
			if _, has := cellInput[tt.otherField]; has {
				t.Errorf("get_cell_ranges body should not carry %s when only the other selector was given; got=%#v", tt.otherField, cellInput)
			}
		})
	}
}

// TestTableGet_AllSheets covers the "read every sheet" path (no --sheet-name):
// get_workbook_structure lists sheets, then each is read in order.
func TestTableGet_AllSheets(t *testing.T) {
	t.Parallel()
	structure := toolOutputStub(testToken, "read", `{"sheets":[{"sheet_id":"s1","sheet_name":"A","row_count":200,"column_count":20,"index":0},{"sheet_id":"s2","sheet_name":"B","row_count":200,"column_count":20,"index":1}]}`)
	regionA := toolOutputStub(testToken, "read", `{"current_region":"A1:A2"}`)
	cellsA := toolOutputStub(testToken, "read", `{"ranges":[{"cells":[[{"value":"项"}],[{"value":"x"}]]}]}`)
	regionB := toolOutputStub(testToken, "read", `{"current_region":"A1:A2"}`)
	cellsB := toolOutputStub(testToken, "read", `{"ranges":[{"cells":[[{"value":"项"}],[{"value":"y"}]]}]}`)
	out, err := runShortcutWithStubs(t, TableGet,
		[]string{"--url", testURL}, structure, regionA, cellsA, regionB, cellsB)
	if err != nil {
		t.Fatalf("execute failed: %v\nout=%s", err, out)
	}
	data := decodeEnvelopeData(t, out)
	sheets, _ := data["sheets"].([]interface{})
	if len(sheets) != 2 {
		t.Fatalf("want 2 sheets (all), got %d", len(sheets))
	}
	got := []string{
		sheets[0].(map[string]interface{})["name"].(string),
		sheets[1].(map[string]interface{})["name"].(string),
	}
	if got[0] != "A" || got[1] != "B" {
		t.Errorf("sheet names = %v, want [A B] in workbook order", got)
	}
}

// ─── +table-get: used-range probe (the pro016 / pro025 fix) ───────────

// anchorRecorderStub returns a BodyFilter-equipped reusable stub that records
// the `range` field of every get_range_as_csv request, so a test can assert the
// used-range probe spans the full physical grid rather than anchoring at A1.
func anchorRecorderStub(token, outputJSON string, anchors *[]string) *httpmock.Stub {
	s := toolOutputStub(token, "read", outputJSON)
	s.Reusable = true
	s.BodyFilter = func(body []byte) bool {
		var wire struct {
			ToolName string `json:"tool_name"`
			Input    string `json:"input"`
		}
		if err := json.Unmarshal(body, &wire); err != nil || wire.ToolName != "get_range_as_csv" {
			return false
		}
		var in struct {
			Range string `json:"range"`
		}
		_ = json.Unmarshal([]byte(wire.Input), &in)
		*anchors = append(*anchors, in.Range)
		return true
	}
	return s
}

// TestTableGet_DefaultProbesFullGrid is the core regression for the pro016 /
// pro025 incident: with no --range, the used-range probe must anchor over the
// whole physical grid (A1:<lastCol><lastRow>) — not A1 — so current_region spans
// internal blank rows/columns and the returned range is the true used range.
func TestTableGet_DefaultProbesFullGrid(t *testing.T) {
	t.Parallel()
	// grid is 200×20 → last column index 19 → "T". The data spans an internal
	// blank row (row 6) and blank column (D), so the true used range is A1:F10.
	structure := toolOutputStub(testToken, "read", `{"sheets":[{"sheet_id":"`+testSheetID+`","sheet_name":"S","row_count":200,"column_count":20,"index":0}]}`)
	var anchors []string
	region := anchorRecorderStub(testToken, `{"current_region":"A1:F10"}`, &anchors)
	cells := toolOutputStub(testToken, "read", `{"ranges":[{"cells":[[{"value":"h"}],[{"value":"x"}]]}]}`)
	out, err := runShortcutWithStubs(t, TableGet,
		[]string{"--url", testURL, "--sheet-name", "S"}, structure, region, cells)
	if err != nil {
		t.Fatalf("execute failed: %v\nout=%s", err, out)
	}
	if len(anchors) != 1 || anchors[0] != "A1:T200" {
		t.Fatalf("used-range probe anchor = %v, want one [A1:T200] (full grid, not A1)", anchors)
	}
	data := decodeEnvelopeData(t, out)
	s0, _ := data["sheets"].([]interface{})[0].(map[string]interface{})
	if s0["range"] != "A1:F10" {
		t.Errorf("output range = %v, want A1:F10 (true used range reported back)", s0["range"])
	}
}

// TestTableGet_UnknownDimsFallBackToA1 locks the fail-open path: when the
// structure read doesn't report row_count / column_count, the probe degrades to
// the legacy A1 anchor rather than building a bogus range or erroring.
func TestTableGet_UnknownDimsFallBackToA1(t *testing.T) {
	t.Parallel()
	structure := toolOutputStub(testToken, "read", `{"sheets":[{"sheet_id":"`+testSheetID+`","sheet_name":"S","index":0}]}`) // no row/col counts
	var anchors []string
	region := anchorRecorderStub(testToken, `{"current_region":"A1:B2"}`, &anchors)
	cells := toolOutputStub(testToken, "read", `{"ranges":[{"cells":[[{"value":"h"}],[{"value":"x"}]]}]}`)
	out, err := runShortcutWithStubs(t, TableGet,
		[]string{"--url", testURL, "--sheet-name", "S"}, structure, region, cells)
	if err != nil {
		t.Fatalf("execute failed: %v\nout=%s", err, out)
	}
	if len(anchors) != 1 || anchors[0] != "A1" {
		t.Fatalf("anchor = %v, want one [A1] (fallback when dims unknown)", anchors)
	}
}

// TestTableGet_SheetIDBackfillsName: when only --sheet-id is given, the structure
// read backfills the sheet's display name into the output spec (previously left
// empty), a free win from now always reading the structure.
func TestTableGet_SheetIDBackfillsName(t *testing.T) {
	t.Parallel()
	structure := toolOutputStub(testToken, "read", `{"sheets":[{"sheet_id":"`+testSheetID+`","sheet_name":"销售","row_count":200,"column_count":20,"index":0}]}`)
	region := toolOutputStub(testToken, "read", `{"current_region":"A1:A2"}`)
	cells := toolOutputStub(testToken, "read", `{"ranges":[{"cells":[[{"value":"h"}],[{"value":"x"}]]}]}`)
	out, err := runShortcutWithStubs(t, TableGet,
		[]string{"--url", testURL, "--sheet-id", testSheetID}, structure, region, cells)
	if err != nil {
		t.Fatalf("execute failed: %v\nout=%s", err, out)
	}
	data := decodeEnvelopeData(t, out)
	s0, _ := data["sheets"].([]interface{})[0].(map[string]interface{})
	if s0["name"] != "销售" {
		t.Errorf("name = %v, want 销售 (backfilled from structure)", s0["name"])
	}
}

// TestTableGet_EmptySheetReportsEmptyRange: an empty sheet still carries a range
// key (empty string) so callers can rely on the field always being present.
func TestTableGet_EmptySheetReportsEmptyRange(t *testing.T) {
	t.Parallel()
	structure := toolOutputStub(testToken, "read", `{"sheets":[{"sheet_id":"`+testSheetID+`","sheet_name":"S","row_count":200,"column_count":20,"index":0}]}`)
	region := toolOutputStub(testToken, "read", `{}`) // no current_region → empty sheet
	out, err := runShortcutWithStubs(t, TableGet,
		[]string{"--url", testURL, "--sheet-name", "S"}, structure, region)
	if err != nil {
		t.Fatalf("execute failed: %v\nout=%s", err, out)
	}
	data := decodeEnvelopeData(t, out)
	s0, _ := data["sheets"].([]interface{})[0].(map[string]interface{})
	if r, has := s0["range"]; !has || r != "" {
		t.Errorf("empty sheet range = %#v (has=%v), want \"\"", r, has)
	}
}

// TestTablePut_AppendProbesFullGrid is the append-side mirror of the table-get
// fix: lastDataRow must anchor its current_region probe over the whole grid so an
// internal blank row doesn't make append land on top of (overwrite) later data.
func TestTablePut_AppendProbesFullGrid(t *testing.T) {
	t.Parallel()
	// Existing data really ends at row 10 (a blank row 6 sits in the middle).
	// Under an A1 anchor current_region would stop at row 5 and append would
	// write to A6 — clobbering rows 7-10. The full-grid anchor reports A1:B10.
	structure := toolOutputStub(testToken, "read", `{"sheets":[{"sheet_id":"`+testSheetID+`","sheet_name":"日志","row_count":200,"column_count":20,"index":0}]}`)
	var anchors []string
	region := anchorRecorderStub(testToken, `{"current_region":"A1:B10"}`, &anchors)
	write := toolOutputStub(testToken, "write", `{"ok":true}`)
	out, err := runShortcutWithStubs(t, TablePut,
		[]string{"--url", testURL, "--sheets",
			`{"sheets":[{"name":"日志","mode":"append","columns":["时间","值"],"dtypes":{"值":"int64"},"data":[["t1",1]]}]}`},
		structure, region, write)
	if err != nil {
		t.Fatalf("execute failed: %v\nout=%s", err, out)
	}
	if len(anchors) != 1 || anchors[0] != "A1:T200" {
		t.Fatalf("append last-data-row probe anchor = %v, want one [A1:T200]", anchors)
	}
	var wire map[string]interface{}
	if err := json.Unmarshal(write.CapturedBody, &wire); err != nil {
		t.Fatalf("decode captured write body: %v", err)
	}
	var input map[string]interface{}
	if err := json.Unmarshal([]byte(wire["input"].(string)), &input); err != nil {
		t.Fatalf("decode tool input: %v", err)
	}
	if input["range"] != "A11:B11" {
		t.Errorf("append range = %v, want A11:B11 (below true last row 10, not on top of data past the blank row)", input["range"])
	}
}

// TestBuildTypedCell_TypeLess verifies a type-less column (Type == "") writes the
// raw scalar unchanged — no @ text format, json.Number preserved — so untyped
// --values lets the backend auto-detect types. An explicit format still attaches.
func TestBuildTypedCell_TypeLess(t *testing.T) {
	t.Parallel()
	num := json.Number("145487")
	pct := json.Number("0.1")
	cases := []struct {
		name    string
		col     tableColumnSpec
		raw     interface{}
		wantVal interface{}
		wantNF  interface{}
	}{
		{"number stays json.Number", tableColumnSpec{Name: "c"}, num, num, nil},
		{"string verbatim", tableColumnSpec{Name: "c"}, "00123", "00123", nil},
		{"bool verbatim", tableColumnSpec{Name: "c"}, true, true, nil},
		{"nil → empty cell", tableColumnSpec{Name: "c"}, nil, nil, nil},
		{"explicit format attaches", tableColumnSpec{Name: "c", Format: "0.0%"}, pct, pct, "0.0%"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cell, err := buildTypedCell(&tc.col, tc.raw)
			if err != nil {
				t.Fatalf("buildTypedCell err: %v", err)
			}
			if cell["value"] != tc.wantVal {
				t.Errorf("value = %#v, want %#v", cell["value"], tc.wantVal)
			}
			var gotNF interface{}
			if cs, _ := cell["cell_styles"].(map[string]interface{}); cs != nil {
				gotNF = cs["number_format"]
			}
			if gotNF != tc.wantNF {
				t.Errorf("number_format = %#v, want %#v", gotNF, tc.wantNF)
			}
		})
	}
}

// TestValidColumnType_AcceptsEmpty locks that an empty type is valid — the
// type-less / raw-passthrough column that --values synthesizes.
func TestValidColumnType_AcceptsEmpty(t *testing.T) {
	t.Parallel()
	for _, ty := range []string{"", "string", "number", "date", "bool"} {
		if !validColumnType(ty) {
			t.Errorf("validColumnType(%q) = false, want true", ty)
		}
	}
	if validColumnType("float") {
		t.Error(`validColumnType("float") = true, want false`)
	}
}
