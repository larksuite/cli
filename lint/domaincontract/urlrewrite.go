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

// unrewrittenURLViolation rejects an added static URL in CLI runtime code that
// is not wrapped in urlrewrite.Rewrite.
func (s *fileDomainScan) unrewrittenURLViolation(rel string, evidence domainEvidence, added []addedLineRange) (lintapi.Violation, bool) {
	if evidence.Kind != "absolute URL" || !urlRewriteRuntimeFile(rel) || s.urlRewriteExempt(rel, evidence.Expr) {
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

// urlRewriteExempt reports whether a static URL needs no Rewrite wrapper: it
// sits inside the endpoint resolver body (platform URLs are rewritten by the
// transport layer), inside a urlrewrite.Rewrite call, or next to a documented
// //nolint:urlrewrite reason.
func (s *fileDomainScan) urlRewriteExempt(rel string, expr ast.Expr) bool {
	aliases := urlRewriteAliases(s.File)
	for node := ast.Node(expr); node != nil; node = s.parents[node] {
		switch n := node.(type) {
		case *ast.CallExpr:
			sel, ok := n.Fun.(*ast.SelectorExpr)
			pkg, pkgOK := sel.X.(*ast.Ident)
			if ok && pkgOK && aliases[pkg.Name] && sel.Sel.Name == "Rewrite" {
				return true
			}
		case *ast.FuncDecl:
			if rel == resolverPath && n.Recv == nil && n.Name.Name == "ResolveEndpoints" {
				return true
			}
		}
	}
	return s.hasURLRewriteExemption(expr)
}

// urlRewriteAliases returns the import names under which this file imports the
// urlrewrite package (usually just "urlrewrite").
func urlRewriteAliases(file *ast.File) map[string]bool {
	aliases := map[string]bool{}
	for _, imp := range file.Imports {
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
	return aliases
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
