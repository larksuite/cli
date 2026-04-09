// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package validate

import (
	"fmt"
	"strings"

	"github.com/larksuite/cli/internal/charcheck"
)

// RejectControlChars rejects C0 control characters (except \t and \n) and
// dangerous Unicode characters from user input.
//
// Delegates to charcheck.RejectControlChars — the single source of truth
// for character-level security checks.
func RejectControlChars(value, flagName string) error {
	return charcheck.RejectControlChars(value, flagName)
}

// RejectCRLF rejects strings containing carriage return (\r) or line feed (\n).
// These characters enable MIME/HTTP header injection and must never appear in
// header field names, values, Content-ID, or filename parameters.
func RejectCRLF(value, fieldName string) error {
	if strings.ContainsAny(value, "\r\n") {
		return fmt.Errorf("%s contains invalid line break characters", fieldName)
	}
	return nil
}

// StripQueryFragment removes any ?query or #fragment suffix from a URL path.
// API parameters must go through structured --params flags, not embedded in
// the path, to prevent parameter injection and behaviour confusion.
func StripQueryFragment(path string) string {
	for i := 0; i < len(path); i++ {
		if path[i] == '?' || path[i] == '#' {
			return path[:i]
		}
	}
	return path
}
