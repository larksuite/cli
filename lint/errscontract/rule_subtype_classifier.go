// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package errscontract

import (
	"go/ast"
	"go/token"
	"regexp"
	"strings"
)

// classifySubtypeExpr inspects a single expression sitting in a `Subtype:`
// slot and returns the lint verdict. Used by scanSubtype to drive both
// CheckAdHocSubtype (ad_hoc_*) and CheckDeclaredSubtype (declared / undeclared / dynamic) signals.
func classifySubtypeExpr(expr ast.Expr, allowlist, nameset map[string]struct{}, adHoc *regexp.Regexp, scope *TypedScope, absPath string) subtypeClassification {
	return subtypeExprClassifier{
		allowlist: allowlist,
		nameset:   nameset,
		adHoc:     adHoc,
		scope:     scope,
		absPath:   absPath,
	}.classify(expr)
}

// subtypeExprClassifier is the strategy object for classifying a single
// expression assigned to a Subtype slot. The public-ish wrapper above keeps the
// scanSubtype callsite simple, while these methods keep each AST expression
// shape isolated enough to change independently.
type subtypeExprClassifier struct {
	allowlist map[string]struct{}
	nameset   map[string]struct{}
	adHoc     *regexp.Regexp
	scope     *TypedScope
	absPath   string
}

func (c subtypeExprClassifier) classify(expr ast.Expr) subtypeClassification {
	switch v := expr.(type) {
	case *ast.SelectorExpr:
		return c.classifySelector(v)
	case *ast.Ident:
		return c.classifyIdent(v)
	case *ast.BasicLit:
		return c.classifyLiteral(v)
	case *ast.CallExpr:
		return c.classifyCall(v)
	}
	return subtypeClassification{}
}

func (c subtypeExprClassifier) classifySelector(sel *ast.SelectorExpr) subtypeClassification {
	if sel == nil || sel.Sel == nil {
		return subtypeClassification{}
	}
	// Typed-first: route every selector through type resolution, regardless
	// of naming. This catches `foreign.MyKind` assigned to a Subtype slot,
	// which the AST fallback intentionally cannot prove.
	if result, handled := classifyConstViaTypes(sel.Sel, c.absPath, c.scope); handled {
		return result
	}
	// AST fallback: only Subtype-prefixed selector names are treated as
	// constant references. Bare `Subtype` is usually a struct-field selector
	// such as `meta.Subtype`, not a constant.
	if !isSubtypeConstName(sel.Sel.Name) {
		return subtypeClassification{}
	}
	if !c.declaredName(sel.Sel.Name) {
		return undeclaredSubtypeConst("selector", sel.Sel.Name)
	}
	return subtypeClassification{}
}

func (c subtypeExprClassifier) classifyIdent(id *ast.Ident) subtypeClassification {
	if id == nil {
		return subtypeClassification{}
	}
	// Typed-first: every identifier in a Subtype slot is type-resolved when
	// scope is available, regardless of its name.
	if result, handled := classifyConstViaTypes(id, c.absPath, c.scope); handled {
		return result
	}
	// AST fallback: in-package const form `SubtypeMissingScope`. The bare
	// `Subtype` identifier is the type name, not a constant reference.
	if isSubtypeConstName(id.Name) {
		if !c.declaredName(id.Name) {
			return undeclaredSubtypeConst("identifier", id.Name)
		}
		return subtypeClassification{}
	}
	// Local identifier — unresolved value, surface as WARNING for review.
	return dynamicSubtypeIdentifier(id.Name)
}

func (c subtypeExprClassifier) classifyLiteral(lit *ast.BasicLit) subtypeClassification {
	if lit == nil || lit.Kind != token.STRING {
		return subtypeClassification{}
	}
	return classifyStringValue(unquoteSimple(lit.Value), c.allowlist, c.adHoc)
}

func (c subtypeExprClassifier) classifyCall(call *ast.CallExpr) subtypeClassification {
	if call == nil || !isSubtypeCast(call.Fun) || len(call.Args) != 1 {
		return subtypeClassification{}
	}
	lit, ok := call.Args[0].(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return dynamicSubtypeCast()
	}
	return c.classifyLiteral(lit)
}

func (c subtypeExprClassifier) declaredName(name string) bool {
	if c.nameset == nil {
		return true
	}
	_, ok := c.nameset[name]
	return ok
}

func isSubtypeConstName(name string) bool {
	return strings.HasPrefix(name, "Subtype") && name != "Subtype"
}

func undeclaredSubtypeConst(kind, name string) subtypeClassification {
	return subtypeClassification{
		rule:       "declared_subtype",
		action:     ActionReject,
		message:    "Subtype " + kind + " " + name + " is not declared in any errs/subtypes*.go file",
		suggestion: "use a declared const from errs/subtypes*.go (or add one) — typo'd " + kind + " names are silently treated as the zero Subtype",
	}
}

func dynamicSubtypeIdentifier(name string) subtypeClassification {
	return subtypeClassification{
		rule:       "declared_subtype",
		action:     ActionWarning,
		message:    "Subtype assigned from identifier " + name + " — value resolution requires manual review",
		suggestion: "prefer named constants from errs/subtypes.go (e.g. errs.SubtypeMissingScope); if dynamic, justify in PR description",
	}
}

func dynamicSubtypeCast() subtypeClassification {
	return subtypeClassification{
		rule:       "declared_subtype",
		action:     ActionWarning,
		message:    "errs.Subtype(...) cast from non-literal expression — value resolution requires manual review",
		suggestion: "prefer named constants from errs/subtypes.go",
	}
}

// classifyStringValue is the inner classifier for unquoted Subtype string
// literals: ad_hoc_* → CheckAdHocSubtype LABEL, declared → silent accept, anything
// else → CheckDeclaredSubtype REJECT.
func classifyStringValue(value string, allowlist map[string]struct{}, adHoc *regexp.Regexp) subtypeClassification {
	if adHoc.MatchString(value) {
		return subtypeClassification{
			rule:       "adhoc_subtype",
			action:     ActionLabel,
			message:    `Subtype "` + value + `" matches ad_hoc_* temporary namespace — add label "needs-taxonomy-decision" [needs-taxonomy-decision]`,
			suggestion: "promote ad_hoc_* to a declared Subtype constant within 1 week",
		}
	}
	if allowlist == nil {
		return subtypeClassification{}
	}
	if _, ok := allowlist[value]; ok {
		return subtypeClassification{}
	}
	return subtypeClassification{
		rule:    "declared_subtype",
		action:  ActionReject,
		message: `Subtype "` + value + `" is not declared in errs/subtypes.go and does not match ad_hoc_* namespace`,
		suggestion: "use a declared const from errs/subtypes.go (e.g. errs.SubtypeMissingScope), " +
			"or use ad_hoc_<name> temporarily and file a taxonomy issue",
	}
}

// classifyConstViaTypes is the typed-resolution gate used by CheckDeclaredSubtype for
// every selector or identifier appearing in a `Subtype:` slot. Unlike the
// AST path it does NOT pre-filter by name prefix — a foreign constant
// named `MyKind` (or any other shape) assigned to `Subtype:` is still sent
// through resolution. Return values:
//
//   - handled=true,  classification.action == ""        : resolved to a
//     declared errs.Subtype constant; accept without further AST checks.
//   - handled=true,  classification.action == ActionReject : resolved to a
//     non-errs / non-Subtype constant; reject end-to-end.
//   - handled=false                                     : nothing to say
//     (scope disabled, file not in typed load, identifier resolves to a
//     non-const such as a struct field or type); caller falls back to AST.
func classifyConstViaTypes(ident *ast.Ident, absPath string, scope *TypedScope) (subtypeClassification, bool) {
	if ident == nil || !scope.Enabled() {
		return subtypeClassification{}, false
	}
	resolved, ok := scope.ResolveSubtypeIdent(absPath, ident)
	if !ok {
		return subtypeClassification{}, false
	}
	if resolved {
		return subtypeClassification{}, true
	}
	// Resolved via type info, but the object is not a canonical errs.Subtype
	// constant — either it lives in a foreign package or it is an errs
	// constant that is not in the Subtype set.
	return subtypeClassification{
		rule:       "declared_subtype",
		action:     ActionReject,
		message:    "Subtype value " + ident.Name + " resolves to a constant outside the canonical errs.Subtype declarations",
		suggestion: "use a declared const from errs/subtypes*.go — typed Subtype values must originate from " + errsPkgPath,
	}, true
}

// isSubtypeCast reports whether a call-expression callee is the
// `errs.Subtype` (or local `Subtype`) type-cast form.
func isSubtypeCast(fun ast.Expr) bool {
	switch f := fun.(type) {
	case *ast.Ident:
		return f.Name == "Subtype"
	case *ast.SelectorExpr:
		return f.Sel != nil && f.Sel.Name == "Subtype"
	}
	return false
}

// unquoteSimple strips one layer of surrounding double or back quotes.
// Sufficient for Go string literals as they appear in the AST.
func unquoteSimple(quoted string) string {
	if len(quoted) >= 2 && (quoted[0] == '"' || quoted[0] == '`') {
		return quoted[1 : len(quoted)-1]
	}
	return quoted
}
