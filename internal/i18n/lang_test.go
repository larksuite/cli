// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package i18n

import "testing"

func TestParse(t *testing.T) {
	tests := []struct {
		in     string
		want   Lang
		wantOK bool
	}{
		{"zh", LangZhCN, true},    // short code
		{"zh_cn", LangZhCN, true}, // canonical locale
		{"en", LangEnUS, true},    // short code
		{"en_us", LangEnUS, true}, // canonical locale
		{"ja", LangJaJP, true},    // short code
		{"pt", LangPtBR, true},    // pt → pt_br, not pt_pt
		{"ms", LangMsMY, true},    // ms → ms_my
		{"", "", false},           // unset
		{"ZH", "", false},         // case-sensitive
		{"zh-CN", "", false},      // hyphen form not accepted
		{"zh_CN", "", false},      // case-sensitive region
		{"ar", "", false},         // not in the supported set
		{"xx", "", false},         // unknown
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, ok := Parse(tt.in)
			if got != tt.want || ok != tt.wantOK {
				t.Errorf("Parse(%q) = (%q, %v), want (%q, %v)", tt.in, got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

func TestIsEnglish(t *testing.T) {
	tests := []struct {
		lang Lang
		want bool
	}{
		{LangEnUS, true},
		{Lang("en"), true}, // legacy short value on disk stays robust
		{LangZhCN, false},
		{LangJaJP, false},
		{Lang("zh"), false},
		{Lang(""), false}, // unset → not English (zh bundle)
		{Lang("garbage"), false},
	}
	for _, tt := range tests {
		t.Run(string(tt.lang), func(t *testing.T) {
			if got := tt.lang.IsEnglish(); got != tt.want {
				t.Errorf("Lang(%q).IsEnglish() = %v, want %v", tt.lang, got, tt.want)
			}
		})
	}
}

func TestBase(t *testing.T) {
	tests := []struct {
		lang Lang
		want string
	}{
		{LangEnUS, "en"},
		{LangZhCN, "zh"},
		{LangJaJP, "ja"},
		{Lang("en"), "en"}, // legacy short value
		{Lang("zh"), "zh"},
		{Lang(""), ""},        // unset
		{Lang("garbage"), ""}, // unknown
	}
	for _, tt := range tests {
		t.Run(string(tt.lang), func(t *testing.T) {
			if got := tt.lang.Base(); got != tt.want {
				t.Errorf("Lang(%q).Base() = %q, want %q", tt.lang, got, tt.want)
			}
		})
	}
}

func TestCodes(t *testing.T) {
	codes := Codes()
	if len(codes) != 14 {
		t.Fatalf("len(Codes()) = %d, want 14", len(codes))
	}
	if codes[0] != "zh_cn" {
		t.Errorf("Codes()[0] = %q, want %q (catalog order)", codes[0], "zh_cn")
	}
	// Every code must round-trip through Parse to itself (canonical).
	for _, c := range codes {
		if got, ok := Parse(c); !ok || string(got) != c {
			t.Errorf("Parse(%q) = (%q, %v), want (%q, true)", c, got, ok, c)
		}
	}
}
