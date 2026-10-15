// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package affordance

import (
	"os"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/apicatalog"
	"github.com/larksuite/cli/internal/meta"
)

func TestAppsMCPGuidance(t *testing.T) {
	previous := mdSource
	t.Cleanup(func() { SetSource(previous) })
	SetSource(os.DirFS("../../affordance"))
	for command, wantExample := range map[string]string{
		"+mcp-get":        "lark-cli apps +mcp-get --app-id <app_id> --as user --format pretty",
		"+mcp-key-create": "lark-cli apps +mcp-key-create --app-id <app_id> --as user",
	} {
		raw, ok := For(apicatalog.Catalog{}, "apps", command)
		if !ok {
			t.Fatalf("missing %s", command)
		}
		a, ok := (meta.Method{Affordance: raw}).ParsedAffordance()
		if !ok {
			t.Fatal("invalid guidance")
		}
		if len(a.Examples) != 1 || a.Examples[0].Command != wantExample {
			t.Fatalf("incorrect example: %v", a.Examples)
		}
		if command == "+mcp-get" {
			// Keep the access model and the recovery path for a caller who is not a developer.
			guidance := strings.Join(append(append([]string{}, a.Prerequisites...), a.Tips...), "\n")
			for _, want := range []string{"online visible scope", "once a developer has enabled MCP", "40906", "ask a developer to enable MCP", "+mcp-key-create", "--format pretty"} {
				if !strings.Contains(guidance, want) {
					t.Fatalf("+mcp-get guidance lost %q:\n%s", want, guidance)
				}
			}
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
