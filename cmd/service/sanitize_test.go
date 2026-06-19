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

	// Truncation: a long single clause is cut to 40 runes with an ellipsis,
	// rune-safe (no split mid-character).
	long := strings.Repeat("文", 60)
	got := sanitizeOptionDesc(long)
	if r := []rune(got); len(r) != 40 || !strings.HasSuffix(got, "...") {
		t.Errorf("truncation = %q (%d runes), want 40 runes ending in ...", got, len(r))
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
