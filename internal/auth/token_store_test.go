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

func mustGetStoredToken(t testing.TB, appID, userOpenID string) *StoredUAToken {
	t.Helper()
	stored, err := GetStoredToken(appID, userOpenID)
	if err != nil {
		t.Fatalf("GetStoredToken() error = %v", err)
	}
	return stored
}

func TestGetStoredTokenDistinguishesMissingFromCorrupt(t *testing.T) {
	setupStoredTokenTest(t)

	const (
		appID      = "cli_corrupt"
		userOpenID = "ou_corrupt"
		secret     = "sensitive-access-token"
	)

	stored, err := GetStoredToken(appID, userOpenID)
	if err != nil || stored != nil {
		t.Fatalf("missing token = (%#v, %v), want (nil, nil)", stored, err)
	}

	malformed := `{"accessToken":"` + secret + `",`
	if writeErr := keychain.Set(keychain.LarkCliService, accountKey(appID, userOpenID), malformed); writeErr != nil {
		t.Fatalf("keychain.Set() error = %v", writeErr)
	}

	stored, err = GetStoredToken(appID, userOpenID)
	if stored != nil {
		t.Fatalf("corrupt token = %#v, want nil", stored)
	}
	var storageErr *errs.InternalError
	if !errors.As(err, &storageErr) || storageErr.Subtype != errs.SubtypeStorage {
		t.Fatalf("corrupt token error = %T (%v), want internal/storage", err, err)
	}
	if !errors.Is(err, errStoredTokenCorrupt) {
		t.Fatalf("corrupt token error = %v, want corruption sentinel in cause chain", err)
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("corrupt token error leaked credential content: %v", err)
	}
}

func TestGetStoredTokenRejectsSemanticallyCorruptJSON(t *testing.T) {
	setupStoredTokenTest(t)

	const (
		appID      = "cli_semantic_corrupt"
		userOpenID = "ou_semantic_corrupt"
		secret     = "sensitive-semantic-token"
	)
	account := accountKey(appID, userOpenID)
	cases := []struct {
		name string
		data string
	}{
		{name: "empty object", data: `{}`},
		{name: "wrong account binding", data: `{"appId":"other-app","userOpenId":"other-user","accessToken":"` + secret + `"}`},
		{name: "missing access token", data: `{"appId":"` + appID + `","userOpenId":"` + userOpenID + `","expiresAt":4102444800000}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := keychain.Set(keychain.LarkCliService, account, tc.data); err != nil {
				t.Fatalf("keychain.Set() error = %v", err)
			}
			stored, err := GetStoredToken(appID, userOpenID)
			if stored != nil {
				t.Fatalf("GetStoredToken() token = %#v, want nil", stored)
			}
			var storageErr *errs.InternalError
			if !errors.As(err, &storageErr) || storageErr.Subtype != errs.SubtypeStorage {
				t.Fatalf("GetStoredToken() error = %T (%v), want internal/storage", err, err)
			}
			if !errors.Is(err, errStoredTokenCorrupt) {
				t.Fatalf("GetStoredToken() error = %v, want corruption sentinel", err)
			}
			if strings.Contains(err.Error(), secret) || strings.Contains(err.Error(), tc.data) {
				t.Fatalf("GetStoredToken() error leaked stored data: %v", err)
			}
		})
	}
}

func TestSetStoredTokenRejectsSemanticCorruption(t *testing.T) {
	setupStoredTokenTest(t)

	token := &StoredUAToken{AppId: "cli_invalid_write", UserOpenId: "ou_invalid_write"}
	err := SetStoredToken(token)
	var storageErr *errs.InternalError
	if !errors.As(err, &storageErr) || storageErr.Subtype != errs.SubtypeStorage {
		t.Fatalf("SetStoredToken() error = %T (%v), want internal/storage", err, err)
	}
	if !errors.Is(err, errStoredTokenCorrupt) {
		t.Fatalf("SetStoredToken() error = %v, want corruption sentinel", err)
	}
	stored, readErr := keychain.Get(keychain.LarkCliService, accountKey(token.AppId, token.UserOpenId))
	if readErr != nil || stored != "" {
		t.Fatalf("invalid write persisted data = (%q, %v), want empty", stored, readErr)
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
