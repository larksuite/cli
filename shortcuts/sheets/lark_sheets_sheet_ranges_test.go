// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/larksuite/cli/shortcuts/common"
	"github.com/spf13/cobra"
)

func mustMarshalSheetsDryRun(t *testing.T, v interface{}) string {
	t.Helper()

	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	return string(b)
}

func newSheetsTestRuntime(t *testing.T, stringFlags map[string]string, boolFlags map[string]bool) *common.RuntimeContext {
	t.Helper()

	cmd := &cobra.Command{Use: "test"}
	for name := range stringFlags {
		cmd.Flags().String(name, "", "")
	}
	for name := range boolFlags {
		cmd.Flags().Bool(name, false, "")
	}
	if err := cmd.ParseFlags(nil); err != nil {
		t.Fatalf("ParseFlags() error = %v", err)
	}
	for name, value := range stringFlags {
		if err := cmd.Flags().Set(name, value); err != nil {
			t.Fatalf("Flags().Set(%q) error = %v", name, err)
		}
	}
	for name, value := range boolFlags {
		if err := cmd.Flags().Set(name, map[bool]string{true: "true", false: "false"}[value]); err != nil {
			t.Fatalf("Flags().Set(%q) error = %v", name, err)
		}
	}
	return &common.RuntimeContext{Cmd: cmd}
}

func TestNormalizeSheetRangeSeparators(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "standard", input: "sheet_123!A1:B2", want: "sheet_123!A1:B2"},
		{name: "escaped ascii", input: `sheet_123\!A1:B2`, want: "sheet_123!A1:B2"},
		{name: "fullwidth", input: "sheet_123！A1:B2", want: "sheet_123!A1:B2"},
		{name: "escaped fullwidth", input: `sheet_123\！A1:B2`, want: "sheet_123!A1:B2"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := normalizeSheetRangeSeparators(tt.input); got != tt.want {
				t.Fatalf("normalizeSheetRangeSeparators(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestValidateSheetRangeInputAcceptsEscapedSeparator(t *testing.T) {
	t.Parallel()

	if err := validateSheetRangeInput("", `sheet_123\！A1:B2`); err != nil {
		t.Fatalf("validateSheetRangeInput() error = %v, want nil", err)
	}
}

func TestSheetReadDryRunNormalizesEscapedSeparator(t *testing.T) {
	t.Parallel()

	runtime := newSheetsTestRuntime(t, map[string]string{
		"spreadsheet-token": "sht_test",
		"range":             `sheet_123\！A1`,
		"sheet-id":          "",
	}, nil)

	got := mustMarshalSheetsDryRun(t, SheetRead.DryRun(context.Background(), runtime))
	if !strings.Contains(got, `"range":"sheet_123!A1:A1"`) {
		t.Fatalf("SheetRead.DryRun() = %s, want normalized escaped separator", got)
	}
}

func TestSheetWriteDryRunNormalizesEscapedSeparator(t *testing.T) {
	t.Parallel()

	runtime := newSheetsTestRuntime(t, map[string]string{
		"spreadsheet-token": "sht_test",
		"range":             `sheet_123\！A1:B2`,
		"values":            `[[1,2],[3,4]]`,
	}, nil)

	got := mustMarshalSheetsDryRun(t, SheetWrite.DryRun(context.Background(), runtime))
	if !strings.Contains(got, `"range":"sheet_123!A1:B2"`) {
		t.Fatalf("SheetWrite.DryRun() = %s, want normalized escaped separator", got)
	}
}

func TestSheetAppendDryRunNormalizesEscapedSeparator(t *testing.T) {
	t.Parallel()

	runtime := newSheetsTestRuntime(t, map[string]string{
		"spreadsheet-token": "sht_test",
		"range":             `sheet_123\！A1:B2`,
		"values":            `[["foo","bar"]]`,
	}, nil)

	got := mustMarshalSheetsDryRun(t, SheetAppend.DryRun(context.Background(), runtime))
	if !strings.Contains(got, `"range":"sheet_123!A1:B2"`) {
		t.Fatalf("SheetAppend.DryRun() = %s, want normalized escaped separator", got)
	}
}

func TestSheetFindDryRunNormalizesEscapedSeparator(t *testing.T) {
	t.Parallel()

	runtime := newSheetsTestRuntime(t, map[string]string{
		"spreadsheet-token": "sht_test",
		"sheet-id":          "sheet_123",
		"find":              "target",
		"range":             `sheet_123\！A1:B2`,
	}, map[string]bool{
		"ignore-case":       false,
		"match-entire-cell": false,
		"search-by-regex":   false,
		"include-formulas":  false,
	})

	got := mustMarshalSheetsDryRun(t, SheetFind.DryRun(context.Background(), runtime))
	if !strings.Contains(got, `"range":"sheet_123!A1:B2"`) {
		t.Fatalf("SheetFind.DryRun() = %s, want normalized escaped separator", got)
	}
}

func TestSheetFindValidateMismatchedRangeSheetID(t *testing.T) {
	t.Parallel()

	rt := newSheetsTestRuntime(t, map[string]string{
		"url": "", "spreadsheet-token": "sht1", "sheet-id": "sheet1", "find": "target",
		"range": "sheet2!A1:B2",
	}, map[string]bool{
		"ignore-case": false, "match-entire-cell": false, "search-by-regex": false, "include-formulas": false,
	})
	err := SheetFind.Validate(context.Background(), rt)
	if err == nil || !strings.Contains(err.Error(), "does not match --sheet-id") {
		t.Fatalf("expected mismatch error, got: %v", err)
	}
}

func TestCellDataValidateRejectsURLAndTokenTogether(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		shortcut  common.Shortcut
		strFlags  map[string]string
		boolFlags map[string]bool
	}{
		{
			name:     "read",
			shortcut: SheetRead,
			strFlags: map[string]string{"url": "https://example.feishu.cn/sheets/shtFromURL", "spreadsheet-token": "shtTOKEN"},
		},
		{
			name:     "write",
			shortcut: SheetWrite,
			strFlags: map[string]string{"url": "https://example.feishu.cn/sheets/shtFromURL", "spreadsheet-token": "shtTOKEN", "values": `[[1]]`},
		},
		{
			name:     "append",
			shortcut: SheetAppend,
			strFlags: map[string]string{"url": "https://example.feishu.cn/sheets/shtFromURL", "spreadsheet-token": "shtTOKEN", "values": `[[1]]`},
		},
		{
			name:      "find",
			shortcut:  SheetFind,
			strFlags:  map[string]string{"url": "https://example.feishu.cn/sheets/shtFromURL", "spreadsheet-token": "shtTOKEN", "sheet-id": "sheet1", "find": "x"},
			boolFlags: map[string]bool{"ignore-case": false, "match-entire-cell": false, "search-by-regex": false, "include-formulas": false},
		},
		{
			name:      "replace",
			shortcut:  SheetReplace,
			strFlags:  map[string]string{"url": "https://example.feishu.cn/sheets/shtFromURL", "spreadsheet-token": "shtTOKEN", "sheet-id": "sheet1", "find": "a", "replacement": "b"},
			boolFlags: map[string]bool{"match-case": false, "match-entire-cell": false, "search-by-regex": false, "include-formulas": false},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			rt := newSheetsTestRuntime(t, tt.strFlags, tt.boolFlags)
			err := tt.shortcut.Validate(context.Background(), rt)
			if err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
				t.Fatalf("expected mutual exclusivity error, got: %v", err)
			}
		})
	}
}

func TestCellDataValidateRejectsInvalidSpreadsheetURL(t *testing.T) {
	t.Parallel()

	rt := newSheetsTestRuntime(t, map[string]string{
		"url":               "https://example.feishu.cn/docx/doxcnNotSheet",
		"spreadsheet-token": "",
	}, nil)
	err := SheetRead.Validate(context.Background(), rt)
	if err == nil || !strings.Contains(err.Error(), "spreadsheet URL") {
		t.Fatalf("expected invalid spreadsheet URL error, got: %v", err)
	}
}

func TestCellDataValidateRejectsNon2DValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		shortcut common.Shortcut
		strFlags map[string]string
	}{
		{
			name:     "write 1d array",
			shortcut: SheetWrite,
			strFlags: map[string]string{"spreadsheet-token": "sht1", "values": `[1,2]`},
		},
		{
			name:     "write object",
			shortcut: SheetWrite,
			strFlags: map[string]string{"spreadsheet-token": "sht1", "values": `{"a":1}`},
		},
		{
			name:     "append string",
			shortcut: SheetAppend,
			strFlags: map[string]string{"spreadsheet-token": "sht1", "values": `"x"`},
		},
		{
			name:     "append null",
			shortcut: SheetAppend,
			strFlags: map[string]string{"spreadsheet-token": "sht1", "values": `null`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			rt := newSheetsTestRuntime(t, tt.strFlags, nil)
			err := tt.shortcut.Validate(context.Background(), rt)
			if err == nil || !strings.Contains(err.Error(), "must be a 2D array") {
				t.Fatalf("expected 2D-array validation error, got: %v", err)
			}
		})
	}
}
