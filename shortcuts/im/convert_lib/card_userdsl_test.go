// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package convertlib

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestConvertInteractiveEventContent(t *testing.T) {
	// invalid JSON → fallback
	if got := ConvertInteractiveEventContent("not-json", nil); got != "[interactive card]" {
		t.Fatalf("invalid JSON = %q, want [interactive card]", got)
	}
	// missing user_dsl → fallback
	if got := ConvertInteractiveEventContent(`{"other":"field"}`, nil); got != "[interactive card]" {
		t.Fatalf("missing user_dsl = %q, want [interactive card]", got)
	}
	// empty user_dsl → fallback
	if got := ConvertInteractiveEventContent(`{"user_dsl":""}`, nil); got != "[interactive card]" {
		t.Fatalf("empty user_dsl = %q, want [interactive card]", got)
	}
	// user_dsl that is not a string (wrong type) → fallback
	if got := ConvertInteractiveEventContent(`{"user_dsl":123}`, nil); got != "[interactive card]" {
		t.Fatalf("non-string user_dsl = %q, want [interactive card]", got)
	}
	// valid user-2 card → <card> output
	userDsl := `{"schema":"2.0","header":{"title":{"tag":"plain_text","content":"Hello"}},"body":{"elements":[{"tag":"markdown","content":"world"}]}}`
	rawContent := `{"user_dsl":"` + strings.ReplaceAll(userDsl, `"`, `\"`) + `"}`
	got := ConvertInteractiveEventContent(rawContent, nil)
	if !strings.HasPrefix(got, `<card title="Hello">`) {
		t.Fatalf("valid card = %q, want prefix <card title=\"Hello\">", got)
	}
	if !strings.Contains(got, "world") {
		t.Fatalf("valid card = %q, want to contain 'world'", got)
	}
}

func makeMentionCard(mdContent string) string {
	obj := map[string]interface{}{
		"schema": "2.0",
		"header": map[string]interface{}{
			"title": map[string]interface{}{"tag": "plain_text", "content": "T"},
		},
		"body": map[string]interface{}{
			"elements": []interface{}{
				map[string]interface{}{"tag": "markdown", "content": mdContent},
			},
		},
	}
	dslBytes, _ := json.Marshal(obj)
	raw, _ := json.Marshal(map[string]interface{}{"user_dsl": string(dslBytes)})
	return string(raw)
}

func TestConvertInteractiveEventContentMentions(t *testing.T) {
	mentions := []interface{}{
		map[string]interface{}{
			"key":  "@_user_1",
			"name": "test-user",
			"id":   map[string]interface{}{"open_id": "fake-uid-001"},
		},
	}

	// quoted attrs: mention_key="key"
	got := ConvertInteractiveEventContent(makeMentionCard(`hi <at mention_key="@_user_1">n</at> done`), mentions)
	if !strings.Contains(got, "@test-user(fake-uid-001)") {
		t.Fatalf("quoted mention_key not resolved, got: %s", got)
	}

	// unquoted attrs (real Lark format): <at id=ou_xxx mention_key=@_user_1></at>
	got = ConvertInteractiveEventContent(makeMentionCard(`hi <at id=fake-uid-001 mention_key=@_user_1></at> done`), mentions)
	if !strings.Contains(got, "@test-user(fake-uid-001)") {
		t.Fatalf("unquoted mention_key not resolved, got: %s", got)
	}

	// mentions_key variant (unquoted)
	got = ConvertInteractiveEventContent(makeMentionCard(`hi <at mentions_key=@_user_1></at> done`), mentions)
	if !strings.Contains(got, "@test-user(fake-uid-001)") {
		t.Fatalf("unquoted mentions_key not resolved, got: %s", got)
	}

	// degradation 1: no mention_key/mentions_key attr → fall back to @id (unquoted)
	got = ConvertInteractiveEventContent(makeMentionCard(`hi <at id=fake-uid-001></at> done`), mentions)
	if !strings.Contains(got, "@fake-uid-001") {
		t.Fatalf("no mention_key unquoted: expected @id fallback, got: %s", got)
	}

	// degradation 2: mention_key not found in mentions → fall back to @id
	got = ConvertInteractiveEventContent(makeMentionCard(`hi <at id=fake-uid-001 mention_key=@_unknown></at> done`), mentions)
	if !strings.Contains(got, "@fake-uid-001") {
		t.Fatalf("key not in mentions: expected @id fallback, got: %s", got)
	}

	// multi-mention: ids=id1,id2,id3 mentions_key=k1,,k3
	// k1 hits → @name(id1), k2 empty → @id2 fallback, k3 not found → @id3 fallback
	got = ConvertInteractiveEventContent(
		makeMentionCard(`<at ids=fake-uid-001,fake-uid-002,fake-uid-003 mentions_key=@_user_1,,@_unknown></at>`),
		mentions,
	)
	want := "@test-user(fake-uid-001)@fake-uid-002@fake-uid-003"
	if !strings.Contains(got, want) {
		t.Fatalf("multi-mention unquoted: want %q in output, got: %s", want, got)
	}
}

func TestUserDslConverterSchema(t *testing.T) {
	c := &userDslConverter{}

	// user-2.ts: schema field present, header at root, body.elements
	schema2 := cardObj{
		"schema": "2.0",
		"header": cardObj{
			"title":    cardObj{"tag": "plain_text", "content": "Schema2 Title"},
			"subtitle": cardObj{"tag": "plain_text", "content": "Sub"},
		},
		"body": cardObj{
			"elements": []interface{}{
				cardObj{"tag": "markdown", "content": "body text"},
			},
		},
	}
	got := c.convert(schema2)
	if got != "<card title=\"Schema2 Title\" subtitle=\"Sub\">\nbody text\n</card>" {
		t.Fatalf("schema2 = %q", got)
	}

	// user-1.ts: no schema field, i18n_header.zh_cn, elements at root
	schema1 := cardObj{
		"i18n_header": cardObj{
			"zh_cn": cardObj{
				"title": cardObj{"tag": "plain_text", "content": "Schema1 Title"},
			},
		},
		"elements": []interface{}{
			cardObj{"tag": "hr"},
		},
	}
	got = c.convert(schema1)
	if got != "<card title=\"Schema1 Title\">\n---\n</card>" {
		t.Fatalf("schema1 = %q", got)
	}

	// user-1.ts: no schema, direct header (real Lark event format)
	schema1Direct := cardObj{
		"header": cardObj{
			"title":    cardObj{"tag": "plain_text", "content": "Direct Header Title"},
			"subtitle": cardObj{"tag": "plain_text", "content": "Direct Sub"},
		},
		"elements": []interface{}{
			cardObj{"tag": "markdown", "content": "direct body"},
		},
	}
	got = c.convert(schema1Direct)
	if got != "<card title=\"Direct Header Title\" subtitle=\"Direct Sub\">\ndirect body\n</card>" {
		t.Fatalf("schema1 direct header = %q", got)
	}

	// no header, no elements → fallback
	got = c.convert(cardObj{})
	if got != "[interactive card]" {
		t.Fatalf("empty card = %q, want [interactive card]", got)
	}

	// card with title only → valid (not "[interactive card]")
	titleOnly := cardObj{
		"schema": "2.0",
		"header": cardObj{"title": cardObj{"tag": "plain_text", "content": "TitleOnly"}},
		"body":   cardObj{"elements": []interface{}{}},
	}
	got = c.convert(titleOnly)
	if !strings.Contains(got, "TitleOnly") {
		t.Fatalf("title-only card = %q, want to contain TitleOnly", got)
	}
}

func TestUserDslConverterDispatch(t *testing.T) {
	c := &userDslConverter{}

	tests := []struct {
		name     string
		elem     cardObj
		want     string
		contains string
	}{
		{
			name: "plain_text",
			elem: cardObj{"tag": "plain_text", "content": "hello"},
			want: "hello",
		},
		{
			name: "markdown",
			elem: cardObj{"tag": "markdown", "content": "**bold**"},
			want: "**bold**",
		},
		{
			name: "hr",
			elem: cardObj{"tag": "hr"},
			want: "---",
		},
		{
			name: "br",
			elem: cardObj{"tag": "br"},
			want: "\n",
		},
		{
			name: "img with img_key",
			elem: cardObj{
				"tag":     "img",
				"img_key": "img_v3_abc",
				"alt":     cardObj{"tag": "plain_text", "content": "Banner"},
			},
			want: "🖼️ Banner(img_key:img_v3_abc)",
		},
		{
			name: "img_combination",
			elem: cardObj{
				"tag": "img_combination",
				"img_list": []interface{}{
					cardObj{"img_key": "k1"},
					cardObj{"img_key": "k2"},
				},
			},
			want: "🖼️ 2 image(s)(keys:k1,k2)",
		},
		{
			name: "button with behaviors default_url",
			elem: cardObj{
				"tag":  "button",
				"text": cardObj{"tag": "plain_text", "content": "Open"},
				"behaviors": []interface{}{
					cardObj{"type": "open_url", "default_url": "https://example.com"},
				},
			},
			want: "[Open](https://example.com)",
		},
		{
			name: "button disabled",
			elem: cardObj{
				"tag":      "button",
				"text":     cardObj{"tag": "plain_text", "content": "Nope"},
				"disabled": true,
			},
			want: "[Nope ✗]",
		},
		{
			name: "button no url",
			elem: cardObj{
				"tag":  "button",
				"text": cardObj{"tag": "plain_text", "content": "Submit"},
			},
			want: "[Submit]",
		},
		{
			name: "action wrapper (user-1 style)",
			elem: cardObj{
				"tag": "action",
				"actions": []interface{}{
					cardObj{"tag": "button", "text": cardObj{"tag": "plain_text", "content": "A"}},
					cardObj{"tag": "button", "text": cardObj{"tag": "plain_text", "content": "B"}},
				},
			},
			want: "[A] [B]",
		},
		{
			name: "overflow",
			elem: cardObj{
				"tag": "overflow",
				"options": []interface{}{
					cardObj{"text": cardObj{"tag": "plain_text", "content": "Edit"}},
					cardObj{"text": cardObj{"tag": "plain_text", "content": "Delete"}},
				},
			},
			want: "⋮ Edit, Delete",
		},
		{
			name: "select_static no selection",
			elem: cardObj{
				"tag": "select_static",
				"options": []interface{}{
					cardObj{"text": cardObj{"tag": "plain_text", "content": "Option1"}, "value": "1"},
					cardObj{"text": cardObj{"tag": "plain_text", "content": "Option2"}, "value": "2"},
				},
			},
			want: "{Option1 / Option2 ▼}",
		},
		{
			name: "select_static with initial_option",
			elem: cardObj{
				"tag":            "select_static",
				"initial_option": "2",
				"options": []interface{}{
					cardObj{"text": cardObj{"tag": "plain_text", "content": "Option1"}, "value": "1"},
					cardObj{"text": cardObj{"tag": "plain_text", "content": "Option2"}, "value": "2"},
				},
			},
			want: "{Option1 / ✓Option2}",
		},
		{
			name: "multi_select_static with selected_values",
			elem: cardObj{
				"tag":             "multi_select_static",
				"selected_values": []interface{}{"A"},
				"options": []interface{}{
					cardObj{"text": cardObj{"tag": "plain_text", "content": "OptA"}, "value": "A"},
					cardObj{"text": cardObj{"tag": "plain_text", "content": "OptB"}, "value": "B"},
				},
			},
			want: "{✓OptA / OptB}(multi)",
		},
		{
			name: "select_person no options no selection shows placeholder",
			elem: cardObj{
				"tag":         "select_person",
				"placeholder": cardObj{"tag": "plain_text", "content": "请选择"},
			},
			want: "{请选择 ▼}",
		},
		{
			name: "select_person with initial_option synthesizes from ID",
			elem: cardObj{
				"tag":            "select_person",
				"initial_option": "fake-open-id-001",
			},
			want: "{✓fake-open-id-001}",
		},
		{
			name: "multi_select_person with selected_values shows IDs and multi",
			elem: cardObj{
				"tag":             "multi_select_person",
				"selected_values": []interface{}{"fake-open-id-001", "fake-open-id-002"},
			},
			want: "{✓fake-open-id-001 / ✓fake-open-id-002}(multi)",
		},
		{
			name: "multi_select_person no selection shows placeholder",
			elem: cardObj{
				"tag":         "multi_select_person",
				"placeholder": cardObj{"tag": "plain_text", "content": "添加人员"},
			},
			want: "{添加人员 ▼}(multi)",
		},
		{
			name: "input with default_value",
			elem: cardObj{
				"tag":           "input",
				"label":         cardObj{"tag": "plain_text", "content": "Reason"},
				"default_value": "prefilled",
			},
			want: "Reason: prefilled___",
		},
		{
			name: "input with placeholder",
			elem: cardObj{
				"tag":         "input",
				"placeholder": cardObj{"tag": "plain_text", "content": "Type here"},
			},
			want: "Type here_____",
		},
		{
			name: "date_picker with initial_date",
			elem: cardObj{
				"tag":          "date_picker",
				"initial_date": "2026-01-01",
			},
			want: "📅 2026-01-01",
		},
		{
			name: "date_picker placeholder",
			elem: cardObj{
				"tag":         "date_picker",
				"placeholder": cardObj{"tag": "plain_text", "content": "Pick date"},
			},
			want: "📅 Pick date",
		},
		{
			name: "picker_time with initial_time",
			elem: cardObj{
				"tag":          "picker_time",
				"initial_time": "14:30",
			},
			want: "🕐 14:30",
		},
		{
			name: "checker unchecked",
			elem: cardObj{
				"tag":  "checker",
				"text": cardObj{"tag": "plain_text", "content": "Task A"},
			},
			want: "[ ] Task A",
		},
		{
			name: "checker checked",
			elem: cardObj{
				"tag":     "checker",
				"checked": true,
				"text":    cardObj{"tag": "plain_text", "content": "Task B"},
			},
			want: "[x] Task B",
		},
		{
			name: "chart with chart_spec",
			elem: cardObj{
				"tag": "chart",
				"chart_spec": cardObj{
					"title":  cardObj{"text": "Sales"},
					"type":   "bar",
					"xField": "month",
					"yField": "value",
					"data": cardObj{"values": []interface{}{
						cardObj{"month": "Jan", "value": float64(10)},
						cardObj{"month": "Feb", "value": float64(20)},
					}},
				},
			},
			want: "📊 Sales (Bar chart)\nSummary: Jan:10, Feb:20",
		},
		{
			name: "chart with compound xField array",
			elem: cardObj{
				"tag": "chart",
				"chart_spec": cardObj{
					"title":  cardObj{"text": "Sales"},
					"type":   "bar",
					"xField": []interface{}{"month", "category"},
					"yField": "value",
					"data": cardObj{"values": []interface{}{
						cardObj{"month": "Jan", "category": "A", "value": float64(10)},
						cardObj{"month": "Feb", "category": "B", "value": float64(20)},
					}},
				},
			},
			want: "📊 Sales (Bar chart)\nSummary: Jan:10, Feb:20",
		},
		{
			name: "chart no custom title uses type name",
			elem: cardObj{
				"tag": "chart",
				"chart_spec": cardObj{
					"type":          "pie",
					"categoryField": "label",
					"valueField":    "val",
					"data": cardObj{"values": []interface{}{
						cardObj{"label": "A", "val": float64(1)},
					}},
				},
			},
			want: "📊 Pie chart\nSummary: A:1",
		},
		{
			name: "chart vchart array data format",
			elem: cardObj{
				"tag": "chart",
				"chart_spec": cardObj{
					"type":   "bar",
					"xField": "x",
					"yField": "y",
					"data": []interface{}{
						cardObj{"id": "s1", "values": []interface{}{
							cardObj{"x": "Jan", "y": float64(5)},
						}},
						cardObj{"id": "s2", "values": []interface{}{
							cardObj{"x": "Feb", "y": float64(8)},
						}},
					},
				},
			},
			want: "📊 Bar chart\nSummary: Jan:5, Feb:8",
		},
		{
			name: "text_tag",
			elem: cardObj{
				"tag":  "text_tag",
				"text": cardObj{"tag": "plain_text", "content": "新功能"},
			},
			want: "「新功能」",
		},
		{
			name: "avatar with user_id",
			elem: cardObj{"tag": "avatar", "user_id": "fake-open-id-001"},
			want: "👤(id:fake-open-id-001)",
		},
		{
			name: "avatar no user_id",
			elem: cardObj{"tag": "avatar"},
			want: "👤",
		},
		{
			name: "select_img no selection",
			elem: cardObj{
				"tag": "select_img",
				"options": []interface{}{
					cardObj{"value": "v1", "img_key": "img_k1"},
					cardObj{"value": "v2", "img_key": "img_k2"},
				},
			},
			want: "{🖼️ Image 1(v1)(img_key:img_k1) / 🖼️ Image 2(v2)(img_key:img_k2)}",
		},
		{
			name: "select_img with selected",
			elem: cardObj{
				"tag":             "select_img",
				"selected_values": []interface{}{"v1"},
				"options": []interface{}{
					cardObj{"value": "v1", "img_key": "img_k1"},
					cardObj{"value": "v2", "img_key": "img_k2"},
				},
			},
			want: "{✓🖼️ Image 1(v1)(img_key:img_k1) / 🖼️ Image 2(v2)(img_key:img_k2)}",
		},
		{
			name: "repeat delegates to elements",
			elem: cardObj{
				"tag": "repeat",
				"elements": []interface{}{
					cardObj{"tag": "markdown", "content": "item A"},
					cardObj{"tag": "markdown", "content": "item B"},
				},
			},
			want: "item A\nitem B",
		},
		{
			name: "audio with file_key",
			elem: cardObj{"tag": "audio", "file_key": "file_abc123"},
			want: "🎵 Audio(key:file_abc123)",
		},
		{
			name: "audio fallback audio_id",
			elem: cardObj{"tag": "audio", "audio_id": "audio_xyz"},
			want: "🎵 Audio(key:audio_xyz)",
		},
		{
			name: "video with file_key",
			elem: cardObj{"tag": "video", "file_key": "video_abc"},
			want: "🎬 Video(key:video_abc)",
		},
		{
			name: "custom_icon returns empty",
			elem: cardObj{"tag": "custom_icon", "img_key": "some_key"},
			want: "",
		},
		{
			name: "standard_icon returns empty",
			elem: cardObj{"tag": "standard_icon", "token": "alarm_outlined"},
			want: "",
		},
		{
			name: "button disabled with disabled_tips",
			elem: cardObj{
				"tag":           "button",
				"text":          cardObj{"tag": "plain_text", "content": "Submit"},
				"disabled":      true,
				"disabled_tips": cardObj{"tag": "plain_text", "content": "Not allowed"},
			},
			want: "[Submit ✗](tips:\"Not allowed\")",
		},
		{
			name: "button with confirm",
			elem: cardObj{
				"tag":  "button",
				"text": cardObj{"tag": "plain_text", "content": "Delete"},
				"confirm": cardObj{
					"title": cardObj{"tag": "plain_text", "content": "确认"},
					"text":  cardObj{"tag": "plain_text", "content": "不可撤销"},
				},
			},
			want: "[Delete](confirm:\"确认: 不可撤销\")",
		},
		{
			name: "overflow with url",
			elem: cardObj{
				"tag": "overflow",
				"options": []interface{}{
					cardObj{"text": cardObj{"tag": "plain_text", "content": "Open"}, "url": "https://example.com"},
					cardObj{"text": cardObj{"tag": "plain_text", "content": "Copy"}, "value": "copy"},
				},
			},
			want: "⋮ [Open](https://example.com), Copy(copy)",
		},
		{
			name: "select_static with initial_index",
			elem: cardObj{
				"tag":           "select_static",
				"initial_index": float64(1),
				"options": []interface{}{
					cardObj{"text": cardObj{"tag": "plain_text", "content": "First"}, "value": "a"},
					cardObj{"text": cardObj{"tag": "plain_text", "content": "Second"}, "value": "b"},
				},
			},
			want: "{First / ✓Second}",
		},
		{
			name: "div text with notation size",
			elem: cardObj{
				"tag": "div",
				"text": cardObj{
					"tag":       "plain_text",
					"content":   "小字注释",
					"text_size": "notation",
				},
			},
			want: "📝 小字注释",
		},
		{
			name: "form",
			elem: cardObj{
				"tag": "form",
				"elements": []interface{}{
					cardObj{"tag": "markdown", "content": "fill this"},
				},
			},
			want: "<form>\nfill this\n</form>",
		},
		{
			name: "collapsible_panel collapsed",
			elem: cardObj{
				"tag":      "collapsible_panel",
				"expanded": false,
				"header":   cardObj{"title": cardObj{"tag": "plain_text", "content": "Details"}},
				"elements": []interface{}{
					cardObj{"tag": "markdown", "content": "inner"},
				},
			},
			want: "▶ Details\n    inner\n▲",
		},
		{
			name: "collapsible_panel expanded",
			elem: cardObj{
				"tag":      "collapsible_panel",
				"expanded": true,
				"header":   cardObj{"title": cardObj{"tag": "plain_text", "content": "Details"}},
				"elements": []interface{}{
					cardObj{"tag": "markdown", "content": "inner"},
				},
			},
			want: "▼ Details\n    inner\n▲",
		},
		{
			name: "interactive_container with behaviors",
			elem: cardObj{
				"tag": "interactive_container",
				"behaviors": []interface{}{
					cardObj{"type": "open_url", "default_url": "https://example.com"},
				},
				"elements": []interface{}{
					cardObj{"tag": "markdown", "content": "Click here"},
				},
			},
			want: "<clickable url=\"https://example.com\">\nClick here\n</clickable>",
		},
		{
			name: "interactive_container no url",
			elem: cardObj{
				"tag": "interactive_container",
				"elements": []interface{}{
					cardObj{"tag": "markdown", "content": "No link"},
				},
			},
			want: "<clickable>\nNo link\n</clickable>",
		},
		{
			name: "column_set with buttons → space-joined",
			elem: cardObj{
				"tag": "column_set",
				"columns": []interface{}{
					cardObj{"tag": "column", "elements": []interface{}{
						cardObj{"tag": "button", "text": cardObj{"tag": "plain_text", "content": "X"}},
					}},
					cardObj{"tag": "column", "elements": []interface{}{
						cardObj{"tag": "button", "text": cardObj{"tag": "plain_text", "content": "Y"}},
					}},
				},
			},
			want: "[X] [Y]",
		},
		{
			name: "person",
			elem: cardObj{"tag": "person", "user_id": "fake-open-id-002"},
			want: "fake-open-id-002",
		},
		{
			name: "unknown tag fallback to content",
			elem: cardObj{"tag": "mystery", "content": "mystery content"},
			want: "mystery content",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := c.convertElement(tt.elem, 0)
			if tt.contains != "" {
				if !strings.Contains(got, tt.contains) {
					t.Fatalf("convertElement(%s) = %q, want to contain %q", tt.name, got, tt.contains)
				}
				return
			}
			if got != tt.want {
				t.Fatalf("convertElement(%s) = %q, want %q", tt.name, got, tt.want)
			}
		})
	}
}

func TestUserDslExtractButtonURL(t *testing.T) {
	c := &userDslConverter{}

	// direct url field wins first
	got := c.extractButtonURL(cardObj{
		"url":       "https://example.com/direct",
		"multi_url": cardObj{"url": "https://example.com/multi"},
		"behaviors": []interface{}{
			cardObj{"type": "open_url", "default_url": "https://example.com/behavior"},
		},
	})
	if got != "https://example.com/direct" {
		t.Fatalf("direct url = %q, want https://example.com/direct", got)
	}

	// multi_url.url when no direct url
	got = c.extractButtonURL(cardObj{
		"multi_url": cardObj{"url": "https://example.com/multi"},
		"behaviors": []interface{}{
			cardObj{"type": "open_url", "default_url": "https://example.com/behavior"},
		},
	})
	if got != "https://example.com/multi" {
		t.Fatalf("multi_url = %q, want https://example.com/multi", got)
	}

	// behaviors default_url as last resort
	got = c.extractButtonURL(cardObj{
		"behaviors": []interface{}{
			cardObj{"type": "open_url", "default_url": "https://example.com/behavior"},
		},
	})
	if got != "https://example.com/behavior" {
		t.Fatalf("behaviors = %q, want https://example.com/behavior", got)
	}

	// non-open_url behavior is ignored
	got = c.extractButtonURL(cardObj{
		"behaviors": []interface{}{
			cardObj{"type": "callback", "default_url": "https://example.com/callback"},
		},
	})
	if got != "" {
		t.Fatalf("non-open_url = %q, want empty", got)
	}

	// no url anywhere → empty
	got = c.extractButtonURL(cardObj{"text": cardObj{"content": "No URL"}})
	if got != "" {
		t.Fatalf("no url = %q, want empty", got)
	}
}

func TestUserDslExtractTableCellValue(t *testing.T) {
	c := &userDslConverter{}

	// nil
	if got := c.extractUserDslTableCellValue(nil); got != "" {
		t.Fatalf("nil = %q, want empty", got)
	}
	// string
	if got := c.extractUserDslTableCellValue("hello"); got != "hello" {
		t.Fatalf("string = %q, want 'hello'", got)
	}
	// float64 integer
	if got := c.extractUserDslTableCellValue(float64(42)); got != "42" {
		t.Fatalf("int float = %q, want '42'", got)
	}
	// float64 decimal
	if got := c.extractUserDslTableCellValue(float64(3.14)); got != "3.14" {
		t.Fatalf("float = %q, want '3.14'", got)
	}
	// []interface{} with text tags → 「text」 format
	got := c.extractUserDslTableCellValue([]interface{}{
		cardObj{"text": "S2", "color": "blue"},
		cardObj{"text": "M1", "color": "red"},
	})
	if got != "「S2」 「M1」" {
		t.Fatalf("tag array = %q, want '「S2」 「M1」'", got)
	}
	// cardObj with content field
	got = c.extractUserDslTableCellValue(cardObj{"content": "cell content"})
	if got != "cell content" {
		t.Fatalf("cardObj with content = %q, want 'cell content'", got)
	}
}

func TestUserDslConvertTable(t *testing.T) {
	c := &userDslConverter{}

	got := c.convertTable(cardObj{
		"columns": []interface{}{
			cardObj{"display_name": "客户名称", "name": "customer_name"},
			cardObj{"display_name": "规模", "name": "scale"},
			cardObj{"display_name": "金额", "name": "arr"},
		},
		"rows": []interface{}{
			cardObj{
				"customer_name": "飞书科技",
				"scale":         []interface{}{cardObj{"text": "S2", "color": "blue"}},
				"arr":           float64(16800),
			},
		},
	})
	want := "| 客户名称 | 规模 | 金额 |\n|------|------|------|\n| 飞书科技 | 「S2」 | 16800 |"
	if got != want {
		t.Fatalf("convertTable() = %q, want %q", got, want)
	}

	// no columns → empty
	if got := c.convertTable(cardObj{}); got != "" {
		t.Fatalf("no columns = %q, want empty", got)
	}
}

func TestLarkMdMentionResolution(t *testing.T) {
	mentions := []interface{}{
		map[string]interface{}{
			"key":  "@_user_1",
			"name": "test-user",
			"id":   map[string]interface{}{"open_id": "fake-uid-001"},
		},
	}

	// lark_md in div.text — the real Lark event format (C01 case)
	card := map[string]interface{}{
		"elements": []interface{}{
			map[string]interface{}{
				"tag": "div",
				"text": map[string]interface{}{
					"tag":     "lark_md",
					"content": "Hello <at id=fake-uid-001></at> check this.",
				},
			},
		},
	}
	dslBytes, _ := json.Marshal(card)
	raw, _ := json.Marshal(map[string]interface{}{"user_dsl": string(dslBytes)})
	got := ConvertInteractiveEventContent(string(raw), mentions)
	if strings.Contains(got, "<at") {
		t.Fatalf("div.text lark_md: raw <at> tag not resolved, got: %s", got)
	}
	if !strings.Contains(got, "@fake-uid-001") {
		t.Fatalf("div.text lark_md: @id not in output, got: %s", got)
	}

	// lark_md in note.elements (C02 case)
	card = map[string]interface{}{
		"elements": []interface{}{
			map[string]interface{}{
				"tag": "note",
				"elements": []interface{}{
					map[string]interface{}{
						"tag":     "lark_md",
						"content": "Note: <at id=fake-uid-001></at> check.",
					},
				},
			},
		},
	}
	dslBytes, _ = json.Marshal(card)
	raw, _ = json.Marshal(map[string]interface{}{"user_dsl": string(dslBytes)})
	got = ConvertInteractiveEventContent(string(raw), mentions)
	if strings.Contains(got, "<at") {
		t.Fatalf("note lark_md: raw <at> tag not resolved, got: %s", got)
	}
	if !strings.Contains(got, "@fake-uid-001") {
		t.Fatalf("note lark_md: @id not in output, got: %s", got)
	}

	// mention_key resolution via mentions map
	card = map[string]interface{}{
		"elements": []interface{}{
			map[string]interface{}{
				"tag": "div",
				"text": map[string]interface{}{
					"tag":     "lark_md",
					"content": `Hi <at mention_key="@_user_1">n</at> done.`,
				},
			},
		},
	}
	dslBytes, _ = json.Marshal(card)
	raw, _ = json.Marshal(map[string]interface{}{"user_dsl": string(dslBytes)})
	got = ConvertInteractiveEventContent(string(raw), mentions)
	if !strings.Contains(got, "@test-user(fake-uid-001)") {
		t.Fatalf("div.text lark_md mention_key: want @test-user(fake-uid-001), got: %s", got)
	}
}

func TestConvertUserDslCardEndToEnd(t *testing.T) {
	// user-2.ts format — matches structure of docs/user-dsl/user-example-2.json
	schema2JSON := `{
		"schema": "2.0",
		"header": {
			"title": {"tag": "plain_text", "content": "飞书卡片组件展示"},
			"template": "blue"
		},
		"body": {
			"elements": [
				{"tag": "markdown", "content": "### 基础文本"},
				{"tag": "hr"},
				{
					"tag": "img",
					"img_key": "img_v3_02122_abc",
					"alt": {"tag": "plain_text", "content": "示例图片"}
				},
				{
					"tag": "button",
					"text": {"tag": "plain_text", "content": "主要按钮"},
					"behaviors": [{"type": "open_url", "default_url": "https://example.com"}]
				},
				{
					"tag": "table",
					"columns": [
						{"display_name": "名称", "name": "name"},
						{"display_name": "数值", "name": "value"}
					],
					"rows": [
						{"name": "项目A", "value": 100},
						{"name": "项目B", "value": 200}
					]
				}
			]
		}
	}`

	got := convertUserDslCard(schema2JSON, nil)

	if !strings.HasPrefix(got, `<card title="飞书卡片组件展示">`) {
		t.Fatalf("e2e schema2: missing card title prefix, got: %s", got)
	}
	if !strings.Contains(got, "### 基础文本") {
		t.Fatal("e2e schema2: missing markdown content")
	}
	if !strings.Contains(got, "---") {
		t.Fatal("e2e schema2: missing hr")
	}
	if !strings.Contains(got, "🖼️ 示例图片(img_key:img_v3_02122_abc)") {
		t.Fatalf("e2e schema2: missing image, got: %s", got)
	}
	if !strings.Contains(got, "[主要按钮](https://example.com)") {
		t.Fatalf("e2e schema2: missing button, got: %s", got)
	}
	if !strings.Contains(got, "| 名称 | 数值 |") {
		t.Fatal("e2e schema2: missing table header")
	}
	if !strings.Contains(got, "| 项目A | 100 |") {
		t.Fatalf("e2e schema2: missing table row, got: %s", got)
	}
	if !strings.HasSuffix(got, "</card>") {
		t.Fatalf("e2e schema2: missing </card> suffix, got: %s", got)
	}

	// user-1.ts format
	schema1JSON := `{
		"i18n_header": {
			"zh_cn": {
				"title": {"tag": "plain_text", "content": "Schema1 卡片"},
				"template": "blue"
			}
		},
		"elements": [
			{"tag": "markdown", "content": "Hello **World**"},
			{
				"tag": "action",
				"actions": [
					{
						"tag": "button",
						"text": {"tag": "plain_text", "content": "跳转"},
						"behaviors": [{"type": "open_url", "default_url": "https://example.com"}]
					}
				]
			}
		]
	}`

	got = convertUserDslCard(schema1JSON, nil)
	if !strings.HasPrefix(got, `<card title="Schema1 卡片">`) {
		t.Fatalf("e2e schema1: missing card title, got: %s", got)
	}
	if !strings.Contains(got, "Hello **World**") {
		t.Fatal("e2e schema1: missing markdown")
	}
	if !strings.Contains(got, "[跳转](https://example.com)") {
		t.Fatalf("e2e schema1: missing button, got: %s", got)
	}
}
