// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package affordance

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/apicatalog"
	"github.com/larksuite/cli/internal/meta"
)

func TestContactUpdateSelfAffordance(t *testing.T) {
	previous := mdSource
	t.Cleanup(func() { SetSource(previous) })
	SetSource(os.DirFS("../../affordance"))
	raw, ok := For(apicatalog.Catalog{}, "contact", "+update-self")
	if !ok {
		t.Fatal("contact +update-self guidance is missing")
	}
	a, ok := (meta.Method{Affordance: raw}).ParsedAffordance()
	if !ok {
		t.Fatal("contact +update-self guidance did not parse")
	}
	ref := "lark-contact/references/lark-contact-update-self.md"
	if !reflect.DeepEqual(a.Skills, []string{"lark-contact", ref}) {
		t.Fatalf("skills = %v", a.Skills)
	}
	if _, err := os.Stat("../../skills/" + ref); err != nil {
		t.Fatalf("guidance reference must ship: %v", err)
	}
	if len(a.Examples) != 2 || !strings.Contains(strings.Join(a.AvoidWhen, " "), "another employee") {
		t.Fatalf("self-only guidance or runnable examples missing: %+v", a)
	}
}
