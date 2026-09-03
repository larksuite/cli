// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package service

import (
	"strings"
	"testing"
)

func TestSanitizeOptionDesc(t *testing.T) {
	cases := map[string]string{
		"":                         "",
		"以 open_id 标识用户":           "以 open_id 标识用户",
		"中文。English second clause": "中文",         // first clause only (。)
		"head；tail":                "head",       // first clause (；)
		"line one\nline two":       "line one",   // first clause (newline)
		"  spaced   out  ":         "spaced out", // whitespace collapsed
		"see [飞书后台](https://x/admin) 详情": "see 飞书后台 详情", // markdown link -> text, url dropped
	}
	for in, want := range cases {
		if got := sanitizeOptionDesc(in); got != want {
			t.Errorf("sanitizeOptionDesc(%q) = %q, want %q", in, got, want)
		}
	}

	// Truncation: a long single clause is fitted to the 80-cell option budget
	// (40 wide runes) with an ellipsis, rune-safe (no split mid-character).
	long := strings.Repeat("文", 60)
	got := sanitizeOptionDesc(long)
	if w := displayWidth(got); w > optionDescBudget || !strings.HasSuffix(got, clauseEllipsis) {
		t.Errorf("truncation = %q (%d cells), want <= %d cells ending in %s", got, w, optionDescBudget, clauseEllipsis)
	}
	if strings.Contains(got, strings.Repeat("文", 40)) {
		t.Errorf("truncation kept more than the budget allows: %q", got)
	}
}

// The field budget is 120 cells: 60 wide runes, so every Chinese clause that
// fitted the old 60-rune cap renders byte-for-byte as before, and English gets
// the room the same meaning needs instead of a mid-word "..." at 60 letters.
func TestSanitizeFieldDesc_CellBudgetKeepsChineseAndFitsEnglish(t *testing.T) {
	zh60 := strings.Repeat("邮", 60)
	if got := sanitizeFieldDesc(zh60); got != zh60 {
		t.Errorf("60 wide runes must fit unchanged, got %q", got)
	}
	en := "User mailbox email address, used as the user mailbox identity. You can obtain the address from the profile API"
	if got := sanitizeFieldDesc(en); got != en {
		t.Errorf("a 115-letter English clause fits the 120-cell budget, got %q", got)
	}
	zh61 := strings.Repeat("邮", 61)
	got := sanitizeFieldDesc(zh61)
	if displayWidth(got) > fieldDescBudget || !strings.HasSuffix(got, clauseEllipsis) {
		t.Errorf("61 wide runes must be fitted with an ellipsis within %d cells, got %q", fieldDescBudget, got)
	}
}

// A shortened clause stops at a unit of meaning: the last sentence end in the
// back 60% of the budget, else the last word boundary in the back half. It
// never stops mid-word, drops the punctuation the cut strands, and ends in one
// ellipsis rune so the reader knows `lark-cli schema` has the rest.
func TestFitClause_SentenceThenWordBoundary(t *testing.T) {
	sentence := "User mailbox email address, used as the user mailbox identity. " +
		"You can obtain the primary mailbox address from the Get user mailbox profile API. " +
		"When calling this API with a user_access_token, you can use the placeholder me"
	// The second sentence ends at cell 146, past the budget, so the cut lands on
	// the first sentence end (cell 63, inside the back 60%) — never mid-word.
	got := fitClause(sentence, fieldDescBudget)
	want := "User mailbox email address, used as the user mailbox identity" + clauseEllipsis
	if got != want {
		t.Errorf("sentence cut\n got %q\nwant %q", got, want)
	}
	if displayWidth(got) > fieldDescBudget {
		t.Errorf("fitted clause exceeds budget: %d cells", displayWidth(got))
	}

	words := strings.Repeat("alphabet ", 20) // no sentence end anywhere
	got = fitClause(words, fieldDescBudget)
	if !strings.HasSuffix(got, "alphabet"+clauseEllipsis) || displayWidth(got) > fieldDescBudget {
		t.Errorf("word cut must end on a whole word within budget, got %q (%d cells)", got, displayWidth(got))
	}

	// An early sentence end (front 40%) does not win over a later word boundary:
	// keeping a longer, still-complete prefix says more.
	early := "Page size. " + strings.Repeat("the limit is between one and one hundred ", 4)
	got = fitClause(early, fieldDescBudget)
	if got == "Page size"+clauseEllipsis {
		t.Errorf("a sentence end in the front 40%% must not shorten the clause to %q", got)
	}
	if strings.Contains(got, "on"+clauseEllipsis) && !strings.Contains(got, "one"+clauseEllipsis) {
		t.Errorf("cut landed mid-word: %q", got)
	}

	// A wide-rune clause with no spaces falls back to Chinese comma/enumeration
	// marks, and the stranded comma is dropped.
	zh := strings.Repeat("邮", 45) + "，" + strings.Repeat("箱", 30)
	got = fitClause(zh, fieldDescBudget)
	if got != strings.Repeat("邮", 45)+clauseEllipsis {
		t.Errorf("Chinese comma boundary\n got %q", got)
	}

	// Decimal points are not sentence ends.
	decimal := strings.Repeat("wait 1.5 seconds then retry ", 6)
	got = fitClause(decimal, fieldDescBudget)
	if strings.HasSuffix(got, "1"+clauseEllipsis) || strings.HasSuffix(got, "1."+clauseEllipsis) {
		t.Errorf("cut inside a decimal number: %q", got)
	}
}

// The doc-reference breadcrumb is dropped in both languages, and only when a
// clause separator precedes it so a subject is never orphaned.
func TestCutDocRef_BothLanguages(t *testing.T) {
	cases := map[string]string{
		"Calendar ID. For details, see [Calendar-related IDs](https://x/y)":         "Calendar ID",
		"Calendar ID. For more information, refer to [the guide](https://x/y)":      "Calendar ID",
		"Owner ID. Please refer to [Get user ID](https://x/y) for how to obtain it": "Owner ID",
		"日程对应的日历 ID。了解更多，参见[日历 ID 说明](https://x/y)。":                                "日程对应的日历 ID",
		"待查询的消息ID。ID 获取方式：\n- 调用接口获取":                                               "待查询的消息ID。ID 获取方式",
		"Please refer to the docs": "Please refer to the docs",
	}
	for in, want := range cases {
		if got := sanitizeFieldDesc(in); got != want {
			t.Errorf("sanitizeFieldDesc(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSanitizeFieldDesc_TrimsDanglingPunctuation(t *testing.T) {
	// A clause cut can strand a connector (e.g. a colon introducing a list the
	// newline cut drops, as in im.reactions.list's message_id); the help line
	// joiner then renders "…获取方式：." — so dangling punctuation must go too.
	cases := map[string]string{
		"待查询的消息ID。ID 获取方式：\n- 调用接口获取": "待查询的消息ID。ID 获取方式",
		"see the list below:\nitem":   "see the list below",
		"逗号结尾，\n下一行":                  "逗号结尾",
	}
	for in, want := range cases {
		if got := sanitizeFieldDesc(in); got != want {
			t.Errorf("sanitizeFieldDesc(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSanitizeFieldDesc_StripsBackquotes(t *testing.T) {
	// pflag's UnquoteUsage takes a backquoted word in a flag's usage string as
	// the flag's metavar: wiki space_id's description rendered the flag as
	// "--space-id my_library" instead of "--space-id string".
	in := "[知识空间id](https://x/wiki)，如果查询我的文档库可替换为`my_library`"
	want := "知识空间id，如果查询我的文档库可替换为my_library"
	if got := sanitizeFieldDesc(in); got != want {
		t.Errorf("sanitizeFieldDesc(%q) = %q, want %q", in, got, want)
	}
}
