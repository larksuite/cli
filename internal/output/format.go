// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package output

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"sort"
)

// Known array field names for pagination.
var knownArrayFields = []string{
	"items", "files", "events", "rooms", "records", "nodes",
	"members", "departments", "calendar_list", "acl_list", "freebusy_list",
}

// isSliceLike reports whether v is any kind of slice (e.g. []interface{},
// []map[string]interface{}, []string, etc.), using reflect so that the
// check is not limited to a single concrete slice type.
func isSliceLike(v interface{}) bool {
	if v == nil {
		return false
	}
	return reflect.TypeOf(v).Kind() == reflect.Slice
}

// toGenericSlice converts any slice type to []interface{} by re-boxing each
// element. This only changes the outer container type; individual elements
// retain their original dynamic type (e.g. map[string]interface{} stays as-is).
// Returns nil if v is not a slice.
func toGenericSlice(v interface{}) []interface{} {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Slice {
		return nil
	}
	out := make([]interface{}, rv.Len())
	for i := 0; i < rv.Len(); i++ {
		out[i] = rv.Index(i).Interface()
	}
	return out
}

// FindArrayField finds the primary array field in a response's data object.
// It first checks knownArrayFields in priority order, then falls back to
// the lexicographically smallest unknown array field for deterministic results.
func FindArrayField(data map[string]interface{}) string {
	for _, name := range knownArrayFields {
		if arr, ok := data[name]; ok {
			if isSliceLike(arr) {
				return name
			}
		}
	}
	// Fallback: lexicographically first array field (deterministic)
	var candidates []string
	for k, v := range data {
		if isSliceLike(v) {
			candidates = append(candidates, k)
		}
	}
	if len(candidates) > 0 {
		sort.Strings(candidates)
		return candidates[0]
	}
	return ""
}

// toGeneric normalises any Go value (structs, typed slices, …) into
// plain map[string]interface{} / []interface{} via a JSON round-trip so
// that subsequent type assertions in format handlers work uniformly.
func toGeneric(v interface{}) interface{} {
	switch v.(type) {
	case map[string]interface{}, []interface{}, nil:
		return v // already generic
	}
	b, err := json.Marshal(v)
	if err != nil {
		return v
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber() // preserve int64 precision (avoid float64 truncation)
	var out interface{}
	if err := dec.Decode(&out); err != nil {
		return v
	}
	return out
}

// ExtractItems extracts the data array from a response.
// It tries two strategies in order:
//  1. Lark API envelope: result["data"][arrayField]  (e.g. {"code":0,"data":{"items":[…]}})
//  2. Direct map: result[arrayField]                 (e.g. {"members":[…],"total":5})
//
// If data is already a plain []interface{}, it is returned as-is.
func ExtractItems(data interface{}) []interface{} {
	resultMap, ok := data.(map[string]interface{})
	if !ok {
		if arr, ok := data.([]interface{}); ok {
			return arr
		}
		return nil
	}

	// Strategy 1: Lark API envelope — result["data"][arrayField]
	if dataObj, ok := resultMap["data"].(map[string]interface{}); ok {
		if field := FindArrayField(dataObj); field != "" {
			if items := toGenericSlice(dataObj[field]); items != nil {
				return items
			}
		}
	}

	// Strategy 2: direct map — result[arrayField]
	// Covers shortcut-level data like {"members":[…], "total":5, "has_more":false}
	if field := FindArrayField(resultMap); field != "" {
		if items := toGenericSlice(resultMap[field]); items != nil {
			return items
		}
	}

	return nil
}

// FormatValue formats a single response and writes it to w.
func FormatValue(w io.Writer, data interface{}, format Format) {
	data = toGeneric(data)
	switch format {
	case FormatNDJSON:
		items := ExtractItems(data)
		if items != nil {
			PrintNdjson(w, items)
		} else {
			PrintNdjson(w, data)
		}

	case FormatTable:
		items := ExtractItems(data)
		if items != nil {
			FormatAsTable(w, items)
		} else {
			FormatAsTable(w, data)
		}

	case FormatCSV:
		items := ExtractItems(data)
		if items != nil {
			FormatAsCSV(w, items)
		} else {
			FormatAsCSV(w, data)
		}

	default: // FormatJSON
		PrintJson(w, data)
	}
}

// PaginatedFormatter holds state across paginated calls to ensure
// consistent columns (table/csv use the first page's columns for all pages).
type PaginatedFormatter struct {
	W           io.Writer
	Format      Format
	isFirstPage bool
	cols        []string // locked after first page
}

// NewPaginatedFormatter creates a formatter that tracks pagination state.
func NewPaginatedFormatter(w io.Writer, format Format) *PaginatedFormatter {
	return &PaginatedFormatter{W: w, Format: format, isFirstPage: true}
}

// FormatPage formats one page of items.
func (pf *PaginatedFormatter) FormatPage(data interface{}) {
	switch pf.Format {
	case FormatJSON, FormatNDJSON:
		if arr, ok := data.([]interface{}); ok {
			PrintNdjson(pf.W, arr)
		} else {
			PrintNdjson(pf.W, data)
		}

	case FormatTable:
		pf.formatStructuredPage(data, func(w io.Writer, rows []map[string]string, cols []string, isFirst bool) {
			widths := computeColumnWidths(rows, cols)
			if isFirst {
				writeHeader(w, cols, widths)
			}
			for _, row := range rows {
				writeRow(w, row, cols, widths)
			}
		})

	case FormatCSV:
		pf.formatStructuredPage(data, func(w io.Writer, rows []map[string]string, cols []string, isFirst bool) {
			writeCSVRows(w, rows, cols, isFirst)
		})
	}
}

// formatStructuredPage handles column-locking logic shared by table and csv.
func (pf *PaginatedFormatter) formatStructuredPage(data interface{}, emit func(io.Writer, []map[string]string, []string, bool)) {
	rows, pageCols, isList := prepareRows(data)
	if len(rows) == 0 {
		if pf.isFirstPage && isList {
			fmt.Fprintln(pf.W, "(empty)")
		}
		return
	}

	if pf.isFirstPage {
		// Lock columns from first page
		pf.cols = pageCols
		pf.isFirstPage = false
		emit(pf.W, rows, pf.cols, true)
	} else {
		// Reuse first page's columns — missing keys become empty, extra keys ignored
		emit(pf.W, rows, pf.cols, false)
	}
}
