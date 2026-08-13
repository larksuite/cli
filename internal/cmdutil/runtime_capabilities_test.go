// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdutil

import (
	"context"
	"errors"
	"testing"

	extcred "github.com/larksuite/cli/extension/credential"
	"github.com/larksuite/cli/internal/credential"
	"github.com/larksuite/cli/internal/runtimeplan"
	"github.com/spf13/cobra"
)

func TestRuntimeCapabilitiesUseNearestDeclaration(t *testing.T) {
	parent := &cobra.Command{Use: "auth"}
	SetRuntimeCapabilities(parent, runtimeplan.CapabilityLocalCredentialManagement)

	inherited := &cobra.Command{Use: "login"}
	parent.AddCommand(inherited)
	got := GetRuntimeCapabilities(inherited)
	if len(got) != 1 || got[0] != runtimeplan.CapabilityLocalCredentialManagement {
		t.Fatalf("inherited capabilities = %v", got)
	}

	diagnostic := &cobra.Command{Use: "status"}
	SetRuntimeCapabilities(diagnostic)
	parent.AddCommand(diagnostic)
	if got := GetRuntimeCapabilities(diagnostic); len(got) != 0 {
		t.Fatalf("explicit empty capabilities = %v", got)
	}
}

func TestRequireRuntimeCapabilitiesUsesPlan(t *testing.T) {
	denied := errors.New("events denied")
	plan := runtimeplan.New(runtimeplan.Options{
		Capabilities: func(capability runtimeplan.Capability) error {
			if capability == runtimeplan.CapabilityRealtimeEvents {
				return denied
			}
			return nil
		},
	})
	f, _, _, _ := TestFactoryWithRuntimePlan(t, nil, plan)

	err := f.RequireRuntimeCapabilities(context.Background(), "event consume", runtimeplan.CapabilityRealtimeEvents)
	if !errors.Is(err, denied) {
		t.Fatalf("error = %v, want plan denial", err)
	}
}

func TestRequireRuntimeCapabilitiesKeepsPurelyLocalCommandsAvailable(t *testing.T) {
	startupErr := errors.New("managed runtime bootstrap failed")
	f, _, _, _ := TestFactoryWithRuntimePlan(t, nil,
		runtimeplan.Failed(startupErr, runtimeplan.MetadataEmbeddedOnly))

	if err := f.RequireRuntimeCapabilities(context.Background(), "local recovery"); err != nil {
		t.Fatalf("capability-free local command = %v, want available", err)
	}
}

func TestRequireRuntimeCapabilitiesBlocksProviderOwnedCredentials(t *testing.T) {
	provider := &runtimeCapabilityProvider{}
	f, _, _, _ := TestFactory(t, nil)
	f.Credential = credential.NewCredentialProvider(
		[]extcred.Provider{provider}, nil, nil, nil,
	)

	err := f.RequireRuntimeCapabilities(
		context.Background(),
		"auth login",
		runtimeplan.CapabilityLocalCredentialManagement,
	)
	if err == nil {
		t.Fatal("expected provider-owned credentials to reject local management")
	}
	if err := f.RequireRuntimeCapabilities(
		context.Background(),
		"profile use",
		runtimeplan.CapabilityLocalProfileMutation,
	); err != nil {
		t.Fatalf("generic provider unexpectedly blocked Standard Profile mutation: %v", err)
	}
}

type runtimeCapabilityProvider struct{}

func (*runtimeCapabilityProvider) Name() string { return "test-provider" }
func (*runtimeCapabilityProvider) ResolveAccount(context.Context) (*extcred.Account, error) {
	return &extcred.Account{AppID: "cli_test"}, nil
}
func (*runtimeCapabilityProvider) ResolveToken(context.Context, extcred.TokenSpec) (*extcred.Token, error) {
	return nil, nil
}
