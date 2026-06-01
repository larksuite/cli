// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package service

import (
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/meta"
	"github.com/spf13/cobra"
)

// imChatMembersCreate: POST chats/{chat_id}/members with one path param and one
// optional enum query param — the canonical case from the screenshot feedback.
func imChatMembersCreate() meta.Method {
	return meta.FromMap(map[string]interface{}{
		"path":       "chats/{chat_id}/members",
		"httpMethod": "POST",
		"parameters": map[string]interface{}{
			"chat_id": map[string]interface{}{
				"type": "string", "location": "path", "required": true,
			},
			"member_id_type": map[string]interface{}{
				"type": "string", "location": "query", "required": false,
				"options": []interface{}{
					map[string]interface{}{"value": "open_id"},
					map[string]interface{}{"value": "user_id"},
				},
			},
		},
	})
}

func TestServiceMethod_TypedFlagRegistered(t *testing.T) {
	f := &cmdutil.Factory{}
	cmd := NewCmdServiceMethod(f, imSpec(), imChatMembersCreate(), "create", "chat.members", nil)

	if cmd.Flags().Lookup("chat-id") == nil {
		t.Error("expected generated --chat-id flag for path param chat_id")
	}
	if cmd.Flags().Lookup("member-id-type") == nil {
		t.Error("expected generated --member-id-type flag for query param member_id_type")
	}
}

// A query param literally named "format" kebab-collides with the global
// --format flag. Generation must skip it (never re-register, never panic) and
// leave the standard --format flag intact.
func TestServiceMethod_TypedFlagReservedCollisionSkipped(t *testing.T) {
	method := map[string]interface{}{
		"path":       "messages",
		"httpMethod": "GET",
		"parameters": map[string]interface{}{
			"format": map[string]interface{}{"type": "string", "location": "query"},
		},
	}

	var cmd *cobra.Command
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("flag generation panicked on reserved-name collision: %v", r)
			}
		}()
		cmd = NewCmdServiceMethod(&cmdutil.Factory{}, imSpec(), meta.FromMap(method), "list", "messages", nil)
	}()

	fl := cmd.Flags().Lookup("format")
	if fl == nil || fl.DefValue != "json" {
		t.Fatalf("standard --format flag must be preserved, got %+v", fl)
	}
}

func TestServiceMethod_TypedFlag_DrivesPathParam(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, testConfig)
	cmd := NewCmdServiceMethod(f, imSpec(), imChatMembersCreate(), "create", "chat.members", nil)
	cmd.SetArgs([]string{"--chat-id", "oc_abc123", "--data", `{"id_list":["ou_x"]}`, "--dry-run"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), "chats/oc_abc123/members") {
		t.Errorf("expected URL with chat_id substituted from --chat-id, got:\n%s", stdout.String())
	}
}

func TestServiceMethod_TypedFlag_DrivesQueryParam(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, testConfig)
	cmd := NewCmdServiceMethod(f, imSpec(), imChatMembersCreate(), "create", "chat.members", nil)
	cmd.SetArgs([]string{"--chat-id", "oc_abc123", "--member-id-type", "open_id", "--data", `{}`, "--dry-run"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "member_id_type") || !strings.Contains(out, "open_id") {
		t.Errorf("expected query param member_id_type=open_id from flag, got:\n%s", out)
	}
}

func TestServiceMethod_TypedFlag_AgreesWithParams(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, testConfig)
	cmd := NewCmdServiceMethod(f, imSpec(), imChatMembersCreate(), "create", "chat.members", nil)
	cmd.SetArgs([]string{"--chat-id", "oc_abc123", "--params", `{"chat_id":"oc_abc123"}`, "--data", `{}`, "--dry-run"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("same value via flag and --params should be accepted, got: %v", err)
	}
	if !strings.Contains(stdout.String(), "chats/oc_abc123/members") {
		t.Errorf("expected URL with chat_id, got:\n%s", stdout.String())
	}
}

// --params is the base; an explicit typed flag overrides the same key.
func TestServiceMethod_TypedFlag_OverridesParams(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, testConfig)
	cmd := NewCmdServiceMethod(f, imSpec(), imChatMembersCreate(), "create", "chat.members", nil)
	cmd.SetArgs([]string{"--chat-id", "oc_flag", "--params", `{"chat_id":"oc_params"}`, "--data", `{}`, "--dry-run"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "chats/oc_flag/members") {
		t.Errorf("expected --chat-id to override --params chat_id, got:\n%s", out)
	}
	if strings.Contains(out, "oc_params") {
		t.Errorf("--params value should have been overridden by the flag, got:\n%s", out)
	}
}

// Override works for a non-string (integer) param too, exercising the int
// register/read path end to end.
func TestServiceMethod_TypedFlag_IntegerOverridesParams(t *testing.T) {
	method := map[string]interface{}{
		"path":       "messages",
		"httpMethod": "GET",
		"parameters": map[string]interface{}{
			"page_size": map[string]interface{}{"type": "integer", "location": "query"},
		},
	}
	f, stdout, _, _ := cmdutil.TestFactory(t, testConfig)
	cmd := NewCmdServiceMethod(f, imSpec(), meta.FromMap(method), "list", "messages", nil)
	cmd.SetArgs([]string{"--page-size", "100", "--params", `{"page_size":5}`, "--dry-run"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "page_size") || !strings.Contains(out, "100") {
		t.Errorf("expected --page-size 100 to override --params page_size=5, got:\n%s", out)
	}
}

// Regression: with no typed flags passed, behavior is byte-identical to today.
func TestServiceMethod_TypedFlag_OnlyParamsStillWorks(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, testConfig)
	cmd := NewCmdServiceMethod(f, imSpec(), imChatMembersCreate(), "create", "chat.members", nil)
	cmd.SetArgs([]string{"--params", `{"chat_id":"oc_abc123"}`, "--data", `{}`, "--dry-run"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), "chats/oc_abc123/members") {
		t.Errorf("expected URL with chat_id from --params, got:\n%s", stdout.String())
	}
}

// Regression: --params null is valid JSON that unmarshals to a nil map. A typed
// flag overlaying onto it must not panic (assignment to a nil map) — null is
// treated as "no base params", with the flag value applied on top.
func TestServiceMethod_TypedFlag_OverridesNullParams(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, testConfig)
	cmd := NewCmdServiceMethod(f, imSpec(), imChatMembersCreate(), "create", "chat.members", nil)
	cmd.SetArgs([]string{"--chat-id", "oc_abc123", "--params", "null", "--data", `{}`, "--dry-run"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("--params null with a typed flag should not error, got: %v", err)
	}
	if !strings.Contains(stdout.String(), "chats/oc_abc123/members") {
		t.Errorf("expected chat_id from --chat-id over null --params, got:\n%s", stdout.String())
	}
}

// Startup smoke test: registering every embedded method must not panic on a
// generated-flag name collision (pflag panics on duplicate registration, which
// would crash the whole CLI at startup), and a known path param must surface as
// a typed flag end to end.
func TestRegisterServiceCommands_GeneratesFlagsNoPanic(t *testing.T) {
	root := &cobra.Command{Use: "lark-cli"}
	f := &cmdutil.Factory{}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("registering all service commands panicked: %v", r)
		}
	}()
	RegisterServiceCommands(root, f)

	create, _, err := root.Find([]string{"im", "chat.members", "create"})
	if err != nil {
		t.Fatalf("im chat.members create not registered: %v", err)
	}
	if create.Flags().Lookup("chat-id") == nil {
		t.Error("expected generated --chat-id flag on im chat.members create")
	}
}

// Locks the boolean and array branches of bindParamFlag end to end (string and
// integer are covered above): a bool flag yields true and a repeatable array
// flag yields all its elements in the request.
func TestServiceMethod_TypedFlag_BoolAndArrayKinds(t *testing.T) {
	method := map[string]interface{}{
		"path":       "items",
		"httpMethod": "GET",
		"parameters": map[string]interface{}{
			"with_deleted": map[string]interface{}{"type": "boolean", "location": "query"},
			"ids":          map[string]interface{}{"type": "list", "location": "query"},
		},
	}
	f, stdout, _, _ := cmdutil.TestFactory(t, testConfig)
	cmd := NewCmdServiceMethod(f, imSpec(), meta.FromMap(method), "list", "items", nil)
	cmd.SetArgs([]string{"--with-deleted", "--ids", "a", "--ids", "b", "--dry-run"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := stdout.String()
	for _, want := range []string{"with_deleted", "true", "ids", "\"a\"", "\"b\""} {
		if !strings.Contains(out, want) {
			t.Errorf("expected dry-run output to contain %q, got:\n%s", want, out)
		}
	}
}

// Override (--params base, typed flag wins) is covered for string and integer
// above; this locks the same semantics for the boolean and array kinds.
func TestServiceMethod_TypedFlag_BoolAndArrayOverrideParams(t *testing.T) {
	method := map[string]interface{}{
		"path":       "items",
		"httpMethod": "GET",
		"parameters": map[string]interface{}{
			"with_deleted": map[string]interface{}{"type": "boolean", "location": "query"},
			"ids":          map[string]interface{}{"type": "list", "location": "query"},
		},
	}
	f, stdout, _, _ := cmdutil.TestFactory(t, testConfig)
	cmd := NewCmdServiceMethod(f, imSpec(), meta.FromMap(method), "list", "items", nil)
	cmd.SetArgs([]string{
		"--params", `{"with_deleted":false,"ids":["from_params"]}`,
		"--with-deleted", "--ids", "a", "--ids", "b",
		"--dry-run",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := stdout.String()
	for _, want := range []string{"with_deleted", "true", "\"a\"", "\"b\""} {
		if !strings.Contains(out, want) {
			t.Errorf("expected flag to override --params (want %q), got:\n%s", want, out)
		}
	}
	if strings.Contains(out, "from_params") {
		t.Errorf("--params array value should have been overridden by --ids, got:\n%s", out)
	}
}
