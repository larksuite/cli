// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package domaincontract

import (
	"go/ast"
	"go/parser"
	"go/token"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/larksuite/cli/lint/lintapi"
)

// requireEnforced pins every violation to the rejecting rule: a regression
// that downgrades the guard to an advisory action must fail here.
func requireEnforced(t *testing.T, vs []lintapi.Violation) {
	t.Helper()
	for _, v := range vs {
		if v.Rule != "no-hardcoded-endpoint" || v.Action != lintapi.ActionReject {
			t.Fatalf("violation not CI-enforced: rule=%q action=%q (%s:%d)", v.Rule, v.Action, v.File, v.Line)
		}
	}
}

func writeFile(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestScanRepo(t *testing.T) {
	root := t.TempDir()
	// Negative: the resolver may hold the literals inside ResolveEndpoints.
	writeFile(t, root, "internal/core/types.go", "package core\n\nfunc ResolveEndpoints(b string) string {\n\treturn \"https://open.feishu.cn\"\n}\n")
	// Negative: non-resolver hosts + a comment reference must not trip the guard.
	writeFile(t, root, "shortcuts/x/display.go", "package x\n\n// see https://open.feishu.cn/document/foo\nvar h = \"https://www.feishu.cn\"\nvar e = \"https://example.feishu.cn\"\nvar r = \"https://registry.npmjs.org/pkg\"\n")
	// Negative: _test.go files may assert literals.
	writeFile(t, root, "internal/y/y_test.go", "package y\n\nvar w = \"https://open.larksuite.com\"\n")
	// Positive: production literal outside the allowlist.
	writeFile(t, root, "internal/z/z.go", "package z\n\nvar bad = \"https://accounts.larksuite.com/oauth\"\n")

	vs, err := ScanRepo(root)
	if err != nil {
		t.Fatal(err)
	}
	requireEnforced(t, vs)
	if len(vs) != 1 {
		t.Fatalf("got %d violations, want 1: %+v", len(vs), vs)
	}
	if filepath.Base(vs[0].File) != "z.go" {
		t.Errorf("violation in %q, want z.go", vs[0].File)
	}
}

// SDK base-URL globals are rejected only when selected off an SDK root
// import; same-name identifiers elsewhere pass.
func TestScanRepoSDKConstants(t *testing.T) {
	root := t.TempDir()
	// Positive: default and renamed imports of the SDK root package.
	writeFile(t, root, "shortcuts/x/ws.go",
		"package x\n\nimport \"github.com/larksuite/oapi-sdk-go/v3\"\n\nvar d = lark.FeishuBaseUrl\n")
	writeFile(t, root, "shortcuts/x/ws2.go",
		"package x\n\nimport sdk \"github.com/larksuite/oapi-sdk-go/v3\"\n\nvar e = sdk.LarkBaseUrl\n")
	// Negative: test file may reference the globals.
	writeFile(t, root, "shortcuts/x/ws_test.go",
		"package x\n\nimport lark \"github.com/larksuite/oapi-sdk-go/v3\"\n\nvar p = lark.LarkBaseUrl\n")
	// Negative: same-name local identifier without the SDK import.
	writeFile(t, root, "shortcuts/y/local.go",
		"package y\n\nvar FeishuBaseUrl = \"local\"\nvar q = FeishuBaseUrl\n")
	// Negative: same-name symbol from an unrelated package.
	writeFile(t, root, "shortcuts/z/other.go",
		"package z\n\nimport other \"example.com/other\"\n\nvar r = other.FeishuBaseUrl\n")
	// Negative: SDK subpackage import does not export the globals.
	writeFile(t, root, "shortcuts/w/sub.go",
		"package w\n\nimport larkws \"github.com/larksuite/oapi-sdk-go/v3/ws\"\n\nvar s = larkws.FeishuBaseUrl\n")
	// Negative: a local value shadowing the SDK import alias is not the package.
	writeFile(t, root, "shortcuts/v/shadow.go",
		"package v\n\nimport lark \"github.com/larksuite/oapi-sdk-go/v3\"\n\ntype endpoint struct { FeishuBaseUrl string }\nvar _ *lark.Client\nfunc local() string { lark := endpoint{}; return lark.FeishuBaseUrl }\n")

	vs, err := ScanRepo(root)
	if err != nil {
		t.Fatal(err)
	}
	requireEnforced(t, vs)
	if len(vs) != 2 {
		t.Fatalf("got %d violations, want 2: %+v", len(vs), vs)
	}
	files := map[string]bool{}
	for _, v := range vs {
		files[filepath.Base(v.File)] = true
	}
	if !files["ws.go"] || !files["ws2.go"] {
		t.Errorf("violations in %v, want ws.go and ws2.go", files)
	}
}

// forbiddenHosts must equal the https hosts in the resolver source, both ways;
// a resolver domain change without a guard update fails here.
func TestForbiddenHostsMatchResolver(t *testing.T) {
	src := filepath.Join("..", "..", "internal", "core", "types.go")
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, src, nil, 0)
	if err != nil {
		t.Fatalf("parse resolver source: %v", err)
	}
	// Walk only the receiverless ResolveEndpoints body — the same scope the
	// production scanner exempts — so unrelated URLs in the file cannot skew
	// the parity check.
	var resolverBody ast.Node
	for _, d := range file.Decls {
		if fd, ok := d.(*ast.FuncDecl); ok && fd.Recv == nil && fd.Name.Name == "ResolveEndpoints" && fd.Body != nil {
			resolverBody = fd.Body
			break
		}
	}
	if resolverBody == nil {
		t.Fatal("ResolveEndpoints function not found in resolver source")
	}
	resolverHosts := map[string]bool{}
	ast.Inspect(resolverBody, func(n ast.Node) bool {
		lit, ok := n.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		v, err := strconv.Unquote(lit.Value)
		if err != nil || !strings.HasPrefix(v, "https://") {
			return true
		}
		// Parse instead of prefix-stripping so a resolver URL that ever gains a
		// path component still compares by bare host against forbiddenHosts.
		u, err := url.Parse(v)
		if err != nil || u.Host == "" {
			return true
		}
		resolverHosts[u.Host] = true
		return true
	})

	guardHosts := map[string]bool{}
	for _, h := range forbiddenHosts {
		guardHosts[h] = true
	}
	for h := range resolverHosts {
		if !guardHosts[h] {
			t.Errorf("resolver host %q is not in the guard's forbidden list", h)
		}
	}
	for h := range guardHosts {
		if !resolverHosts[h] {
			t.Errorf("guard forbids %q which the resolver does not define", h)
		}
	}
}

// Dot-import rejection and case-insensitive literal matching.
func TestScanRepoDotImportAndCase(t *testing.T) {
	root := t.TempDir()
	// Positive: dot-import of the SDK root package.
	writeFile(t, root, "shortcuts/a/dot.go",
		"package a\n\nimport . \"github.com/larksuite/oapi-sdk-go/v3\"\n\nvar d = FeishuBaseUrl\n")
	// Positive: uppercase host literal.
	writeFile(t, root, "shortcuts/b/upper.go",
		"package b\n\nvar u = \"https://OPEN.FEISHU.CN/api\"\n")
	// Negative: dot-import of an SDK subpackage is out of the globals' scope.
	writeFile(t, root, "shortcuts/c/sub.go",
		"package c\n\nimport . \"github.com/larksuite/oapi-sdk-go/v3/ws\"\n\nvar s = 1\n")

	vs, err := ScanRepo(root)
	if err != nil {
		t.Fatal(err)
	}
	requireEnforced(t, vs)
	if len(vs) != 2 {
		t.Fatalf("got %d violations, want 2: %+v", len(vs), vs)
	}
	files := map[string]bool{}
	for _, v := range vs {
		files[filepath.Base(v.File)] = true
	}
	if !files["dot.go"] || !files["upper.go"] {
		t.Errorf("violations in %v, want dot.go and upper.go", files)
	}
}

// The resolver file is scoped per-function: a hardcoded host outside the
// ResolveEndpoints body is rejected.
func TestScanRepoResolverFunctionScope(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "internal/core/types.go",
		"package core\n\nfunc ResolveEndpoints(b string) string {\n\treturn \"https://open.feishu.cn\"\n}\n\nfunc bypass() string { return \"https://open.feishu.cn\" }\n\ntype localResolver struct{}\nfunc (localResolver) ResolveEndpoints() string { return \"https://open.feishu.cn\" }\n")

	vs, err := ScanRepo(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(vs) != 2 {
		t.Fatalf("got %d violations, want 2 (helper and receiver method): %+v", len(vs), vs)
	}
	for _, v := range vs {
		if filepath.Base(v.File) != "types.go" {
			t.Errorf("violation in %q, want types.go", v.File)
		}
	}
}

// Escape sequences cannot hide a host: literals are unquoted before matching.
func TestScanRepoEscapedLiteral(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "internal/e/e.go",
		"package e\n\nvar h = \"https://open.feishu\\u002ecn\"\n")

	vs, err := ScanRepo(root)
	if err != nil {
		t.Fatal(err)
	}
	requireEnforced(t, vs)
	if len(vs) != 1 {
		t.Fatalf("got %d violations, want 1: %+v", len(vs), vs)
	}
}
