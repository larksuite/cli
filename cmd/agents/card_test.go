// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package agents

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	iagents "github.com/larksuite/cli/internal/agents"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/output"
)

// cardTestOpts builds a cardOptions driving agentCardRun against a real
// (test) Factory. The card is synthesized statically, so no API call
// is made and stdout carries the capability card envelope.
func cardTestOpts(t *testing.T, ref string) (*cardOptions, *core.CliConfig) {
	t.Helper()
	registerScripted()
	cfg := &core.CliConfig{AppID: "cli_x", AppSecret: "fake-secret", Brand: core.BrandFeishu}
	f, _, _, _ := cmdutil.TestFactory(t, cfg)
	cmd := resolveCmd(t, true, "bot") // reuses the common_test.go helper (--as=bot)
	return &cardOptions{Factory: f, Cmd: cmd, Ref: ref, As: "bot", Format: "json"}, cfg
}

// TestAgentCardRun_StaticCard verifies that `agents card fakecat:min`
// returns the statically synthesized capability card (no API), with
// task_cancel gated off and the three context_* caps on, and the agent_id
// echoed from the ref.
func TestAgentCardRun_StaticCard(t *testing.T) {
	opts, _ := cardTestOpts(t, "fakecat:min")
	out := opts.Factory.IOStreams.Out.(interface{ Bytes() []byte })

	if err := agentCardRun(opts); err != nil {
		t.Fatalf("card should be statically synthesized and not error: %v", err)
	}

	var env output.Envelope
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("output should be valid envelope JSON: %v", err)
	}
	if !env.OK {
		t.Errorf("ok should be true: %+v", env)
	}
	data, ok := env.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("data should be a card object, got %T", env.Data)
	}
	if data["agent_id"] != "min" {
		t.Errorf("agent_id should echo the ref, got %v", data["agent_id"])
	}
	if data["provider"] != "fakecat" {
		t.Errorf("provider should be fakecat, got %v", data["provider"])
	}
	// source was removed from the card (schema tightening).
	if _, present := data["source"]; present {
		t.Errorf("card should no longer carry a source field, got %v", data["source"])
	}
	caps, ok := data["capabilities"].(map[string]interface{})
	if !ok {
		t.Fatalf("capabilities should be an object, got %T", data["capabilities"])
	}
	if caps["task_cancel"] != false {
		t.Errorf("fakecat:min task_cancel should be false, got %v", caps["task_cancel"])
	}
	if caps["context_list"] != true || caps["context_get"] != true || caps["context_delete"] != true {
		t.Errorf("fakecat:min should support the three context capabilities, got %v", caps)
	}
	// The lean card embeds NO parameter details; has_parameters is the always-
	// emitted (non-null) cue. fakecat:min declares no params ⇒ []; the old parameters
	// field must be gone entirely.
	if hp, ok := data["has_parameters"].([]interface{}); !ok {
		t.Errorf("has_parameters should be a non-null array, got %T (%v)", data["has_parameters"], data["has_parameters"])
	} else if len(hp) != 0 {
		t.Errorf("fakecat:min has_parameters should be empty, got %v", hp)
	}
	if _, present := data["parameters"]; present {
		t.Errorf("the lean card must not embed a parameters field (use --operation), got %v", data["parameters"])
	}
	if ids, ok := data["identity"].([]interface{}); !ok || len(ids) == 0 {
		t.Errorf("identity should be a non-null non-empty array, got %T (%v)", data["identity"], data["identity"])
	}
	// card no longer exposes scope: the required_scopes field was removed from
	// AgentCard (scope is an internal registration item used only for preflight).
	if _, present := data["required_scopes"]; present {
		t.Errorf("card should no longer carry a required_scopes field, got %v", data["required_scopes"])
	}
}

// TestAgentCardRun_UserOnlyProviderRemainsDiscoverable pins the separation
// between Card discovery and operation identity enforcement. An unsupported
// default bot identity still gets the static user-only Card without Describe;
// a supported user identity may use the configured runtime to enrich it.
func TestAgentCardRun_UserOnlyProviderRemainsDiscoverable(t *testing.T) {
	registerScripted()
	describeCalls := 0
	fakeUserOnlyDescribe = func(rt iagents.Runtime) (*iagents.CardInfo, error) {
		describeCalls++
		if rt.IsBot() {
			t.Fatal("Describe must not run with the provider-unsupported bot identity")
		}
		return &iagents.CardInfo{Name: "enriched for user"}, nil
	}
	t.Cleanup(func() { fakeUserOnlyDescribe = nil })

	botFactory := unconfiguredFactory(t)
	botCmd := resolveCmd(t, false, "") // unconfigured auto-detect falls back to bot
	botOut := botFactory.IOStreams.Out.(interface{ Bytes() []byte })
	if err := agentCardRun(&cardOptions{Factory: botFactory, Cmd: botCmd, Ref: "fakeuseronly:agt_x", Format: "json"}); err != nil {
		t.Fatalf("the static user-only Card should remain discoverable under default bot: %v", err)
	}
	if describeCalls != 0 {
		t.Fatalf("unsupported bot identity must skip dynamic Describe, calls=%d", describeCalls)
	}
	var botEnv output.Envelope
	if err := json.Unmarshal(botOut.Bytes(), &botEnv); err != nil || !botEnv.OK {
		t.Fatalf("bot discovery should emit a valid success envelope: err=%v out=%s", err, botOut.Bytes())
	}

	userFactory, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{AppID: "cli_x", AppSecret: "fake-secret", Brand: core.BrandFeishu})
	userCmd := resolveCmd(t, true, "user")
	userOut := userFactory.IOStreams.Out.(interface{ Bytes() []byte })
	if err := agentCardRun(&cardOptions{Factory: userFactory, Cmd: userCmd, Ref: "fakeuseronly:agt_x", As: "user", Format: "json"}); err != nil {
		t.Fatalf("the supported user identity should read and enrich the Card: %v", err)
	}
	if describeCalls != 1 {
		t.Fatalf("supported user identity should invoke Describe once, calls=%d", describeCalls)
	}
	var userEnv output.Envelope
	if err := json.Unmarshal(userOut.Bytes(), &userEnv); err != nil {
		t.Fatalf("user Card output should be valid JSON: %v (%s)", err, userOut.Bytes())
	}
	data, _ := userEnv.Data.(map[string]interface{})
	if data["name"] != "enriched for user" {
		t.Fatalf("supported user identity should receive dynamic enrichment, got %v", data["name"])
	}
}

// TestAgentCardRun_PrettyFormat verifies that with --format pretty (opt-in
// since the json default flip), the card renders as a human-readable listing.
// The output must surface the identity and capability names in plain text so
// the stream is not valid envelope JSON.
func TestAgentCardRun_PrettyFormat(t *testing.T) {
	opts, _ := cardTestOpts(t, "fakecat:min")
	opts.Format = "pretty"
	out := opts.Factory.IOStreams.Out.(interface{ Bytes() []byte })

	if err := agentCardRun(opts); err != nil {
		t.Fatalf("card pretty should not error: %v", err)
	}

	text := string(out.Bytes())
	// A pretty rendering is human text, not a JSON envelope.
	var env output.Envelope
	if json.Unmarshal(out.Bytes(), &env) == nil && env.OK {
		t.Fatalf("pretty format should not output a JSON envelope: %s", text)
	}
	if !strings.Contains(text, "min") {
		t.Errorf("pretty output should contain agent_id: %s", text)
	}
	// context_list is a declared capability of the fakecat:min card; it must appear.
	if !strings.Contains(text, "context_list") {
		t.Errorf("pretty output should list capabilities: %s", text)
	}
}

// TestAgentCardRun_JSONFormat pins that --format json still emits the envelope.
func TestAgentCardRun_JSONFormat(t *testing.T) {
	opts, _ := cardTestOpts(t, "fakecat:min")
	opts.Format = "json"
	out := opts.Factory.IOStreams.Out.(interface{ Bytes() []byte })

	if err := agentCardRun(opts); err != nil {
		t.Fatalf("card json should not error: %v", err)
	}
	var env output.Envelope
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("json format should be a valid envelope: %v (%s)", err, string(out.Bytes()))
	}
	if !env.OK {
		t.Errorf("ok should be true: %+v", env)
	}
}

// TestAgentCardJqFlagRegisteredAndConsumed pins the quality-review fix: the
// --jq flag must actually be REGISTERED on `agents card` (the run path already
// called jqExpr/JqFilter, but without the flag `--jq` was an unknown-flag
// exit 2 — and the skill doc teaches AI to copy `card ... --jq`). Executed via
// the real command so registration + consumption are proven together.
func TestAgentCardJqFlagRegisteredAndConsumed(t *testing.T) {
	cfg := &core.CliConfig{AppID: "cli_x", AppSecret: "fake-secret", Brand: core.BrandFeishu}
	f, _, _, _ := cmdutil.TestFactory(t, cfg)
	cmd := NewCmdAgentCard(f)
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetContext(context.Background())
	cmd.SetArgs([]string{"fakecat:min", "--as", "bot", "--jq", ".data.agent_id"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("card --jq should not error: %v", err)
	}
	out := f.IOStreams.Out.(interface{ Bytes() []byte })
	got := strings.TrimSpace(string(out.Bytes()))
	if !strings.Contains(got, "min") || strings.Contains(got, `"ok"`) {
		t.Errorf("--jq .data.agent_id should output only the filtered result, got %q", got)
	}
}

// TestPrintCardPretty_NilCard pins that a nil card degrades to a placeholder
// line instead of panicking (card.go nil branch).
func TestPrintCardPretty_NilCard(t *testing.T) {
	out := &bytes.Buffer{}
	printCardPretty(out, nil)
	if !strings.Contains(out.String(), "(no card)") {
		t.Errorf("nil card should print a placeholder line, got: %q", out.String())
	}
}

// TestPrintCardPretty_AllOptionalFields exercises every optional-field branch of
// the pretty renderer that a minimal static card omits: the dynamic-card Name
// (taking precedence over ProviderLabel), Description, declared Parameters, and
// the Skills block (both the named skill and the id-fallback when Name is empty).
func TestPrintCardPretty_AllOptionalFields(t *testing.T) {
	card := &iagents.AgentCard{
		Provider:      "demo",
		ProviderLabel: "demo custom agent",
		Name:          "Demo Agent", // only dynamic cards have Name; it should override ProviderLabel
		AgentID:       "agt_demo",
		Description:   "a helpful demo agent",
		Identity: []iagents.IdentitySpec{
			{Type: "user"},
			{Type: "bot", Precondition: "must be on the channel allowlist"},
		},
		Capabilities: iagents.Capabilities{
			ContextList: true,
			TaskCancel:  false,
		},
		HasParameters: []string{"send"},
		Skills: []iagents.CardSkill{
			{ID: "sk_1", Name: "Sales Analysis"},
			{ID: "sk_2"}, // no Name → falls back to ID
		},
	}
	out := &bytes.Buffer{}
	printCardPretty(out, card)
	text := out.String()

	for _, want := range []string{
		"Demo Agent (agt_demo)",            // dynamic Name takes precedence over ProviderLabel
		"a helpful demo agent",             // Description branch
		"identity: user, bot",              // IdentitySpec types are joined
		"must be on the channel allowlist", // identity precondition must be visible in pretty
		"parameters: send",                 // has_parameters cue + --operation pointer
		"skills:",                          // Skills block header
		"Sales Analysis",                   // skill with a Name
		"sk_2",                             // skill without a Name → id fallback
	} {
		if !strings.Contains(text, want) {
			t.Errorf("pretty output should contain %q, got:\n%s", want, text)
		}
	}
}

// TestPrintCardPretty_StripsANSIFromRemoteFields pins that a remote card's
// agent-controlled Name/Description cannot smuggle ANSI escapes to the
// terminal (this sanitization is applied to every pretty surface).
func TestPrintCardPretty_StripsANSIFromRemoteFields(t *testing.T) {
	card := &iagents.AgentCard{
		Provider:    "demo",
		AgentID:     "agt_demo",
		Name:        "\x1b[31mEvil\x1b[0m Agent",
		Description: "desc\x1b[2Jwipe",
	}
	out := &bytes.Buffer{}
	printCardPretty(out, card)
	text := out.String()
	if strings.Contains(text, "\x1b") {
		t.Errorf("ANSI sequences in remote card fields must be stripped: %q", text)
	}
	if !strings.Contains(text, "Evil Agent") || !strings.Contains(text, "descwipe") {
		t.Errorf("readable text should remain after stripping, got: %q", text)
	}
}

// TestPrintCardPretty_StaticFallsBackToProviderLabel pins that a static card
// (no dynamic Name) renders its ProviderLabel as the header.
func TestPrintCardPretty_StaticFallsBackToProviderLabel(t *testing.T) {
	card := &iagents.AgentCard{
		Provider:      "demo",
		ProviderLabel: "demo custom agent",
		AgentID:       "agt_demo",
	}
	out := &bytes.Buffer{}
	printCardPretty(out, card)
	if !strings.Contains(out.String(), "demo custom agent (agt_demo)") {
		t.Errorf("should fall back to ProviderLabel when Name is empty, got:\n%s", out.String())
	}
}

// TestAgentCardRun_InvalidRef surfaces a malformed ref as a validation error
// before any provider is built.
func TestAgentCardRun_InvalidRef(t *testing.T) {
	opts, _ := cardTestOpts(t, "no-colon")
	if err := agentCardRun(opts); err == nil {
		t.Fatal("malformed ref should error")
	}
}

// TestNewCmdAgentCard_ReadRiskAndArgs pins ExactArgs(1), read risk, and the
// presence of --format and --as flags.
func TestNewCmdAgentCard_ReadRiskAndArgs(t *testing.T) {
	cmd := NewCmdAgentCard(nil)
	if level, ok := cmdutil.GetRisk(cmd); !ok || level != cmdutil.RiskRead {
		t.Errorf("agents card should be marked read risk, got level=%q ok=%v", level, ok)
	}
	if err := cmd.Args(cmd, []string{}); err == nil {
		t.Error("agents card missing ref should report an argument error (ExactArgs 1)")
	}
	if err := cmd.Args(cmd, []string{"fakeflow:x"}); err != nil {
		t.Errorf("agents card with a single ref should be valid: %v", err)
	}
	fl := cmd.Flags().Lookup("format")
	if fl == nil {
		t.Fatal("agents card should have a --format flag")
	}
	// Default output format is unified: card default flips from pretty to json.
	if fl.DefValue != "json" {
		t.Errorf("card --format default should flip to json, got %q", fl.DefValue)
	}
	if cmd.Flags().Lookup("as") == nil {
		t.Error("agents card should have an --as flag")
	}
}
