// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/shortcuts/common"
)

const (
	batchSize         = 500
	baseV3ServicePath = "/open-apis/base/v3"
)

type fieldTypeSpec struct {
	Type  string
	Extra map[string]interface{}
}

func parseJSONObject(pc *parseCtx, raw string, flagName string) (map[string]interface{}, error) {
	resolved, err := loadJSONInput(pc, raw, flagName)
	if err != nil {
		return nil, err
	}
	var result map[string]interface{}
	if err := common.ParseJSON([]byte(resolved), &result); err != nil {
		return nil, formatJSONError(flagName, "object", err)
	}
	return result, nil
}

func parseJSONArray(pc *parseCtx, raw string, flagName string) ([]interface{}, error) {
	resolved, err := loadJSONInput(pc, raw, flagName)
	if err != nil {
		return nil, err
	}
	var result []interface{}
	if err := common.ParseJSON([]byte(resolved), &result); err != nil {
		return nil, formatJSONError(flagName, "array", err)
	}
	return result, nil
}

func parseStringListFlexible(pc *parseCtx, raw string, flagName string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	resolved, err := loadJSONInput(pc, raw, flagName)
	if err != nil {
		return nil, err
	}
	if strings.HasPrefix(resolved, "[") {
		var result []string
		if err := common.ParseJSON([]byte(resolved), &result); err != nil {
			return nil, formatJSONError(flagName, "string array", err)
		}
		return result, nil
	}
	raw = resolved
	parts := strings.Split(raw, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		item := strings.TrimSpace(part)
		if item != "" {
			result = append(result, item)
		}
	}
	return result, nil
}

func parseStringList(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		item := strings.TrimSpace(part)
		if item != "" {
			result = append(result, item)
		}
	}
	return result
}

func deepMergeMaps(dst, src map[string]interface{}) map[string]interface{} {
	if dst == nil {
		dst = map[string]interface{}{}
	}
	for key, value := range src {
		if srcMap, ok := value.(map[string]interface{}); ok {
			if dstMap, ok := dst[key].(map[string]interface{}); ok {
				dst[key] = deepMergeMaps(dstMap, srcMap)
			} else {
				dst[key] = deepMergeMaps(map[string]interface{}{}, srcMap)
			}
			continue
		}
		dst[key] = value
	}
	return dst
}

func cloneMap(src map[string]interface{}) map[string]interface{} {
	if src == nil {
		return nil
	}
	dst := make(map[string]interface{}, len(src))
	for key, value := range src {
		dst[key] = cloneValue(value)
	}
	return dst
}

func cloneValue(value interface{}) interface{} {
	switch val := value.(type) {
	case map[string]interface{}:
		return cloneMap(val)
	case []interface{}:
		cloned := make([]interface{}, len(val))
		for i, item := range val {
			cloned[i] = cloneValue(item)
		}
		return cloned
	default:
		return val
	}
}

func resolveFieldTypeSpec(typeName string) (fieldTypeSpec, error) {
	trimmed := strings.TrimSpace(typeName)
	if trimmed == "" {
		return fieldTypeSpec{}, fmt.Errorf("field type cannot be empty")
	}
	switch strings.ToLower(trimmed) {
	case "text", "phone", "url", "email", "barcode":
		return fieldTypeSpec{Type: "text"}, nil
	case "number":
		return fieldTypeSpec{Type: "number", Extra: map[string]interface{}{"style": map[string]interface{}{"type": "number", "formatter": "0"}}}, nil
	case "currency":
		return fieldTypeSpec{Type: "number", Extra: map[string]interface{}{"style": map[string]interface{}{"type": "currency", "currency_code": "CNY", "formatter": "0.00"}}}, nil
	case "progress":
		return fieldTypeSpec{Type: "number", Extra: map[string]interface{}{"style": map[string]interface{}{"type": "progress", "min": 0, "max": 100, "color": "Blue"}}}, nil
	case "rating":
		return fieldTypeSpec{Type: "number", Extra: map[string]interface{}{"style": map[string]interface{}{"type": "rating", "icon": "star", "min": 1, "max": 5}}}, nil
	case "singleselect", "single_select", "single-select":
		return fieldTypeSpec{Type: "select", Extra: map[string]interface{}{"multiple": false}}, nil
	case "multiselect", "multi_select", "multi-select":
		return fieldTypeSpec{Type: "select", Extra: map[string]interface{}{"multiple": true}}, nil
	case "datetime", "date", "date_time", "date-time":
		return fieldTypeSpec{Type: "datetime", Extra: map[string]interface{}{"style": map[string]interface{}{"format": "yyyy/MM/dd"}}}, nil
	case "checkbox":
		return fieldTypeSpec{Type: "checkbox"}, nil
	case "user", "groupchat", "group_chat", "group-chat":
		return fieldTypeSpec{Type: "user", Extra: map[string]interface{}{"multiple": true}}, nil
	case "attachment":
		return fieldTypeSpec{Type: "attachment"}, nil
	case "link":
		return fieldTypeSpec{Type: "link"}, nil
	case "twowaylink", "two_way_link", "two-way-link":
		return fieldTypeSpec{Type: "link", Extra: map[string]interface{}{"bidirectional": true}}, nil
	case "formula":
		return fieldTypeSpec{Type: "formula"}, nil
	case "location":
		return fieldTypeSpec{Type: "location"}, nil
	case "autonumber", "auto_number", "auto-number":
		return fieldTypeSpec{Type: "auto_number", Extra: map[string]interface{}{"style": map[string]interface{}{"rules": []interface{}{map[string]interface{}{"type": "text", "text": "NO."}, map[string]interface{}{"type": "incremental_number", "length": 3}}}}}, nil
	case "createdtime", "created_time", "created-time":
		return fieldTypeSpec{Type: "created_at", Extra: map[string]interface{}{"style": map[string]interface{}{"format": "yyyy/MM/dd"}}}, nil
	case "modifiedtime", "modified_time", "modified-time":
		return fieldTypeSpec{Type: "updated_at", Extra: map[string]interface{}{"style": map[string]interface{}{"format": "yyyy/MM/dd"}}}, nil
	default:
		return fieldTypeSpec{}, fmt.Errorf("unsupported field type %q in base/v3", typeName)
	}
}

func normalizeFieldTypeName(typeName string) string {
	return strings.TrimSpace(typeName)
}

func normalizeViewTypeName(typeName string) string {
	trimmed := strings.TrimSpace(typeName)
	if trimmed == "" {
		return trimmed
	}
	switch strings.ToLower(trimmed) {
	case "grid":
		return "grid"
	case "kanban":
		return "kanban"
	case "gallery":
		return "gallery"
	case "gantt":
		return "gantt"
	case "calendar":
		return "calendar"
	default:
		return trimmed
	}
}

func normalizeSelectOptions(raw interface{}) []interface{} {
	src, ok := raw.([]interface{})
	if !ok {
		return nil
	}
	result := make([]interface{}, 0, len(src))
	for _, item := range src {
		switch v := item.(type) {
		case string:
			result = append(result, map[string]interface{}{"name": v})
		case map[string]interface{}:
			option := map[string]interface{}{}
			if name, _ := v["name"].(string); name != "" {
				option["name"] = name
			}
			if hue, _ := v["hue"].(string); hue != "" {
				option["hue"] = hue
			}
			if lightness, _ := v["lightness"].(string); lightness != "" {
				option["lightness"] = lightness
			}
			if len(option) > 0 {
				result = append(result, option)
			}
		}
	}
	return result
}

func buildFieldBody(fieldName string, typeName string, property map[string]interface{}, uiType string, description string, isPrimary bool, isHidden bool) (map[string]interface{}, error) {
	if isPrimary {
		return nil, fmt.Errorf("base/v3 does not support setting primary field in field body")
	}
	if isHidden {
		return nil, fmt.Errorf("base/v3 does not support hidden field creation in field body")
	}
	spec, err := resolveFieldTypeSpec(typeName)
	if err != nil {
		return nil, err
	}
	body := map[string]interface{}{
		"type": spec.Type,
		"name": fieldName,
	}
	body = deepMergeMaps(body, cloneMap(spec.Extra))
	if description != "" {
		_ = description
	}
	if uiType != "" {
		switch strings.ToLower(uiType) {
		case "currency":
			body["type"] = "number"
			body["style"] = map[string]interface{}{"type": "currency", "currency_code": "CNY", "formatter": "0.00"}
		case "progress":
			body["type"] = "number"
			body["style"] = map[string]interface{}{"type": "progress", "min": 0, "max": 100, "color": "Blue"}
		case "rating":
			body["type"] = "number"
			body["style"] = map[string]interface{}{"type": "rating", "icon": "star", "min": 1, "max": 5}
		}
	}
	if property == nil {
		return body, nil
	}
	property = cloneMap(property)
	switch body["type"] {
	case "number", "datetime", "created_at", "updated_at", "auto_number":
		style, _ := body["style"].(map[string]interface{})
		if style == nil {
			style = map[string]interface{}{}
		}
		if inner, ok := property["style"].(map[string]interface{}); ok {
			style = deepMergeMaps(style, inner)
			delete(property, "style")
		}
		style = deepMergeMaps(style, property)
		if len(style) > 0 {
			body["style"] = style
		}
	case "select":
		if options, ok := property["options"]; ok {
			body["options"] = normalizeSelectOptions(options)
			delete(property, "options")
		}
		if multiple, ok := property["multiple"].(bool); ok {
			body["multiple"] = multiple
			delete(property, "multiple")
		}
		body = deepMergeMaps(body, property)
	case "user":
		if multiple, ok := property["multiple"].(bool); ok {
			body["multiple"] = multiple
			delete(property, "multiple")
		}
	case "link":
		if tableID, _ := property["table_id"].(string); tableID != "" {
			body["link_table"] = tableID
			delete(property, "table_id")
		}
		if tableID, _ := property["link_table"].(string); tableID != "" {
			body["link_table"] = tableID
			delete(property, "link_table")
		}
		if multiple, ok := property["multiple"].(bool); ok {
			_ = multiple
			delete(property, "multiple")
		}
		if backName, _ := property["back_field_name"].(string); backName != "" {
			body["bidirectional"] = true
			body["bidirectional_link_field_name"] = backName
			delete(property, "back_field_name")
		}
		body = deepMergeMaps(body, property)
	case "formula":
		if expr, _ := property["formula_expression"].(string); expr != "" {
			body["expression"] = expr
			delete(property, "formula_expression")
		}
		if expr, _ := property["expression"].(string); expr != "" {
			body["expression"] = expr
			delete(property, "expression")
		}
		body = deepMergeMaps(body, property)
	default:
		body = deepMergeMaps(body, property)
	}
	return body, nil
}

func buildTableFieldBodies(rawFields string, rawFieldSpecs string) ([]interface{}, error) {
	if rawFields != "" {
		var fields []interface{}
		if err := common.ParseJSON([]byte(rawFields), &fields); err != nil {
			return nil, fmt.Errorf("--fields invalid JSON, must be a field definition array")
		}
		return fields, nil
	}
	specs, err := parseNamedTypeSpecs(rawFieldSpecs, "field-specs")
	if err != nil {
		return nil, err
	}
	fields := make([]interface{}, 0, len(specs))
	for _, spec := range specs {
		body, err := buildFieldBody(spec.Name, normalizeFieldTypeName(spec.Type), nil, "", "", false, false)
		if err != nil {
			return nil, fmt.Errorf("field %q: %w", spec.Name, err)
		}
		fields = append(fields, body)
	}
	return fields, nil
}

func baseV3Path(parts ...string) string {
	clean := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.Trim(part, "/")
		if part != "" {
			clean = append(clean, url.PathEscape(part))
		}
	}
	return baseV3ServicePath + "/" + strings.Join(clean, "/")
}

func baseV3Raw(runtime *common.RuntimeContext, method, path string, params map[string]interface{}, data interface{}) (map[string]interface{}, error) {
	queryParams := make(larkcore.QueryParams)
	for k, v := range params {
		queryParams.Set(k, fmt.Sprintf("%v", v))
	}
	req := &larkcore.ApiReq{
		HttpMethod:  strings.ToUpper(method),
		ApiPath:     path,
		Body:        data,
		QueryParams: queryParams,
	}
	h := make(http.Header)
	h.Set("X-App-Id", runtime.Config.AppID)
	resp, err := runtime.DoAPI(req, larkcore.WithHeaders(h))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= http.StatusBadRequest {
		body := strings.TrimSpace(string(resp.RawBody))
		if body == "" {
			return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
		}
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, body)
	}
	var result map[string]interface{}
	dec := json.NewDecoder(bytes.NewReader(resp.RawBody))
	dec.UseNumber()
	if err := dec.Decode(&result); err != nil {
		return nil, fmt.Errorf("response parse error: %w", err)
	}
	return result, nil
}

func baseV3Call(runtime *common.RuntimeContext, method, path string, params map[string]interface{}, data interface{}) (map[string]interface{}, error) {
	result, err := baseV3Raw(runtime, method, path, params, data)
	return handleBaseAPIResult(result, err, "API call failed")
}

func baseV3CallAny(runtime *common.RuntimeContext, method, path string, params map[string]interface{}, data interface{}) (interface{}, error) {
	result, err := baseV3Raw(runtime, method, path, params, data)
	return handleBaseAPIResultAny(result, err, "API call failed")
}

func toInt(v interface{}) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	case json.Number:
		i, _ := n.Int64()
		return int(i)
	case string:
		i, _ := strconv.Atoi(strings.TrimSpace(n))
		return i
	default:
		return 0
	}
}

func toStringSlice(v interface{}) []string {
	arr, ok := v.([]interface{})
	if !ok {
		return nil
	}
	result := make([]string, 0, len(arr))
	for _, item := range arr {
		if s, ok := item.(string); ok {
			result = append(result, s)
		}
	}
	return result
}

func listAllTables(runtime *common.RuntimeContext, baseToken string, offset, limit int) ([]map[string]interface{}, int, error) {
	if limit <= 0 {
		return nil, 0, fmt.Errorf("limit must be greater than 0")
	}
	data, err := baseV3Call(runtime, "GET", baseV3Path("bases", baseToken, "tables"), map[string]interface{}{"offset": offset, "limit": limit}, nil)
	if err != nil {
		return nil, 0, err
	}
	rawItems, _ := data["tables"].([]interface{})
	if len(rawItems) == 0 {
		rawItems, _ = data["items"].([]interface{})
	}
	if len(rawItems) == 0 {
		if _, hasID := data["id"]; hasID {
			rawItems = []interface{}{data}
		}
	}
	items := make([]map[string]interface{}, 0, len(rawItems))
	for _, item := range rawItems {
		if m, ok := item.(map[string]interface{}); ok {
			items = append(items, m)
		}
	}
	total := toInt(data["total"])
	if total == 0 {
		total = len(items)
	}
	return items, total, nil
}

func listAllFields(runtime *common.RuntimeContext, baseToken, tableID string, offset, limit int) ([]map[string]interface{}, int, error) {
	if limit <= 0 {
		return nil, 0, fmt.Errorf("limit must be greater than 0")
	}
	data, err := baseV3Call(runtime, "GET", baseV3Path("bases", baseToken, "tables", tableID, "fields"), map[string]interface{}{"offset": offset, "limit": limit}, nil)
	if err != nil {
		return nil, 0, err
	}
	rawItems, _ := data["fields"].([]interface{})
	items := make([]map[string]interface{}, 0, len(rawItems))
	for _, item := range rawItems {
		if m, ok := item.(map[string]interface{}); ok {
			items = append(items, m)
		}
	}
	total := toInt(data["total"])
	if total == 0 {
		total = len(items)
	}
	return items, total, nil
}

func listAllViews(runtime *common.RuntimeContext, baseToken, tableID string, offset, limit int) ([]map[string]interface{}, int, error) {
	if limit <= 0 {
		return nil, 0, fmt.Errorf("limit must be greater than 0")
	}
	data, err := baseV3Call(runtime, "GET", baseV3Path("bases", baseToken, "tables", tableID, "views"), map[string]interface{}{"offset": offset, "limit": limit}, nil)
	if err != nil {
		return nil, 0, err
	}
	rawItems, _ := data["views"].([]interface{})
	items := make([]map[string]interface{}, 0, len(rawItems))
	for _, item := range rawItems {
		if m, ok := item.(map[string]interface{}); ok {
			items = append(items, m)
		}
	}
	total := toInt(data["total"])
	if total == 0 {
		total = len(items)
	}
	return items, total, nil
}

func resolveFieldRef(fields []map[string]interface{}, ref string) (map[string]interface{}, error) {
	for _, field := range fields {
		if ref == fieldID(field) || ref == fieldName(field) {
			return field, nil
		}
	}
	return nil, fmt.Errorf("field %q not found", ref)
}

func resolveTableRef(tables []map[string]interface{}, ref string) (map[string]interface{}, error) {
	for _, table := range tables {
		if ref == tableID(table) || ref == tableNameFromMap(table) {
			return table, nil
		}
	}
	return nil, fmt.Errorf("table %q not found", ref)
}

func resolveViewRef(views []map[string]interface{}, ref string) (map[string]interface{}, error) {
	for _, view := range views {
		if ref == viewID(view) || ref == viewName(view) {
			return view, nil
		}
	}
	return nil, fmt.Errorf("view %q not found", ref)
}

func normalizeRecordInputs(raw string) ([]map[string]interface{}, error) {
	var records []interface{}
	if err := common.ParseJSON([]byte(raw), &records); err != nil {
		return nil, fmt.Errorf("--records invalid JSON, must be a record array")
	}
	result := make([]map[string]interface{}, 0, len(records))
	for idx, item := range records {
		record, ok := item.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("record %d must be an object", idx+1)
		}
		if fields, ok := record["fields"].(map[string]interface{}); ok {
			normalized := map[string]interface{}{"fields": fields}
			if recordID, ok := record["record_id"].(string); ok && recordID != "" {
				normalized["record_id"] = recordID
			}
			result = append(result, normalized)
			continue
		}
		result = append(result, map[string]interface{}{"fields": record})
	}
	return result, nil
}

func chunkRecords(records []map[string]interface{}, size int) [][]map[string]interface{} {
	if size <= 0 {
		size = 1
	}
	chunks := [][]map[string]interface{}{}
	for start := 0; start < len(records); start += size {
		end := start + size
		if end > len(records) {
			end = len(records)
		}
		chunks = append(chunks, records[start:end])
	}
	return chunks
}

func chunkStringIDs(ids []string, size int) [][]string {
	if size <= 0 {
		size = 1
	}
	chunks := [][]string{}
	for start := 0; start < len(ids); start += size {
		end := start + size
		if end > len(ids) {
			end = len(ids)
		}
		chunks = append(chunks, ids[start:end])
	}
	return chunks
}

func fieldName(field map[string]interface{}) string {
	if v, _ := field["name"].(string); v != "" {
		return v
	}
	v, _ := field["field_name"].(string)
	return v
}

func fieldID(field map[string]interface{}) string {
	if v, _ := field["id"].(string); v != "" {
		return v
	}
	v, _ := field["field_id"].(string)
	return v
}

func fieldTypeName(field map[string]interface{}) string {
	if v, _ := field["type"].(string); v != "" {
		return v
	}
	return fmt.Sprintf("%v", field["type"])
}

func tableID(table map[string]interface{}) string {
	if v, _ := table["id"].(string); v != "" {
		return v
	}
	v, _ := table["table_id"].(string)
	return v
}

func tableNameFromMap(table map[string]interface{}) string {
	if v, _ := table["name"].(string); v != "" {
		return v
	}
	v, _ := table["table_name"].(string)
	return v
}

func viewID(view map[string]interface{}) string {
	if v, _ := view["id"].(string); v != "" {
		return v
	}
	v, _ := view["view_id"].(string)
	return v
}

func viewName(view map[string]interface{}) string {
	if v, _ := view["name"].(string); v != "" {
		return v
	}
	v, _ := view["view_name"].(string)
	return v
}

func canonicalValue(v interface{}) string {
	switch val := v.(type) {
	case nil:
		return ""
	case []interface{}:
		if len(val) == 1 {
			return canonicalValue(val[0])
		}
	case map[string]interface{}:
		if id, ok := val["id"]; ok {
			return canonicalValue(id)
		}
		if text, ok := val["text"]; ok {
			return canonicalValue(text)
		}
	case string:
		return strings.TrimSpace(val)
	case float64:
		if val == float64(int64(val)) {
			return fmt.Sprintf("%d", int64(val))
		}
	}
	b, _ := json.Marshal(v)
	return string(b)
}

func parseNamedTypeSpecs(raw string, flagName string) ([]namedTypeSpec, error) {
	var tuples []interface{}
	if err := common.ParseJSON([]byte(raw), &tuples); err != nil {
		return nil, fmt.Errorf("--%s invalid JSON array", flagName)
	}
	result := make([]namedTypeSpec, 0, len(tuples))
	for idx, item := range tuples {
		pair, ok := item.([]interface{})
		if !ok || len(pair) != 2 {
			return nil, fmt.Errorf("--%s item %d must be [name, type]", flagName, idx+1)
		}
		name, ok1 := pair[0].(string)
		typeName, ok2 := pair[1].(string)
		if !ok1 || !ok2 {
			return nil, fmt.Errorf("--%s item %d must be [string, string]", flagName, idx+1)
		}
		result = append(result, namedTypeSpec{Name: name, Type: typeName})
	}
	return result, nil
}

type namedTypeSpec struct {
	Name string
	Type string
}

func selectRecordFields(items []map[string]interface{}, fields []string) []map[string]interface{} {
	if len(fields) == 0 {
		return items
	}
	result := make([]map[string]interface{}, 0, len(items))
	for _, item := range items {
		entry := map[string]interface{}{}
		if recordID, _ := item["record_id"].(string); recordID != "" {
			entry["record_id"] = recordID
		}
		selected := map[string]interface{}{}
		fieldMap, _ := item["fields"].(map[string]interface{})
		for _, name := range fields {
			if value, ok := fieldMap[name]; ok {
				selected[name] = value
			}
		}
		entry["fields"] = selected
		result = append(result, entry)
	}
	return result
}

func compareScalar(left interface{}, right interface{}) int {
	lf, lerr := strconv.ParseFloat(canonicalValue(left), 64)
	rf, rerr := strconv.ParseFloat(canonicalValue(right), 64)
	if lerr == nil && rerr == nil {
		switch {
		case lf < rf:
			return -1
		case lf > rf:
			return 1
		default:
			return 0
		}
	}
	ls := canonicalValue(left)
	rs := canonicalValue(right)
	switch {
	case ls < rs:
		return -1
	case ls > rs:
		return 1
	default:
		return 0
	}
}

func asSet(v interface{}) map[string]bool {
	set := map[string]bool{}
	switch val := v.(type) {
	case []interface{}:
		for _, item := range val {
			set[canonicalValue(item)] = true
		}
	default:
		if c := canonicalValue(v); c != "" {
			set[c] = true
		}
	}
	return set
}

func valueEmpty(v interface{}) bool {
	switch val := v.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(val) == ""
	case []interface{}:
		return len(val) == 0
	case map[string]interface{}:
		return len(val) == 0
	default:
		return canonicalValue(v) == ""
	}
}

func matchesCondition(value interface{}, condition []interface{}) bool {
	if len(condition) < 2 {
		return false
	}
	op, _ := condition[1].(string)
	var target interface{}
	if len(condition) > 2 {
		target = condition[2]
	}
	switch op {
	case "==":
		return compareScalar(value, target) == 0
	case "!=":
		return compareScalar(value, target) != 0
	case ">":
		return compareScalar(value, target) > 0
	case ">=":
		return compareScalar(value, target) >= 0
	case "<":
		return compareScalar(value, target) < 0
	case "<=":
		return compareScalar(value, target) <= 0
	case "empty":
		return valueEmpty(value)
	case "non_empty":
		return !valueEmpty(value)
	case "intersects":
		left := asSet(value)
		right := asSet(target)
		for key := range left {
			if right[key] {
				return true
			}
		}
		return false
	case "disjoint":
		left := asSet(value)
		right := asSet(target)
		for key := range left {
			if right[key] {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func normalizeFilterConfig(raw map[string]interface{}) (string, [][]interface{}) {
	logic, _ := raw["logic"].(string)
	if logic == "" {
		logic, _ = raw["conjunction"].(string)
	}
	if logic == "" {
		logic = "and"
	}
	rawConditions, _ := raw["conditions"].([]interface{})
	conditions := make([][]interface{}, 0, len(rawConditions))
	for _, item := range rawConditions {
		switch cond := item.(type) {
		case []interface{}:
			conditions = append(conditions, cond)
		case map[string]interface{}:
			fieldName, ok := cond["field"]
			if !ok {
				fieldName = cond["field_name"]
			}
			conditions = append(conditions, []interface{}{fieldName, cond["operator"], cond["value"]})
		}
	}
	return logic, conditions
}

func filterRecords(items []map[string]interface{}, filter map[string]interface{}) []map[string]interface{} {
	logic, conditions := normalizeFilterConfig(filter)
	if len(conditions) == 0 {
		return items
	}
	result := make([]map[string]interface{}, 0, len(items))
	for _, item := range items {
		fields, _ := item["fields"].(map[string]interface{})
		matches := logic != "or"
		for _, cond := range conditions {
			fieldRef := canonicalValue(cond[0])
			value := fields[fieldRef]
			matched := matchesCondition(value, cond)
			if logic == "or" {
				matches = matches || matched
			} else {
				matches = matches && matched
			}
		}
		if matches {
			result = append(result, item)
		}
	}
	return result
}

func normalizeSortConfig(raw []interface{}) []map[string]interface{} {
	result := make([]map[string]interface{}, 0, len(raw))
	for _, item := range raw {
		if m, ok := item.(map[string]interface{}); ok {
			entry := map[string]interface{}{}
			if field, _ := m["field"].(string); field != "" {
				entry["field"] = field
			} else if field, _ := m["field_name"].(string); field != "" {
				entry["field"] = field
			}
			if desc, ok := m["desc"].(bool); ok {
				entry["desc"] = desc
			}
			result = append(result, entry)
		}
	}
	return result
}

func sortRecords(items []map[string]interface{}, sortConfig []interface{}) []map[string]interface{} {
	normalized := normalizeSortConfig(sortConfig)
	if len(normalized) == 0 {
		return items
	}
	sorted := append([]map[string]interface{}{}, items...)
	sort.SliceStable(sorted, func(i, j int) bool {
		leftFields, _ := sorted[i]["fields"].(map[string]interface{})
		rightFields, _ := sorted[j]["fields"].(map[string]interface{})
		for _, spec := range normalized {
			fieldRef, _ := spec["field"].(string)
			desc, _ := spec["desc"].(bool)
			cmp := compareScalar(leftFields[fieldRef], rightFields[fieldRef])
			if cmp == 0 {
				continue
			}
			if desc {
				return cmp > 0
			}
			return cmp < 0
		}
		return false
	})
	return sorted
}

func sleepBetweenBatches(index int, total int) {
	if index < total-1 {
		time.Sleep(600 * time.Millisecond)
	}
}

// ── Dashboard Block data_config normalization & validation ───────────

func normalizeDataConfig(cfg map[string]interface{}) map[string]interface{} {
	if cfg == nil {
		return nil
	}
	out := cloneMap(cfg)
	// series[].rollup → 大写
	if arr, ok := out["series"].([]interface{}); ok {
		for i, it := range arr {
			if m, ok := it.(map[string]interface{}); ok {
				if r, ok := m["rollup"].(string); ok && r != "" {
					m["rollup"] = strings.ToUpper(strings.TrimSpace(r))
				}
				arr[i] = m
			}
		}
		out["series"] = arr
	}
	// group_by.sort 的 type/order → 小写
	if gb, ok := out["group_by"].([]interface{}); ok {
		for i, g := range gb {
			if m, ok := g.(map[string]interface{}); ok {
				if md, ok := m["mode"].(string); ok {
					m["mode"] = strings.ToLower(strings.TrimSpace(md))
				}
				if sub, ok := m["sort"].(map[string]interface{}); ok {
					if t, ok := sub["type"].(string); ok {
						sub["type"] = strings.ToLower(strings.TrimSpace(t))
					}
					if o, ok := sub["order"].(string); ok {
						sub["order"] = strings.ToLower(strings.TrimSpace(o))
					}
					m["sort"] = sub
				}
				gb[i] = m
			}
		}
		out["group_by"] = gb
	}
	return out
}

func validateBlockDataConfig(blockType string, cfg map[string]interface{}) []string {
	var errs []string
	// table_name 必填
	if tn, _ := cfg["table_name"].(string); strings.TrimSpace(tn) == "" {
		errs = append(errs, "缺少必填字段 table_name")
	}
	// series 与 count_all 互斥且必有其一
	_, hasSeries := cfg["series"]
	_, hasCountAll := cfg["count_all"]
	if !(hasSeries || hasCountAll) {
		errs = append(errs, "series 与 count_all 二选一，至少提供其一")
	}
	if hasSeries && hasCountAll {
		errs = append(errs, "series 与 count_all 互斥，不可同时存在")
	}
	// series 校验
	if hasSeries {
		arr, ok := cfg["series"].([]interface{})
		if !ok || len(arr) == 0 {
			errs = append(errs, "series 必须是非空数组")
		} else {
			// rollup 支持：SUM / MAX / MIN / AVERAGE（不支持 COUNTA；计数请使用 count_all）
			allowed := map[string]bool{"SUM": true, "MAX": true, "MIN": true, "AVERAGE": true}
			for i, it := range arr {
				m, ok := it.(map[string]interface{})
				if !ok {
					errs = append(errs, fmt.Sprintf("series[%d] 必须是对象", i))
					continue
				}
				fn, _ := m["field_name"].(string)
				if strings.TrimSpace(fn) == "" {
					errs = append(errs, fmt.Sprintf("series[%d].field_name 不能为空", i))
				}
				r, _ := m["rollup"].(string)
				r = strings.ToUpper(strings.TrimSpace(r))
				if !allowed[r] {
					errs = append(errs, fmt.Sprintf("series[%d].rollup 不在允许枚举内: %s", i, r))
				}
			}
		}
	}
	// group_by 最多 2 个，字段名必填，sort 合法
	if gb, ok := cfg["group_by"].([]interface{}); ok {
		if len(gb) > 2 {
			errs = append(errs, "group_by 最多支持 2 个维度")
		}
		for i, g := range gb {
			m, ok := g.(map[string]interface{})
			if !ok {
				errs = append(errs, fmt.Sprintf("group_by[%d] 必须是对象", i))
				continue
			}
			fn, _ := m["field_name"].(string)
			if strings.TrimSpace(fn) == "" {
				errs = append(errs, fmt.Sprintf("group_by[%d].field_name 不能为空", i))
			}
			if sub, ok := m["sort"].(map[string]interface{}); ok {
				t, _ := sub["type"].(string)
				t = strings.ToLower(strings.TrimSpace(t))
				o, _ := sub["order"].(string)
				o = strings.ToLower(strings.TrimSpace(o))
				if t != "group" && t != "value" && t != "view" {
					errs = append(errs, fmt.Sprintf("group_by[%d].sort.type 仅支持 group|value|view", i))
				}
				if o != "asc" && o != "desc" {
					errs = append(errs, fmt.Sprintf("group_by[%d].sort.order 仅支持 asc|desc", i))
				}
			}
		}
	}
	// filter 基本结构
	if f, ok := cfg["filter"].(map[string]interface{}); ok {
		conj := strings.ToLower(strings.TrimSpace(fmt.Sprintf("%v", f["conjunction"])))
		if conj == "" {
			conj = "and"
		}
		if conj != "and" && conj != "or" {
			errs = append(errs, "filter.conjunction 仅支持 and|or")
		}
		if conds, ok := f["conditions"].([]interface{}); ok {
			allowedOps := map[string]bool{"is": true, "isnot": true, "contains": true, "doesnotcontain": true, "isempty": true, "isnotempty": true, "isgreater": true, "isgreaterequal": true, "isless": true, "islessequal": true}
			for i, it := range conds {
				m, ok := it.(map[string]interface{})
				if !ok {
					errs = append(errs, fmt.Sprintf("filter.conditions[%d] 必须是对象", i))
					continue
				}
				fn, _ := m["field_name"].(string)
				if strings.TrimSpace(fn) == "" {
					errs = append(errs, fmt.Sprintf("filter.conditions[%d].field_name 不能为空", i))
				}
				op, _ := m["operator"].(string)
				key := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(op), " ", ""))
				if !allowedOps[key] {
					errs = append(errs, fmt.Sprintf("filter.conditions[%d].operator 不支持: %s", i, op))
				}
				if key != "isempty" && key != "isnotempty" {
					if _, has := m["value"]; !has {
						errs = append(errs, fmt.Sprintf("filter.conditions[%d].value 缺失", i))
					}
				}
			}
		}
	}
	return errs
}

func formatDataConfigErrors(errs []string) error {
	if len(errs) == 0 {
		return nil
	}
	return fmt.Errorf("data_config 校验失败:\n- %s\n参考: skills/lark-base/references/dashboard-block-data-config.md", strings.Join(errs, "\n- "))
}
