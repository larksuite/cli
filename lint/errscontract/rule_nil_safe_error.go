// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package errscontract

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
)

// CheckNilSafeError enforces that every typed *Error struct embedding
// Problem by value defines its own pointer-receiver Error() method whose
// first statement is a nil-receiver guard returning "".
//
// Why: the embedded Problem provides Error() via promotion, but a typed-
// nil interface holder (`var e *XxxError; var err error = e`) bypasses
// the promoted method's receiver guard and panics on err.Error().
// Each typed wrapper therefore needs its own nil-safe override.
//
// Scope: errs/ package files. Unexported helper structs are skipped —
// they are not part of the public taxonomy.
//
// Returns REJECT violations.
func CheckNilSafeError(path, src string) []Violation {
	if !isErrsPackagePath(path) {
		return nil
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, src, parser.ParseComments)
	if err != nil {
		return nil
	}

	// Collect every exported struct embedding Problem-by-value.
	embedders := collectProblemEmbedders(file)
	if len(embedders) == 0 {
		return nil
	}

	// Find all Error() methods defined on those types (pointer or value receiver).
	errorMethods := collectMethodsNamed(file, "Error")

	var out []Violation
	for name, pos := range embedders {
		fn, ok := errorMethods[name]
		if !ok {
			out = append(out, Violation{
				Rule:    "nil_safe_error",
				Action:  ActionReject,
				File:    path,
				Line:    fset.Position(pos).Line,
				Message: "typed error " + name + " embeds Problem by value but defines no own Error() — typed-nil holders will panic via promoted method",
				Suggestion: "add `func (e *" + name + ") Error() string { if e == nil { return \"\" }; return e.Problem.Error() }` " +
					"so an interface holding a typed-nil pointer returns \"\" instead of panicking",
			})
			continue
		}
		if !hasNilReceiverGuard(fn) {
			out = append(out, Violation{
				Rule:       "nil_safe_error",
				Action:     ActionReject,
				File:       path,
				Line:       fset.Position(fn.Pos()).Line,
				Message:    "typed error " + name + ".Error() lacks `if e == nil { return \"\" }` nil-receiver guard",
				Suggestion: "first statement of " + name + ".Error() must be the nil-receiver guard so typed-nil holders cannot panic",
			})
		}
	}
	return out
}

// collectProblemEmbedders returns the map of exported *Error struct names
// in the file that embed Problem (by value) → declaration position.
func collectProblemEmbedders(file *ast.File) map[string]token.Pos {
	out := map[string]token.Pos{}
	ast.Inspect(file, func(n ast.Node) bool {
		ts, ok := n.(*ast.TypeSpec)
		if !ok {
			return true
		}
		st, ok := ts.Type.(*ast.StructType)
		if !ok {
			return true
		}
		name := ts.Name.Name
		if !ast.IsExported(name) || !strings.HasSuffix(name, "Error") || name == "Error" {
			return true
		}
		if !embedsProblem(st) {
			return true
		}
		out[name] = ts.Pos()
		return true
	})
	return out
}

// collectMethodsNamed returns receiver-type name → FuncDecl for every
// method whose declared name matches methodName. The receiver type may
// be either `T` or `*T`; both forms are recorded under "T".
func collectMethodsNamed(file *ast.File, methodName string) map[string]*ast.FuncDecl {
	out := map[string]*ast.FuncDecl{}
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv == nil || len(fn.Recv.List) == 0 {
			continue
		}
		if fn.Name == nil || fn.Name.Name != methodName {
			continue
		}
		recv := receiverTypeName(fn.Recv.List[0].Type)
		if recv == "" {
			continue
		}
		out[recv] = fn
	}
	return out
}

// receiverTypeName extracts T from a method receiver expression (`T` or `*T`).
func receiverTypeName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		if id, ok := t.X.(*ast.Ident); ok {
			return id.Name
		}
	}
	return ""
}

// hasNilReceiverGuard reports whether the first statement of fn is the
// canonical `if e == nil { return ... }` guard. The receiver name is read
// from fn.Recv so the check is robust to renamed receivers.
func hasNilReceiverGuard(fn *ast.FuncDecl) bool {
	if fn.Body == nil || len(fn.Body.List) == 0 {
		return false
	}
	recvName := ""
	if len(fn.Recv.List) > 0 && len(fn.Recv.List[0].Names) > 0 {
		recvName = fn.Recv.List[0].Names[0].Name
	}
	if recvName == "" {
		return false
	}
	ifs, ok := fn.Body.List[0].(*ast.IfStmt)
	if !ok {
		return false
	}
	bin, ok := ifs.Cond.(*ast.BinaryExpr)
	if !ok || bin.Op != token.EQL {
		return false
	}
	// Accept either `recv == nil` or `nil == recv`.
	if !isIdent(bin.X, recvName) || !isIdent(bin.Y, "nil") {
		if !isIdent(bin.Y, recvName) || !isIdent(bin.X, "nil") {
			return false
		}
	}
	// Body must contain a ReturnStmt (we don't require empty-string return —
	// some types return a more specific sentinel; the contract is "return
	// without dereferencing the nil receiver").
	if ifs.Body == nil {
		return false
	}
	for _, stmt := range ifs.Body.List {
		if _, ok := stmt.(*ast.ReturnStmt); ok {
			return true
		}
	}
	return false
}

func isIdent(expr ast.Expr, name string) bool {
	id, ok := expr.(*ast.Ident)
	return ok && id.Name == name
}
