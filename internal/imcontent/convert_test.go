// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package imcontent

import (
	"fmt"
	"reflect"
	"testing"
)

// TestConvertBodyContentDispatch covers the four answers the dispatch itself
// gives, independently of any converter: no context, no content, a registered
// type, and an unregistered one.
func TestConvertBodyContentDispatch(t *testing.T) {
	tests := []struct {
		name        string
		messageType string
		ctx         *ConvertContext
		want        string
	}{
		{name: "nil context", messageType: "text", ctx: nil, want: ""},
		{name: "empty content", messageType: "text", ctx: &ConvertContext{}, want: ""},
		{
			name:        "empty content, merge_forward",
			messageType: "merge_forward",
			ctx:         &ConvertContext{},
			want:        "",
		},
		{
			name:        "registered type",
			messageType: "text",
			ctx:         &ConvertContext{RawContent: `{"text":"hello"}`},
			want:        "hello",
		},
		{
			name:        "unregistered type falls back to a labelled placeholder",
			messageType: "unknown_type",
			ctx:         &ConvertContext{RawContent: `{"text":"hello"}`},
			want:        "[unknown_type]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ConvertBodyContent(tt.messageType, tt.ctx); got != tt.want {
				t.Fatalf("ConvertBodyContent(%q) = %q, want %q", tt.messageType, got, tt.want)
			}
		})
	}
}

// TestConvertBodyContentReachesEveryRegisteredConverter guards the table itself:
// a registered type must be answered by its converter, never by the
// unknown-type placeholder. A typo in a table key would otherwise be invisible —
// the message would still render, just as "[share_chat]".
func TestConvertBodyContentReachesEveryRegisteredConverter(t *testing.T) {
	for messageType, converter := range converters {
		if converter == nil {
			t.Errorf("converter for %q is nil", messageType)
			continue
		}
		placeholder := fmt.Sprintf("[%s]", messageType)
		got := ConvertBodyContent(messageType, &ConvertContext{RawContent: `{}`})
		if got == placeholder {
			t.Errorf("ConvertBodyContent(%q) = %q — the dispatch missed the registered converter", messageType, got)
		}
	}
}

func TestMergeForwardSummary(t *testing.T) {
	// The pure converter summarises; expanding the tree needs an API client and
	// lives in the shortcut layer.
	if got := ConvertBodyContent("merge_forward", &ConvertContext{
		RawContent: `{"create_message_ids":["om_1","om_2"]}`,
	}); got != "[Merged forward: 2 messages]" {
		t.Fatalf("merge_forward with ids = %q, want %q", got, "[Merged forward: 2 messages]")
	}

	// merge_forward content is often a plain-text placeholder rather than JSON.
	if got := ConvertBodyContent("merge_forward", &ConvertContext{
		RawContent: `chat history`,
	}); got != "[Merged forward]" {
		t.Fatalf("merge_forward without ids = %q, want %q", got, "[Merged forward]")
	}
}

func TestParseMergeForwardIDs(t *testing.T) {
	got := ParseMergeForwardIDs(`{"create_message_ids":["om_2","om_1"]}`)
	if want := []string{"om_2", "om_1"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ParseMergeForwardIDs() = %#v, want %#v", got, want)
	}

	// Order is the server's; non-string entries are skipped, not coerced.
	if got := ParseMergeForwardIDs(`{"create_message_ids":["om_1",42,null]}`); !reflect.DeepEqual(got, []string{"om_1"}) {
		t.Fatalf("ParseMergeForwardIDs(mixed types) = %#v, want %#v", got, []string{"om_1"})
	}
	if got := ParseMergeForwardIDs(`{invalid`); got != nil {
		t.Fatalf("ParseMergeForwardIDs(invalid JSON) = %#v, want nil", got)
	}
	if got := ParseMergeForwardIDs(`{}`); len(got) != 0 {
		t.Fatalf("ParseMergeForwardIDs(no ids) = %#v, want empty", got)
	}
}

func TestExtractMentionOpenID(t *testing.T) {
	if got := extractMentionOpenID("ou_plain"); got != "ou_plain" {
		t.Fatalf("extractMentionOpenID(string) = %q, want %q", got, "ou_plain")
	}
	if got := extractMentionOpenID(map[string]interface{}{"open_id": "ou_nested"}); got != "ou_nested" {
		t.Fatalf("extractMentionOpenID(object) = %q, want %q", got, "ou_nested")
	}
	if got := extractMentionOpenID(map[string]interface{}{"user_id": "u_1"}); got != "" {
		t.Fatalf("extractMentionOpenID(no open_id) = %q, want empty", got)
	}
	if got := extractMentionOpenID(42); got != "" {
		t.Fatalf("extractMentionOpenID(number) = %q, want empty", got)
	}
}
