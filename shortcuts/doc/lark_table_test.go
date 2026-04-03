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
	if !strings.Contains(result, "| A | B |") {
		t.Errorf("expected data row, got:\n%s", result)
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

func TestNormalizeLarkTables_UnevenColumns(t *testing.T) {
	input := `<lark-table><lark-tr><lark-td>A</lark-td><lark-td>B</lark-td></lark-tr><lark-tr><lark-td>C</lark-td></lark-tr></lark-table>`
	result := NormalizeLarkTables(input)
	if !strings.Contains(result, "| C |  |") {
		t.Errorf("expected row padding for uneven columns, got:\n%s", result)
	}
}

func TestNormalizeLarkTables_FlattenGrid(t *testing.T) {
	input := `<lark-table><lark-tr><lark-td><grid><column>Col 1</column><column>Col 2</column></grid></lark-td></lark-tr></lark-table>`
	result := NormalizeLarkTables(input)
	// Grid columns should be joined with newlines, which in GFM table cells become <br>
	if !strings.Contains(result, "Col 1<br>Col 2") {
		t.Errorf("expected flattened grid with <br>, got:\n%s", result)
	}
}

func TestNormalizeLarkTables_StripTextTags(t *testing.T) {
	input := `<lark-table><lark-tr><lark-td><text color="red">Red Text</text></lark-td></lark-tr></lark-table>`
	result := NormalizeLarkTables(input)
	if !strings.Contains(result, "| Red Text |") {
		t.Errorf("expected text tag to be stripped, got:\n%s", result)
	}
}

func TestNormalizeLarkTables_BrTag(t *testing.T) {
	input := `<lark-table><lark-tr><lark-td>Line 1<br/>Line 2</lark-td></lark-tr></lark-table>`
	result := NormalizeLarkTables(input)
	if !strings.Contains(result, "Line 1<br>Line 2") {
		t.Errorf("expected <br/> to be normalized to <br>, got:\n%s", result)
	}
}

func TestNormalizeLarkTables_NoTable(t *testing.T) {
	input := "# Hello\n\nJust some text."
	result := NormalizeLarkTables(input)
	if result != input {
		t.Errorf("should not modify text without lark-table")
	}
}
