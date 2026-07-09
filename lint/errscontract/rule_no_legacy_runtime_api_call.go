// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package errscontract

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
)

// CheckNoLegacyRuntimeAPICall flags calls to the runtime's auto-classifying
// API helpers (CallAPI / DoAPIJSON / DoAPIJSONWithLogID). Those helpers
// classify a response without the running command's identity context, so a
// Lark authorization failure cannot carry MissingScopes / ConsoleURL /
// Identity. Code must call the domain's typed wrapper or runtime.DoAPIJSONTyped
// / runtime.DoAPI + errclass.BuildAPIError so failures classify into
// fully-populated typed errs.* errors. forbidigo cannot see these because they
// are method calls, not output.Err* identifiers — this AST rule covers that gap
// repo-wide.
//
// Applies to every .go path; skips _test.go fixtures. A typed wrapper like
// driveCallAPI is an unqualified call (*ast.Ident), not a selector, so it is not
// matched. runtime.DoAPI / runtime.RawAPI are intentionally not listed: they
// return the raw response for the caller to classify and do not emit a legacy
// envelope themselves.
//
// Files that do not import shortcuts/common are skipped: the legacy helpers
// are methods on common.RuntimeContext, so a same-named method on another
// receiver (for example the event domain's APIClient interface, whose
// implementation classifies into typed errs.* errors) is not a legacy call.
func CheckNoLegacyRuntimeAPICall(path, src string) []Violation {
	if strings.HasSuffix(path, "_test.go") {
		return nil
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, src, parser.ParseComments)
	if err != nil {
		return nil
	}
	if !importsPath(file, commonImportPath) {
		return nil
	}
	var out []Violation
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel == nil {
			return true
		}
		if name, ok := matchLegacyRuntimeAPIMethod(sel.Sel.Name); ok {
			out = append(out, Violation{
				Rule:    "no_legacy_runtime_api_call",
				Action:  ActionReject,
				File:    path,
				Line:    fset.Position(call.Pos()).Line,
				Message: "runtime." + name + " classifies the response without the command's identity context (no MissingScopes/ConsoleURL/Identity on authorization failures); it is forbidden",
				Suggestion: "call the domain's typed API wrapper (for example driveCallAPI or callTaskAPITyped) or runtime.DoAPI + errclass.BuildAPIError " +
					"so failures classify into typed errs.* errors",
			})
		}
		return true
	})
	return out
}

// matchLegacyRuntimeAPIMethod returns the name when it is one of the runtime's
// legacy auto-classifying API helper methods.
func matchLegacyRuntimeAPIMethod(name string) (string, bool) {
	switch name {
	case "CallAPI", "DoAPIJSON", "DoAPIJSONWithLogID":
		return name, true
	}
	return "", false
}

// importsPath reports whether the file imports the given package path.
func importsPath(file *ast.File, importPath string) bool {
	for _, imp := range file.Imports {
		if imp.Path == nil {
			continue
		}
		if strings.Trim(imp.Path.Value, "`\"") == importPath {
			return true
		}
	}
	return false
}
