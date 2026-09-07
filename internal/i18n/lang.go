// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package i18n

import "strings"

// Lang is a Feishu locale (e.g. "zh_cn"); "" means unset.
type Lang string

const (
	LangZhCN Lang = "zh_cn"
	LangEnUS Lang = "en_us"
	LangJaJP Lang = "ja_jp"
	LangKoKR Lang = "ko_kr"
	LangFrFR Lang = "fr_fr"
	LangDeDE Lang = "de_de"
	LangEsES Lang = "es_es"
	LangItIT Lang = "it_it"
	LangRuRU Lang = "ru_ru"
	LangPtBR Lang = "pt_br"
	LangThTH Lang = "th_th"
	LangViVN Lang = "vi_vn"
	LangIdID Lang = "id_id"
	LangMsMY Lang = "ms_my"
)

type langEntry struct {
	Code  Lang   // canonical Feishu locale
	Short string // ISO 639-1 code, also accepted as input shorthand
}

// catalog is the single source of truth; order drives --help and error listing.
var catalog = []langEntry{
	{LangZhCN, "zh"}, {LangEnUS, "en"}, {LangJaJP, "ja"}, {LangKoKR, "ko"},
	{LangFrFR, "fr"}, {LangDeDE, "de"}, {LangEsES, "es"}, {LangItIT, "it"},
	{LangRuRU, "ru"}, {LangPtBR, "pt"}, {LangThTH, "th"}, {LangViVN, "vi"},
	{LangIdID, "id"}, {LangMsMY, "ms"},
}

// find matches a short code or Feishu locale against the catalog (case-sensitive).
func find(s string) (langEntry, bool) {
	for _, e := range catalog {
		if string(e.Code) == s || e.Short == s {
			return e, true
		}
	}
	return langEntry{}, false
}

// Parse resolves a short code or Feishu locale to its canonical Lang.
// "" and unrecognized values return ("", false).
func Parse(s string) (Lang, bool) {
	e, ok := find(s)
	return e.Code, ok
}

// UsesEnglishUI reports whether l should render the English TUI bundle.
// Only two bundles exist (zh, en): zh_cn renders Chinese, every other
// recognized locale renders English — a user who set a preference the TUI has
// no bundle for is more likely to read English than Chinese. Unset and
// unrecognized values render Chinese: they mean "no preference expressed",
// not "prefers a non-Chinese language".
func (l Lang) UsesEnglishUI() bool {
	e, ok := find(string(l))
	if !ok {
		return false
	}
	return e.Code != LangZhCN
}

// Base returns the ISO 639-1 short code ("en_us" → "en"), or "" if unknown.
func (l Lang) Base() string {
	e, _ := find(string(l))
	return e.Short
}

// CodesWithShort renders the accepted values as "zh_cn (zh), en_us (en), ...".
// Short codes are accepted input too, so a listing that hides them sends users
// who typed one to hunt for a canonical locale they did not need.
func CodesWithShort() string {
	var b strings.Builder
	for i, e := range catalog {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(string(e.Code))
		b.WriteString(" (")
		b.WriteString(e.Short)
		b.WriteString(")")
	}
	return b.String()
}
