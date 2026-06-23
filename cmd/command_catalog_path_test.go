// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"reflect"
	"testing"

	"github.com/spf13/cobra"
)

// TestCommandCatalogPath pins that the auth-hint path reconstruction inverts the
// service command tree for any depth — flat dotted resources AND genuinely
// nested resources — so it round-trips through apicatalog.Resolve instead of
// assuming a fixed root->service->resource->method shape.
func TestCommandCatalogPath(t *testing.T) {
	chain := func(names ...string) *cobra.Command {
		var parent, leaf *cobra.Command
		for _, n := range names {
			c := &cobra.Command{Use: n}
			if parent != nil {
				parent.AddCommand(c)
			}
			parent = c
			leaf = c
		}
		return leaf
	}

	tests := []struct {
		name string
		leaf *cobra.Command
		want []string
	}{
		{"flat dotted resource", chain("lark-cli", "im", "chat.members", "create"), []string{"im", "chat.members", "create"}},
		{"nested resources", chain("lark-cli", "im", "spaces", "items", "get"), []string{"im", "spaces", "items", "get"}},
		{"service level", chain("lark-cli", "im"), []string{"im"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := commandCatalogPath(tt.leaf); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("commandCatalogPath = %v, want %v", got, tt.want)
			}
		})
	}

	// The root command (no parent) has no catalog path.
	if got := commandCatalogPath(&cobra.Command{Use: "lark-cli"}); len(got) != 0 {
		t.Errorf("root path = %v, want empty", got)
	}
}
