// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/larksuite/cli/internal/vfs/localfileio"
)

var testPC = &parseCtx{fio: &localfileio.LocalFileIO{}}

func TestParseHelpers(t *testing.T) {
	tmpDir := t.TempDir()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd err=%v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir err=%v", err)
	}
	tmp, err := os.CreateTemp(".", "base-json-*.json")
	if err != nil {
		t.Fatalf("temp file err=%v", err)
	}
	if _, err := tmp.WriteString(`{"name":"from-file"}`); err != nil {
		t.Fatalf("write temp file err=%v", err)
	}
	_ = tmp.Close()
	obj, err := parseJSONObject(testPC, `{"name":"demo"}`, "json")
	if err != nil || obj["name"] != "demo" {
		t.Fatalf("obj=%v err=%v", obj, err)
	}
	if _, err := parseJSONObject(testPC, `[1]`, "json"); err == nil || !strings.Contains(err.Error(), "invalid JSON object") {
		t.Fatalf("err=%v", err)
	}
	obj, err = parseJSONObject(testPC, "@"+tmp.Name(), "json")
	if err != nil || obj["name"] != "from-file" {
		t.Fatalf("file obj=%v err=%v", obj, err)
	}
	arr, err := parseJSONArray(testPC, `[1,2]`, "items")
	if err != nil || len(arr) != 2 {
		t.Fatalf("arr=%v err=%v", arr, err)
	}
	if _, err := parseJSONArray(testPC, `{"a":1}`, "items"); err == nil || !strings.Contains(err.Error(), "invalid JSON array") {
		t.Fatalf("err=%v", err)
	}
	list, err := parseStringListFlexible(testPC, "a, b, ,c", "fields")
	if err != nil || !reflect.DeepEqual(list, []string{"a", "b", "c"}) {
		t.Fatalf("list=%v err=%v", list, err)
	}
	list, err = parseStringListFlexible(testPC, `["x","y"]`, "fields")
	if err != nil || !reflect.DeepEqual(list, []string{"x", "y"}) {
		t.Fatalf("list=%v err=%v", list, err)
	}
	if _, err := parseStringListFlexible(testPC, `[1]`, "fields"); err == nil || !strings.Contains(err.Error(), "invalid JSON string array") {
		t.Fatalf("err=%v", err)
	}
	if _, err := parseJSONValue(testPC, "{", "json"); err == nil || !strings.Contains(err.Error(), "tip: pass a JSON object/array directly") {
		t.Fatalf("err=%v", err)
	}
	if !reflect.DeepEqual(parseStringList("m,n"), []string{"m", "n"}) {
		t.Fatalf("parseStringList mismatch")
	}
}

func TestMapHelpers(t *testing.T) {
	dst := map[string]interface{}{"style": map[string]interface{}{"type": "number"}}
	src := map[string]interface{}{"style": map[string]interface{}{"formatter": "0.00"}, "name": "Amount"}
	merged := deepMergeMaps(dst, src)
	style := merged["style"].(map[string]interface{})
	if style["type"] != "number" || style["formatter"] != "0.00" || merged["name"] != "Amount" {
		t.Fatalf("merged=%v", merged)
	}
	cloned := cloneMap(merged)
	cloned["name"] = "Changed"
	if merged["name"] != "Amount" {
		t.Fatalf("clone modified source: %v", merged)
	}
}

func TestResolveFieldTypeSpecAndNormalization(t *testing.T) {
	spec, err := resolveFieldTypeSpec("currency")
	if err != nil || spec.Type != "number" {
		t.Fatalf("spec=%v err=%v", spec, err)
	}
	if _, ok := spec.Extra["style"]; !ok {
		t.Fatalf("spec=%v", spec)
	}
	spec, err = resolveFieldTypeSpec("multi-select")
	if err != nil || spec.Type != "select" || spec.Extra["multiple"] != true {
		t.Fatalf("spec=%v err=%v", spec, err)
	}
	spec, err = resolveFieldTypeSpec("two_way_link")
	if err != nil || spec.Type != "link" || spec.Extra["bidirectional"] != true {
		t.Fatalf("spec=%v err=%v", spec, err)
	}
	if _, err := resolveFieldTypeSpec("unknown"); err == nil || !strings.Contains(err.Error(), "unsupported field type") {
		t.Fatalf("err=%v", err)
	}
	if normalizeFieldTypeName(" text ") != "text" {
		t.Fatalf("normalizeFieldTypeName failed")
	}
	if normalizeViewTypeName("  Kanban ") != "kanban" {
		t.Fatalf("normalizeViewTypeName failed")
	}
	if normalizeViewTypeName("Custom") != "Custom" {
		t.Fatalf("normalizeViewTypeName should preserve unknown values")
	}
	options := normalizeSelectOptions([]interface{}{"A", map[string]interface{}{"name": "B", "hue": "blue"}, 1})
	if len(options) != 2 {
		t.Fatalf("options=%v", options)
	}
}

func TestBuildFieldBody(t *testing.T) {
	if _, err := buildFieldBody("Name", "text", nil, "", "", true, false); err == nil || !strings.Contains(err.Error(), "primary") {
		t.Fatalf("err=%v", err)
	}
	if _, err := buildFieldBody("Name", "text", nil, "", "", false, true); err == nil || !strings.Contains(err.Error(), "hidden") {
		t.Fatalf("err=%v", err)
	}
	body, err := buildFieldBody("Amount", "number", map[string]interface{}{"precision": 2}, "currency", "", false, false)
	if err != nil || body["type"] != "number" {
		t.Fatalf("body=%v err=%v", body, err)
	}
	style := body["style"].(map[string]interface{})
	if style["type"] != "currency" || toInt(style["precision"]) != 2 {
		t.Fatalf("style=%v", style)
	}
	body, err = buildFieldBody("Status", "multi-select", map[string]interface{}{"options": []interface{}{"Todo", map[string]interface{}{"name": "Done", "hue": "green"}}, "multiple": true}, "", "", false, false)
	if err != nil || body["multiple"] != true {
		t.Fatalf("body=%v err=%v", body, err)
	}
	if len(body["options"].([]interface{})) != 2 {
		t.Fatalf("options=%v", body["options"])
	}
	body, err = buildFieldBody("Owner", "user", map[string]interface{}{"multiple": false}, "", "", false, false)
	if err != nil || body["multiple"] != false {
		t.Fatalf("body=%v err=%v", body, err)
	}
	body, err = buildFieldBody("Relation", "link", map[string]interface{}{"table_id": "tbl_target", "back_field_name": "Back"}, "", "", false, false)
	if err != nil || body["link_table"] != "tbl_target" || body["bidirectional"] != true || body["bidirectional_link_field_name"] != "Back" {
		t.Fatalf("body=%v err=%v", body, err)
	}
	body, err = buildFieldBody("Expr", "formula", map[string]interface{}{"formula_expression": "1+1"}, "", "", false, false)
	if err != nil || body["expression"] != "1+1" {
		t.Fatalf("body=%v err=%v", body, err)
	}
}

func TestBuildTableFieldBodies(t *testing.T) {
	fields, err := buildTableFieldBodies(`[{"name":"Name","type":"text"}]`, "")
	if err != nil || len(fields) != 1 {
		t.Fatalf("fields=%v err=%v", fields, err)
	}
	fields, err = buildTableFieldBodies("", `[["Name","text"],["Amount","currency"]]`)
	if err != nil || len(fields) != 2 {
		t.Fatalf("fields=%v err=%v", fields, err)
	}
	if _, err := buildTableFieldBodies("", `[["Name"]]`); err == nil || !strings.Contains(err.Error(), "must be [name, type]") {
		t.Fatalf("err=%v", err)
	}
}

func TestBaseV3Helpers(t *testing.T) {
	if baseV3Path("/bases/", "app_1", "/tables/", "tbl_1") != "/open-apis/base/v3/bases/app_1/tables/tbl_1" {
		t.Fatalf("baseV3Path mismatch")
	}
	if baseV3Path("bases", "app_1", "tables", "tbl/1", "fields", "fld?1", "views", "视图 1") != "/open-apis/base/v3/bases/app_1/tables/tbl%2F1/fields/fld%3F1/views/%E8%A7%86%E5%9B%BE%201" {
		t.Fatalf("baseV3Path encode mismatch")
	}
	if toInt("42") != 42 || toInt(7.0) != 7 {
		t.Fatalf("toInt mismatch")
	}
	if !reflect.DeepEqual(toStringSlice([]interface{}{"a", "b", 1}), []string{"a", "b"}) {
		t.Fatalf("toStringSlice mismatch")
	}
}

func TestRecordAndChunkHelpers(t *testing.T) {
	records, err := normalizeRecordInputs(`[{"record_id":"rec_1","fields":{"Name":"Alice"}},{"Name":"Bob"}]`)
	if err != nil || len(records) != 2 {
		t.Fatalf("records=%v err=%v", records, err)
	}
	if _, err := normalizeRecordInputs(`[1]`); err == nil || !strings.Contains(err.Error(), "must be an object") {
		t.Fatalf("err=%v", err)
	}
	if len(chunkRecords(records, 1)) != 2 || len(chunkStringIDs([]string{"a", "b", "c"}, 2)) != 2 {
		t.Fatalf("chunk helpers mismatch")
	}
}

func TestResolveAndSimplifyHelpers(t *testing.T) {
	fields := []map[string]interface{}{{"id": "fld_1", "name": "Name", "type": "text"}, {"field_id": "fld_2", "field_name": "Age", "type": "number", "multiple": true}}
	tables := []map[string]interface{}{{"id": "tbl_1", "name": "Orders"}}
	views := []map[string]interface{}{{"id": "vew_1", "name": "Main", "type": "grid"}}
	if field, err := resolveFieldRef(fields, "Age"); err != nil || fieldID(field) != "fld_2" {
		t.Fatalf("field=%v err=%v", field, err)
	}
	if table, err := resolveTableRef(tables, "tbl_1"); err != nil || tableNameFromMap(table) != "Orders" {
		t.Fatalf("table=%v err=%v", table, err)
	}
	if view, err := resolveViewRef(views, "Main"); err != nil || viewID(view) != "vew_1" {
		t.Fatalf("view=%v err=%v", view, err)
	}
	if _, err := resolveViewRef(views, "Missing"); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("err=%v", err)
	}
	simplifiedFields := simplifyFields(fields)
	if len(simplifiedFields) != 2 {
		t.Fatalf("simplifiedFields=%v", simplifiedFields)
	}
	simplifiedViews := simplifyViews(views)
	if len(simplifiedViews) != 1 {
		t.Fatalf("simplifiedViews=%v", simplifiedViews)
	}
}

func TestFilterAndSortHelpers(t *testing.T) {
	items := []map[string]interface{}{
		{"record_id": "rec_1", "fields": map[string]interface{}{"Name": "Alice", "Age": 18, "Tags": []interface{}{"a", "b"}}},
		{"record_id": "rec_2", "fields": map[string]interface{}{"Name": "Bob", "Age": 30, "Tags": []interface{}{"c"}}},
	}
	selected := selectRecordFields(items, []string{"Name"})
	if selected[0]["record_id"] != "rec_1" {
		t.Fatalf("selected=%v", selected)
	}
	if compareScalar(2, 10) >= 0 || compareScalar("b", "a") <= 0 {
		t.Fatalf("compareScalar mismatch")
	}
	if canonicalValue([]interface{}{"x"}) != "x" || canonicalValue(map[string]interface{}{"text": "hello"}) != "hello" {
		t.Fatalf("canonicalValue mismatch")
	}
	logic, conditions := normalizeFilterConfig(map[string]interface{}{
		"conjunction": "or",
		"conditions":  []interface{}{map[string]interface{}{"field_name": "Name", "operator": "==", "value": "Alice"}},
	})
	if logic != "or" || len(conditions) != 1 {
		t.Fatalf("logic=%s conditions=%v", logic, conditions)
	}
	filtered := filterRecords(items, map[string]interface{}{
		"logic": "and",
		"conditions": []interface{}{
			[]interface{}{"Age", ">=", 18},
			[]interface{}{"Tags", "intersects", []interface{}{"b"}},
		},
	})
	if len(filtered) != 1 || filtered[0]["record_id"] != "rec_1" {
		t.Fatalf("filtered=%v", filtered)
	}
	sorted := sortRecords(items, []interface{}{map[string]interface{}{"field": "Age", "desc": true}})
	if sorted[0]["record_id"] != "rec_2" {
		t.Fatalf("sorted=%v", sorted)
	}
	if !matchesCondition(nil, []interface{}{"Name", "empty"}) {
		t.Fatalf("matchesCondition empty failed")
	}
}

func TestJSONInputHelpers(t *testing.T) {
	if got, err := loadJSONInput(testPC, `{"name":"demo"}`, "json"); err != nil || got != `{"name":"demo"}` {
		t.Fatalf("got=%q err=%v", got, err)
	}
	if _, err := loadJSONInput(testPC, "@", "json"); err == nil || !strings.Contains(err.Error(), "file path cannot be empty") {
		t.Fatalf("err=%v", err)
	}
	tmp := t.TempDir()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd err=%v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir err=%v", err)
	}
	emptyPath := "empty.json"
	if err := os.WriteFile(emptyPath, []byte("  \n"), 0o644); err != nil {
		t.Fatalf("write empty file err=%v", err)
	}
	if _, err := loadJSONInput(testPC, "@"+emptyPath, "json"); err == nil || !strings.Contains(err.Error(), "is empty") {
		t.Fatalf("err=%v", err)
	}
	syntaxErr := formatJSONError("json", "object", &json.SyntaxError{Offset: 7})
	if !strings.Contains(syntaxErr.Error(), "near byte 7") || !strings.Contains(syntaxErr.Error(), "tip: pass a JSON object/array directly") {
		t.Fatalf("syntaxErr=%v", syntaxErr)
	}
	typeErr := formatJSONError("json", "object", &json.UnmarshalTypeError{Field: "filter_info"})
	if !strings.Contains(typeErr.Error(), `field "filter_info"`) {
		t.Fatalf("typeErr=%v", typeErr)
	}
}

func TestIdentifierAndValueHelpers(t *testing.T) {
	if normalizeViewTypeName("") != "" || normalizeViewTypeName(" Gantt ") != "gantt" || normalizeViewTypeName("gallery") != "gallery" || normalizeViewTypeName("calendar") != "calendar" || normalizeViewTypeName("grid") != "grid" {
		t.Fatalf("normalizeViewTypeName unexpected")
	}
	if tableID(map[string]interface{}{"table_id": "tbl_alt"}) != "tbl_alt" {
		t.Fatalf("tableID alt key failed")
	}
	if tableNameFromMap(map[string]interface{}{"table_name": "Orders"}) != "Orders" {
		t.Fatalf("tableName alt key failed")
	}
	if viewID(map[string]interface{}{"view_id": "vew_alt"}) != "vew_alt" {
		t.Fatalf("viewID alt key failed")
	}
	if viewName(map[string]interface{}{"view_name": "Main"}) != "Main" {
		t.Fatalf("viewName alt key failed")
	}
	if viewType(map[string]interface{}{"view_type": "grid"}) != "grid" {
		t.Fatalf("viewType alt key failed")
	}
	if !valueEmpty(nil) || !valueEmpty("  ") || !valueEmpty([]interface{}{}) || !valueEmpty(map[string]interface{}{}) {
		t.Fatalf("valueEmpty empty cases failed")
	}
	if valueEmpty(0) {
		t.Fatalf("valueEmpty should keep numeric zero as non-empty")
	}
}

func TestConditionHelpers(t *testing.T) {
	if matchesCondition("x", []interface{}{"Name"}) {
		t.Fatalf("short condition should be false")
	}
	cases := []struct {
		name  string
		value interface{}
		cond  []interface{}
		want  bool
	}{
		{"eq", 1.0, []interface{}{"Age", "==", 1.0}, true},
		{"neq", 1.0, []interface{}{"Age", "!=", 2.0}, true},
		{"gt", 3.0, []interface{}{"Age", ">", 2.0}, true},
		{"gte", 3.0, []interface{}{"Age", ">=", 3.0}, true},
		{"lt", 1.0, []interface{}{"Age", "<", 2.0}, true},
		{"lte", 1.0, []interface{}{"Age", "<=", 1.0}, true},
		{"empty", " ", []interface{}{"Name", "empty"}, true},
		{"non_empty", "Alice", []interface{}{"Name", "non_empty"}, true},
		{"intersects", []interface{}{"a", "b"}, []interface{}{"Tags", "intersects", []interface{}{"c", "b"}}, true},
		{"disjoint", []interface{}{"a", "b"}, []interface{}{"Tags", "disjoint", []interface{}{"c", "d"}}, true},
		{"unknown", "Alice", []interface{}{"Name", "contains", "A"}, false},
	}
	for _, tt := range cases {
		if got := matchesCondition(tt.value, tt.cond); got != tt.want {
			t.Fatalf("%s got=%v want=%v", tt.name, got, tt.want)
		}
	}
}

func TestSleepBetweenBatches(t *testing.T) {
	start := time.Now()
	sleepBetweenBatches(0, 1)
	if elapsed := time.Since(start); elapsed > 200*time.Millisecond {
		t.Fatalf("unexpected sleep for last batch: %v", elapsed)
	}
	start = time.Now()
	sleepBetweenBatches(0, 2)
	if elapsed := time.Since(start); elapsed < 550*time.Millisecond {
		t.Fatalf("expected sleep between batches, got %v", elapsed)
	}
}

func TestResolveFieldTypeSpecMoreAliases(t *testing.T) {
	cases := []struct {
		input    string
		wantType string
		check    func(fieldTypeSpec) bool
	}{
		{"", "", func(spec fieldTypeSpec) bool { return false }},
		{"progress", "number", func(spec fieldTypeSpec) bool {
			return spec.Extra["style"].(map[string]interface{})["type"] == "progress"
		}},
		{"rating", "number", func(spec fieldTypeSpec) bool { return spec.Extra["style"].(map[string]interface{})["type"] == "rating" }},
		{"single-select", "select", func(spec fieldTypeSpec) bool { return spec.Extra["multiple"] == false }},
		{"group-chat", "user", func(spec fieldTypeSpec) bool { return spec.Extra["multiple"] == true }},
		{"auto-number", "auto_number", func(spec fieldTypeSpec) bool { _, ok := spec.Extra["style"]; return ok }},
		{"created-time", "created_at", func(spec fieldTypeSpec) bool {
			return spec.Extra["style"].(map[string]interface{})["format"] == "yyyy/MM/dd"
		}},
		{"modified_time", "updated_at", func(spec fieldTypeSpec) bool {
			return spec.Extra["style"].(map[string]interface{})["format"] == "yyyy/MM/dd"
		}},
	}
	if _, err := resolveFieldTypeSpec(cases[0].input); err == nil || !strings.Contains(err.Error(), "cannot be empty") {
		t.Fatalf("err=%v", err)
	}
	for _, tt := range cases[1:] {
		spec, err := resolveFieldTypeSpec(tt.input)
		if err != nil || spec.Type != tt.wantType || !tt.check(spec) {
			t.Fatalf("input=%s spec=%v err=%v", tt.input, spec, err)
		}
	}
}

func TestNamedSpecAndSortHelpers(t *testing.T) {
	specs, err := parseNamedTypeSpecs(`[["Name","text"],["Amount","number"]]`, "fields")
	if err != nil || len(specs) != 2 || specs[1].Type != "number" {
		t.Fatalf("specs=%v err=%v", specs, err)
	}
	if _, err := parseNamedTypeSpecs(`{}`, "fields"); err == nil || !strings.Contains(err.Error(), "invalid JSON array") {
		t.Fatalf("err=%v", err)
	}
	if _, err := parseNamedTypeSpecs(`[["Name"]]`, "fields"); err == nil || !strings.Contains(err.Error(), "must be [name, type]") {
		t.Fatalf("err=%v", err)
	}
	if _, err := parseNamedTypeSpecs(`[[1,"text"]]`, "fields"); err == nil || !strings.Contains(err.Error(), "must be [string, string]") {
		t.Fatalf("err=%v", err)
	}
	normalized := normalizeSortConfig([]interface{}{
		map[string]interface{}{"field_name": "Priority", "desc": true},
		map[string]interface{}{"field": "Amount"},
		"ignored",
	})
	if len(normalized) != 2 || normalized[0]["field"] != "Priority" || normalized[0]["desc"] != true || normalized[1]["field"] != "Amount" {
		t.Fatalf("normalized=%v", normalized)
	}
}

func TestCanonicalSelectAndCompareHelpers(t *testing.T) {
	if fieldTypeName(map[string]interface{}{"kind": "text"}) != "<nil>" {
		t.Fatalf("fieldTypeName fallback mismatch")
	}
	if got := canonicalValue(map[string]interface{}{"id": "opt_1"}); got != "opt_1" {
		t.Fatalf("canonical id=%q", got)
	}
	if got := canonicalValue(1.5); got != "1.5" {
		t.Fatalf("canonical float=%q", got)
	}
	if got := canonicalValue([]interface{}{"x", "y"}); !strings.Contains(got, "x") || !strings.Contains(got, "y") {
		t.Fatalf("canonical array=%q", got)
	}
	if compareScalar("2", 2.0) != 0 || compareScalar("a", "b") >= 0 {
		t.Fatalf("compareScalar mismatch")
	}
	set := asSet(" Alice ")
	if !set["Alice"] || len(set) != 1 {
		t.Fatalf("set=%v", set)
	}
	selected := selectRecordFields([]map[string]interface{}{{"record_id": "rec_1", "fields": map[string]interface{}{"Name": "Alice"}}}, nil)
	if selected[0]["fields"].(map[string]interface{})["Name"] != "Alice" {
		t.Fatalf("selected=%v", selected)
	}
	if _, err := resolveFieldRef([]map[string]interface{}{{"id": "fld_1", "name": "Name"}}, "Missing"); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("err=%v", err)
	}
	if _, err := resolveTableRef([]map[string]interface{}{{"id": "tbl_1", "name": "Orders"}}, "Missing"); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("err=%v", err)
	}
}
