// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package citation defines the wire type, process-level gate, and time
// normalization for command citations. It depends on nothing above the
// envvars constant table so every layer may import it.
package citation

import (
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/larksuite/cli/internal/envvars"
)

// Citation is one wire entry of the envelope-level citations array.
// SourceType, URL and Title carry no omitempty: they are required on the wire
// (entries without a URL are dropped by Normalize before serialization, and
// an int omitempty would silently drop a required zero).
type Citation struct {
	SourceType  SourceType `json:"source_type"`
	URL         string     `json:"url"`
	Title       string     `json:"title"`
	Snippet     string     `json:"snippet,omitempty"`
	ResourceID  string     `json:"resource_id,omitempty"`
	PublishTime string     `json:"publish_time,omitempty"`
}

// Enabled reports whether citation output is on. Only the exact value "1"
// enables it; no second boolean grammar is introduced.
func Enabled() bool {
	return os.Getenv(envvars.CliCitation) == "1"
}

// Normalize drops entries without a URL and returns nil for an empty result
// so the envelope omits the citations key entirely.
func Normalize(items []Citation) []Citation {
	var kept []Citation
	for _, item := range items {
		if item.URL == "" {
			continue
		}
		kept = append(kept, item)
	}
	return kept
}

// Time accepts unix seconds, unix milliseconds (int, int64, float64, or a
// digit string), or a preformatted timestamp and returns RFC3339 with an
// explicit offset. It returns "" when the input cannot be parsed. Digit
// inputs are classified by decimal length: up to 10 digits are seconds,
// exactly 13 digits are milliseconds, anything else is unparseable.
func Time(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case int:
		return timeFromUnixDigits(strconv.FormatInt(int64(t), 10))
	case int64:
		return timeFromUnixDigits(strconv.FormatInt(t, 10))
	case float64:
		return timeFromUnixDigits(strconv.FormatInt(int64(t), 10))
	case string:
		s := strings.TrimSpace(t)
		if s == "" {
			return ""
		}
		if isDigits(s) {
			return timeFromUnixDigits(s)
		}
		if parsed, err := time.Parse(time.RFC3339, s); err == nil {
			return parsed.Format(time.RFC3339)
		}
		for _, layout := range []string{"2006-01-02 15:04:05", "2006-01-02 15:04"} {
			if parsed, err := time.ParseInLocation(layout, s, time.Local); err == nil {
				return parsed.Format(time.RFC3339)
			}
		}
		return ""
	default:
		return ""
	}
}

func isDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return len(s) > 0
}

func timeFromUnixDigits(s string) string {
	if !isDigits(s) { // 负数等非纯数字形态一律不可解析
		return ""
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n <= 0 {
		// zero and negative epoch values are not valid publish times
		return ""
	}
	switch {
	case len(s) <= 10:
		return time.Unix(n, 0).Format(time.RFC3339)
	case len(s) == 13:
		return time.UnixMilli(n).Format(time.RFC3339)
	default:
		return ""
	}
}
