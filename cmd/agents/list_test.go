// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package agents

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	iagents "github.com/larksuite/cli/internal/agents"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/output"
)

// listFactory returns a Factory writing to a fresh stdout buffer plus a
// listOptions bound to it, ready to drive agentListRun without any API.
func listFactory() (*listOptions, *bytes.Buffer) {
	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	f := &cmdutil.Factory{IOStreams: &cmdutil.IOStreams{Out: out, ErrOut: errOut}}
	return &listOptions{Factory: f, Format: "json"}, out
}

// decodeProviders unmarshals the envelope on out and returns data.providers.
func decodeProviders(t *testing.T, out *bytes.Buffer) []interface{} {
	t.Helper()
	var env output.Envelope
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("output should be valid envelope JSON: %v (%s)", err, out.String())
	}
	data, _ := env.Data.(map[string]interface{})
	providers, _ := data["providers"].([]interface{})
	return providers
}

// findProvider returns the provider entry whose scheme matches, or nil.
func findProvider(providers []interface{}, scheme string) map[string]interface{} {
	for _, pv := range providers {
		p, _ := pv.(map[string]interface{})
		if p["scheme"] == scheme {
			return p
		}
	}
	return nil
}

// TestAgentListRun_ProviderFieldsV2 pins the provider entry contract: the
// base entry carries all fields sourced from iagents.Info (the single source
// of truth), the legacy free-text description field is gone, and discoverable
// is no longer exposed.
func TestAgentListRun_ProviderFieldsV2(t *testing.T) {
	opts, out := listFactory()
	if err := agentListRun(opts); err != nil {
		t.Fatalf("list should not error: %v", err)
	}
	prov, ok := iagents.Info("base")
	if !ok {
		t.Fatal("the base provider should already be registered (top-level agents blank import)")
	}
	p := findProvider(decodeProviders(t, out), "base")
	if p == nil {
		t.Fatalf("list should include the base provider: %s", out.String())
	}
	if p["label"] != prov.Label {
		t.Errorf("label should come from Provider.Label %q, got %v", prov.Label, p["label"])
	}
	if p["agent_ref_format"] != prov.AgentRefFormat() {
		t.Errorf("agent_ref_format should come from Provider.AgentRefFormat() %q, got %v", prov.AgentRefFormat(), p["agent_ref_format"])
	}
	if p["kind"] != string(prov.Kind()) {
		t.Errorf("kind should come from Provider.Kind() %q, got %v", prov.Kind(), p["kind"])
	}
	if p["agent_id_source"] != prov.AgentIDSource {
		t.Errorf("agent_id_source should come from Provider.AgentIDSource, got %v", p["agent_id_source"])
	}
	if _, present := p["description"]; present {
		t.Errorf("the old description field should be removed (double-source with label), got %v", p)
	}
	if _, present := p["discoverable"]; present {
		t.Errorf("the discoverable field should be removed from the provider list, got %v", p["discoverable"])
	}
}

// TestAgentListRun_EnvelopeShape verifies the JSON envelope carries
// data.providers[] with the full field contract.
func TestAgentListRun_EnvelopeShape(t *testing.T) {
	opts, out := listFactory()
	if err := agentListRun(opts); err != nil {
		t.Fatalf("list should not error: %v", err)
	}
	var env output.Envelope
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("output should be valid envelope JSON: %v (%s)", err, out.String())
	}
	if !env.OK {
		t.Errorf("ok should be true: %+v", env)
	}
	providers := decodeProviders(t, out)
	if len(providers) == 0 {
		t.Fatalf("data.providers should be a non-empty array: %s", out.String())
	}
	first, ok := providers[0].(map[string]interface{})
	if !ok {
		t.Fatalf("provider entry should be an object, got %T", providers[0])
	}
	for _, key := range []string{"scheme", "label", "agent_ref_format", "kind", "agent_id_source"} {
		if _, present := first[key]; !present {
			t.Errorf("provider entry missing field %q: %v", key, first)
		}
	}
	if _, present := first["discoverable"]; present {
		t.Errorf("provider entry should not contain a discoverable field: %v", first)
	}
}

// TestAgentListDefaultFormatIsJSON pins the default flip: `agents list`
// without --format emits the JSON envelope (pretty is opt-in).
func TestAgentListDefaultFormatIsJSON(t *testing.T) {
	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	f := &cmdutil.Factory{IOStreams: &cmdutil.IOStreams{Out: out, ErrOut: errOut}}
	cmd := NewCmdAgentList(f)
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("agents list should not error: %v", err)
	}
	var env output.Envelope
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("default output should be a JSON envelope: %v (%s)", err, out.String())
	}
	if !env.OK {
		t.Errorf("ok should be true: %+v", env)
	}
}

// TestAgentListRun_PrettyFormat pins the opt-in --format pretty branch: a header
// row plus tab-separated provider lines, not a JSON envelope.
func TestAgentListRun_PrettyFormat(t *testing.T) {
	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	f := &cmdutil.Factory{IOStreams: &cmdutil.IOStreams{Out: out, ErrOut: errOut}}
	opts := &listOptions{Factory: f, Format: "pretty"}

	if err := agentListRun(opts); err != nil {
		t.Fatalf("list pretty should not error: %v", err)
	}
	text := out.String()
	// A pretty rendering is human text, not a JSON envelope.
	var env output.Envelope
	if json.Unmarshal(out.Bytes(), &env) == nil && env.OK {
		t.Fatalf("pretty format should not output a JSON envelope: %s", text)
	}
	if !strings.HasPrefix(text, "SCHEME") {
		t.Errorf("pretty output should start with a header row: %s", text)
	}
	if !strings.Contains(text, "base") {
		t.Errorf("pretty output should contain the base provider: %s", text)
	}
	if !strings.Contains(text, "base:<agent_id>") {
		t.Errorf("pretty output should contain the base ref format: %s", text)
	}
	// agent_id_source is surfaced as a footer (not a column) so the newcomer's
	// "where do I get an agent_id" cue does not disappear in the pretty view.
	if !strings.Contains(text, "agent_id source") {
		t.Errorf("pretty output should contain the agent_id_source footer hint: %s", text)
	}
}

// TestAgentListScheme_UnsupportedCapability pins that `agents list fakeflow`
// on a provider without Discoverer is unsupported_capability (exit 2) with the
// AgentIDSource text as hint, and — because the probe runs before any client
// construction — works on an unconfigured Factory.
func TestAgentListScheme_UnsupportedCapability(t *testing.T) {
	registerScripted()
	opts, _ := listFactory()
	opts.Scheme = "fakeflow"
	err := agentListRun(opts)
	if err == nil {
		t.Fatal("fakeflow does not implement Discoverer, so list fakeflow should error")
	}
	if !errs.IsValidation(err) {
		t.Fatalf("should be a validation error, got %T (%v)", err, err)
	}
	if code := output.ExitCodeOf(err); code != output.ExitValidation {
		t.Fatalf("exit code should be 2, got %d", code)
	}
	p, ok := errs.ProblemOf(err)
	if !ok || p.Subtype != errs.Subtype("unsupported_capability") {
		t.Fatalf("subtype should be unsupported_capability, got %+v", p)
	}
	if !strings.Contains(err.Error(), "provider 'fakeflow' does not support listing agents") {
		t.Errorf("message should state that listing is not supported, got %q", err.Error())
	}
	if !strings.Contains(p.Hint, fakeflowAgentIDSource) {
		t.Errorf("hint should be the AgentIDSource text, got %q", p.Hint)
	}
}

// TestAgentListScheme_UnknownScheme pins that an unregistered scheme is
// invalid_argument and the message lists the registered schemes.
func TestAgentListScheme_UnknownScheme(t *testing.T) {
	opts, _ := listFactory()
	opts.Scheme = "nosuch"
	err := agentListRun(opts)
	if err == nil {
		t.Fatal("an unknown scheme should error")
	}
	if !errs.IsValidation(err) {
		t.Fatalf("should be a validation error, got %T (%v)", err, err)
	}
	p, ok := errs.ProblemOf(err)
	if !ok || p.Subtype != errs.SubtypeInvalidArgument {
		t.Fatalf("subtype should be invalid_argument, got %+v", p)
	}
	if !strings.Contains(err.Error(), "nosuch") || !strings.Contains(err.Error(), "base") {
		t.Errorf("message should contain the unknown scheme and the registered scheme list, got %q", err.Error())
	}
	// Hand-written validation errors carry a recovery hint pointing at
	// `agents list` for provider discovery.
	if !strings.Contains(p.Hint, "agents list") {
		t.Errorf("unknown-scheme hint should point to `agents list`, got %q", p.Hint)
	}
}

// catSpec builds a catalog AgentSpec with the mandatory core hooks (the list
// tests only exercise enumeration, never Send/GetTask, but Register requires
// both non-nil).
func catSpec(id, name, desc string) iagents.AgentSpec {
	return iagents.AgentSpec{
		ID: id, Name: name, Description: desc,
		Send:    iagents.SendOp{Handler: func(context.Context, iagents.Runtime, iagents.SendInput) (*iagents.AgentTask, error) { return nil, nil }},
		GetTask: iagents.TaskGetOp{Handler: func(context.Context, iagents.Runtime, string) (*iagents.AgentTask, error) { return nil, nil }},
	}
}

// registerFakeDisc registers a catalog scheme with two entries. Its enumeration
// is derived offline from the static Catalog. It leaks into the package-level
// registry for the rest of this package run.
func registerFakeDisc() {
	iagents.Register(iagents.Provider{
		Scheme:        "fakedisc",
		Label:         "test fake (catalog)",
		AgentIDSource: "test only",
		Identities:    []iagents.IdentitySpec{{Type: iagents.IdentityUser}},
		Catalog: []iagents.AgentSpec{
			catSpec("a1", "Agent One", "the first"),
			catSpec("a2", "Agent Two", ""),
		},
	})
}

// TestAgentListScheme_CatalogListsAgents pins the catalog positive path: a
// catalog provider enumerates its static entries offline into
// {agents:[AgentSummary...]} + meta.count (sorted by AgentRef).
func TestAgentListScheme_CatalogListsAgents(t *testing.T) {
	registerFakeDisc()
	cfg := &core.CliConfig{AppID: "cli_x", AppSecret: "fake-secret", Brand: core.BrandFeishu}
	f, _, _, _ := cmdutil.TestFactory(t, cfg)
	cmd := &cobra.Command{Use: "list"}
	cmd.SetContext(context.Background())
	opts := &listOptions{Factory: f, Cmd: cmd, Format: "json", Scheme: "fakedisc"}
	out := f.IOStreams.Out.(interface{ Bytes() []byte })

	if err := agentListRun(opts); err != nil {
		t.Fatalf("list fakedisc should not error: %v", err)
	}
	var env output.Envelope
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("output should be valid envelope JSON: %v (%s)", err, string(out.Bytes()))
	}
	data, _ := env.Data.(map[string]interface{})
	agents, ok := data["agents"].([]interface{})
	if !ok || len(agents) != 2 {
		t.Fatalf("data.agents should have 2 entries, got %v", data["agents"])
	}
	first, _ := agents[0].(map[string]interface{})
	if first["agent_ref"] != "fakedisc:a1" || first["name"] != "Agent One" {
		t.Errorf("agents[0] should be an AgentSummary {agent_ref, name}, got %v", first)
	}
	// A catalog enumeration is offline and unpaginated, so it goes through
	// listMeta (meta.count), not listMetaPage (meta.pagination).
	if env.Meta == nil || env.Meta.Count != 2 {
		t.Errorf("meta.count should be 2, got %+v", env.Meta)
	}
}

// TestAgentListScheme_InstanceListAgentsOnline pins the instance online path: an
// instance provider that wires the optional ListAgents hook enumerates via it,
// and the hook receives an identity-pinned runtime (not nil).
func TestAgentListScheme_InstanceListAgentsOnline(t *testing.T) {
	var gotRT iagents.Runtime
	spec := catSpec("", "", "")
	iagents.Register(iagents.Provider{
		Scheme:        "fakelive",
		Label:         "test fake (instance live-enum)",
		AgentIDSource: "test only",
		Identities:    []iagents.IdentitySpec{{Type: iagents.IdentityUser}, {Type: iagents.IdentityBot}},
		Instance:      &spec,
		ListAgents: func(_ context.Context, rt iagents.Runtime, _ iagents.PageParams) ([]iagents.AgentSummary, iagents.PageInfo, error) {
			gotRT = rt
			return []iagents.AgentSummary{{AgentRef: "fakelive:x", Name: "Live X"}}, iagents.PageInfo{}, nil
		},
	})

	cfg := &core.CliConfig{AppID: "cli_x", AppSecret: "fake-secret", Brand: core.BrandFeishu}
	f, _, _, _ := cmdutil.TestFactory(t, cfg)
	cmd := &cobra.Command{Use: "list"}
	cmd.Flags().String("as", "", "identity")
	cmd.SetContext(context.Background())
	opts := &listOptions{Factory: f, Cmd: cmd, Format: "json", Scheme: "fakelive", PageSize: defaultPageSize}
	out := f.IOStreams.Out.(interface{ Bytes() []byte })

	if err := agentListRun(opts); err != nil {
		t.Fatalf("list fakelive should not error: %v", err)
	}
	if gotRT == nil {
		t.Error("the ListAgents hook should receive a non-nil identity-pinned runtime")
	}
	var env output.Envelope
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("output should be valid envelope JSON: %v (%s)", err, string(out.Bytes()))
	}
	data, _ := env.Data.(map[string]interface{})
	if agents, _ := data["agents"].([]interface{}); len(agents) != 1 {
		t.Fatalf("data.agents should have 1 entry, got %v", data["agents"])
	}
}

// TestAgentListScheme_PaginationMeta pins the command-level pagination envelope
// for the instance `list <scheme>` path: a ListAgents hook that returns a page
// plus PageInfo{HasMore,NextToken} surfaces as meta.has_more / meta.page_token,
// and meta.next carries a "next page" action replaying the scheme with
// --page-size / --page-token.
func TestAgentListScheme_PaginationMeta(t *testing.T) {
	spec := catSpec("", "", "")
	iagents.Register(iagents.Provider{
		Scheme:        "fakelivepage",
		Label:         "test fake (instance paginated live-enum)",
		AgentIDSource: "test only",
		Identities:    []iagents.IdentitySpec{{Type: iagents.IdentityUser}, {Type: iagents.IdentityBot}},
		Instance:      &spec,
		ListAgents: func(_ context.Context, _ iagents.Runtime, page iagents.PageParams) ([]iagents.AgentSummary, iagents.PageInfo, error) {
			if page.Size != 2 {
				t.Errorf("the ListAgents hook should receive the requested page size 2, got %d", page.Size)
			}
			return []iagents.AgentSummary{
					{AgentRef: "fakelivepage:x", Name: "Live X"},
					{AgentRef: "fakelivepage:y", Name: "Live Y"},
				},
				iagents.PageInfo{NextToken: "2", HasMore: true}, nil
		},
	})

	cfg := &core.CliConfig{AppID: "cli_x", AppSecret: "fake-secret", Brand: core.BrandFeishu}
	f, _, _, _ := cmdutil.TestFactory(t, cfg)
	cmd := &cobra.Command{Use: "list"}
	cmd.Flags().String("as", "", "identity")
	cmd.SetContext(context.Background())
	opts := &listOptions{Factory: f, Cmd: cmd, Format: "json", Scheme: "fakelivepage", PageSize: 2}
	out := f.IOStreams.Out.(interface{ Bytes() []byte })

	if err := agentListRun(opts); err != nil {
		t.Fatalf("paged list fakelivepage should not error: %v", err)
	}
	var env output.Envelope
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("output should be valid envelope JSON: %v (%s)", err, string(out.Bytes()))
	}
	if env.Meta == nil {
		t.Fatal("a paged list should carry meta")
	}
	if env.Meta.Pagination == nil {
		t.Fatal("a paged list should carry meta.pagination")
	}
	if env.Meta.Pagination.Complete {
		t.Error("meta.pagination.complete should be false while a next page exists")
	}
	if env.Meta.Pagination.NextToken != "2" {
		t.Errorf("meta.pagination.next_token should be the next cursor \"2\", got %q", env.Meta.Pagination.NextToken)
	}
	found := false
	for _, n := range env.Meta.Next {
		if n.Label == "next page" && strings.Contains(n.Command, "lark-cli agents list fakelivepage") &&
			strings.Contains(n.Command, "--page-size 2") && strings.Contains(n.Command, "--page-token 2") {
			found = true
		}
	}
	if !found {
		t.Errorf("meta.next should contain a next page action replaying the scheme + --page-size/--page-token, got %+v", env.Meta.Next)
	}
}

// TestAgentListScheme_OnlineRunsScopePreflight pins #8: the online enumeration
// path now runs the same all-or-nothing scope preflight every other online verb
// runs. An instance provider whose user identity declares scopes, driven by a
// user whose token lacks them, fails fast with missing_scope (exit 3) BEFORE
// ListAgents is called.
func TestAgentListScheme_OnlineRunsScopePreflight(t *testing.T) {
	called := false
	spec := catSpec("", "", "")
	iagents.Register(iagents.Provider{
		Scheme:        "fakescopelive",
		Label:         "test fake (scoped live-enum)",
		AgentIDSource: "test only",
		Identities:    []iagents.IdentitySpec{{Type: iagents.IdentityUser, Scopes: []string{"live:read"}}},
		Instance:      &spec,
		ListAgents: func(context.Context, iagents.Runtime, iagents.PageParams) ([]iagents.AgentSummary, iagents.PageInfo, error) {
			called = true
			return nil, iagents.PageInfo{}, nil
		},
	})
	// The stored user token holds an unrelated scope (non-empty so the preflight
	// actually runs) but not the required one.
	swapStoredScopes(t, []string{"unrelated:scope"})

	cfg := &core.CliConfig{AppID: "cli_x", AppSecret: "fake-secret", Brand: core.BrandFeishu}
	f, _, _, _ := cmdutil.TestFactory(t, cfg)
	opts := &listOptions{Factory: f, Cmd: resolveCmd(t, true, "user"), Format: "json", Scheme: "fakescopelive", As: "user", PageSize: defaultPageSize}

	err := agentListRun(opts)
	if err == nil {
		t.Fatal("listing as a user missing the required scope should fail with missing_scope")
	}
	if code := output.ExitCodeOf(err); code != 3 {
		t.Fatalf("missing scope should be exit 3, got %d (%v)", code, err)
	}
	p, ok := errs.ProblemOf(err)
	if !ok || p.Subtype != errs.SubtypeMissingScope {
		t.Fatalf("subtype should be missing_scope, got %+v", p)
	}
	if called {
		t.Error("ListAgents must NOT be called when the scope preflight fails")
	}
}

// TestAgentListScheme_OnlineChecksIdentity pins #8: the online enumeration path
// enforces the user|bot identity whitelist. An explicitly unsupported --as is
// rejected as a validation error before the online ListAgents call.
func TestAgentListScheme_OnlineChecksIdentity(t *testing.T) {
	called := false
	spec := catSpec("", "", "")
	iagents.Register(iagents.Provider{
		Scheme:        "fakelivewl",
		Label:         "test fake (identity-whitelist live-enum)",
		AgentIDSource: "test only",
		Identities:    []iagents.IdentitySpec{{Type: iagents.IdentityUser}, {Type: iagents.IdentityBot}},
		Instance:      &spec,
		ListAgents: func(context.Context, iagents.Runtime, iagents.PageParams) ([]iagents.AgentSummary, iagents.PageInfo, error) {
			called = true
			return nil, iagents.PageInfo{}, nil
		},
	})

	cfg := &core.CliConfig{AppID: "cli_x", AppSecret: "fake-secret", Brand: core.BrandFeishu}
	f, _, _, _ := cmdutil.TestFactory(t, cfg)
	opts := &listOptions{Factory: f, Cmd: resolveCmd(t, true, "admin"), Format: "json", Scheme: "fakelivewl", As: "admin", PageSize: defaultPageSize}

	err := agentListRun(opts)
	if err == nil {
		t.Fatal("an unsupported identity should be rejected before the online call")
	}
	if !errs.IsValidation(err) {
		t.Fatalf("unsupported identity should be a validation error, got %T (%v)", err, err)
	}
	if called {
		t.Error("ListAgents must NOT be called when the identity whitelist fails")
	}
}

// TestAgentListScheme_OnlineChecksProviderIdentity covers the provider-level
// identity subset in addition to the global user|bot vocabulary. A user-only
// online provider must reject bot before constructing/calling ListAgents.
func TestAgentListScheme_OnlineChecksProviderIdentity(t *testing.T) {
	called := false
	spec := catSpec("", "", "")
	iagents.Register(iagents.Provider{
		Scheme:        "fakeliveuseronly",
		Label:         "test fake (user-only live-enum)",
		AgentIDSource: "test only",
		Identities:    []iagents.IdentitySpec{{Type: iagents.IdentityUser}},
		Instance:      &spec,
		ListAgents: func(context.Context, iagents.Runtime, iagents.PageParams) ([]iagents.AgentSummary, iagents.PageInfo, error) {
			called = true
			return nil, iagents.PageInfo{}, nil
		},
	})

	cfg := &core.CliConfig{AppID: "cli_x", AppSecret: "fake-secret", Brand: core.BrandFeishu}
	f, _, _, _ := cmdutil.TestFactory(t, cfg)
	opts := &listOptions{
		Factory: f, Cmd: resolveCmd(t, true, "bot"), Format: "json",
		Scheme: "fakeliveuseronly", As: "bot", PageSize: defaultPageSize,
	}

	err := agentListRun(opts)
	if err == nil {
		t.Fatal("bot should be rejected by a user-only online provider")
	}
	p, ok := errs.ProblemOf(err)
	var validationErr *errs.ValidationError
	if !ok || p.Subtype != errs.SubtypeInvalidArgument || !errors.As(err, &validationErr) || validationErr.Param != "--as" {
		t.Fatalf("provider identity rejection should be invalid_argument for --as, got problem=%+v err=%v", p, err)
	}
	if called {
		t.Error("ListAgents must NOT be called when the provider identity check fails")
	}
}

// TestAgentListScheme_PrettyStripsANSI pins that `agents list <scheme> --format
// pretty` strips ANSI escapes from agent-controlled Name/Description (here from
// static catalog entries) before they reach the terminal.
func TestAgentListScheme_PrettyStripsANSI(t *testing.T) {
	iagents.Register(iagents.Provider{
		Scheme:        "fakedirty",
		Label:         "test fake (dirty names)",
		AgentIDSource: "test only",
		Identities:    []iagents.IdentitySpec{{Type: iagents.IdentityUser}},
		Catalog:       []iagents.AgentSpec{catSpec("a1", "\x1b[31mEvil\x1b[0m One", "d\x1b[2Jesc")},
	})

	cfg := &core.CliConfig{AppID: "cli_x", AppSecret: "fake-secret", Brand: core.BrandFeishu}
	f, _, _, _ := cmdutil.TestFactory(t, cfg)
	cmd := &cobra.Command{Use: "list"}
	cmd.SetContext(context.Background())
	opts := &listOptions{Factory: f, Cmd: cmd, Format: "pretty", Scheme: "fakedirty"}
	out := f.IOStreams.Out.(interface{ Bytes() []byte })

	if err := agentListRun(opts); err != nil {
		t.Fatalf("list fakedirty pretty should not error: %v", err)
	}
	text := string(out.Bytes())
	if strings.Contains(text, "\x1b") {
		t.Errorf("ANSI sequences in agent Name/Description must be stripped: %q", text)
	}
	if !strings.Contains(text, "Evil One") || !strings.Contains(text, "desc") {
		t.Errorf("readable text should remain after stripping, got %q", text)
	}
}

// TestAgentListJqFlagRegisteredAndConsumed pins the quality-review fix: the
// --jq flag must be registered on `agents list` and filter the envelope.
func TestAgentListJqFlagRegisteredAndConsumed(t *testing.T) {
	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	f := &cmdutil.Factory{IOStreams: &cmdutil.IOStreams{Out: out, ErrOut: errOut}}
	cmd := NewCmdAgentList(f)
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetContext(context.Background())
	cmd.SetArgs([]string{"--jq", ".ok"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("agents list --jq should not error: %v", err)
	}
	if got := strings.TrimSpace(out.String()); got != "true" {
		t.Errorf("--jq .ok should output only true, got %q", got)
	}
}

// TestNewCmdAgentList_ReadRisk pins the read risk annotation, the json default
// of --format, the --jq flag presence, and that list takes at most one
// positional arg (the scheme).
func TestNewCmdAgentList_ReadRisk(t *testing.T) {
	cmd := NewCmdAgentList(nil)
	if level, ok := cmdutil.GetRisk(cmd); !ok || level != cmdutil.RiskRead {
		t.Errorf("agents list should be marked read risk, got level=%q ok=%v", level, ok)
	}
	fl := cmd.Flags().Lookup("format")
	if fl == nil {
		t.Fatal("agents list should have a --format flag")
	}
	if fl.DefValue != "json" {
		t.Errorf("--format default should flip to json, got %q", fl.DefValue)
	}
	if cmd.Flags().Lookup("jq") == nil {
		t.Error("agents list should have a --jq flag")
	}
	if cmd.Flags().Lookup("as") == nil {
		t.Error("agents list should register an --as flag (needed to pick the identity for online enumeration)")
	}
	if err := cmd.Args(cmd, []string{}); err != nil {
		t.Errorf("agents list with no args should be valid: %v", err)
	}
	if err := cmd.Args(cmd, []string{"base"}); err != nil {
		t.Errorf("agents list <scheme> should be valid: %v", err)
	}
	if err := cmd.Args(cmd, []string{"base", "extra"}); err == nil {
		t.Error("agents list with more than 1 positional argument should error (MaximumNArgs 1)")
	}
}
