// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package service

import (
	"encoding/json"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/larksuite/cli/internal/affordance"
	"github.com/larksuite/cli/internal/cmdmeta"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/imcontract"
	"github.com/larksuite/cli/internal/meta"
	"github.com/larksuite/cli/internal/recovery"
	"github.com/larksuite/cli/internal/skillref"
	"github.com/larksuite/cli/internal/surface"
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

func TestPrepareMethodHelpProjectsConcealedSchemaPointer(t *testing.T) {
	orig := affordanceLookup
	t.Cleanup(func() { affordanceLookup = orig })
	affordanceLookup = func(_, _ string) (json.RawMessage, bool) {
		return json.RawMessage(`{"use_when":["发文本消息"]}`), true
	}

	f, _, _, _ := cmdutil.TestFactory(t, testConfig)
	m := map[string]interface{}{
		"id": "messages.create", "path": "messages", "httpMethod": "POST",
		"description": "发送消息",
	}
	cmd := NewCmdServiceMethod(f, imSpec(), meta.FromMap(m), "create", "messages", nil)
	plan := surface.NewPlan(map[surface.CommandID]surface.CommandState{
		surface.CommandSchema: surface.CommandConcealed,
	})
	projector := recovery.NewProjector(func() *surface.Plan { return plan })

	if !PrepareMethodHelpWithProjection(cmd, nil, nil, func() bool {
		return projector.CanReference(recovery.TargetSchema)
	}) {
		t.Fatal("PrepareMethodHelpWithProjection returned false for a service-method command")
	}
	if strings.Contains(cmd.Long, "lark-cli schema") ||
		strings.Contains(cmd.Long, "Full parameter schema:") {
		t.Fatalf("concealed schema left a dead method-help pointer:\n%s", cmd.Long)
	}
	for _, want := range []string{"发送消息", "When to use:", "发文本消息"} {
		if !strings.Contains(cmd.Long, want) {
			t.Errorf("schema projection removed unrelated help %q:\n%s", want, cmd.Long)
		}
	}
}

func TestPrepareMethodHelpPreservesAffordanceAndAddsContractOnce(t *testing.T) {
	orig := affordanceLookup
	t.Cleanup(func() { affordanceLookup = orig })
	affordanceLookup = func(_, _ string) (json.RawMessage, bool) {
		return json.RawMessage(`{
			"use_when":["forward one message"],
			"avoid_when":["a new send is required"],
			"prerequisites":["source message is visible"],
			"examples":[{"description":"forward","command":"lark-cli im messages forward ..."}],
			"skills":["lark-im"]
		}`), true
	}
	skillFS := fstest.MapFS{"lark-im/SKILL.md": {Data: []byte("# IM")}}
	f, _, _, _ := cmdutil.TestFactory(t, testConfig)
	m := map[string]interface{}{
		"id": "chat.moderation.update", "path": "chats/{chat_id}/moderation", "httpMethod": "PUT", "description": "Update moderation",
	}
	cmd := NewCmdServiceMethod(f, imSpec(), meta.FromMap(m), "update", "chat.moderation", nil)
	if strings.Contains(cmd.Long, imcontract.HelpAcceptanceOnly.Text()) {
		t.Fatalf("contract help must stay lazy at build time:\n%s", cmd.Long)
	}

	for range 2 {
		if !PrepareMethodHelp(cmd, skillFS) {
			t.Fatal("PrepareMethodHelp returned false")
		}
	}
	for _, want := range []string{
		"When to use:", "Avoid when:", "Prerequisites:", "Examples:",
		"Related skills", "Full parameter schema:",
		imcontract.HelpAcceptanceOnly.Text(),
	} {
		if n := strings.Count(cmd.Long, want); n != 1 {
			t.Fatalf("%q appears %d times, want once:\n%s", want, n, cmd.Long)
		}
	}
	contractAt := strings.Index(cmd.Long, imcontract.HelpAcceptanceOnly.Text())
	schemaAt := strings.Index(cmd.Long, "Full parameter schema:")
	if contractAt < 0 || schemaAt < 0 || contractAt > schemaAt {
		t.Fatalf("contract help must precede schema pointer:\n%s", cmd.Long)
	}
}

func TestModerationGetHelpAdvertisesPaginationCompleteness(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, testConfig)
	m := map[string]interface{}{
		"id": "chat.moderation.get", "path": "chats/{chat_id}/moderation", "httpMethod": "GET",
		"description": "Get moderation", "risk": "read",
		"parameters": map[string]interface{}{
			"chat_id":    map[string]interface{}{"type": "string", "location": "path", "required": true},
			"page_token": map[string]interface{}{"type": "string", "location": "query"},
		},
	}
	cmd := NewCmdServiceMethod(f, imSpec(), meta.FromMap(m), "get", "chat.moderation", nil)
	if flag := cmd.Flags().Lookup("page-all"); flag == nil || flag.Hidden {
		t.Fatalf("moderation get must expose --page-all: %#v", flag)
	}
	if !PrepareMethodHelp(cmd, nil) {
		t.Fatal("PrepareMethodHelp returned false")
	}
	if !strings.Contains(cmd.Long, imcontract.HelpCompleteness.Text()) {
		t.Fatalf("moderation get help omitted completeness contract:\n%s", cmd.Long)
	}
}

// PrepareShortcutHelp composes a shortcut's Long from its overlay (without a
// schema pointer), preserves the selected tips on the command for the root help
// renderer, and leaves shortcuts without an overlay entry (and non-shortcut
// commands) for the default help path.
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
	for _, want := range []string{"Create an event", "When to use:", "高层创建日程"} {
		if !strings.Contains(sc.Long, want) {
			t.Errorf("shortcut Long missing %q:\n%s", want, sc.Long)
		}
	}
	for _, unwanted := range []string{"Risk: write", "Tips:", "start/end 收 ISO 8601"} {
		if strings.Contains(sc.Long, unwanted) {
			t.Errorf("shortcut Long must leave %q for the root tail renderer:\n%s", unwanted, sc.Long)
		}
	}
	if got := cmdutil.GetTips(sc); len(got) != 1 || got[0] != "start/end 收 ISO 8601" {
		t.Fatalf("shortcut tips = %#v, want the declarative tip preserved for tail rendering", got)
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

func TestPrepareShortcutHelpStoresOverlayTipsForTailOnce(t *testing.T) {
	orig := affordanceLookup
	t.Cleanup(func() { affordanceLookup = orig })
	affordanceLookup = func(_, _ string) (json.RawMessage, bool) {
		return json.RawMessage(`{"use_when":["create"],"tips":["overlay tip"]}`), true
	}

	sc := &cobra.Command{Use: "+create", Short: "Create"}
	cmdmeta.SetSource(sc, cmdmeta.SourceShortcut, false)
	cmdmeta.SetAffordanceRef(sc, "calendar", "+create")
	cmdutil.SetTips(sc, []string{"declarative tip"})

	for range 2 {
		if !PrepareShortcutHelp(sc, nil) {
			t.Fatal("PrepareShortcutHelp returned false")
		}
	}
	if strings.Contains(sc.Long, "overlay tip") || strings.Contains(sc.Long, "Tips:") {
		t.Fatalf("overlay tips must be left for the common tail renderer:\n%s", sc.Long)
	}
	if got := cmdutil.GetTips(sc); len(got) != 1 || got[0] != "overlay tip" {
		t.Fatalf("tips = %#v, want overlay tip once", got)
	}
}

func TestPrepareShortcutHelpAddsContractWithoutAffordance(t *testing.T) {
	sc := &cobra.Command{
		Use: "+chat-list", Short: "List chats",
		Run: func(*cobra.Command, []string) {},
	}
	cmdmeta.SetSource(sc, cmdmeta.SourceShortcut, false)
	cmdmeta.SetAffordanceRef(sc, "im", "+chat-list")
	cmdutil.SetRisk(sc, "read")
	imcontract.AnnotateHelpContract(sc, "im +chat-list")

	for range 2 {
		if !PrepareShortcutHelp(sc, nil) {
			t.Fatal("PrepareShortcutHelp returned false for contract-bearing shortcut")
		}
	}
	if n := strings.Count(sc.Long, imcontract.HelpCompleteness.Text()); n != 1 {
		t.Fatalf("contract help appears %d times, want once:\n%s", n, sc.Long)
	}
	if sc.Short != "List chats" || !strings.HasPrefix(sc.Long, "List chats") {
		t.Fatalf("visible description changed: Short=%q Long=%q", sc.Short, sc.Long)
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

func TestDomainSkillReferenceRequiresReadableCommandSurface(t *testing.T) {
	content := fstest.MapFS{
		"lark-im/SKILL.md": {Data: []byte("# im")},
	}
	resolver, err := skillref.New(content, nil)
	if err != nil {
		t.Fatalf("skillref.New(): %v", err)
	}
	root := &cobra.Command{Use: "lark-cli"}
	domain := &cobra.Command{Use: "im", Short: "IM"}
	cmdmeta.SetSource(domain, cmdmeta.SourceService, false)
	domain.AddCommand(&cobra.Command{Use: "messages", Run: func(*cobra.Command, []string) {}})
	root.AddCommand(domain)

	if !PrepareDomainHelpWithReferences(domain, nil, resolver) {
		t.Fatal("PrepareDomainHelp returned false")
	}
	if strings.Contains(domain.Long, "skills read") {
		t.Fatalf("concealed skills/read leaked through resolver:\n%s", domain.Long)
	}
}

func TestDomainSkillReferenceUsesDeclaredAffordanceName(t *testing.T) {
	affordance.SetSource(fstest.MapFS{
		"docs.md": {Data: []byte("# docs\n> skill: lark-doc\n")},
	})
	t.Cleanup(func() { affordance.SetSource(nil) })
	content := fstest.MapFS{
		"lark-doc/SKILL.md": {Data: []byte("# docs")},
	}
	resolver, err := skillref.New(content, nil)
	if err != nil {
		t.Fatalf("skillref.New(): %v", err)
	}
	root := &cobra.Command{Use: "lark-cli"}
	domain := &cobra.Command{Use: "docs", Short: "Docs"}
	cmdmeta.SetSource(domain, cmdmeta.SourceService, false)
	cmdmeta.SetDomain(domain, "docs")
	domain.AddCommand(&cobra.Command{Use: "documents", Run: func(*cobra.Command, []string) {}})
	root.AddCommand(domain)

	if !PrepareDomainHelpWithReferences(domain, content, resolver) {
		t.Fatal("PrepareDomainHelp returned false")
	}
	if !strings.Contains(domain.Long, "skills read lark-doc") {
		t.Fatalf("declared domain skill was not used:\n%s", domain.Long)
	}
	if strings.Contains(domain.Long, "skills read lark-docs") {
		t.Fatalf("command-name inference overrode declared domain skill:\n%s", domain.Long)
	}
}

func TestPrepareDomainHelpDisplaysConfiguredSkills(t *testing.T) {
	affordance.SetSource(fstest.MapFS{
		"docs.md": {Data: []byte("# docs\n> skill: lark-doc\n\n## Skills\n- lark-drive\n- lark-doc\n- lark-missing\n")},
	})
	t.Cleanup(func() { affordance.SetSource(nil) })
	content := fstest.MapFS{
		"lark-doc/SKILL.md":   {Data: []byte("# docs")},
		"lark-drive/SKILL.md": {Data: []byte("# drive")},
	}

	root := &cobra.Command{Use: "lark-cli"}
	domain := &cobra.Command{Use: "docs", Short: "Docs"}
	cmdmeta.SetSource(domain, cmdmeta.SourceService, false)
	cmdmeta.SetDomain(domain, "docs")
	domain.AddCommand(&cobra.Command{Use: "documents", Run: func(*cobra.Command, []string) {}})
	root.AddCommand(domain)

	if !PrepareDomainHelp(domain, content) {
		t.Fatal("PrepareDomainHelp returned false")
	}
	if !strings.Contains(domain.Long, "Domain skills (concepts, command choice, conventions):") {
		t.Fatalf("multiple configured skills did not use the domain list form:\n%s", domain.Long)
	}
	docAt := strings.Index(domain.Long, "skills read lark-doc")
	driveAt := strings.Index(domain.Long, "skills read lark-drive")
	if docAt < 0 || driveAt < 0 || docAt >= driveAt {
		t.Fatalf("configured domain skills are missing or out of order (doc=%d drive=%d):\n%s", docAt, driveAt, domain.Long)
	}
	if strings.Contains(domain.Long, "lark-missing") {
		t.Fatalf("unresolvable domain skill must be omitted:\n%s", domain.Long)
	}

	remappedContent := fstest.MapFS{
		"acme-docx/SKILL.md":  {Data: []byte("# docs")},
		"acme-drive/SKILL.md": {Data: []byte("# drive")},
	}
	resolver, err := skillref.New(remappedContent, []skillref.Mapping{
		{From: skillref.Ref{Skill: "lark-doc"}, To: skillref.Ref{Skill: "acme-docx"}},
		{From: skillref.Ref{Skill: "lark-drive"}, To: skillref.Ref{Skill: "acme-drive"}},
	})
	if err != nil {
		t.Fatalf("skillref.New(): %v", err)
	}
	remapped := &cobra.Command{Use: "docs", Short: "Docs"}
	cmdmeta.SetSource(remapped, cmdmeta.SourceService, false)
	cmdmeta.SetDomain(remapped, "docs")
	remapped.AddCommand(&cobra.Command{Use: "documents", Run: func(*cobra.Command, []string) {}})
	remappedRoot := &cobra.Command{Use: "lark-cli"}
	remappedRoot.AddCommand(remapped)

	if !PrepareDomainHelpWithReferences(remapped, remappedContent, resolver) {
		t.Fatal("PrepareDomainHelpWithReferences returned false")
	}
	for _, want := range []string{"skills read acme-docx", "skills read acme-drive"} {
		if !strings.Contains(remapped.Long, want) {
			t.Errorf("remapped domain help missing %q:\n%s", want, remapped.Long)
		}
	}
	for _, canonical := range []string{"skills read lark-doc", "skills read lark-drive"} {
		if strings.Contains(remapped.Long, canonical) {
			t.Errorf("remapped domain help leaked canonical pointer %q:\n%s", canonical, remapped.Long)
		}
	}
}

// A shortcut that sets a hand-authored Long keeps it as the lead: the
// affordance block is appended below, not clobbered, and re-rendering does not
// double-append.
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
// The domain-guide pointer is likewise gated: removing the domain's skill
// drops the pointer instead of leaving it dangling.
func TestPrepareDomainHelp_GatesGuidePointerOnFS(t *testing.T) {
	present := domainCmd("Consume and manage real-time events", "")
	if !PrepareDomainHelp(present, fstest.MapFS{"lark-event/SKILL.md": &fstest.MapFile{Data: []byte("x")}}) {
		t.Fatal("PrepareDomainHelp returned false for a domain-tagged command")
	}
	const want = "Consume and manage real-time events\n\n" +
		"Risk levels (read | write | high-risk-write) appear in each command's --help; high-risk-write requires --yes, only after the user confirms.\n\n" +
		"Domain guide (concepts, command choice, conventions): lark-cli skills read lark-event"
	if present.Long != want {
		t.Errorf("single-skill domain help changed\n got: %q\nwant: %q", present.Long, want)
	}

	removed := domainCmd("Consume and manage real-time events", "")
	if !PrepareDomainHelp(removed, fstest.MapFS{}) {
		t.Fatal("PrepareDomainHelp returned false for a domain-tagged command")
	}
	if strings.Contains(removed.Long, "skills read lark-event") {
		t.Errorf("removed skill must leave no domain-guide pointer; got:\n%s", removed.Long)
	}
}

func TestPrepareDomainHelp_FallsBackToShort(t *testing.T) {
	dom := domainCmd("Message and group chat management", "")
	if !PrepareDomainHelp(dom, nil) {
		t.Fatal("PrepareDomainHelp returned false for a domain-tagged command")
	}
	if !strings.HasPrefix(dom.Long, "Message and group chat management") {
		t.Errorf("Short should seed Long when no hand-authored Long exists; got:\n%s", dom.Long)
	}
}
