// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package recordexport converts the Base OpenAPI record matrix into a stable,
// typed row model that output formats can share. The matrix is intentionally
// parsed only once at the package boundary; exporters never depend on loose
// map keys or parallel arrays.
package recordexport

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"
)

const RecordIDColumnName = "record_id"

// ValueKind is the outer decoded JSON representation used for lightweight
// shape validation. Nested objects remain map[string]any values.
type ValueKind string

const (
	KindString  ValueKind = "string"
	KindNumber  ValueKind = "number"
	KindBoolean ValueKind = "boolean"
	KindObject  ValueKind = "object"
)

type fieldTypeSpec struct {
	kind         ValueKind
	repeated     bool
	physicalType string
}

// specForFieldType is the single mapping from the OpenAPI field_type contract
// to NDJSON outer-shape validation and manifest physical types. Record values
// remain decoded JSON values; the CLI does not construct Go structs for cells.
func specForFieldType(fieldType string) (fieldTypeSpec, bool) {
	switch fieldType {
	case "text", "formula", "lookup", "auto_number", "datetime", "created_at", "updated_at", "not_support":
		return fieldTypeSpec{kind: KindString, physicalType: "string|null"}, true
	case "number":
		return fieldTypeSpec{kind: KindNumber, physicalType: "number|null"}, true
	case "checkbox":
		return fieldTypeSpec{kind: KindBoolean, physicalType: "boolean"}, true
	case "location":
		return fieldTypeSpec{
			kind: KindObject, physicalType: "struct<lng number, lat number, full_address string>|null",
		}, true
	case "select":
		return fieldTypeSpec{kind: KindString, repeated: true, physicalType: "array<string>"}, true
	case "user", "group_chat", "created_by", "updated_by":
		return fieldTypeSpec{
			kind: KindObject, repeated: true,
			physicalType: "array<struct<id string, name string>>",
		}, true
	case "link":
		return fieldTypeSpec{
			kind: KindObject, repeated: true,
			physicalType: "array<struct<id string>>",
		}, true
	case "attachment":
		return fieldTypeSpec{
			kind: KindObject, repeated: true,
			physicalType: "array<struct<file_token string, size number, name string>>",
		}, true
	default:
		return fieldTypeSpec{}, false
	}
}

// Column is the format-neutral schema used by all record exporters. FieldID
// and FieldType are empty only for the synthetic record_id system column.
type Column struct {
	Name      string
	FieldID   string
	FieldType string
	System    bool
}

// PhysicalType renders the compact, engine-neutral type used in manifests.
func (c Column) PhysicalType() string {
	if c.System {
		return "string"
	}
	spec, ok := specForFieldType(c.FieldType)
	if !ok {
		return ""
	}
	return spec.physicalType
}

// Record stores decoded JSON values in the same order as Dataset.Columns.
// Object cells remain map[string]any and are not converted into Go structs.
type Record struct {
	Values []any
}

// Dataset is the stable tabular model consumed by NDJSON now and by future
// JSON-array or Parquet exporters. SourceColumns retains the complete OpenAPI
// schema for cross-page consistency checks, while Columns contains the actual
// exported columns (including the synthetic record_id column).
type Dataset struct {
	Timezone      string
	SourceColumns []Column
	Columns       []Column
	Records       []Record
}

// IgnoredField mirrors the structured OpenAPI read warning.
type IgnoredField struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

// Page contains one parsed matrix page and its query metadata.
type Page struct {
	Dataset        Dataset
	Rev            *int64
	HasMore        bool
	IgnoredFields  []IgnoredField
	QueryContext   map[string]any
	RecordNotFound []string
}

// MatrixError means the OpenAPI response does not satisfy its parallel-array
// contract. The command boundary wraps it as a typed invalid-response error.
type MatrixError struct {
	Reason string
}

func (e *MatrixError) Error() string { return "invalid record matrix: " + e.Reason }

// SchemaChangedError protects a multi-page export from mixing schemas.
type SchemaChangedError struct {
	Reason string
}

func (e *SchemaChangedError) Error() string {
	return "record schema changed between pages: " + e.Reason
}

// ParseMatrix converts the current OpenAPI matrix shape into a typed page.
func ParseMatrix(data map[string]any) (Page, error) {
	timezone, ok := data["timezone"].(string)
	if !ok || strings.TrimSpace(timezone) == "" {
		return Page{}, &MatrixError{Reason: "timezone must be a non-empty string"}
	}
	fields, err := stringList(data["fields"], "fields")
	if err != nil {
		return Page{}, err
	}
	fieldIDs, err := stringList(data["field_id_list"], "field_id_list")
	if err != nil {
		return Page{}, err
	}
	fieldTypes, err := stringList(data["field_type_list"], "field_type_list")
	if err != nil {
		return Page{}, err
	}
	if len(fields) != len(fieldIDs) || len(fields) != len(fieldTypes) {
		return Page{}, &MatrixError{Reason: fmt.Sprintf(
			"fields, field_id_list, and field_type_list lengths differ (%d, %d, %d)",
			len(fields), len(fieldIDs), len(fieldTypes),
		)}
	}

	sourceColumns := make([]Column, 0, len(fields))
	exportColumns := []Column{{
		Name: RecordIDColumnName, System: true,
	}}
	exportSourceIndexes := make([]int, 0, len(fields))
	for index := range fields {
		column, err := sourceColumn(fields[index], fieldIDs[index], fieldTypes[index])
		if err != nil {
			return Page{}, err
		}
		sourceColumns = append(sourceColumns, column)
		// The system join key intentionally wins over a same-named Base field.
		if column.Name == RecordIDColumnName {
			continue
		}
		exportColumns = append(exportColumns, column)
		exportSourceIndexes = append(exportSourceIndexes, index)
	}

	recordIDs, err := stringList(data["record_id_list"], "record_id_list")
	if err != nil {
		return Page{}, err
	}
	rawRows, ok := data["data"].([]any)
	if !ok {
		return Page{}, &MatrixError{Reason: "data must be an array of rows"}
	}
	if len(recordIDs) != len(rawRows) {
		return Page{}, &MatrixError{Reason: fmt.Sprintf(
			"record_id_list and data lengths differ (%d, %d)", len(recordIDs), len(rawRows),
		)}
	}

	records := make([]Record, 0, len(rawRows))
	for rowIndex, rawRow := range rawRows {
		row, ok := rawRow.([]any)
		if !ok {
			return Page{}, &MatrixError{Reason: fmt.Sprintf("data row %d must be an array", rowIndex+1)}
		}
		if len(row) != len(sourceColumns) {
			return Page{}, &MatrixError{Reason: fmt.Sprintf(
				"data row %d has %d cells; schema has %d columns", rowIndex+1, len(row), len(sourceColumns),
			)}
		}
		values := make([]any, 1, len(exportColumns))
		values[0] = recordIDs[rowIndex]
		for _, sourceIndex := range exportSourceIndexes {
			value, err := normalizeCell(sourceColumns[sourceIndex], row[sourceIndex], timezone)
			if err != nil {
				return Page{}, &MatrixError{Reason: fmt.Sprintf(
					"row %d column %q: %v", rowIndex+1, sourceColumns[sourceIndex].Name, err,
				)}
			}
			values = append(values, value)
		}
		records = append(records, Record{Values: values})
	}

	page := Page{Dataset: Dataset{
		Timezone: timezone, SourceColumns: sourceColumns, Columns: exportColumns, Records: records,
	}}
	if raw, exists := data["rev"]; exists && raw != nil {
		value, ok := raw.(json.Number)
		if !ok {
			return Page{}, &MatrixError{Reason: fmt.Sprintf("rev must be an integer, got %T", raw)}
		}
		rev, err := value.Int64()
		if err != nil || rev < 0 {
			return Page{}, &MatrixError{Reason: "rev must be a non-negative integer"}
		}
		page.Rev = &rev
	}
	if raw, exists := data["has_more"]; exists {
		value, ok := raw.(bool)
		if !ok {
			return Page{}, &MatrixError{Reason: "has_more must be a boolean"}
		}
		page.HasMore = value
	}
	if raw, exists := data["query_context"]; exists && raw != nil {
		value, ok := raw.(map[string]any)
		if !ok {
			return Page{}, &MatrixError{Reason: "query_context must be an object"}
		}
		page.QueryContext = value
	}
	if raw, exists := data["ignored_fields"]; exists && raw != nil {
		page.IgnoredFields, err = ignoredFieldList(raw)
		if err != nil {
			return Page{}, err
		}
	}
	if raw, exists := data["record_not_found"]; exists && raw != nil {
		page.RecordNotFound, err = stringList(raw, "record_not_found")
		if err != nil {
			return Page{}, err
		}
	}
	return page, nil
}

// AppendPage appends rows only after proving that the complete source schema
// and timezone still match the first page.
func (d *Dataset) AppendPage(page Page) error {
	if d.Timezone != page.Dataset.Timezone {
		return &SchemaChangedError{Reason: fmt.Sprintf("timezone changed from %q to %q", d.Timezone, page.Dataset.Timezone)}
	}
	if !reflect.DeepEqual(d.SourceColumns, page.Dataset.SourceColumns) {
		return &SchemaChangedError{Reason: "fields, field IDs, field types, or field order changed"}
	}
	d.Records = append(d.Records, page.Dataset.Records...)
	return nil
}

func sourceColumn(name, fieldID, fieldType string) (Column, error) {
	if _, ok := specForFieldType(fieldType); !ok {
		return Column{}, &MatrixError{Reason: fmt.Sprintf("field %q has unsupported field type %q", name, fieldType)}
	}
	return Column{Name: name, FieldID: fieldID, FieldType: fieldType}, nil
}

func normalizeCell(column Column, value any, timezone string) (any, error) {
	spec, ok := specForFieldType(column.FieldType)
	if !ok {
		return nil, newDetailErrorf("unsupported field type %q", column.FieldType)
	}
	if spec.repeated {
		if value == nil {
			return []any{}, nil
		}
		items, ok := value.([]any)
		if !ok {
			return nil, newDetailErrorf("expected array, got %T", value)
		}
		for index, item := range items {
			switch spec.kind {
			case KindString:
				if _, ok := item.(string); !ok {
					return nil, newDetailErrorf("array item %d must be a string, got %T", index+1, item)
				}
			case KindObject:
				if _, ok := item.(map[string]any); !ok {
					return nil, newDetailErrorf("array item %d must be an object, got %T", index+1, item)
				}
			}
		}
		return items, nil
	}
	if value == nil {
		if column.FieldType == "checkbox" {
			return false, nil
		}
		return nil, nil
	}
	switch spec.kind {
	case KindString:
		text, ok := value.(string)
		if !ok {
			return nil, newDetailErrorf("expected string, got %T", value)
		}
		if column.FieldType == "datetime" || column.FieldType == "created_at" || column.FieldType == "updated_at" {
			return normalizeDateTime(text, timezone)
		}
		return text, nil
	case KindNumber:
		switch value.(type) {
		case json.Number, float64, float32, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
			return value, nil
		default:
			return nil, newDetailErrorf("expected number, got %T", value)
		}
	case KindBoolean:
		if _, ok := value.(bool); !ok {
			return nil, newDetailErrorf("expected boolean, got %T", value)
		}
		return value, nil
	case KindObject:
		if _, ok := value.(map[string]any); !ok {
			return nil, newDetailErrorf("expected object, got %T", value)
		}
		return value, nil
	default:
		return nil, newDetailErrorf("unsupported value kind %q", spec.kind)
	}
}

func normalizeDateTime(value, timezone string) (string, error) {
	if _, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return value, nil
	}
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return "", wrapDetailErrorf(err, "invalid timezone %q", timezone)
	}
	parsed, err := time.ParseInLocation("2006-01-02 15:04:05", value, location)
	if err != nil {
		return "", newDetailErrorf("expected YYYY-MM-DD HH:mm:ss or RFC3339, got %q", value)
	}
	return parsed.Format(time.RFC3339), nil
}

func stringList(raw any, name string) ([]string, error) {
	items, ok := raw.([]any)
	if !ok {
		return nil, &MatrixError{Reason: name + " must be an array of strings"}
	}
	values := make([]string, 0, len(items))
	for index, item := range items {
		value, ok := item.(string)
		if !ok {
			return nil, &MatrixError{Reason: fmt.Sprintf("%s item %d must be a string", name, index+1)}
		}
		values = append(values, value)
	}
	return values, nil
}

func ignoredFieldList(raw any) ([]IgnoredField, error) {
	items, ok := raw.([]any)
	if !ok {
		return nil, &MatrixError{Reason: "ignored_fields must be an array"}
	}
	fields := make([]IgnoredField, 0, len(items))
	for index, item := range items {
		object, ok := item.(map[string]any)
		if !ok {
			return nil, &MatrixError{Reason: fmt.Sprintf("ignored_fields item %d must be an object", index+1)}
		}
		field := IgnoredField{}
		var valid bool
		field.ID, valid = object["id"].(string)
		if !valid {
			return nil, &MatrixError{Reason: fmt.Sprintf("ignored_fields item %d id must be a string", index+1)}
		}
		field.Name, valid = object["name"].(string)
		if !valid {
			return nil, &MatrixError{Reason: fmt.Sprintf("ignored_fields item %d name must be a string", index+1)}
		}
		field.Reason, valid = object["reason"].(string)
		if !valid {
			return nil, &MatrixError{Reason: fmt.Sprintf("ignored_fields item %d reason must be a string", index+1)}
		}
		fields = append(fields, field)
	}
	return fields, nil
}
