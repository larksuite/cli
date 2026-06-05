// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package i18n

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

// IsEnglish reports whether l uses the English TUI bundle (robust to "en_us"
// and legacy "en").
func (l Lang) IsEnglish() bool {
	e, _ := find(string(l))
	return e.Code == LangEnUS
}

// Base returns the ISO 639-1 short code ("en_us" → "en"), or "" if unknown.
func (l Lang) Base() string {
	e, _ := find(string(l))
	return e.Short
}

// Codes lists the canonical locales, for --help and error messages.
func Codes() []string {
	out := make([]string, len(catalog))
	for i, e := range catalog {
		out[i] = string(e.Code)
	}
	return out
}
