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

// Citation is one structured citation entry as builders produce it. It is not
// serialized directly: the envelope carries each entry as an XML <document>
// string (see EncodeXML). SourceType, URL and Title are required on the wire;
// entries without a URL are dropped by Normalize before serialization.
type Citation struct {
	SourceType  SourceType
	URL         string
	Title       string
	Snippet     string
	PublishTime string
}

// Enabled reports whether citation output is on.
//
// DEBUG DEFAULT-ON (joint-debugging branch only): citations are enabled
// unless the gate is explicitly set to the exact value "0", so PPE runs are
// not at the mercy of which env vars the agent platform injects. The
// mainline contract remains opt-in ("1" enables, everything else is off);
// restore that before merging to main.
func Enabled() bool {
	return os.Getenv(envvars.CliCitation) != "0"
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
