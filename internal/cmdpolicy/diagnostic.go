// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdpolicy

// diagnosticPaths lists command paths that are unconditionally allowed,
// regardless of any user-layer Rule. Entries must satisfy two properties:
//
//  1. Read-only. The command performs no I/O outside the local process
//     and never mutates remote state.
//  2. Self-reflective. Denying the command would produce a UX dead-end
//     where the operator can no longer inspect / validate the policy
//     that is locking them out.
//
// Today this is `config policy show` and `config plugins show` --
// both purely local introspection over the resolved policy. Keep the
// list small and audited: every entry is a permanent hole in the
// fail-closed boundary.
var diagnosticPaths = map[string]bool{
	"config/policy/show":  true,
	"config/plugins/show": true,
}

// IsDiagnosticPath reports whether the given canonical command path is
// exempt from user-layer pruning. Exported for test packages; callers
// inside this package use the unexported helper.
func IsDiagnosticPath(path string) bool {
	return diagnosticPaths[path]
}

// DiagnosticPaths returns the exempt self-inspection command paths for a
// build-local presentation projection. Enforcement always leaves these
// operator escape hatches available.
func DiagnosticPaths() []string {
	out := make([]string, 0, len(diagnosticPaths))
	for p := range diagnosticPaths {
		out = append(out, p)
	}
	return out
}
