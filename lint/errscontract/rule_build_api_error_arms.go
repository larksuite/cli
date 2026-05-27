// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package errscontract

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
)

// canonicalCategories enumerates every taxonomy Category that BuildAPIError
// must route. Kept in sync with errs/category.go. The lint refuses to
// silently accept the omission of a new Category — when the taxonomy grows,
// either BuildAPIError gets an explicit arm or this list is updated
// consciously (drawing a reviewer's attention).
var canonicalCategories = []string{
	"CategoryValidation",
	"CategoryAuthentication",
	"CategoryAuthorization",
	"CategoryConfig",
	"CategoryNetwork",
	"CategoryAPI",
	"CategoryPolicy",
	"CategoryInternal",
	"CategoryConfirmation",
}

// CheckBuildAPIErrorArms enforces that the BuildAPIError switch in
// internal/errclass/classify.go (a) covers every Category in the canonical
// taxonomy and (b) has a `default` arm that fail-closes to an InternalError
// envelope — never returns nil and never falls through to emit an empty
// Problem on the wire.
//
// Scope: only the canonical classify.go file. Other switch statements on
// Category in callers (e.g. UI rendering) intentionally remain free-form.
//
// Returns REJECT violations.
func CheckBuildAPIErrorArms(path, src string) []Violation {
	if !isClassifyPath(path) {
		return nil
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, src, parser.ParseComments)
	if err != nil {
		return nil
	}

	var out []Violation
	found := false
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name == nil || fn.Name.Name != "BuildAPIError" || fn.Body == nil {
			continue
		}
		found = true
		sw := findCategorySwitch(fn.Body)
		if sw == nil {
			out = append(out, Violation{
				Rule:    "build_api_error_arms",
				Action:  ActionReject,
				File:    path,
				Line:    fset.Position(fn.Pos()).Line,
				Message: "BuildAPIError has no Category switch — typed routing is the entire purpose of this function",
				Suggestion: "restore the `switch meta.Category { case errs.CategoryX: ...; default: <fail-closed InternalError> }` " +
					"structure",
			})
			continue
		}
		out = append(out, checkSwitchArms(path, fset, sw)...)
	}
	if !found {
		out = append(out, Violation{
			Rule:       "build_api_error_arms",
			Action:     ActionReject,
			File:       path,
			Line:       1,
			Message:    "BuildAPIError function not found in classify.go — the typed-routing entry point must exist on this file",
			Suggestion: "define `func BuildAPIError(resp map[string]any, cc ClassifyContext) error` with the canonical Category switch",
		})
	}
	return out
}

// isClassifyPath matches both repo-relative ("internal/errclass/classify.go")
// and slashed paths that contain the same suffix when scanning nested roots.
func isClassifyPath(path string) bool {
	p := strings.ReplaceAll(path, "\\", "/")
	return p == "internal/errclass/classify.go" || strings.HasSuffix(p, "/internal/errclass/classify.go")
}

// findCategorySwitch returns the first switch statement inside body whose
// arms reference `errs.Category*` selectors. Returns nil if no such switch
// exists. The shallow scan is sufficient — BuildAPIError contains exactly
// one taxonomy switch in production.
func findCategorySwitch(body *ast.BlockStmt) *ast.SwitchStmt {
	var found *ast.SwitchStmt
	ast.Inspect(body, func(n ast.Node) bool {
		if found != nil {
			return false
		}
		sw, ok := n.(*ast.SwitchStmt)
		if !ok || sw.Body == nil {
			return true
		}
		if switchMentionsCategory(sw) {
			found = sw
			return false
		}
		return true
	})
	return found
}

// switchMentionsCategory reports whether sw has at least one arm with an
// `errs.Category*` case expression. This is the cheap heuristic that
// identifies the canonical taxonomy switch without depending on type info.
func switchMentionsCategory(sw *ast.SwitchStmt) bool {
	for _, stmt := range sw.Body.List {
		cc, ok := stmt.(*ast.CaseClause)
		if !ok {
			continue
		}
		for _, expr := range cc.List {
			if categoryName(expr) != "" {
				return true
			}
		}
	}
	return false
}

// categoryName returns the `Category*` selector name (e.g. "CategoryAPI")
// for an `errs.Category*` expression, or "" when expr is not such a
// selector. Also accepts a bare `Category*` ident for an in-package
// switch (rare but possible).
func categoryName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.SelectorExpr:
		if x, ok := t.X.(*ast.Ident); ok && x.Name == "errs" && t.Sel != nil &&
			strings.HasPrefix(t.Sel.Name, "Category") {
			return t.Sel.Name
		}
	case *ast.Ident:
		if strings.HasPrefix(t.Name, "Category") {
			return t.Name
		}
	}
	return ""
}

// checkSwitchArms validates two invariants against the located switch:
//
//  1. Every Category in canonicalCategories appears as a case expression.
//  2. The switch has a default arm whose body returns a non-nil expression
//     (i.e. fails closed to InternalError).
func checkSwitchArms(path string, fset *token.FileSet, sw *ast.SwitchStmt) []Violation {
	covered := map[string]struct{}{}
	var defaultArm *ast.CaseClause
	for _, stmt := range sw.Body.List {
		cc, ok := stmt.(*ast.CaseClause)
		if !ok {
			continue
		}
		if cc.List == nil {
			defaultArm = cc
			continue
		}
		for _, expr := range cc.List {
			if name := categoryName(expr); name != "" {
				covered[name] = struct{}{}
			}
		}
	}

	var out []Violation
	for _, cat := range canonicalCategories {
		if _, ok := covered[cat]; ok {
			continue
		}
		out = append(out, Violation{
			Rule:    "build_api_error_arms",
			Action:  ActionReject,
			File:    path,
			Line:    fset.Position(sw.Pos()).Line,
			Message: "BuildAPIError switch is missing explicit arm for errs." + cat,
			Suggestion: "add a case clause for errs." + cat + " that routes to the matching typed *Error; " +
				"the canonical taxonomy is fixed in errs/category.go and every Category must be handled",
		})
	}

	if defaultArm == nil {
		out = append(out, Violation{
			Rule:    "build_api_error_arms",
			Action:  ActionReject,
			File:    path,
			Line:    fset.Position(sw.Pos()).Line,
			Message: "BuildAPIError switch has no default arm — unknown Category would fall through and emit an empty Problem",
			Suggestion: "add `default:` that fail-closes to `&errs.InternalError{Problem: ...SubtypeSDKError...}` " +
				"so unrecognised Category values cannot produce a wire-invalid envelope",
		})
	} else if !defaultReturnsInternalError(defaultArm) {
		out = append(out, Violation{
			Rule:    "build_api_error_arms",
			Action:  ActionReject,
			File:    path,
			Line:    fset.Position(defaultArm.Pos()).Line,
			Message: "BuildAPIError default arm returns nil or has no return — must fail closed to InternalError",
			Suggestion: "return `&errs.InternalError{Problem: errs.Problem{Category: errs.CategoryInternal, Subtype: errs.SubtypeSDKError, ...}}` " +
				"so an unrecognised Category never silently drops the failure",
		})
	}
	return out
}

// defaultReturnsInternalError checks the default arm's body has a return
// statement whose first result is *errs.InternalError — either constructed
// via `&errs.InternalError{...}` composite literal or `errs.NewInternalError(...)`
// constructor. Accepts both selector form (`errs.InternalError`) and bare
// identifier (`InternalError`) so unit-test fixtures with a local stub
// package match. The BuildAPIError default arm MUST fail closed to
// InternalError; other typed errors (APIError, etc.) silently drop the
// "unknown Category" signal and are rejected by this rule.
func defaultReturnsInternalError(cc *ast.CaseClause) bool {
	for _, stmt := range cc.Body {
		ret, ok := stmt.(*ast.ReturnStmt)
		if !ok || len(ret.Results) == 0 {
			continue
		}
		if isInternalErrorReturn(ret.Results[0]) {
			return true
		}
	}
	return false
}

func isInternalErrorReturn(expr ast.Expr) bool {
	switch e := expr.(type) {
	case *ast.UnaryExpr:
		// &errs.InternalError{...} or &InternalError{...}
		if e.Op != token.AND {
			return false
		}
		if cl, ok := e.X.(*ast.CompositeLit); ok {
			return isInternalErrorType(cl.Type)
		}
	case *ast.CompositeLit:
		// errs.InternalError{...} or InternalError{...} (value, rare for errors)
		return isInternalErrorType(e.Type)
	case *ast.CallExpr:
		// errs.NewInternalError(...) or NewInternalError(...)
		return isNewInternalErrorCall(e.Fun)
	}
	return false
}

func isInternalErrorType(t ast.Expr) bool {
	switch x := t.(type) {
	case *ast.SelectorExpr:
		return x.Sel.Name == "InternalError"
	case *ast.Ident:
		return x.Name == "InternalError"
	}
	return false
}

func isNewInternalErrorCall(fn ast.Expr) bool {
	switch x := fn.(type) {
	case *ast.SelectorExpr:
		return x.Sel.Name == "NewInternalError"
	case *ast.Ident:
		return x.Name == "NewInternalError"
	}
	return false
}
