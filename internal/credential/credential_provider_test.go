package credential

import (
	"context"
	"errors"
	"testing"

	extcred "github.com/larksuite/cli/extension/credential"
)

type mockExtProvider struct {
	name    string
	account *extcred.Account
	token   *extcred.Token
	err     error
}

func (m *mockExtProvider) Name() string { return m.name }
func (m *mockExtProvider) ResolveAccount(ctx context.Context) (*extcred.Account, error) {
	return m.account, m.err
}
func (m *mockExtProvider) ResolveToken(ctx context.Context, req extcred.TokenSpec) (*extcred.Token, error) {
	return m.token, m.err
}

type mockDefaultAcct struct {
	account *Account
	err     error
}

func (m *mockDefaultAcct) ResolveAccount(ctx context.Context) (*Account, error) {
	return m.account, m.err
}

type mockDefaultToken struct {
	result *TokenResult
	err    error
}

func (m *mockDefaultToken) ResolveToken(ctx context.Context, req TokenSpec) (*TokenResult, error) {
	return m.result, m.err
}

func TestCredentialProvider_AccountFromExtension(t *testing.T) {
	cp := NewCredentialProvider(
		[]extcred.Provider{&mockExtProvider{name: "env", account: &extcred.Account{AppID: "ext_app", Brand: "lark"}}},
		&mockDefaultAcct{account: &Account{AppID: "default_app"}},
		&mockDefaultToken{}, nil,
	)
	acct, err := cp.ResolveAccount(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if acct.AppID != "ext_app" {
		t.Errorf("expected ext_app, got %s", acct.AppID)
	}
}

func TestCredentialProvider_AccountFallsToDefault(t *testing.T) {
	cp := NewCredentialProvider(
		[]extcred.Provider{&mockExtProvider{name: "skip"}},
		&mockDefaultAcct{account: &Account{AppID: "default_app", Brand: "feishu"}},
		&mockDefaultToken{}, nil,
	)
	acct, err := cp.ResolveAccount(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if acct.AppID != "default_app" {
		t.Errorf("expected default_app, got %s", acct.AppID)
	}
}

func TestCredentialProvider_AccountBlockStopsChain(t *testing.T) {
	cp := NewCredentialProvider(
		[]extcred.Provider{&mockExtProvider{name: "blocker", err: &extcred.BlockError{Provider: "blocker", Reason: "denied"}}},
		&mockDefaultAcct{account: &Account{AppID: "default_app"}},
		&mockDefaultToken{}, nil,
	)
	_, err := cp.ResolveAccount(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	var blockErr *extcred.BlockError
	if !errors.As(err, &blockErr) {
		t.Fatalf("expected BlockError, got %T", err)
	}
}

func TestCredentialProvider_AccountCached(t *testing.T) {
	cp := NewCredentialProvider(
		[]extcred.Provider{&mockExtProvider{name: "env", account: &extcred.Account{AppID: "cached"}}},
		nil, nil, nil,
	)
	a1, _ := cp.ResolveAccount(context.Background())
	a2, _ := cp.ResolveAccount(context.Background())
	if a1 != a2 {
		t.Error("expected same pointer (cached)")
	}
}

func TestCredentialProvider_TokenFromExtension(t *testing.T) {
	cp := NewCredentialProvider(
		[]extcred.Provider{&mockExtProvider{name: "env", token: &extcred.Token{Value: "ext_tok", Source: "env"}}},
		&mockDefaultAcct{}, &mockDefaultToken{result: &TokenResult{Token: "default_tok"}}, nil,
	)
	result, err := cp.ResolveToken(context.Background(), TokenSpec{Type: TokenTypeUAT})
	if err != nil {
		t.Fatal(err)
	}
	if result.Token != "ext_tok" {
		t.Errorf("expected ext_tok, got %s", result.Token)
	}
}

func TestCredentialProvider_TokenFallsToDefault(t *testing.T) {
	cp := NewCredentialProvider(
		[]extcred.Provider{&mockExtProvider{name: "skip"}},
		&mockDefaultAcct{}, &mockDefaultToken{result: &TokenResult{Token: "default_tok"}}, nil,
	)
	result, err := cp.ResolveToken(context.Background(), TokenSpec{Type: TokenTypeUAT})
	if err != nil {
		t.Fatal(err)
	}
	if result.Token != "default_tok" {
		t.Errorf("expected default_tok, got %s", result.Token)
	}
}
