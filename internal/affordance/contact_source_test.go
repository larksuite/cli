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

func TestContactSearchUserAffordanceTips(t *testing.T) {
	prev := mdSource
	t.Cleanup(func() { SetSource(prev) })
	SetSource(os.DirFS("../../affordance"))

	raw, ok := For(apicatalog.Catalog{}, "contact", "+search-user")
	if !ok {
		t.Fatal("For(contact, +search-user) ok=false")
	}
	a, ok := (meta.Method{Affordance: raw}).ParsedAffordance()
	if !ok {
		t.Fatal("contact +search-user affordance did not parse")
	}
	if len(a.Tips) == 0 {
		t.Fatal("tips migration must keep operational guidance in structured Tips")
	}
	if !containsItem(a.Tips, "41050") || !containsItem(a.Tips, "visibility scope") {
		t.Fatalf("tips must carry the 41050 visibility-scope remediation: %v", a.Tips)
	}
	if !containsItem(a.Tips, "has_more=true") {
		t.Fatalf("tips must keep the has_more guidance from the Go shortcut metadata: %v", a.Tips)
	}
}

func TestContactBatchQueryExamplePinsUserIdentity(t *testing.T) {
	prev := mdSource
	t.Cleanup(func() { SetSource(prev) })
	SetSource(os.DirFS("../../affordance"))

	method := meta.FromMap(map[string]interface{}{"id": "user_profiles.batch_query", "httpMethod": "POST"})
	service := meta.ServiceFromMap(map[string]interface{}{
		"name": "contact",
		"resources": map[string]interface{}{
			"user_profiles": map[string]interface{}{"methods": map[string]interface{}{"batch_query": method}},
		},
	})
	catalog := apicatalog.New(apicatalog.SourceEmbedded, []meta.Service{service})

	raw, ok := For(catalog, "contact", "user_profiles.batch_query")
	if !ok {
		t.Fatal("For(contact, user_profiles.batch_query) ok=false")
	}
	a, ok := (meta.Method{Affordance: raw}).ParsedAffordance()
	if !ok {
		t.Fatal("contact user_profiles batch_query affordance did not parse")
	}
	if len(a.Examples) != 1 {
		t.Fatalf("examples = %#v, want exactly one", a.Examples)
	}
	if !strings.Contains(a.Examples[0].Command, "--as user") {
		t.Fatalf("batch_query example must pin user identity (the endpoint rejects bot tokens): %q", a.Examples[0].Command)
	}
}
