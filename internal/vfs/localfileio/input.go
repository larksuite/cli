// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package localfileio

import "fmt"

// rejectControlChars rejects C0 control characters (except \t and \n) and
// dangerous Unicode characters from path input. Used by safePath before
// any filesystem access.
func rejectControlChars(value, flagName string) error {
	for _, r := range value {
		if r != '\t' && r != '\n' && (r < 0x20 || r == 0x7f) {
			return fmt.Errorf("%s contains invalid control characters", flagName)
		}
		if isDangerousUnicode(r) {
			return fmt.Errorf("%s contains dangerous Unicode characters", flagName)
		}
	}
	return nil
}

// isDangerousUnicode identifies Unicode code points used for visual spoofing attacks.
func isDangerousUnicode(r rune) bool {
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
