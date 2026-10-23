// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"context"
	"crypto/ecdsa"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/auth/jwt"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/internal/keysigner"
)

type revokeRoundTripFunc func(*http.Request) (*http.Response, error)

func (fn revokeRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

type errReadCloser struct {
	err error
}

func (r errReadCloser) Read(_ []byte) (int, error) {
	return 0, r.err
}

func (r errReadCloser) Close() error {
	return nil
}

func TestRevokeToken_PostsExpectedForm(t *testing.T) {
	reg := &httpmock.Registry{}
	t.Cleanup(func() { reg.Verify(t) })

	stub := &httpmock.Stub{
		Method: "POST",
		URL:    PathOAuthRevoke,
		Body:   map[string]interface{}{"code": 0},
		BodyFilter: func(body []byte) bool {
			values, err := url.ParseQuery(string(body))
			if err != nil {
				return false
			}
			return values.Get("client_id") == "cli_a" &&
				values.Get("client_secret") == "secret_b" &&
				values.Get("token") == "user-access-token" &&
				values.Get("token_type_hint") == "access_token"
		},
	}
	reg.Register(stub)

	err := RevokeToken(context.Background(), httpmock.NewClient(reg), ClientAuth{AppID: "cli_a", AppSecret: "secret_b"}, core.BrandFeishu, "user-access-token", "access_token")
	if err != nil {
		t.Fatalf("RevokeToken() error = %v", err)
	}
	if got := stub.CapturedHeaders.Get("Content-Type"); got != "application/x-www-form-urlencoded" {
		t.Fatalf("Content-Type = %q", got)
	}
}

func TestRevokeToken_PrivateKeyJWT(t *testing.T) {
	for _, brand := range []core.LarkBrand{core.BrandFeishu, core.BrandLark} {
		t.Run(string(brand), func(t *testing.T) {
			root := t.TempDir()
			for _, name := range []string{"HOME", "USERPROFILE", "LOCALAPPDATA", "LARKSUITE_CLI_DATA_DIR"} {
				t.Setenv(name, root)
			}
			signer := newFakeAuthSigner(t)
			ca := ClientAuth{AppID: "cli_revoke", AppSecret: "must-not-send", AuthMethod: core.AuthMethodPrivateKeyJWTLocalKeyPair,
				Signer: signer, KeyLabel: "registered-key"}
			seen := map[string]bool{}
			client := &http.Client{Transport: revokeRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				if err := req.ParseForm(); err != nil {
					t.Fatal(err)
				}
				form := req.PostForm
				if req.Method != http.MethodPost || req.URL.Path != PathOAuthRevoke ||
					form.Get("client_id") != ca.AppID || form.Get("token") != "token-to-revoke" ||
					form.Get("client_assertion_type") != jwt.ClientAssertionType || form.Has("client_secret") ||
					req.Header.Get("Authorization") != "" || req.Header.Get("DPoP") != "" {
					t.Fatal("revoke request did not use assertion-only application authentication")
				}
				assertion := form.Get("client_assertion")
				claims := jwtTestPart(t, assertion, 1)
				jti, _ := claims["jti"].(string)
				if claims["iss"] != ca.AppID || claims["sub"] != ca.AppID || claims["aud"] != core.ClientAssertionAudience(brand) || jti == "" || seen[jti] {
					t.Fatal("revoke assertion identity or freshness is incorrect")
				}
				seen[jti] = true
				parts := strings.Split(assertion, ".")
				sig, err := base64.RawURLEncoding.DecodeString(parts[2])
				digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
				if err != nil || len(sig) != 64 || !ecdsa.Verify(&signer.key.PublicKey, digest[:], new(big.Int).SetBytes(sig[:32]), new(big.Int).SetBytes(sig[32:])) {
					t.Fatal("revoke assertion signature is invalid")
				}
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"code":0}`)), Header: make(http.Header)}, nil
			})}
			for _, hint := range []string{"refresh_token", "access_token"} {
				if err := RevokeToken(context.Background(), client, ca, brand, "token-to-revoke", hint); err != nil {
					t.Fatal(err)
				}
			}
			if len(seen) != 2 || signer.ensureCalls != 0 {
				t.Fatalf("assertions=%d, key creations=%d", len(seen), signer.ensureCalls)
			}
		})
	}
}

func TestRevokeToken_MissingSigningKeyDoesNotUseSecret(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"HOME", "USERPROFILE", "LOCALAPPDATA", "LARKSUITE_CLI_DATA_DIR"} {
		t.Setenv(name, root)
	}
	signer := newFakeAuthSigner(t)
	signer.publicErr = keysigner.ErrKeyNotFound
	client := &http.Client{Transport: revokeRoundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("missing signing key must fail before HTTP, without Secret fallback")
		return nil, nil
	})}
	err := RevokeToken(context.Background(), client, ClientAuth{AppID: "cli_revoke", AppSecret: "must-not-send",
		AuthMethod: core.AuthMethodPrivateKeyJWTLocalKeyPair, Signer: signer, KeyLabel: "missing-key"}, core.BrandFeishu, "token", "refresh_token")
	p, ok := errs.ProblemOf(err)
	if !ok || p.Category != errs.CategoryAuthentication || p.Subtype != errs.SubtypeSigningKeyMissing || !errors.Is(err, keysigner.ErrKeyNotFound) || signer.ensureCalls != 0 {
		t.Fatalf("error=%v, key creations=%d", err, signer.ensureCalls)
	}
}

func TestRevokeToken_ExternalSigner(t *testing.T) {
	helper := &fakeExternalAssertionSigner{}
	ca := ClientAuth{AppID: "cli_revoke", AuthMethod: core.AuthMethodPrivateKeyJWTLocalKeyPair,
		KeyProvider: core.KeylessProviderLarkSuite, KeyLabel: "external-key", externalSigner: helper}
	client := &http.Client{Transport: revokeRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if err := req.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if req.PostForm.Get("client_assertion") != "external.jwt.1" || req.PostForm.Has("client_secret") {
			t.Fatal("external assertion was not used for revocation")
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{}`)), Header: make(http.Header)}, nil
	})}
	if err := RevokeToken(context.Background(), client, ca, core.BrandFeishu, "token", "access_token"); err != nil {
		t.Fatal(err)
	}
	if helper.calls != 1 || helper.keyRef != ca.KeyLabel || helper.clientID != ca.AppID || helper.audience != core.ClientAssertionAudience(core.BrandFeishu) {
		t.Fatalf("external signer did not receive the configured key, app and audience: %+v", helper)
	}
}

func TestRevokeToken_DoFailureReturnsTypedNetworkError(t *testing.T) {
	sentinel := errors.New("transport down")
	httpClient := &http.Client{
		Transport: revokeRoundTripFunc(func(req *http.Request) (*http.Response, error) {
			return nil, sentinel
		}),
	}

	err := RevokeToken(context.Background(), httpClient, ClientAuth{AppID: "cli_a", AppSecret: "secret_b"}, core.BrandFeishu, "user-access-token", "access_token")
	if err == nil {
		t.Fatal("expected error")
	}
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed error, got %T", err)
	}
	if p.Category != errs.CategoryNetwork || p.Subtype != errs.SubtypeNetworkTransport {
		t.Fatalf("problem = %#v, want network/transport", p)
	}
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected cause %v to be preserved, got %v", sentinel, err)
	}
}

func TestRevokeToken_ReportsHTTPError(t *testing.T) {
	reg := &httpmock.Registry{}
	t.Cleanup(func() { reg.Verify(t) })

	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    PathOAuthRevoke,
		Status: 400,
		Body:   map[string]interface{}{"error": "invalid_token"},
	})

	err := RevokeToken(context.Background(), httpmock.NewClient(reg), ClientAuth{AppID: "cli_a", AppSecret: "secret_b"}, core.BrandFeishu, "user-access-token", "access_token")
	if err == nil {
		t.Fatal("expected error")
	}
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed error, got %T", err)
	}
	if p.Category != errs.CategoryAPI || p.Code != 400 {
		t.Fatalf("problem = %#v, want api error with HTTP 400", p)
	}
	if !strings.Contains(err.Error(), "invalid_token") {
		t.Fatalf("expected invalid_token error, got %v", err)
	}
}

func TestRevokeToken_ReportsOAuthCodeErrorAsTypedAPIError(t *testing.T) {
	reg := &httpmock.Registry{}
	t.Cleanup(func() { reg.Verify(t) })

	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    PathOAuthRevoke,
		Body: map[string]interface{}{
			"code": 12345,
			"msg":  "invalid revoke state",
		},
	})

	err := RevokeToken(context.Background(), httpmock.NewClient(reg), ClientAuth{AppID: "cli_a", AppSecret: "secret_b"}, core.BrandFeishu, "user-access-token", "access_token")
	if err == nil {
		t.Fatal("expected error")
	}
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed error, got %T", err)
	}
	if p.Category != errs.CategoryAPI || p.Code != 12345 {
		t.Fatalf("problem = %#v, want api error with code 12345", p)
	}
	if !strings.Contains(err.Error(), "invalid revoke state") {
		t.Fatalf("expected oauth error message, got %v", err)
	}
}

func TestRevokeToken_ReportsOAuthErrorFieldAsTypedAPIError(t *testing.T) {
	reg := &httpmock.Registry{}
	t.Cleanup(func() { reg.Verify(t) })

	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    PathOAuthRevoke,
		Body: map[string]interface{}{
			"error":             "invalid_token",
			"error_description": "token already expired",
		},
	})

	err := RevokeToken(context.Background(), httpmock.NewClient(reg), ClientAuth{AppID: "cli_a", AppSecret: "secret_b"}, core.BrandFeishu, "user-access-token", "access_token")
	if err == nil {
		t.Fatal("expected error")
	}
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed error, got %T", err)
	}
	if p.Category != errs.CategoryAPI {
		t.Fatalf("problem = %#v, want api error", p)
	}
	if !strings.Contains(err.Error(), "token already expired") {
		t.Fatalf("expected oauth error_description, got %v", err)
	}
}

func TestRevokeToken_ReadFailureReturnsTypedInternalError(t *testing.T) {
	sentinel := errors.New("read failed")
	httpClient := &http.Client{
		Transport: revokeRoundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       errReadCloser{err: sentinel},
				Header:     make(http.Header),
			}, nil
		}),
	}

	err := RevokeToken(context.Background(), httpClient, ClientAuth{AppID: "cli_a", AppSecret: "secret_b"}, core.BrandFeishu, "user-access-token", "access_token")
	if err == nil {
		t.Fatal("expected error")
	}
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed error, got %T", err)
	}
	if p.Category != errs.CategoryInternal || p.Subtype != errs.SubtypeInvalidResponse {
		t.Fatalf("problem = %#v, want internal/invalid_response", p)
	}
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected cause %v to be preserved, got %v", sentinel, err)
	}
	if !strings.Contains(err.Error(), "token revoke read error") {
		t.Fatalf("expected read error message, got %v", err)
	}
	if _, ok := err.(*errs.InternalError); !ok {
		t.Fatalf("expected *errs.InternalError, got %T", err)
	}
}
