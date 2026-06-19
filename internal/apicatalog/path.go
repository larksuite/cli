// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apicatalog

import "strings"

// ParsePath normalizes positional command arguments into the path segments
// Resolve consumes. It accepts two equivalent forms:
//
//	im.messages.reply  -> single arg, split on "."
//	im messages reply  -> multiple args, used as-is
//
// "im chat.members bots" as a single quoted arg is NOT supported; quote
// arguments individually if your shell needs it. A resource keeps its internal
// dots when passed as one segment (e.g. "chat.members"); findResource's
// longest-prefix descent resolves both the split and the one-segment forms to
// the same target. Returns nil for zero args (bare invocation -> TargetAll).
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
