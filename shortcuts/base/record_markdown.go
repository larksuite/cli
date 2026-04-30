// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
)

const maxRecordMarkdownIgnoredFields = 20

func validateRecordReadFormat(runtime *common.RuntimeContext) error {
	switch runtime.Str("format") {
	case "", "json", "markdown":
		return nil
	default:
		return output.ErrValidation("--format must be json or markdown")
	}
}

func outputRecordMarkdown(runtime *common.RuntimeContext, data map[string]interface{}) error {
	if runtime.JqExpr != "" {
		if !runtime.Changed("format") {
			runtime.Out(data, nil)
			return nil
		}
		return output.ErrValidation("--jq and --format markdown are mutually exclusive")
	}
	rendered, err := renderRecordMarkdown(data)
	if err != nil {
		fmt.Fprintf(runtime.IO().ErrOut, "warning: record markdown render failed, falling back to json: %v\n", err)
		runtime.Out(data, nil)
		return nil
	}
	scanResult := output.ScanForSafety(runtime.Cmd.CommandPath(), data, runtime.IO().ErrOut)
	if scanResult.Blocked {
		return scanResult.BlockErr
	}
	if scanResult.Alert != nil {
		output.WriteAlertWarning(runtime.IO().ErrOut, scanResult.Alert)
	}
	fmt.Fprint(runtime.IO().Out, rendered)
	return nil
}

func renderRecordMarkdown(data map[string]interface{}) (string, error) {
	fields := stringSliceValue(data["fields"])
	recordIDs := stringSliceValue(data["record_id_list"])
	rows, ok := data["data"].([]interface{})
	if len(fields) == 0 || !ok {
		return "", output.ErrValidation("--format markdown requires record matrix response with fields, record_id_list, and data")
	}

	var b strings.Builder
	b.WriteString("`_record_id` is metadata for record operations, not a table field.\n\n")

	columns := append([]string{"_record_id"}, fields...)
	writeMarkdownRow(&b, columns)
	writeMarkdownSeparator(&b, len(columns))
	for i, rowValue := range rows {
		rowItems, _ := rowValue.([]interface{})
		cells := make([]string, 0, len(columns))
		if i < len(recordIDs) {
			cells = append(cells, recordIDs[i])
		} else {
			cells = append(cells, "")
		}
		for j := range fields {
			if j < len(rowItems) {
				cells = append(cells, markdownCell(rowItems[j]))
			} else {
				cells = append(cells, "")
			}
		}
		writeMarkdownRow(&b, cells)
	}

	meta := recordMarkdownMeta(data)
	if len(meta) > 0 {
		b.WriteString("\nMeta: ")
		b.WriteString(strings.Join(meta, "; "))
		b.WriteByte('\n')
	}
	if ignored := ignoredFieldsMarkdown(data["ignored_fields"]); ignored != "" {
		b.WriteString("Ignored fields: ")
		b.WriteString(ignored)
		b.WriteByte('\n')
	}
	return b.String(), nil
}

func recordMarkdownMeta(data map[string]interface{}) []string {
	meta := []string{fmt.Sprintf("count=%d", ignoredFieldsCount(data["record_id_list"]))}
	if hasMore, ok := data["has_more"]; ok {
		meta = append(meta, "has_more="+markdownInlineValue(hasMore))
	}
	if queryContext, ok := data["query_context"].(map[string]interface{}); ok {
		for _, key := range []string{"record_scope", "field_scope", "search_scope"} {
			if value, ok := queryContext[key]; ok {
				meta = append(meta, key+"="+markdownInlineValue(value))
			}
		}
	}
	if ignoredCount := ignoredFieldsCount(data["ignored_fields"]); ignoredCount > 0 {
		meta = append(meta, fmt.Sprintf("ignored_fields=%d", ignoredCount))
	}
	return meta
}

func ignoredFieldsCount(value interface{}) int {
	switch v := value.(type) {
	case []interface{}:
		return len(v)
	case []string:
		return len(v)
	case nil:
		return 0
	default:
		return 1
	}
}

func ignoredFieldsMarkdown(value interface{}) string {
	items := markdownListItems(value)
	if len(items) == 0 {
		return ""
	}
	total := len(items)
	if len(items) > maxRecordMarkdownIgnoredFields {
		items = items[:maxRecordMarkdownIgnoredFields]
		items = append(items, fmt.Sprintf("...(%d total)", total))
	}
	return strings.Join(items, ", ")
}

func markdownListItems(value interface{}) []string {
	switch v := value.(type) {
	case []interface{}:
		items := make([]string, 0, len(v))
		for _, item := range v {
			items = append(items, markdownInlineValue(item))
		}
		return items
	case []string:
		items := make([]string, 0, len(v))
		for _, item := range v {
			items = append(items, markdownInlineValue(item))
		}
		return items
	case nil:
		return nil
	default:
		return []string{markdownInlineValue(v)}
	}
}

func writeMarkdownRow(b *strings.Builder, cells []string) {
	b.WriteString("| ")
	for i, cell := range cells {
		if i > 0 {
			b.WriteString(" | ")
		}
		b.WriteString(markdownTableText(cell))
	}
	b.WriteString(" |\n")
}

func writeMarkdownSeparator(b *strings.Builder, columns int) {
	b.WriteString("| ")
	for i := 0; i < columns; i++ {
		if i > 0 {
			b.WriteString(" | ")
		}
		b.WriteString("---")
	}
	b.WriteString(" |\n")
}

func markdownCell(value interface{}) string {
	return markdownInlineValue(value)
}

func markdownInlineValue(value interface{}) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return v
	case json.Number:
		return v.String()
	case bool:
		if v {
			return "true"
		}
		return "false"
	case float64:
		return fmt.Sprintf("%v", v)
	case int:
		return fmt.Sprintf("%d", v)
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprintf("%v", v)
		}
		return string(b)
	}
}

func markdownTableText(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	value = strings.ReplaceAll(value, "|", "\\|")
	value = strings.ReplaceAll(value, "\r\n", "<br>")
	value = strings.ReplaceAll(value, "\n", "<br>")
	return value
}

func stringSliceValue(value interface{}) []string {
	switch v := value.(type) {
	case []interface{}:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	case []string:
		return append([]string(nil), v...)
	default:
		return nil
	}
}
