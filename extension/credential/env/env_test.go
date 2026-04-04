package env

import (
	"context"
	"errors"
	"testing"

	"github.com/larksuite/cli/extension/credential"
)

func TestProvider_Name(t *testing.T) {
	if (&Provider{}).Name() != "env" {
		t.Fail()
	}
}

func TestResolveAccount_BothSet(t *testing.T) {
	t.Setenv("LARK_APP_ID", "cli_test")
	t.Setenv("LARK_APP_SECRET", "secret_test")
	t.Setenv("LARK_BRAND", "feishu")

	acct, err := (&Provider{}).ResolveAccount(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if acct.AppID != "cli_test" || acct.AppSecret != "secret_test" || acct.Brand != "feishu" {
		t.Errorf("unexpected: %+v", acct)
	}
}

func TestResolveAccount_NeitherSet(t *testing.T) {
	acct, err := (&Provider{}).ResolveAccount(context.Background())
	if err != nil || acct != nil {
		t.Errorf("expected nil, nil; got %+v, %v", acct, err)
	}
}

func TestResolveAccount_OnlyIDSet(t *testing.T) {
	t.Setenv("LARK_APP_ID", "cli_test")
	_, err := (&Provider{}).ResolveAccount(context.Background())
	var blockErr *credential.BlockError
	if !errors.As(err, &blockErr) {
		t.Fatalf("expected BlockError, got %v", err)
	}
}

func TestResolveAccount_OnlySecretSet(t *testing.T) {
	t.Setenv("LARK_APP_SECRET", "secret_test")
	_, err := (&Provider{}).ResolveAccount(context.Background())
	var blockErr *credential.BlockError
	if !errors.As(err, &blockErr) {
		t.Fatalf("expected BlockError, got %v", err)
	}
}

func TestResolveAccount_DefaultBrand(t *testing.T) {
	t.Setenv("LARK_APP_ID", "cli_test")
	t.Setenv("LARK_APP_SECRET", "secret_test")
	acct, _ := (&Provider{}).ResolveAccount(context.Background())
	if acct.Brand != "lark" {
		t.Errorf("expected 'lark', got %q", acct.Brand)
	}
}

func TestResolveToken_UATSet(t *testing.T) {
	t.Setenv("LARK_USER_ACCESS_TOKEN", "u-env")
	tok, err := (&Provider{}).ResolveToken(context.Background(), credential.TokenSpec{Type: credential.TokenTypeUAT})
	if err != nil {
		t.Fatal(err)
	}
	if tok.Value != "u-env" || tok.Source != "env:LARK_USER_ACCESS_TOKEN" {
		t.Errorf("unexpected: %+v", tok)
	}
}

func TestResolveToken_TATSet(t *testing.T) {
	t.Setenv("LARK_TENANT_ACCESS_TOKEN", "t-env")
	tok, err := (&Provider{}).ResolveToken(context.Background(), credential.TokenSpec{Type: credential.TokenTypeTAT})
	if err != nil {
		t.Fatal(err)
	}
	if tok.Value != "t-env" || tok.Source != "env:LARK_TENANT_ACCESS_TOKEN" {
		t.Errorf("unexpected: %+v", tok)
	}
}

func TestResolveToken_NotSet(t *testing.T) {
	tok, err := (&Provider{}).ResolveToken(context.Background(), credential.TokenSpec{Type: credential.TokenTypeUAT})
	if err != nil || tok != nil {
		t.Errorf("expected nil, nil; got %+v, %v", tok, err)
	}
}

func TestResolveAccount_StrictModeBot(t *testing.T) {
	t.Setenv("LARK_APP_ID", "app")
	t.Setenv("LARK_APP_SECRET", "secret")
	t.Setenv("LARKSUITE_CLI_STRICT_MODE", "bot")
	acct, err := (&Provider{}).ResolveAccount(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !acct.SupportedIdentities.BotOnly() {
		t.Errorf("expected bot-only, got %d", acct.SupportedIdentities)
	}
}

func TestResolveAccount_StrictModeUser(t *testing.T) {
	t.Setenv("LARK_APP_ID", "app")
	t.Setenv("LARK_APP_SECRET", "secret")
	t.Setenv("LARKSUITE_CLI_STRICT_MODE", "user")
	acct, err := (&Provider{}).ResolveAccount(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !acct.SupportedIdentities.UserOnly() {
		t.Errorf("expected user-only, got %d", acct.SupportedIdentities)
	}
}

func TestResolveAccount_StrictModeOff(t *testing.T) {
	t.Setenv("LARK_APP_ID", "app")
	t.Setenv("LARK_APP_SECRET", "secret")
	t.Setenv("LARKSUITE_CLI_STRICT_MODE", "off")
	acct, err := (&Provider{}).ResolveAccount(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if acct.SupportedIdentities != credential.SupportsAll {
		t.Errorf("expected SupportsAll, got %d", acct.SupportedIdentities)
	}
}

func TestResolveAccount_InferFromUATOnly(t *testing.T) {
	t.Setenv("LARK_APP_ID", "app")
	t.Setenv("LARK_APP_SECRET", "secret")
	t.Setenv("LARK_USER_ACCESS_TOKEN", "u-tok")
	acct, err := (&Provider{}).ResolveAccount(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !acct.SupportedIdentities.UserOnly() {
		t.Errorf("expected user-only from UAT inference, got %d", acct.SupportedIdentities)
	}
}

func TestResolveAccount_InferFromTATOnly(t *testing.T) {
	t.Setenv("LARK_APP_ID", "app")
	t.Setenv("LARK_APP_SECRET", "secret")
	t.Setenv("LARK_TENANT_ACCESS_TOKEN", "t-tok")
	acct, err := (&Provider{}).ResolveAccount(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !acct.SupportedIdentities.BotOnly() {
		t.Errorf("expected bot-only from TAT inference, got %d", acct.SupportedIdentities)
	}
}

func TestResolveAccount_InferBothTokens(t *testing.T) {
	t.Setenv("LARK_APP_ID", "app")
	t.Setenv("LARK_APP_SECRET", "secret")
	t.Setenv("LARK_USER_ACCESS_TOKEN", "u-tok")
	t.Setenv("LARK_TENANT_ACCESS_TOKEN", "t-tok")
	acct, err := (&Provider{}).ResolveAccount(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if acct.SupportedIdentities != credential.SupportsAll {
		t.Errorf("expected SupportsAll, got %d", acct.SupportedIdentities)
	}
}

func TestResolveAccount_StrictModeOverridesTokenInference(t *testing.T) {
	t.Setenv("LARK_APP_ID", "app")
	t.Setenv("LARK_APP_SECRET", "secret")
	t.Setenv("LARK_USER_ACCESS_TOKEN", "u-tok")
	t.Setenv("LARK_TENANT_ACCESS_TOKEN", "t-tok")
	t.Setenv("LARKSUITE_CLI_STRICT_MODE", "bot")
	acct, err := (&Provider{}).ResolveAccount(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !acct.SupportedIdentities.BotOnly() {
		t.Errorf("strict mode should override token inference, got %d", acct.SupportedIdentities)
	}
}
