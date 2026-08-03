// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"bytes"
	"errors"
	"net/http"
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
	mu            sync.Mutex
	stored        *StoredUAToken
	loadErr       error
	persistCalls  int
	failPersistAt map[int]bool
	removeCalls   int
}

func installUATStoreStub(t *testing.T, initial *StoredUAToken) *uatStoreStub {
	t.Helper()
	stub := &uatStoreStub{stored: cloneStoredUAToken(initial), failPersistAt: make(map[int]bool)}
	originalLoad := loadStoredUAToken
	originalPersist := persistStoredUAToken
	originalRemove := removeStoredUAToken

	loadStoredUAToken = func(appID, userOpenID string) (*StoredUAToken, error) {
		stub.mu.Lock()
		defer stub.mu.Unlock()
		if stub.loadErr != nil {
			return nil, stub.loadErr
		}
		if stub.stored == nil || stub.stored.AppId != appID || stub.stored.UserOpenId != userOpenID {
			return nil, nil
		}
		return cloneStoredUAToken(stub.stored), nil
	}
	persistStoredUAToken = func(token *StoredUAToken) error {
		stub.mu.Lock()
		defer stub.mu.Unlock()
		stub.persistCalls++
		if stub.failPersistAt[stub.persistCalls] {
			return errors.New("credential store unavailable")
		}
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
		loadStoredUAToken = originalLoad
		persistStoredUAToken = originalPersist
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
	return cloneStoredUAToken(s.stored), s.persistCalls
}

func (s *uatStoreStub) failPersistCall(call int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failPersistAt[call] = true
}

func (s *uatStoreStub) setLoadError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loadErr = err
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

func TestRefreshReusedReturnsWinnerStoredByConcurrentProcess(t *testing.T) {
	now := time.Now()
	attempted := refreshTestToken("expired-access", "old-refresh", now.Add(-time.Minute), now.Add(72*time.Hour))
	winner := refreshTestToken("winner-access", "old-refresh", now.Add(time.Hour), now.Add(7*24*time.Hour))
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
	stored, _ := store.snapshot()
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
	stored, _ := store.snapshot()
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
	stored, _ := store.snapshot()
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
	stored, _ := store.snapshot()
	if stored == nil || stored.RefreshToken != expired.RefreshToken {
		t.Fatalf("AC4: locally expired state was destroyed: %#v", stored)
	}
}

func TestCredentialStoreReadFailureIsNotReportedAsMissing(t *testing.T) {
	now := time.Now()
	store := installUATStoreStub(t, refreshTestToken("access", "refresh", now.Add(time.Hour), now.Add(24*time.Hour)))
	store.setLoadError(errs.NewInternalError(errs.SubtypeStorage, "keychain access denied"))

	_, err := GetValidAccessToken(http.DefaultClient, UATCallOptions{
		UserOpenId: "ou_user",
		AppId:      "cli_test",
		Domain:     core.BrandFeishu,
		ErrOut:     &bytes.Buffer{},
	})
	if err == nil {
		t.Fatal("AC4: unreadable credential store should return an error")
	}
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Subtype != errs.SubtypeStorage {
		t.Fatalf("AC4: problem = %#v, want storage instead of token_missing", problem)
	}
}

func TestRefreshPreflightPreventsRotationWhenStorageIsUnwritable(t *testing.T) {
	now := time.Now()
	attempted := refreshTestToken("expired-access", "usable-refresh", now.Add(-time.Minute), now.Add(24*time.Hour))
	store := installUATStoreStub(t, attempted)
	store.failPersistCall(1)
	called := false
	client := &http.Client{Transport: uatRoundTripFunc(func(*http.Request) (*http.Response, error) {
		called = true
		return nil, errors.New("refresh endpoint should not be called")
	})}

	_, err := GetValidAccessToken(client, UATCallOptions{
		UserOpenId: "ou_user",
		AppId:      "cli_test",
		AppSecret:  "secret",
		Domain:     core.BrandFeishu,
		ErrOut:     &bytes.Buffer{},
	})
	if err == nil {
		t.Fatal("AC3: unwritable storage should stop refresh before remote rotation")
	}
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Subtype != errs.SubtypeStorage {
		t.Fatalf("AC3: problem = %#v, want storage", problem)
	}
	if called {
		t.Fatal("AC3: remote refresh was called despite failed storage preflight")
	}
	stored, _ := store.snapshot()
	if stored == nil || stored.RefreshToken != attempted.RefreshToken {
		t.Fatalf("AC3: preflight failure changed token state: %#v", stored)
	}
}

func TestRefreshRetriesRotatedTokenPersistence(t *testing.T) {
	now := time.Now()
	attempted := refreshTestToken("expired-access", "old-refresh", now.Add(-time.Minute), now.Add(24*time.Hour))
	store := installUATStoreStub(t, attempted)
	store.failPersistCall(2)
	registry := successfulRefreshRegistry("rotated-access", "rotated-refresh", nil)
	t.Cleanup(func() { registry.Verify(t) })

	got, err := GetValidAccessToken(httpmock.NewClient(registry), UATCallOptions{
		UserOpenId: "ou_user",
		AppId:      "cli_test",
		AppSecret:  "secret",
		Domain:     core.BrandFeishu,
		ErrOut:     &bytes.Buffer{},
	})
	if err != nil {
		t.Fatalf("AC3: one transient persistence failure should recover: %v", err)
	}
	if got != "rotated-access" {
		t.Fatalf("AC3: access token = %q, want rotated token", got)
	}
	stored, persistCalls := store.snapshot()
	if persistCalls != 3 {
		t.Fatalf("AC3: persist calls = %d, want preflight + failed write + retry", persistCalls)
	}
	if stored == nil || stored.RefreshToken != "rotated-refresh" {
		t.Fatalf("AC3: rotated refresh token was not recovered: %#v", stored)
	}
}

func TestLoginWaitsForInFlightRefreshAndWins(t *testing.T) {
	now := time.Now()
	attempted := refreshTestToken("expired-access", "old-refresh", now.Add(-time.Minute), now.Add(24*time.Hour))
	store := installUATStoreStub(t, attempted)
	requestStarted := make(chan struct{})
	releaseRefresh := make(chan struct{})
	registry := successfulRefreshRegistry("rotated-access", "rotated-refresh", func(*http.Request) {
		close(requestStarted)
		<-releaseRefresh
	})
	t.Cleanup(func() { registry.Verify(t) })
	refreshDone := make(chan error, 1)
	go func() {
		_, err := GetValidAccessToken(httpmock.NewClient(registry), UATCallOptions{
			UserOpenId: "ou_user", AppId: "cli_test", AppSecret: "secret", Domain: core.BrandFeishu, ErrOut: &bytes.Buffer{},
		})
		refreshDone <- err
	}()
	<-requestStarted

	login := refreshTestToken("login-access", "login-refresh", now.Add(time.Hour), now.Add(7*24*time.Hour))
	login.GrantedAt = now.UnixMilli()
	loginDone := make(chan error, 1)
	go func() { loginDone <- SetStoredToken(login) }()
	select {
	case err := <-loginDone:
		t.Fatalf("AC2: login bypassed in-flight refresh lock: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	close(releaseRefresh)
	if err := <-refreshDone; err != nil {
		t.Fatalf("AC2: refresh failed: %v", err)
	}
	if err := <-loginDone; err != nil {
		t.Fatalf("AC2: login persistence failed: %v", err)
	}
	stored, _ := store.snapshot()
	if stored == nil || stored.RefreshToken != login.RefreshToken || stored.GrantedAt != login.GrantedAt {
		t.Fatalf("AC2: stale refresh overwrote newer login: %#v", stored)
	}
}

func TestLogoutWaitsForInFlightRefreshAndWins(t *testing.T) {
	now := time.Now()
	attempted := refreshTestToken("expired-access", "old-refresh", now.Add(-time.Minute), now.Add(24*time.Hour))
	store := installUATStoreStub(t, attempted)
	requestStarted := make(chan struct{})
	releaseRefresh := make(chan struct{})
	registry := successfulRefreshRegistry("rotated-access", "rotated-refresh", func(*http.Request) {
		close(requestStarted)
		<-releaseRefresh
	})
	registry.Register(&httpmock.Stub{
		Method: "POST",
		URL:    PathOAuthRevoke,
		Body:   map[string]interface{}{"code": 0},
	})
	t.Cleanup(func() { registry.Verify(t) })
	refreshDone := make(chan error, 1)
	go func() {
		_, err := GetValidAccessToken(httpmock.NewClient(registry), UATCallOptions{
			UserOpenId: "ou_user", AppId: "cli_test", AppSecret: "secret", Domain: core.BrandFeishu, ErrOut: &bytes.Buffer{},
		})
		refreshDone <- err
	}()
	<-requestStarted

	logoutDone := make(chan error, 1)
	go func() {
		logoutDone <- RevokeAndRemoveStoredToken(httpmock.NewClient(registry), "cli_test", "secret", core.BrandFeishu, "ou_user")
	}()
	select {
	case err := <-logoutDone:
		t.Fatalf("AC2: logout bypassed in-flight refresh lock: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	close(releaseRefresh)
	if err := <-refreshDone; err != nil {
		t.Fatalf("AC2: refresh failed: %v", err)
	}
	if err := <-logoutDone; err != nil {
		t.Fatalf("AC2: logout failed: %v", err)
	}
	stored, _ := store.snapshot()
	if stored != nil {
		t.Fatalf("AC2: refresh recreated token after logout: %#v", stored)
	}
}

func successfulRefreshRegistry(accessToken, refreshToken string, onMatch func(*http.Request)) *httpmock.Registry {
	registry := &httpmock.Registry{}
	registry.Register(&httpmock.Stub{
		Method:  "POST",
		URL:     "/open-apis/authen/v2/oauth/token",
		OnMatch: onMatch,
		Body: map[string]interface{}{
			"code":                     0,
			"access_token":             accessToken,
			"refresh_token":            refreshToken,
			"expires_in":               3600,
			"refresh_token_expires_in": 604800,
		},
	})
	return registry
}
