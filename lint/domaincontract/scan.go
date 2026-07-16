// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package domaincontract guards the Go CLI against direct reuse of the current
// resolver-owned host FQDNs outside core.ResolveEndpoints.
package domaincontract

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/larksuite/cli/lint/lintapi"
)

// forbiddenHosts are the resolver-owned FQDNs. They may only appear as string
// literals in the allowlisted resolver source.
var forbiddenHosts = []string{
	"open.feishu.cn", "accounts.feishu.cn", "mcp.feishu.cn", "applink.feishu.cn",
	"open.larksuite.com", "accounts.larksuite.com", "mcp.larksuite.com", "applink.larksuite.com",
}

// forbiddenIdents are the SDK root package's base-URL globals; referencing
// them picks a host without the resolver. Matched as selectors on an SDK root
// import, so unrelated same-name identifiers are not flagged.
var forbiddenIdents = map[string]bool{
	"FeishuBaseUrl": true,
	"LarkBaseUrl":   true,
}

// sdkModulePrefix identifies imports of the Lark OAPI SDK.
const sdkModulePrefix = "github.com/larksuite/oapi-sdk-go/"

// sdkImportAliases returns the file's local names for the SDK root package
// (subpackages do not export the base-URL globals).
func sdkImportAliases(file *ast.File) map[string]bool {
	aliases := map[string]bool{}
	for _, imp := range file.Imports {
		path, err := strconv.Unquote(imp.Path.Value)
		if err != nil || !strings.HasPrefix(path, sdkModulePrefix) {
			continue
		}
		if strings.Contains(strings.TrimPrefix(path, sdkModulePrefix), "/") {
			continue // subpackage, not the root
		}
		name := "lark" // the SDK root package's package name
		if imp.Name != nil {
			name = imp.Name.Name
		}
		aliases[name] = true
	}
	return aliases
}

// allowlist holds the only file allowed to carry the literals wholesale:
// this rule's own host list. The resolver file is scoped per-function instead
// (see resolverPath).
var allowlist = map[string]bool{
	filepath.FromSlash("lint/domaincontract/scan.go"): true,
}

// resolverPath is the resolver source; host literals are permitted only
// inside its ResolveEndpoints function body.
var resolverPath = filepath.FromSlash("internal/core/types.go")

func skipDir(name string) bool {
	switch name {
	case "vendor", "testdata", "node_modules", ".git", ".claude":
		return true
	}
	return false
}

// ScanRepo walks production .go files under root and flags string literals
// containing a forbidden resolver host outside the allowlist. Comments and
// _test.go files are not scanned.
func ScanRepo(root string) ([]lintapi.Violation, error) {
	var out []lintapi.Violation
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr == nil && allowlist[rel] {
			return nil
		}
		fset := token.NewFileSet()
		file, perr := parser.ParseFile(fset, path, nil, 0)
		if perr != nil {
			return nil // unparseable file: not our concern
		}
		display := path
		if relErr == nil {
			display = rel
		}
		var allowedFrom, allowedTo token.Pos
		if relErr == nil && rel == resolverPath {
			for _, d := range file.Decls {
				if fd, ok := d.(*ast.FuncDecl); ok && fd.Recv == nil && fd.Name.Name == "ResolveEndpoints" && fd.Body != nil {
					allowedFrom, allowedTo = fd.Body.Pos(), fd.Body.End()
					break
				}
			}
		}
		inResolverBody := func(p token.Pos) bool {
			return allowedFrom != token.NoPos && p >= allowedFrom && p <= allowedTo
		}
		// Dot-imports of the SDK root would hide its globals from this
		// parse-level guard, so the import form itself is rejected.
		for _, imp := range file.Imports {
			path, uerr := strconv.Unquote(imp.Path.Value)
			if uerr != nil || imp.Name == nil || imp.Name.Name != "." {
				continue
			}
			if strings.HasPrefix(path, sdkModulePrefix) &&
				!strings.Contains(strings.TrimPrefix(path, sdkModulePrefix), "/") {
				pos := fset.Position(imp.Pos())
				out = append(out, lintapi.Violation{
					Rule:       "no-hardcoded-endpoint",
					Action:     lintapi.ActionReject,
					File:       display,
					Line:       pos.Line,
					Message:    "dot-import of the SDK root package defeats the endpoint guard",
					Suggestion: "import the SDK with a package name",
				})
			}
		}
		sdkAliases := sdkImportAliases(file)
		ast.Inspect(file, func(n ast.Node) bool {
			switch node := n.(type) {
			case *ast.SelectorExpr:
				pkg, ok := node.X.(*ast.Ident)
				if ok && pkg.Obj == nil && forbiddenIdents[node.Sel.Name] && sdkAliases[pkg.Name] {
					pos := fset.Position(node.Pos())
					out = append(out, lintapi.Violation{
						Rule:       "no-hardcoded-endpoint",
						Action:     lintapi.ActionReject,
						File:       display,
						Line:       pos.Line,
						Message:    "SDK base-URL global " + pkg.Name + "." + node.Sel.Name + " bypasses the resolver — use core.ResolveEndpoints",
						Suggestion: "derive the host from core.ResolveEndpoints(brand) instead of the SDK global",
					})
				}
			case *ast.BasicLit:
				if node.Kind != token.STRING {
					return true
				}
				if inResolverBody(node.Pos()) {
					return true
				}
				// Unquote and lowercase so escapes or casing cannot hide a host.
				value := node.Value
				if v, err := strconv.Unquote(value); err == nil {
					value = v
				}
				lower := strings.ToLower(value)
				for _, host := range forbiddenHosts {
					if strings.Contains(lower, host) {
						pos := fset.Position(node.Pos())
						out = append(out, lintapi.Violation{
							Rule:       "no-hardcoded-endpoint",
							Action:     lintapi.ActionReject,
							File:       display,
							Line:       pos.Line,
							Message:    "hardcoded resolver host " + host + " — outbound domains must come from core.ResolveEndpoints",
							Suggestion: "use core.ResolveEndpoints(brand) instead of a literal host",
						})
						return true
					}
				}
			}
			return true
		})
		return nil
	})
	return out, err
}
