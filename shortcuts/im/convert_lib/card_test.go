// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package convertlib

import (
	"strings"
	"testing"
)

func newTestCardConverter(mode cardMode) *cardConverter {
	return &cardConverter{
		mode: mode,
		attachment: cardObj{
			"persons": cardObj{
				"ou_person": cardObj{"content": "Alice"},
			},
			"at_users": cardObj{
				"ou_at": cardObj{"content": "Bob", "user_id": "u_bob"},
			},
			"images": cardObj{
				"img_1": cardObj{"token": "img_tok_1"},
			},
		},
	}
}

func TestConvertCard(t *testing.T) {
	rawCard := `{"json_card":"{\"schema\":1,\"header\":{\"title\":{\"content\":\"Card Title\"}},\"body\":{\"elements\":[{\"tag\":\"text\",\"property\":{\"content\":\"hello\"}},{\"tag\":\"button\",\"property\":{\"text\":{\"content\":\"Open\"},\"actions\":[{\"type\":\"open_url\",\"action\":{\"url\":\"https://example.com\"}}]}}]}}","json_attachment":"{\"persons\":{\"ou_1\":{\"content\":\"Alice\"}}}"}`
	got := convertCard(rawCard, nil)
	want := "<card title=\"Card Title\">\nhello\n[Open](https://example.com)\n</card>"
	if got != want {
		t.Fatalf("convertCard(json_card) = %q, want %q", got, want)
	}

	legacy := `{"header":{"title":{"content":"Legacy Card"}},"elements":[{"tag":"div","text":{"content":"legacy body"}}]}`
	gotLegacy := convertCard(legacy, nil)
	wantLegacy := "**Legacy Card**\nlegacy body"
	if gotLegacy != wantLegacy {
		t.Fatalf("convertCard(legacy) = %q, want %q", gotLegacy, wantLegacy)
	}
}

func TestCardUtilityFunctions(t *testing.T) {
	if !allColumnsAreButtons([]string{"[Open]", "[More](https://example.com)"}) {
		t.Fatal("allColumnsAreButtons() = false, want true")
	}
	if allColumnsAreButtons([]string{"plain text", "[Open]"}) {
		t.Fatal("allColumnsAreButtons() = true, want false")
	}
	if got := cardEscapeAttr("a\\\"b\nc\rd\t"); got != "a\\\\\\\"b\\nc\\rd\\t" {
		t.Fatalf("cardEscapeAttr() = %q", got)
	}
	if got := cardFormatMillisToISO8601("1710500000000"); got == "" {
		t.Fatal("cardFormatMillisToISO8601() returned empty")
	}
	if got := cardNormalizeTimeFormat("1710500000"); got == "1710500000" {
		t.Fatalf("cardNormalizeTimeFormat() did not normalize seconds: %q", got)
	}
	if got := cardNormalizeTimeFormat("2026-03-23"); got != "2026-03-23" {
		t.Fatalf("cardNormalizeTimeFormat() = %q, want original value", got)
	}
}

func TestCardConverterMethods(t *testing.T) {
	c := newTestCardConverter(cardModeDetailed)

	if got := c.convertLink(cardObj{"content": "Spec", "url": cardObj{"url": "https://example.com"}}); got != "[Spec](https://example.com)" {
		t.Fatalf("convertLink() = %q", got)
	}
	if got := c.convertMarkdown(cardObj{"content": "**bold**"}); got != "**bold**" {
		t.Fatalf("convertMarkdown() = %q", got)
	}
	if got := c.convertMarkdownV1(cardObj{"fallback": cardObj{"tag": "text", "property": cardObj{"content": "fallback"}}}, cardObj{}); got != "fallback" {
		t.Fatalf("convertMarkdownV1() = %q", got)
	}
	if got := c.convertDiv(cardObj{
		"text":   cardObj{"tag": "text", "property": cardObj{"content": "Title"}, "text_size": "notation"},
		"fields": []interface{}{cardObj{"text": cardObj{"tag": "text", "property": cardObj{"content": "Field 1"}}}},
		"extra":  cardObj{"tag": "text", "property": cardObj{"content": "Extra"}},
	}, ""); got != "📝 Title\nField 1\nExtra" {
		t.Fatalf("convertDiv() = %q", got)
	}
	if got := c.convertNote(cardObj{"elements": []interface{}{
		cardObj{"tag": "text", "property": cardObj{"content": "Tip"}},
		cardObj{"tag": "link", "property": cardObj{"content": "Doc", "url": cardObj{"url": "https://example.com/doc"}}},
	}}); got != "📝 Tip [Doc](https://example.com/doc)" {
		t.Fatalf("convertNote() = %q", got)
	}
	if got := c.convertEmoji(cardObj{"key": "OK"}); got != "👌" {
		t.Fatalf("convertEmoji() = %q", got)
	}
	if got := c.convertLocalDatetime(cardObj{"milliseconds": "1710500000000"}); got == "" {
		t.Fatal("convertLocalDatetime() returned empty")
	}
	if got := c.convertList(cardObj{"items": []interface{}{
		cardObj{"level": float64(0), "type": "ul", "elements": []interface{}{cardObj{"tag": "text", "property": cardObj{"content": "item1"}}}},
		cardObj{"level": float64(1), "type": "ol", "order": float64(2), "elements": []interface{}{cardObj{"tag": "text", "property": cardObj{"content": "item2"}}}},
	}}); got != "- item1\n  2. item2" {
		t.Fatalf("convertList() = %q", got)
	}
	if got := c.convertBlockquote(cardObj{"content": "line1\nline2"}); got != "> line1\n> line2" {
		t.Fatalf("convertBlockquote() = %q", got)
	}
	if got := c.convertCodeBlock(cardObj{"language": "go", "contents": []interface{}{
		cardObj{"contents": []interface{}{cardObj{"content": "fmt.Println(1)"}}},
	}}); got != "```go\nfmt.Println(1)```" {
		t.Fatalf("convertCodeBlock() = %q", got)
	}
	if got := c.convertCodeSpan(cardObj{"content": "x := 1"}); got != "`x := 1`" {
		t.Fatalf("convertCodeSpan() = %q", got)
	}
	if got := c.convertHeading(cardObj{"level": float64(2), "content": "Title"}); got != "## Title" {
		t.Fatalf("convertHeading() = %q", got)
	}
	if got := c.convertFallbackText(cardObj{"text": cardObj{"content": "fallback"}}); got != "fallback" {
		t.Fatalf("convertFallbackText() = %q", got)
	}
	if got := c.convertTextTag(cardObj{"text": cardObj{"content": "Tag"}}); got != "「Tag」" {
		t.Fatalf("convertTextTag() = %q", got)
	}
	if got := c.convertNumberTag(cardObj{"text": cardObj{"content": "42"}, "url": cardObj{"url": "https://example.com/42"}}); got != "[42](https://example.com/42)" {
		t.Fatalf("convertNumberTag() = %q", got)
	}
	if got := c.convertUnknown(cardObj{"title": cardObj{"content": "mystery"}}, "unknown"); got != "mystery" {
		t.Fatalf("convertUnknown() = %q", got)
	}
	if got := c.convertColumnSet(cardObj{"columns": []interface{}{
		cardObj{"tag": "column", "elements": []interface{}{cardObj{"tag": "button", "property": cardObj{"text": cardObj{"content": "A"}}}}},
		cardObj{"tag": "column", "elements": []interface{}{cardObj{"tag": "button", "property": cardObj{"text": cardObj{"content": "B"}}}}},
	}}, 0); got != "[A] [B]" {
		t.Fatalf("convertColumnSet() = %q", got)
	}
	if got := c.convertForm(cardObj{"elements": []interface{}{cardObj{"tag": "text", "property": cardObj{"content": "form body"}}}}, ""); got != "<form>\nform body\n</form>" {
		t.Fatalf("convertForm() = %q", got)
	}
	if got := c.convertCollapsiblePanel(cardObj{"expanded": true, "header": cardObj{"title": cardObj{"content": "More"}}, "elements": []interface{}{cardObj{"tag": "text", "property": cardObj{"content": "inside"}}}}, ""); got != "▼ More\n    inside\n▲" {
		t.Fatalf("convertCollapsiblePanel() = %q", got)
	}
	if got := c.convertInteractiveContainer(cardObj{"actions": []interface{}{cardObj{"type": "open_url", "action": cardObj{"url": "https://example.com"}}}, "elements": []interface{}{cardObj{"tag": "text", "property": cardObj{"content": "Click here"}}}}, "cta_1"); got != "<clickable url=\"https://example.com\" id=\"cta_1\">\nClick here\n</clickable>" {
		t.Fatalf("convertInteractiveContainer() = %q", got)
	}
	if got := c.convertRepeat(cardObj{"elements": []interface{}{cardObj{"tag": "text", "property": cardObj{"content": "repeat"}}}}); got != "repeat" {
		t.Fatalf("convertRepeat() = %q", got)
	}
	if got := c.convertActions(cardObj{"actions": []interface{}{
		cardObj{"tag": "button", "property": cardObj{"text": cardObj{"content": "One"}}},
		cardObj{"tag": "button", "property": cardObj{"text": cardObj{"content": "Two"}}},
	}}); got != "[One] [Two]" {
		t.Fatalf("convertActions() = %q", got)
	}
	if got := c.convertOverflow(cardObj{"options": []interface{}{
		cardObj{"text": cardObj{"content": "Edit"}},
		cardObj{"text": cardObj{"content": "Delete"}},
	}}); got != "⋮ Edit, Delete" {
		t.Fatalf("convertOverflow() = %q", got)
	}
	if got := c.convertSelect(cardObj{
		"options": []interface{}{
			cardObj{"text": cardObj{"content": "Alice"}, "value": "a"},
			cardObj{"text": cardObj{"content": "Bob"}, "value": "b"},
		},
		"selectedValues": []interface{}{"a"},
	}, "select_person", true); got != "{✓Alice / Bob}(multi type:person)" {
		t.Fatalf("convertSelect() = %q", got)
	}
	if got := c.convertSelectImg(cardObj{"options": []interface{}{cardObj{"value": "1"}, cardObj{"value": "2"}}, "selectedValues": []interface{}{"2"}}, ""); got != "{🖼️ Image 1 / ✓🖼️ Image 2}" {
		t.Fatalf("convertSelectImg() = %q", got)
	}
	if got := c.convertInput(cardObj{"label": cardObj{"content": "Reason"}, "placeholder": cardObj{"content": "Type"}, "inputType": "multiline_text"}, ""); got != "Reason: Type..." {
		t.Fatalf("convertInput() = %q", got)
	}
	if got := c.convertDatePicker(cardObj{"initialDate": "1710500000"}, "", "date"); got == "" || !strings.HasPrefix(got, "📅 ") {
		t.Fatalf("convertDatePicker(date) = %q", got)
	}
	if got := c.convertChecker(cardObj{"checked": true, "text": cardObj{"content": "Done"}}, "chk_1"); got != "[x] Done(id:chk_1)" {
		t.Fatalf("convertChecker() = %q", got)
	}
	if got := c.convertImage(cardObj{"alt": cardObj{"content": "Poster"}, "imageID": "img_1"}, ""); got != "🖼️ Poster(img_token:img_tok_1)" {
		t.Fatalf("convertImage() = %q", got)
	}
	if got := c.convertImgCombination(cardObj{"imgList": []interface{}{cardObj{"imageID": "img_1"}, cardObj{"imageID": "img_2"}}}); got != "🖼️ 2 image(s)(keys:img_1,img_2)" {
		t.Fatalf("convertImgCombination() = %q", got)
	}
	if got := c.convertChart(cardObj{"chartSpec": cardObj{
		"title":  cardObj{"text": "Sales"},
		"type":   "bar",
		"xField": "month",
		"yField": "value",
		"data": cardObj{"values": []interface{}{
			cardObj{"month": "Jan", "value": 10},
			cardObj{"month": "Feb", "value": 20},
		}},
	}}, ""); got != "📊 SalesBar chart\nSummary: Jan:10, Feb:20" {
		t.Fatalf("convertChart() = %q", got)
	}
	if got := c.convertAudio(cardObj{"fileID": "audio_1"}, ""); got != "🎵 Audio(key:audio_1)" {
		t.Fatalf("convertAudio() = %q", got)
	}
	if got := c.convertVideo(cardObj{"videoID": "video_1"}, ""); got != "🎬 Video(key:video_1)" {
		t.Fatalf("convertVideo() = %q", got)
	}
	if got := c.convertTable(cardObj{
		"columns": []interface{}{
			cardObj{"displayName": "Name", "name": "name"},
			cardObj{"displayName": "Score", "name": "score"},
		},
		"rows": []interface{}{
			cardObj{
				"name":  cardObj{"data": "Alice"},
				"score": cardObj{"data": float64(95.5)},
			},
		},
	}); got != "| Name | Score |\n|------|------|\n| Alice | 95.50 |" {
		t.Fatalf("convertTable() = %q", got)
	}
	if got := c.extractTableCellValue([]interface{}{cardObj{"text": "Tag 1"}, cardObj{"text": "Tag 2"}}); got != "「Tag 1」 「Tag 2」" {
		t.Fatalf("extractTableCellValue() = %q", got)
	}
	if got := c.convertPerson(cardObj{"userID": "ou_person"}, ""); got != "@Alice(open_id:ou_person)" {
		t.Fatalf("convertPerson() = %q", got)
	}
	if got := c.convertPersonV1(cardObj{"userID": "ou_person"}, ""); got != "@Alice(open_id:ou_person)" {
		t.Fatalf("convertPersonV1() = %q", got)
	}
	if got := c.convertPersonList(cardObj{"persons": []interface{}{cardObj{"id": "u1"}, cardObj{"id": "u2"}}}); got != "@user(id:u1), @user(id:u2)" {
		t.Fatalf("convertPersonList() = %q", got)
	}
	if got := c.convertAvatar(cardObj{"userID": "ou_person"}, ""); got != "👤(id:ou_person)" {
		t.Fatalf("convertAvatar() = %q", got)
	}
	if got := c.convertAt(cardObj{"userID": "ou_at"}); got != "@Bob(user_id:u_bob)" {
		t.Fatalf("convertAt() = %q", got)
	}
	if style := c.extractTextStyle(cardObj{"textStyle": cardObj{"attributes": []interface{}{"bold", "italic", "strikethrough"}}}); !style.bold || !style.italic || !style.strikethrough {
		t.Fatalf("extractTextStyle() = %#v", style)
	}
	if got := c.applyTextStyle("hello", cardObj{"textStyle": cardObj{"attributes": []interface{}{"bold", "italic"}}}); got != "***hello***" {
		t.Fatalf("applyTextStyle() = %q", got)
	}
	if got := (interactiveConverter{}).Convert(&ConvertContext{RawContent: `{"json_card":"{\"body\":{\"elements\":[{\"tag\":\"text\",\"property\":{\"content\":\"inside\"}}]}}"}`}); got != "<card>\ninside\n</card>" {
		t.Fatalf("interactiveConverter.Convert() = %q", got)
	}
}

func TestConvertAtWithMentions(t *testing.T) {
	mentions := []interface{}{
		map[string]interface{}{
			"key":  "@_user_1",
			"id":   "ou_6b64bef911a5a3ea763df8ffd9258f59",
			"name": "燕忠毅",
		},
	}
	attachment := cardObj{
		"at_users": cardObj{
			"cde8a6c8": cardObj{
				"user_id":     "754700000001",
				"content":     "燕忠毅",
				"mention_key": "@_user_1",
			},
		},
	}

	// Concise mode: should show @Name(open_id) when mention resolves.
	concise := &cardConverter{
		mode:          cardModeConcise,
		attachment:    attachment,
		mentionsByKey: buildMentionsByKey(mentions),
	}
	if got := concise.convertAt(cardObj{"userID": "cde8a6c8"}); got != "@燕忠毅(ou_6b64bef911a5a3ea763df8ffd9258f59)" {
		t.Fatalf("convertAt(concise with mentions) = %q", got)
	}

	// Detailed mode: label should be open_id when resolved from mentions.
	detailed := &cardConverter{
		mode:          cardModeDetailed,
		attachment:    attachment,
		mentionsByKey: buildMentionsByKey(mentions),
	}
	if got := detailed.convertAt(cardObj{"userID": "cde8a6c8"}); got != "@燕忠毅(open_id:ou_6b64bef911a5a3ea763df8ffd9258f59)" {
		t.Fatalf("convertAt(detailed with mentions) = %q", got)
	}

	// No mention_key: falls back to at_users.user_id with user_id label (existing behavior).
	noMentionKey := &cardConverter{
		mode: cardModeDetailed,
		attachment: cardObj{
			"at_users": cardObj{
				"ou_at": cardObj{"content": "Bob", "user_id": "u_bob"},
			},
		},
	}
	if got := noMentionKey.convertAt(cardObj{"userID": "ou_at"}); got != "@Bob(user_id:u_bob)" {
		t.Fatalf("convertAt(fallback no mention_key) = %q", got)
	}

	// mention_key present but mentionsByKey nil: still falls back gracefully.
	nilMentions := &cardConverter{
		mode: cardModeDetailed,
		attachment: cardObj{
			"at_users": cardObj{
				"cde8a6c8": cardObj{
					"user_id":     "754700000001",
					"content":     "燕忠毅",
					"mention_key": "@_user_1",
				},
			},
		},
	}
	if got := nilMentions.convertAt(cardObj{"userID": "cde8a6c8"}); got != "@燕忠毅(user_id:754700000001)" {
		t.Fatalf("convertAt(fallback nil mentionsByKey) = %q", got)
	}
}

func TestCardConverterExtractTextHelpers(t *testing.T) {
	c := newTestCardConverter(cardModeDetailed)

	if got := c.extractTextFromProperty(cardObj{
		"i18nContent": cardObj{
			"zh_cn": "你好",
			"en_us": "hello",
		},
	}); got != "你好" {
		t.Fatalf("extractTextFromProperty(i18n) = %q", got)
	}

	if got := c.extractTextFromProperty(cardObj{"content": "content-first"}); got != "content-first" {
		t.Fatalf("extractTextFromProperty(content) = %q", got)
	}

	if got := c.extractTextFromProperty(cardObj{
		"elements": []interface{}{
			cardObj{"property": cardObj{"content": "A"}},
			cardObj{"content": "B"},
			123,
		},
	}); got != "AB" {
		t.Fatalf("extractTextFromProperty(elements) = %q", got)
	}

	if got := c.extractTextFromProperty(cardObj{"text": "plain-text"}); got != "plain-text" {
		t.Fatalf("extractTextFromProperty(text) = %q", got)
	}

	if got := c.extractTextContent(cardObj{"property": cardObj{"content": "wrapped"}}); got != "wrapped" {
		t.Fatalf("extractTextContent(property) = %q", got)
	}

	if got := c.extractTextFromProperty(cardObj{}); got != "" {
		t.Fatalf("extractTextFromProperty(empty) = %q, want empty", got)
	}
}

func TestCardConverterDispatch(t *testing.T) {
	c := newTestCardConverter(cardModeDetailed)

	tests := []struct {
		name     string
		elem     cardObj
		want     string
		contains string
	}{
		{name: "plain text", elem: cardObj{"tag": "plain_text", "property": cardObj{"content": "hello"}}, want: "hello"},
		{name: "markdown", elem: cardObj{"tag": "markdown", "property": cardObj{"content": "**bold**"}}, want: "**bold**"},
		{name: "markdown v1", elem: cardObj{"tag": "markdown_v1", "fallback": cardObj{"tag": "text", "property": cardObj{"content": "fallback"}}}, want: "fallback"},
		{name: "div", elem: cardObj{"tag": "div", "property": cardObj{"text": cardObj{"tag": "text", "property": cardObj{"content": "Body"}}}}, want: "Body"},
		{name: "note", elem: cardObj{"tag": "note", "property": cardObj{"elements": []interface{}{cardObj{"tag": "text", "property": cardObj{"content": "Tip"}}}}}, want: "📝 Tip"},
		{name: "hr", elem: cardObj{"tag": "hr"}, want: "---"},
		{name: "br", elem: cardObj{"tag": "br"}, want: "\n"},
		{name: "column set", elem: cardObj{"tag": "column_set", "property": cardObj{"columns": []interface{}{
			cardObj{"tag": "column", "elements": []interface{}{cardObj{"tag": "button", "property": cardObj{"text": cardObj{"content": "A"}}}}},
			cardObj{"tag": "column", "elements": []interface{}{cardObj{"tag": "button", "property": cardObj{"text": cardObj{"content": "B"}}}}},
		}}}, want: "[A] [B]"},
		{name: "person", elem: cardObj{"tag": "person", "property": cardObj{"userID": "ou_person"}}, want: "@Alice(open_id:ou_person)"},
		{name: "at", elem: cardObj{"tag": "at", "property": cardObj{"userID": "ou_at"}}, want: "@Bob(user_id:u_bob)"},
		{name: "at all", elem: cardObj{"tag": "at_all"}, want: "@everyone"},
		{name: "actions", elem: cardObj{"tag": "actions", "property": cardObj{"actions": []interface{}{
			cardObj{"tag": "button", "property": cardObj{"text": cardObj{"content": "One"}}},
			cardObj{"tag": "button", "property": cardObj{"text": cardObj{"content": "Two"}}},
		}}}, want: "[One] [Two]"},
		{name: "input", elem: cardObj{"tag": "input", "property": cardObj{"label": cardObj{"content": "Reason"}, "placeholder": cardObj{"content": "Type"}, "inputType": "multiline_text"}}, want: "Reason: Type..."},
		{name: "date", elem: cardObj{"tag": "date_picker", "property": cardObj{"initialDate": "1710500000"}}, contains: "📅 "},
		{name: "checker", elem: cardObj{"tag": "checker", "id": "chk_1", "property": cardObj{"checked": true, "text": cardObj{"content": "Done"}}}, want: "[x] Done(id:chk_1)"},
		{name: "image", elem: cardObj{"tag": "image", "property": cardObj{"alt": cardObj{"content": "Poster"}, "imageID": "img_1"}}, want: "🖼️ Poster(img_token:img_tok_1)"},
		{name: "interactive", elem: cardObj{"tag": "interactive_container", "id": "cta_1", "property": cardObj{
			"actions":  []interface{}{cardObj{"type": "open_url", "action": cardObj{"url": "https://example.com"}}},
			"elements": []interface{}{cardObj{"tag": "text", "property": cardObj{"content": "Click here"}}},
		}}, want: "<clickable url=\"https://example.com\" id=\"cta_1\">\nClick here\n</clickable>"},
		{name: "text tag", elem: cardObj{"tag": "text_tag", "property": cardObj{"text": cardObj{"content": "Tag"}}}, want: "「Tag」"},
		{name: "link", elem: cardObj{"tag": "link", "property": cardObj{"content": "Spec", "url": cardObj{"url": "https://example.com"}}}, want: "[Spec](https://example.com)"},
		{name: "emoji", elem: cardObj{"tag": "emoji", "property": cardObj{"key": "OK"}}, want: "👌"},
		{name: "card header suppressed", elem: cardObj{"tag": "card_header"}, want: ""},
		{name: "unknown", elem: cardObj{"tag": "mystery", "property": cardObj{"title": cardObj{"content": "mystery"}}}, want: "mystery"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := c.convertElement(tt.elem, 0)
			if tt.contains != "" {
				if !strings.Contains(got, tt.contains) {
					t.Fatalf("convertElement(%s) = %q, want containing %q", tt.name, got, tt.contains)
				}
				return
			}
			if got != tt.want {
				t.Fatalf("convertElement(%s) = %q, want %q", tt.name, got, tt.want)
			}
		})
	}
}
