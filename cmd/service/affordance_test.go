// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package service

import (
	"encoding/json"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/larksuite/cli/internal/cmdmeta"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/meta"
	"github.com/spf13/cobra"
)

func TestRenderAffordance(t *testing.T) {
	raw := json.RawMessage(`{
		"use_when": ["发送文本消息"],
		"avoid_when": ["群已解散"],
		"prerequisites": ["已获取 chat_id"],
		"tips": ["富文本用 msg_type=post"],
		"examples": [
			{"description":"发一条文本","command":"lark-cli im messages create --params '{...}'"},
			{"command":"lark-cli im messages list"},
			{"description":"no command, skipped","command":""}
		],
		"related": ["im.messages.list"]
	}`)
	out := renderAffordance(meta.Method{Affordance: raw})
	for _, want := range []string{
		"When to use:", "发送文本消息",
		"Avoid when:", "群已解散",
		"Prerequisites:", "已获取 chat_id",
		"Tips:", "富文本用 msg_type=post",
		"Examples:", "发一条文本", "lark-cli im messages create --params '{...}'",
		"lark-cli im messages list", // example with no description -> bare command line
		"Related:", "im.messages.list",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("renderAffordance missing %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "no command, skipped") {
		t.Errorf("example with empty command should be skipped:\n%s", out)
	}

	// Absent or empty affordance renders nothing (so methods without an overlay
	// add nothing to their help).
	if renderAffordance(meta.Method{}) != "" || renderAffordance(meta.Method{Affordance: json.RawMessage(`{}`)}) != "" {
		t.Error("empty affordance should render nothing")
	}
}

// Affordance is rendered lazily (at --help time) rather than baked into the
// command's Long, so building a command never carries the affordance block —
// even for a method whose metadata happens to declare one.
func TestServiceMethod_AffordanceNotInLong(t *testing.T) {
	withAff := map[string]interface{}{
		"id": "messages.create", "path": "messages", "httpMethod": "POST", "description": "发送消息",
		"affordance": map[string]interface{}{
			"examples": []interface{}{
				map[string]interface{}{"description": "发文本", "command": "lark-cli im messages create ..."},
			},
		},
	}
	f, _, _, _ := cmdutil.TestFactory(t, testConfig)
	cmd := NewCmdServiceMethod(f, imSpec(), meta.FromMap(withAff), "create", "messages", nil)
	if strings.Contains(cmd.Long, "Examples:") {
		t.Errorf("affordance must not be baked into Long (lazy):\n%s", cmd.Long)
	}
	// The lookup ref is recorded so the help path can resolve it later.
	if svc, method, ok := cmdmeta.AffordanceRef(cmd); !ok || svc != "im" || method != "messages.create" {
		t.Errorf("affordance ref = %q/%q (ok=%v), want im/messages.create", svc, method, ok)
	}
}

// RenderAffordanceForCmd resolves a command's overlay through the (injectable)
// lookup and renders it; commands without a ref render nothing.
func TestRenderAffordanceForCmd(t *testing.T) {
	orig := affordanceLookup
	t.Cleanup(func() { affordanceLookup = orig })
	affordanceLookup = func(service, methodID string) (json.RawMessage, bool) {
		if service != "im" || methodID != "messages.create" {
			return nil, false
		}
		return json.RawMessage(`{"use_when":["发文本消息"],"tips":["富文本用 msg_type=post"],"examples":[{"description":"发一条","command":"lark-cli im messages create ..."}]}`), true
	}

	f, _, _, _ := cmdutil.TestFactory(t, testConfig)
	withRef := map[string]interface{}{"id": "messages.create", "path": "messages", "httpMethod": "POST", "description": "发送消息"}
	cmd := NewCmdServiceMethod(f, imSpec(), meta.FromMap(withRef), "create", "messages", nil)
	block := RenderAffordanceForCmd(cmd)
	for _, want := range []string{"When to use:", "发文本消息", "Tips:", "富文本用 msg_type=post", "Examples:", "lark-cli im messages create ..."} {
		if !strings.Contains(block, want) {
			t.Errorf("RenderAffordanceForCmd missing %q in:\n%s", want, block)
		}
	}

	// No overlay for this method id -> empty block.
	noRef := map[string]interface{}{"id": "x.list", "path": "x", "httpMethod": "GET", "description": "d"}
	cmd2 := NewCmdServiceMethod(f, imSpec(), meta.FromMap(noRef), "list", "x", nil)
	if got := RenderAffordanceForCmd(cmd2); got != "" {
		t.Errorf("method with no overlay should render nothing, got:\n%s", got)
	}
}

// PrepareMethodHelp composes the guidance into Long at the top: description,
// then the affordance block, then the full-schema pointer — so an agent reads
// when-to-use/examples before the flag list.
func TestPrepareMethodHelp(t *testing.T) {
	orig := affordanceLookup
	t.Cleanup(func() { affordanceLookup = orig })
	affordanceLookup = func(_, _ string) (json.RawMessage, bool) {
		return json.RawMessage(`{"use_when":["发文本消息"],"examples":[{"description":"发一条","command":"lark-cli im messages create ..."}]}`), true
	}

	f, _, _, _ := cmdutil.TestFactory(t, testConfig)
	m := map[string]interface{}{"id": "messages.create", "path": "messages", "httpMethod": "POST", "description": "发送消息"}
	cmd := NewCmdServiceMethod(f, imSpec(), meta.FromMap(m), "create", "messages", nil)

	if !PrepareMethodHelp(cmd, nil) {
		t.Fatal("PrepareMethodHelp returned false for a service-method command")
	}
	long := cmd.Long
	// Description leads; affordance block sits above the schema pointer.
	descAt := strings.Index(long, "发送消息")
	useAt := strings.Index(long, "When to use:")
	exAt := strings.Index(long, "Examples:")
	schemaAt := strings.Index(long, "Full parameter schema:")
	if descAt != 0 {
		t.Errorf("description should lead Long, got:\n%s", long)
	}
	if !(descAt < useAt && useAt < exAt && exAt < schemaAt) {
		t.Errorf("order should be description < affordance < schema pointer; got desc=%d use=%d ex=%d schema=%d\n%s", descAt, useAt, exAt, schemaAt, long)
	}

	// A non-service command (no schema-path annotation) is left untouched.
	if PrepareMethodHelp(&cobra.Command{Use: "plain"}, nil) {
		t.Error("PrepareMethodHelp should return false for a non-service command")
	}
}

// PrepareShortcutHelp composes a shortcut's Long from its overlay with the same
// top layout as method help (no schema pointer), folding declarative tips when
// the overlay declares none, and leaves shortcuts without an overlay entry (and
// non-shortcut commands) for the default help path.
func TestPrepareShortcutHelp(t *testing.T) {
	orig := affordanceLookup
	t.Cleanup(func() { affordanceLookup = orig })
	affordanceLookup = func(service, methodID string) (json.RawMessage, bool) {
		if service == "calendar" && methodID == "+create" {
			return json.RawMessage(`{"use_when":["高层创建日程"],"skills":["lark-calendar"]}`), true
		}
		return nil, false
	}

	sc := &cobra.Command{Use: "+create", Short: "Create an event"}
	cmdmeta.SetSource(sc, cmdmeta.SourceShortcut, false)
	cmdmeta.SetAffordanceRef(sc, "calendar", "+create")
	cmdutil.SetRisk(sc, "write")
	cmdutil.SetTips(sc, []string{"start/end 收 ISO 8601"})

	if !PrepareShortcutHelp(sc, nil) {
		t.Fatal("PrepareShortcutHelp returned false for a shortcut with an overlay")
	}
	for _, want := range []string{"Create an event", "Risk: write", "When to use:", "高层创建日程", "Tips:", "start/end 收 ISO 8601"} {
		if !strings.Contains(sc.Long, want) {
			t.Errorf("shortcut Long missing %q:\n%s", want, sc.Long)
		}
	}
	if strings.Contains(sc.Long, "Full parameter schema:") {
		t.Errorf("shortcut Long must not carry a schema pointer:\n%s", sc.Long)
	}

	// No overlay entry -> leave it for the default help path.
	bare := &cobra.Command{Use: "+bare", Short: "x"}
	cmdmeta.SetSource(bare, cmdmeta.SourceShortcut, false)
	cmdmeta.SetAffordanceRef(bare, "calendar", "+bare")
	if PrepareShortcutHelp(bare, nil) {
		t.Error("PrepareShortcutHelp should return false when the shortcut has no overlay")
	}

	// Non-shortcut source is ignored even with a ref.
	notSc := &cobra.Command{Use: "create", Short: "x"}
	cmdmeta.SetAffordanceRef(notSc, "calendar", "+create")
	if PrepareShortcutHelp(notSc, nil) {
		t.Error("PrepareShortcutHelp should return false for a non-shortcut command")
	}
}

// Related-skill pointers are gated on existence: a skill that resolves in the
// skill FS renders, a typo is dropped (never print an unopenable `skills read`),
// and a nil skill FS suppresses the whole block.
func TestRelatedSkillsStatGating(t *testing.T) {
	orig := affordanceLookup
	t.Cleanup(func() { affordanceLookup = orig })
	affordanceLookup = func(_, _ string) (json.RawMessage, bool) {
		return json.RawMessage(`{"use_when":["x"],"skills":["lark-real","lark-typo","lark-real/references/deep.md","lark-real/references/missing.md"]}`), true
	}
	skillFS := fstest.MapFS{
		"lark-real/SKILL.md":           {Data: []byte("# real")},
		"lark-real/references/deep.md": {Data: []byte("# deep")},
	}

	f, _, _, _ := cmdutil.TestFactory(t, testConfig)
	m := map[string]interface{}{"id": "messages.create", "path": "messages", "httpMethod": "POST", "description": "d"}

	cmd := NewCmdServiceMethod(f, imSpec(), meta.FromMap(m), "create", "messages", nil)
	if !PrepareMethodHelp(cmd, skillFS) {
		t.Fatal("PrepareMethodHelp returned false")
	}
	if !strings.Contains(cmd.Long, "skills read lark-real\n") {
		t.Errorf("existing bare-name skill should render on its own line; got:\n%s", cmd.Long)
	}
	if strings.Contains(cmd.Long, "lark-typo") {
		t.Errorf("nonexistent skill must be dropped, not printed as an unopenable pointer; got:\n%s", cmd.Long)
	}
	// A name/relpath reference to an existing file renders; a missing one drops.
	if !strings.Contains(cmd.Long, "skills read lark-real/references/deep.md") {
		t.Errorf("existing reference entry should render; got:\n%s", cmd.Long)
	}
	if strings.Contains(cmd.Long, "references/missing.md") {
		t.Errorf("nonexistent reference must be dropped; got:\n%s", cmd.Long)
	}

	// nil skill FS: the whole Related-skills block is suppressed.
	bare := NewCmdServiceMethod(f, imSpec(), meta.FromMap(m), "create", "messages", nil)
	PrepareMethodHelp(bare, nil)
	if strings.Contains(bare.Long, "Related skills") {
		t.Errorf("nil skillFS should suppress the skills block; got:\n%s", bare.Long)
	}
}

// A shortcut that set a hand-authored Long (as the docs shortcuts do in
// PostMount) keeps it as the lead: the affordance block is appended below, not
// clobbered, and re-rendering does not double-append.
func TestPrepareShortcutHelp_PreservesPostMountLong(t *testing.T) {
	orig := affordanceLookup
	t.Cleanup(func() { affordanceLookup = orig })
	affordanceLookup = func(_, _ string) (json.RawMessage, bool) {
		return json.RawMessage(`{"use_when":["高层创建日程"]}`), true
	}

	const authored = "Custom docs help. AI agents MUST read the skill first."
	sc := &cobra.Command{Use: "+create", Short: "Create", Long: authored}
	cmdmeta.SetSource(sc, cmdmeta.SourceShortcut, false)
	cmdmeta.SetAffordanceRef(sc, "calendar", "+create")

	if !PrepareShortcutHelp(sc, nil) {
		t.Fatal("PrepareShortcutHelp returned false for a shortcut with an overlay")
	}
	if !strings.HasPrefix(sc.Long, authored) {
		t.Errorf("hand-authored Long must lead, not be clobbered; got:\n%s", sc.Long)
	}
	if !strings.Contains(sc.Long, "When to use:") {
		t.Errorf("affordance block should be appended below the base; got:\n%s", sc.Long)
	}
	// Re-render must reuse the captured base, not append the block twice.
	PrepareShortcutHelp(sc, nil)
	if n := strings.Count(sc.Long, "When to use:"); n != 1 {
		t.Errorf("affordance appended %d times across re-renders, want 1:\n%s", n, sc.Long)
	}
}

// domainCmd wires a domain-tagged command with a subcommand under a root, the
// shape PrepareDomainHelp expects.
func domainCmd(short, long string) *cobra.Command {
	root := &cobra.Command{Use: "root"}
	dom := &cobra.Command{Use: "event", Short: short, Long: long}
	cmdmeta.SetDomain(dom, "event")
	dom.AddCommand(&cobra.Command{Use: "consume", Run: func(*cobra.Command, []string) {}})
	root.AddCommand(dom)
	return dom
}

func TestPrepareDomainHelp_PreservesHandAuthoredLong(t *testing.T) {
	const long = "Unified event consumption system. Use 'event consume <EventKey>'."
	dom := domainCmd("Consume and manage real-time events", long)

	if !PrepareDomainHelp(dom, nil) {
		t.Fatal("PrepareDomainHelp returned false for a domain-tagged command")
	}
	if !strings.HasPrefix(dom.Long, long) {
		t.Errorf("hand-authored Long must lead; got:\n%s", dom.Long)
	}
	if !strings.Contains(dom.Long, "Risk levels") {
		t.Errorf("domain guidance should be appended; got:\n%s", dom.Long)
	}

	// Re-rendering must not append the guidance a second time.
	PrepareDomainHelp(dom, nil)
	if n := strings.Count(dom.Long, "Risk levels"); n != 1 {
		t.Errorf("guidance appended %d times across re-renders, want 1:\n%s", n, dom.Long)
	}
}

// A service domain carries only a Short at help time; it seeds the base.
func TestPrepareDomainHelp_FallsBackToShort(t *testing.T) {
	dom := domainCmd("Message and group chat management", "")
	if !PrepareDomainHelp(dom, nil) {
		t.Fatal("PrepareDomainHelp returned false for a domain-tagged command")
	}
	if !strings.HasPrefix(dom.Long, "Message and group chat management") {
		t.Errorf("Short should seed Long when no hand-authored Long exists; got:\n%s", dom.Long)
	}
}
