// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package recovery

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// TestCloneTypedHandlesEveryTypedError fails when errs declares a typed error
// CloneTyped does not clone.
//
// CloneTyped switches over concrete types behind errs.TypedError, an exported
// interface with no closed set of implementations, so the compiler cannot tell
// anyone that a new one was missed. The cost of missing one is silent: the
// value falls through to the default branch, callers are handed the producer's
// own error instead of a copy, and the isolation that keeps one Shutdown
// handler's edit from reaching the next is gone with nothing failing.
//
// Comparing the two sets from source is what turns that into a red build.
func TestCloneTypedHandlesEveryTypedError(t *testing.T) {
	declared := typedErrorsDeclaredInErrs(t)
	cloned := typedErrorsHandledByCloneTyped(t)

	if len(declared) == 0 {
		t.Fatal("found no typed errors in errs; the scan no longer covers its own premise")
	}

	for _, name := range declared {
		if !slices.Contains(cloned, name) {
			t.Errorf("errs.%s is a typed error but CloneTyped does not clone it: "+
				"add a case for it, or every caller silently shares the producer's value", name)
		}
	}
	for _, name := range cloned {
		if !slices.Contains(declared, name) {
			t.Errorf("CloneTyped has a case for errs.%s, which errs no longer declares", name)
		}
	}
}

// typedErrorsDeclaredInErrs returns every type in errs that carries a Problem,
// which is what makes a value satisfy errs.TypedError: Problem itself supplies
// ProblemDetail, and a struct embedding it promotes that method.
func typedErrorsDeclaredInErrs(t *testing.T) []string {
	t.Helper()
	pkg := parsePackage(t, filepath.Join("..", "..", "errs"))

	var names []string
	for _, file := range pkg {
		ast.Inspect(file, func(n ast.Node) bool {
			spec, ok := n.(*ast.TypeSpec)
			if !ok {
				return true
			}
			structType, ok := spec.Type.(*ast.StructType)
			if !ok {
				return true
			}
			if spec.Name.Name == "Problem" || embedsProblem(structType) {
				names = append(names, spec.Name.Name)
			}
			return true
		})
	}
	slices.Sort(names)
	return slices.Compact(names)
}

// embedsProblem reports whether the struct embeds Problem by value, the shape
// every typed error in errs uses to inherit ProblemDetail.
func embedsProblem(structType *ast.StructType) bool {
	for _, field := range structType.Fields.List {
		if len(field.Names) > 0 {
			continue // named field, not an embed
		}
		if ident, ok := field.Type.(*ast.Ident); ok && ident.Name == "Problem" {
			return true
		}
	}
	return false
}

// typedErrorsHandledByCloneTyped returns the errs type names CloneTyped's
// switch names in a case clause.
func typedErrorsHandledByCloneTyped(t *testing.T) []string {
	t.Helper()
	pkg := parsePackage(t, ".")

	var names []string
	for _, file := range pkg {
		ast.Inspect(file, func(n ast.Node) bool {
			decl, ok := n.(*ast.FuncDecl)
			if !ok || decl.Name.Name != "CloneTyped" {
				return true
			}
			ast.Inspect(decl.Body, func(inner ast.Node) bool {
				clause, ok := inner.(*ast.CaseClause)
				if !ok {
					return true
				}
				for _, expr := range clause.List {
					if name, ok := errsTypeName(expr); ok {
						names = append(names, name)
					}
				}
				return true
			})
			return false
		})
	}
	slices.Sort(names)
	return slices.Compact(names)
}

// errsTypeName extracts X from a `case *errs.X` clause expression.
func errsTypeName(expr ast.Expr) (string, bool) {
	star, ok := expr.(*ast.StarExpr)
	if !ok {
		return "", false
	}
	sel, ok := star.X.(*ast.SelectorExpr)
	if !ok {
		return "", false
	}
	pkgIdent, ok := sel.X.(*ast.Ident)
	if !ok || pkgIdent.Name != "errs" {
		return "", false
	}
	return sel.Sel.Name, true
}

func parsePackage(t *testing.T, dir string) []*ast.File {
	t.Helper()
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, dir, func(fi fs.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", dir, err)
	}
	var files []*ast.File
	for _, pkg := range pkgs {
		for _, file := range pkg.Files {
			files = append(files, file)
		}
	}
	return files
}
