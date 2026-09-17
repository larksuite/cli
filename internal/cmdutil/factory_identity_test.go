// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdutil

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/core"
)

func agentConfig() *core.CliConfig {
	return &core.CliConfig{
		AppID:           "cli_agent",
		AppSecret:       "secret",
		Brand:           core.BrandFeishu,
		CredentialKind:  core.CredentialAAT,
		PrincipalOpenID: "ou_agent",
		UserOpenId:      "ou_agent",
	}
}

// newIdentityCmd builds a command carrying just the identity flag, so these
// tests exercise the resolution rules rather than any command's own wiring.
func newIdentityCmd(as string) *cobra.Command {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("as", "", "identity")
	if as != "" {
		_ = cmd.Flags().Set("as", as)
	}
	return cmd
}

func TestResolveIdentityForAgentProfile(t *testing.T) {
	tests := []struct {
		name    string
		flagAs  string
		want    core.Identity
		wantErr bool
	}{
		{name: "explicit user", flagAs: "user", want: core.AsUser},
		{name: "explicit auto resolves to user", flagAs: "auto", want: core.AsUser},
		{name: "unset resolves to user", want: core.AsUser},
		{name: "explicit bot is refused", flagAs: "bot", wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// The mock HTTP registry has no stubs: reaching the network here
			// would fail the test, which is what makes "identity selection
			// resolves no credential" an assertion rather than a claim.
			f, _, _, _ := TestFactory(t, agentConfig())
			cmd := newIdentityCmd(tc.flagAs)

			got, err := f.ResolveIdentity(context.Background(), cmd, core.Identity(tc.flagAs))
			if tc.wantErr {
				var validationErr *errs.ValidationError
				if !errors.As(err, &validationErr) || validationErr.Subtype != errs.SubtypeFailedPrecondition {
					t.Fatalf("error = %T %v, want failed_precondition", err, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("ResolveIdentity() error = %v", err)
			}
			if got != tc.want {
				t.Fatalf("identity = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestResolveIdentityLeavesUnclassifiedProfilesAlone pins that a profile without
// a credential classification keeps its existing behaviour: auto still falls to
// bot when no user credential is present.
func TestResolveIdentityLeavesUnclassifiedProfilesAlone(t *testing.T) {
	cfg := &core.CliConfig{AppID: "cli_app", AppSecret: "secret", Brand: core.BrandFeishu}
	for _, flagAs := range []string{"", "user", "bot", "auto"} {
		name := flagAs
		if name == "" {
			name = "unset"
		}
		t.Run(name, func(t *testing.T) {
			f, _, _, _ := TestFactory(t, cfg)
			got, err := f.ResolveIdentity(context.Background(), newIdentityCmd(flagAs), core.Identity(flagAs))
			if err != nil {
				t.Fatalf("ResolveIdentity() error = %v", err)
			}
			if got != core.AsUser && got != core.AsBot {
				t.Fatalf("identity = %q, want user or bot", got)
			}
			if flagAs == "user" && got != core.AsUser {
				t.Fatalf("identity = %q, explicit user must win", got)
			}
			if flagAs == "bot" && got != core.AsBot {
				t.Fatalf("identity = %q, explicit bot must win", got)
			}
		})
	}
}

// TestRequireHumanCredentialRefusesAgentProfile pins that commands built around
// a person — an interactive login, a logout, a scope grant — are refused on an
// Agent Employee profile, locally and before any request.
func TestRequireHumanCredentialRefusesAgentProfile(t *testing.T) {
	f, _, _, _ := TestFactory(t, agentConfig())
	err := f.RequireHumanCredential(context.Background(), "auth login")
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) || validationErr.Subtype != errs.SubtypeFailedPrecondition {
		t.Fatalf("error = %T %v, want failed_precondition", err, err)
	}
	if validationErr.Hint == "" {
		t.Fatal("the refusal must say what to do instead")
	}
}

// TestRequireHumanCredentialAllowsHumanProfile pins that the same gate is
// invisible to every profile that predates classification.
func TestRequireHumanCredentialAllowsHumanProfile(t *testing.T) {
	f, _, _, _ := TestFactory(t, &core.CliConfig{AppID: "cli_app", AppSecret: "secret", Brand: core.BrandFeishu})
	if err := f.RequireHumanCredential(context.Background(), "auth login"); err != nil {
		t.Fatalf("RequireHumanCredential() error = %v, want nil", err)
	}
}

// TestCheckAgentEligibilityAllowsNonAgentProfiles pins that the gate is invisible outside an AAT profile.
func TestCheckAgentEligibilityAllowsNonAgentProfiles(t *testing.T) {
	f := newFactoryWithCredentialKind(t, core.CredentialUAT)
	if err := f.CheckAgentEligibility(context.Background(), false, false, "im.messages.create"); err != nil {
		t.Errorf("a UAT profile must not be gated, got: %v", err)
	}
}

// TestCheckAgentEligibilityRefusesUnmarkedMethodForAgent pins the refusal shape:
// failed_precondition naming the command, with a hint that never names the bypass variable.
func TestCheckAgentEligibilityRefusesUnmarkedMethodForAgent(t *testing.T) {
	f := newFactoryWithCredentialKind(t, core.CredentialAAT)
	err := f.CheckAgentEligibility(context.Background(), false, true, "im.messages.create")
	if err == nil {
		t.Fatal("expected a refusal")
	}
	var vErr *errs.ValidationError
	if !errors.As(err, &vErr) {
		t.Fatalf("want *errs.ValidationError, got %T", err)
	}
	if vErr.Subtype != errs.SubtypeFailedPrecondition {
		t.Errorf("subtype = %q, want %q", vErr.Subtype, errs.SubtypeFailedPrecondition)
	}
	if !strings.Contains(vErr.Message, "im.messages.create") {
		t.Errorf("message should name the command, got: %s", vErr.Message)
	}
	if strings.Contains(vErr.Hint, "LARKSUITE_CLI_AGENT_IDENTITY_GATE") {
		t.Errorf("the hint must not advertise the bypass switch, got: %s", vErr.Hint)
	}
	if !strings.Contains(vErr.Hint, "Agent platform") {
		t.Errorf("the hint should point at the Agent platform, got: %s", vErr.Hint)
	}
}

// TestCheckAgentEligibilityAllowsMarkedMethodForAgent pins that an agent-marked method is allowed.
func TestCheckAgentEligibilityAllowsMarkedMethodForAgent(t *testing.T) {
	f := newFactoryWithCredentialKind(t, core.CredentialAAT)
	if err := f.CheckAgentEligibility(context.Background(), true, true, "im.messages.create"); err != nil {
		t.Errorf("a marked method must be allowed, got: %v", err)
	}
}

// TestCheckAgentEligibilitySkippedWhenGateOff pins that the off switch silences the gate.
func TestCheckAgentEligibilitySkippedWhenGateOff(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_AGENT_IDENTITY_GATE", "off")
	f := newFactoryWithCredentialKind(t, core.CredentialAAT)
	if err := f.CheckAgentEligibility(context.Background(), false, true, "im.messages.create"); err != nil {
		t.Errorf("the gate is off, so nothing may be refused, got: %v", err)
	}
}

// TestCheckAgentEligibilityReplacesHintWhenServiceIsUnannotated pins that an
// unannotated service replaces the capability-grant hint rather than appending to it.
func TestCheckAgentEligibilityReplacesHintWhenServiceIsUnannotated(t *testing.T) {
	f := newFactoryWithCredentialKind(t, core.CredentialAAT)
	err := f.CheckAgentEligibility(context.Background(), false, false, "im.messages.create")
	var vErr *errs.ValidationError
	if !errors.As(err, &vErr) {
		t.Fatalf("want *errs.ValidationError, got %T (%v)", err, err)
	}
	if strings.Contains(vErr.Hint, "Agent platform") {
		t.Errorf("hint must not lead with the Agent-platform capability-grant fix when no method in the service is agent-marked, got: %s", vErr.Hint)
	}
	if !strings.Contains(vErr.Hint, "open platform") {
		t.Errorf("hint should point at the open platform (the api_definition owner), got: %s", vErr.Hint)
	}
	if !strings.Contains(vErr.Hint, "not something the current profile can fix") {
		t.Errorf("hint should say this profile cannot self-fix it, got: %s", vErr.Hint)
	}
}
