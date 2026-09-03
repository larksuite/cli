// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package domaincontract

import (
	"go/ast"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/larksuite/cli/lint/lintapi"
)

const urlRewriteImport = "github.com/larksuite/cli/internal/urlrewrite"

func (s *fileDomainScan) unrewrittenURLViolation(rel string, evidence domainEvidence, added []addedLineRange) (lintapi.Violation, bool) {
	if evidence.Kind != "absolute URL" || !urlRewriteRuntimeFile(rel) ||
		(rel == resolverPath && s.inEndpointResolver(evidence.Expr)) || s.inURLRewriteCall(evidence.Expr) || s.hasURLRewriteExemption(evidence.Expr) {
		return lintapi.Violation{}, false
	}
	start := s.Fset.Position(evidence.Expr.Pos()).Line
	end := s.Fset.Position(evidence.Expr.End()).Line
	line, ok := firstAddedLineInSpan(added, start, end)
	if !ok {
		return lintapi.Violation{}, false
	}
	return lintapi.Violation{
		Rule:       urlRewriteRule,
		Action:     lintapi.ActionReject,
		File:       rel,
		Line:       line,
		Message:    "new static URL must pass through urlrewrite.Rewrite",
		Suggestion: "rewrite the complete URL at its use site, or add //nolint:urlrewrite with a reason when it is not a routable URL",
	}, true
}

func urlRewriteRuntimeFile(rel string) bool {
	rel = filepath.ToSlash(rel)
	if strings.HasSuffix(rel, "_test.go") || strings.Contains(rel, "/testdata/") ||
		strings.HasPrefix(rel, "internal/qualitygate/") || strings.HasPrefix(rel, "internal/testutil/") ||
		strings.HasPrefix(rel, "internal/urlrewrite/") {
		return false
	}
	return rel == "main.go" || strings.HasPrefix(rel, "cmd/") ||
		strings.HasPrefix(rel, "internal/") || strings.HasPrefix(rel, "shortcuts/")
}

func (s *fileDomainScan) inURLRewriteCall(expr ast.Expr) bool {
	aliases := map[string]bool{}
	for _, imp := range s.File.Imports {
		path, err := strconv.Unquote(imp.Path.Value)
		if err != nil || path != urlRewriteImport {
			continue
		}
		name := "urlrewrite"
		if imp.Name != nil {
			name = imp.Name.Name
		}
		aliases[name] = true
	}
	for node := ast.Node(expr); node != nil; node = s.parents[node] {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			continue
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		pkg, pkgOK := sel.X.(*ast.Ident)
		if ok && pkgOK && aliases[pkg.Name] && sel.Sel.Name == "Rewrite" && !s.atPackageScope(call) {
			return true
		}
	}
	return false
}

func (s *fileDomainScan) atPackageScope(node ast.Node) bool {
	for parent := s.parents[node]; parent != nil; parent = s.parents[parent] {
		switch parent.(type) {
		case *ast.FuncDecl, *ast.FuncLit:
			return false
		}
		if _, ok := parent.(*ast.File); ok {
			return true
		}
	}
	return false
}

func (s *fileDomainScan) inEndpointResolver(expr ast.Expr) bool {
	for node := ast.Node(expr); node != nil; node = s.parents[node] {
		if fn, ok := node.(*ast.FuncDecl); ok {
			return fn.Recv == nil && fn.Name.Name == "ResolveEndpoints"
		}
	}
	return false
}

func (s *fileDomainScan) hasURLRewriteExemption(expr ast.Expr) bool {
	start := s.Fset.Position(expr.Pos()).Line
	end := s.Fset.Position(expr.End()).Line
	for _, group := range s.File.Comments {
		line := s.Fset.Position(group.Pos()).Line
		if line != start-1 && (line < start || line > end) {
			continue
		}
		for _, comment := range group.List {
			const marker = "nolint:urlrewrite"
			if index := strings.Index(comment.Text, marker); index >= 0 && strings.TrimSpace(comment.Text[index+len(marker):]) != "" {
				return true
			}
		}
	}
	return false
}
