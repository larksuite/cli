// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package errscontract

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
)

// CheckNoRegistrar forbids the registrar anti-pattern in service / internal
// packages (excluding internal/errclass, which legitimately owns codeMeta).
//
// Detects two registrar anti-patterns:
//  1. Direct call to mergeCodeMeta from outside internal/output
//     (mergeCodeMeta is the central registry's panic-on-dup ingress)
//  2. Calls to functions matching the (*)RegisterServiceMap(*) pattern,
//     a registrar antipattern that broke production/test parity
//     (the registered service map wouldn't fire in test binaries that
//     didn't transitively import the registering service).
func CheckNoRegistrar(path, src string) []Violation {
	if !isServiceScope(path) {
		return nil
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, src, parser.ParseComments)
	if err != nil {
		return nil
	}
	var out []Violation
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		callee := calleeName(call.Fun)
		if callee == "" {
			return true
		}
		// The registrar antipattern can hide behind middle affixes too
		// (e.g. FooRegisterServiceMapBar). strings.Contains catches all
		// shapes that the prefix/suffix pair missed.
		if callee == "mergeCodeMeta" || strings.Contains(callee, "RegisterServiceMap") {
			out = append(out, Violation{
				Rule:    "no_registrar",
				Action:  ActionReject,
				File:    path,
				Line:    fset.Position(call.Pos()).Line,
				Message: "registrar pattern forbidden: " + callee + " must not be called from service / internal code",
				Suggestion: "add CodeMeta entries in internal/errclass/codemeta_<service>.go (same-package init()); " +
					"registries fail silently when the service is not transitively imported by the test binary",
			})
		}
		return true
	})
	return out
}

// calleeName returns the function name for a call expression, supporting
// bare Ident calls (e.g. "mergeCodeMeta(...)") and SelectorExpr forms
// (e.g. "output.RegisterServiceMap(...)").
func calleeName(expr ast.Expr) string {
	switch f := expr.(type) {
	case *ast.Ident:
		return f.Name
	case *ast.SelectorExpr:
		if f.Sel != nil {
			return f.Sel.Name
		}
	}
	return ""
}

// isServiceScope reports whether a path is subject to CheckNoRegistrar. Matches paths
// under shortcuts/ or internal/ but excludes internal/errclass (which
// legitimately owns codeMeta) and test files.
func isServiceScope(path string) bool {
	if strings.HasSuffix(path, "_test.go") {
		return false
	}
	// Normalize separators for cross-platform paths.
	p := strings.ReplaceAll(path, "\\", "/")
	switch {
	case strings.HasPrefix(p, "shortcuts/") || strings.Contains(p, "/shortcuts/"):
		return true
	case strings.HasPrefix(p, "internal/errclass/") || strings.Contains(p, "/internal/errclass/"):
		return false
	case strings.HasPrefix(p, "internal/output/") || strings.Contains(p, "/internal/output/"):
		// CheckNoRegistrar carves out internal/output: it is the typed-envelope
		// writer, not a service. Without this guard
		// any legitimate registrar-shaped symbol there would trigger a
		// false-positive REJECT.
		return false
	case strings.HasPrefix(p, "internal/") || strings.Contains(p, "/internal/"):
		return true
	}
	return false
}
