// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package output

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestFormatValue_JSON(t *testing.T) {
	data := map[string]interface{}{"name": "Alice"}

	var buf bytes.Buffer
	FormatValue(&buf, data, FormatJSON)
	out := buf.String()

	// Should be pretty-printed JSON
	if !strings.Contains(out, `"name"`) {
		t.Errorf("JSON output should contain field name, got:\n%s", out)
	}
	if !strings.Contains(out, "Alice") {
		t.Errorf("JSON output should contain value, got:\n%s", out)
	}
}

func TestFormatValue_NDJSON(t *testing.T) {
	data := map[string]interface{}{
		"data": map[string]interface{}{
			"items": []interface{}{
				map[string]interface{}{"id": float64(1)},
				map[string]interface{}{"id": float64(2)},
			},
		},
	}

	var buf bytes.Buffer
	FormatValue(&buf, data, FormatNDJSON)
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")

	if len(lines) != 2 {
		t.Fatalf("NDJSON should output 2 lines, got %d:\n%s", len(lines), buf.String())
	}

	for _, line := range lines {
		var obj map[string]interface{}
		if err := json.Unmarshal([]byte(line), &obj); err != nil {
			t.Errorf("each NDJSON line should be valid JSON: %s", line)
		}
	}
}

func TestFormatValue_Table(t *testing.T) {
	data := map[string]interface{}{
		"data": map[string]interface{}{
			"items": []interface{}{
				map[string]interface{}{"name": "Alice"},
			},
		},
	}

	var buf bytes.Buffer
	FormatValue(&buf, data, FormatTable)
	out := buf.String()

	if !strings.Contains(out, "name") {
		t.Errorf("table output should contain 'name' header, got:\n%s", out)
	}
	if !strings.Contains(out, "Alice") {
		t.Errorf("table output should contain 'Alice', got:\n%s", out)
	}
}

func TestFormatValue_CSV(t *testing.T) {
	data := map[string]interface{}{
		"data": map[string]interface{}{
			"items": []interface{}{
				map[string]interface{}{"name": "Alice"},
			},
		},
	}

	var buf bytes.Buffer
	FormatValue(&buf, data, FormatCSV)
	out := buf.String()
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")

	if len(lines) != 2 {
		t.Fatalf("CSV should have header + 1 row, got %d lines:\n%s", len(lines), out)
	}
	if lines[0] != "name" {
		t.Errorf("CSV header should be 'name', got: %s", lines[0])
	}
	if lines[1] != "Alice" {
		t.Errorf("CSV row should be 'Alice', got: %s", lines[1])
	}
}

func TestPaginatedFormatter_JSON(t *testing.T) {
	var buf bytes.Buffer
	pf := NewPaginatedFormatter(&buf, FormatJSON)

	pf.FormatPage([]interface{}{
		map[string]interface{}{"id": float64(1)},
		map[string]interface{}{"id": float64(2)},
	})
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != 2 {
		t.Errorf("paginated JSON should emit 2 lines (NDJSON), got %d:\n%s", len(lines), buf.String())
	}
}

func TestPaginatedFormatter_NDJSON(t *testing.T) {
	var buf bytes.Buffer
	pf := NewPaginatedFormatter(&buf, FormatNDJSON)

	pf.FormatPage([]interface{}{map[string]interface{}{"id": float64(1)}})
	out := strings.TrimSpace(buf.String())

	var obj map[string]interface{}
	if err := json.Unmarshal([]byte(out), &obj); err != nil {
		t.Errorf("NDJSON paginated output should be valid JSON: %s", out)
	}
}

func TestPaginatedFormatter_Table(t *testing.T) {
	var buf bytes.Buffer
	pf := NewPaginatedFormatter(&buf, FormatTable)

	page1 := []interface{}{map[string]interface{}{"name": "Alice"}}
	page2 := []interface{}{map[string]interface{}{"name": "Bob"}}

	pf.FormatPage(page1)
	out1 := buf.String()
	if !strings.Contains(out1, "─") {
		t.Error("first table page should contain separator")
	}

	buf.Reset()
	pf.FormatPage(page2)
	out2 := buf.String()
	if strings.Contains(out2, "─") {
		t.Error("continuation table page should not contain separator")
	}
	if !strings.Contains(out2, "Bob") {
		t.Error("continuation table page should contain data")
	}
}

func TestPaginatedFormatter_CSV(t *testing.T) {
	var buf bytes.Buffer
	pf := NewPaginatedFormatter(&buf, FormatCSV)

	page1 := []interface{}{map[string]interface{}{"name": "Alice"}}
	page2 := []interface{}{map[string]interface{}{"name": "Bob"}}

	pf.FormatPage(page1)
	lines1 := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines1) != 2 {
		t.Errorf("first CSV page should have header + data, got %d lines", len(lines1))
	}

	buf.Reset()
	pf.FormatPage(page2)
	lines2 := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines2) != 1 {
		t.Errorf("continuation CSV page should have only data, got %d lines", len(lines2))
	}
}

func TestPaginatedFormatter_ColumnConsistency(t *testing.T) {
	// Page 1 has {a, b}, page 2 has {a, b, c} — c should be ignored in CSV
	var buf bytes.Buffer
	pf := NewPaginatedFormatter(&buf, FormatCSV)

	pf.FormatPage([]interface{}{map[string]interface{}{"a": "1", "b": "2"}})
	header := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")[0]

	buf.Reset()
	pf.FormatPage([]interface{}{map[string]interface{}{"a": "3", "b": "4", "c": "5"}})
	dataLine := strings.TrimRight(buf.String(), "\n")

	// Header and data should have same number of columns
	headerCols := strings.Count(header, ",") + 1
	dataCols := strings.Count(dataLine, ",") + 1
	if headerCols != dataCols {
		t.Errorf("column count mismatch: header has %d, data has %d\nheader: %s\ndata: %s",
			headerCols, dataCols, header, dataLine)
	}
}

func TestExtractItems(t *testing.T) {
	// Standard Lark response
	data := map[string]interface{}{
		"code": float64(0),
		"msg":  "success",
		"data": map[string]interface{}{
			"items": []interface{}{
				map[string]interface{}{"id": float64(1)},
				map[string]interface{}{"id": float64(2)},
			},
			"has_more":   true,
			"page_token": "abc",
		},
	}

	items := ExtractItems(data)
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}

	// Different array field
	data2 := map[string]interface{}{
		"data": map[string]interface{}{
			"members": []interface{}{
				map[string]interface{}{"user_id": "u1"},
			},
		},
	}

	items2 := ExtractItems(data2)
	if len(items2) != 1 {
		t.Fatalf("expected 1 member, got %d", len(items2))
	}

	// Already an array
	arr := []interface{}{"a", "b"}
	items3 := ExtractItems(arr)
	if len(items3) != 2 {
		t.Fatalf("expected 2 items from raw array, got %d", len(items3))
	}

	// Non-response
	items4 := ExtractItems("string")
	if items4 != nil {
		t.Fatalf("expected nil for non-response, got %v", items4)
	}

	// No data field and no array field
	items5 := ExtractItems(map[string]interface{}{"foo": "bar"})
	if items5 != nil {
		t.Fatalf("expected nil for no data/array field, got %v", items5)
	}

	// Direct map with array field (shortcut data like {"members":[…], "total":5})
	directMap := map[string]interface{}{
		"members":    []interface{}{map[string]interface{}{"name": "Alice"}},
		"total":      float64(1),
		"has_more":   false,
		"page_token": "",
	}
	items6 := ExtractItems(directMap)
	if len(items6) != 1 {
		t.Fatalf("expected 1 item from direct map, got %d", len(items6))
	}

	// Direct map — plain array passed directly (e.g. calendar freebusy items)
	plainArr := []interface{}{
		map[string]interface{}{"start": "10:00", "end": "11:00"},
	}
	items7 := ExtractItems(plainArr)
	if len(items7) != 1 {
		t.Fatalf("expected 1 item from plain array, got %d", len(items7))
	}
}

// --- Typed-slice regression tests ---
// These cover the scenario where shortcut code uses []map[string]interface{}
// (or other typed slices) instead of []interface{} in outData.

func TestExtractItems_TypedMapSlice(t *testing.T) {
	// Simulates shortcut pattern: outData["chats"] = []map[string]interface{}{...}
	data := map[string]interface{}{
		"chats": []map[string]interface{}{
			{"chat_id": "oc_abc", "name": "Test Chat"},
			{"chat_id": "oc_def", "name": "Dev Chat"},
		},
		"total":    2,
		"has_more": false,
	}
	items := ExtractItems(data)
	if len(items) != 2 {
		t.Fatalf("expected 2 items from typed map slice, got %d", len(items))
	}
	// Verify elements are still map[string]interface{} (flattenItem can handle them)
	for i, item := range items {
		if _, ok := item.(map[string]interface{}); !ok {
			t.Errorf("item[%d] should be map[string]interface{}, got %T", i, item)
		}
	}
}

func TestExtractItems_TypedMapSlice_InEnvelope(t *testing.T) {
	// Typed slice inside a Lark API envelope: result["data"]["items"] = []map[string]interface{}{...}
	data := map[string]interface{}{
		"code": float64(0),
		"data": map[string]interface{}{
			"items": []map[string]interface{}{
				{"id": "1", "name": "Alice"},
			},
			"has_more": false,
		},
	}
	items := ExtractItems(data)
	if len(items) != 1 {
		t.Fatalf("expected 1 item from typed slice in envelope, got %d", len(items))
	}
}

func TestFormatValue_Table_TypedMapSlice(t *testing.T) {
	// The core bug: --format table with []map[string]interface{} should render
	// multi-column table, not a key-value two-column fallback.
	data := map[string]interface{}{
		"chats": []map[string]interface{}{
			{"chat_id": "oc_abc", "name": "Lark Dev"},
		},
		"total":    1,
		"has_more": false,
	}

	var buf bytes.Buffer
	FormatValue(&buf, data, FormatTable)
	out := buf.String()

	// Should have column headers from the data fields
	if !strings.Contains(out, "chat_id") {
		t.Errorf("table should contain 'chat_id' column header, got:\n%s", out)
	}
	if !strings.Contains(out, "name") {
		t.Errorf("table should contain 'name' column header, got:\n%s", out)
	}
	if !strings.Contains(out, "Lark Dev") {
		t.Errorf("table should contain data value 'Lark Dev', got:\n%s", out)
	}
	// Should NOT render as key-value fallback (metadata as rows)
	if strings.Contains(out, "has_more") {
		t.Errorf("table should not contain metadata 'has_more' as a row, got:\n%s", out)
	}
}

func TestFormatValue_CSV_TypedMapSlice(t *testing.T) {
	data := map[string]interface{}{
		"messages": []map[string]interface{}{
			{"message_id": "om_abc", "content": "hello"},
			{"message_id": "om_def", "content": "world"},
		},
		"total": 2,
	}

	var buf bytes.Buffer
	FormatValue(&buf, data, FormatCSV)
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")

	if len(lines) != 3 {
		t.Fatalf("CSV should have header + 2 rows, got %d lines:\n%s", len(lines), buf.String())
	}
	// Header should contain data field names, not top-level map keys
	header := lines[0]
	if !strings.Contains(header, "message_id") {
		t.Errorf("CSV header should contain 'message_id', got: %s", header)
	}
}

func TestFormatValue_NDJSON_TypedMapSlice(t *testing.T) {
	data := map[string]interface{}{
		"tasks": []map[string]interface{}{
			{"guid": "t1", "url": "https://example.com/t1"},
			{"guid": "t2", "url": "https://example.com/t2"},
		},
	}

	var buf bytes.Buffer
	FormatValue(&buf, data, FormatNDJSON)
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")

	if len(lines) != 2 {
		t.Fatalf("NDJSON should output 2 lines, got %d:\n%s", len(lines), buf.String())
	}
	for i, line := range lines {
		var obj map[string]interface{}
		if err := json.Unmarshal([]byte(line), &obj); err != nil {
			t.Errorf("NDJSON line %d should be valid JSON: %s", i, line)
		}
		if _, ok := obj["guid"]; !ok {
			t.Errorf("NDJSON line %d should contain 'guid' field, got: %s", i, line)
		}
	}
}

func TestFormatValue_LegacyFormats(t *testing.T) {
	data := map[string]interface{}{
		"data": map[string]interface{}{
			"items": []interface{}{
				map[string]interface{}{"name": "Alice"},
			},
		},
	}

	// "data" parses to FormatJSON with ok=false
	dataFmt, dataOK := ParseFormat("data")
	if dataOK {
		t.Error("ParseFormat('data') should return ok=false")
	}
	var buf2 bytes.Buffer
	FormatValue(&buf2, data, dataFmt)
	out2 := buf2.String()
	if !strings.Contains(out2, "items") {
		t.Errorf("ParseFormat('data') → JSON should output full response, got:\n%s", out2)
	}

	// unknown format parses to FormatJSON with ok=false
	fooFmt, fooOK := ParseFormat("foobar")
	if fooOK {
		t.Error("ParseFormat('foobar') should return ok=false")
	}
	var buf3 bytes.Buffer
	FormatValue(&buf3, data, fooFmt)
	out3 := buf3.String()
	if !strings.Contains(out3, "items") {
		t.Errorf("ParseFormat('foobar') → JSON should output full response, got:\n%s", out3)
	}
}
