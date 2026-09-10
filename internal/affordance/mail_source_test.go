// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package affordance

import (
	"os"
	"testing"

	"github.com/larksuite/cli/internal/meta"
)

func TestMailAffordanceRoutesSenderReferences(t *testing.T) {
	previousSource := mdSource
	t.Cleanup(func() { SetSource(previousSource) })
	SetSource(os.DirFS("../../affordance"))
	for _, verb := range []string{"list", "get", "set", "delete"} {
		t.Run(verb, func(t *testing.T) {
			raw, ok := For("mail", "+sender-"+verb)
			if !ok {
				t.Fatal("missing sender affordance")
			}
			guidance, ok := (meta.Method{Affordance: raw}).ParsedAffordance()
			if !ok {
				t.Fatal("invalid sender affordance")
			}
			ref := "lark-mail/references/lark-mail-sender-" + verb + ".md"
			for _, skill := range guidance.Skills {
				if skill == ref {
					if _, err := os.Stat("../../skills/" + ref); err != nil {
						t.Fatal(err)
					}
					return
				}
			}
			t.Fatalf("skills=%v, want %s", guidance.Skills, ref)
		})
	}
}
