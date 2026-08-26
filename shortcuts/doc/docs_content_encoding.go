// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"unicode/utf8"

	"github.com/larksuite/cli/errs"
)

// validateDocsWriteContentEncoding rejects document content whose underlying
// byte sequence is not valid UTF-8 before a write reaches the API. Valid Unicode
// text is preserved verbatim; replacement characters and NULs are not reliable
// evidence of corruption because they can be intentional document content.
func validateDocsWriteContentEncoding(content string) error {
	if content == "" {
		return nil
	}
	if !utf8.ValidString(content) {
		return errs.NewValidationError(errs.SubtypeInvalidArgument,
			"--content must be valid UTF-8").
			WithParam("--content").
			WithHint("save the draft as UTF-8, run lark-cli with the draft workspace as its working directory, and pass --content @./<relative-path>; do not pipe document text through PowerShell")
	}
	return nil
}
