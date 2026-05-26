// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package i18n

// ValidLanguages defines all supported language codes.
var ValidLanguages = []string{
	"zh", "en", "ja", "ko", "fr", "de", "es", "it", "ru", "pt",
	"ar", "hi", "tr", "pl", "nl", "sv", "th", "vi", "id", "ms",
}

// IsValidLang checks if the given language code is supported.
func IsValidLang(lang string) bool {
	for _, valid := range ValidLanguages {
		if valid == lang {
			return true
		}
	}
	return false
}

// NormalizeLang normalizes language code, returning "zh" for invalid inputs.
func NormalizeLang(lang string) string {
	if IsValidLang(lang) {
		return lang
	}
	return "zh"
}
