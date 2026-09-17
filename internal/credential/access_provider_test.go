// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package credential

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/larksuite/cli/errs"
	extcred "github.com/larksuite/cli/extension/credential"
	"github.com/larksuite/cli/internal/core"
)

// accessExtProvider implements the optional access interface on top of the
// long-standing Provider, which is how an external credential source opts in to
// returning a credential and its subject together.
type accessExtProvider struct {
	mockExtProvider
	access    *extcred.Access
	accessErr error
	// tokenCalls counts legacy resolutions, so a test can prove an Agent
	// Employee account never reached the path that cannot verify it.
	tokenCalls *int
}

func (p *accessExtProvider) ResolveAccess(ctx context.Context, req extcred.AccessSpec) (*extcred.Access, error) {
	if p.accessErr != nil {
		return nil, p.accessErr
	}
	return p.access, nil
}

func (p *accessExtProvider) ResolveToken(ctx context.Context, req extcred.TokenSpec) (*extcred.Token, error) {
	if p.tokenCalls != nil {
		*p.tokenCalls++
	}
	return p.mockExtProvider.ResolveToken(ctx, req)
}

// countingLegacyProvider is a provider that never implements the access
// interface, used to prove which requests fall through to the legacy path.
type countingLegacyProvider struct {
	mockExtProvider
	tokenCalls *int
}

func (p *countingLegacyProvider) ResolveToken(ctx context.Context, req extcred.TokenSpec) (*extcred.Token, error) {
	if p.tokenCalls != nil {
		*p.tokenCalls++
	}
	return p.mockExtProvider.ResolveToken(ctx, req)
}

func agentAccount() *extcred.Account {
	return &extcred.Account{
		AppID:               "cli_agent",
		AppSecret:           extcred.NoAppSecret,
		Brand:               extcred.BrandFeishu,
		OpenID:              "ou_agent",
		CredentialKind:      extcred.CredentialAAT,
		SupportedIdentities: extcred.SupportsUser,
	}
}

func userSpec() AccessSpec { return AccessSpec{Identity: core.AsUser, AppID: "cli_agent"} }

// TestExternalUnclassifiedAccountUsesLegacyTokenPath pins that a provider which
// never heard of credential kinds keeps working exactly as before.
func TestExternalUnclassifiedAccountUsesLegacyTokenPath(t *testing.T) {
	calls := 0
	prov := &countingLegacyProvider{
		mockExtProvider: mockExtProvider{
			name:    "legacy",
			account: &extcred.Account{AppID: "cli_legacy", Brand: extcred.BrandFeishu},
			token:   &extcred.Token{Value: "uat-value", Scopes: "im:message:read"},
		},
		tokenCalls: &calls,
	}
	p := NewCredentialProvider([]extcred.Provider{prov}, nil, nil, nil)

	access, err := p.ResolveAccess(context.Background(), AccessSpec{Identity: core.AsUser, AppID: "cli_legacy"})
	if err != nil {
		t.Fatalf("ResolveAccess() error = %v", err)
	}
	if access.Value != "uat-value" || access.Kind != core.CredentialUAT {
		t.Fatalf("access = %#v, want the legacy user token", access)
	}
	if access.VerificationSource != VerificationLegacy {
		t.Fatalf("verificationSource = %q, want legacy", access.VerificationSource)
	}
	if calls == 0 {
		t.Fatal("an unclassified account must still use the legacy token path")
	}
}

// TestExternalAccessProviderPreferredWhenPresent pins that a provider offering
// the access interface is used for it even on an unclassified account.
func TestExternalAccessProviderPreferredWhenPresent(t *testing.T) {
	calls := 0
	prov := &accessExtProvider{
		mockExtProvider: mockExtProvider{
			name:    "external",
			account: &extcred.Account{AppID: "cli_legacy", Brand: extcred.BrandFeishu},
			token:   &extcred.Token{Value: "should-not-be-used"},
		},
		access:     &extcred.Access{Value: "access-value", Kind: extcred.CredentialUAT, Scopes: "im:message:read"},
		tokenCalls: &calls,
	}
	p := NewCredentialProvider([]extcred.Provider{prov}, nil, nil, nil)

	access, err := p.ResolveAccess(context.Background(), AccessSpec{Identity: core.AsUser, AppID: "cli_legacy"})
	if err != nil {
		t.Fatalf("ResolveAccess() error = %v", err)
	}
	if access.Value != "access-value" {
		t.Fatalf("value = %q, want the access-interface result", access.Value)
	}
	if access.VerificationSource != VerificationProviderManaged {
		t.Fatalf("verificationSource = %q, want provider_managed", access.VerificationSource)
	}
}

// TestExternalAgentAccountResolvesThroughAccessProvider pins the intended path
// for an Agent Employee served by an external system.
func TestExternalAgentAccountResolvesThroughAccessProvider(t *testing.T) {
	calls := 0
	prov := &accessExtProvider{
		mockExtProvider: mockExtProvider{name: "external", account: agentAccount()},
		access: &extcred.Access{
			Value:     "external-aat",
			Kind:      extcred.CredentialAAT,
			OpenID:    "ou_agent",
			ExpiresAt: time.Now().Add(time.Hour),
		},
		tokenCalls: &calls,
	}
	p := NewCredentialProvider([]extcred.Provider{prov}, nil, nil, nil)

	access, err := p.ResolveAccess(context.Background(), userSpec())
	if err != nil {
		t.Fatalf("ResolveAccess() error = %v", err)
	}
	if access.Value != "external-aat" || access.Kind != core.CredentialAAT || access.OpenID != "ou_agent" {
		t.Fatalf("access = %#v", access)
	}
	if calls != 0 {
		t.Fatalf("legacy token path was consulted %d times; an Agent Employee must never reach it", calls)
	}
}

// TestExternalAgentAccountWithoutPrincipalResolvesThroughAccessProvider pins
// that a forwarded Agent Employee credential serves the user lane with no
// principal named on either side, and never reaches the legacy token path.
func TestExternalAgentAccountWithoutPrincipalResolvesThroughAccessProvider(t *testing.T) {
	calls := 0
	acct := agentAccount()
	acct.OpenID = ""
	prov := &accessExtProvider{
		mockExtProvider: mockExtProvider{name: "env", account: acct},
		access:          &extcred.Access{Value: "injected-aat", Kind: extcred.CredentialAAT},
		tokenCalls:      &calls,
	}
	p := NewCredentialProvider([]extcred.Provider{prov}, nil, nil, nil)

	resolved, err := p.ResolveAccount(context.Background())
	if err != nil {
		t.Fatalf("ResolveAccount() error = %v", err)
	}
	if resolved.CredentialKind != core.CredentialAAT || resolved.PrincipalOpenID != "" {
		t.Fatalf("account = %#v, want aat with no principal", resolved)
	}
	access, err := p.ResolveAccess(context.Background(), userSpec())
	if err != nil {
		t.Fatalf("ResolveAccess() error = %v", err)
	}
	if access.Value != "injected-aat" || access.Kind != core.CredentialAAT {
		t.Fatalf("access = %#v", access)
	}
	if calls != 0 {
		t.Fatalf("legacy token path was consulted %d times; an Agent Employee must never reach it", calls)
	}
}

// TestExternalAgentAccountWithoutAccessProviderFailsClosed pins that a provider
// which classifies an account as an Agent Employee but cannot serve one is
// refused instead of falling back to some other credential.
func TestExternalAgentAccountWithoutAccessProviderFailsClosed(t *testing.T) {
	calls := 0
	prov := &countingLegacyProvider{
		mockExtProvider: mockExtProvider{
			name:    "external",
			account: agentAccount(),
			token:   &extcred.Token{Value: "must-not-be-used"},
		},
		tokenCalls: &calls,
	}
	p := NewCredentialProvider([]extcred.Provider{prov}, nil, nil, nil)

	_, err := p.ResolveAccess(context.Background(), userSpec())
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) || validationErr.Subtype != errs.SubtypeFailedPrecondition {
		t.Fatalf("error = %T %v, want failed_precondition", err, err)
	}
	if calls != 0 {
		t.Fatalf("legacy token path was consulted %d times; there must be no fallback", calls)
	}
}

// TestExternalAgentAccountRejectsBotBeforeConsultingProvider pins that the
// identity refusal happens on static account facts, without asking the provider
// for anything.
func TestExternalAgentAccountRejectsBotBeforeConsultingProvider(t *testing.T) {
	calls := 0
	prov := &accessExtProvider{
		mockExtProvider: mockExtProvider{name: "external", account: agentAccount()},
		access:          &extcred.Access{Value: "unreachable", Kind: extcred.CredentialAAT, OpenID: "ou_agent"},
		tokenCalls:      &calls,
	}
	p := NewCredentialProvider([]extcred.Provider{prov}, nil, nil, nil)

	_, err := p.ResolveAccess(context.Background(), AccessSpec{Identity: core.AsBot, AppID: "cli_agent"})
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) || validationErr.Subtype != errs.SubtypeFailedPrecondition {
		t.Fatalf("error = %T %v, want failed_precondition", err, err)
	}
	if calls != 0 {
		t.Fatal("bot refusal must not consult the credential source")
	}
}

func TestExternalAccessProviderContractViolations(t *testing.T) {
	future := time.Now().Add(time.Hour)
	tests := []struct {
		name   string
		access *extcred.Access
		err    error
	}{
		{
			name:   "nil access leaves an Agent Employee with no fallback",
			access: nil,
		},
		{
			name:   "kind that contradicts the declared account",
			access: &extcred.Access{Value: "v", Kind: extcred.CredentialUAT, OpenID: "ou_agent", ExpiresAt: future},
		},
		{
			name:   "principal that does not match the declared account",
			access: &extcred.Access{Value: "v", Kind: extcred.CredentialAAT, OpenID: "ou_someone_else", ExpiresAt: future},
		},
		{
			name:   "missing principal",
			access: &extcred.Access{Value: "v", Kind: extcred.CredentialAAT, ExpiresAt: future},
		},
		{
			name:   "already expired",
			access: &extcred.Access{Value: "v", Kind: extcred.CredentialAAT, OpenID: "ou_agent", ExpiresAt: time.Now().Add(-time.Minute)},
		},
		{
			name:   "no expiry at all",
			access: &extcred.Access{Value: "v", Kind: extcred.CredentialAAT, OpenID: "ou_agent"},
		},
		{
			name:   "empty credential",
			access: &extcred.Access{Value: "", Kind: extcred.CredentialAAT, OpenID: "ou_agent", ExpiresAt: future},
		},
		{
			name:   "unknown kind",
			access: &extcred.Access{Value: "v", Kind: extcred.CredentialKind("something-else"), OpenID: "ou_agent", ExpiresAt: future},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			prov := &accessExtProvider{
				mockExtProvider: mockExtProvider{
					name:    "external",
					account: agentAccount(),
					token:   &extcred.Token{Value: "must-not-be-used"},
				},
				access:     tc.access,
				tokenCalls: &calls,
			}
			p := NewCredentialProvider([]extcred.Provider{prov}, nil, nil, nil)

			if _, err := p.ResolveAccess(context.Background(), userSpec()); err == nil {
				t.Fatal("expected the request to fail rather than send an unchecked credential")
			}
			if calls != 0 {
				t.Fatalf("legacy token path was consulted %d times; a rejected Access must not fall back", calls)
			}
		})
	}
}

// TestExternalAccessProviderErrorPreservesCause pins that a provider's own
// refusal reaches the caller intact instead of being replaced by a generic
// credential failure.
func TestExternalAccessProviderErrorPreservesCause(t *testing.T) {
	blocked := &extcred.BlockError{Provider: "external", Reason: "credential service unreachable"}
	prov := &accessExtProvider{
		mockExtProvider: mockExtProvider{name: "external", account: agentAccount()},
		accessErr:       blocked,
	}
	p := NewCredentialProvider([]extcred.Provider{prov}, nil, nil, nil)

	_, err := p.ResolveAccess(context.Background(), userSpec())
	var blockErr *extcred.BlockError
	if !errors.As(err, &blockErr) {
		t.Fatalf("error = %T %v, want the provider's own BlockError", err, err)
	}
}

// TestExternalAgentAccountSkipsScopeResolution pins that a scope pre-check on an
// Agent Employee profile reports "no scope facts" without resolving anything: an
// Agent Employee credential is issued without scopes, and a pre-check must not
// acquire a credential to discover that.
func TestExternalAgentAccountSkipsScopeResolution(t *testing.T) {
	calls := 0
	prov := &accessExtProvider{
		mockExtProvider: mockExtProvider{name: "external", account: agentAccount()},
		access: &extcred.Access{
			Value: "external-aat", Kind: extcred.CredentialAAT,
			OpenID: "ou_agent", ExpiresAt: time.Now().Add(time.Hour),
		},
		tokenCalls: &calls,
	}
	p := NewCredentialProvider([]extcred.Provider{prov}, nil, nil, nil)

	meta, err := p.ResolveScopeMetadata(context.Background(), userSpec())
	if err != nil {
		t.Fatalf("ResolveScopeMetadata() error = %v", err)
	}
	if meta.Available || meta.Scopes != "" {
		t.Fatalf("meta = %#v, want no scope facts", meta)
	}
	if calls != 0 {
		t.Fatalf("legacy token path was consulted %d times during a pre-check", calls)
	}
}

// TestExternalAgentAccountAutoIdentityIsUser pins that auto has one possible
// answer for an Agent Employee and needs no probing to find it.
func TestExternalAgentAccountAutoIdentityIsUser(t *testing.T) {
	prov := &accessExtProvider{
		mockExtProvider: mockExtProvider{name: "external", account: agentAccount()},
	}
	p := NewCredentialProvider([]extcred.Provider{prov}, nil, nil, nil)

	hint, err := p.ResolveIdentityHint(context.Background())
	if err != nil {
		t.Fatalf("ResolveIdentityHint() error = %v", err)
	}
	if hint.AutoAs != core.AsUser {
		t.Fatalf("AutoAs = %q, want user", hint.AutoAs)
	}
}
