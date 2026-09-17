// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdutil

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/agentsupport"
	"github.com/larksuite/cli/internal/core"
)

// TestWarnAgentIdentityGateOffOnceFiresOncePerInstance pins the per-instance
// guard: a second Factory in the same process still warns.
func TestWarnAgentIdentityGateOffOnceFiresOncePerInstance(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_AGENT_IDENTITY_GATE", "off")
	f1, _, stderr1, _ := TestFactory(t, agentConfig())
	f1.WarnAgentIdentityGateOffOnce()
	f1.WarnAgentIdentityGateOffOnce()
	if n := strings.Count(stderr1.String(), "LARKSUITE_CLI_AGENT_IDENTITY_GATE=off"); n != 1 {
		t.Fatalf("expected exactly one warning for f1, got %d", n)
	}

	f2, _, stderr2, _ := TestFactory(t, agentConfig())
	f2.WarnAgentIdentityGateOffOnce()
	if !strings.Contains(stderr2.String(), "LARKSUITE_CLI_AGENT_IDENTITY_GATE=off") {
		t.Fatal("a second Factory instance must print its own warning")
	}
}

// newFactoryWithCredentialKind builds a Factory whose active profile is classified as kind.
func newFactoryWithCredentialKind(t *testing.T, kind core.CredentialKind) *Factory {
	t.Helper()
	f, _, _, _ := TestFactory(t, &core.CliConfig{
		AppID:           "cli_test",
		AppSecret:       "secret",
		Brand:           core.BrandFeishu,
		CredentialKind:  kind,
		PrincipalOpenID: "ou_test",
		UserOpenId:      "ou_test",
	})
	return f
}

// TestWarnAgentIdentityGateOffOnceSilentAtDefault pins that a user who never
// touched the switch gets no notice. The notice names a deliberate override; at
// the default there is nothing to report, and reporting it anyway would put a
// line on stderr for a state nobody chose.
func TestWarnAgentIdentityGateOffOnceSilentAtDefault(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_AGENT_IDENTITY_GATE", "")
	if err := os.Unsetenv("LARKSUITE_CLI_AGENT_IDENTITY_GATE"); err != nil {
		t.Fatal(err)
	}
	f, _, stderr, _ := TestFactory(t, agentConfig())
	f.WarnAgentIdentityGateOffOnce()
	if stderr.String() != "" {
		t.Fatalf("an untouched default must print nothing, got: %s", stderr.String())
	}
}

// TestAgentIdentityGateDefaultsOn pins the shipped default and how the switch
// reads. Generated commands filter by identity and an Agent Employee is a
// distinct identity, so the gate is on unless someone turns it off — and only a
// negative spelling turns it off. An exported-but-empty variable and a typo both
// keep the default, so neither becomes a quiet way to disable the check.
func TestAgentIdentityGateDefaultsOn(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_AGENT_IDENTITY_GATE", "")
	if err := os.Unsetenv("LARKSUITE_CLI_AGENT_IDENTITY_GATE"); err != nil {
		t.Fatal(err)
	}
	if !AgentIdentityGateEnabled() {
		t.Fatal("the identity gate must be on unless it is explicitly turned off")
	}
	for _, on := range []string{"on", "1", "true", "", "typo"} {
		t.Setenv("LARKSUITE_CLI_AGENT_IDENTITY_GATE", on)
		if !AgentIdentityGateEnabled() {
			t.Errorf("%q must leave the gate enabled", on)
		}
	}
	for _, off := range []string{"off", "0", "false", "no"} {
		t.Setenv("LARKSUITE_CLI_AGENT_IDENTITY_GATE", off)
		if AgentIdentityGateEnabled() {
			t.Errorf("%q must disable the gate", off)
		}
	}
}

// TestWarnAgentIdentityGateOffOnceSilentWhenValueDoesNotDisable pins that the
// notice tracks what the value did, not that a value exists. "maybe" and ""
// fall back to the default, which leaves the gate on; announcing it as
// disabled would be false.
func TestWarnAgentIdentityGateOffOnceSilentWhenValueDoesNotDisable(t *testing.T) {
	for _, value := range []string{"maybe", "", "on"} {
		t.Run("value="+value, func(t *testing.T) {
			t.Setenv("LARKSUITE_CLI_AGENT_IDENTITY_GATE", value)
			f, _, stderr, _ := TestFactory(t, agentConfig())
			f.WarnAgentIdentityGateOffOnce()
			if stderr.String() != "" {
				t.Fatalf("a value that leaves the gate on must print nothing, got: %s", stderr.String())
			}
		})
	}
}

// TestCheckAgentSupportHintNamesTheDependency pins the advice on a denied
// shortcut. The verdict is denied because an OpenAPI the shortcut calls does
// not accept an agent token; a capability grant cannot change that, so the
// hint must not send the agent to ask for one.
func TestCheckAgentSupportHintNamesTheDependency(t *testing.T) {
	f := newFactoryWithCredentialKind(t, core.CredentialAAT)
	err := f.CheckAgentSupport(context.Background(), agentsupport.Denied, "im +watch")
	if err == nil {
		t.Fatal("a denied shortcut must be refused for an Agent Employee")
	}
	var vErr *errs.ValidationError
	if !errors.As(err, &vErr) {
		t.Fatalf("want a ValidationError, got %T: %v", err, err)
	}
	if !strings.Contains(vErr.Hint, "does not accept an Agent Employee token") {
		t.Errorf("hint should name the dependency, got: %q", vErr.Hint)
	}
	if strings.Contains(vErr.Hint, "grant this Agent Employee the capability") {
		t.Errorf("hint must not point at a capability grant that cannot help, got: %q", vErr.Hint)
	}
}
