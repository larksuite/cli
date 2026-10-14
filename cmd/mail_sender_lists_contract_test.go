// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd_test

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/meta"
	"github.com/larksuite/cli/internal/registry"
)

// TestMailSenderListGeneratedContract pins both halves of this capability:
// generated metadata must retain the six user-mailbox methods and the checked-in
// operator guide must contain examples accepted by that generated command tree.
// Bare-module builds intentionally ship only meta_data_default.json, so they have
// no generated contract to validate and are skipped like other metadata tests.
func TestMailSenderListGeneratedContract(t *testing.T) {
	docPath := filepath.Join("..", "skills", "lark-mail", "references", "lark-mail-sender-lists.md")
	doc, err := os.ReadFile(docPath)
	if err != nil {
		t.Fatal(err)
	}

	if len(registry.EmbeddedServicesTyped()) == 0 {
		t.Skip("generated API metadata is not embedded in this bare-module test run")
	}

	cases := []struct {
		resource string
		segment  string
	}{
		{resource: "user_mailbox.allow_senders", segment: "allow_senders"},
		{resource: "user_mailbox.blocked_senders", segment: "blocked_senders"},
	}
	for _, tc := range cases {
		t.Run(tc.resource, func(t *testing.T) {
			target, err := registry.EmbeddedCatalog().Resolve([]string{"mail", tc.resource})
			if err != nil {
				t.Fatal(err)
			}
			if target.Resource == nil {
				t.Fatalf("%s resolved without a resource", tc.resource)
			}
			assertSenderListMethod(t, target.Resource.Resource, "list", "GET", tc.segment, false)
			assertSenderListMethod(t, target.Resource.Resource, "batch_create", "POST", tc.segment+"/batch_create", true)
			assertSenderListMethod(t, target.Resource.Resource, "batch_remove", "POST", tc.segment+"/batch_remove", true)
		})
	}

	refs := parseRefs(string(doc))
	if len(refs) != 12 {
		t.Fatalf("sender-list guide command examples = %d, want 12", len(refs))
	}
	if findings := checkRefs(buildCmdExampleCatalog(), refs); len(findings) != 0 {
		for _, finding := range findings {
			t.Errorf("line %d: %s %q uses %q", finding.line, finding.kind, finding.path, finding.flag)
		}
		t.Fatalf("sender-list guide has %d command example(s) outside the generated CLI contract", len(findings))
	}
}

func assertSenderListMethod(t *testing.T, resource meta.Resource, name, httpMethod, pathSuffix string, write bool) {
	t.Helper()
	method, ok := resource.Method(name)
	if !ok {
		t.Fatalf("method %s missing", name)
	}
	if method.HTTPMethod != httpMethod || !strings.HasSuffix(method.Path, pathSuffix) {
		t.Errorf("%s transport = %s %s, want %s .../%s", name, method.HTTPMethod, method.Path, httpMethod, pathSuffix)
	}
	if got := method.Identities(); !reflect.DeepEqual(got, []string{"user"}) {
		t.Errorf("%s identities = %v, want [user]", name, got)
	}
	mailbox, ok := method.Parameters["user_mailbox_id"]
	if !ok {
		t.Fatalf("%s parameters omit user_mailbox_id", name)
	}
	if mailbox.Location != "path" || !mailbox.Required {
		t.Errorf("%s user_mailbox_id = %+v, want required path parameter", name, mailbox)
	}

	if !write {
		if method.HTTPMethod != "GET" || method.Risk != "read" || method.Danger {
			t.Errorf("list risk contract = method:%s risk:%s danger:%v", method.HTTPMethod, method.Risk, method.Danger)
		}
		assertFieldNames(t, name+" parameters", method.Parameters, "keyword", "page_size", "page_token", "user_mailbox_id")
		for _, field := range []string{"keyword", "page_size", "page_token"} {
			if method.Parameters[field].Location != "query" {
				t.Errorf("list parameter %s location = %q, want query", field, method.Parameters[field].Location)
			}
		}
		assertFieldNames(t, name+" response", method.ResponseBody, "has_more", "items", "page_token")
		items := method.ResponseBody["items"]
		if items.Type != "array" {
			t.Errorf("list items type = %q, want array", items.Type)
		}
		assertFieldNames(t, name+" items", items.Properties, "sender")
		return
	}

	if method.Risk != "write" || !method.Danger {
		t.Errorf("%s risk contract = risk:%s danger:%v, want write/true", name, method.Risk, method.Danger)
	}
	if name == "batch_create" {
		assertFieldNames(t, name+" body", method.RequestBody, "items")
		items := method.RequestBody["items"]
		if items.Type != "array" || !items.Required {
			t.Errorf("batch_create items = %+v, want required array", items)
		}
		assertFieldNames(t, name+" items", items.Properties, "sender", "sender_type")
	} else {
		assertFieldNames(t, name+" body", method.RequestBody, "senders")
		senders := method.RequestBody["senders"]
		if senders.Type != "array" || !senders.Required {
			t.Errorf("batch_remove senders = %+v, want required array", senders)
		}
	}
	assertFieldNames(t, name+" response", method.ResponseBody, "failed_items")
	failed := method.ResponseBody["failed_items"]
	if failed.Type != "array" {
		t.Errorf("%s failed_items type = %q, want array", name, failed.Type)
	}
	assertFieldNames(t, name+" failed_items", failed.Properties, "reason_code", "sender")
}

func assertFieldNames(t *testing.T, label string, fields map[string]meta.Field, want ...string) {
	t.Helper()
	got := make([]string, 0, len(fields))
	for name := range fields {
		got = append(got, name)
	}
	sort.Strings(got)
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("%s = %v, want %v", label, got, want)
	}
}
