// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package imcontent

import (
	"reflect"
	"testing"
	"time"
)

// These cover the pure helpers directly, where they live. They used to run
// against shortcuts/im/convert_lib forwarders, which meant the only thing
// keeping three of those forwarders in the tree was this file.

func TestParseJSONObject(t *testing.T) {
	got, err := ParseJSONObject(`{"text":"hello","count":2}`)
	if err != nil {
		t.Fatalf("ParseJSONObject() error = %v", err)
	}
	if got["text"] != "hello" {
		t.Fatalf("ParseJSONObject() text = %#v, want %#v", got["text"], "hello")
	}

	if invalid, err := ParseJSONObject(`{invalid`); err == nil || invalid != nil {
		t.Fatalf("ParseJSONObject() invalid JSON = (%#v, %v), want (nil, err)", invalid, err)
	}
}

func TestBuildMentionKeyMap(t *testing.T) {
	mentions := []interface{}{
		map[string]interface{}{"key": "@_user_1", "name": "Alice"},
		map[string]interface{}{"key": "@_user_2", "name": "Bob"},
		map[string]interface{}{"key": "", "name": "Ignored"},
		map[string]interface{}{"key": "@_user_3"},
	}

	got := BuildMentionKeyMap(mentions)
	want := map[string]string{
		"@_user_1": "Alice",
		"@_user_2": "Bob",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("BuildMentionKeyMap() = %#v, want %#v", got, want)
	}
}

func TestResolveMentionKeys(t *testing.T) {
	got := ResolveMentionKeys("hi @_user_1 and @_user_2", map[string]string{
		"@_user_1": "Alice",
		"@_user_2": "Bob",
	})
	want := "hi @Alice and @Bob"
	if got != want {
		t.Fatalf("ResolveMentionKeys() = %q, want %q", got, want)
	}
}

func TestFormatTimestamp(t *testing.T) {
	sec := int64(1710500000)
	want := time.Unix(sec, 0).Local().Format("2006-01-02 15:04:05")

	if got := FormatTimestamp("1710500000"); got != want {
		t.Fatalf("FormatTimestamp(seconds) = %q, want %q", got, want)
	}
	if got := FormatTimestamp("1710500000000"); got != want {
		t.Fatalf("FormatTimestamp(milliseconds) = %q, want %q", got, want)
	}
	if got := FormatTimestamp(""); got != "" {
		t.Fatalf("FormatTimestamp(empty) = %q, want empty", got)
	}
	if got := FormatTimestamp("not-a-number"); got != "" {
		t.Fatalf("FormatTimestamp(invalid) = %q, want empty", got)
	}
	if got := FormatTimestamp("0"); got != "" {
		t.Fatalf("FormatTimestamp(zero) = %q, want empty", got)
	}
	// 10 digits is still seconds; the millisecond divide starts at 13.
	futureSec := int64(10000000000)
	wantFuture := time.Unix(futureSec, 0).Local().Format("2006-01-02 15:04:05")
	if got := FormatTimestamp("10000000000"); got != wantFuture {
		t.Fatalf("FormatTimestamp(future seconds) = %q, want %q", got, wantFuture)
	}
}

func TestExtractPostBlocksText(t *testing.T) {
	blocks := []interface{}{
		[]interface{}{
			map[string]interface{}{"tag": "text", "text": "hello "},
			map[string]interface{}{"tag": "at", "user_name": "Alice"},
			map[string]interface{}{"tag": "text", "text": " "},
			map[string]interface{}{"tag": "a", "text": "docs", "href": "https://example.com"},
		},
		[]interface{}{
			map[string]interface{}{"tag": "img", "image_key": "img_123"},
		},
		[]interface{}{},
	}

	got := ExtractPostBlocksText(blocks)
	want := "hello @Alice [docs](https://example.com)\n![Image](img_123)"
	if got != want {
		t.Fatalf("ExtractPostBlocksText() = %q, want %q", got, want)
	}
}
