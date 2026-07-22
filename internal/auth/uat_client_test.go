// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/larksuite/cli/internal/auth/jwt"
	"github.com/larksuite/cli/internal/core"
)

type uatRoundTripFunc func(*http.Request) (*http.Response, error)

func (fn uatRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

type retryExternalAssertionSigner struct {
	calls int
}

func (s *retryExternalAssertionSigner) SignClientAssertion(_ context.Context, _, _, _ string) (string, string, error) {
	s.calls++
	return jwt.ClientAssertionType, fmt.Sprintf("refresh.jwt.%d", s.calls), nil
}

func TestDoRefreshToken_PrivateKeyJWTRetryResolvesOnceAndRemintsAssertion(t *testing.T) {
	signer := &retryExternalAssertionSigner{}
	resolveCalls := 0
	previous := resolveExternalAssertionSigner
	resolveExternalAssertionSigner = func(_ context.Context, provider string) (clientAssertionSigner, error) {
		resolveCalls++
		if provider != core.KeylessProviderLarkSuite {
			t.Fatalf("provider = %q", provider)
		}
		return signer, nil
	}
	t.Cleanup(func() { resolveExternalAssertionSigner = previous })

	var forms []url.Values
	httpClient := &http.Client{Transport: uatRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
		form, err := url.ParseQuery(string(body))
		if err != nil {
			return nil, err
		}
		forms = append(forms, form)

		responseBody := `{"code":20050,"error":"server_error","error_description":"retry"}`
		if len(forms) == 2 {
			// A success-shaped response without a token lets the test exercise the
			// retry without writing platform keychain state.
			responseBody = `{"code":0}`
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(responseBody)),
			Request:    req,
		}, nil
	})}

	now := time.Now().UnixMilli()
	stored := &StoredUAToken{
		UserOpenId:       "ou_test",
		AppId:            "cli_external",
		RefreshToken:     "refresh-token",
		RefreshExpiresAt: now + int64(time.Hour/time.Millisecond),
		Scope:            "offline_access",
		GrantedAt:        now,
	}
	opts := UATCallOptions{
		UserOpenId:  stored.UserOpenId,
		AppId:       stored.AppId,
		Domain:      core.BrandFeishu,
		AuthMethod:  core.AuthMethodPrivateKeyJWT,
		KeyLabel:    "openclaw-lark",
		KeyProvider: core.KeylessProviderLarkSuite,
		ErrOut:      io.Discard,
	}

	updated, err := doRefreshToken(httpClient, opts, stored)
	if err == nil || !strings.Contains(err.Error(), "no access_token") {
		t.Fatalf("doRefreshToken error = %v, want missing access_token after retry", err)
	}
	if updated != nil {
		t.Fatalf("updated token = %#v, want nil", updated)
	}
	if resolveCalls != 1 {
		t.Fatalf("provider resolution calls = %d, want 1", resolveCalls)
	}
	if signer.calls != 2 {
		t.Fatalf("assertion signing calls = %d, want 2", signer.calls)
	}
	if len(forms) != 2 {
		t.Fatalf("token endpoint requests = %d, want 2", len(forms))
	}
	first := forms[0].Get("client_assertion")
	second := forms[1].Get("client_assertion")
	if first == "" || second == "" || first == second {
		t.Fatalf("assertions = (%q, %q), want two fresh values", first, second)
	}
	for _, form := range forms {
		if form.Get("grant_type") != "refresh_token" {
			t.Fatalf("grant_type = %q, want refresh_token", form.Get("grant_type"))
		}
		if form.Has("client_secret") {
			t.Fatalf("private_key_jwt form leaked client_secret: %v", form)
		}
	}
}
