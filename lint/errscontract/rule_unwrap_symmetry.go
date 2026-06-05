// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package errscontract

import (
	"go/parser"
	"go/token"
)

// CheckUnwrapSymmetry enforces that every typed *Error struct embedding
// Problem defines its own nil-safe Unwrap() method, mirroring the Error()
// nil-guard contract enforced by CheckNilSafeError.
//
// Why: typed errors carry a Cause field and downstream callers traverse
// it via errors.Unwrap / errors.Is. A typed-nil holder
// (`var e *XxxError; var err error = e`) would otherwise dispatch through
// the embedded Problem.Unwrap (or panic when none exists), bypassing the
// type's own intent.
//
// Scope: errs/ package files. Unexported helper structs are skipped.
//
// Returns REJECT violations.
func CheckUnwrapSymmetry(path, src string) []Violation {
	if !isErrsPackagePath(path) {
		return nil
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, src, parser.ParseComments)
	if err != nil {
		return nil
	}

	embedders := collectProblemEmbedders(file)
	if len(embedders) == 0 {
		return nil
	}

	unwrapMethods := collectMethodsNamed(file, "Unwrap")

	var out []Violation
	for name, pos := range embedders {
		fn, ok := unwrapMethods[name]
		if !ok {
			out = append(out, Violation{
				Rule:    "unwrap_symmetry",
				Action:  ActionReject,
				File:    path,
				Line:    fset.Position(pos).Line,
				Message: "typed error " + name + " embeds Problem but defines no own Unwrap() — typed-nil holders cannot be safely traversed by errors.Unwrap",
				Suggestion: "add `func (e *" + name + ") Unwrap() error { if e == nil { return nil }; return e.Cause }` " +
					"so the typed envelope and the standard errors.Is/Unwrap traversal stay in sync",
			})
			continue
		}
		if !hasNilReceiverGuard(fn) {
			out = append(out, Violation{
				Rule:       "unwrap_symmetry",
				Action:     ActionReject,
				File:       path,
				Line:       fset.Position(fn.Pos()).Line,
				Message:    "typed error " + name + ".Unwrap() lacks `if e == nil { return nil }` nil-receiver guard",
				Suggestion: "first statement of " + name + ".Unwrap() must be the nil-receiver guard so errors.Unwrap on a typed-nil holder returns nil instead of panicking",
			})
		}
	}
	return out
}
