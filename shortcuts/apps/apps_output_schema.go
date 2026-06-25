// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

type appsCellFormatter func(interface{}) string

type appsOutputColumn struct {
	Key    string
	Label  string
	Value  func(map[string]interface{}) interface{}
	Format appsCellFormatter
}

type appsOutputSchema struct {
	Columns []appsOutputColumn
	Strict  bool
}

func appsProjectRows(rows []map[string]interface{}, schema appsOutputSchema) []map[string]interface{} {
	out := make([]map[string]interface{}, 0, len(rows))
	for _, row := range rows {
		out = append(out, appsProjectRow(row, schema))
	}
	return out
}

func appsProjectRow(row map[string]interface{}, schema appsOutputSchema) map[string]interface{} {
	out := make(map[string]interface{}, len(schema.Columns))
	declared := make(map[string]struct{}, len(schema.Columns))
	for _, col := range schema.Columns {
		if col.Key == "" {
			continue
		}
		declared[col.Key] = struct{}{}
		value := row[col.Key]
		if col.Value != nil {
			value = col.Value(row)
		}
		if value != nil {
			out[col.Key] = value
		}
	}
	if !schema.Strict {
		for key, value := range row {
			if _, ok := declared[key]; !ok {
				out[key] = value
			}
		}
	}
	return out
}

func appsPrintSchemaTable(w io.Writer, rows []map[string]interface{}, schema appsOutputSchema) {
	if len(rows) == 0 {
		fmt.Fprintln(w, "(no data)")
		return
	}
	headers := make([]string, 0, len(schema.Columns))
	for _, col := range schema.Columns {
		if col.Key == "" {
			continue
		}
		headers = append(headers, appsColumnLabel(col))
	}
	if len(headers) == 0 {
		fmt.Fprintln(w, "(no data)")
		return
	}
	matrix := make([][]string, 0, len(rows)+1)
	matrix = append(matrix, headers)
	for _, row := range rows {
		line := make([]string, 0, len(schema.Columns))
		for _, col := range schema.Columns {
			if col.Key == "" {
				continue
			}
			value := row[col.Key]
			if col.Value != nil {
				value = col.Value(row)
			}
			line = append(line, appsFormatCell(value, col.Format))
		}
		matrix = append(matrix, line)
	}
	widths := appsColumnWidths(matrix)
	for i, row := range matrix {
		cells := make([]string, len(row))
		for j, cell := range row {
			cells[j] = appsPad(cell, widths[j])
		}
		fmt.Fprintln(w, strings.TrimRight(strings.Join(cells, "  "), " "))
		if i == 0 {
			sep := make([]string, len(widths))
			for j, width := range widths {
				sep[j] = strings.Repeat("─", width)
			}
			fmt.Fprintln(w, strings.Join(sep, "  "))
		}
	}
}

func appsColumnLabel(col appsOutputColumn) string {
	if col.Label != "" {
		return col.Label
	}
	return col.Key
}

func appsFormatCell(value interface{}, formatter appsCellFormatter) string {
	if formatter != nil {
		return formatter(value)
	}
	return appsDefaultCell(value)
}

func appsDefaultCell(value interface{}) string {
	if value == nil {
		return ""
	}
	switch v := value.(type) {
	case string:
		return v
	case json.Number:
		return v.String()
	case bool:
		return strconv.FormatBool(v)
	case int:
		return strconv.Itoa(v)
	case int8, int16, int32, int64:
		return fmt.Sprintf("%d", v)
	case uint, uint8, uint16, uint32, uint64:
		return fmt.Sprintf("%d", v)
	case float32:
		return appsFormatFloat(float64(v))
	case float64:
		return appsFormatFloat(v)
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprint(v)
		}
		return string(b)
	}
}

func appsFormatFloat(value float64) string {
	if math.Trunc(value) == value {
		return strconv.FormatInt(int64(value), 10)
	}
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func appsColumnWidths(matrix [][]string) []int {
	if len(matrix) == 0 {
		return nil
	}
	widths := make([]int, len(matrix[0]))
	for _, row := range matrix {
		for i, cell := range row {
			if width := utf8.RuneCountInString(cell); width > widths[i] {
				widths[i] = width
			}
		}
	}
	return widths
}

func appsPad(s string, width int) string {
	delta := width - utf8.RuneCountInString(s)
	if delta <= 0 {
		return s
	}
	return s + strings.Repeat(" ", delta)
}

func appsFormatNS(layout string) appsCellFormatter {
	return func(value interface{}) string {
		ns, ok := appsInt64Value(value)
		if !ok || ns <= 0 {
			return appsDefaultCell(value)
		}
		return time.Unix(0, ns).Local().Format(layout)
	}
}

func appsFormatSec(layout string) appsCellFormatter {
	return func(value interface{}) string {
		sec, ok := appsInt64Value(value)
		if !ok || sec <= 0 {
			return appsDefaultCell(value)
		}
		return time.Unix(sec, 0).Local().Format(layout)
	}
}

func appsFormatDurationMS(value interface{}) string {
	ms, ok := appsFloat64Value(value)
	if !ok || ms < 0 {
		return appsDefaultCell(value)
	}
	switch {
	case ms < 1:
		return fmt.Sprintf("%.2fms", ms)
	case ms < 1000:
		return fmt.Sprintf("%.0fms", ms)
	case ms < 60000:
		return fmt.Sprintf("%.2fs", ms/1000)
	case ms < 3600000:
		return fmt.Sprintf("%.1fm", ms/60000)
	default:
		return fmt.Sprintf("%.1fh", ms/3600000)
	}
}

func appsInt64Value(value interface{}) (int64, bool) {
	switch v := value.(type) {
	case int:
		return int64(v), true
	case int8:
		return int64(v), true
	case int16:
		return int64(v), true
	case int32:
		return int64(v), true
	case int64:
		return v, true
	case uint:
		return appsUint64ToInt64(uint64(v))
	case uint8:
		return int64(v), true
	case uint16:
		return int64(v), true
	case uint32:
		return int64(v), true
	case uint64:
		return appsUint64ToInt64(v)
	case float32:
		f := float64(v)
		if math.Trunc(f) == f && f <= float64(math.MaxInt64) && f >= float64(math.MinInt64) {
			return int64(f), true
		}
	case float64:
		if math.Trunc(v) == v && v <= float64(math.MaxInt64) && v >= float64(math.MinInt64) {
			return int64(v), true
		}
	case json.Number:
		if n, err := v.Int64(); err == nil {
			return n, true
		}
		if f, err := v.Float64(); err == nil && math.Trunc(f) == f {
			return int64(f), true
		}
	case string:
		raw := strings.TrimSpace(v)
		if n, err := strconv.ParseInt(raw, 10, 64); err == nil {
			return n, true
		}
		if f, err := strconv.ParseFloat(raw, 64); err == nil && math.Trunc(f) == f {
			return int64(f), true
		}
	}
	return 0, false
}

func appsFloat64Value(value interface{}) (float64, bool) {
	switch v := value.(type) {
	case int:
		return float64(v), true
	case int8:
		return float64(v), true
	case int16:
		return float64(v), true
	case int32:
		return float64(v), true
	case int64:
		return float64(v), true
	case uint:
		return float64(v), true
	case uint8:
		return float64(v), true
	case uint16:
		return float64(v), true
	case uint32:
		return float64(v), true
	case uint64:
		return float64(v), true
	case float32:
		return float64(v), true
	case float64:
		return v, true
	case json.Number:
		f, err := v.Float64()
		return f, err == nil
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		return f, err == nil
	default:
		return 0, false
	}
}

func appsUint64ToInt64(value uint64) (int64, bool) {
	if value > uint64(math.MaxInt64) {
		return 0, false
	}
	return int64(value), true
}

func appsAttrValue(key string) func(map[string]interface{}) interface{} {
	return func(row map[string]interface{}) interface{} {
		return appsAttributeValue(row["attributes"], key)
	}
}

func appsAttributeValue(raw interface{}, key string) interface{} {
	switch attrs := raw.(type) {
	case map[string]interface{}:
		return attrs[key]
	case []interface{}:
		for _, rawItem := range attrs {
			item, ok := rawItem.(map[string]interface{})
			if !ok {
				continue
			}
			itemKey := strings.TrimSpace(fmt.Sprint(firstObservabilityValue(item, "key", "name")))
			if itemKey == key {
				return firstObservabilityValue(item, "value")
			}
		}
	case []map[string]interface{}:
		for _, item := range attrs {
			itemKey := strings.TrimSpace(fmt.Sprint(firstObservabilityValue(item, "key", "name")))
			if itemKey == key {
				return firstObservabilityValue(item, "value")
			}
		}
	}
	return nil
}
