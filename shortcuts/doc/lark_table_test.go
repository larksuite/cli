// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"strings"
	"testing"
)

func TestNormalizeLarkTables_Basic(t *testing.T) {
	input := `<lark-table><lark-tr><lark-td>Name</lark-td><lark-td>Age</lark-td></lark-tr><lark-tr><lark-td>Alice</lark-td><lark-td>30</lark-td></lark-tr></lark-table>`

	result := NormalizeLarkTables(input)

	if !strings.Contains(result, "| Name | Age |") {
		t.Errorf("expected GFM header row, got:\n%s", result)
	}
	if !strings.Contains(result, "| --- | --- |") {
		t.Errorf("expected separator row, got:\n%s", result)
	}
	if !strings.Contains(result, "| Alice | 30 |") {
		t.Errorf("expected data row, got:\n%s", result)
	}
}

func TestNormalizeLarkTables_NoHeaderRow(t *testing.T) {
	input := `<lark-table header-row="false"><lark-tr><lark-td>A</lark-td><lark-td>B</lark-td></lark-tr></lark-table>`

	result := NormalizeLarkTables(input)

	if strings.Contains(result, "| --- |") {
		t.Errorf("should not have separator when header-row=false, got:\n%s", result)
	}
}

func TestNormalizeLarkTables_PreservesCodeBlocks(t *testing.T) {
	input := "<lark-table><lark-tr><lark-td>```go\nfmt.Println()\n```</lark-td></lark-tr></lark-table>"

	result := NormalizeLarkTables(input)

	if !strings.Contains(result, "<lark-table>") {
		t.Errorf("should preserve original lark-table when cells contain fenced code, got:\n%s", result)
	}
}

func TestNormalizeLarkTables_PipeEscape(t *testing.T) {
	input := `<lark-table><lark-tr><lark-td>A|B</lark-td><lark-td>C</lark-td></lark-tr></lark-table>`

	result := NormalizeLarkTables(input)

	if !strings.Contains(result, `A\|B`) {
		t.Errorf("pipe in cell should be escaped, got:\n%s", result)
	}
}

func TestNormalizeLarkTables_NoTable(t *testing.T) {
	input := "# Hello\n\nJust some text."
	result := NormalizeLarkTables(input)
	if result != input {
		t.Errorf("should not modify text without lark-table")
	}
}
