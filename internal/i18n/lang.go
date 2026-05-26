// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package i18n

// ValidLanguages defines all supported language codes, aligned with the
// Feishu client UI's official language list.
var ValidLanguages = []string{
	"zh", "en", "ja", "ko", "fr", "de", "es", "it", "ru", "pt",
	"th", "vi", "id", "ms",
}

// IsValidLang checks if the given language code is supported.
// Case-sensitive: "ZH" or "Zh" return false. Callers that want to reject
// invalid input upfront should pair this with output.ErrValidation; callers
// that want a safe default in defensive read paths should compare to "en"
// and fall back to zh explicitly (see config/{bind,init}_messages.go).
func IsValidLang(lang string) bool {
	for _, valid := range ValidLanguages {
		if valid == lang {
			return true
		}
	}
	return false
}
