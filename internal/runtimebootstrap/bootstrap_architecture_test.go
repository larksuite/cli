// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package runtimebootstrap

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/vfs"
)

const externalCredentialImplementation = "github.com/larksuite/cli/internal/externalcredential"

func TestOnlyExtendedSelectorImportsExternalCredentialImplementation(t *testing.T) {
	entries, err := vfs.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}

	var extendedSelectorImportsAdapter bool
	for _, entry := range entries {
		if entry.IsDir() ||
			!strings.HasSuffix(entry.Name(), ".go") ||
			strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		path := filepath.Join(".", entry.Name())
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
			if importPath != externalCredentialImplementation {
				continue
			}
			if entry.Name() != "selector_extended.go" {
				t.Errorf("%s imports the external credential implementation; Standard bootstrap must stay product-neutral", path)
				continue
			}
			extendedSelectorImportsAdapter = true
		}
	}
	if !extendedSelectorImportsAdapter {
		t.Fatal("selector_extended.go must remain the explicit external credential composition edge")
	}
}
