// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"bytes"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
)

func withStubbedUATStore(t *testing.T, initial *StoredUAToken) (*StoredUAToken, *int) {
	t.Helper()
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
	if *removeCalls != 1 {
		t.Fatalf("RemoveStoredToken calls = %d, want 1", *removeCalls)
	}
}
