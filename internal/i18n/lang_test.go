// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package i18n

import (
	"strings"
	"testing"
)

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

func TestCatalog(t *testing.T) {
	if len(catalog) != 14 {
		t.Fatalf("len(catalog) = %d, want 14", len(catalog))
	}
	if catalog[0].Code != LangZhCN {
		t.Errorf("catalog[0].Code = %q, want %q", catalog[0].Code, LangZhCN)
	}
	// Every code must round-trip through Parse to itself (canonical).
	for _, e := range catalog {
		c := string(e.Code)
		if got, ok := Parse(c); !ok || string(got) != c {
			t.Errorf("Parse(%q) = (%q, %v), want (%q, true)", c, got, ok, c)
		}
	}
}

func TestCodesWithShort(t *testing.T) {
	got := CodesWithShort()
	if want := "zh_cn (zh), en_us (en), ja_jp (ja)"; !strings.HasPrefix(got, want) {
		t.Errorf("CodesWithShort() = %q, want prefix %q (catalog order)", got, want)
	}
	// Every catalog entry must be listed with both spellings a user may type,
	// so the listing never sends someone hunting for a value it omitted.
	for _, e := range catalog {
		entry := string(e.Code) + " (" + e.Short + ")"
		if !strings.Contains(got, entry) {
			t.Errorf("CodesWithShort() = %q, missing %q", got, entry)
		}
	}
	if n := strings.Count(got, ", "); n != len(catalog)-1 {
		t.Errorf("CodesWithShort() has %d separators, want %d", n, len(catalog)-1)
	}
}

// wantEnglishBundle pins, by hand, which TUI bundle each canonical locale
// renders in. Written out rather than derived, so it disagrees with the
// implementation when the implementation is wrong.
var wantEnglishBundle = map[Lang]bool{
	LangZhCN: false,
	LangEnUS: true,
	LangJaJP: true,
	LangKoKR: true,
	LangFrFR: true,
	LangDeDE: true,
	LangEsES: true,
	LangItIT: true,
	LangRuRU: true,
	LangPtBR: true,
	LangThTH: true,
	LangViVN: true,
	LangIdID: true,
	LangMsMY: true,
}

func TestUsesEnglishUI(t *testing.T) {
	for lang, want := range wantEnglishBundle {
		if got := lang.UsesEnglishUI(); got != want {
			t.Errorf("UsesEnglishUI(%q) = %v, want %v", lang, got, want)
		}
	}

	// Short codes resolve to their canonical locale's bundle.
	for short, want := range map[Lang]bool{"zh": false, "en": true, "ja": true} {
		if got := short.UsesEnglishUI(); got != want {
			t.Errorf("UsesEnglishUI(%q) = %v, want %v", short, got, want)
		}
	}

	// Anything expressing no usable preference renders Chinese: it means "no
	// preference stated", not "prefers a non-Chinese language".
	for _, l := range []Lang{
		"",        // unset
		"unknown", // not in the catalog
		"ZH",      // wrong case: find() is case-sensitive
		"en_US",   // wrong case for a real locale
	} {
		if l.UsesEnglishUI() {
			t.Errorf("UsesEnglishUI(%q) = true, want false", l)
		}
	}
}

func TestUsesEnglishUI_CoversEveryCatalogEntry(t *testing.T) {
	// A locale added to the catalog without a hand-written entry above has had
	// no one decide which bundle it renders in.
	for _, e := range catalog {
		if _, ok := wantEnglishBundle[e.Code]; !ok {
			t.Errorf("catalog has %q but wantEnglishBundle does not: "+
				"decide which TUI bundle it renders in", e.Code)
		}
	}
	for lang := range wantEnglishBundle {
		if _, ok := find(string(lang)); !ok {
			t.Errorf("wantEnglishBundle has %q but the catalog does not", lang)
		}
	}
}
