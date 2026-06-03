// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package errscontract

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
)

// migratedEnvelopePaths lists the source-tree prefixes that have been migrated
// to the typed errs.* taxonomy. On these paths, constructing a legacy
// output.ExitError / output.ErrDetail envelope literal directly is forbidden —
// call sites must return a typed errs.* error instead. Future domains opt in by
// appending their path prefix here.
var migratedEnvelopePaths = []string{
	"shortcuts/base/",
	"shortcuts/calendar/",
	"shortcuts/drive/",
	"shortcuts/mail/",
	"shortcuts/okr/",
	"shortcuts/task/",
	"shortcuts/whiteboard/",
	"shortcuts/im/",
}

// legacyOutputImportPath is the import path of the package that declares the
// legacy ExitError / ErrDetail envelope types. The rule resolves whatever local
// name (default or alias) this path is bound to in each file, so an aliased
// import cannot bypass the check.
const legacyOutputImportPath = "github.com/larksuite/cli/internal/output"

// CheckNoLegacyEnvelopeLiteral flags direct construction of legacy
// output.ExitError / output.ErrDetail composite literals on migrated paths.
// forbidigo can ban identifiers but not composite literals, so this AST rule
// covers the gap left after a path is migrated to typed errs.* errors.
//
// Path-scoped to migratedEnvelopePaths (mirrors how CheckProblemEmbed restricts
// by path); skips _test.go fixtures. output.ErrBare(...) is a CallExpr, not a
// CompositeLit, so the predicate exit-signal helper is naturally not flagged.
func CheckNoLegacyEnvelopeLiteral(path, src string) []Violation {
	if !isMigratedEnvelopePath(path) || strings.HasSuffix(path, "_test.go") {
		return nil
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, src, parser.ParseComments)
	if err != nil {
		return nil
	}
	// Resolve the local name(s) bound to the legacy output import path. A file
	// may bind it as the default `output`, an alias (`legacy "...output"`), or a
	// dot-import (qualifier becomes ""), in which case ExitError/ErrDetail appear
	// as bare unqualified idents.
	localNames, dotImported := resolveLegacyOutputNames(file)
	var out []Violation
	ast.Inspect(file, func(n ast.Node) bool {
		lit, ok := n.(*ast.CompositeLit)
		if !ok {
			return true
		}
		if name, ok := legacyEnvelopeTypeName(lit.Type, localNames, dotImported); ok {
			out = append(out, Violation{
				Rule:    "no_legacy_envelope_literal",
				Action:  ActionReject,
				File:    path,
				Line:    fset.Position(lit.Pos()).Line,
				Message: "direct construction of legacy output." + name + " is forbidden on migrated paths; return a typed errs.* error (output.ErrBare remains allowed for predicate exit signals)",
				Suggestion: "replace the &output." + name + "{...} literal with a typed errs.* constructor " +
					"(e.g. errs.NewValidationError / errs.NewAPIError / errs.NewNetworkError)",
			})
		}
		return true
	})
	return out
}

// isMigratedEnvelopePath reports whether path falls under any migrated path
// prefix in migratedEnvelopePaths.
func isMigratedEnvelopePath(path string) bool {
	p := strings.ReplaceAll(path, "\\", "/")
	for _, prefix := range migratedEnvelopePaths {
		if strings.HasPrefix(p, prefix) || strings.Contains(p, "/"+prefix) {
			return true
		}
	}
	return false
}

// resolveLegacyOutputNames walks the file's import declarations and returns the
// set of local names bound to legacyOutputImportPath, plus whether the path was
// dot-imported. Default imports bind the package's own name ("output"); aliased
// imports bind the alias; dot-imports bind names into the file scope.
func resolveLegacyOutputNames(file *ast.File) (map[string]struct{}, bool) {
	names := make(map[string]struct{})
	dotImported := false
	for _, imp := range file.Imports {
		if imp.Path == nil {
			continue
		}
		p := strings.Trim(imp.Path.Value, "`\"")
		if p != legacyOutputImportPath {
			continue
		}
		switch {
		case imp.Name == nil:
			// Default import: local name is the package name "output".
			names["output"] = struct{}{}
		case imp.Name.Name == ".":
			dotImported = true
		case imp.Name.Name == "_":
			// Blank import cannot reference the types; ignore.
		default:
			names[imp.Name.Name] = struct{}{}
		}
	}
	return names, dotImported
}

// legacyEnvelopeTypeName reports whether a composite-literal Type names the
// legacy ExitError / ErrDetail envelope and returns the bare type name. It
// matches a qualified selector (pkg.ExitError) when pkg is one of the resolved
// local names for the legacy output import, and — when the package was
// dot-imported — also matches a bare unqualified ExitError / ErrDetail ident.
func legacyEnvelopeTypeName(expr ast.Expr, localNames map[string]struct{}, dotImported bool) (string, bool) {
	if sel, ok := expr.(*ast.SelectorExpr); ok {
		x, ok := sel.X.(*ast.Ident)
		if !ok || sel.Sel == nil {
			return "", false
		}
		if _, bound := localNames[x.Name]; !bound {
			return "", false
		}
		return matchLegacyEnvelopeName(sel.Sel.Name)
	}
	if dotImported {
		if ident, ok := expr.(*ast.Ident); ok {
			return matchLegacyEnvelopeName(ident.Name)
		}
	}
	return "", false
}

// matchLegacyEnvelopeName returns the name when it is one of the legacy
// envelope type names.
func matchLegacyEnvelopeName(name string) (string, bool) {
	switch name {
	case "ExitError", "ErrDetail":
		return name, true
	}
	return "", false
}
