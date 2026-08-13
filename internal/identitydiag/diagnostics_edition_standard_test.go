// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build !extended

package identitydiag

import (
	"context"
	"strings"
	"testing"

	extcred "github.com/larksuite/cli/extension/credential"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/credential"
)

func TestStandardDiagnosticsIgnoreEditionOnlyOnDemandCapability(t *testing.T) {
	cfg := &core.CliConfig{
		AppID: "cli_x", Brand: core.BrandFeishu,
		SupportedIdentities: uint8(extcred.SupportsUser),
	}
	f, _, _, _ := cmdutil.TestFactory(t, cfg)
	f.Credential = credential.NewCredentialProvider([]extcred.Provider{&fakeExtProvider{
		name:    "provider",
		account: &extcred.Account{AppID: "cli_x", SupportedIdentities: extcred.SupportsUser},
		caps: credential.ProviderCapabilities{
			SkipUserInfoEnrichment: true,
			ProvidesOnDemandAuth:   true,
		},
	}}, nil, nil, f.HttpClient)

	got := Diagnose(context.Background(), f, cfg, false)
	if got.User.Available || got.User.Status != StatusMissing {
		t.Fatalf("user = %#v, want established missing/unavailable result", got.User)
	}
}

type standardBlockingProvider struct{}

func (standardBlockingProvider) Name() string { return "corp-provider" }

func (standardBlockingProvider) ResolveAccount(context.Context) (*extcred.Account, error) {
	return nil, &extcred.BlockError{
		Provider: "corp-provider",
		Reason:   "provider is temporarily unavailable",
	}
}

func (standardBlockingProvider) ResolveToken(context.Context, extcred.TokenSpec) (*extcred.Token, error) {
	return nil, nil
}

func TestStandardDiagnosticsPreserveBlockedExtensionSource(t *testing.T) {
	cfg := &core.CliConfig{
		AppID:               "cli_x",
		AppSecret:           "secret",
		Brand:               core.BrandFeishu,
		SupportedIdentities: uint8(extcred.SupportsBot),
	}
	f, _, _, _ := cmdutil.TestFactory(t, cfg)
	f.Credential = credential.NewCredentialProvider(
		[]extcred.Provider{standardBlockingProvider{}},
		nil, nil, f.HttpClient,
	)

	got := Diagnose(context.Background(), f, cfg, false)
	if !strings.Contains(got.Bot.Message, "provided by corp-provider") {
		t.Fatalf("bot = %#v, want established extension-provider diagnosis", got.Bot)
	}
}
