// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdutil

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/agentsupport"
	"github.com/larksuite/cli/internal/core"
)

func TestCheckAgentSupportAllowsUnknownAndAllowed(t *testing.T) {
	f := newFactoryWithCredentialKind(t, core.CredentialAAT)
	for _, v := range []agentsupport.Verdict{agentsupport.Unknown, agentsupport.Allowed} {
		if err := f.CheckAgentSupport(context.Background(), v, "im +send"); err != nil {
			t.Errorf("AgentSupport %q must be allowed, got: %v", v, err)
		}
	}
}

func TestCheckAgentSupportRefusesDenied(t *testing.T) {
	f := newFactoryWithCredentialKind(t, core.CredentialAAT)
	err := f.CheckAgentSupport(context.Background(), agentsupport.Denied, "im +send")
	if err == nil {
		t.Fatal("expected a refusal")
	}
	var vErr *errs.ValidationError
	if !errors.As(err, &vErr) || vErr.Subtype != errs.SubtypeFailedPrecondition {
		t.Fatalf("want failed_precondition ValidationError, got %v", err)
	}
	if !strings.Contains(vErr.Message, "im +send") {
		t.Errorf("message should name the command, got: %s", vErr.Message)
	}
}

// TestCheckAgentSupportIgnoresIdentityGateSwitch pins that the metaAPI fuse does
// not reach the shortcut verdict; that side is rolled back by clearing the table.
func TestCheckAgentSupportIgnoresIdentityGateSwitch(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_AGENT_IDENTITY_GATE", "off")
	f := newFactoryWithCredentialKind(t, core.CredentialAAT)
	if err := f.CheckAgentSupport(context.Background(), agentsupport.Denied, "im +send"); err == nil {
		t.Error("the identity-gate switch must not disable the shortcut verdict")
	}
}

// TestCheckAgentSupportHintDoesNotAdvertiseAnySwitch pins that the hint never
// names a bypass variable an autonomous consumer would set and leave set.
func TestCheckAgentSupportHintDoesNotAdvertiseAnySwitch(t *testing.T) {
	f := newFactoryWithCredentialKind(t, core.CredentialAAT)
	err := f.CheckAgentSupport(context.Background(), agentsupport.Denied, "im +send")
	var vErr *errs.ValidationError
	if !errors.As(err, &vErr) {
		t.Fatalf("want *errs.ValidationError, got %T", err)
	}
	for _, name := range []string{
		"LARKSUITE_CLI_AGENT_IDENTITY_GATE",
		"LARKSUITE_CLI_AGENT_SCOPE_PRECHECK",
	} {
		if strings.Contains(vErr.Hint, name) {
			t.Errorf("hint must not mention %s, got: %s", name, vErr.Hint)
		}
	}
}

func TestCheckAgentSupportAllowsNonAgentProfiles(t *testing.T) {
	f := newFactoryWithCredentialKind(t, core.CredentialUAT)
	if err := f.CheckAgentSupport(context.Background(), agentsupport.Denied, "im +send"); err != nil {
		t.Errorf("a UAT profile must not be gated, got: %v", err)
	}
}
