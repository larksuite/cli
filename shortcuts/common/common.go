// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/util"
)

func FormatSize(bytes int64) string {
	if bytes < 1024 {
		return fmt.Sprintf("%d B", bytes)
	}
	if bytes < 1024*1024 {
		return fmt.Sprintf("%.1f KB", float64(bytes)/1024)
	}
	if bytes < 1024*1024*1024 {
		return fmt.Sprintf("%.1f MB", float64(bytes)/1024/1024)
	}
	return fmt.Sprintf("%.1f GB", float64(bytes)/1024/1024/1024)
}

func MaskToken(token string) string {
	if len(token) < 2 {
		return "***"
	}
	if len(token) <= 8 {
		return token[:2] + "***"
	}
	return token[:4] + "..." + token[len(token)-4:]
}

// ParseTime converts time expressions to Unix seconds string.
//
// Optional hint: "end" makes day-granularity inputs snap to 23:59:59 instead of 00:00:00.
//
//	ParseTime("2026-01-01")        → 2026-01-01 00:00:00
//	ParseTime("2026-01-01", "end") → 2026-01-01 23:59:59
//
// Supported formats: ISO 8601 (with or without time/timezone), date-only, Unix timestamp.
func ParseTime(input string, hint ...string) (string, error) {
	input = strings.TrimSpace(input)
	isEnd := len(hint) > 0 && hint[0] == "end"

	// snapDay aligns to start-of-day or end-of-day based on hint.
	snapDay := func(t time.Time) time.Time {
		if isEnd {
			return time.Date(t.Year(), t.Month(), t.Day(), 23, 59, 59, 0, t.Location())
		}
		return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
	}

	// ISO 8601 with timezone (precise)
	tzFormats := []string{
		time.RFC3339,
		"2006-01-02T15:04Z07:00",
		"2006-01-02T15:04:05Z07:00",
	}
	for _, f := range tzFormats {
		if t, err := time.Parse(f, input); err == nil {
			return fmt.Sprintf("%d", t.Unix()), nil
		}
	}
	// ISO 8601 without timezone — with time component (precise)
	preciseFormats := []string{
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
		"2006-01-02T15:04",
		"2006-01-02 15:04",
	}
	for _, f := range preciseFormats {
		if t, err := time.ParseInLocation(f, input, time.Local); err == nil {
			return fmt.Sprintf("%d", t.Unix()), nil
		}
	}
	// Date-only (day-granularity)
	if t, err := time.ParseInLocation("2006-01-02", input, time.Local); err == nil {
		return fmt.Sprintf("%d", snapDay(t).Unix()), nil
	}
	// Unix timestamp (precise, passed through as-is) — must be purely numeric
	var ts int64
	if n, err := fmt.Sscanf(input, "%d", &ts); err == nil && n == 1 && ts > 0 && fmt.Sprintf("%d", ts) == input {
		return input, nil
	}
	return "", fmt.Errorf("cannot parse time %q (supported: ISO 8601 e.g. 2026-01-01 / 2026-01-01T15:04:05+08:00, Unix timestamp)", input)
}

// FormatTimeWithSeconds converts Unix seconds/ms string to local time string with seconds precision.
func FormatTimeWithSeconds(ts interface{}) string {
	if ts == nil {
		return ""
	}
	s := fmt.Sprintf("%v", ts)
	if s == "" {
		return ""
	}
	var n int64
	fmt.Sscanf(s, "%d", &n)
	if n == 0 {
		return s
	}
	if n > 1e12 {
		n = n / 1000
	}
	t := time.Unix(n, 0)
	return t.Local().Format("2006-01-02 15:04:05")
}

// FormatTime converts Unix seconds/ms string to local time string.
func FormatTime(ts interface{}) string {
	if ts == nil {
		return ""
	}
	s := fmt.Sprintf("%v", ts)
	if s == "" {
		return ""
	}
	var n int64
	fmt.Sscanf(s, "%d", &n)
	if n == 0 {
		return s
	}
	// Detect ms vs seconds
	if n > 1e12 {
		n = n / 1000
	}
	t := time.Unix(n, 0)
	return t.Local().Format("2006-01-02 15:04")
}

// SplitCSV 解析逗号分隔的列表，忽略空项并去除空格
func SplitCSV(input string) []string {
	if input == "" {
		return nil
	}
	parts := strings.Split(input, ",")
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

// CheckApiError checks if API result is an error and prints it to w.
func CheckApiError(w io.Writer, result interface{}, action string) bool {
	if resultMap, ok := result.(map[string]interface{}); ok {
		code, _ := util.ToFloat64(resultMap["code"])
		if code != 0 {
			msg, _ := resultMap["msg"].(string)
			output.PrintError(w, fmt.Sprintf("%s: [%.0f] %s", action, code, msg))
			return true
		}
	}
	return false
}

// HandleApiResult checks for network/API errors and returns the "data" field.
func HandleApiResult(result interface{}, err error, action string) (map[string]interface{}, error) {
	if err != nil {
		return nil, output.Errorf(output.ExitAPI, "api_error", "%s: %s", action, err)
	}
	resultMap, _ := result.(map[string]interface{})
	code, _ := util.ToFloat64(resultMap["code"])
	if code != 0 {
		msg, _ := resultMap["msg"].(string)
		larkCode := int(code)
		fullMsg := fmt.Sprintf("%s: [%d] %s", action, larkCode, msg)
		return nil, output.ErrAPI(larkCode, fullMsg, resultMap["error"])
	}
	data, _ := resultMap["data"].(map[string]interface{})
	return data, nil
}

// TruncateStr truncates s to at most n runes.
func TruncateStr(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}
