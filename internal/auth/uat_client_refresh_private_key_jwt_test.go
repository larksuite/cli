// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"context"
	"errors"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/larksuite/cli/internal/core"
)

type failSecondAssertionSigner struct {
	calls atomic.Int32
}

func (s *failSecondAssertionSigner) SignClientAssertion(context.Context, string, string, string) (string, string, error) {
	if s.calls.Add(1) == 1 {
		return "urn:ietf:params:oauth:client-assertion-type:jwt-bearer", "assertion-first", nil
	}
	return "", "", errors.New("second assertion failed")
}

func TestDoRefreshToken_ProviderResolutionFailsBeforeHTTP(t *testing.T) {
	stored := newRefreshTestToken()
	opts := newRefreshTestOptions(stored)
	opts.AuthMethod = core.AuthMethodPrivateKeyJWTLocalKeyPair
	opts.KeyProvider = core.KeylessProviderLarkSuite
	opts.KeyLabel = "openclaw-lark"

	previous := resolveExternalAssertionSigner
	resolveExternalAssertionSigner = func(context.Context, string) (clientAssertionSigner, error) {
		return nil, errors.New("provider unavailable")
	}
	t.Cleanup(func() { resolveExternalAssertionSigner = previous })

	var calls atomic.Int32
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return nil, errors.New("must not send")
	})}
	refreshed, err := doRefreshToken(context.Background(), client, opts, stored)
	if err == nil || refreshed != nil {
		t.Fatalf("doRefreshToken() = (%v, %v), want provider error", refreshed, err)
	}
	if calls.Load() != 0 {
		t.Fatalf("HTTP calls = %d, want 0", calls.Load())
	}
	if stored.RefreshToken != "refresh-old" {
		t.Fatalf("refresh token mutated to %q", stored.RefreshToken)
	}
}

func TestDoRefreshToken_UncertainRequestThenSigningFailurePreservesToken(t *testing.T) {
	setupStoredTokenTest(t)
	stored := newRefreshTestToken()
	if err := SetStoredToken(stored); err != nil {
		t.Fatalf("SetStoredToken() error = %v", err)
	}
	opts := newRefreshTestOptions(stored)
	opts.AuthMethod = core.AuthMethodPrivateKeyJWTLocalKeyPair
	opts.KeyProvider = core.KeylessProviderLarkSuite
	opts.KeyLabel = "openclaw-lark"

	signer := &failSecondAssertionSigner{}
	previous := resolveExternalAssertionSigner
	resolveExternalAssertionSigner = func(context.Context, string) (clientAssertionSigner, error) {
		return signer, nil
	}
	t.Cleanup(func() { resolveExternalAssertionSigner = previous })

	var calls atomic.Int32
	client := scriptedRefreshClient(t, []refreshHTTPTestStep{{
		err: errors.New("connection lost after write"), markWritten: true,
	}}, &calls)
	refreshed, err := doRefreshToken(context.Background(), client, opts, stored)
	if err == nil || refreshed != nil {
		t.Fatalf("doRefreshToken() = (%v, %v), want signing error", refreshed, err)
	}
	if calls.Load() != 1 || signer.calls.Load() != 2 {
		t.Fatalf("HTTP calls=%d sign calls=%d, want 1 and 2", calls.Load(), signer.calls.Load())
	}
	if IsNeedUserAuthorizationError(err) {
		t.Fatalf("signing failure replaced with need-user-authorization: %v", err)
	}
	if got := mustGetStoredToken(t, stored.AppId, stored.UserOpenId); got == nil || got.RefreshToken != stored.RefreshToken {
		t.Fatalf("stored token = %#v, want original token preserved", got)
	}
}

func TestGetValidAccessTokenPrivateKeyJWTFinishesRefreshAfterCallerCancellation(t *testing.T) {
	signer := newFakeAuthSigner(t)
	setupStoredTokenTest(t)
	stored := newRefreshTestToken()
	opts := newRefreshTestOptions(stored)
	opts.AuthMethod, opts.Signer, opts.KeyLabel = core.AuthMethodPrivateKeyJWT, signer, "app-key"
	if err := SetStoredToken(stored); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		cancel()
		if req.Context().Err() != nil {
			t.Fatal("caller cancellation interrupted token rotation")
		}
		return refreshHTTPResponse(req, `{"access_token":"access-new","refresh_token":"refresh-new","expires_in":120,"refresh_token_expires_in":600}`), nil
	})}
	token, err := GetValidAccessToken(ctx, client, opts)
	if err != nil || token == nil || token.AccessToken != "access-new" {
		t.Fatalf("refresh result = (%+v, %v)", token, err)
	}
	if got := mustGetStoredToken(t, stored.AppId, stored.UserOpenId); got == nil || got.RefreshToken != "refresh-new" {
		t.Fatalf("rotated token not stored: %#v", got)
	}
	if err := withTokenStorageLock(stored.AppId, stored.UserOpenId, func() error { return nil }); err != nil {
		t.Fatalf("lock not released: %v", err)
	}
}
