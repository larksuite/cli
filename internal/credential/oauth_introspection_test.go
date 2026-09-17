// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package credential

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/keychain"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return fn(req) }

func jsonHTTPResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestInspectClientCredentialsMintsWithoutScopeAndIntrospectsAAT(t *testing.T) {
	exp := time.Now().Add(2 * time.Hour).Unix()
	requests := map[string]url.Values{}
	httpClient := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatal(err)
		}
		values, err := url.ParseQuery(string(body))
		if err != nil {
			t.Fatal(err)
		}
		requests[req.URL.Path] = values
		switch req.URL.Path {
		case core.OAuthTokenV3Path:
			return jsonHTTPResponse(http.StatusOK,
				`{"code":0,"access_token":"aat-secret-value","token_type":"Bearer","expires_in":7200}`), nil
		case auth.PathOAuthIntrospect:
			return jsonHTTPResponse(http.StatusOK,
				`{"active":true,"client_id":"cli_aat","token_type":"Bearer","iat":1787639434,"exp":`+
					strconv.FormatInt(exp, 10)+`,"user_type":66,"open_id":"ou_agent"}`), nil
		default:
			t.Fatalf("unexpected path %s", req.URL.Path)
			return nil, nil
		}
	})}

	minted, introspection, err := InspectClientCredentials(
		context.Background(), httpClient, core.BrandFeishu, "cli_aat", "app-secret")
	if err != nil {
		t.Fatalf("InspectClientCredentials() error = %v", err)
	}
	if minted.Token != "aat-secret-value" || minted.ExpiresAt.Sub(minted.GrantedAt) != 2*time.Hour {
		t.Fatalf("minted = %#v", minted)
	}
	if !introspection.IsAgentEmployee() || introspection.OpenID != "ou_agent" {
		t.Fatalf("introspection = %#v", introspection)
	}
	tokenForm := requests[core.OAuthTokenV3Path]
	if tokenForm.Get("grant_type") != "client_credentials" || tokenForm.Get("client_id") != "cli_aat" ||
		tokenForm.Get("client_secret") != "app-secret" {
		t.Fatalf("token form = %#v", tokenForm)
	}
	if _, exists := tokenForm["scope"]; exists {
		t.Fatalf("token form must not contain scope: %#v", tokenForm)
	}
	introspectForm := requests[auth.PathOAuthIntrospect]
	if introspectForm.Get("token") != "aat-secret-value" || introspectForm.Get("client_id") != "cli_aat" ||
		introspectForm.Get("client_secret") != "app-secret" {
		t.Fatalf("introspection form = %#v", introspectForm)
	}
	if _, exists := introspectForm["scope"]; exists {
		t.Fatalf("introspection form must not contain scope: %#v", introspectForm)
	}
}

func TestIntrospectAccessTokenRejectsUntrustedSubjectFacts(t *testing.T) {
	future := time.Now().Add(time.Hour).Unix()
	tests := []struct {
		name        string
		body        string
		wantSubtype errs.Subtype
	}{
		{
			name:        "inactive",
			body:        `{"active":false,"client_id":"cli_aat","exp":` + strconv.FormatInt(future, 10) + `,"user_type":66,"open_id":"ou_agent"}`,
			wantSubtype: errs.SubtypeTokenInvalid,
		},
		{
			name:        "client mismatch",
			body:        `{"active":true,"client_id":"cli_other","exp":` + strconv.FormatInt(future, 10) + `,"user_type":66,"open_id":"ou_agent"}`,
			wantSubtype: errs.SubtypeInvalidClient,
		},
		{
			name:        "missing Agent open ID",
			body:        `{"active":true,"client_id":"cli_aat","exp":` + strconv.FormatInt(future, 10) + `,"user_type":66}`,
			wantSubtype: errs.SubtypeInvalidResponse,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			httpClient := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return jsonHTTPResponse(http.StatusOK, tc.body), nil
			})}
			_, err := IntrospectAccessToken(context.Background(), httpClient, core.BrandFeishu,
				"cli_aat", "app-secret", "token")
			problem, ok := errs.ProblemOf(err)
			if !ok || problem.Subtype != tc.wantSubtype {
				t.Fatalf("problem = %#v, want subtype %s", problem, tc.wantSubtype)
			}
		})
	}
}

func TestIntrospectAccessTokenClassifiesItsOwnErrorResponse(t *testing.T) {
	tests := []struct {
		name        string
		body        string
		wantCode    int
		wantSubtype errs.Subtype
	}{
		{
			name:        "invalid client",
			body:        `{"error":"invalid_client","error_description":"The specified app does not exist.","code":20048}`,
			wantCode:    20048,
			wantSubtype: errs.SubtypeInvalidClient,
		},
		{
			name:        "invalid token",
			body:        `{"error":"invalid_token","error_description":"The token is invalid.","code":20078}`,
			wantCode:    20078,
			wantSubtype: errs.SubtypeTokenInvalid,
		},
		{
			name:        "unknown IAM error",
			body:        `{"error":"invalid_request","error_description":"The request was rejected.","code":1106061}`,
			wantCode:    1106061,
			wantSubtype: errs.SubtypeUnknown,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			httpClient := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return jsonHTTPResponse(http.StatusBadRequest, tc.body), nil
			})}
			_, err := IntrospectAccessToken(context.Background(), httpClient, core.BrandFeishu,
				"cli_aat", "app-secret", "token")
			if err == nil {
				t.Fatal("expected error")
			}
			problem, ok := errs.ProblemOf(err)
			if !ok || problem.Code != tc.wantCode || problem.Subtype != tc.wantSubtype {
				t.Fatalf("problem = %#v, want code=%d subtype=%s", problem, tc.wantCode, tc.wantSubtype)
			}
		})
	}
}

func TestDefaultTokenProviderExpiredAATMintsIntrospectsAndReplacesCache(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	kc := newCredentialMemoryKeychain()
	now := time.Now()
	if err := auth.SetStoredAAT(kc, &auth.StoredAAToken{
		AppID:       "cli_aat",
		UserOpenID:  "ou_agent",
		AccessToken: "expired-aat",
		GrantedAt:   now.Add(-2 * time.Hour).UnixMilli(),
		ExpiresAt:   now.Add(-time.Minute).UnixMilli(),
	}); err != nil {
		t.Fatal(err)
	}
	if err := core.SaveMultiAppConfig(&core.MultiAppConfig{Apps: []core.AppConfig{{
		AppId:           "cli_aat",
		AppSecret:       core.PlainSecret("app-secret"),
		Brand:           core.BrandFeishu,
		Users:           []core.AppUser{},
		CredentialKind:  core.CredentialAAT,
		PrincipalOpenID: "ou_agent",
	}}}); err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(2 * time.Hour).Unix()
	httpClient := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case core.OAuthTokenV3Path:
			return jsonHTTPResponse(http.StatusOK,
				`{"code":0,"access_token":"renewed-aat","expires_in":7200}`), nil
		case auth.PathOAuthIntrospect:
			return jsonHTTPResponse(http.StatusOK,
				`{"active":true,"client_id":"cli_aat","exp":`+strconv.FormatInt(future, 10)+
					`,"user_type":66,"open_id":"ou_agent"}`), nil
		default:
			t.Fatalf("unexpected path %s", req.URL.Path)
			return nil, nil
		}
	})}
	account := NewDefaultAccountProvider(func() keychain.KeychainAccess { return kc }, "", core.ProfileFromConfig)
	provider := NewDefaultTokenProvider(account, func() (*http.Client, error) { return httpClient, nil }, io.Discard)
	spine := NewCredentialProvider(nil, account, provider, nil)
	hint, err := spine.ResolveIdentityHint(context.Background())
	if err != nil || hint.AutoAs != core.AsUser {
		t.Fatalf("ResolveIdentityHint() = %#v, %v; AAT auto must be user", hint, err)
	}

	access, err := spine.ResolveAccess(context.Background(), AccessSpec{Identity: core.AsUser, AppID: "cli_aat"})
	if err != nil {
		t.Fatalf("ResolveAccess() error = %v", err)
	}
	if access.Value != "renewed-aat" {
		t.Fatalf("token = %q, want renewed-aat", access.Value)
	}
	if access.Kind != core.CredentialAAT || access.OpenID != "ou_agent" {
		t.Fatalf("access = %#v, want AAT bound to ou_agent", access)
	}
	if access.VerificationSource != VerificationLocalIntrospection {
		t.Fatalf("verificationSource = %q, want local Introspection", access.VerificationSource)
	}
	stored, err := auth.GetStoredAAT(kc, "cli_aat")
	if err != nil || stored.AccessToken != "renewed-aat" || stored.UserOpenID != "ou_agent" {
		t.Fatalf("stored = %#v, err = %v", stored, err)
	}

	// Bot must fail before the network: the same app credentials mint an Agent
	// Employee token, so a fallback here would put it on the tenant wire.
	_, err = spine.ResolveAccess(context.Background(), AccessSpec{Identity: core.AsBot, AppID: "cli_aat"})
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) || validationErr.Subtype != errs.SubtypeFailedPrecondition {
		t.Fatalf("ResolveAccess(bot) error = %T %v, want failed_precondition", err, err)
	}

	// The pre-existing token path must refuse an Agent Employee profile outright
	// rather than fall through to an unverified credential.
	_, err = provider.ResolveToken(context.Background(), TokenSpec{Type: TokenTypeUAT, AppID: "cli_aat"})
	if err == nil {
		t.Fatal("ResolveToken(UAT) must refuse an Agent Employee profile")
	}
	_, err = provider.ResolveToken(context.Background(), TokenSpec{Type: TokenTypeTAT, AppID: "cli_aat"})
	if !errors.As(err, &validationErr) || validationErr.Subtype != errs.SubtypeFailedPrecondition {
		t.Fatalf("ResolveToken(TAT) error = %T %v, want failed_precondition", err, err)
	}
}

type credentialMemoryKeychain struct {
	items map[string]string
}

func newCredentialMemoryKeychain() *credentialMemoryKeychain {
	return &credentialMemoryKeychain{items: map[string]string{}}
}

func (k *credentialMemoryKeychain) key(service, account string) string {
	return service + "\x00" + account
}
func (k *credentialMemoryKeychain) Get(service, account string) (string, error) {
	return k.items[k.key(service, account)], nil
}
func (k *credentialMemoryKeychain) Set(service, account, value string) error {
	k.items[k.key(service, account)] = value
	return nil
}
func (k *credentialMemoryKeychain) Remove(service, account string) error {
	delete(k.items, k.key(service, account))
	return nil
}
