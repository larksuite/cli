// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package imcontent

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ParseJSONObject parses a raw JSON string into a map.
func ParseJSONObject(raw string) (map[string]interface{}, error) {
	var value map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return nil, err
	}
	return value, nil
}

func invalidJSONPlaceholder(kind string) string {
	if kind == "" {
		return "[Invalid JSON content]"
	}
	return fmt.Sprintf("[Invalid %s JSON]", kind)
}

// BuildMentionKeyMap builds a key-to-name lookup from a message mentions array.
func BuildMentionKeyMap(mentions []interface{}) map[string]string {
	result := map[string]string{}
	for _, raw := range mentions {
		item, _ := raw.(map[string]interface{})
		key, _ := item["key"].(string)
		name, _ := item["name"].(string)
		if key != "" && name != "" {
			result[key] = name
		}
	}
	return result
}

// ResolveMentionKeys replaces mention keys in text with @name format.
func ResolveMentionKeys(text string, mentionMap map[string]string) string {
	for key, name := range mentionMap {
		text = strings.ReplaceAll(text, key, "@"+name)
	}
	return text
}

// FormatTimestamp converts a Unix timestamp string in seconds or milliseconds
// to local time. It returns an empty string for empty or invalid values.
func FormatTimestamp(timestamp string) string {
	if timestamp == "" {
		return ""
	}
	value, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil || value == 0 {
		return ""
	}
	if len(strings.TrimLeft(timestamp, "+-")) >= 13 {
		value /= 1000
	}
	return time.Unix(value, 0).Local().Format("2006-01-02 15:04:05")
}

var xmlBodyEscaper = strings.NewReplacer(
	"&", "&amp;",
	"<", "&lt;",
	">", "&gt;",
)

func xmlEscapeBody(value string) string {
	return xmlBodyEscaper.Replace(value)
}

func escapeMDLinkText(value string) string {
	value = strings.ReplaceAll(value, `[`, `\[`)
	value = strings.ReplaceAll(value, `]`, `\]`)
	return value
}

// ExtractPostBlocksText extracts plain text from post-style content blocks.
func ExtractPostBlocksText(blocks []interface{}) string {
	var lines []string
	for _, paragraph := range blocks {
		elements, _ := paragraph.([]interface{})
		var builder strings.Builder
		for _, raw := range elements {
			element, _ := raw.(map[string]interface{})
			builder.WriteString(renderPostElem(element))
		}
		if line := builder.String(); line != "" {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n")
}
