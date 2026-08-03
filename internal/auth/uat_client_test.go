// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"bytes"
	"errors"
	"net/http"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
)

type uatRoundTripFunc func(*http.Request) (*http.Response, error)

func (fn uatRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

type uatStoreStub struct {
	mu          sync.Mutex
	stored      *StoredUAToken
	removeCalls int
}

func installUATStoreStub(t *testing.T, initial *StoredUAToken) *uatStoreStub {
	t.Helper()
	stub := &uatStoreStub{stored: cloneStoredUAToken(initial)}
	originalGet := getStoredUAToken
	originalSet := setStoredUAToken
	originalRemove := removeStoredUAToken

	getStoredUAToken = func(appID, userOpenID string) *StoredUAToken {
		stub.mu.Lock()
		defer stub.mu.Unlock()
		if stub.stored == nil || stub.stored.AppId != appID || stub.stored.UserOpenId != userOpenID {
			return nil
		}
		return cloneStoredUAToken(stub.stored)
	}
	setStoredUAToken = func(token *StoredUAToken) error {
		stub.mu.Lock()
		defer stub.mu.Unlock()
		stub.stored = cloneStoredUAToken(token)
		return nil
	}
	removeStoredUAToken = func(appID, userOpenID string) error {
		stub.mu.Lock()
		defer stub.mu.Unlock()
		stub.removeCalls++
		if stub.stored != nil && stub.stored.AppId == appID && stub.stored.UserOpenId == userOpenID {
			stub.stored = nil
		}
		return nil
	}
	t.Cleanup(func() {
		getStoredUAToken = originalGet
		setStoredUAToken = originalSet
		removeStoredUAToken = originalRemove
	})
	return stub
}

func cloneStoredUAToken(token *StoredUAToken) *StoredUAToken {
	if token == nil {
		return nil
	}
	copy := *token
	return &copy
}

func (s *uatStoreStub) replace(token *StoredUAToken) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stored = cloneStoredUAToken(token)
}

func (s *uatStoreStub) snapshot() (*StoredUAToken, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return cloneStoredUAToken(s.stored), s.removeCalls
}

func refreshTestToken(accessToken, refreshToken string, expiresAt, refreshExpiresAt time.Time) *StoredUAToken {
	return &StoredUAToken{
		UserOpenId:       "ou_user",
		AppId:            "cli_test",
		AccessToken:      accessToken,
		RefreshToken:     refreshToken,
		ExpiresAt:        expiresAt.UnixMilli(),
		RefreshExpiresAt: refreshExpiresAt.UnixMilli(),
		Scope:            "offline_access",
		GrantedAt:        time.Now().Add(-24 * time.Hour).UnixMilli(),
	}
}

func TestRefreshLockPathIsSharedAcrossConfigDirectories(t *testing.T) {
	firstConfigDir := t.TempDir()
	secondConfigDir := t.TempDir()

	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", firstConfigDir)
	first := refreshLockPath("cli_test", "ou_user")
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", secondConfigDir)
	second := refreshLockPath("cli_test", "ou_user")

	if first != second {
		t.Fatalf("AC1: refresh lock changed with config directory: first=%q second=%q", first, second)
	}
	if filepath.Dir(first) != refreshLockDir() {
		t.Fatalf("AC1: refresh lock dir = %q, want credential-scoped dir %q", filepath.Dir(first), refreshLockDir())
	}
}

func TestRefreshReusedReturnsWinnerStoredByConcurrentProcess(t *testing.T) {
	now := time.Now()
	attempted := refreshTestToken("expired-access", "old-refresh", now.Add(-time.Minute), now.Add(72*time.Hour))
	winner := refreshTestToken("winner-access", "new-refresh", now.Add(time.Hour), now.Add(7*24*time.Hour))
	store := installUATStoreStub(t, attempted)

	registry := &httpmock.Registry{}
	registry.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/authen/v2/oauth/token",
		OnMatch: func(_ *http.Request) {
			store.replace(winner)
		},
		Body: map[string]interface{}{
			"code": 20073,
			"msg":  "refresh token has already been used",
		},
	})
	t.Cleanup(func() { registry.Verify(t) })

	got, err := GetValidAccessToken(httpmock.NewClient(registry), UATCallOptions{
		UserOpenId: "ou_user",
		AppId:      "cli_test",
		AppSecret:  "secret",
		Domain:     core.BrandFeishu,
		ErrOut:     &bytes.Buffer{},
	})
	if err != nil {
		t.Fatalf("AC2: concurrent winner should recover without reauthorization: %v", err)
	}
	if got != winner.AccessToken {
		t.Fatalf("AC2: access token = %q, want concurrent winner %q", got, winner.AccessToken)
	}
	stored, removeCalls := store.snapshot()
	if removeCalls != 0 {
		t.Fatalf("AC2: remove calls = %d, want 0", removeCalls)
	}
	if stored == nil || stored.RefreshToken != winner.RefreshToken {
		t.Fatalf("AC2: concurrent winner was removed or replaced: %#v", stored)
	}
}

func TestRefreshRevokedPreservesUnexpiredStoredState(t *testing.T) {
	now := time.Now()
	attempted := refreshTestToken("expired-access", "revoked-refresh", now.Add(-time.Minute), now.Add(72*time.Hour))
	store := installUATStoreStub(t, attempted)

	registry := &httpmock.Registry{}
	registry.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/authen/v2/oauth/token",
		Body: map[string]interface{}{
			"code": 20064,
			"msg":  "refresh token has been revoked",
		},
	})
	t.Cleanup(func() { registry.Verify(t) })

	_, err := GetValidAccessToken(httpmock.NewClient(registry), UATCallOptions{
		UserOpenId: "ou_user",
		AppId:      "cli_test",
		AppSecret:  "secret",
		Domain:     core.BrandFeishu,
		ErrOut:     &bytes.Buffer{},
	})
	if err == nil {
		t.Fatal("AC4: revoked refresh token should return a typed authorization error")
	}
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Subtype != errs.SubtypeRefreshTokenRevoked {
		t.Fatalf("AC4: problem = %#v, want refresh_token_revoked", problem)
	}
	stored, removeCalls := store.snapshot()
	if removeCalls != 0 {
		t.Fatalf("AC4: remove calls = %d, want 0 for unexpired local state", removeCalls)
	}
	if stored == nil || stored.RefreshToken != attempted.RefreshToken {
		t.Fatalf("AC4: refresh state was destroyed: %#v", stored)
	}
}

func TestRefreshTransportFailurePreservesStoredState(t *testing.T) {
	now := time.Now()
	attempted := refreshTestToken("expired-access", "refresh-secret", now.Add(-time.Minute), now.Add(72*time.Hour))
	store := installUATStoreStub(t, attempted)
	client := &http.Client{Transport: uatRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("connection reset")
	})}
	errOut := &bytes.Buffer{}

	_, err := GetValidAccessToken(client, UATCallOptions{
		UserOpenId: "ou_user",
		AppId:      "cli_test",
		AppSecret:  "app-secret",
		Domain:     core.BrandFeishu,
		ErrOut:     errOut,
	})
	if err == nil {
		t.Fatal("AC3: transport failure should return a typed network error")
	}
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Subtype != errs.SubtypeNetworkTransport {
		t.Fatalf("AC3: problem = %#v, want network_transport", problem)
	}
	stored, removeCalls := store.snapshot()
	if removeCalls != 0 {
		t.Fatalf("AC3: remove calls = %d, want 0", removeCalls)
	}
	if stored == nil || stored.RefreshToken != attempted.RefreshToken {
		t.Fatalf("AC3: transport failure destroyed refresh state: %#v", stored)
	}
	combinedOutput := errOut.String() + err.Error()
	for _, secret := range []string{attempted.AccessToken, attempted.RefreshToken, "app-secret"} {
		if bytes.Contains([]byte(combinedOutput), []byte(secret)) {
			t.Fatalf("AC5: output exposed secret %q: %s", secret, combinedOutput)
		}
	}
}

func TestLocallyExpiredRefreshTokenIsPreserved(t *testing.T) {
	now := time.Now()
	expired := refreshTestToken("expired-access", "expired-refresh", now.Add(-time.Hour), now.Add(-time.Minute))
	store := installUATStoreStub(t, expired)

	_, err := GetValidAccessToken(http.DefaultClient, UATCallOptions{
		UserOpenId: "ou_user",
		AppId:      "cli_test",
		Domain:     core.BrandFeishu,
		ErrOut:     &bytes.Buffer{},
	})
	if err == nil {
		t.Fatal("AC4: expired refresh token should return a typed error")
	}
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Subtype != errs.SubtypeRefreshTokenExpired {
		t.Fatalf("AC4: problem = %#v, want refresh_token_expired", problem)
	}
	stored, removeCalls := store.snapshot()
	if removeCalls != 0 {
		t.Fatalf("AC4: remove calls = %d, want 0", removeCalls)
	}
	if stored == nil || stored.RefreshToken != expired.RefreshToken {
		t.Fatalf("AC4: locally expired state was destroyed: %#v", stored)
	}
}
