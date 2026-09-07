// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

// Business authors use extension/command. common owns the existing runner and
// may expose only a token-gated internal handshake; a second model or compiler
// here would create another compatibility contract for the same command.
func TestCommonDoesNotExportSecondCommandAuthoringSurface(t *testing.T) {
	forbidden := map[string]struct{}{
		"JSONValue": {}, "Definition": {}, "Define": {}, "Hooks": {}, "Renderer": {},
		"CommandMetadata": {}, "Identity": {}, "Risk": {}, "AuthorizationDefinition": {},
		"IdentityAuthorization": {}, "ConditionalScope": {}, "ScopeRequirement": {},
		"InputDefinition": {}, "InputField": {}, "InputDefault": {}, "CLIInput": {},
		"FlagAlias": {}, "FlagAliasMode": {}, "AliasConflictPolicy": {}, "ValueSource": {},
		"CLIEncoding": {}, "Provided": {}, "Relation": {}, "RelationKind": {},
		"PresenceMode": {}, "RelationStage": {}, "ValueShape": {}, "StringShape": {},
		"BooleanShape": {}, "IntegerShape": {}, "NumberShape": {}, "NullShape": {},
		"ConstShape": {}, "ArrayShape": {}, "ObjectShape": {}, "ValueField": {},
		"OneOfShape": {}, "DataDefinition": {}, "DataField": {}, "OutputDefinition": {},
		"ResultMetaDefinition": {}, "Result": {}, "Success": {}, "Partial": {},
		"OutcomeDefinition": {}, "PartialFailureDefinition": {}, "FailedItemDefinition": {},
		"ArtifactDefinition": {}, "CommandContext": {}, "PaginationOptions": {},
		"ErasedDefinition": {}, "ErasedHooks": {}, "ErasedResult": {},
		"CompileErasedDefinition": {}, "DoTypedAPIJSON": {}, "DoTypedAPIJSONWithOptions": {},
		"CallTypedAPI": {}, "CollectCommandPages": {}, "CloneShortcut": {}, "CloneShortcuts": {},
	}
	forEachCommonProductionDeclaration(t, func(file, name string, exported bool, _ *ast.FieldList) {
		if !exported {
			return
		}
		if _, duplicate := forbidden[name]; duplicate {
			t.Errorf("%s exports duplicate command symbol %s; use extension/command", file, name)
		}
	})
}

func TestCommonCommandBridgeRemainsSealedAndNarrow(t *testing.T) {
	allowed := map[string]bool{
		"CompileCommandDefinition": false,
		"DoHostedAPIJSON":          false,
		"CallHostedAPI":            false,
		"CollectHostedPages":       false,
		"ShortcutSchema":           false,
		"CloneHostedShortcuts":     false,
	}
	forEachCommonProductionDeclaration(t, func(file, name string, exported bool, params *ast.FieldList) {
		if !exported {
			return
		}
		_, audited := allowed[name]
		bridgeFile := strings.HasPrefix(filepath.Base(file), "typed_") || filepath.Base(file) == "clone.go"
		if bridgeFile && !audited {
			t.Errorf("%s exports unexpected command bridge symbol %s", file, name)
			return
		}
		if !audited {
			return
		}
		if !usesCommandBridgeAccess(params) {
			t.Errorf("%s bridge function %s is callable without internal commandbridge access", file, name)
		}
		allowed[name] = true
	})
	for name, found := range allowed {
		if !found {
			t.Errorf("audited command bridge function %s is missing", name)
		}
	}
}

func forEachCommonProductionDeclaration(t *testing.T, visit func(file, name string, exported bool, params *ast.FieldList)) {
	t.Helper()
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, declaration := range parsed.Decls {
			switch value := declaration.(type) {
			case *ast.FuncDecl:
				if value.Recv == nil {
					visit(file, value.Name.Name, value.Name.IsExported(), value.Type.Params)
				}
			case *ast.GenDecl:
				for _, spec := range value.Specs {
					switch item := spec.(type) {
					case *ast.TypeSpec:
						visit(file, item.Name.Name, item.Name.IsExported(), nil)
					case *ast.ValueSpec:
						for _, name := range item.Names {
							visit(file, name.Name, name.IsExported(), nil)
						}
					}
				}
			}
		}
	}
}

func usesCommandBridgeAccess(fields *ast.FieldList) bool {
	if fields == nil {
		return false
	}
	found := false
	ast.Inspect(fields, func(node ast.Node) bool {
		selector, ok := node.(*ast.SelectorExpr)
		if !ok || selector.Sel.Name != "Access" {
			return true
		}
		qualifier, ok := selector.X.(*ast.Ident)
		if ok && qualifier.Name == "commandbridge" {
			found = true
			return false
		}
		return true
	})
	return found
}
