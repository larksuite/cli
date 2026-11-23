// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package main

import (
	"slices"
	"testing"

	"github.com/larksuite/cli/internal/flagalias"
	"github.com/spf13/cobra"
)

func TestCommandFromCobraExportsAliasesAsFirstClassMetadata(t *testing.T) {
	root := &cobra.Command{Use: "lark-cli"}
	cmd := &cobra.Command{Use: "+messages"}
	cmd.Flags().String("order", "desc", "message order")
	root.AddCommand(cmd)
	if err := flagalias.Bind(cmd, []flagalias.Spec{{Canonical: "order", Aliases: []string{"sort", "sort-order"}}}); err != nil {
		t.Fatal(err)
	}

	entry := commandFromCobra(cmd, nil)
	flag := findFlag(entry.Flags, "order")
	if flag == nil {
		t.Fatal("manifest is missing canonical --order")
	}
	if !slices.Equal(flag.Aliases, []string{"sort", "sort-order"}) {
		t.Fatalf("manifest aliases = %v", flag.Aliases)
	}
	if _, leaked := flag.Annotations[flagalias.AnnotationAliases]; leaked {
		t.Fatalf("internal alias annotation leaked into manifest: %#v", flag.Annotations)
	}
	if findFlag(entry.Flags, "sort") != nil || findFlag(entry.Flags, "sort-order") != nil {
		t.Fatalf("aliases were exported as independent flags: %#v", entry.Flags)
	}
}
