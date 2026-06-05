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
				"img_1": cardObj{"token": "img_tok_1", "origin_key": "img_v3_test_key1"},
			},
			"option_users": cardObj{
				"opt_alice": cardObj{"content": "Alice"},
				"opt_bob":   cardObj{"content": "Bob"},
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

	// C008 root cause: json_attachment as object (not string) — persons resolved via attachment
	withObjAttachment := `{"json_card":"{\"schema\":1,\"header\":{\"title\":{\"content\":\"Title\"}},\"body\":{\"elements\":[{\"tag\":\"person\",\"property\":{\"userID\":\"ou_1\"}}]}}","json_attachment":{"persons":{"ou_1":{"content":"Alice"}}}}`
	if got := convertCard(withObjAttachment, nil); !strings.Contains(got, "Alice") {
		t.Fatalf("convertCard(json_attachment object) = %q, want person name resolved", got)
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
		"text":   cardObj{"tag": "text", "property": cardObj{"content": "Title", "textStyle": cardObj{"size": "notation"}}},
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
	// select_person with no option text: names resolved from option_users attachment
	if got := c.convertSelect(cardObj{
		"options": []interface{}{
			cardObj{"value": "opt_alice"},
			cardObj{"value": "opt_bob"},
		},
		"selectedValues": []interface{}{"opt_alice"},
	}, "select_person", true); got != "{✓Alice / Bob}(multi type:person)" {
		t.Fatalf("convertSelect(person no-text) = %q", got)
	}
	if got := c.convertSelectImg(cardObj{"options": []interface{}{cardObj{"value": "1"}, cardObj{"value": "2"}}, "selectedValues": []interface{}{"2"}}, ""); got != "{🖼️ Image 1(1) / ✓🖼️ Image 2(2)}" {
		t.Fatalf("convertSelectImg() = %q", got)
	}
	if got := c.convertSelectImg(cardObj{"options": []interface{}{cardObj{"value": "opt_a", "imageID": "img_1"}, cardObj{"value": "opt_b"}}, "selectedValues": []interface{}{"opt_a"}}, ""); got != "{✓🖼️ Image 1(opt_a)(img_key:img_v3_test_key1) / 🖼️ Image 2(opt_b)}" {
		t.Fatalf("convertSelectImg(with imageID) = %q", got)
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
	if got := c.convertImage(cardObj{"alt": cardObj{"content": "Poster"}, "imageID": "img_1"}, ""); got != "🖼️ Poster(img_key:img_v3_test_key1)" {
		t.Fatalf("convertImage() = %q", got)
	}
	if got := c.convertImgCombination(cardObj{"imgList": []interface{}{cardObj{"imageID": "img_1"}, cardObj{"imageID": "img_2"}}}); got != "🖼️ 2 image(s)(keys:img_v3_test_key1,img_2)" {
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
	}}, ""); got != "📊 Sales (Bar chart)\nSummary: Jan:10, Feb:20" {
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
				"score": cardObj{"data": "95.5"},
			},
		},
	}); got != "| Name | Score |\n|------|------|\n| Alice | 95.5 |" {
		t.Fatalf("convertTable() = %q", got)
	}
	if got := c.extractTableCellValue([]interface{}{cardObj{"text": "Tag 1"}, cardObj{"text": "Tag 2"}}); got != "「Tag 1」 「Tag 2」" {
		t.Fatalf("extractTableCellValue() = %q", got)
	}
	if got := c.extractTableCellValue("[map[text:VIP] map[text:Premium]]"); got != "VIP, Premium" {
		t.Fatalf("extractTableCellValue(go-format array) = %q", got)
	}
	if got := c.extractTableCellValue("[map[text:VIP Plus] map[text:Premium Pro]]"); got != "VIP Plus, Premium Pro" {
		t.Fatalf("extractTableCellValue(go-format array with spaces) = %q", got)
	}
	if got := c.extractTableCellValue("[map[bold:true text:VIP Plus] map[text:Premium Pro bold:false]]"); got != "VIP Plus, Premium Pro" {
		t.Fatalf("extractTableCellValue(go-format array multi-key) = %q", got)
	}
	if got := c.convertPerson(cardObj{"userID": "ou_person"}, ""); got != "Alice(open_id:ou_person)" {
		t.Fatalf("convertPerson() = %q", got)
	}
	if got := c.convertPersonV1(cardObj{"userID": "ou_person"}, ""); got != "Alice(open_id:ou_person)" {
		t.Fatalf("convertPersonV1() = %q", got)
	}
	if got := c.convertPersonList(cardObj{"persons": []interface{}{cardObj{"id": "u1"}, cardObj{"id": "ou_person"}}}); got != "user(id:u1), Alice(open_id:ou_person)" {
		t.Fatalf("convertPersonList() = %q", got)
	}
	if got := c.convertAvatar(cardObj{"userID": "ou_person"}, ""); got != "👤 Alice(open_id:ou_person)" {
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

	// C001: collapsible panel in concise mode (collapsed) must still render content
	cc := newTestCardConverter(cardModeConcise)
	if got := cc.convertCollapsiblePanel(cardObj{
		"expanded": false,
		"header":   cardObj{"title": cardObj{"content": "Details"}},
		"elements": []interface{}{cardObj{"tag": "text", "property": cardObj{"content": "hidden info"}}},
	}, ""); !strings.Contains(got, "hidden info") {
		t.Fatalf("convertCollapsiblePanel(concise,collapsed) = %q, want content rendered", got)
	}

	// C002: extractHeaderSubtitle
	if got := c.extractHeaderSubtitle(cardObj{"property": cardObj{
		"subtitle": cardObj{"property": cardObj{"content": "Q3 Budget"}},
	}}); got != "Q3 Budget" {
		t.Fatalf("extractHeaderSubtitle() = %q", got)
	}

	// C003: extractHeaderTags
	if got := c.extractHeaderTags(cardObj{"textTagList": []interface{}{
		cardObj{"tag": "text_tag", "property": cardObj{"text": cardObj{"content": "Approved"}}},
	}}); got != "「Approved」" {
		t.Fatalf("extractHeaderTags() = %q", got)
	}

	// C007: convertButton disabled with disabledTips
	if got := c.convertButton(cardObj{
		"text":         cardObj{"content": "Submit"},
		"disabled":     true,
		"disabledTips": cardObj{"content": "Only managers can submit"},
	}, ""); got != "[Submit ✗](tips:\"Only managers can submit\")" {
		t.Fatalf("convertButton(disabled+tips) = %q", got)
	}
}

func TestConvertAtWithMentions(t *testing.T) {
	mentions := []interface{}{
		map[string]interface{}{
			"key":  "@_user_1",
			"id":   "ou_xxxx",
			"name": "测试用户",
		},
	}
	attachment := cardObj{
		"at_users": cardObj{
			"xxxxx": cardObj{
				"user_id":     "0000000001",
				"content":     "测试用户",
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
	if got := concise.convertAt(cardObj{"userID": "xxxxx"}); got != "@测试用户(ou_xxxx)" {
		t.Fatalf("convertAt(concise with mentions) = %q", got)
	}

	// Detailed mode: label should be open_id when resolved from mentions.
	detailed := &cardConverter{
		mode:          cardModeDetailed,
		attachment:    attachment,
		mentionsByKey: buildMentionsByKey(mentions),
	}
	if got := detailed.convertAt(cardObj{"userID": "xxxxx"}); got != "@测试用户(open_id:ou_xxxx)" {
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
				"xxxxx": cardObj{
					"user_id":     "0000000001",
					"content":     "测试用户",
					"mention_key": "@_user_1",
				},
			},
		},
	}
	if got := nilMentions.convertAt(cardObj{"userID": "xxxxx"}); got != "@测试用户(user_id:0000000001)" {
		t.Fatalf("convertAt(fallback nil mentionsByKey) = %q", got)
	}
}

func TestCardConverterCoverageGaps(t *testing.T) {
	dc := newTestCardConverter(cardModeDetailed)
	cc := newTestCardConverter(cardModeConcise)

	// ── convertCard edge cases ────────────────────────────────────────────────

	// invalid JSON → fallback label
	if got := convertCard("not-json", nil); got != "[interactive card]" {
		t.Fatalf("convertCard(invalid JSON) = %q", got)
	}
	// empty card body → card wrapper with no content
	if got := convertCard(`{"json_card":"{\"body\":{\"elements\":[]}}"}`, nil); got != "<card>\n</card>" {
		t.Fatalf("convertCard(empty body) = %q", got)
	}
	// card_schema field is read without error
	withSchema := `{"json_card":"{\"body\":{\"elements\":[{\"tag\":\"text\",\"property\":{\"content\":\"hi\"}}]}}","card_schema":2}`
	if got := convertCard(withSchema, nil); !strings.Contains(got, "hi") {
		t.Fatalf("convertCard(card_schema) = %q", got)
	}

	// ── buildMentionsByKey ────────────────────────────────────────────────────

	// non-map entry is skipped gracefully
	m := buildMentionsByKey([]interface{}{
		"not-a-map",
		map[string]interface{}{"key": "@_user_x", "name": "Test User"},
		map[string]interface{}{"name": "no-key"},
	})
	if _, ok := m["@_user_x"]; !ok {
		t.Fatal("buildMentionsByKey: valid entry missing")
	}
	if len(m) != 1 {
		t.Fatalf("buildMentionsByKey: expected 1 entry, got %d", len(m))
	}

	// ── convertLegacyCard ────────────────────────────────────────────────────

	// no texts → fallback
	if got := convertLegacyCard(cardObj{}); got != "[interactive card]" {
		t.Fatalf("convertLegacyCard(empty) = %q", got)
	}
	// elements at top level (not under body)
	legacyTopLevel := cardObj{
		"header":   cardObj{"title": cardObj{"content": "Title"}},
		"elements": []interface{}{cardObj{"tag": "markdown", "content": "body text"}},
	}
	if got := convertLegacyCard(legacyTopLevel); !strings.Contains(got, "Title") {
		t.Fatalf("convertLegacyCard(top-level elements) = %q", got)
	}
	// body elements path
	legacyBodyElements := cardObj{
		"body": cardObj{"elements": []interface{}{cardObj{"tag": "markdown", "content": "body text"}}},
	}
	if got := convertLegacyCard(legacyBodyElements); !strings.Contains(got, "body text") {
		t.Fatalf("convertLegacyCard(body elements) = %q", got)
	}

	// ── legacyExtractTexts ───────────────────────────────────────────────────

	var out []string
	// div with text.content
	legacyExtractTexts([]interface{}{
		cardObj{"tag": "div", "text": cardObj{"content": "div-text"}},
		cardObj{"tag": "plain_text", "content": "plain-content"},
	}, &out)
	if len(out) != 2 || out[0] != "div-text" || out[1] != "plain-content" {
		t.Fatalf("legacyExtractTexts(div+plain_text) = %v", out)
	}
	// column_set recurses into columns
	out = nil
	legacyExtractTexts([]interface{}{
		cardObj{"tag": "column_set", "columns": []interface{}{
			cardObj{"elements": []interface{}{cardObj{"tag": "markdown", "content": "col-text"}}},
		}},
	}, &out)
	if len(out) != 1 || out[0] != "col-text" {
		t.Fatalf("legacyExtractTexts(column_set) = %v", out)
	}
	// generic elements fallback
	out = nil
	legacyExtractTexts([]interface{}{
		cardObj{"tag": "unknown_parent", "elements": []interface{}{
			cardObj{"tag": "markdown", "content": "nested"},
		}},
	}, &out)
	if len(out) != 1 || out[0] != "nested" {
		t.Fatalf("legacyExtractTexts(elements fallback) = %v", out)
	}
	// non-map element skipped
	out = nil
	legacyExtractTexts([]interface{}{"not-a-map"}, &out)
	if len(out) != 0 {
		t.Fatalf("legacyExtractTexts(non-map) = %v", out)
	}

	// ── convert (cardConverter.convert) ─────────────────────────────────────

	conv := &cardConverter{mode: cardModeDetailed}
	// invalid JSON
	if got := conv.convert("bad-json", 0); got != "<card>\n[Unable to parse card content]\n</card>" {
		t.Fatalf("convert(bad-json) = %q", got)
	}
	// subtitle only (no title)
	subtitleOnlyCard := `{"header":{"subtitle":{"content":"Sub"}},"body":{"elements":[{"tag":"text","property":{"content":"body"}}]}}`
	if got := conv.convert(subtitleOnlyCard, 0); !strings.Contains(got, `subtitle="Sub"`) {
		t.Fatalf("convert(subtitle only) = %q", got)
	}
	// title + subtitle
	bothCard := `{"header":{"title":{"content":"T"},"subtitle":{"content":"S"}},"body":{"elements":[{"tag":"text","property":{"content":"b"}}]}}`
	if got := conv.convert(bothCard, 0); !strings.Contains(got, `title="T" subtitle="S"`) {
		t.Fatalf("convert(title+subtitle) = %q", got)
	}
	// headerTags present
	tagsCard := `{"header":{"textTagList":[{"tag":"text_tag","property":{"text":{"content":"Tag1"}}}]},"body":{"elements":[{"tag":"text","property":{"content":"body"}}]}}`
	if got := conv.convert(tagsCard, 0); !strings.Contains(got, "「Tag1」") {
		t.Fatalf("convert(headerTags) = %q", got)
	}
	// empty body
	noBodyCard := `{"header":{"title":{"content":"Empty"}}}`
	if got := conv.convert(noBodyCard, 0); !strings.Contains(got, `title="Empty"`) {
		t.Fatalf("convert(empty body) = %q", got)
	}

	// ── extractHeaderTitle ───────────────────────────────────────────────────

	// flat header["title"] (no property wrapper)
	if got := dc.extractHeaderTitle(cardObj{"title": cardObj{"content": "Flat Title"}}); got != "Flat Title" {
		t.Fatalf("extractHeaderTitle(flat) = %q", got)
	}
	// no title at all
	if got := dc.extractHeaderTitle(cardObj{}); got != "" {
		t.Fatalf("extractHeaderTitle(empty) = %q", got)
	}

	// ── extractHeaderSubtitle ────────────────────────────────────────────────

	// flat path
	if got := dc.extractHeaderSubtitle(cardObj{"subtitle": cardObj{"content": "Flat Sub"}}); got != "Flat Sub" {
		t.Fatalf("extractHeaderSubtitle(flat) = %q", got)
	}

	// ── extractHeaderTags ────────────────────────────────────────────────────

	// flat header (no property wrapper)
	if got := dc.extractHeaderTags(cardObj{"textTagList": []interface{}{
		cardObj{"tag": "text_tag", "property": cardObj{"text": cardObj{"content": "Flat"}}},
	}}); got != "「Flat」" {
		t.Fatalf("extractHeaderTags(flat) = %q", got)
	}
	// empty tag list
	if got := dc.extractHeaderTags(cardObj{}); got != "" {
		t.Fatalf("extractHeaderTags(empty) = %q", got)
	}
	// all tags produce empty content
	if got := dc.extractHeaderTags(cardObj{"textTagList": []interface{}{cardObj{"tag": "text_tag", "property": cardObj{}}}}); got != "" {
		t.Fatalf("extractHeaderTags(all-empty) = %q", got)
	}
	// non-map entry in tag list is skipped
	if got := dc.extractHeaderTags(cardObj{"textTagList": []interface{}{
		"not-a-map",
		cardObj{"tag": "text_tag", "property": cardObj{"text": cardObj{"content": "Valid"}}},
	}}); got != "「Valid」" {
		t.Fatalf("extractHeaderTags(non-map skip) = %q", got)
	}

	// ── convertBody ──────────────────────────────────────────────────────────

	// property-wrapped elements
	bodyWithProp := cardObj{
		"property": cardObj{
			"elements": []interface{}{cardObj{"tag": "text", "property": cardObj{"content": "prop-elem"}}},
		},
	}
	if got := dc.convertBody(bodyWithProp); got != "prop-elem" {
		t.Fatalf("convertBody(property-wrapped) = %q", got)
	}
	// empty body
	if got := dc.convertBody(cardObj{}); got != "" {
		t.Fatalf("convertBody(empty) = %q", got)
	}

	// ── convertElements ──────────────────────────────────────────────────────

	// non-map element is skipped
	if got := dc.convertElements([]interface{}{"not-a-map", cardObj{"tag": "text", "property": cardObj{"content": "ok"}}}, 0); got != "ok" {
		t.Fatalf("convertElements(non-map skip) = %q", got)
	}

	// ── extractTextContent ───────────────────────────────────────────────────

	if got := dc.extractTextContent(nil); got != "" {
		t.Fatalf("extractTextContent(nil) = %q", got)
	}
	if got := dc.extractTextContent("string-val"); got != "string-val" {
		t.Fatalf("extractTextContent(string) = %q", got)
	}
	// non-map, non-string → ""
	if got := dc.extractTextContent(42); got != "" {
		t.Fatalf("extractTextContent(int) = %q", got)
	}

	// ── convertPlainText ─────────────────────────────────────────────────────

	if got := dc.convertPlainText(cardObj{"content": ""}); got != "" {
		t.Fatalf("convertPlainText(empty) = %q", got)
	}

	// ── convertMarkdown ──────────────────────────────────────────────────────

	// elements path
	if got := dc.convertMarkdown(cardObj{"elements": []interface{}{cardObj{"tag": "text", "property": cardObj{"content": "elem-md"}}}}); got != "elem-md" {
		t.Fatalf("convertMarkdown(elements) = %q", got)
	}
	// empty → ""
	if got := dc.convertMarkdown(cardObj{}); got != "" {
		t.Fatalf("convertMarkdown(empty) = %q", got)
	}

	// ── convertMarkdownV1 ────────────────────────────────────────────────────

	// content fallback (no elements, no fallback)
	if got := dc.convertMarkdownV1(cardObj{}, cardObj{"content": "v1-content"}); got != "v1-content" {
		t.Fatalf("convertMarkdownV1(content fallback) = %q", got)
	}
	// all empty
	if got := dc.convertMarkdownV1(cardObj{}, cardObj{}); got != "" {
		t.Fatalf("convertMarkdownV1(empty) = %q", got)
	}
	// elements path
	if got := dc.convertMarkdownV1(cardObj{}, cardObj{"elements": []interface{}{cardObj{"tag": "text", "property": cardObj{"content": "v1-elem"}}}}); got != "v1-elem" {
		t.Fatalf("convertMarkdownV1(elements) = %q", got)
	}

	// ── convertLink ──────────────────────────────────────────────────────────

	// no URL → return content as-is
	if got := dc.convertLink(cardObj{"content": "Plain Link"}); got != "Plain Link" {
		t.Fatalf("convertLink(no url) = %q", got)
	}
	// empty content defaults to "Link"
	if got := dc.convertLink(cardObj{}); got != "Link" {
		t.Fatalf("convertLink(empty content) = %q", got)
	}

	// ── convertEmoji ─────────────────────────────────────────────────────────

	if got := dc.convertEmoji(cardObj{"key": "WAVE"}); got != ":WAVE:" {
		t.Fatalf("convertEmoji(unknown key) = %q", got)
	}

	// ── convertLocalDatetime ─────────────────────────────────────────────────

	// float64 milliseconds
	if got := dc.convertLocalDatetime(cardObj{"milliseconds": float64(1710500000000)}); got == "" {
		t.Fatal("convertLocalDatetime(float64) returned empty")
	}
	// no milliseconds → fallback text
	if got := dc.convertLocalDatetime(cardObj{"fallbackText": "sometime"}); got != "sometime" {
		t.Fatalf("convertLocalDatetime(fallback) = %q", got)
	}

	// ── convertBlockquote ────────────────────────────────────────────────────

	// elements path
	if got := dc.convertBlockquote(cardObj{"elements": []interface{}{cardObj{"tag": "text", "property": cardObj{"content": "quote elem"}}}}); got != "> quote elem" {
		t.Fatalf("convertBlockquote(elements) = %q", got)
	}
	// empty → ""
	if got := dc.convertBlockquote(cardObj{}); got != "" {
		t.Fatalf("convertBlockquote(empty) = %q", got)
	}

	// ── convertCodeBlock ─────────────────────────────────────────────────────

	// no language → "plaintext"
	if got := dc.convertCodeBlock(cardObj{"contents": []interface{}{cardObj{"contents": []interface{}{cardObj{"content": "x"}}}}}); !strings.HasPrefix(got, "```plaintext") {
		t.Fatalf("convertCodeBlock(no language) = %q", got)
	}
	// non-map line content skipped
	if got := dc.convertCodeBlock(cardObj{"language": "go", "contents": []interface{}{"not-a-map"}}); got != "```go\n```" {
		t.Fatalf("convertCodeBlock(non-map line) = %q", got)
	}

	// ── convertHeading ───────────────────────────────────────────────────────

	// elements path
	if got := dc.convertHeading(cardObj{"level": float64(1), "elements": []interface{}{cardObj{"tag": "text", "property": cardObj{"content": "Title"}}}}); got != "# Title" {
		t.Fatalf("convertHeading(elements) = %q", got)
	}
	// level < 1 → clamped to 1
	if got := dc.convertHeading(cardObj{"level": float64(0), "content": "H"}); got != "# H" {
		t.Fatalf("convertHeading(level=0) = %q", got)
	}
	// level > 6 → clamped to 6
	if got := dc.convertHeading(cardObj{"level": float64(9), "content": "H"}); got != "###### H" {
		t.Fatalf("convertHeading(level=9) = %q", got)
	}

	// ── convertFallbackText ──────────────────────────────────────────────────

	// elements path
	if got := dc.convertFallbackText(cardObj{"elements": []interface{}{cardObj{"tag": "text", "property": cardObj{"content": "fb-elem"}}}}); got != "fb-elem" {
		t.Fatalf("convertFallbackText(elements) = %q", got)
	}
	// empty → ""
	if got := dc.convertFallbackText(cardObj{}); got != "" {
		t.Fatalf("convertFallbackText(empty) = %q", got)
	}

	// ── convertNumberTag ─────────────────────────────────────────────────────

	// no URL → return text
	if got := dc.convertNumberTag(cardObj{"text": cardObj{"content": "99"}}); got != "99" {
		t.Fatalf("convertNumberTag(no url) = %q", got)
	}
	// empty text → ""
	if got := dc.convertNumberTag(cardObj{}); got != "" {
		t.Fatalf("convertNumberTag(empty) = %q", got)
	}

	// ── convertUnknown ───────────────────────────────────────────────────────

	// detailed mode with no matching field → "[Unknown content](tag:X)"
	if got := dc.convertUnknown(cardObj{}, "exotic"); got != "[Unknown content](tag:exotic)" {
		t.Fatalf("convertUnknown(detailed, no field) = %q", got)
	}
	// concise mode → "[Unknown content]"
	if got := cc.convertUnknown(cardObj{}, "exotic"); got != "[Unknown content]" {
		t.Fatalf("convertUnknown(concise) = %q", got)
	}
	// elements fallback
	if got := dc.convertUnknown(cardObj{"elements": []interface{}{cardObj{"tag": "text", "property": cardObj{"content": "deep"}}}}, "x"); got != "deep" {
		t.Fatalf("convertUnknown(elements) = %q", got)
	}

	// ── allColumnsAreButtons ─────────────────────────────────────────────────

	if allColumnsAreButtons(nil) {
		t.Fatal("allColumnsAreButtons(nil) should be false")
	}
	// contains newline → false
	if allColumnsAreButtons([]string{"[A]\n[B]"}) {
		t.Fatal("allColumnsAreButtons(newline) should be false")
	}

	// ── convertColumn ────────────────────────────────────────────────────────

	if got := dc.convertColumn(cardObj{}, 0); got != "" {
		t.Fatalf("convertColumn(empty) = %q", got)
	}

	// ── convertRepeat ────────────────────────────────────────────────────────

	if got := dc.convertRepeat(cardObj{}); got != "" {
		t.Fatalf("convertRepeat(empty) = %q", got)
	}

	// ── convertButton ────────────────────────────────────────────────────────

	// no text → "Button"
	if got := dc.convertButton(cardObj{}, ""); got != "[Button]" {
		t.Fatalf("convertButton(no text) = %q", got)
	}
	// confirm dialog
	if got := dc.convertButton(cardObj{
		"text":    cardObj{"content": "OK"},
		"confirm": cardObj{"title": cardObj{"content": "Sure?"}, "text": cardObj{"content": "Cannot undo"}},
	}, ""); got != `[OK](confirm:"Sure?: Cannot undo")` {
		t.Fatalf("convertButton(confirm) = %q", got)
	}
	// action without URL (loop continues, no URL appended)
	if got := dc.convertButton(cardObj{
		"text":    cardObj{"content": "Do"},
		"actions": []interface{}{cardObj{"type": "other", "action": cardObj{}}},
	}, ""); got != "[Do]" {
		t.Fatalf("convertButton(action no url) = %q", got)
	}
	// non-map action skipped
	if got := dc.convertButton(cardObj{
		"text":    cardObj{"content": "Do"},
		"actions": []interface{}{"not-a-map"},
	}, ""); got != "[Do]" {
		t.Fatalf("convertButton(non-map action) = %q", got)
	}

	// ── convertActions ───────────────────────────────────────────────────────

	// empty actions → ""
	if got := dc.convertActions(cardObj{}); got != "" {
		t.Fatalf("convertActions(empty) = %q", got)
	}

	// ── convertOverflow ──────────────────────────────────────────────────────

	// empty options → ""
	if got := dc.convertOverflow(cardObj{}); got != "" {
		t.Fatalf("convertOverflow(empty) = %q", got)
	}
	// option with value (no URL)
	if got := dc.convertOverflow(cardObj{"options": []interface{}{
		cardObj{"text": cardObj{"content": "Edit"}},
		cardObj{"text": cardObj{"content": "Remove"}, "value": "rm"},
	}}); got != "⋮ Edit, Remove(rm)" {
		t.Fatalf("convertOverflow(value) = %q", got)
	}
	// option with URL via actions
	if got := dc.convertOverflow(cardObj{"options": []interface{}{
		cardObj{
			"text":    cardObj{"content": "Go"},
			"actions": []interface{}{cardObj{"type": "open_url", "action": cardObj{"url": "https://example.com"}}},
		},
	}}); got != "⋮ [Go](https://example.com)" {
		t.Fatalf("convertOverflow(url action) = %q", got)
	}
	// option with no text → skipped
	if got := dc.convertOverflow(cardObj{"options": []interface{}{cardObj{}}}); got != "⋮ " {
		t.Fatalf("convertOverflow(no-text option) = %q", got)
	}

	// ── convertSelect (non-multi paths) ─────────────────────────────────────

	// initialOption selects by value
	if got := dc.convertSelect(cardObj{
		"options":       []interface{}{cardObj{"text": cardObj{"content": "A"}, "value": "a"}, cardObj{"text": cardObj{"content": "B"}, "value": "b"}},
		"initialOption": "b",
	}, "select_static", false); got != "{A / ✓B}" {
		t.Fatalf("convertSelect(initialOption) = %q", got)
	}
	// initialIndex selects by position
	if got := dc.convertSelect(cardObj{
		"options":      []interface{}{cardObj{"text": cardObj{"content": "First"}, "value": "f"}, cardObj{"text": cardObj{"content": "Second"}, "value": "s"}},
		"initialIndex": float64(0),
	}, "select_static", false); got != "{✓First / Second}" {
		t.Fatalf("convertSelect(initialIndex) = %q", got)
	}
	// empty options → placeholder
	if got := dc.convertSelect(cardObj{
		"placeholder": cardObj{"content": "Pick one"},
	}, "sel", false); got != "{Pick one ▼}" {
		t.Fatalf("convertSelect(empty+placeholder) = %q", got)
	}
	// empty options, default placeholder
	if got := dc.convertSelect(cardObj{}, "sel", false); got != "{Please select ▼}" {
		t.Fatalf("convertSelect(default placeholder) = %q", got)
	}
	// no selected → last option gets arrow
	if got := dc.convertSelect(cardObj{
		"options": []interface{}{cardObj{"text": cardObj{"content": "X"}, "value": "x"}, cardObj{"text": cardObj{"content": "Y"}, "value": "y"}},
	}, "sel", false); got != "{X / Y ▼}" {
		t.Fatalf("convertSelect(no-selected) = %q", got)
	}
	// option with empty text + empty value → skipped
	if got := dc.convertSelect(cardObj{
		"options": []interface{}{cardObj{"value": ""}, cardObj{"text": cardObj{"content": "Valid"}, "value": "v"}},
	}, "sel", false); got != "{Valid ▼}" {
		t.Fatalf("convertSelect(skip empty option) = %q", got)
	}
	// option value used as text when no text element
	if got := dc.convertSelect(cardObj{
		"options": []interface{}{cardObj{"value": "raw-val"}},
	}, "sel", false); got != "{raw-val ▼}" {
		t.Fatalf("convertSelect(value as text) = %q", got)
	}

	// ── convertSelectImg ─────────────────────────────────────────────────────

	// imageID with only token (no origin_key)
	imgTokenOnly := &cardConverter{
		mode: cardModeDetailed,
		attachment: cardObj{
			"images": cardObj{
				"img-tok": cardObj{"token": "tok-abc"},
			},
		},
	}
	if got := imgTokenOnly.convertSelectImg(cardObj{"options": []interface{}{cardObj{"value": "v", "imageID": "img-tok"}}}, ""); !strings.Contains(got, "img_token:tok-abc") {
		t.Fatalf("convertSelectImg(token only) = %q", got)
	}

	// ── convertInput ─────────────────────────────────────────────────────────

	// defaultValue path
	if got := dc.convertInput(cardObj{"defaultValue": "prefilled"}, ""); got != "prefilled___" {
		t.Fatalf("convertInput(defaultValue) = %q", got)
	}
	// no label, no placeholder → "_____"
	if got := dc.convertInput(cardObj{}, ""); got != "_____" {
		t.Fatalf("convertInput(empty) = %q", got)
	}

	// ── convertDatePicker ────────────────────────────────────────────────────

	// time picker with value
	if got := dc.convertDatePicker(cardObj{"initialTime": "14:30"}, "", "time"); got != "🕐 14:30" {
		t.Fatalf("convertDatePicker(time) = %q", got)
	}
	// datetime picker with value
	if got := dc.convertDatePicker(cardObj{"initialDatetime": "2026-06-03T12:00:00Z"}, "", "datetime"); got != "📅 2026-06-03T12:00:00Z" {
		t.Fatalf("convertDatePicker(datetime) = %q", got)
	}
	// default picker type
	if got := dc.convertDatePicker(cardObj{}, "", "other"); !strings.HasPrefix(got, "📅 ") {
		t.Fatalf("convertDatePicker(default type) = %q", got)
	}
	// no value → placeholder
	if got := dc.convertDatePicker(cardObj{"placeholder": cardObj{"content": "Pick date"}}, "", "date"); got != "📅 Pick date" {
		t.Fatalf("convertDatePicker(placeholder) = %q", got)
	}
	// normalise ms timestamp
	if got := dc.convertDatePicker(cardObj{"initialDate": "1710500000000"}, "", "date"); !strings.HasPrefix(got, "📅 ") || got == "📅 1710500000000" {
		t.Fatalf("convertDatePicker(ms timestamp) = %q", got)
	}

	// ── convertImage ─────────────────────────────────────────────────────────

	// title overrides alt
	if got := dc.convertImage(cardObj{"alt": cardObj{"content": "Alt"}, "title": cardObj{"content": "Title"}, "imageID": "img_1"}, ""); !strings.Contains(got, "Title") {
		t.Fatalf("convertImage(title override) = %q", got)
	}
	// token-only image ID
	tokenConverter := &cardConverter{
		mode: cardModeDetailed,
		attachment: cardObj{
			"images": cardObj{"img-tok": cardObj{"token": "tok-xyz"}},
		},
	}
	if got := tokenConverter.convertImage(cardObj{"imageID": "img-tok"}, ""); !strings.Contains(got, "img_token:tok-xyz") {
		t.Fatalf("convertImage(token only) = %q", got)
	}
	// imageID not in attachment → fallback img_key:imageID
	if got := dc.convertImage(cardObj{"imageID": "unknown-img"}, ""); !strings.Contains(got, "img_key:unknown-img") {
		t.Fatalf("convertImage(no attachment entry) = %q", got)
	}

	// ── convertImgCombination ────────────────────────────────────────────────

	// empty list
	if got := dc.convertImgCombination(cardObj{}); got != "" {
		t.Fatalf("convertImgCombination(empty) = %q", got)
	}
	// token-only image
	if got := tokenConverter.convertImgCombination(cardObj{"imgList": []interface{}{cardObj{"imageID": "img-tok"}}}); !strings.Contains(got, "tok-xyz") {
		t.Fatalf("convertImgCombination(token) = %q", got)
	}
	// imageID with no attachment entry → raw imageID used as key
	if got := dc.convertImgCombination(cardObj{"imgList": []interface{}{cardObj{"imageID": "raw-id"}}}); !strings.Contains(got, "raw-id") {
		t.Fatalf("convertImgCombination(raw id) = %q", got)
	}

	// ── convertChart ─────────────────────────────────────────────────────────

	// no chartSpec title → type name becomes title
	if got := dc.convertChart(cardObj{"chartSpec": cardObj{"type": "pie"}}, ""); !strings.Contains(got, "Pie chart") {
		t.Fatalf("convertChart(no title, typed) = %q", got)
	}

	// ── extractChartSummary ──────────────────────────────────────────────────

	// array-format data (VChart series)
	if got := dc.extractChartSummary(cardObj{"chartSpec": cardObj{
		"type":   "line",
		"xField": "x",
		"yField": "y",
		"data":   []interface{}{cardObj{"id": "s1", "values": []interface{}{cardObj{"x": "A", "y": 1}}}},
	}}, "line"); got != "A:1" {
		t.Fatalf("extractChartSummary(array data) = %q", got)
	}
	// pie chart
	if got := dc.extractChartSummary(cardObj{"chartSpec": cardObj{
		"type":          "pie",
		"categoryField": "cat",
		"valueField":    "val",
		"data":          cardObj{"values": []interface{}{cardObj{"cat": "A", "val": 10}, cardObj{"cat": "B", "val": 20}}},
	}}, "pie"); got != "A:10, B:20" {
		t.Fatalf("extractChartSummary(pie) = %q", got)
	}
	// pie with missing fields → fallback count
	if got := dc.extractChartSummary(cardObj{"chartSpec": cardObj{
		"type": "pie",
		"data": cardObj{"values": []interface{}{cardObj{"cat": "X"}}},
	}}, "pie"); !strings.Contains(got, "1 data point") {
		t.Fatalf("extractChartSummary(pie missing fields) = %q", got)
	}
	// unknown chart type → count fallback
	if got := dc.extractChartSummary(cardObj{"chartSpec": cardObj{
		"type": "radar",
		"data": cardObj{"values": []interface{}{cardObj{"x": 1}, cardObj{"x": 2}}},
	}}, "radar"); !strings.Contains(got, "2 data point") {
		t.Fatalf("extractChartSummary(unknown type) = %q", got)
	}
	// line/bar with missing fields → count fallback
	if got := dc.extractChartSummary(cardObj{"chartSpec": cardObj{
		"type": "bar",
		"data": cardObj{"values": []interface{}{cardObj{"x": "a"}}},
	}}, "bar"); !strings.Contains(got, "1 data point") {
		t.Fatalf("extractChartSummary(bar missing fields) = %q", got)
	}
	// no chartSpec → ""
	if got := dc.extractChartSummary(cardObj{}, "line"); got != "" {
		t.Fatalf("extractChartSummary(no chartSpec) = %q", got)
	}
	// empty values → ""
	if got := dc.extractChartSummary(cardObj{"chartSpec": cardObj{"data": cardObj{"values": []interface{}{}}}}, "line"); got != "" {
		t.Fatalf("extractChartSummary(empty values) = %q", got)
	}

	// ── convertAudio ─────────────────────────────────────────────────────────

	// audioID fallback (fileID empty)
	if got := dc.convertAudio(cardObj{"audioID": "fake-audio-id"}, ""); got != "🎵 Audio(key:fake-audio-id)" {
		t.Fatalf("convertAudio(audioID) = %q", got)
	}
	// no ID
	if got := dc.convertAudio(cardObj{}, ""); got != "🎵 Audio" {
		t.Fatalf("convertAudio(no id) = %q", got)
	}

	// ── convertTable ─────────────────────────────────────────────────────────

	// column with no displayName → use name
	if got := dc.convertTable(cardObj{
		"columns": []interface{}{cardObj{"name": "col1"}},
		"rows":    []interface{}{cardObj{"col1": cardObj{"data": "v"}}},
	}); !strings.Contains(got, "col1") {
		t.Fatalf("convertTable(no displayName) = %q", got)
	}

	// ── extractTableCellValue ────────────────────────────────────────────────

	// float64
	if got := dc.extractTableCellValue(float64(3.14)); got != "3.14" {
		t.Fatalf("extractTableCellValue(float64) = %q", got)
	}
	// cardObj (map)
	if got := dc.extractTableCellValue(cardObj{"content": "map-val"}); got != "map-val" {
		t.Fatalf("extractTableCellValue(cardObj) = %q", got)
	}
	// unknown type → ""
	if got := dc.extractTableCellValue(true); got != "" {
		t.Fatalf("extractTableCellValue(bool) = %q", got)
	}

	// ── convertPerson ────────────────────────────────────────────────────────

	// concise mode with known person
	if got := cc.convertPerson(cardObj{"userID": "ou_person"}, ""); got != "Alice" {
		t.Fatalf("convertPerson(concise, known) = %q", got)
	}
	// notation fallback when person not in attachment
	withNotation := &cardConverter{mode: cardModeDetailed, attachment: cardObj{}}
	if got := withNotation.convertPerson(cardObj{"userID": "ou_unknown", "notation": cardObj{"content": "Unknown User"}}, ""); !strings.Contains(got, "Unknown User") {
		t.Fatalf("convertPerson(notation) = %q", got)
	}
	// no name, detailed
	noPersonAttachment := &cardConverter{mode: cardModeDetailed, attachment: cardObj{}}
	if got := noPersonAttachment.convertPerson(cardObj{"userID": "fake-uid-001"}, ""); got != "user(open_id:fake-uid-001)" {
		t.Fatalf("convertPerson(no name, detailed) = %q", got)
	}
	// no name, concise
	if got := cc.convertPerson(cardObj{"userID": "fake-uid-002"}, ""); got != "fake-uid-002" {
		t.Fatalf("convertPerson(no name, concise) = %q", got)
	}
	// empty userID
	if got := dc.convertPerson(cardObj{}, ""); got != "" {
		t.Fatalf("convertPerson(empty userID) = %q", got)
	}

	// ── convertPersonV1 ──────────────────────────────────────────────────────

	// concise mode with known person
	if got := cc.convertPersonV1(cardObj{"userID": "ou_person"}, ""); got != "Alice" {
		t.Fatalf("convertPersonV1(concise, known) = %q", got)
	}
	// no name, detailed
	if got := noPersonAttachment.convertPersonV1(cardObj{"userID": "fake-uid-003"}, ""); got != "user(open_id:fake-uid-003)" {
		t.Fatalf("convertPersonV1(no name, detailed) = %q", got)
	}
	// no name, concise
	noPersonConcise := &cardConverter{mode: cardModeConcise, attachment: cardObj{}}
	if got := noPersonConcise.convertPersonV1(cardObj{"userID": "fake-uid-004"}, ""); got != "fake-uid-004" {
		t.Fatalf("convertPersonV1(no name, concise) = %q", got)
	}
	// empty userID
	if got := dc.convertPersonV1(cardObj{}, ""); got != "" {
		t.Fatalf("convertPersonV1(empty userID) = %q", got)
	}

	// ── convertPersonList ────────────────────────────────────────────────────

	// concise mode
	if got := cc.convertPersonList(cardObj{"persons": []interface{}{cardObj{"id": "ou_person"}}}); got != "Alice" {
		t.Fatalf("convertPersonList(concise) = %q", got)
	}
	// person with no id → "user"
	if got := dc.convertPersonList(cardObj{"persons": []interface{}{cardObj{}}}); got != "user" {
		t.Fatalf("convertPersonList(no id) = %q", got)
	}
	// empty list
	if got := dc.convertPersonList(cardObj{}); got != "" {
		t.Fatalf("convertPersonList(empty) = %q", got)
	}

	// ── convertAvatar ────────────────────────────────────────────────────────

	// concise mode with known person
	if got := cc.convertAvatar(cardObj{"userID": "ou_person"}, ""); got != "👤 Alice" {
		t.Fatalf("convertAvatar(concise, known) = %q", got)
	}
	// no name, with userID
	noAvatar := &cardConverter{mode: cardModeConcise, attachment: cardObj{}}
	if got := noAvatar.convertAvatar(cardObj{"userID": "fake-uid-005"}, ""); got != "👤(id:fake-uid-005)" {
		t.Fatalf("convertAvatar(no name, with id) = %q", got)
	}
	// no name, no userID
	if got := noAvatar.convertAvatar(cardObj{}, ""); got != "👤" {
		t.Fatalf("convertAvatar(no name, no id) = %q", got)
	}

	// ── convertAt (no attachment) ────────────────────────────────────────────

	noAttachmentConverter := &cardConverter{mode: cardModeDetailed}
	if got := noAttachmentConverter.convertAt(cardObj{"userID": "fake-uid-006"}); got != "@user(open_id:fake-uid-006)" {
		t.Fatalf("convertAt(no attachment, detailed) = %q", got)
	}
	noAttachmentConcise := &cardConverter{mode: cardModeConcise}
	if got := noAttachmentConcise.convertAt(cardObj{"userID": "fake-uid-007"}); got != "@fake-uid-007" {
		t.Fatalf("convertAt(no attachment, concise) = %q", got)
	}
	// empty userID
	if got := noAttachmentConverter.convertAt(cardObj{}); got != "" {
		t.Fatalf("convertAt(empty userID) = %q", got)
	}
	// concise, no fromMentions, userName set but no actualUserID
	conciseAt := &cardConverter{
		mode: cardModeConcise,
		attachment: cardObj{
			"at_users": cardObj{"ou_nouid": cardObj{"content": "Test User"}},
		},
	}
	if got := conciseAt.convertAt(cardObj{"userID": "ou_nouid"}); got != "@Test User(ou_nouid)" {
		t.Fatalf("convertAt(concise, no actual uid) = %q", got)
	}

	// ── applyTextStyle ───────────────────────────────────────────────────────

	// strikethrough
	if got := dc.applyTextStyle("text", cardObj{"textStyle": cardObj{"attributes": []interface{}{"strikethrough"}}}); got != "~~text~~" {
		t.Fatalf("applyTextStyle(strikethrough) = %q", got)
	}

	// ── cardFormatMillisToISO8601 ─────────────────────────────────────────────

	if got := cardFormatMillisToISO8601("not-a-number"); got != "" {
		t.Fatalf("cardFormatMillisToISO8601(invalid) = %q", got)
	}

	// ── cardNormalizeTimeFormat ───────────────────────────────────────────────

	// millisecond timestamp (13 digits)
	if got := cardNormalizeTimeFormat("1710500000000"); got == "1710500000000" {
		t.Fatal("cardNormalizeTimeFormat(ms) should normalize")
	}
	// empty string
	if got := cardNormalizeTimeFormat(""); got != "" {
		t.Fatalf("cardNormalizeTimeFormat(empty) = %q", got)
	}
}

func TestCardConverterRemainingBranches(t *testing.T) {
	dc := newTestCardConverter(cardModeDetailed)
	cc := newTestCardConverter(cardModeConcise)

	// ── extractHeaderTitle (property-wrapped path) ────────────────────────────

	if got := dc.extractHeaderTitle(cardObj{"property": cardObj{"title": cardObj{"content": "Prop Title"}}}); got != "Prop Title" {
		t.Fatalf("extractHeaderTitle(property-wrapped) = %q", got)
	}

	// ── convertNote edge cases ────────────────────────────────────────────────

	// empty elements → ""
	if got := dc.convertNote(cardObj{"elements": []interface{}{}}); got != "" {
		t.Fatalf("convertNote(empty elements) = %q", got)
	}
	// non-map element skipped; element producing empty text skipped → ""
	if got := dc.convertNote(cardObj{"elements": []interface{}{"not-a-map", cardObj{"tag": "card_header"}}}); got != "" {
		t.Fatalf("convertNote(all-skip) = %q, want empty", got)
	}

	// ── convertColumnSet newline-join (non-button columns) ───────────────────

	if got := dc.convertColumnSet(cardObj{"columns": []interface{}{
		cardObj{"tag": "column", "elements": []interface{}{cardObj{"tag": "text", "property": cardObj{"content": "A"}}}},
		cardObj{"tag": "column", "elements": []interface{}{cardObj{"tag": "text", "property": cardObj{"content": "B"}}}},
	}}, 0); got != "A\n\nB" {
		t.Fatalf("convertColumnSet(non-button) = %q", got)
	}
	// non-map column skipped
	if got := dc.convertColumnSet(cardObj{"columns": []interface{}{"not-a-map"}}, 0); got != "" {
		t.Fatalf("convertColumnSet(non-map col) = %q", got)
	}

	// ── convertAt remaining branches ─────────────────────────────────────────

	// detailed, userName set but no actualUserID → @Name(open_id:origKey)
	noUID := &cardConverter{
		mode: cardModeDetailed,
		attachment: cardObj{
			"at_users": cardObj{"ou_nouid": cardObj{"content": "Test User"}},
		},
	}
	if got := noUID.convertAt(cardObj{"userID": "ou_nouid"}); got != "@Test User(open_id:ou_nouid)" {
		t.Fatalf("convertAt(detailed, no actualUID) = %q", got)
	}
	// detailed, no userName but actualUserID present → @user(user_id:X)
	noName := &cardConverter{
		mode: cardModeDetailed,
		attachment: cardObj{
			"at_users": cardObj{"ou_noname": cardObj{"user_id": "fake-uid-010"}},
		},
	}
	if got := noName.convertAt(cardObj{"userID": "ou_noname"}); got != "@user(user_id:fake-uid-010)" {
		t.Fatalf("convertAt(detailed, no name, with actualUID) = %q", got)
	}
	// concise, fromMentions=true, actualUserID set
	_ = cc // suppress unused

	// ── lookupPersonName / lookupOptionUserName / getImageKeyAndToken nil attachment ──

	nilAttach := &cardConverter{mode: cardModeDetailed}
	if got := nilAttach.lookupPersonName("any"); got != "" {
		t.Fatalf("lookupPersonName(nil attachment) = %q", got)
	}
	if got := nilAttach.lookupOptionUserName("any"); got != "" {
		t.Fatalf("lookupOptionUserName(nil attachment) = %q", got)
	}
	k, tok := nilAttach.getImageKeyAndToken("any")
	if k != "" || tok != "" {
		t.Fatalf("getImageKeyAndToken(nil attachment) = %q, %q", k, tok)
	}

	// ── goMapArrayTexts edge case (text at end without bracket) ──────────────

	// text value is the last thing in the string with no closing bracket
	if got := goMapArrayTexts("[map[text:last"); len(got) != 1 || got[0] != "last" {
		t.Fatalf("goMapArrayTexts(no closing bracket) = %v", got)
	}
	// string that doesn't look like a go map array
	if got := goMapArrayTexts("plain string"); got != nil {
		t.Fatalf("goMapArrayTexts(plain) = %v", got)
	}

	// ── convertMarkdownElements non-map element skipped ───────────────────────

	if got := dc.convertMarkdownElements([]interface{}{"not-a-map", cardObj{"tag": "text", "property": cardObj{"content": "valid"}}}); got != "valid" {
		t.Fatalf("convertMarkdownElements(non-map skip) = %q", got)
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
		{name: "person", elem: cardObj{"tag": "person", "property": cardObj{"userID": "ou_person"}}, want: "Alice(open_id:ou_person)"},
		{name: "person_v1", elem: cardObj{"tag": "person_v1", "property": cardObj{"userID": "ou_person"}}, want: "Alice(open_id:ou_person)"},
		{name: "person_list", elem: cardObj{"tag": "person_list", "property": cardObj{"persons": []interface{}{cardObj{"id": "ou_person"}}}}, want: "Alice(open_id:ou_person)"},
		{name: "avatar", elem: cardObj{"tag": "avatar", "property": cardObj{"userID": "ou_person"}}, want: "👤 Alice(open_id:ou_person)"},
		{name: "at", elem: cardObj{"tag": "at", "property": cardObj{"userID": "ou_at"}}, want: "@Bob(user_id:u_bob)"},
		{name: "at all", elem: cardObj{"tag": "at_all"}, want: "@everyone"},
		{name: "overflow", elem: cardObj{"tag": "overflow", "property": cardObj{"options": []interface{}{
			cardObj{"text": cardObj{"content": "Edit"}},
			cardObj{"text": cardObj{"content": "Delete"}, "value": "del"},
		}}}, want: "⋮ Edit, Delete(del)"},
		{name: "select_static non-multi", elem: cardObj{"tag": "select_static", "property": cardObj{
			"options":       []interface{}{cardObj{"text": cardObj{"content": "Option A"}, "value": "a"}, cardObj{"text": cardObj{"content": "Option B"}, "value": "b"}},
			"initialOption": "a",
		}}, want: "{✓Option A / Option B}"},
		{name: "multi_select_static", elem: cardObj{"tag": "multi_select_static", "property": cardObj{
			"options":        []interface{}{cardObj{"text": cardObj{"content": "X"}, "value": "x"}, cardObj{"text": cardObj{"content": "Y"}, "value": "y"}},
			"selectedValues": []interface{}{"x"},
		}}, want: "{✓X / Y}(multi)"},
		{name: "select_img", elem: cardObj{"tag": "select_img", "property": cardObj{"options": []interface{}{cardObj{"value": "v1"}}, "selectedValues": []interface{}{"v1"}}}, want: "{✓🖼️ Image 1(v1)}"},
		{name: "picker_time", elem: cardObj{"tag": "picker_time", "property": cardObj{"initialTime": "09:30"}}, want: "🕐 09:30"},
		{name: "picker_datetime", elem: cardObj{"tag": "picker_datetime", "property": cardObj{"initialDatetime": "2026-06-03T10:00:00Z"}}, want: "📅 2026-06-03T10:00:00Z"},
		{name: "img", elem: cardObj{"tag": "img", "property": cardObj{"alt": cardObj{"content": "Photo"}}}, want: "🖼️ Photo"},
		{name: "img_combination", elem: cardObj{"tag": "img_combination", "property": cardObj{"imgList": []interface{}{cardObj{"imageID": "img_1"}, cardObj{"imageID": "img_1"}}}}, want: "🖼️ 2 image(s)(keys:img_v3_test_key1,img_v3_test_key1)"},
		{name: "table", elem: cardObj{"tag": "table", "property": cardObj{"columns": []interface{}{cardObj{"displayName": "Col", "name": "col"}}, "rows": []interface{}{cardObj{"col": cardObj{"data": "val"}}}}}, want: "| Col |\n|------|\n| val |"},
		{name: "chart", elem: cardObj{"tag": "chart", "property": cardObj{"chartSpec": cardObj{"title": cardObj{"text": "Q1"}, "type": "line", "xField": "x", "yField": "y", "data": cardObj{"values": []interface{}{cardObj{"x": "Jan", "y": 5}}}}}}, want: "📊 Q1 (Line chart)\nSummary: Jan:5"},
		{name: "audio", elem: cardObj{"tag": "audio", "property": cardObj{"fileID": "fake-audio-key"}}, want: "🎵 Audio(key:fake-audio-key)"},
		{name: "video", elem: cardObj{"tag": "video", "property": cardObj{"fileID": "fake-video-key"}}, want: "🎬 Video(key:fake-video-key)"},
		{name: "collapsible_panel", elem: cardObj{"tag": "collapsible_panel", "property": cardObj{"expanded": true, "header": cardObj{"title": cardObj{"content": "More"}}, "elements": []interface{}{cardObj{"tag": "text", "property": cardObj{"content": "inside"}}}}}, want: "▼ More\n    inside\n▲"},
		{name: "form", elem: cardObj{"tag": "form", "property": cardObj{"elements": []interface{}{cardObj{"tag": "text", "property": cardObj{"content": "fill me"}}}}}, want: "<form>\nfill me\n</form>"},
		{name: "number_tag", elem: cardObj{"tag": "number_tag", "property": cardObj{"text": cardObj{"content": "7"}, "url": cardObj{"url": "https://example.com/7"}}}, want: "[7](https://example.com/7)"},
		{name: "local_datetime", elem: cardObj{"tag": "local_datetime", "property": cardObj{"milliseconds": "1710500000000"}}, contains: "202"},
		{name: "list", elem: cardObj{"tag": "list", "property": cardObj{"items": []interface{}{cardObj{"level": float64(0), "type": "ul", "elements": []interface{}{cardObj{"tag": "text", "property": cardObj{"content": "x"}}}}}}}, want: "- x"},
		{name: "blockquote", elem: cardObj{"tag": "blockquote", "property": cardObj{"content": "quoted"}}, want: "> quoted"},
		{name: "code_block", elem: cardObj{"tag": "code_block", "property": cardObj{"language": "go", "contents": []interface{}{cardObj{"contents": []interface{}{cardObj{"content": "x:=1"}}}}}}, want: "```go\nx:=1```"},
		{name: "code_span", elem: cardObj{"tag": "code_span", "property": cardObj{"content": "foo"}}, want: "`foo`"},
		{name: "heading", elem: cardObj{"tag": "heading", "property": cardObj{"level": float64(3), "content": "H3"}}, want: "### H3"},
		{name: "fallback_text", elem: cardObj{"tag": "fallback_text", "property": cardObj{"text": cardObj{"content": "fallback"}}}, want: "fallback"},
		{name: "repeat", elem: cardObj{"tag": "repeat", "property": cardObj{"elements": []interface{}{cardObj{"tag": "text", "property": cardObj{"content": "rep"}}}}}, want: "rep"},
		{name: "actions", elem: cardObj{"tag": "actions", "property": cardObj{"actions": []interface{}{
			cardObj{"tag": "button", "property": cardObj{"text": cardObj{"content": "One"}}},
			cardObj{"tag": "button", "property": cardObj{"text": cardObj{"content": "Two"}}},
		}}}, want: "[One] [Two]"},
		{name: "input", elem: cardObj{"tag": "input", "property": cardObj{"label": cardObj{"content": "Reason"}, "placeholder": cardObj{"content": "Type"}, "inputType": "multiline_text"}}, want: "Reason: Type..."},
		{name: "date", elem: cardObj{"tag": "date_picker", "property": cardObj{"initialDate": "1710500000"}}, contains: "📅 "},
		{name: "checker", elem: cardObj{"tag": "checker", "id": "chk_1", "property": cardObj{"checked": true, "text": cardObj{"content": "Done"}}}, want: "[x] Done(id:chk_1)"},
		{name: "image", elem: cardObj{"tag": "image", "property": cardObj{"alt": cardObj{"content": "Poster"}, "imageID": "img_1"}}, want: "🖼️ Poster(img_key:img_v3_test_key1)"},
		{name: "interactive", elem: cardObj{"tag": "interactive_container", "id": "cta_1", "property": cardObj{
			"actions":  []interface{}{cardObj{"type": "open_url", "action": cardObj{"url": "https://example.com"}}},
			"elements": []interface{}{cardObj{"tag": "text", "property": cardObj{"content": "Click here"}}},
		}}, want: "<clickable url=\"https://example.com\" id=\"cta_1\">\nClick here\n</clickable>"},
		{name: "text tag", elem: cardObj{"tag": "text_tag", "property": cardObj{"text": cardObj{"content": "Tag"}}}, want: "「Tag」"},
		{name: "link", elem: cardObj{"tag": "link", "property": cardObj{"content": "Spec", "url": cardObj{"url": "https://example.com"}}}, want: "[Spec](https://example.com)"},
		{name: "emoji", elem: cardObj{"tag": "emoji", "property": cardObj{"key": "OK"}}, want: "👌"},
		{name: "card header suppressed", elem: cardObj{"tag": "card_header"}, want: ""},
		{name: "custom_icon suppressed", elem: cardObj{"tag": "custom_icon"}, want: ""},
		{name: "standard_icon suppressed", elem: cardObj{"tag": "standard_icon"}, want: ""},
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
