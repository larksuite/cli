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

func withStubbedUATStore(t *testing.T, initial *StoredUAToken) (*StoredUAToken, *int) {
	t.Helper()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	origGet := getStoredUAToken
	origSet := setStoredUAToken
	origRemove := removeStoredUAToken

	stored := initial
	removeCalls := 0
	getStoredUAToken = func(appID, userOpenID string) *StoredUAToken {
		if stored == nil || stored.AppId != appID || stored.UserOpenId != userOpenID {
			return nil
		}
		copy := *stored
		return &copy
	}
	setStoredUAToken = func(token *StoredUAToken) error {
		copy := *token
		stored = &copy
		return nil
	}
	removeStoredUAToken = func(appID, userOpenID string) error {
		removeCalls++
		if stored != nil && stored.AppId == appID && stored.UserOpenId == userOpenID {
			stored = nil
		}
		return nil
	}
	t.Cleanup(func() {
		getStoredUAToken = origGet
		setStoredUAToken = origSet
		removeStoredUAToken = origRemove
	})

	return stored, &removeCalls
}

func TestGetValidAccessToken_PreservesUnexpiredRefreshTokenOnRefreshReused(t *testing.T) {
	now := time.Now()
	initial := &StoredUAToken{
		UserOpenId:       "ou_user",
		AppId:            "cli_test",
		AccessToken:      "expired-access-token",
		RefreshToken:     "refresh-token",
		ExpiresAt:        now.Add(-time.Minute).UnixMilli(),
		RefreshExpiresAt: now.Add(72 * time.Hour).UnixMilli(),
		Scope:            "offline_access",
		GrantedAt:        now.Add(-24 * time.Hour).UnixMilli(),
	}
	_, removeCalls := withStubbedUATStore(t, initial)

	reg := &httpmock.Registry{}
	t.Cleanup(func() { reg.Verify(t) })
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/authen/v2/oauth/token",
		Body: map[string]interface{}{
			"code": 20073,
			"msg":  "refresh token has already been used",
		},
	})

	_, err := GetValidAccessToken(httpmock.NewClient(reg), UATCallOptions{
		UserOpenId: "ou_user",
		AppId:      "cli_test",
		AppSecret:  "secret",
		Domain:     core.BrandFeishu,
		ErrOut:     &bytes.Buffer{},
	})
	if err == nil {
		t.Fatal("expected refresh error")
	}
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed error, got %T: %v", err, err)
	}
	if p.Category != errs.CategoryAuthentication || p.Subtype != errs.SubtypeRefreshTokenReused {
		t.Fatalf("problem = %#v, want authentication/refresh_token_reused", p)
	}
	if p.Code != 20073 {
		t.Fatalf("problem code = %d, want 20073", p.Code)
	}
	if p.Hint == "" {
		t.Fatal("expected non-empty hint for preserved refresh state")
	}
	if *removeCalls != 0 {
		t.Fatalf("RemoveStoredToken calls = %d, want 0", *removeCalls)
	}
	got := getStoredUAToken("cli_test", "ou_user")
	if got == nil {
		t.Fatal("stored token was removed; want preserved for retry/diagnosis")
	}
	if got.RefreshToken != initial.RefreshToken || got.RefreshExpiresAt != initial.RefreshExpiresAt {
		t.Fatalf("stored refresh state changed: got token=%q expires=%d, want token=%q expires=%d",
			got.RefreshToken, got.RefreshExpiresAt, initial.RefreshToken, initial.RefreshExpiresAt)
	}
}

func TestGetValidAccessToken_ClearsLocallyExpiredRefreshToken(t *testing.T) {
	now := time.Now()
	initial := &StoredUAToken{
		UserOpenId:       "ou_user",
		AppId:            "cli_test",
		AccessToken:      "expired-access-token",
		RefreshToken:     "refresh-token",
		ExpiresAt:        now.Add(-2 * time.Hour).UnixMilli(),
		RefreshExpiresAt: now.Add(-time.Minute).UnixMilli(),
	}
	_, removeCalls := withStubbedUATStore(t, initial)

	_, err := GetValidAccessToken(&http.Client{}, UATCallOptions{
		UserOpenId: "ou_user",
		AppId:      "cli_test",
		AppSecret:  "secret",
		Domain:     core.BrandFeishu,
		ErrOut:     &bytes.Buffer{},
	})
	if err == nil {
		t.Fatal("expected missing authorization error")
	}
	var needAuth *NeedAuthorizationError
	if !errors.As(err, &needAuth) {
		t.Fatalf("expected need authorization cause, got %T: %v", err, err)
	}
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed error, got %T: %v", err, err)
	}
	if p.Category != errs.CategoryAuthentication || p.Subtype != errs.SubtypeTokenMissing {
		t.Fatalf("problem = %#v, want authentication/token_missing", p)
	}
	if *removeCalls != 1 {
		t.Fatalf("RemoveStoredToken calls = %d, want 1", *removeCalls)
	}
}

func TestGetValidAccessToken_WaitingRefreshDoesNotReturnExpiredStoredToken(t *testing.T) {
	now := time.Now()
	initial := &StoredUAToken{
		UserOpenId:       "ou_user",
		AppId:            "cli_test",
		AccessToken:      "expired-access-token",
		RefreshToken:     "refresh-token",
		ExpiresAt:        now.Add(-time.Minute).UnixMilli(),
		RefreshExpiresAt: now.Add(time.Hour).UnixMilli(),
	}
	_, removeCalls := withStubbedUATStore(t, initial)

	key := "cli_test:ou_user"
	ch := make(chan struct{})
	refreshLocks.Store(key, ch)
	t.Cleanup(func() {
		refreshLocks.Delete(key)
	})

	var wg sync.WaitGroup
	wg.Add(1)
	var got string
	var err error
	go func() {
		defer wg.Done()
		got, err = GetValidAccessToken(&http.Client{}, UATCallOptions{
			UserOpenId: "ou_user",
			AppId:      "cli_test",
			AppSecret:  "secret",
			Domain:     core.BrandFeishu,
			ErrOut:     &bytes.Buffer{},
		})
	}()
	close(ch)
	wg.Wait()

	if err == nil {
		t.Fatalf("expected refresh failure, got token %q", got)
	}
	if got != "" {
		t.Fatalf("access token = %q, want empty", got)
	}
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed error, got %T: %v", err, err)
	}
	if p.Category != errs.CategoryAuthentication || p.Subtype != errs.SubtypeRefreshServerError {
		t.Fatalf("problem = %#v, want authentication/refresh_server_error", p)
	}
	if *removeCalls != 0 {
		t.Fatalf("RemoveStoredToken calls = %d, want 0", *removeCalls)
	}
}

func TestGetValidAccessToken_WrapsRetryTransportFailure(t *testing.T) {
	now := time.Now()
	initial := &StoredUAToken{
		UserOpenId:       "ou_user",
		AppId:            "cli_test",
		AccessToken:      "expired-access-token",
		RefreshToken:     "refresh-token",
		ExpiresAt:        now.Add(-time.Minute).UnixMilli(),
		RefreshExpiresAt: now.Add(time.Hour).UnixMilli(),
	}
	_, removeCalls := withStubbedUATStore(t, initial)

	reg := &httpmock.Registry{}
	t.Cleanup(func() { reg.Verify(t) })
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/authen/v2/oauth/token",
		Body: map[string]interface{}{
			"code": 20050,
			"msg":  "refresh endpoint transient error",
		},
	})

	_, err := GetValidAccessToken(httpmock.NewClient(reg), UATCallOptions{
		UserOpenId: "ou_user",
		AppId:      "cli_test",
		AppSecret:  "secret",
		Domain:     core.BrandFeishu,
		ErrOut:     &bytes.Buffer{},
	})
	if err == nil {
		t.Fatal("expected retry transport error")
	}
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed error, got %T: %v", err, err)
	}
	if p.Category != errs.CategoryNetwork || p.Subtype != errs.SubtypeNetworkTransport {
		t.Fatalf("problem = %#v, want network/transport", p)
	}
	if *removeCalls != 0 {
		t.Fatalf("RemoveStoredToken calls = %d, want 0", *removeCalls)
	}
}

func TestGetValidAccessToken_WrapsInitialTransportFailure(t *testing.T) {
	now := time.Now()
	initial := &StoredUAToken{
		UserOpenId:       "ou_user",
		AppId:            "cli_test",
		AccessToken:      "expired-access-token",
		RefreshToken:     "refresh-token",
		ExpiresAt:        now.Add(-time.Minute).UnixMilli(),
		RefreshExpiresAt: now.Add(time.Hour).UnixMilli(),
	}
	_, removeCalls := withStubbedUATStore(t, initial)

	_, err := GetValidAccessToken(httpmock.NewClient(&httpmock.Registry{}), UATCallOptions{
		UserOpenId: "ou_user",
		AppId:      "cli_test",
		AppSecret:  "secret",
		Domain:     core.BrandFeishu,
		ErrOut:     &bytes.Buffer{},
	})
	if err == nil {
		t.Fatal("expected initial transport error")
	}
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed error, got %T: %v", err, err)
	}
	if p.Category != errs.CategoryNetwork || p.Subtype != errs.SubtypeNetworkTransport {
		t.Fatalf("problem = %#v, want network/transport", p)
	}
	if *removeCalls != 0 {
		t.Fatalf("RemoveStoredToken calls = %d, want 0", *removeCalls)
	}
}

func TestBuildRefreshFailureError_OAuthStyleNoCodeIsAuthenticationError(t *testing.T) {
	err := buildRefreshFailureError(map[string]interface{}{
		"error":             "invalid_grant",
		"error_description": "refresh token revoked",
	}, UATCallOptions{
		UserOpenId: "ou_user",
		AppId:      "cli_test",
		Domain:     core.BrandFeishu,
	})
	if err == nil {
		t.Fatal("expected refresh error")
	}
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed error, got %T: %v", err, err)
	}
	if p.Category != errs.CategoryAuthentication || p.Subtype != errs.SubtypeRefreshServerError {
		t.Fatalf("problem = %#v, want authentication/refresh_server_error", p)
	}
	if p.Code != 0 {
		t.Fatalf("problem code = %d, want 0 for no-code OAuth error", p.Code)
	}
	var authErr *errs.AuthenticationError
	if !errors.As(err, &authErr) {
		t.Fatalf("expected AuthenticationError, got %T", err)
	}
	if authErr.UserOpenID != "ou_user" {
		t.Fatalf("user_open_id = %q, want ou_user", authErr.UserOpenID)
	}
}

func TestBuildRefreshFailureError_PreservesAPIResponseMetadata(t *testing.T) {
	err := buildRefreshFailureError(map[string]interface{}{
		"code":   99991400,
		"msg":    "too many requests",
		"log_id": "log-123",
	}, UATCallOptions{
		UserOpenId: "ou_user",
		AppId:      "cli_test",
		Domain:     core.BrandFeishu,
	})
	if err == nil {
		t.Fatal("expected refresh error")
	}
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed error, got %T: %v", err, err)
	}
	if p.Category != errs.CategoryAPI || p.Subtype != errs.SubtypeRateLimit {
		t.Fatalf("problem = %#v, want api/rate_limit", p)
	}
	if p.LogID != "log-123" {
		t.Fatalf("log_id = %q, want log-123", p.LogID)
	}
}
