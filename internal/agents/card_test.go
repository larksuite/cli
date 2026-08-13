// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package agents

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/core"
)

// fakeRT is a no-op Runtime for exercising BuildCard's rt != nil path.
type fakeRT struct{}

func (fakeRT) AgentID() string           { return "" }
func (fakeRT) IsBot() bool               { return false }
func (fakeRT) Params() map[string]string { return nil }
func (fakeRT) CallAPI(context.Context, string, string, map[string]string, any) (json.RawMessage, error) {
	return nil, nil
}
func (fakeRT) CallMultipart(context.Context, string, string, map[string]string, []FilePart) (json.RawMessage, error) {
	return nil, nil
}

func TestCardSupports(t *testing.T) {
	c := &AgentCard{Capabilities: Capabilities{TaskCancel: false, ContextList: true}}
	if c.Supports(CapTaskCancel) {
		t.Error("task_cancel should not be supported")
	}
	if !c.Supports(CapContextList) {
		t.Error("context_list should be supported")
	}
	if c.Supports("nonexistent") {
		t.Error("unknown capability should be treated as unsupported")
	}
	// nil guard branch: a nil receiver is treated as unsupported; a zero-value Capabilities is all false.
	var nilCard *AgentCard
	if nilCard.Supports(CapContextList) {
		t.Error("nil card should be treated as unsupported")
	}
	if (&AgentCard{}).Supports(CapContextList) {
		t.Error("zero-value Capabilities should be treated as unsupported")
	}
	// Each capability constant must map to its own struct field (the switch has no gaps or mismatches).
	all := &AgentCard{Capabilities: Capabilities{
		ArtifactDownload: true, FileInput: true, InputRequired: true,
		ContextList: true, ContextGet: true, ContextDelete: true,
		TaskCancel: true, TaskGet: true, TaskList: true,
	}}
	for _, k := range []string{
		CapArtifactDownload, CapFileInput, CapInputRequired,
		CapContextList, CapContextGet, CapContextDelete,
		CapTaskCancel, CapTaskGet, CapTaskList,
	} {
		if !all.Supports(k) {
			t.Errorf("Supports(%q) should be true when all Capabilities are true", k)
		}
	}
}

// TestDeriveCapabilities pins the crown jewel: capability = wired-hook presence.
func TestDeriveCapabilities(t *testing.T) {
	// Minimal (echo-like): only the core hooks + read verbs.
	min := coreSpec("echo")
	min.ListContexts = ContextListOp{Handler: func(context.Context, Runtime, PageParams) ([]ContextSummary, PageInfo, error) {
		return nil, PageInfo{}, nil
	}}
	c := DeriveCapabilities(&min, core.BrandFeishu)
	if !c.TaskGet {
		t.Error("task_get should be true (GetTask is a mandatory core hook)")
	}
	if !c.ContextList {
		t.Error("context_list should be true (ListContexts wired)")
	}
	// The three context caps are independent: only ListContexts is wired here, so
	// context_get / context_delete stay false (no umbrella multi_turn bit).
	if c.TaskCancel || c.ArtifactDownload || c.TaskList || c.FileInput || c.InputRequired || c.ContextGet || c.ContextDelete {
		t.Errorf("unwired capabilities should be false, got %+v", c)
	}

	// Full (reporter-like): everything wired / declared.
	full := coreSpec("reporter")
	full.ListTasks = TaskListOp{Handler: func(context.Context, Runtime, string, PageParams) ([]TaskSummary, PageInfo, error) {
		return nil, PageInfo{}, nil
	}}
	full.CancelTask = TaskCancelOp{Handler: func(context.Context, Runtime, string) error { return nil }}
	full.ListContexts = ContextListOp{Handler: func(context.Context, Runtime, PageParams) ([]ContextSummary, PageInfo, error) {
		return nil, PageInfo{}, nil
	}}
	full.GetContext = ContextGetOp{Handler: func(context.Context, Runtime, string) (*ContextDetail, error) { return nil, nil }}
	full.DeleteContext = ContextDeleteOp{Handler: func(context.Context, Runtime, string) error { return nil }}
	full.DownloadArtifact = ArtifactDownloadOp{Handler: func(context.Context, Runtime, string, string) (*ArtifactData, error) { return nil, nil }}
	full.FileInput = true
	full.InputRequired = true
	c = DeriveCapabilities(&full, core.BrandFeishu)
	if !(c.TaskGet && c.TaskList && c.TaskCancel && c.ContextList && c.ContextGet && c.ContextDelete && c.ArtifactDownload && c.FileInput && c.InputRequired) {
		t.Errorf("a fully-wired spec should have every capability true, got %+v", c)
	}
}

// TestBuildCardOffline pins that BuildCard with rt=nil fills registration
// metadata + derived caps + static per-agent metadata, always offline (Describe
// is never invoked without a runtime).
func TestBuildCardOffline(t *testing.T) {
	prov := catalogProvider("nc", "a1")
	prov.Identities = []IdentitySpec{{Type: IdentityBot, Precondition: "requires an allowlist entry"}}
	prov.Catalog[0].Describe = func(context.Context, Runtime) (*CardInfo, error) {
		return &CardInfo{Name: "REMOTE"}, nil // must NOT be called with rt=nil
	}
	spec := &prov.Catalog[0]

	card := BuildCard(context.Background(), prov, spec, "a1", core.BrandFeishu, nil)
	if card.Provider != "nc" || card.AgentID != "a1" {
		t.Fatalf("provider/agent_id: %+v", card)
	}
	if card.ProviderLabel != prov.Label || card.AgentIDSource != prov.AgentIDSource {
		t.Fatalf("registration metadata should be pre-filled: %+v", card)
	}
	if len(card.Identity) != 1 || card.Identity[0].Type != IdentityBot {
		t.Fatalf("identity should come from the provider: %+v", card.Identity)
	}
	if card.HasParameters == nil || len(card.HasParameters) != 0 {
		t.Fatalf("has_parameters should be empty but non-nil (always emit []): %#v", card.HasParameters)
	}
	if !card.Capabilities.TaskGet {
		t.Error("task_get should be derived true")
	}
	if card.Name != "name-a1" {
		t.Errorf("offline card should use the static spec Name (not the rt=nil Describe), got %q", card.Name)
	}
}

// TestBuildCardDynamicDescribe pins the rt != nil path: Describe enriches
// Name/Description when it succeeds, and a Describe error is swallowed so the
// card degrades to the offline version (best-effort).
func TestBuildCardDynamicDescribe(t *testing.T) {
	prov := instanceProvider("dyn") // instance spec: no static Name
	prov.Instance.Describe = func(context.Context, Runtime) (*CardInfo, error) {
		return &CardInfo{Name: "Remote Name", Description: "Remote Desc"}, nil
	}
	card := BuildCard(context.Background(), prov, prov.Instance, "agt_x", core.BrandFeishu, fakeRT{})
	if card.Name != "Remote Name" || card.Description != "Remote Desc" {
		t.Errorf("rt != nil + Describe should enrich the card, got name=%q desc=%q", card.Name, card.Description)
	}

	// A Describe error degrades to the offline card (no enrichment), never fails.
	prov.Instance.Describe = func(context.Context, Runtime) (*CardInfo, error) {
		return nil, errs.NewInternalError(errs.SubtypeUnknown, "describe boom")
	}
	card = BuildCard(context.Background(), prov, prov.Instance, "agt_x", core.BrandFeishu, fakeRT{})
	if card.Name != "" {
		t.Errorf("a Describe error should be swallowed → offline card (instance has no static Name), got name=%q", card.Name)
	}
	if !card.Capabilities.TaskGet {
		t.Error("caps should still be present on the degraded card")
	}
}
