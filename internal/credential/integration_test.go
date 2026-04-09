// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package credential_test

import (
	"context"
	"testing"

	extcred "github.com/larksuite/cli/extension/credential"
	envprovider "github.com/larksuite/cli/extension/credential/env"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/credential"
	"github.com/larksuite/cli/internal/envvars"
)

type noopKC struct{}

func (n *noopKC) Get(service, account string) (string, error) { return "", nil }
func (n *noopKC) Set(service, account, value string) error    { return nil }
func (n *noopKC) Remove(service, account string) error        { return nil }

func TestFullChain_EnvWins(t *testing.T) {
	t.Setenv(envvars.CliAppID, "env_app")
	t.Setenv(envvars.CliAppSecret, "env_secret")
	t.Setenv(envvars.CliUserAccessToken, "env_uat")

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
	t.Setenv(envvars.CliAppID, "")
	t.Setenv(envvars.CliAppSecret, "")
	dir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", dir)

	botMode := core.StrictModeBot
	multi := &core.MultiAppConfig{
		Apps: []core.AppConfig{{
			AppId:      "cfg_app",
			AppSecret:  core.PlainSecret("cfg_secret"),
			Brand:      core.BrandLark,
			StrictMode: &botMode,
		}},
	}
	if err := core.SaveMultiAppConfig(multi); err != nil {
		t.Fatal(err)
	}

	ep := &envprovider.Provider{}
	defaultAcct := credential.NewDefaultAccountProvider(&noopKC{}, "")

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
