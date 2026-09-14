// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package affordance

import (
	"os"
	"testing"

	"github.com/larksuite/cli/internal/apicatalog"
	"github.com/larksuite/cli/internal/meta"
)

func TestAppsMCPGuidance(t *testing.T) {
	previous := mdSource
	t.Cleanup(func() { SetSource(previous) })
	SetSource(os.DirFS("../../affordance"))
	for _, command := range []string{"+mcp-get", "+mcp-key-create"} {
		raw, ok := For(apicatalog.Catalog{}, "apps", command)
		if !ok {
			t.Fatalf("missing %s", command)
		}
		a, ok := (meta.Method{Affordance: raw}).ParsedAffordance()
		if !ok {
			t.Fatal("invalid guidance")
		}
		if len(a.Examples) != 1 || a.Examples[0].Command != "lark-cli apps "+command+" --app-id app_example --as user" {
			t.Fatalf("incorrect example: %v", a.Examples)
		}
		found := false
		for _, s := range a.Skills {
			if s == "lark-apps/references/lark-apps-mcp.md" {
				found = true
			}
		}
		if !found {
			t.Fatal("missing MCP skill reference")
		}
	}
}
