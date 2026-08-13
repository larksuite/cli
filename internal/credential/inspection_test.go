// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package credential

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	extcred "github.com/larksuite/cli/extension/credential"
	"github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/vfs"
)

type inspectionExtProvider struct {
	capabilities ProviderCapabilities
	token        *extcred.Token
	accountErr   error
	accountCalls int
	tokenCalls   int
}

func (p *inspectionExtProvider) Name() string { return "inspection-test" }
func (p *inspectionExtProvider) ResolveAccount(context.Context) (*extcred.Account, error) {
	p.accountCalls++
	if p.accountErr != nil {
		return nil, p.accountErr
	}
	return &extcred.Account{
		AppID:               "cli_test",
		Brand:               extcred.BrandFeishu,
		SupportedIdentities: extcred.SupportsAll,
	}, nil
}
func (p *inspectionExtProvider) ResolveToken(context.Context, extcred.TokenSpec) (*extcred.Token, error) {
	p.tokenCalls++
	return p.token, nil
}
func (p *inspectionExtProvider) CredentialCapabilities() ProviderCapabilities {
	return p.capabilities
}

func TestInspectSourceUsesSingleCachedProviderSelection(t *testing.T) {
	ext := &inspectionExtProvider{}
	provider := NewCredentialProvider([]extcred.Provider{ext}, nil, nil, nil)

	first, err := provider.InspectSource(context.Background())
	if err != nil {
		t.Fatalf("first InspectSource() error = %v", err)
	}
	second, err := provider.InspectSource(context.Background())
	if err != nil {
		t.Fatalf("second InspectSource() error = %v", err)
	}
	if ext.accountCalls != 1 {
		t.Fatalf("ResolveAccount() calls = %d, want one immutable selection", ext.accountCalls)
	}
	if !first.Managed || first.Name != "inspection-test" || first.AppID != "cli_test" {
		t.Fatalf("first inspection = %#v", first)
	}
	if *first != *second {
		t.Fatalf("source inspection changed: first=%#v second=%#v", first, second)
	}
}

func TestInspectSourcePreservesDefaultCommandErrorPath(t *testing.T) {
	resolveErr := errors.New("default account is not configured")
	provider := NewCredentialProvider(nil, &mockDefaultAcct{err: resolveErr}, nil, nil)

	got, err := provider.InspectSource(context.Background())
	if err != nil {
		t.Fatalf("InspectSource() error = %v, want default resolution deferred to the command", err)
	}
	if got == nil || got.Managed || got.Name != "default" {
		t.Fatalf("InspectSource() = %#v, want unmanaged default source", got)
	}
}

func TestInspectSourceManagedFailureRemainsFailClosed(t *testing.T) {
	resolveErr := errors.New("managed provider unavailable")
	ext := &inspectionExtProvider{accountErr: resolveErr}
	provider := NewCredentialProvider([]extcred.Provider{ext}, nil, nil, nil)

	got, err := provider.InspectSource(context.Background())
	if !errors.Is(err, resolveErr) {
		t.Fatalf("InspectSource() error = %v, want %v", err, resolveErr)
	}
	if got == nil || !got.Managed || got.Name != "inspection-test" {
		t.Fatalf("InspectSource() = %#v, want selected managed source", got)
	}
}

func TestInspectToken_DefaultPreservesStoredScopeMetadataWithoutCredentialValue(t *testing.T) {
	originalGet := getStoredToken
	originalStatus := getStoredTokenStatus
	t.Cleanup(func() {
		getStoredToken = originalGet
		getStoredTokenStatus = originalStatus
	})

	getStoredToken = func(appID, openID string) *auth.StoredUAToken {
		return &auth.StoredUAToken{
			AppId:            appID,
			UserOpenId:       openID,
			AccessToken:      "must-not-leak",
			RefreshToken:     "must-not-leak-refresh",
			Scope:            "im:message docx:document",
			ExpiresAt:        11,
			RefreshExpiresAt: 22,
			GrantedAt:        33,
		}
	}
	getStoredTokenStatus = func(*auth.StoredUAToken) string { return "valid" }

	provider := NewCredentialProvider(nil, &mockDefaultAcct{account: &Account{
		AppID: "cli_test", UserOpenId: "ou_test",
	}}, &mockDefaultToken{}, nil)
	got, err := provider.InspectToken(context.Background(), TokenInspectionRequest{
		TokenSpec: TokenSpec{Type: TokenTypeUAT, AppID: "cli_test"},
	})
	if err != nil {
		t.Fatalf("InspectToken() error = %v", err)
	}
	if !got.Present || got.Status != TokenInspectionReady || got.ScopeState != ScopeKnown {
		t.Fatalf("inspection = %#v", got)
	}
	if got.Scopes != "im:message docx:document" || got.ExpiresAtMillis != 11 ||
		got.RefreshExpiresAtMillis != 22 || got.GrantedAtMillis != 33 {
		t.Fatalf("metadata = %#v", got)
	}
	if rendered := fmt.Sprintf("%+v", got); strings.Contains(rendered, "must-not-leak") {
		t.Fatalf("inspection leaked credential value: %s", rendered)
	}
}

func TestTokenInspectionHasNoCredentialValueField(t *testing.T) {
	typ := reflect.TypeOf(TokenInspection{})
	for i := 0; i < typ.NumField(); i++ {
		name := strings.ToLower(typ.Field(i).Name)
		for _, forbidden := range []string{"token", "secret", "credential", "value"} {
			if strings.Contains(name, forbidden) {
				t.Fatalf("TokenInspection field %q could carry a credential value", typ.Field(i).Name)
			}
		}
	}
}

func TestInspectToken_UninspectableManagedScopesDoNotResolveToken(t *testing.T) {
	ext := &inspectionExtProvider{
		capabilities: ProviderCapabilities{ProvidesOnDemandAuth: true, CanInspectScopes: false},
		token:        &extcred.Token{Value: "opaque-placeholder", Scopes: "must:not:be:used"},
	}
	provider := NewCredentialProvider([]extcred.Provider{ext}, nil, nil, nil)
	got, err := provider.InspectToken(context.Background(), TokenInspectionRequest{
		TokenSpec:     TokenSpec{Type: TokenTypeUAT, AppID: "cli_test"},
		IncludeScopes: true,
	})
	if err != nil {
		t.Fatalf("InspectToken() error = %v", err)
	}
	if got.ScopeState != ScopeUnsupported || !got.Present || got.Status != TokenInspectionAvailableLive {
		t.Fatalf("inspection = %#v", got)
	}
	if ext.tokenCalls != 0 {
		t.Fatalf("ResolveToken() calls = %d, want 0", ext.tokenCalls)
	}
}

func TestInspectToken_InspectableManagedSourceDistinguishesScopeState(t *testing.T) {
	for _, test := range []struct {
		name      string
		scopes    string
		wantState ScopeState
	}{
		{name: "known", scopes: "im:message", wantState: ScopeKnown},
		{name: "unknown", scopes: "", wantState: ScopeUnknown},
		{name: "blank is unknown", scopes: "   ", wantState: ScopeUnknown},
	} {
		t.Run(test.name, func(t *testing.T) {
			ext := &inspectionExtProvider{
				capabilities: ProviderCapabilities{ProvidesOnDemandAuth: true, CanInspectScopes: true},
				token:        &extcred.Token{Value: "external-secret-token", Scopes: test.scopes},
			}
			provider := NewCredentialProvider([]extcred.Provider{ext}, nil, nil, nil)
			got, err := provider.InspectToken(context.Background(), TokenInspectionRequest{
				TokenSpec:     TokenSpec{Type: TokenTypeUAT, AppID: "cli_test"},
				IncludeScopes: true,
			})
			if err != nil {
				t.Fatalf("InspectToken() error = %v", err)
			}
			if got.ScopeState != test.wantState || !got.Present {
				t.Fatalf("inspection = %#v, want scope state %q", got, test.wantState)
			}
			if rendered := fmt.Sprintf("%+v", got); strings.Contains(rendered, "external-secret-token") {
				t.Fatalf("inspection leaked credential value: %s", rendered)
			}
		})
	}
}

func TestDiagnosticCommandsUseCredentialInspectionBoundary(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller() could not locate test source")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
	files := []string{
		"cmd/auth/check.go",
		"cmd/auth/status.go",
		"cmd/config/show.go",
		"cmd/doctor/doctor.go",
		"internal/identitydiag/diagnostics.go",
	}
	forbidden := []string{
		"ActiveExtensionProviderName(",
		"GetStoredToken(",
		"TokenStatus(",
		"internal/keychain",
		"internal/externalcredential",
		"os.Getenv(",
		".ExternalCredential.Mode",
		"ExternalCredential != nil",
		"ExternalCredential == nil",
	}
	for _, relative := range files {
		data, err := vfs.ReadFile(filepath.Join(repoRoot, relative))
		if err != nil {
			t.Fatalf("ReadFile(%s) error = %v", relative, err)
		}
		for _, token := range forbidden {
			if strings.Contains(string(data), token) {
				t.Errorf("%s bypasses credential inspection with %q", relative, token)
			}
		}
	}
}
