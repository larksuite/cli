// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package client

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/vfs"
)

const externalCredentialImplementation = "github.com/larksuite/cli/internal/externalcredential"

// These functions own protocols that are explicitly outside the
// service-returned file plane: user-supplied external downloads and MCP.
// Adding a boundary is an architectural decision; //nolint alone must never
// make a new raw HTTP path possible.
var shortcutRawHTTPBoundaries = map[string]string{
	"common/mcp_client.go:DoMCPCall":                           "MCP protocol client",
	"doc/doc_resource_cover.go:downloadDocCoverURL":            "validated user-supplied cover URL",
	"doc/doc_resource_cover.go:newDocCoverHTTPClient":          "guarded external-download client",
	"doc/doc_resource_cover.go:cloneDocCoverTransport":         "guarded external-download transport",
	"doc/local_doc_resources.go:downloadRemoteDocImageContent": "validated user-supplied image URL",
	"doc/local_doc_resources.go:probeRemoteDocImageRequest":    "validated user-supplied image URL",
	"im/helpers.go:startURLDownload":                           "validated user-supplied media URL",
}

func TestCoreDoesNotOwnExternalCredentialProductModel(t *testing.T) {
	checkCoreDirectoryIsProductNeutral(t, filepath.Join("..", "core"))
}

func TestRuntimeFrameworkDoesNotImportExternalCredentialImplementation(t *testing.T) {
	for _, dir := range []string{
		".",
		filepath.Join("..", "cmdutil"),
		filepath.Join("..", "credential"),
	} {
		checkDirectoryDoesNotImportExternalCredential(t, dir)
	}
}

func checkCoreDirectoryIsProductNeutral(t *testing.T, dir string) {
	t.Helper()
	entries, err := vfs.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}

	forbidden := []string{
		"externalcredential",
		"external-credential",
		"credential_proxy",
		"platform_proxy",
	}
	for _, entry := range entries {
		path := filepath.Join(dir, entry.Name())
		if entry.IsDir() {
			checkCoreDirectoryIsProductNeutral(t, path)
			continue
		}
		if !strings.HasSuffix(entry.Name(), ".go") {
			continue
		}
		src, err := vfs.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		normalizedSource := strings.ToLower(string(src))
		for _, productSymbol := range forbidden {
			if strings.Contains(normalizedSource, strings.ToLower(productSymbol)) {
				t.Errorf("%s contains external credential product symbol %q; keep the core profile model product-neutral", path, productSymbol)
			}
		}
	}
}

func TestBusinessShortcutsUseRuntimeCredentialBoundaries(t *testing.T) {
	checkBusinessShortcutDirectory(t, filepath.Join("..", "..", "shortcuts"))
}

func TestShortcutRawHTTPConstructionIsConfinedToExplicitBoundaries(t *testing.T) {
	root := filepath.Join("..", "..", "shortcuts")
	checkShortcutRawHTTPDirectory(t, root, root)
}

func TestRawHTTPConstructionGuardRecognizesAliasedBypasses(t *testing.T) {
	const source = `package fixture
import nethttp "net/http"
func bypass() {
	_ = &nethttp.Client{}
	_, _ = nethttp.NewRequest(nethttp.MethodGet, "https://files.example", nil)
	_ = nethttp.DefaultClient
}`
	file, err := parser.ParseFile(token.NewFileSet(), "fixture.go", source, 0)
	if err != nil {
		t.Fatal(err)
	}
	uses := rawHTTPConstructions(file, map[string]struct{}{"nethttp": {}})
	joined := strings.Join(uses, ",")
	for _, want := range []string{"Client literal", "NewRequest", "DefaultClient"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("raw HTTP uses = %q, want %q", joined, want)
		}
	}
}

func checkDirectoryDoesNotImportExternalCredential(t *testing.T, dir string) {
	t.Helper()
	entries, err := vfs.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		path := filepath.Join(dir, entry.Name())
		if entry.IsDir() {
			checkDirectoryDoesNotImportExternalCredential(t, path)
			continue
		}
		if !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		src, err := vfs.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, src, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		for _, spec := range file.Imports {
			importPath, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				t.Fatalf("parse import in %s: %v", path, err)
			}
			if importPath == externalCredentialImplementation ||
				strings.HasPrefix(importPath, externalCredentialImplementation+"/") {
				t.Errorf("%s imports external credential implementation; inject a source-neutral runtime boundary", path)
			}
		}
	}
}

func checkBusinessShortcutDirectory(t *testing.T, dir string) {
	t.Helper()
	entries, err := vfs.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		path := filepath.Join(dir, entry.Name())
		if entry.IsDir() {
			if entry.Name() != "common" {
				checkBusinessShortcutDirectory(t, path)
			}
			continue
		}
		if !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		src, err := vfs.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, src, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		for _, spec := range file.Imports {
			importPath, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				t.Fatalf("parse import in %s: %v", path, err)
			}
			if importPath == externalCredentialImplementation ||
				strings.HasPrefix(importPath, externalCredentialImplementation+"/") {
				t.Errorf("%s imports external credential implementation; use a source-neutral runtime capability boundary", path)
			}
		}
		ast.Inspect(file, func(node ast.Node) bool {
			selector, ok := node.(*ast.SelectorExpr)
			if ok && selector.Sel.Name == "ExternalCredential" {
				t.Errorf("%s selects ExternalCredential directly; use a runtime capability boundary", path)
			}
			return true
		})
	}
}

func checkShortcutRawHTTPDirectory(t *testing.T, root, dir string) {
	t.Helper()
	entries, err := vfs.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		path := filepath.Join(dir, entry.Name())
		if entry.IsDir() {
			checkShortcutRawHTTPDirectory(t, root, path)
			continue
		}
		if !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		src, err := vfs.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, src, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		httpAliases := make(map[string]struct{})
		for _, spec := range file.Imports {
			importPath, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				t.Fatalf("parse import in %s: %v", path, err)
			}
			if importPath != "net/http" {
				continue
			}
			alias := "http"
			if spec.Name != nil {
				alias = spec.Name.Name
			}
			if alias == "." {
				t.Errorf("%s dot-imports net/http; raw HTTP boundaries must remain statically auditable", path)
				continue
			}
			if alias != "_" {
				httpAliases[alias] = struct{}{}
			}
		}
		if len(httpAliases) == 0 {
			continue
		}

		relativePath, err := filepath.Rel(root, path)
		if err != nil {
			t.Fatalf("relative shortcut path for %s: %v", path, err)
		}
		relativePath = filepath.ToSlash(relativePath)
		for _, decl := range file.Decls {
			fn, isFunc := decl.(*ast.FuncDecl)
			uses := rawHTTPConstructions(decl, httpAliases)
			if len(uses) == 0 {
				continue
			}
			if !isFunc {
				t.Errorf("%s constructs raw net/http at package scope (%s); route service-returned file bytes through RuntimeContext.RemoteFiles",
					path, strings.Join(uses, ", "))
				continue
			}
			boundary := relativePath + ":" + fn.Name.Name
			if _, allowed := shortcutRawHTTPBoundaries[boundary]; allowed {
				continue
			}
			t.Errorf("%s function %s constructs raw net/http (%s); route service-returned file bytes through RuntimeContext.RemoteFiles or declare a reviewed non-file-plane boundary",
				path, fn.Name.Name, strings.Join(uses, ", "))
		}
	}
}

func rawHTTPConstructions(node ast.Node, aliases map[string]struct{}) []string {
	seen := make(map[string]struct{})
	var uses []string
	add := func(name string) {
		if _, ok := seen[name]; ok {
			return
		}
		seen[name] = struct{}{}
		uses = append(uses, name)
	}
	isHTTPSelector := func(selector *ast.SelectorExpr) bool {
		ident, ok := selector.X.(*ast.Ident)
		if !ok {
			return false
		}
		_, ok = aliases[ident.Name]
		return ok
	}

	ast.Inspect(node, func(current ast.Node) bool {
		switch typed := current.(type) {
		case *ast.CallExpr:
			selector, ok := typed.Fun.(*ast.SelectorExpr)
			if !ok || !isHTTPSelector(selector) {
				break
			}
			switch selector.Sel.Name {
			case "NewRequest", "NewRequestWithContext", "Get", "Post", "PostForm", "Head", "Serve", "ListenAndServe":
				add(selector.Sel.Name)
			}
		case *ast.CompositeLit:
			selector, ok := typed.Type.(*ast.SelectorExpr)
			if !ok || !isHTTPSelector(selector) {
				break
			}
			switch selector.Sel.Name {
			case "Client", "Request", "Transport":
				add(selector.Sel.Name + " literal")
			}
		case *ast.SelectorExpr:
			if !isHTTPSelector(typed) {
				break
			}
			switch typed.Sel.Name {
			case "DefaultClient", "DefaultTransport":
				add(typed.Sel.Name)
			}
		}
		return true
	})
	return uses
}
