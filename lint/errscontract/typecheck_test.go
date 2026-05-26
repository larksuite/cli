// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package errscontract

import (
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"testing"
)

// TestTypedScope_RejectsForeignSubtypeConst proves that the typed-resolution
// pass rejects a Subtype-named constant declared in a non-errs package, even
// when the constant's NAME matches a declared errs Subtype. This is the
// behavior selector-name matching alone could not deliver.
//
// The test exercises collectSubtypeConsts and ResolveSubtypeIdent directly
// against a synthetic types.Package. A full ScanRepo integration test would
// need a synthetic go.mod whose module path happens to be
// github.com/larksuite/cli — which would conflict with the real repo — so
// we exercise the resolution helpers directly here.
func TestTypedScope_RejectsForeignSubtypeConst(t *testing.T) {
	// Synthesize what go/packages would have produced: an errs package
	// holding the canonical Subtype type plus SubtypeMissingScope const,
	// and a foreign consumer package that re-defines its own Subtype type
	// with an identically-named SubtypeMissingScope const.
	src := `package fakeerrs

type Subtype string

const SubtypeMissingScope Subtype = "missing_scope"
`
	fset := token.NewFileSet()
	errsFile, err := parser.ParseFile(fset, "fakeerrs/subtypes.go", src, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse fakeerrs: %v", err)
	}
	conf := &types.Config{Importer: importer.Default()}
	errsPkg, err := conf.Check(errsPkgPath, fset, []*ast.File{errsFile}, nil)
	if err != nil {
		t.Fatalf("type-check fakeerrs: %v", err)
	}

	foreignSrc := `package foreign

type Subtype string

const SubtypeMissingScope Subtype = "fraudulent"
`
	foreignFile, err := parser.ParseFile(fset, "foreign/foreign.go", foreignSrc, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse foreign: %v", err)
	}
	foreignPkg, err := conf.Check("example.com/foreign", fset, []*ast.File{foreignFile}, nil)
	if err != nil {
		t.Fatalf("type-check foreign: %v", err)
	}

	scope := &TypedScope{
		typedFiles:        map[string]*types.Info{},
		errsSubtypeConsts: map[string]*types.Const{},
	}

	// collectSubtypeConsts should pick up SubtypeMissingScope from the
	// canonical errs package but NOT from the foreign one (different pkg).
	collectSubtypeConsts(errsPkg, scope.errsSubtypeConsts)
	collectSubtypeConsts(foreignPkg, scope.errsSubtypeConsts)
	if _, ok := scope.errsSubtypeConsts["SubtypeMissingScope"]; !ok {
		t.Fatalf("expected SubtypeMissingScope to be captured from errs")
	}
	if got := scope.errsSubtypeConsts["SubtypeMissingScope"].Pkg().Path(); got != errsPkgPath {
		t.Fatalf("captured const came from %q, want %q", got, errsPkgPath)
	}

	// Now type-check a consumer file that uses BOTH constants, and verify
	// ResolveSubtypeIdent accepts the errs reference and rejects the foreign
	// one with the (resolved=false, ok=true) pair CheckDeclaredSubtype treats as REJECT.
	consumerSrc := `package consumer

import (
	errs "` + errsPkgPath + `"
	foreign "example.com/foreign"
)

var _ errs.Subtype = errs.SubtypeMissingScope
var _ foreign.Subtype = foreign.SubtypeMissingScope
`
	consumerFile, err := parser.ParseFile(fset, "consumer/x.go", consumerSrc, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse consumer: %v", err)
	}
	imp := &fakeImporter{m: map[string]*types.Package{
		errsPkgPath:           errsPkg,
		"example.com/foreign": foreignPkg,
	}}
	conf2 := &types.Config{Importer: imp}
	info := &types.Info{
		Uses: map[*ast.Ident]types.Object{},
	}
	if _, err := conf2.Check("example.com/consumer", fset, []*ast.File{consumerFile}, info); err != nil {
		t.Fatalf("type-check consumer: %v", err)
	}
	scope.typedFiles["consumer/x.go"] = info

	// Walk the consumer file to find the SubtypeMissingScope selectors and
	// drive ResolveSubtypeIdent against each one.
	var goodIdent, foreignIdent *ast.Ident
	ast.Inspect(consumerFile, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "SubtypeMissingScope" {
			return true
		}
		obj := info.Uses[sel.Sel]
		if obj == nil {
			return true
		}
		switch obj.Pkg().Path() {
		case errsPkgPath:
			goodIdent = sel.Sel
		case "example.com/foreign":
			foreignIdent = sel.Sel
		}
		return true
	})
	if goodIdent == nil || foreignIdent == nil {
		t.Fatalf("did not find both selector idents in consumer source")
	}

	resolved, ok := scope.ResolveSubtypeIdent("consumer/x.go", goodIdent)
	if !ok {
		t.Fatalf("errs reference should resolve via type info")
	}
	if !resolved {
		t.Errorf("errs.SubtypeMissingScope should resolve=true; got resolved=false")
	}

	resolved, ok = scope.ResolveSubtypeIdent("consumer/x.go", foreignIdent)
	if !ok {
		t.Fatalf("foreign reference should still produce ok=true (so CheckDeclaredSubtype can reject)")
	}
	if resolved {
		t.Errorf("foreign.SubtypeMissingScope must NOT resolve=true; selector-name matching alone would have falsely accepted it")
	}
}

// fakeImporter is a minimal types.Importer used by the test to satisfy
// cross-package imports without going through go/packages.
type fakeImporter struct {
	m map[string]*types.Package
}

func (f *fakeImporter) Import(path string) (*types.Package, error) {
	if p, ok := f.m[path]; ok {
		return p, nil
	}
	return importer.Default().Import(path)
}

// TestTypedScope_FallsBackWhenDisabled documents the no-op contract: when
// the scope is empty (loader failed or the unit-test API was used), the
// production walker falls back to AST-only resolution. ResolveSubtypeIdent
// must signal ok=false so the caller knows to consult the nameset path.
func TestTypedScope_FallsBackWhenDisabled(t *testing.T) {
	var scope *TypedScope
	if scope.Enabled() {
		t.Fatalf("nil scope must report Enabled()=false")
	}
	if resolved, ok := scope.ResolveSubtypeIdent("x.go", &ast.Ident{Name: "SubtypeFoo"}); resolved || ok {
		t.Fatalf("disabled scope must return (false,false); got (%v,%v)", resolved, ok)
	}

	empty := &TypedScope{}
	if empty.Enabled() {
		t.Fatalf("empty scope must report Enabled()=false")
	}
}

// TestTypedScope_EnabledRequiresBothTypedFilesAndSubtypeConsts pins the
// tightened Enabled() contract: half-loaded scopes (typed files indexed
// but errs.Subtype const set empty, or vice versa) must report disabled
// so callers fall back to AST instead of over-rejecting every selector.
func TestTypedScope_EnabledRequiresBothTypedFilesAndSubtypeConsts(t *testing.T) {
	onlyFiles := &TypedScope{
		typedFiles:        map[string]*types.Info{"x.go": {Uses: map[*ast.Ident]types.Object{}}},
		errsSubtypeConsts: map[string]*types.Const{},
	}
	if onlyFiles.Enabled() {
		t.Errorf("scope with files but no errs subtype consts must be disabled — typed pass would over-reject everything")
	}

	onlyConsts := &TypedScope{
		typedFiles:        map[string]*types.Info{},
		errsSubtypeConsts: map[string]*types.Const{"SubtypeFoo": nil},
	}
	if onlyConsts.Enabled() {
		t.Errorf("scope with consts but no typed files must be disabled — no per-file lookup is possible")
	}

	both := &TypedScope{
		typedFiles:        map[string]*types.Info{"x.go": {Uses: map[*ast.Ident]types.Object{}}},
		errsSubtypeConsts: map[string]*types.Const{"SubtypeFoo": nil},
	}
	if !both.Enabled() {
		t.Errorf("scope with both populated must be enabled")
	}
}

// TestTypedScope_RejectsForeignNonPrefixedConst pins the A+ behavior of
// Refinement 1: even a constant whose name does NOT begin with "Subtype"
// is rejected when assigned to a Subtype: slot, because it does not
// resolve to a declared errs.Subtype constant. The legacy AST path was
// name-gated on the "Subtype" prefix and silently accepted such
// references.
func TestTypedScope_RejectsForeignNonPrefixedConst(t *testing.T) {
	fset := token.NewFileSet()

	// Canonical errs package with a real Subtype const.
	errsSrc := `package fakeerrs

type Subtype string

const SubtypeMissingScope Subtype = "missing_scope"
`
	errsFile, err := parser.ParseFile(fset, "fakeerrs/subtypes.go", errsSrc, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse errs: %v", err)
	}
	conf := &types.Config{Importer: importer.Default()}
	errsPkg, err := conf.Check(errsPkgPath, fset, []*ast.File{errsFile}, nil)
	if err != nil {
		t.Fatalf("type-check errs: %v", err)
	}

	// Foreign package declaring a constant named MyKind (NOT Subtype-prefixed).
	// Under the legacy AST gate this would have been ignored entirely.
	foreignSrc := `package foreign

type Kind string

const MyKind Kind = "wrong"
`
	foreignFile, err := parser.ParseFile(fset, "foreign/foreign.go", foreignSrc, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse foreign: %v", err)
	}
	foreignPkg, err := conf.Check("example.com/foreign", fset, []*ast.File{foreignFile}, nil)
	if err != nil {
		t.Fatalf("type-check foreign: %v", err)
	}

	scope := &TypedScope{
		typedFiles:        map[string]*types.Info{},
		errsSubtypeConsts: map[string]*types.Const{},
	}
	collectSubtypeConsts(errsPkg, scope.errsSubtypeConsts)

	// Consumer references foreign.MyKind so the type-checker records it
	// in Info.Uses; we then drive ResolveSubtypeIdent against that ident.
	consumerSrc := `package consumer

import foreign "example.com/foreign"

var _ foreign.Kind = foreign.MyKind
`
	consumerFile, err := parser.ParseFile(fset, "consumer/x.go", consumerSrc, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse consumer: %v", err)
	}
	imp := &fakeImporter{m: map[string]*types.Package{
		errsPkgPath:           errsPkg,
		"example.com/foreign": foreignPkg,
	}}
	conf2 := &types.Config{Importer: imp}
	info := &types.Info{Uses: map[*ast.Ident]types.Object{}}
	if _, err := conf2.Check("example.com/consumer", fset, []*ast.File{consumerFile}, info); err != nil {
		t.Fatalf("type-check consumer: %v", err)
	}
	scope.typedFiles["consumer/x.go"] = info

	var foreignIdent *ast.Ident
	ast.Inspect(consumerFile, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "MyKind" {
			return true
		}
		foreignIdent = sel.Sel
		return true
	})
	if foreignIdent == nil {
		t.Fatalf("did not find foreign.MyKind selector in consumer source")
	}

	resolved, ok := scope.ResolveSubtypeIdent("consumer/x.go", foreignIdent)
	if !ok {
		t.Fatalf("foreign non-prefixed const must produce ok=true so CheckDeclaredSubtype can reject; got ok=false")
	}
	if resolved {
		t.Errorf("foreign.MyKind (non-Subtype-prefixed) must NOT resolve=true; legacy AST gate would have skipped it silently")
	}

	// Drive the classifier directly to prove end-to-end rejection.
	c, handled := classifyConstViaTypes(foreignIdent, "consumer/x.go", scope)
	if !handled {
		t.Fatalf("typed classifier must handle resolved foreign const; got handled=false")
	}
	if c.action != ActionReject {
		t.Errorf("classifier action = %q, want REJECT", c.action)
	}
}
