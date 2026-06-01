// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package errscontract

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
)

// CheckBuilderImmutable enforces builder immutability: a `With*` method on
// a typed *Error must not stash a caller-provided slice or map directly
// into a receiver field. The caller can later mutate the slice/map
// (append, delete) and silently corrupt the already-emitted typed envelope.
//
// Required shape — defensive clone:
//
//	func (e *PermissionError) WithMissingScopes(scopes ...string) *PermissionError {
//	    e.MissingScopes = slices.Clone(scopes)
//	    return e
//	}
//
// Violating shape — raw assignment:
//
//	func (e *PermissionError) WithMissingScopes(scopes ...string) *PermissionError {
//	    e.MissingScopes = scopes
//	    return e
//	}
//
// Detection strategy (AST-only, no type info):
//   - Method name starts with "With" and takes at least one parameter
//   - One parameter is a slice (`[]T`), variadic (`...T`), or map (`map[K]V`)
//   - The method body contains `e.<Field> = <paramName>` where <paramName>
//     is exactly the slice/map parameter, with no slices.Clone / maps.Clone
//     wrapper.
//
// Scope: errs/ package files (typed builders live there).
//
// Returns REJECT violations.
func CheckBuilderImmutable(path, src string) []Violation {
	if !isErrsPackagePath(path) {
		return nil
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, src, parser.ParseComments)
	if err != nil {
		return nil
	}

	var out []Violation
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv == nil || fn.Body == nil {
			continue
		}
		if fn.Name == nil || !strings.HasPrefix(fn.Name.Name, "With") {
			continue
		}
		// Only fire on methods whose receiver is a typed *Error from this package.
		recvType := receiverTypeName(fn.Recv.List[0].Type)
		if recvType == "" || !strings.HasSuffix(recvType, "Error") || recvType == "Error" {
			continue
		}
		refParams := collectReferenceTypeParams(fn.Type)
		if len(refParams) == 0 {
			continue
		}
		out = append(out, scanBuilderBody(path, fset, fn, refParams)...)
	}
	return out
}

// collectReferenceTypeParams returns the names of parameters whose type
// is a slice, variadic, or map (the reference-mutable shapes the rule
// guards). Pointer-to-slice / pointer-to-map are also considered.
func collectReferenceTypeParams(ft *ast.FuncType) map[string]struct{} {
	out := map[string]struct{}{}
	if ft.Params == nil {
		return out
	}
	for _, field := range ft.Params.List {
		if !isReferenceType(field.Type) {
			continue
		}
		for _, n := range field.Names {
			if n.Name != "" && n.Name != "_" {
				out[n.Name] = struct{}{}
			}
		}
	}
	return out
}

// isReferenceType reports whether expr names a slice, variadic, or map.
// Pointer-to-slice / map are also reference-typed for our purposes.
func isReferenceType(expr ast.Expr) bool {
	switch t := expr.(type) {
	case *ast.ArrayType:
		// nil Len → slice (`[]T`). Fixed-length arrays are value types.
		return t.Len == nil
	case *ast.MapType:
		return true
	case *ast.Ellipsis:
		return true
	case *ast.StarExpr:
		return isReferenceType(t.X)
	}
	return false
}

// scanBuilderBody walks fn.Body and emits a violation for each
// `recv.<Field> = <param>` assignment whose RHS is a bare reference-
// typed parameter (not wrapped in slices.Clone / maps.Clone).
func scanBuilderBody(path string, fset *token.FileSet, fn *ast.FuncDecl, refParams map[string]struct{}) []Violation {
	var out []Violation
	recvName := ""
	if len(fn.Recv.List[0].Names) > 0 {
		recvName = fn.Recv.List[0].Names[0].Name
	}
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok || (assign.Tok != token.ASSIGN && assign.Tok != token.DEFINE) {
			return true
		}
		if len(assign.Lhs) != 1 || len(assign.Rhs) != 1 {
			return true
		}
		// LHS must be `recv.Field`.
		sel, ok := assign.Lhs[0].(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if recvName != "" && !isIdent(sel.X, recvName) {
			return true
		}
		// RHS must be a bare reference-typed parameter ident with no
		// defensive Clone wrapper.
		paramID, ok := assign.Rhs[0].(*ast.Ident)
		if !ok {
			return true
		}
		if _, isRef := refParams[paramID.Name]; !isRef {
			return true
		}
		fieldName := ""
		if sel.Sel != nil {
			fieldName = sel.Sel.Name
		}
		out = append(out, Violation{
			Rule:    "builder_immutable",
			Action:  ActionReject,
			File:    path,
			Line:    fset.Position(assign.Pos()).Line,
			Message: fn.Name.Name + " stashes caller-owned " + paramID.Name + " into " + fieldName + " without defensive copy",
			Suggestion: "wrap the assignment with slices.Clone / maps.Clone (e.g. `" +
				sel.Sel.Name + " = slices.Clone(" + paramID.Name + ")`); raw assignment lets the caller mutate the already-emitted typed envelope",
		})
		return true
	})
	return out
}
