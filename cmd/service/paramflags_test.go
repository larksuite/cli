// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package service

import (
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/meta"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
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

// A param whose kebab name collides with a global flag (here "format" vs the
// global --format) gets no typed flag, but the collision is no longer silent:
// non-colliding params still get flags, the global --format is untouched, and
// --help shows the exact --params form and steers the reader off --format.
func TestServiceMethod_ParamsOnly_HelpSteersToParams(t *testing.T) {
	method := map[string]interface{}{
		"path":       "things/{thing_id}",
		"httpMethod": "GET",
		"parameters": map[string]interface{}{
			"thing_id": map[string]interface{}{"type": "string", "location": "path", "required": true},
			"format": map[string]interface{}{"type": "string", "location": "query", "min": "1", "max": "64", "description": "返回的消息体格式。", "options": []interface{}{
				map[string]interface{}{"value": "full"},
				map[string]interface{}{"value": "metadata"},
			}},
		},
	}
	cmd := NewCmdServiceMethod(&cmdutil.Factory{}, imSpec(), meta.FromMap(method), "get", "things", nil)

	if cmd.Flags().Lookup("thing-id") == nil {
		t.Error("non-colliding param should still get a typed --thing-id flag")
	}
	if fl := cmd.Flags().Lookup("format"); fl == nil || fl.DefValue != "json" {
		t.Fatalf("global --format must be preserved (not shadowed), got %+v", fl)
	}
	for _, want := range []string{`--params '{"format"`, "返回的消息体格式", "full", "metadata", "min: 1, max: 64", "do not use --format"} {
		if !strings.Contains(cmd.Long, want) {
			t.Errorf("help should contain %q so the reader uses --params, not --format; got:\n%s", want, cmd.Long)
		}
	}
}

// The collision guard derives reserved names from the actual flag sets — local
// flags plus the root's persistent flags passed in — so a future persistent
// flag is covered with no hand-maintained list. Here a param named "profile"
// (a root persistent flag) is skipped while a normal param is bound.
func TestParamFlagBinder_PersistentFlagReserved(t *testing.T) {
	cmd := &cobra.Command{Use: "x"}
	reserved := pflag.NewFlagSet("root", pflag.ContinueOnError)
	reserved.String("profile", "", "use a specific profile")

	m := meta.FromMap(map[string]interface{}{"parameters": map[string]interface{}{
		"profile": map[string]interface{}{"type": "string", "location": "query"},
		"id":      map[string]interface{}{"type": "string", "location": "path"},
	}})
	b := newParamFlagBinder(cmd, m.Params(), reserved)

	if cmd.Flags().Lookup("id") == nil {
		t.Error("non-colliding param should get a typed flag")
	}
	if cmd.Flags().Lookup("profile") != nil {
		t.Error("param colliding with a reserved persistent flag must not be registered")
	}
	found := false
	for _, p := range b.paramsOnly {
		if p.field.Name == "profile" {
			found = true
		}
	}
	if !found {
		t.Error("colliding param should be recorded for the --params help note")
	}
}

// boolIntQueryMethod is the fixture for the zero-value semantics tests: one
// boolean and one integer query param, where false and 0 are meaningful values.
func boolIntQueryMethod(required bool) meta.Method {
	return meta.FromMap(map[string]interface{}{
		"path":       "items",
		"httpMethod": "GET",
		"parameters": map[string]interface{}{
			"with_deleted": map[string]interface{}{"type": "boolean", "location": "query", "required": required},
			"page_size":    map[string]interface{}{"type": "integer", "location": "query"},
		},
	})
}

// Presence is intent: a typed flag is only overlaid when explicitly Changed,
// so --flag=false / --flag 0 are real values and must be sent — not silently
// dropped as "empty", which would let the API default win over an explicit
// user choice.
func TestServiceMethod_TypedFlag_ExplicitFalseAndZeroAreSent(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, testConfig)
	cmd := NewCmdServiceMethod(f, imSpec(), boolIntQueryMethod(false), "list", "items", nil)
	cmd.SetArgs([]string{"--with-deleted=false", "--page-size", "0", "--dry-run"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := stdout.String()
	for _, want := range []string{`"with_deleted": false`, `"page_size": 0`} {
		if !strings.Contains(out, want) {
			t.Errorf("explicit zero value must be sent (want %s), got:\n%s", want, out)
		}
	}
}

// An explicitly provided false satisfies a required query parameter — the
// pre-flight must not report "missing" for a value the user just set.
func TestServiceMethod_TypedFlag_ExplicitFalseSatisfiesRequired(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, testConfig)
	cmd := NewCmdServiceMethod(f, imSpec(), boolIntQueryMethod(true), "list", "items", nil)
	cmd.SetArgs([]string{"--with-deleted=false", "--dry-run"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("required param explicitly set to false must pass pre-flight, got: %v", err)
	}
	if !strings.Contains(stdout.String(), `"with_deleted": false`) {
		t.Errorf("explicit false must be sent, got:\n%s", stdout.String())
	}
}

// The same presence-is-intent rule applies to the --params JSON base: a key
// deliberately written as false/0 is sent. (Zero values used to be silently
// dropped; this locks the corrected semantics as the contract.)
func TestServiceMethod_Params_JSONZeroValuesAreSent(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, testConfig)
	cmd := NewCmdServiceMethod(f, imSpec(), boolIntQueryMethod(false), "list", "items", nil)
	cmd.SetArgs([]string{"--params", `{"with_deleted":false,"page_size":0}`, "--dry-run"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := stdout.String()
	for _, want := range []string{`"with_deleted": false`, `"page_size": 0`} {
		if !strings.Contains(out, want) {
			t.Errorf("--params zero value must be sent (want %s), got:\n%s", want, out)
		}
	}
}

// "" stays unusable: a required parameter fed an empty-string placeholder is
// still caught by the friendly pre-flight error, not sent as an empty value.
func TestServiceMethod_Params_EmptyStringStillMissing(t *testing.T) {
	method := meta.FromMap(map[string]interface{}{
		"path":       "items",
		"httpMethod": "GET",
		"parameters": map[string]interface{}{
			"user_id_type": map[string]interface{}{"type": "string", "location": "query", "required": true},
		},
	})
	f, _, _, _ := cmdutil.TestFactory(t, testConfig)
	cmd := NewCmdServiceMethod(f, imSpec(), method, "list", "items", nil)
	cmd.SetArgs([]string{"--params", `{"user_id_type":""}`, "--dry-run"})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "missing required query parameter") {
		t.Fatalf("empty string for a required param should still pre-flight error, got: %v", err)
	}
}

// A declared optional query param fed "" is dropped (unusable value), not sent
// as an empty query value — the declared-param loop owns the decision and the
// undeclared passthrough must not resurrect it. Undeclared keys stay the
// verbatim raw escape hatch.
func TestServiceMethod_Params_EmptyOptionalDroppedUndeclaredKept(t *testing.T) {
	method := meta.FromMap(map[string]interface{}{
		"path":       "items",
		"httpMethod": "GET",
		"parameters": map[string]interface{}{
			"user_id_type": map[string]interface{}{"type": "string", "location": "query"},
		},
	})
	f, stdout, _, _ := cmdutil.TestFactory(t, testConfig)
	cmd := NewCmdServiceMethod(f, imSpec(), method, "list", "items", nil)
	cmd.SetArgs([]string{"--params", `{"user_id_type":"","custom_key":"v1"}`, "--dry-run"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := stdout.String()
	if strings.Contains(out, "user_id_type") {
		t.Errorf("declared optional param with empty value must be dropped, got:\n%s", out)
	}
	if !strings.Contains(out, `"custom_key": "v1"`) {
		t.Errorf("undeclared key must pass through verbatim, got:\n%s", out)
	}
}

// min/max from the metadata surface on the typed flag's help line, in the same
// vocabulary as the envelope's minimum/maximum.
func TestParamFlagUsage_Bounds(t *testing.T) {
	cases := []struct{ name, min, max, want string }{
		{"both", "1", "100", "min: 1, max: 100"},
		{"min only", "1", "", "min: 1"},
		{"max only", "", "64", "max: 64"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fields := meta.FromMap(map[string]interface{}{"parameters": map[string]interface{}{
				"page_size": map[string]interface{}{"type": "integer", "location": "query", "min": tc.min, "max": tc.max},
			}}).Params()
			if usage := paramFlagUsage(fields[0]); !strings.Contains(usage, tc.want) {
				t.Errorf("usage = %q, want contains %q", usage, tc.want)
			}
		})
	}
	t.Run("no bounds, no clause", func(t *testing.T) {
		fields := meta.FromMap(map[string]interface{}{"parameters": map[string]interface{}{
			"page_token": map[string]interface{}{"type": "string", "location": "query"},
		}}).Params()
		if usage := paramFlagUsage(fields[0]); strings.Contains(usage, "min:") || strings.Contains(usage, "max:") {
			t.Errorf("usage without bounds should not mention min/max, got %q", usage)
		}
	})
}

// The sanitized field description rides the help line — a bare name like
// user_mailbox_id carries no meaning. The cut is at note separators (;), NOT
// at sentence ends (。): the later sentence often holds the key affordance.
func TestParamFlagUsage_Description(t *testing.T) {
	fields := meta.FromMap(map[string]interface{}{"parameters": map[string]interface{}{
		"user_mailbox_id": map[string]interface{}{
			"type": "string", "location": "path", "required": true,
			"description": `用户邮箱地址。当使用用户身份访问时，可以输入"me"代表当前调用接口用户;后续补充说明不该出现`,
		},
	}}).Params()
	usage := paramFlagUsage(fields[0])
	if !strings.Contains(usage, `可以输入"me"代表当前调用接口用户`) {
		t.Errorf("description must keep full sentences up to the note separator, got %q", usage)
	}
	if strings.Contains(usage, "补充说明") {
		t.Errorf("text after the note separator must be cut, got %q", usage)
	}

	t.Run("long description truncated", func(t *testing.T) {
		fields := meta.FromMap(map[string]interface{}{"parameters": map[string]interface{}{
			"x": map[string]interface{}{
				"type": "string", "location": "query",
				"description": strings.Repeat("长", 80),
			},
		}}).Params()
		usage := paramFlagUsage(fields[0])
		if !strings.Contains(usage, "...") {
			t.Errorf("long description should be truncated with ellipsis, got %q", usage)
		}
		if strings.Contains(usage, strings.Repeat("长", 61)) {
			t.Errorf("description should not exceed the cap, got %q", usage)
		}
	})

	t.Run("trailing sentence punctuation trimmed", func(t *testing.T) {
		fields := meta.FromMap(map[string]interface{}{"parameters": map[string]interface{}{
			"x": map[string]interface{}{
				"type": "string", "location": "query", "description": "返回格式。",
			},
		}}).Params()
		if usage := paramFlagUsage(fields[0]); strings.Contains(usage, "。.") {
			t.Errorf("clause join must not double the punctuation, got %q", usage)
		}
	})
}

// Pins the convergence contract: the params-only addendum renders the SAME
// fieldFacts list the typed flag's usage line joins inline — a fact added to
// fieldFacts reaches both surfaces, and neither can drift over what a param's
// help says (the addendum once rendered values-only enums and silently lacked
// the API default).
func TestParamHelp_BothSurfacesRenderFieldFacts(t *testing.T) {
	f := meta.FromMap(map[string]interface{}{"parameters": map[string]interface{}{
		"mode": map[string]interface{}{
			"type": "string", "location": "query",
			"description": "模式选择。",
			"default":     "fast",
			"min":         "1", "max": "8",
			"options": []interface{}{
				map[string]interface{}{"value": "fast", "description": "快速"},
				map[string]interface{}{"value": "full"},
			},
		},
	}}).Params()[0]

	facts := fieldFacts(f)
	if len(facts) != 4 { // description, enum, bounds, API default
		t.Fatalf("fieldFacts = %v, want 4 facts", facts)
	}
	usage := paramFlagUsage(f)
	help := (&paramFlagBinder{paramsOnly: []paramsOnlyField{{field: f}}}).paramsOnlyHelp()
	for _, fact := range facts {
		if !strings.Contains(usage, fact) {
			t.Errorf("usage line missing fact %q: %q", fact, usage)
		}
		if !strings.Contains(help, fact) {
			t.Errorf("params-only addendum missing fact %q:\n%s", fact, help)
		}
	}
}

// Bounds reach the registered flag's help end to end.
func TestServiceMethod_TypedFlag_HelpShowsBounds(t *testing.T) {
	method := meta.FromMap(map[string]interface{}{
		"path":       "items",
		"httpMethod": "GET",
		"parameters": map[string]interface{}{
			"page_size": map[string]interface{}{"type": "integer", "location": "query", "min": "1", "max": "100", "default": "20"},
		},
	})
	cmd := NewCmdServiceMethod(&cmdutil.Factory{}, imSpec(), method, "list", "items", nil)
	fl := cmd.Flags().Lookup("page-size")
	if fl == nil {
		t.Fatal("expected generated --page-size flag")
	}
	if !strings.Contains(fl.Usage, "min: 1, max: 100") {
		t.Errorf("flag usage should carry bounds, got %q", fl.Usage)
	}
}

// The missing-required hint must name both recovery paths — the typed flag and
// the --params fallback — so a reader who only knows one input style can
// proceed without a round-trip through schema.
func TestServiceMethod_MissingRequired_HintNamesFlagAndParams(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, testConfig)
	cmd := NewCmdServiceMethod(f, imSpec(), imChatMembersCreate(), "create", "chat.members", nil)
	cmd.SetArgs([]string{"--data", `{"id_list":["ou_x"]}`, "--dry-run"})

	err := cmd.Execute()
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected *errs.ValidationError, got %T: %v", err, err)
	}
	for _, want := range []string{"--chat-id", `--params '{"chat_id": "<value>"}'`, "lark-cli schema im.chat.members.create"} {
		if !strings.Contains(ve.Hint, want) {
			t.Errorf("hint %q should contain %q", ve.Hint, want)
		}
	}
}

// A params-only required field (kebab name claimed by the standard --format
// flag) has no typed flag to offer: the hint must give only the --params form,
// never steer the reader to the colliding flag.
func TestServiceMethod_MissingRequired_ParamsOnlyHintSkipsFlag(t *testing.T) {
	method := meta.FromMap(map[string]interface{}{
		"path":       "messages",
		"httpMethod": "GET",
		"parameters": map[string]interface{}{
			"format": map[string]interface{}{"type": "string", "location": "query", "required": true},
		},
	})
	f, _, _, _ := cmdutil.TestFactory(t, testConfig)
	cmd := NewCmdServiceMethod(f, imSpec(), method, "list", "messages", nil)
	cmd.SetArgs([]string{"--dry-run"})

	err := cmd.Execute()
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected *errs.ValidationError, got %T: %v", err, err)
	}
	if !strings.Contains(ve.Hint, `--params '{"format": "<value>"}'`) {
		t.Errorf("hint %q should carry the --params form", ve.Hint)
	}
	if strings.Contains(ve.Hint, "set --format") {
		t.Errorf("hint %q must not steer to the colliding --format flag", ve.Hint)
	}
}
