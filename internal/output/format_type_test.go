// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package output

import (
	"testing"

	"github.com/larksuite/cli/errs"
)

func TestParseFormat(t *testing.T) {
	tests := []struct {
		input  string
		want   Format
		wantOK bool
	}{
		{"json", FormatJSON, true},
		{"JSON", FormatJSON, true},
		{"Json", FormatJSON, true},
		{"ndjson", FormatNDJSON, true},
		{"NDJSON", FormatNDJSON, true},
		{"Ndjson", FormatNDJSON, true},
		{"table", FormatTable, true},
		{"TABLE", FormatTable, true},
		{"Table", FormatTable, true},
		{"csv", FormatCSV, true},
		{"CSV", FormatCSV, true},
		{"Csv", FormatCSV, true},
		{"pretty", FormatPretty, true},
		{"PRETTY", FormatPretty, true},
		{"Pretty", FormatPretty, true},
		{"", FormatJSON, true},
		// Legacy/unknown values fall back to JSON with ok=false
		{"data", FormatJSON, false},
		{"raw", FormatJSON, false},
		{"RAW", FormatJSON, false},
		{"DATA", FormatJSON, false},
		{"foobar", FormatJSON, false},
		{"xml", FormatJSON, false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, ok := ParseFormat(tt.input)
			if got != tt.want {
				t.Errorf("ParseFormat(%q) format = %v, want %v", tt.input, got, tt.want)
			}
			if ok != tt.wantOK {
				t.Errorf("ParseFormat(%q) ok = %v, want %v", tt.input, ok, tt.wantOK)
			}
		})
	}
}

func TestFormatString(t *testing.T) {
	tests := []struct {
		format Format
		want   string
	}{
		{FormatJSON, "json"},
		{FormatNDJSON, "ndjson"},
		{FormatTable, "table"},
		{FormatCSV, "csv"},
		{FormatPretty, "pretty"},
		{Format(99), "unknown(99)"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := tt.format.String()
			if got != tt.want {
				t.Errorf("Format(%d).String() = %q, want %q", tt.format, got, tt.want)
			}
		})
	}
}

func TestFormatValid(t *testing.T) {
	for _, format := range []Format{FormatJSON, FormatNDJSON, FormatTable, FormatCSV, FormatPretty} {
		if !format.Valid() {
			t.Errorf("Format(%d).Valid() = false, want true", format)
		}
	}
	if Format(99).Valid() {
		t.Error("Format(99).Valid() = true, want false")
	}
}

func TestParseFormatStrict(t *testing.T) {
	valid := []struct {
		input string
		want  Format
	}{
		{"", FormatJSON},
		{"json", FormatJSON},
		{"JSON", FormatJSON},
		{"ndjson", FormatNDJSON},
		{"table", FormatTable},
		{"csv", FormatCSV},
		{"pretty", FormatPretty},
		{"Pretty", FormatPretty},
	}
	for _, tt := range valid {
		t.Run("valid/"+tt.input, func(t *testing.T) {
			got, err := ParseFormatStrict(tt.input)
			if err != nil {
				t.Fatalf("ParseFormatStrict(%q) error = %v, want nil", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("ParseFormatStrict(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}

	// Unknown values are a typed validation error on --format, never a silent
	// fallback to JSON.
	for _, input := range []string{"yaml", "xml", "data", "raw", "tabel"} {
		t.Run("unknown/"+input, func(t *testing.T) {
			got, err := ParseFormatStrict(input)
			if err == nil {
				t.Fatalf("ParseFormatStrict(%q) error = nil, want validation error", input)
			}
			problem, ok := errs.ProblemOf(err)
			if !ok || problem.Category != errs.CategoryValidation {
				t.Fatalf("ParseFormatStrict(%q) problem = %#v, %v; want validation category", input, problem, ok)
			}
			if got != FormatJSON {
				t.Errorf("ParseFormatStrict(%q) format = %v, want FormatJSON sentinel", input, got)
			}
		})
	}
}
