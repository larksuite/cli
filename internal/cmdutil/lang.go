// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdutil

import (
	"strings"

	"github.com/larksuite/cli/internal/i18n"
	"github.com/larksuite/cli/internal/output"
)

// ParseLangFlag validates and canonicalizes a --lang value, shared by config
// and profile so every entry point honors one contract. Empty is unset (no-op);
// a non-empty value must resolve via i18n.Parse or it errors.
func ParseLangFlag(raw string) (i18n.Lang, error) {
	if raw == "" {
		return "", nil
	}
	lang, ok := i18n.Parse(raw)
	if !ok {
		return "", output.ErrValidation(
			"invalid --lang %q; valid values: %s",
			raw, strings.Join(i18n.Codes(), ", "))
	}
	return lang, nil
}
