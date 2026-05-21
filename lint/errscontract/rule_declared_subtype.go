// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package errscontract

import (
	"go/ast"
	"go/parser"
	"go/token"
	"regexp"
)

// CheckDeclaredSubtype enforces that `Subtype:` literals resolve to a
// declared constant value (allowlist), match the ad_hoc_* namespace (deferred
// to CheckAdHocSubtype), or are dynamic (WARNING). Undeclared static literals are
// rejected.
//
// allowlist holds declared Subtype const values (e.g. "missing_scope"). The
// production CLI derives this from errs/subtypes*.go via the AST; unit tests
// pass in a fixture map. Passing nil disables CheckDeclaredSubtype entirely.
//
// Use CheckDeclaredSubtypeWithNames to additionally reject typo'd selector
// references like `errs.SubtypeBogus` that pass the "Subtype*" prefix gate but
// reference no declared constant.
func CheckDeclaredSubtype(path, src string, allowlist map[string]struct{}) []Violation {
	return CheckDeclaredSubtypeWithNames(path, src, allowlist, nil)
}

// CheckDeclaredSubtypeWithNames is the strengthened entry point. When
// nameset is non-nil, selector references with the form `<pkg>.SubtypeFoo`
// must resolve to a declared name in the set; otherwise they emit REJECT.
// Passing nil for nameset preserves the legacy prefix-only behaviour.
func CheckDeclaredSubtypeWithNames(path, src string, allowlist, nameset map[string]struct{}) []Violation {
	if allowlist == nil {
		return nil
	}
	v, _ := scanSubtype(path, src, allowlist, nameset, nil, "")
	out := v[:0]
	for _, vv := range v {
		if vv.Action == ActionReject || vv.Action == ActionWarning {
			out = append(out, vv)
		}
	}
	return out
}

// checkDeclaredSubtypeWithTypedScope is the production walker invoked by ScanRepo. When
// scope is enabled, every Subtype-shaped selector is resolved via type
// information first: a confirmed errs.Subtype constant skips the AST
// nameset check, and a foreign-package Subtype constant is rejected even
// when its name matches the nameset. Scope can be nil — in which case
// behaviour collapses to CheckDeclaredSubtypeWithNames.
//
// absPath is the absolute path used during go/packages loading so the
// typed scope can locate per-file *types.Info; rel is the human-readable
// path embedded in violation reports.
func checkDeclaredSubtypeWithTypedScope(rel, absPath, src string, allowlist, nameset map[string]struct{}, scope *TypedScope) []Violation {
	if allowlist == nil {
		return nil
	}
	v, _ := scanSubtype(rel, src, allowlist, nameset, scope, absPath)
	out := v[:0]
	for _, vv := range v {
		if vv.Action == ActionReject || vv.Action == ActionWarning {
			out = append(out, vv)
		}
	}
	return out
}

// scanSubtype walks the file AST and classifies every `Subtype:` key-value
// assignment in a composite literal. It returns the full classified list; the
// two callers (CheckAdHocSubtype / CheckDeclaredSubtype) filter by Action.
//
// nameset, when non-nil, lets the classifier reject selector references that
// pass the "Subtype*" prefix gate but resolve to no declared constant.
//
// scope+absPath, when set, enable typed resolution: every Subtype-shaped
// identifier is first resolved through go/types to verify it references a
// constant declared in the canonical errs package. A foreign-package
// Subtype-named constant is rejected even when nameset permits it (because
// selector-name matching alone cannot distinguish packages).
func scanSubtype(path, src string, allowlist, nameset map[string]struct{}, scope *TypedScope, absPath string) ([]Violation, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, src, parser.ParseComments)
	if err != nil {
		return nil, err
	}
	adHoc := regexp.MustCompile(`^ad_hoc_[a-z0-9_]+$`)
	var out []Violation
	emit := func(pos token.Pos, c subtypeClassification) {
		if c.action == "" {
			return
		}
		out = append(out, Violation{
			Rule:       c.rule,
			Action:     c.action,
			File:       path,
			Line:       fset.Position(pos).Line,
			Message:    c.message,
			Suggestion: c.suggestion,
		})
	}
	// Track CompositeLit nodes whose Type elides to CodeMeta (map/slice
	// elements where the outer Type already names CodeMeta). We populate this
	// set on the outer pass so the inner pass can recognise positional
	// `{cat, subtype, retryable}` entries that don't carry their own Type
	// expression.
	codeMetaElided := map[*ast.CompositeLit]bool{}
	ast.Inspect(file, func(n ast.Node) bool {
		outer, ok := n.(*ast.CompositeLit)
		if !ok || !typeYieldsCodeMeta(outer.Type) {
			return true
		}
		for _, el := range outer.Elts {
			// `key: {cat, subtype, retryable}` — map literal
			if kv, ok := el.(*ast.KeyValueExpr); ok {
				if inner, ok := kv.Value.(*ast.CompositeLit); ok && inner.Type == nil {
					codeMetaElided[inner] = true
				}
				continue
			}
			// `{cat, subtype, retryable}` — slice/array element
			if inner, ok := el.(*ast.CompositeLit); ok && inner.Type == nil {
				codeMetaElided[inner] = true
			}
		}
		return true
	})

	ast.Inspect(file, func(n ast.Node) bool {
		cl, ok := n.(*ast.CompositeLit)
		if !ok {
			return true
		}
		// Keyed form: `Subtype: <expr>` — covered for every struct literal.
		for _, el := range cl.Elts {
			kv, ok := el.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			keyIdent, ok := kv.Key.(*ast.Ident)
			if !ok || keyIdent.Name != "Subtype" {
				continue
			}
			emit(kv.Pos(), classifySubtypeExpr(kv.Value, allowlist, nameset, adHoc, scope, absPath))
		}
		// Positional form: `{cat, subtype, retryable}` used by
		// internal/errclass/codemeta*.go for CodeMeta entries. Subtype is
		// element [1] by positional convention. We inspect when the
		// composite literal's Type expression directly names CodeMeta OR
		// when the Type was elided because the enclosing map/slice already
		// declared CodeMeta as its value type.
		if (isCodeMetaType(cl.Type) || codeMetaElided[cl]) && len(cl.Elts) >= 2 {
			// Don't double-emit if element [1] is itself a KeyValueExpr (handled above).
			if _, isKV := cl.Elts[1].(*ast.KeyValueExpr); !isKV {
				emit(cl.Elts[1].Pos(), classifySubtypeExpr(cl.Elts[1], allowlist, nameset, adHoc, scope, absPath))
			}
		}
		return true
	})
	return out, nil
}

// isCodeMetaType reports whether a composite-literal Type expression directly
// names the CodeMeta struct (bare or qualified).
func isCodeMetaType(expr ast.Expr) bool {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name == "CodeMeta"
	case *ast.SelectorExpr:
		return t.Sel != nil && t.Sel.Name == "CodeMeta"
	}
	return false
}

// typeYieldsCodeMeta reports whether a Type expression for a map/slice/array
// composite literal has CodeMeta as its element/value type. Used so we can
// recognise that the elided `{cat, subtype, retryable}` entries inside such a
// literal are positional CodeMeta values whose Subtype lives at element [1].
func typeYieldsCodeMeta(expr ast.Expr) bool {
	switch t := expr.(type) {
	case *ast.MapType:
		return isCodeMetaType(t.Value)
	case *ast.ArrayType:
		return isCodeMetaType(t.Elt)
	}
	return false
}
