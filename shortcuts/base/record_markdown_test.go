// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"fmt"
	"strings"
	"testing"
)

func TestRenderRecordMarkdownEmptyResult(t *testing.T) {
	got, err := renderRecordMarkdown(map[string]interface{}{
		"fields":         []interface{}{"Name", "Age"},
		"record_id_list": []interface{}{},
		"data":           []interface{}{},
		"has_more":       false,
	})
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	for _, want := range []string{
		"| _record_id | Name | Age |",
		"Meta: count=0; has_more=false",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q:\n%s", want, got)
		}
	}
}

func TestRenderRecordMarkdownEscapesTableCells(t *testing.T) {
	got, err := renderRecordMarkdown(map[string]interface{}{
		"fields":         []interface{}{"Name|Label", "Note"},
		"record_id_list": []interface{}{"rec_1"},
		"data":           []interface{}{[]interface{}{"A|B", "line1\nline2"}},
	})
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	for _, want := range []string{
		"| _record_id | Name\\|Label | Note |",
		"| rec_1 | A\\|B | line1<br>line2 |",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q:\n%s", want, got)
		}
	}
}

func TestRenderRecordMarkdownTruncatesIgnoredFields(t *testing.T) {
	ignored := make([]interface{}, maxRecordMarkdownIgnoredFields+2)
	for i := range ignored {
		ignored[i] = fmt.Sprintf("Field%d", i+1)
	}
	got, err := renderRecordMarkdown(map[string]interface{}{
		"fields":         []interface{}{"Name"},
		"record_id_list": []interface{}{"rec_1"},
		"data":           []interface{}{[]interface{}{"Alice"}},
		"ignored_fields": ignored,
	})
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(got, fmt.Sprintf("ignored_fields=%d", len(ignored))) ||
		!strings.Contains(got, fmt.Sprintf("...(%d total)", len(ignored))) ||
		strings.Contains(got, "Field22") {
		t.Fatalf("ignored field truncation mismatch:\n%s", got)
	}
}
