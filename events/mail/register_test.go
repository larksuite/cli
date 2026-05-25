// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"testing"
)

func TestRegister_HasOneEventKey(t *testing.T) {
	keys := Keys()
	if len(keys) != 1 {
		t.Fatalf("expected 1 EventKey, got %d", len(keys))
	}
}

func TestRegister_HasSubscriptionKeyMailbox(t *testing.T) {
	k := Keys()[0]
	var foundSubKey bool
	for _, p := range k.Params {
		if p.Name == "mailbox" && p.SubscriptionKey {
			foundSubKey = true
		}
	}
	if !foundSubKey {
		t.Error("mailbox param should be marked SubscriptionKey=true")
	}
}

func TestRegister_HasNormalizeMatchProcess(t *testing.T) {
	k := Keys()[0]
	if k.NormalizeParams == nil {
		t.Error("NormalizeParams hook should be set")
	}
	if k.Match == nil {
		t.Error("Match hook should be set")
	}
	if k.Process == nil {
		t.Error("Process hook should be set")
	}
	if k.PreConsume == nil {
		t.Error("PreConsume hook should be set")
	}
}

func TestRegister_HasAllParams(t *testing.T) {
	k := Keys()[0]
	wantNames := []string{"mailbox", "folders", "labels", "msg-format"}
	gotNames := make(map[string]bool)
	for _, p := range k.Params {
		gotNames[p.Name] = true
	}
	for _, n := range wantNames {
		if !gotNames[n] {
			t.Errorf("missing param: %s", n)
		}
	}
}

// TestRegister_SchemaIsCustomMailReceivedPayload verifies Schema.Custom is used
// (not Native) because processMailEvent produces the complete output shape.
// Schema.Native is incompatible with Process per registry validation.
func TestRegister_SchemaIsCustomMailReceivedPayload(t *testing.T) {
	k := Keys()[0]
	if k.Schema.Custom == nil || k.Schema.Custom.Type == nil {
		t.Fatal("Schema should be Custom with non-nil Type")
	}
	if k.Schema.Custom.Type.Name() != "MailReceivedPayload" {
		t.Errorf("schema type is %v, want MailReceivedPayload", k.Schema.Custom.Type)
	}
}

func TestRegister_AuthTypesUserOnly(t *testing.T) {
	k := Keys()[0]
	if len(k.AuthTypes) != 1 || k.AuthTypes[0] != "user" {
		t.Errorf("AuthTypes = %v, want [user]", k.AuthTypes)
	}
}

func TestRegister_RequiredScopes(t *testing.T) {
	k := Keys()[0]
	want := map[string]bool{
		"mail:event":                                true,
		"mail:user_mailbox.event.mail_address:read": true,
		"mail:user_mailbox:readonly":                true,
		"mail:user_mailbox.message:readonly":        true,
	}
	got := make(map[string]bool)
	for _, s := range k.Scopes {
		got[s] = true
	}
	for s := range want {
		if !got[s] {
			t.Errorf("missing scope: %s", s)
		}
	}
}
