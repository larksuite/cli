// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package recordexport

import (
	"encoding/json"
	"io"
	"time"
	"unicode/utf8"
)

const (
	ManifestVersion = "v1"
	FormatNDJSON    = "ndjson"
	emptyColumnHint = "All values are empty; type inference may be ambiguous. Avoid analyzing this column unless required."
	maxExampleRunes = 128
	maxExampleItems = 3
)

// ColumnManifest is the user-facing description of one actual data column.
type ColumnManifest struct {
	FieldID          string      `json:"field_id,omitempty"`
	FieldType        string      `json:"field_type,omitempty"`
	PhysicalType     string      `json:"physical_type"`
	Stats            ColumnStats `json:"stats"`
	Example          any         `json:"example,omitempty"`
	ExampleTruncated bool        `json:"example_truncated,omitempty"`
	Hint             string      `json:"hint,omitempty"`
}

// ColumnStats contains only the metrics that are meaningful for a column's
// Base field type. All metrics describe the records in this export.
type ColumnStats struct {
	NullCount  *int     `json:"null_count,omitempty"`
	EmptyCount *int     `json:"empty_count,omitempty"`
	TrueCount  *int     `json:"true_count,omitempty"`
	Min        any      `json:"min,omitempty"`
	Max        any      `json:"max,omitempty"`
	Avg        *float64 `json:"avg,omitempty"`
	MaxLength  *int     `json:"max_length,omitempty"`
	AvgLength  *float64 `json:"avg_length,omitempty"`
}

// Manifest describes one exported artifact and the exact query boundary that
// produced it. Format is explicit so the same structure can describe future
// JSON-array and Parquet outputs.
type Manifest struct {
	ManifestVersion     string                    `json:"manifest_version"`
	Format              string                    `json:"format"`
	BaseToken           string                    `json:"base_token"`
	TableID             string                    `json:"table_id"`
	Rev                 *int64                    `json:"rev,omitempty"`
	Timezone            string                    `json:"timezone"`
	QueryContext        map[string]any            `json:"query_context,omitempty"`
	Offset              int                       `json:"offset,omitempty"`
	RequestedLimit      int                       `json:"requested_limit,omitempty"`
	RecordsCount        int                       `json:"records_count"`
	PageCount           int                       `json:"page_count"`
	HasMore             bool                      `json:"has_more"`
	NextOffset          *int                      `json:"next_offset,omitempty"`
	RecordFile          string                    `json:"record_file"`
	RecordFileSizeBytes int64                     `json:"record_file_size_bytes"`
	ManifestFile        string                    `json:"manifest_file"`
	Columns             map[string]ColumnManifest `json:"columns"`
	IgnoredFields       []IgnoredField            `json:"ignored_fields,omitempty"`
	RecordNotFound      []string                  `json:"record_not_found,omitempty"`
}

// ManifestOptions carries query and file details that are outside Dataset.
type ManifestOptions struct {
	BaseToken           string
	TableID             string
	Rev                 *int64
	QueryContext        map[string]any
	Offset              int
	RequestedLimit      int
	PageCount           int
	HasMore             bool
	RecordFile          string
	RecordFileSizeBytes int64
	ManifestFile        string
	IgnoredFields       []IgnoredField
	RecordNotFound      []string
}

// MinimalManifest is the stable low-token stdout result.
type MinimalManifest struct {
	RecordFile          string `json:"record_file"`
	RecordFileSizeBytes int64  `json:"record_file_size_bytes"`
	ManifestFile        string `json:"manifest_file"`
	RecordsCount        int    `json:"records_count"`
	HasMore             bool   `json:"has_more"`
}

func BuildManifest(dataset Dataset, opts ManifestOptions) Manifest {
	manifest := Manifest{
		ManifestVersion:     ManifestVersion,
		Format:              FormatNDJSON,
		BaseToken:           opts.BaseToken,
		TableID:             opts.TableID,
		Rev:                 opts.Rev,
		Timezone:            dataset.Timezone,
		QueryContext:        opts.QueryContext,
		Offset:              opts.Offset,
		RequestedLimit:      opts.RequestedLimit,
		RecordsCount:        len(dataset.Records),
		PageCount:           opts.PageCount,
		HasMore:             opts.HasMore,
		RecordFile:          opts.RecordFile,
		RecordFileSizeBytes: opts.RecordFileSizeBytes,
		ManifestFile:        opts.ManifestFile,
		Columns:             make(map[string]ColumnManifest, len(dataset.Columns)),
		IgnoredFields:       opts.IgnoredFields,
		RecordNotFound:      opts.RecordNotFound,
	}
	if opts.HasMore {
		nextOffset := opts.Offset + len(dataset.Records)
		manifest.NextOffset = &nextOffset
	}
	for columnIndex, column := range dataset.Columns {
		metadata := ColumnManifest{
			FieldID:      column.FieldID,
			FieldType:    column.FieldType,
			PhysicalType: column.PhysicalType(),
			Stats:        buildColumnStats(dataset.Records, columnIndex, column),
		}
		if example, truncated, found := bestExample(dataset.Records, columnIndex); found {
			metadata.Example = example
			metadata.ExampleTruncated = truncated
		} else {
			metadata.Hint = emptyColumnHint
		}
		manifest.Columns[column.Name] = metadata
	}
	return manifest
}

func buildColumnStats(records []Record, columnIndex int, column Column) ColumnStats {
	if column.System {
		maxLength := 0
		for _, record := range records {
			if columnIndex >= len(record.Values) {
				continue
			}
			value, ok := record.Values[columnIndex].(string)
			if ok && utf8.RuneCountInString(value) > maxLength {
				maxLength = utf8.RuneCountInString(value)
			}
		}
		return ColumnStats{MaxLength: &maxLength}
	}

	spec, ok := specForFieldType(column.FieldType)
	if !ok {
		return ColumnStats{}
	}
	if spec.repeated {
		emptyCount, maxLength, totalLength := 0, 0, 0
		for _, record := range records {
			if columnIndex >= len(record.Values) {
				continue
			}
			items, ok := record.Values[columnIndex].([]any)
			if !ok {
				continue
			}
			length := len(items)
			totalLength += length
			if length == 0 {
				emptyCount++
			}
			if length > maxLength {
				maxLength = length
			}
		}
		avgLength := 0.0
		if len(records) > 0 {
			avgLength = float64(totalLength) / float64(len(records))
		}
		return ColumnStats{
			EmptyCount: &emptyCount,
			MaxLength:  &maxLength,
			AvgLength:  &avgLength,
		}
	}

	switch column.FieldType {
	case "number":
		nullCount, valueCount := 0, 0
		var minValue, maxValue, total float64
		for _, record := range records {
			if columnIndex >= len(record.Values) || record.Values[columnIndex] == nil {
				nullCount++
				continue
			}
			value, ok := numberAsFloat64(record.Values[columnIndex])
			if !ok {
				continue
			}
			if valueCount == 0 || value < minValue {
				minValue = value
			}
			if valueCount == 0 || value > maxValue {
				maxValue = value
			}
			total += value
			valueCount++
		}
		stats := ColumnStats{NullCount: &nullCount}
		if valueCount > 0 {
			avg := total / float64(valueCount)
			stats.Min, stats.Max, stats.Avg = minValue, maxValue, &avg
		}
		return stats

	case "datetime", "created_at", "updated_at":
		nullCount := 0
		var minValue, maxValue string
		var minTime, maxTime time.Time
		found := false
		for _, record := range records {
			if columnIndex >= len(record.Values) || record.Values[columnIndex] == nil {
				nullCount++
				continue
			}
			value, ok := record.Values[columnIndex].(string)
			if !ok {
				continue
			}
			parsed, err := time.Parse(time.RFC3339Nano, value)
			if err != nil {
				continue
			}
			if !found || parsed.Before(minTime) {
				minValue, minTime = value, parsed
			}
			if !found || parsed.After(maxTime) {
				maxValue, maxTime = value, parsed
			}
			found = true
		}
		stats := ColumnStats{NullCount: &nullCount}
		if found {
			stats.Min, stats.Max = minValue, maxValue
		}
		return stats

	case "checkbox":
		trueCount := 0
		for _, record := range records {
			if columnIndex < len(record.Values) && record.Values[columnIndex] == true {
				trueCount++
			}
		}
		return ColumnStats{TrueCount: &trueCount}

	case "location":
		nullCount := 0
		for _, record := range records {
			if columnIndex >= len(record.Values) || record.Values[columnIndex] == nil {
				nullCount++
			}
		}
		return ColumnStats{NullCount: &nullCount}

	default:
		nullCount, maxLength := 0, 0
		for _, record := range records {
			if columnIndex >= len(record.Values) || record.Values[columnIndex] == nil {
				nullCount++
				continue
			}
			value, ok := record.Values[columnIndex].(string)
			if ok && utf8.RuneCountInString(value) > maxLength {
				maxLength = utf8.RuneCountInString(value)
			}
		}
		return ColumnStats{NullCount: &nullCount, MaxLength: &maxLength}
	}
}

func numberAsFloat64(value any) (float64, bool) {
	switch typed := value.(type) {
	case json.Number:
		converted, err := typed.Float64()
		return converted, err == nil
	case float64:
		return typed, true
	case float32:
		return float64(typed), true
	case int:
		return float64(typed), true
	case int8:
		return float64(typed), true
	case int16:
		return float64(typed), true
	case int32:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case uint:
		return float64(typed), true
	case uint8:
		return float64(typed), true
	case uint16:
		return float64(typed), true
	case uint32:
		return float64(typed), true
	case uint64:
		return float64(typed), true
	default:
		return 0, false
	}
}

func (m Manifest) Minimal() MinimalManifest {
	return MinimalManifest{
		RecordFile: m.RecordFile, RecordFileSizeBytes: m.RecordFileSizeBytes,
		ManifestFile: m.ManifestFile,
		RecordsCount: m.RecordsCount, HasMore: m.HasMore,
	}
}

// WriteManifest writes deterministic indented JSON without HTML escaping.
func WriteManifest(w io.Writer, manifest Manifest) error {
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(manifest); err != nil {
		return wrapDetailError("encode manifest", err)
	}
	return nil
}

func bestExample(records []Record, columnIndex int) (any, bool, bool) {
	var best any
	bestSize := 0
	bestTruncated := false
	found := false
	for _, record := range records {
		if columnIndex >= len(record.Values) {
			continue
		}
		value := record.Values[columnIndex]
		if isEmptyExample(value) {
			continue
		}
		encoded, err := json.Marshal(value)
		if err != nil {
			continue
		}
		if found && len(encoded) >= bestSize {
			continue
		}
		best, bestTruncated = truncateExample(value)
		bestSize = len(encoded)
		found = true
	}
	return best, bestTruncated, found
}

func isEmptyExample(value any) bool {
	switch typed := value.(type) {
	case nil:
		return true
	case string:
		return typed == ""
	case []any:
		return len(typed) == 0
	case map[string]any:
		return len(typed) == 0
	default:
		return false
	}
}

func truncateExample(value any) (any, bool) {
	switch typed := value.(type) {
	case string:
		if utf8.RuneCountInString(typed) <= maxExampleRunes {
			return typed, false
		}
		runes := []rune(typed)
		return string(runes[:maxExampleRunes]), true
	case []any:
		limit := len(typed)
		truncated := false
		if limit > maxExampleItems {
			limit = maxExampleItems
			truncated = true
		}
		items := make([]any, 0, limit)
		for _, item := range typed[:limit] {
			value, itemTruncated := truncateExample(item)
			items = append(items, value)
			truncated = truncated || itemTruncated
		}
		return items, truncated
	case map[string]any:
		object := make(map[string]any, len(typed))
		truncated := false
		for key, item := range typed {
			value, itemTruncated := truncateExample(item)
			object[key] = value
			truncated = truncated || itemTruncated
		}
		return object, truncated
	default:
		return value, false
	}
}
