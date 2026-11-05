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

func TestHTMLAffordanceRouting(t *testing.T) {
	previous := mdSource
	t.Cleanup(func() { SetSource(previous) })
	SetSource(os.DirFS("../../affordance"))
	for _, verb := range []string{"publish", "fetch"} {
		t.Run(verb, func(t *testing.T) {
			raw, ok := For(apicatalog.Catalog{}, "html", "+"+verb)
			if !ok {
				t.Fatal("missing shortcut affordance")
			}
			guidance, ok := (meta.Method{Affordance: raw}).ParsedAffordance()
			if !ok {
				t.Fatal("invalid shortcut affordance")
			}
			if len(guidance.Examples) == 0 {
				t.Fatal("missing runnable examples")
			}
			hasTokenExample := false
			for _, example := range guidance.Examples {
				if strings.Contains(example.Command, " --token ") {
					hasTokenExample = true
				}
				if !strings.HasPrefix(example.Command, "lark-cli html +"+verb+" ") {
					t.Errorf("unexpected command %q", example.Command)
				}
			}
			if verb == "publish" && !hasTokenExample {
				t.Fatal("missing target token publication example")
			}
			if !strings.Contains(string(raw), "lark-html/references/lark-html-"+verb+".md") {
				t.Fatal("missing skill reference")
			}
		})
	}
}
