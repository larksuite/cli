// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apicatalog

import "strings"

// ParsePath normalizes positional command arguments into the path segments
// Resolve consumes. Every argument is split on ".", so all of these name the
// same target:
//
//	im.messages.reply     -> one dotted argument
//	im messages reply     -> separate arguments
//	im messages.reply     -> a mix of the two
//
// Splitting only the single-argument form used to make the mixed one fail while
// the dotted one succeeded — and the resulting "Unknown resource" quoted back a
// string that, passed as one argument, resolved perfectly well. A resource keeps
// its internal dots either way: findResource rejoins segments longest-prefix
// first, so "chat.members" as one argument and as two both descend to the same
// resource. Returns nil for zero args (bare invocation -> TargetAll).
func ParsePath(args []string) []string {
	var out []string
	for _, arg := range args {
		if !strings.Contains(arg, ".") {
			out = append(out, arg)
			continue
		}
		out = append(out, strings.Split(arg, ".")...)
	}
	return out
}
