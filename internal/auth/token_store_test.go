// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/keychain"
	"github.com/zalando/go-keyring"
)

func setupStoredTokenTest(t *testing.T) {
	t.Helper()
	root := t.TempDir()
	t.Setenv("HOME", filepath.Join(root, "home"))
	t.Setenv("LARKSUITE_CLI_DATA_DIR", filepath.Join(root, "data"))
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", filepath.Join(root, "config"))
	keyring.MockInit()
}

func mustGetStoredToken(t *testing.T, appID, userOpenID string) *StoredUAToken {
	t.Helper()
	token, err := GetStoredToken(appID, userOpenID)
	if err != nil {
		t.Fatalf("GetStoredToken() error = %v", err)
	}
	return token
}

func requireStoredTokenCorrupt(t *testing.T, err error, sensitiveValues ...string) {
	t.Helper()
	var storageErr *errs.InternalError
	if !errors.As(err, &storageErr) || storageErr.Subtype != errs.SubtypeStorage {
		t.Fatalf("error = %T %v, want internal/storage", err, err)
	}
	if !errors.Is(err, errStoredTokenCorrupt) {
		t.Fatalf("error = %v, want corrupt-token cause", err)
	}
	for _, sensitive := range sensitiveValues {
		if sensitive != "" && strings.Contains(err.Error(), sensitive) {
			t.Fatalf("error leaked stored token content %q: %v", sensitive, err)
		}
	}
}

func TestGetStoredTokenDistinguishesMissingFromCorrupt(t *testing.T) {
	setupStoredTokenTest(t)

	stored, err := GetStoredToken("cli_missing", "ou_missing")
	if err != nil || stored != nil {
		t.Fatalf("missing token = (%#v, %v), want (nil, nil)", stored, err)
	}

	const sensitive = "sensitive-access-token"
	if err := keychain.Set(keychain.LarkCliService, accountKey("cli_corrupt", "ou_corrupt"),
		`{"accessToken":"`+sensitive+`"`); err != nil {
		t.Fatalf("keychain.Set() error = %v", err)
	}
	stored, err = GetStoredToken("cli_corrupt", "ou_corrupt")
	if stored != nil || err == nil {
		t.Fatalf("corrupt token = (%#v, %v), want (nil, error)", stored, err)
	}
	requireStoredTokenCorrupt(t, err, sensitive)
}

func TestGetStoredTokenRejectsInvalidAccountIdentity(t *testing.T) {
	setupStoredTokenTest(t)

	tests := []struct {
		name            string
		appID           string
		userOpenID      string
		record          string
		sensitiveValues []string
	}{
		{
			name:            "missing app ID",
			appID:           "cli_missing_app",
			userOpenID:      "ou_missing_app",
			record:          `{"userOpenId":"ou_missing_app","accessToken":"access-missing-app"}`,
			sensitiveValues: []string{"access-missing-app"},
		},
		{
			name:            "missing user open ID",
			appID:           "cli_missing_user",
			userOpenID:      "ou_missing_user",
			record:          `{"appId":"cli_missing_user","refreshToken":"refresh-missing-user"}`,
			sensitiveValues: []string{"refresh-missing-user"},
		},
		{
			name:       "mismatched app ID",
			appID:      "cli_expected_app",
			userOpenID: "ou_expected_app",
			record: `{"appId":"cli_stored_app","userOpenId":"ou_expected_app",` +
				`"accessToken":"access-mismatched-app"}`,
			sensitiveValues: []string{"cli_stored_app", "access-mismatched-app"},
		},
		{
			name:       "mismatched user open ID",
			appID:      "cli_expected_user",
			userOpenID: "ou_expected_user",
			record: `{"appId":"cli_expected_user","userOpenId":"ou_stored_user",` +
				`"refreshToken":"refresh-mismatched-user"}`,
			sensitiveValues: []string{"ou_stored_user", "refresh-mismatched-user"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := keychain.Set(keychain.LarkCliService, accountKey(tt.appID, tt.userOpenID),
				tt.record); err != nil {
				t.Fatalf("keychain.Set() error = %v", err)
			}
			stored, err := GetStoredToken(tt.appID, tt.userOpenID)
			if stored != nil || err == nil {
				t.Fatalf("invalid identity token = (%#v, %v), want (nil, error)", stored, err)
			}
			requireStoredTokenCorrupt(t, err, tt.sensitiveValues...)
		})
	}
}

func TestStoredTokenGenerationGuard(t *testing.T) {
	setupStoredTokenTest(t)
	now := time.Now()
	generation0 := &StoredUAToken{
		AppId:            "cli_generation_guard",
		UserOpenId:       "ou_generation_guard",
		AccessToken:      "access-g0",
		RefreshToken:     "refresh-g0",
		ExpiresAt:        now.Add(-time.Minute).UnixMilli(),
		RefreshExpiresAt: now.Add(time.Hour).UnixMilli(),
	}
	generation1 := *generation0
	generation1.AccessToken = "access-g1"
	generation1.RefreshToken = "refresh-g1"
	generation2 := generation1
	generation2.AccessToken = "access-g2"
	generation2.RefreshToken = "refresh-g2"

	if err := SetStoredToken(generation0); err != nil {
		t.Fatalf("SetStoredToken(generation 0) error = %v", err)
	}
	withLock := func(fn func() error) {
		t.Helper()
		if err := withTokenStorageLock(generation0.AppId, generation0.UserOpenId, fn); err != nil {
			t.Fatalf("withTokenStorageLock() error = %v", err)
		}
	}

	withLock(func() error {
		current, swapped, err := compareAndSwapStoredToken(
			generation0.AppId, generation0.UserOpenId, generation0, &generation1,
		)
		if err == nil && (!swapped || current == nil || current.RefreshToken != generation1.RefreshToken) {
			t.Fatalf("matching swap = (%#v, %v), want generation 1 stored", current, swapped)
		}
		return err
	})
	if err := SetStoredToken(&generation2); err != nil {
		t.Fatalf("SetStoredToken(generation 2) error = %v", err)
	}

	withLock(func() error {
		current, swapped, err := compareAndSwapStoredToken(
			generation0.AppId, generation0.UserOpenId, &generation1, generation0,
		)
		if err == nil && (swapped || current == nil || current.RefreshToken != generation2.RefreshToken) {
			t.Fatalf("stale swap = (%#v, %v), want generation 2 retained", current, swapped)
		}
		return err
	})
	withLock(func() error {
		current, deleted, err := compareAndDeleteStoredToken(
			generation0.AppId, generation0.UserOpenId, &generation1,
		)
		if err == nil && (deleted || current == nil || current.RefreshToken != generation2.RefreshToken) {
			t.Fatalf("stale delete = (%#v, %v), want generation 2 retained", current, deleted)
		}
		return err
	})
	withLock(func() error {
		current, deleted, err := compareAndDeleteStoredToken(
			generation0.AppId, generation0.UserOpenId, &generation2,
		)
		if err == nil && (!deleted || current != nil) {
			t.Fatalf("matching delete = (%#v, %v), want token removed", current, deleted)
		}
		return err
	})
	if current := mustGetStoredToken(t, generation0.AppId, generation0.UserOpenId); current != nil {
		t.Fatalf("stored token = %#v, want removed", current)
	}
}
