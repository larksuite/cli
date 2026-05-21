// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package errscontract

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
)

// CheckProblemEmbed enforces the errs/ typed-error contract on a single
// source file: every exported struct whose name ends in "Error" must embed the
// package-local Problem (or errs.Problem when imported).
//
// Predicate + test-coverage parity are checked at the directory level by
// CheckErrsContract; this AST-only entry is the unit-testable core.
func CheckProblemEmbed(path, src string) []Violation {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, src, parser.ParseComments)
	if err != nil {
		return nil
	}
	var out []Violation
	ast.Inspect(file, func(n ast.Node) bool {
		typeSpec, ok := n.(*ast.TypeSpec)
		if !ok {
			return true
		}
		structType, ok := typeSpec.Type.(*ast.StructType)
		if !ok {
			return true
		}
		name := typeSpec.Name.Name
		// Only enforce CheckProblemEmbed on EXPORTED *Error types — unexported helper
		// structs that happen to end in "Error" are internal scratch types,
		// not part of the typed taxonomy.
		if !ast.IsExported(name) || !strings.HasSuffix(name, "Error") {
			return true
		}
		if !embedsProblem(structType) {
			out = append(out, Violation{
				Rule:       "problem_embed",
				Action:     ActionReject,
				File:       path,
				Line:       fset.Position(typeSpec.Pos()).Line,
				Message:    "typed error " + name + " must embed errs.Problem (RFC 7807-aligned canonical shape)",
				Suggestion: "add `errs.Problem` (or `Problem` if in errs package) as the first embedded field",
			})
		}
		return true
	})
	return out
}

// embedsProblem reports whether the struct embeds the canonical Problem type
// (bare `Problem` when defined in errs, or `errs.Problem` when imported).
func embedsProblem(s *ast.StructType) bool {
	for _, f := range s.Fields.List {
		if len(f.Names) != 0 {
			continue // not embedded
		}
		switch t := f.Type.(type) {
		case *ast.Ident:
			if t.Name == "Problem" {
				return true
			}
		case *ast.SelectorExpr:
			if x, ok := t.X.(*ast.Ident); ok && x.Name == "errs" && t.Sel != nil && t.Sel.Name == "Problem" {
				return true
			}
		}
	}
	return false
}
