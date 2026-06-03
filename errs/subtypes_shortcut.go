// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package errs

// Subtypes raised by the typed shortcut protocol (shortcuts/common). Only
// cross-field semantic failures need their own subtype here; per-field
// failures (required missing / enum invalid / typed-primitive format) reuse
// SubtypeInvalidArgument.
const (
	SubtypeShortcutOneOfMissing    Subtype = "shortcut_oneof_missing"
	SubtypeShortcutOneOfMultiple   Subtype = "shortcut_oneof_multiple"
	SubtypeShortcutGroupIncomplete Subtype = "shortcut_group_incomplete"
)
