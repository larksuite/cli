// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package credential_test

import (
	"context"
	"testing"

	"github.com/larksuite/cli/brand"
	"github.com/larksuite/cli/envnames"
	extcred "github.com/larksuite/cli/extension/credential"
	envprovider "github.com/larksuite/cli/extension/credential/env"
	configpkg "github.com/larksuite/cli/internal/config"
	"github.com/larksuite/cli/internal/credential"
	"github.com/larksuite/cli/internal/i18n"
	"github.com/larksuite/cli/internal/identity"
	"github.com/larksuite/cli/internal/keychain"
	"github.com/larksuite/cli/internal/secret"
)

type noopKC struct{}

func (n *noopKC) Get(service, account string) (string, error) { return "", nil }
func (n *noopKC) Set(service, account, value string) error    { return nil }
func (n *noopKC) Remove(service, account string) error        { return nil }

func TestFullChain_EnvWins(t *testing.T) {
	t.Setenv(envnames.CliAppID, "env_app")
	t.Setenv(envnames.CliAppSecret, "env_secret")
	t.Setenv(envnames.CliUserAccessToken, "env_uat")

	ep := &envprovider.Provider{}
	cp := credential.NewCredentialProvider(
		[]extcred.Provider{ep},
		nil, nil, nil,
	)

	acct, err := cp.ResolveAccount(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if acct.AppID != "env_app" {
		t.Errorf("expected env_app, got %s", acct.AppID)
	}

	result, err := cp.ResolveToken(context.Background(), credential.TokenSpec{
		Type: credential.TokenTypeUAT, AppID: "env_app",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Token != "env_uat" {
		t.Errorf("expected env_uat, got %s", result.Token)
	}
}

func TestFullChain_Fallthrough(t *testing.T) {
	// env provider returns nil (no env vars set), falls through to default token
	ep := &envprovider.Provider{}
	mock := &mockDefaultTokenProvider{token: "mock_tok", scopes: "drive:read"}

	cp := credential.NewCredentialProvider(
		[]extcred.Provider{ep},
		nil, mock, nil,
	)
	result, err := cp.ResolveToken(context.Background(), credential.TokenSpec{
		Type: credential.TokenTypeUAT, AppID: "app1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Token != "mock_tok" || result.Scopes != "drive:read" {
		t.Errorf("unexpected: %+v", result)
	}
}

type mockDefaultTokenProvider struct {
	token  string
	scopes string
}

func (m *mockDefaultTokenProvider) ResolveToken(ctx context.Context, req credential.TokenSpec) (*credential.TokenResult, error) {
	return &credential.TokenResult{Token: m.token, Scopes: m.scopes}, nil
}

func TestFullChain_ConfigStrictMode(t *testing.T) {
	t.Setenv(envnames.CliAppID, "")
	t.Setenv(envnames.CliAppSecret, "")
	dir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", dir)

	botMode := identity.StrictModeBot
	multi := &configpkg.MultiAppConfig{
		Apps: []configpkg.AppConfig{{
			AppId:      "cfg_app",
			AppSecret:  secret.PlainSecret("cfg_secret"),
			Brand:      brand.Lark,
			StrictMode: &botMode,
		}},
	}
	if err := configpkg.SaveMultiAppConfig(multi); err != nil {
		t.Fatal(err)
	}

	ep := &envprovider.Provider{}
	defaultAcct := credential.NewDefaultAccountProvider(func() keychain.KeychainAccess { return &noopKC{} }, "", brand.ProfileFromConfig)

	cp := credential.NewCredentialProvider(
		[]extcred.Provider{ep},
		defaultAcct, nil, nil,
	)

	acct, err := cp.ResolveAccount(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if acct.SupportedIdentities != uint8(extcred.SupportsBot) {
		t.Errorf("expected SupportsBot (%d), got %d", extcred.SupportsBot, acct.SupportedIdentities)
	}
}

// TestFullChain_LangSurvivesProductionPath exercises the exact data flow the
// production Factory uses (factory_default.go Phase 3): disk → multi config →
// DefaultAccountProvider.ResolveAccount → Account → ToCliConfig. If Lang gets
// dropped at the credential boundary (as it would when Account lacks the field),
// shortcuts/common/runner.go RuntimeContext.Lang() returns "" and downstream
// consumers (mail signature, etc.) silently fall back to defaults — defeating
// the whole point of persisting --lang.
func TestFullChain_LangSurvivesProductionPath(t *testing.T) {
	t.Setenv(envnames.CliAppID, "")
	t.Setenv(envnames.CliAppSecret, "")
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	multi := &configpkg.MultiAppConfig{
		Apps: []configpkg.AppConfig{{
			AppId:     "cfg_app",
			AppSecret: secret.PlainSecret("cfg_secret"),
			Brand:     brand.Feishu,
			Lang:      i18n.LangJaJP,
		}},
	}
	if err := configpkg.SaveMultiAppConfig(multi); err != nil {
		t.Fatalf("SaveMultiAppConfig: %v", err)
	}

	defaultAcct := credential.NewDefaultAccountProvider(func() keychain.KeychainAccess { return &noopKC{} }, "", brand.ProfileFromConfig)
	acct, err := defaultAcct.ResolveAccount(context.Background())
	if err != nil {
		t.Fatalf("ResolveAccount: %v", err)
	}
	if acct.Lang != i18n.LangJaJP {
		t.Errorf("Account.Lang = %q, want %q (DefaultAccountProvider must propagate Lang from config)", acct.Lang, i18n.LangJaJP)
	}

	cfg := acct.ToCliConfig()
	if cfg == nil {
		t.Fatal("ToCliConfig() = nil")
	}
	if cfg.Lang != i18n.LangJaJP {
		t.Errorf("CliConfig.Lang = %q, want %q (this is the value RuntimeContext.Lang() reads in production)", cfg.Lang, i18n.LangJaJP)
	}
}
