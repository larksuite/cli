// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package charcheck provides character-level security checks shared across
// path validation (localfileio) and input validation (validate) packages.
// Keeping these checks in one place ensures consistent detection of dangerous
// Unicode and control characters throughout the codebase.
package charcheck

import "fmt"

// RejectControlChars rejects C0 control characters (except \t and \n) and
// dangerous Unicode characters (Bidi overrides, zero-width, line/paragraph
// separators) that enable visual spoofing attacks.
func RejectControlChars(value, flagName string) error {
	for _, r := range value {
		if r != '\t' && r != '\n' && (r < 0x20 || r == 0x7f) {
			return fmt.Errorf("%s contains invalid control characters", flagName)
		}
		if IsDangerousUnicode(r) {
			return fmt.Errorf("%s contains dangerous Unicode characters", flagName)
		}
	}
	return nil
}

// IsDangerousUnicode identifies Unicode code points used for visual spoofing
// attacks. These characters are invisible or alter text direction, allowing
// attackers to make "report.exe" display as "report.txt" (Bidi override) or
// insert hidden content (zero-width characters).
func IsDangerousUnicode(r rune) bool {
	switch {
	case r >= 0x200B && r <= 0x200D: // zero-width space/non-joiner/joiner
		return true
	case r == 0xFEFF: // BOM / ZWNBSP
		return true
	case r >= 0x202A && r <= 0x202E: // Bidi: LRE/RLE/PDF/LRO/RLO
		return true
	case r >= 0x2028 && r <= 0x2029: // line/paragraph separator
		return true
	case r >= 0x2066 && r <= 0x2069: // Bidi isolates: LRI/RLI/FSI/PDI
		return true
	}
	return false
}
