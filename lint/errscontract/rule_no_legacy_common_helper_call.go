// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package errscontract

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
)

// migratedCommonHelperPaths lists source-tree prefixes whose command validation
// has migrated to typed errs.* envelopes. On these paths, calls to common's
// legacy validation/save helpers are forbidden; callers must use the typed
// common replacements or construct an errs.* typed error directly.
var migratedCommonHelperPaths = []string{
	"shortcuts/drive/",
	"shortcuts/mail/",
}

const commonImportPath = "github.com/larksuite/cli/shortcuts/common"

var legacyCommonHelperReplacements = map[string]string{
	"FlagErrorf":              "common.ValidationErrorf",
	"MutuallyExclusive":       "common.MutuallyExclusiveTyped",
	"AtLeastOne":              "common.AtLeastOneTyped",
	"ExactlyOne":              "common.ExactlyOneTyped",
	"ValidatePageSize":        "common.ValidatePageSizeTyped",
	"ValidateChatID":          "common.ValidateChatIDTyped",
	"ValidateUserID":          "common.ValidateUserIDTyped",
	"ValidateSafePath":        "common.ValidateSafePathTyped",
	"RejectDangerousChars":    "common.RejectDangerousCharsTyped",
	"WrapInputStatError":      "common.WrapInputStatErrorTyped",
	"WrapSaveErrorByCategory": "common.WrapSaveErrorTyped",
	"ResolveOpenIDs":          "common.ResolveOpenIDsTyped",
	"HandleApiResult":         "runtime.CallAPITyped",
}

// CheckNoLegacyCommonHelperCall flags any reference to common's legacy helper
// APIs on migrated paths — direct calls and function-value references alike,
// so `f := common.FlagErrorf; f(...)` cannot slip past the guard. These
// helpers return legacy output envelopes or bare errors, so migrated domains
// should use their typed-aware replacements.
func CheckNoLegacyCommonHelperCall(path, src string) []Violation {
	if !isMigratedCommonHelperPath(path) || strings.HasSuffix(path, "_test.go") {
		return nil
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, src, parser.ParseComments)
	if err != nil {
		return nil
	}
	localNames, dotImported := resolveCommonNames(file)
	var out []Violation
	report := func(pos token.Pos, name, replacement string) {
		out = append(out, Violation{
			Rule:       "no_legacy_common_helper_call",
			Action:     ActionReject,
			File:       path,
			Line:       fset.Position(pos).Line,
			Message:    "common." + name + " returns a legacy error shape and is forbidden on migrated paths",
			Suggestion: "replace common." + name + " with " + replacement + " or a typed errs.* constructor",
		})
	}
	// Pass 1: qualified references (common.X / alias.X). Record every
	// selector field so the dot-import pass below never mistakes another
	// package's same-named field for a common helper.
	selFields := make(map[*ast.Ident]struct{})
	ast.Inspect(file, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		selFields[sel.Sel] = struct{}{}
		x, ok := sel.X.(*ast.Ident)
		if !ok {
			return true
		}
		if _, bound := localNames[x.Name]; !bound {
			return true
		}
		if replacement, ok := legacyCommonHelperReplacements[sel.Sel.Name]; ok {
			report(sel.Pos(), sel.Sel.Name, replacement)
		}
		return true
	})
	// Pass 2: unqualified references under a dot import.
	if dotImported {
		ast.Inspect(file, func(n ast.Node) bool {
			ident, ok := n.(*ast.Ident)
			if !ok {
				return true
			}
			if _, isField := selFields[ident]; isField {
				return true
			}
			if replacement, ok := legacyCommonHelperReplacements[ident.Name]; ok {
				report(ident.Pos(), ident.Name, replacement)
			}
			return true
		})
	}
	return out
}

func isMigratedCommonHelperPath(path string) bool {
	p := strings.ReplaceAll(path, "\\", "/")
	for _, prefix := range migratedCommonHelperPaths {
		if strings.HasPrefix(p, prefix) || strings.Contains(p, "/"+prefix) {
			return true
		}
	}
	return false
}

func resolveCommonNames(file *ast.File) (map[string]struct{}, bool) {
	names := make(map[string]struct{})
	dotImported := false
	for _, imp := range file.Imports {
		if imp.Path == nil {
			continue
		}
		p := strings.Trim(imp.Path.Value, "`\"")
		if p != commonImportPath {
			continue
		}
		switch {
		case imp.Name == nil:
			names["common"] = struct{}{}
		case imp.Name.Name == ".":
			dotImported = true
		case imp.Name.Name == "_":
		default:
			names[imp.Name.Name] = struct{}{}
		}
	}
	return names, dotImported
}
