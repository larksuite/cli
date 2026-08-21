// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package rules

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"

	"github.com/larksuite/cli/internal/qualitygate/report"
	"github.com/larksuite/cli/internal/vfs"
)

const roundTripRetryWaiver = "qualitygate:allow-roundtrip-retry"

// CheckTransportRetryLoops rejects RoundTrip implementations that invoke a
// lower RoundTrip from a loop. A RoundTripper handles one request, so this
// structure can make retries invisible to the caller.
func CheckTransportRetryLoops(repo string, changedFiles []string, changedOnly bool) ([]report.Diagnostic, error) {
	files, err := transportRuleFiles(repo, changedFiles, changedOnly)
	if err != nil {
		return nil, err
	}

	var diagnostics []report.Diagnostic
	for _, path := range files {
		src, err := vfs.ReadFile(filepath.Join(repo, filepath.FromSlash(path)))
		if err != nil {
			return nil, err
		}
		fileDiagnostics, err := checkTransportRetrySource(path, src)
		if err != nil {
			return nil, err
		}
		diagnostics = append(diagnostics, fileDiagnostics...)
	}
	return diagnostics, nil
}

func checkTransportRetrySource(path string, src []byte) ([]report.Diagnostic, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, src, parser.ParseComments)
	if err != nil {
		return nil, err
	}

	var diagnostics []report.Diagnostic
	for _, declaration := range file.Decls {
		fn, ok := declaration.(*ast.FuncDecl)
		if !ok || fn.Recv == nil || fn.Name == nil || fn.Name.Name != "RoundTrip" || fn.Body == nil {
			continue
		}
		loop := roundTripCallLoop(fn.Body)
		if loop == nil || hasRoundTripRetryWaiver(fn.Doc) {
			continue
		}
		diagnostics = append(diagnostics, report.Diagnostic{
			Rule:    "transport_no_automatic_retry",
			Action:  report.ActionReject,
			File:    path,
			Line:    fset.Position(loop.Pos()).Line,
			Message: "RoundTrip calls another RoundTrip from a loop, which can hide repeated requests from the caller",
			Suggestion: "remove the transport-level retry; an exceptional implementation must add " +
				"`// qualitygate:allow-roundtrip-retry <reason>` to the RoundTrip doc comment",
		})
	}
	return diagnostics, nil
}

func roundTripCallLoop(body *ast.BlockStmt) ast.Node {
	var found ast.Node
	ast.Inspect(body, func(node ast.Node) bool {
		if node == nil || found != nil {
			return false
		}
		switch loop := node.(type) {
		case *ast.ForStmt:
			if containsRoundTripCall(loop.Body) {
				found = loop
				return false
			}
		case *ast.RangeStmt:
			if containsRoundTripCall(loop.Body) {
				found = loop
				return false
			}
		}
		return true
	})
	return found
}

func containsRoundTripCall(node ast.Node) bool {
	found := false
	ast.Inspect(node, func(node ast.Node) bool {
		if node == nil || found {
			return false
		}
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if ok && selector.Sel.Name == "RoundTrip" {
			found = true
			return false
		}
		return true
	})
	return found
}

func hasRoundTripRetryWaiver(doc *ast.CommentGroup) bool {
	if doc == nil {
		return false
	}
	for _, comment := range doc.List {
		text := strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(comment.Text, "//"), "/*"))
		index := strings.Index(text, roundTripRetryWaiver)
		if index < 0 {
			continue
		}
		reason := strings.TrimSpace(strings.TrimSuffix(text[index+len(roundTripRetryWaiver):], "*/"))
		if reason != "" {
			return true
		}
	}
	return false
}

func transportRuleFiles(repo string, changedFiles []string, changedOnly bool) ([]string, error) {
	if changedOnly {
		var files []string
		for _, path := range changedFiles {
			path = filepath.ToSlash(path)
			if !isTransportRuleGoFile(path) {
				continue
			}
			if _, err := vfs.Stat(filepath.Join(repo, filepath.FromSlash(path))); err != nil {
				if errors.Is(err, fs.ErrNotExist) {
					continue
				}
				return nil, err
			}
			files = append(files, path)
		}
		sort.Strings(files)
		return files, nil
	}

	var files []string
	if err := walkTransportRuleFiles(repo, "", &files); err != nil {
		return nil, err
	}
	sort.Strings(files)
	return files, nil
}

func walkTransportRuleFiles(repo, rel string, files *[]string) error {
	entries, err := vfs.ReadDir(filepath.Join(repo, filepath.FromSlash(rel)))
	if err != nil {
		return err
	}
	for _, entry := range entries {
		child := filepath.ToSlash(filepath.Join(rel, entry.Name()))
		if entry.IsDir() {
			if skipTransportRuleDir(entry.Name()) {
				continue
			}
			if err := walkTransportRuleFiles(repo, child, files); err != nil {
				return err
			}
			continue
		}
		if isTransportRuleGoFile(child) {
			*files = append(*files, child)
		}
	}
	return nil
}

func skipTransportRuleDir(name string) bool {
	return name == ".git" || name == ".tmp" || name == "node_modules" || name == "testdata" || name == "vendor"
}

func isTransportRuleGoFile(path string) bool {
	return strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go")
}
