// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package errscontract

import (
	"go/ast"
	"go/types"
	"path/filepath"
	"strings"

	"golang.org/x/tools/go/packages"
)

// errsPkgPath is the canonical import path of the typed-errors package.
// CheckDeclaredSubtype's typed-resolution pass verifies that a `pkg.SubtypeXxx` selector's
// resolved object belongs to this exact package — selector-name matching
// alone would have falsely accepted an identically-named constant from a
// foreign package.
const errsPkgPath = "github.com/larksuite/cli/errs"

// TypedScope captures the workspace-wide type information used by CheckDeclaredSubtype's
// typed-resolution pass. The zero value is a no-op (typed pass disabled);
// LoadTypedScope populates it.
//
// Once populated:
//   - typedFiles maps an absolute Go file path to the *types.Info of its
//     package. The walker uses it to resolve selector / ident references on
//     a per-file basis: Info.Uses[ident] yields the *types.Object pointed
//     at by that identifier, including the originating package.
//   - errsSubtypeConsts holds the typed Subtype constants declared in the
//     errs package. A resolved object is a "real" Subtype only when it
//     appears in this set.
type TypedScope struct {
	typedFiles        map[string]*types.Info
	errsSubtypeConsts map[string]*types.Const
}

// Enabled reports whether the typed-resolution pass can answer questions
// about errs.Subtype references. It requires both:
//
//   - typedFiles non-empty (go/packages.Load produced usable type info);
//   - errsSubtypeConsts non-empty (the canonical errs.Subtype const set
//     was actually discovered).
//
// Requiring both avoids the half-loaded failure mode where typed-file
// indexing succeeded but the errs package was not visited — every
// resolution attempt would then claim "foreign const" and over-reject.
// Callers fall back to AST-only resolution when Enabled returns false.
func (s *TypedScope) Enabled() bool {
	if s == nil {
		return false
	}
	return len(s.typedFiles) > 0 && len(s.errsSubtypeConsts) > 0
}

// LookupFileInfo returns the per-package types.Info covering the given Go
// file (path matching the absolute path used during the load). Callers use
// it to resolve *ast.Ident → *types.Object via Info.Uses.
func (s *TypedScope) LookupFileInfo(absPath string) (*types.Info, bool) {
	if s == nil {
		return nil, false
	}
	info, ok := s.typedFiles[filepath.Clean(absPath)]
	return info, ok
}

// LoadTypedScope loads the workspace rooted at root with full type
// information and returns a scope ready for CheckDeclaredSubtype typed resolution. A
// non-nil error reports an unrecoverable failure (the loader could not
// even start); a successful return with Enabled() == false indicates the
// loader ran but produced no usable type info (e.g. the errs package was
// missing) — in which case the caller should fall back silently to the
// AST-only path.
func LoadTypedScope(root string) (*TypedScope, error) {
	cfg := &packages.Config{
		Mode: packages.NeedName |
			packages.NeedFiles |
			packages.NeedCompiledGoFiles |
			packages.NeedImports |
			packages.NeedDeps |
			packages.NeedTypes |
			packages.NeedSyntax |
			packages.NeedTypesInfo,
		Dir:   root,
		Tests: false,
	}
	pkgs, err := packages.Load(cfg, "./...")
	if err != nil {
		return nil, err
	}

	scope := &TypedScope{
		typedFiles:        map[string]*types.Info{},
		errsSubtypeConsts: map[string]*types.Const{},
	}

	packages.Visit(pkgs, nil, func(p *packages.Package) {
		if p == nil || p.TypesInfo == nil {
			return
		}
		// Index file → TypesInfo for the walker.
		for _, f := range p.CompiledGoFiles {
			scope.typedFiles[filepath.Clean(f)] = p.TypesInfo
		}
		// Capture declared Subtype constants from the canonical errs package
		// so CheckDeclaredSubtype can reject selectors that resolve to a foreign-package
		// const sharing the same name.
		if p.PkgPath == errsPkgPath && p.Types != nil {
			collectSubtypeConsts(p.Types, scope.errsSubtypeConsts)
		}
	})
	return scope, nil
}

// collectSubtypeConsts scans a *types.Package for exported constants of
// type errs.Subtype whose name starts with "Subtype" and records them by
// name. The "Subtype" name prefix is enforced so the helper aligns with
// the CheckDeclaredSubtype AST pass and avoids matching the underlying `Subtype` type
// definition itself.
func collectSubtypeConsts(pkg *types.Package, into map[string]*types.Const) {
	if pkg == nil || pkg.Scope() == nil {
		return
	}
	for _, name := range pkg.Scope().Names() {
		if !strings.HasPrefix(name, "Subtype") || name == "Subtype" {
			continue
		}
		obj := pkg.Scope().Lookup(name)
		c, ok := obj.(*types.Const)
		if !ok {
			continue
		}
		// Verify the constant's type is errs.Subtype (not e.g. a foreign
		// "Subtype"-named string alias re-exported from this package).
		named, ok := c.Type().(*types.Named)
		if !ok {
			continue
		}
		if named.Obj() == nil || named.Obj().Name() != "Subtype" ||
			named.Obj().Pkg() == nil || named.Obj().Pkg().Path() != errsPkgPath {
			continue
		}
		into[name] = c
	}
}

// ResolveSubtypeIdent inspects the identifier used as the value of a
// `Subtype:` composite-literal field and reports the typed-scope verdict
// via the (resolved, ok) tuple:
//
//   - (true, true): the identifier is a declared errs.Subtype constant.
//     The AST pass may skip its nameset check for this site.
//   - (false, true): definitive rejection — the identifier resolved to a
//     constant in a non-errs package, or to a non-Subtype constant inside
//     errs. Caller MUST NOT fall back to AST resolution; CheckDeclaredSubtype should
//     reject this site.
//   - (false, false): typed scope cannot decide (scope disabled, no file
//     info, sel==nil, no type info for the identifier, or the resolved
//     object is not a constant). Caller defers to AST-only resolution.
func (s *TypedScope) ResolveSubtypeIdent(absPath string, sel *ast.Ident) (resolved, ok bool) {
	if !s.Enabled() {
		return false, false
	}
	info, found := s.LookupFileInfo(absPath)
	if !found || info == nil || sel == nil {
		return false, false
	}
	obj, found := info.Uses[sel]
	if !found || obj == nil {
		// No type info for this identifier — caller falls back to AST.
		return false, false
	}
	c, isConst := obj.(*types.Const)
	if !isConst {
		return false, false
	}
	if c.Pkg() == nil || c.Pkg().Path() != errsPkgPath {
		// Foreign-package constant assigned to a Subtype: slot. Reject —
		// the caller routes ALL selectors through this path regardless of
		// name shape, so this branch fires for both `foreign.SubtypeFoo`
		// and `foreign.MyKind`.
		return false, true
	}
	if _, declared := s.errsSubtypeConsts[c.Name()]; !declared {
		// In the errs package but not a Subtype const (defense-in-depth).
		return false, true
	}
	return true, true
}
