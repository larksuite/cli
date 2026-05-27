// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package errscontract

import "strings"

// RunAll executes all four checks on the given source. allowlist controls CheckDeclaredSubtype;
// pass nil to skip it. Use RunAllWithNames to enable strengthened CheckDeclaredSubtype name
// resolution.
func RunAll(path, src string, allowlist map[string]struct{}) []Violation {
	return RunAllWithNames(path, src, allowlist, nil)
}

// RunAllWithNames is RunAll with the strengthened CheckDeclaredSubtype. nameset, when
// non-nil, lets CheckDeclaredSubtype reject typo'd `errs.SubtypeBogus` selectors that
// reference no declared constant.
func RunAllWithNames(path, src string, allowlist, nameset map[string]struct{}) []Violation {
	var out []Violation
	if strings.HasPrefix(path, "errs/") || strings.Contains(path, "/errs/") {
		// CheckProblemEmbed fires on errs/ files only (caller may also enforce parity
		// across directory via CheckErrsContract).
		out = append(out, CheckProblemEmbed(path, src)...)
	}
	out = append(out, CheckNoRegistrar(path, src)...)
	out = append(out, CheckAdHocSubtype(path, src)...)
	out = append(out, CheckTypedErrorCompleteness(path, src)...)
	if allowlist != nil {
		out = append(out, CheckDeclaredSubtypeWithNames(path, src, allowlist, nameset)...)
	}
	return out
}
