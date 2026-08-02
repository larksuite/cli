// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package flagcontract keeps flag aliases on the shared framework path.
package flagcontract

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"

	"github.com/larksuite/cli/lint/lintapi"
)

const aliasOwnerPath = "internal/flagalias/flagalias.go"

// ScanOptions mirrors the aggregate lint runner's incremental interface. The
// alias rules are repository invariants and intentionally scan all production
// Go files; ChangedFrom is retained for a uniform caller contract.
type ScanOptions struct {
	ChangedFrom string
}

func ScanRepoWithOptions(root string, _ ScanOptions) ([]lintapi.Violation, error) {
	var out []lintapi.Violation
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", ".claude", "vendor", "node_modules", "testdata":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return nil // another compiler/lint stage owns syntax errors
		}
		ast.Inspect(file, func(node ast.Node) bool {
			value, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			selector, ok := value.Fun.(*ast.SelectorExpr)
			if ok && selector.Sel.Name == "SetNormalizeFunc" && rel != aliasOwnerPath {
				out = append(out, violation(fset, rel, value.Pos(),
					"flag_alias_normalizer_owner",
					"SetNormalizeFunc is owned by internal/flagalias",
					"declare exact synonyms with common.Flag.Aliases or call flagalias.Bind from a framework adapter"))
			}
			return true
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].File != out[j].File {
			return out[i].File < out[j].File
		}
		if out[i].Line != out[j].Line {
			return out[i].Line < out[j].Line
		}
		return out[i].Rule < out[j].Rule
	})
	return out, nil
}

func violation(fset *token.FileSet, file string, pos token.Pos, rule, message, suggestion string) lintapi.Violation {
	return lintapi.Violation{
		Rule:       rule,
		Action:     lintapi.ActionReject,
		File:       file,
		Line:       fset.Position(pos).Line,
		Message:    message,
		Suggestion: suggestion,
	}
}
