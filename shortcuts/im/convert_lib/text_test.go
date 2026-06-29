// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package convertlib

import "testing"

func TestTextConverterConvert(t *testing.T) {
	ctx := &ConvertContext{
		RawContent: `{"text":"hi @_user_1"}`,
		MentionMap: map[string]string{"@_user_1": "Alice"},
	}

	if got := (textConverter{}).Convert(ctx); got != "hi @Alice" {
		t.Fatalf("textConverter.Convert() = %q, want %q", got, "hi @Alice")
	}
}

func TestTextConverterConvertFallsBackToRawContent(t *testing.T) {
	ctx := &ConvertContext{RawContent: `{"message":"no text field"}`}

	if got := (textConverter{}).Convert(ctx); got != ctx.RawContent {
		t.Fatalf("textConverter.Convert() = %q, want raw content %q", got, ctx.RawContent)
	}
}

func TestTextConverterConvertInvalidJSON(t *testing.T) {
	ctx := &ConvertContext{RawContent: `{invalid`}

	if got := (textConverter{}).Convert(ctx); got != "[Invalid text JSON]" {
		t.Fatalf("textConverter.Convert() = %q, want %q", got, "[Invalid text JSON]")
	}
}

func TestPostConverterConvert(t *testing.T) {
	ctx := &ConvertContext{
		RawContent: `{"zh_cn":{"title":"Weekly Update","content":[[{"tag":"text","text":"Hello "},{"tag":"at","user_name":"Alice"}],[{"tag":"a","text":"Spec","href":"https://example.com/spec"}]]}}`,
		MentionMap: map[string]string{},
	}

	want := "Weekly Update\nHello @Alice\n[Spec](https://example.com/spec)"
	if got := (postConverter{}).Convert(ctx); got != want {
		t.Fatalf("postConverter.Convert() = %q, want %q", got, want)
	}
}

func TestPostConverterConvertFallback(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "invalid json", raw: `{invalid`, want: "[Invalid rich text JSON]"},
		{name: "no locale body", raw: `{"unknown":"value"}`, want: "[Rich text message]"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := (postConverter{}).Convert(&ConvertContext{RawContent: tt.raw}); got != tt.want {
				t.Fatalf("postConverter.Convert() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestUnwrapPostLocale(t *testing.T) {
	direct := map[string]interface{}{"title": "Direct"}
	if got := unwrapPostLocale(direct); got["title"] != "Direct" {
		t.Fatalf("unwrapPostLocale(direct) = %#v, want direct body", got)
	}

	localized := map[string]interface{}{
		"zh_cn": map[string]interface{}{"title": "Chinese"},
	}
	if got := unwrapPostLocale(localized); got["title"] != "Chinese" {
		t.Fatalf("unwrapPostLocale(localized) = %#v, want zh_cn body", got)
	}

	deterministicFallback := map[string]interface{}{
		"z_locale": map[string]interface{}{"title": "Zulu"},
		"a_locale": map[string]interface{}{"title": "Alpha"},
	}
	if got := unwrapPostLocale(deterministicFallback); got["title"] != "Alpha" {
		t.Fatalf("unwrapPostLocale(deterministic fallback) = %#v, want alphabetically first locale body", got)
	}
}

func TestRenderPostElem(t *testing.T) {
	tests := []struct {
		name string
		el   map[string]interface{}
		want string
	}{
		{name: "text", el: map[string]interface{}{"tag": "text", "text": "hello"}, want: "hello"},
		{name: "link", el: map[string]interface{}{"tag": "a", "text": "doc", "href": "https://example.com"}, want: "[doc](https://example.com)"},
		{name: "mention all", el: map[string]interface{}{"tag": "at", "user_id": "@_all"}, want: `<at user_id="all"></at>`},
		{name: "mention user with id", el: map[string]interface{}{"tag": "at", "user_id": "ou_user_1", "user_name": "Alice"}, want: `<at user_id="ou_user_1">Alice</at>`},
		{name: "mention user name only", el: map[string]interface{}{"tag": "at", "user_name": "Alice"}, want: "@Alice"},
		{name: "mention user id only", el: map[string]interface{}{"tag": "at", "user_id": "@_user_1"}, want: "@@_user_1"},
		{name: "image", el: map[string]interface{}{"tag": "img", "image_key": "img_123"}, want: "![Image](img_123)"},
		{name: "image no key", el: map[string]interface{}{"tag": "img"}, want: "[Image]"},
		{name: "md text", el: map[string]interface{}{"tag": "md", "text": "##### 标题\n\n<at user_id=\"ou_xxx\">Alice</at> 你好"}, want: "##### 标题\n\n<at user_id=\"ou_xxx\">Alice</at> 你好"},
		{name: "media", el: map[string]interface{}{"tag": "media", "file_key": "file_123"}, want: "[Media: file_123]"},
		{name: "code block", el: map[string]interface{}{"tag": "code_block", "language": "go", "text": "fmt.Println(1)"}, want: "\n```go\nfmt.Println(1)\n```\n"},
		{name: "hr", el: map[string]interface{}{"tag": "hr"}, want: "\n---\n"},
		{name: "unknown", el: map[string]interface{}{"tag": "unknown", "text": "fallback"}, want: "fallback"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := renderPostElem(tt.el); got != tt.want {
				t.Fatalf("renderPostElem() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestRenderPostElemEmotionStyleMd covers the 3 gaps closed in design §字段补全:
// emotion -> :emoji_type:, text.style -> Markdown emphasis (composable),
// md -> raw passthrough, while unknown tags keep the default text fallback.
func TestRenderPostElemEmotionStyleMd(t *testing.T) {
	tests := []struct {
		name string
		el   map[string]interface{}
		want string
	}{
		{name: "emotion", el: map[string]interface{}{"tag": "emotion", "emoji_type": "SMILE"}, want: ":SMILE:"},
		{name: "emotion empty", el: map[string]interface{}{"tag": "emotion"}, want: ""},
		{name: "md passthrough", el: map[string]interface{}{"tag": "md", "text": "# Heading\n- item"}, want: "# Heading\n- item"},
		{name: "style bold", el: map[string]interface{}{"tag": "text", "text": "hi", "style": []interface{}{"bold"}}, want: "**hi**"},
		{name: "style italic", el: map[string]interface{}{"tag": "text", "text": "hi", "style": []interface{}{"italic"}}, want: "*hi*"},
		{name: "style underline", el: map[string]interface{}{"tag": "text", "text": "hi", "style": []interface{}{"underline"}}, want: "<u>hi</u>"},
		{name: "style lineThrough", el: map[string]interface{}{"tag": "text", "text": "hi", "style": []interface{}{"lineThrough"}}, want: "~~hi~~"},
		{name: "style composable bold+lineThrough", el: map[string]interface{}{"tag": "text", "text": "hi", "style": []interface{}{"bold", "lineThrough"}}, want: "~~**hi**~~"},
		// bold+italic collapses to ***hi*** (CommonMark-valid), not *(**hi**)*.
		{name: "style composable bold+italic", el: map[string]interface{}{"tag": "text", "text": "hi", "style": []interface{}{"bold", "italic"}}, want: "***hi***"},
		{name: "style empty no wrap", el: map[string]interface{}{"tag": "text", "text": "plain", "style": []interface{}{}}, want: "plain"},
		{name: "link with style", el: map[string]interface{}{"tag": "a", "text": "doc", "href": "https://example.com", "style": []interface{}{"bold"}}, want: "**[doc](https://example.com)**"},
		{name: "mention with style", el: map[string]interface{}{"tag": "at", "user_name": "Alice", "style": []interface{}{"italic"}}, want: "*@Alice*"},
		{name: "unknown tag default", el: map[string]interface{}{"tag": "weird", "text": "fallback"}, want: "fallback"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := renderPostElem(tt.el); got != tt.want {
				t.Fatalf("renderPostElem(%s) = %q, want %q", tt.name, got, tt.want)
			}
		})
	}
}

func TestSelectContentBlocks(t *testing.T) {
	tests := []struct {
		name string
		body map[string]interface{}
		want int
	}{
		{
			name: "content_v2 present and non-empty",
			body: map[string]interface{}{
				"content":    []interface{}{[]interface{}{map[string]interface{}{"tag": "text", "text": "old"}}},
				"content_v2": []interface{}{[]interface{}{map[string]interface{}{"tag": "md", "text": "new"}}},
			},
			want: 1,
		},
		{
			name: "content_v2 empty array",
			body: map[string]interface{}{
				"content":    []interface{}{[]interface{}{map[string]interface{}{"tag": "text", "text": "old"}}},
				"content_v2": []interface{}{},
			},
			want: 1,
		},
		{
			name: "content_v2 nil",
			body: map[string]interface{}{
				"content": []interface{}{[]interface{}{map[string]interface{}{"tag": "text", "text": "old"}}},
			},
			want: 1,
		},
		{
			name: "content_v2 wrong type",
			body: map[string]interface{}{
				"content":    []interface{}{[]interface{}{map[string]interface{}{"tag": "text", "text": "old"}}},
				"content_v2": "not_an_array",
			},
			want: 1,
		},
		{
			name: "both missing",
			body: map[string]interface{}{},
			want: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := selectContentBlocks(tt.body)
			if len(got) != tt.want {
				t.Fatalf("selectContentBlocks() len = %d, want %d", len(got), tt.want)
			}
		})
	}
}

func TestPostConverterConvertContentV2(t *testing.T) {
	// AC-M1-H1: content_v2 present → use content_v2 blocks (md passthrough)
	ctx := &ConvertContext{
		RawContent: `{"content_v2":[[{"tag":"md","text":"##### 标题\n\n<at user_id=\"ou_xxx\">Alice</at> 你好"}]],"content":[[{"tag":"text","text":"old path"}]]}`,
	}
	want := "##### 标题\n\n<at user_id=\"ou_xxx\">Alice</at> 你好"
	if got := (postConverter{}).Convert(ctx); got != want {
		t.Fatalf("postConverter.Convert(content_v2) = %q, want %q", got, want)
	}

	// AC-M1-H2: no content_v2 → use content blocks with new at/img format
	ctx2 := &ConvertContext{
		RawContent: `{"content":[[{"tag":"at","user_id":"ou_xxx","user_name":"Bob"},{"tag":"text","text":" "},{"tag":"img","image_key":"img_123"}]]}`,
		Mentions:   []interface{}{map[string]interface{}{"key": "ou_xxx", "id": "ou_bob", "name": "Bob"}},
	}
	want2 := `<at user_id="ou_xxx">Bob</at> ![Image](img_123)`
	if got := (postConverter{}).Convert(ctx2); got != want2 {
		t.Fatalf("postConverter.Convert(content) = %q, want %q", got, want2)
	}

	// AC-M1-E1: content_v2 empty → fallback to content
	ctx3 := &ConvertContext{
		RawContent: `{"content_v2":[],"content":[[{"tag":"text","text":"fallback path"}]]}`,
	}
	want3 := "fallback path"
	if got := (postConverter{}).Convert(ctx3); got != want3 {
		t.Fatalf("postConverter.Convert(empty content_v2) = %q, want %q", got, want3)
	}
}
