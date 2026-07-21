// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package output

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"
)

// Known array field names for pagination.
var knownArrayFields = []string{
	"items", "files", "events", "rooms", "records", "nodes",
	"members", "departments", "calendar_list", "acl_list", "freebusy_list",
	"users",
}

// FindArrayField finds the primary array field in a response's data object.
// It first checks knownArrayFields in priority order, then falls back to
// the lexicographically smallest unknown array field for deterministic results.
func FindArrayField(data map[string]interface{}) string {
	for _, name := range knownArrayFields {
		if arr, ok := data[name]; ok {
			if _, isArr := arr.([]interface{}); isArr {
				return name
			}
		}
	}
	// Fallback: lexicographically first array field (deterministic)
	var candidates []string
	for k, v := range data {
		if _, isArr := v.([]interface{}); isArr {
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
			if items, ok := dataObj[field].([]interface{}); ok {
				return items
			}
		}
	}

	// Strategy 2: direct map — result[arrayField]
	// Covers shortcut-level data like {"members":[…], "total":5, "has_more":false}
	if field := FindArrayField(resultMap); field != "" {
		if items, ok := resultMap[field].([]interface{}); ok {
			return items
		}
	}

	return nil
}

// FormatValue formats a single response and writes it to w.
func FormatValue(w io.Writer, data interface{}, format Format) {
	err := WriteFormatted(w, data, format)
	switch {
	case err == nil:
		return
	case isOutputMarshalError(err) && format == FormatNDJSON:
		legacyStderrf("ndjson marshal error: %v\n", err)
	case isOutputMarshalError(err):
		legacyStderrf("json marshal error: %v\n", err)
	}
}

// WriteFormatted formats a single response and returns marshal or write errors.
func WriteFormatted(w io.Writer, data interface{}, format Format) error {
	data = toGeneric(data)
	switch format {
	case FormatNDJSON:
		items := ExtractItems(data)
		if items != nil {
			return WriteNDJSON(w, items)
		}
		return WriteNDJSON(w, data)

	case FormatTable:
		items := ExtractItems(data)
		if items != nil {
			return WriteTable(w, items)
		}
		return WriteTable(w, data)

	case FormatCSV:
		items := ExtractItems(data)
		if items != nil {
			return WriteCSV(w, items)
		}
		return WriteCSV(w, data)

	default: // FormatJSON
		return WriteJSON(w, data)
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
	err := pf.WritePage(data)
	if isOutputMarshalError(err) && (pf.Format == FormatJSON || pf.Format == FormatNDJSON) {
		legacyStderrf("ndjson marshal error: %v\n", err)
	}
}

// WritePage formats one page of items and returns marshal or write errors.
func (pf *PaginatedFormatter) WritePage(data interface{}) error {
	switch pf.Format {
	case FormatJSON, FormatNDJSON:
		if arr, ok := data.([]interface{}); ok {
			return WriteNDJSON(pf.W, arr)
		}
		return WriteNDJSON(pf.W, data)

	case FormatTable:
		return pf.formatStructuredPage(data, func(w io.Writer, rows []map[string]string, cols []string, isFirst bool) error {
			widths := computeColumnWidths(rows, cols)
			if isFirst {
				if err := writeHeader(w, cols, widths); err != nil {
					return err
				}
			}
			for _, row := range rows {
				if err := writeRow(w, row, cols, widths); err != nil {
					return err
				}
			}
			return nil
		})

	case FormatCSV:
		return pf.formatStructuredPage(data, func(w io.Writer, rows []map[string]string, cols []string, isFirst bool) error {
			return writeCSVRows(w, rows, cols, isFirst)
		})
	}
	return nil
}

// formatStructuredPage handles column-locking logic shared by table and csv.
func (pf *PaginatedFormatter) formatStructuredPage(data interface{}, emit func(io.Writer, []map[string]string, []string, bool) error) error {
	rows, pageCols, isList := prepareRows(data)
	if len(rows) == 0 {
		if pf.isFirstPage && isList {
			_, err := fmt.Fprintln(pf.W, "(empty)")
			return err
		}
		return nil
	}

	if pf.isFirstPage {
		// Lock columns from first page
		pf.cols = pageCols
		pf.isFirstPage = false
		return emit(pf.W, rows, pf.cols, true)
	} else {
		// Reuse first page's columns — missing keys become empty, extra keys ignored
		return emit(pf.W, rows, pf.cols, false)
	}
}
