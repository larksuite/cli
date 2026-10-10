// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package citation

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// SourceType identifies the product resource represented by a citation.
type SourceType int

const (
	SourceUnknown SourceType = 0
	SourceMail    SourceType = 12
)

// Citation is the JSON shape attached to successful command envelopes.
type Citation struct {
	SourceType  SourceType `json:"source_type"`
	URL         string     `json:"url"`
	Title       string     `json:"title"`
	Snippet     string     `json:"snippet,omitempty"`
	PublishTime string     `json:"publish_time,omitempty"`
}

// Filter drops citations that cannot be opened by a client.
func Filter(items []Citation) []Citation {
	out := make([]Citation, 0, len(items))
	for _, item := range items {
		if strings.TrimSpace(item.URL) == "" {
			continue
		}
		out = append(out, item)
	}
	return out
}

// Time normalizes OpenAPI timestamp values into RFC3339. Mail APIs commonly
// return Unix milliseconds as either numbers or decimal strings.
func Time(value interface{}) string {
	switch v := value.(type) {
	case nil:
		return ""
	case time.Time:
		if v.IsZero() {
			return ""
		}
		return v.UTC().Format(time.RFC3339)
	case string:
		return timeString(v)
	case jsonNumber:
		return timeString(v.String())
	case int:
		return unixTimestamp(int64(v))
	case int64:
		return unixTimestamp(v)
	case float64:
		return unixTimestamp(int64(v))
	case float32:
		return unixTimestamp(int64(v))
	default:
		return timeString(fmt.Sprint(value))
	}
}

type jsonNumber interface {
	String() string
}

func timeString(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return ""
	}
	if t, err := time.Parse(time.RFC3339, value); err == nil {
		return t.UTC().Format(time.RFC3339)
	}
	if n, err := strconv.ParseInt(value, 10, 64); err == nil {
		return unixTimestamp(n)
	}
	return ""
}

func unixTimestamp(value int64) string {
	if value <= 0 {
		return ""
	}
	if value > 1_000_000_000_000 {
		return time.UnixMilli(value).UTC().Format(time.RFC3339)
	}
	return time.Unix(value, 0).UTC().Format(time.RFC3339)
}
