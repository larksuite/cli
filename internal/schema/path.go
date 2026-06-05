// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package schema

import "strings"

// ParsePath normalizes the positional arguments of `lark-cli schema` into a
// slice of path segments. It accepts two equivalent forms:
//
//	lark-cli schema im.messages.reply  -> single arg, split on "."
//	lark-cli schema im messages reply  -> multiple args, used as-is
//	lark-cli schema "im chat.members bots" is NOT a supported form; quote
//	arguments individually if your shell needs it. Nested resources keep their
//	internal dots (e.g. "chat.members").
//
// Returns nil for zero args (bare invocation).
func ParsePath(args []string) []string {
	switch len(args) {
	case 0:
		return nil
	case 1:
		if strings.Contains(args[0], ".") {
			return strings.Split(args[0], ".")
		}
		return []string{args[0]}
	default:
		return args
	}
}
