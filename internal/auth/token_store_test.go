// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"context"
	"errors"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/dpop"
	"github.com/larksuite/cli/internal/keychain"
	"github.com/larksuite/cli/internal/keysigner"
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

func newAuthDPoPStore(t *testing.T) (*dpop.KeyStore, *dpop.Key) {
	t.Helper()
	signer := newAuthDPoPTestSigner()
	store := dpop.NewKeyStoreWithSigner(deviceFlowMetadata{}, signer)
	key, err := store.EnsureContext(context.Background(), "stored-user-key")
	if err != nil {
		t.Fatal(err)
	}
	return store, key
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
	if !strings.Contains(storageErr.Hint, "auth login") {
		t.Fatalf("corrupt token hint = %q, want re-authorization guidance", storageErr.Hint)
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
			if !strings.Contains(storageErr.Hint, "auth login") {
				t.Fatalf("GetStoredToken() hint = %q, want re-authorization guidance", storageErr.Hint)
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
	if storageErr.Hint != "" {
		t.Fatalf("SetStoredToken() hint = %q, want no re-authorization guidance on the write path", storageErr.Hint)
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

func TestSetStoredTokenReplacesCorruptEntry(t *testing.T) {
	setupStoredTokenTest(t)

	const (
		appID      = "cli_replace"
		userOpenID = "ou_replace"
	)
	account := accountKey(appID, userOpenID)
	if err := keychain.Set(keychain.LarkCliService, account, `{"accessToken":`); err != nil {
		t.Fatalf("keychain.Set() error = %v", err)
	}
	if _, err := GetStoredToken(appID, userOpenID); !errors.Is(err, errStoredTokenCorrupt) {
		t.Fatalf("precondition GetStoredToken() error = %v, want corruption", err)
	}

	now := time.Now()
	fresh := &StoredUAToken{
		AppId:            appID,
		UserOpenId:       userOpenID,
		AccessToken:      "fresh-access",
		RefreshToken:     "fresh-refresh",
		ExpiresAt:        now.Add(time.Hour).UnixMilli(),
		RefreshExpiresAt: now.Add(24 * time.Hour).UnixMilli(),
		GrantedAt:        now.UnixMilli(),
	}
	if err := SetStoredToken(fresh); err != nil {
		t.Fatalf("SetStoredToken() over corrupt entry error = %v; the recovery hint promises a new login replaces it", err)
	}
	got := mustGetStoredToken(t, appID, userOpenID)
	if got == nil || got.AccessToken != "fresh-access" {
		t.Fatalf("stored token after re-login = %#v, want fresh token", got)
	}
}

func TestStoredDPoPTokenRestoresExactBindingAndFailsClosed(t *testing.T) {
	setupStoredTokenTest(t)
	store, key := newAuthDPoPStore(t)
	key.Clock().RestoreState(dpop.ClockState{OffsetMillis: 90_000, SyncedAtMillis: 1_700_000_000_000})
	if err := store.SaveContext(context.Background(), key); err != nil {
		t.Fatal(err)
	}
	jkt, err := key.Thumbprint()
	if err != nil {
		t.Fatal(err)
	}
	token := &StoredUAToken{
		AppId:                "cli-dpop",
		UserOpenId:           "ou-dpop",
		AccessToken:          "access-dpop",
		RefreshToken:         "refresh-dpop",
		ExpiresAt:            time.Now().Add(time.Hour).UnixMilli(),
		RefreshExpiresAt:     time.Now().Add(24 * time.Hour).UnixMilli(),
		TokenType:            StoredTokenTypeDPoP,
		DPoPKeyID:            key.ID(),
		DPoPJKT:              jkt,
		DPoPKeySecurityLevel: string(key.SecurityLevel()),
		ClockOffsetMs:        -120_000,
		ClockSyncedAtMs:      1_700_000_100_000,
	}
	if err := SetStoredToken(token); err != nil {
		t.Fatal(err)
	}
	result, err := GetValidAccessToken(context.Background(), http.DefaultClient, UATCallOptions{
		AppId: token.AppId, UserOpenId: token.UserOpenId, DPoPMode: core.DPoPModeRequired, DPoPKeyStore: store,
	})
	if err != nil || result == nil || result.AccessToken != token.AccessToken || result.DPoP == nil ||
		result.DPoP.Key().Clock().State() != (dpop.ClockState{OffsetMillis: token.ClockOffsetMs, SyncedAtMillis: token.ClockSyncedAtMs}) {
		t.Fatalf("restored token = (%+v, %v)", result, err)
	}

	for _, tc := range []struct {
		name    string
		mutate  func(*StoredUAToken)
		subtype errs.Subtype
	}{
		{name: "bearer forbidden", mutate: func(t *StoredUAToken) {
			t.TokenType, t.DPoPKeyID, t.DPoPJKT = StoredTokenTypeBearer, "", ""
		}, subtype: errs.SubtypeDPoPRequired},
		{name: "missing key id", mutate: func(t *StoredUAToken) { t.DPoPKeyID = "" }, subtype: errs.SubtypeDPoPKeyMissing},
		{name: "wrong thumbprint", mutate: func(t *StoredUAToken) { t.DPoPJKT = "wrong" }, subtype: errs.SubtypeDPoPBindingMismatch},
		{name: "wrong protection level", mutate: func(t *StoredUAToken) {
			t.DPoPKeySecurityLevel = string(keysigner.SecurityLevelL1)
		}, subtype: errs.SubtypeDPoPBindingMismatch},
	} {
		t.Run(tc.name, func(t *testing.T) {
			changed := *token
			tc.mutate(&changed)
			result, err := accessTokenResultFromStored(context.Background(), &changed, UATCallOptions{
				DPoPMode: core.DPoPModeRequired, DPoPKeyStore: store,
			})
			problem, ok := errs.ProblemOf(err)
			if result != nil || !ok || problem.Subtype != tc.subtype || problem.Hint == "" {
				t.Fatalf("accessTokenResultFromStored() = (%+v, %v), want %s", result, err, tc.subtype)
			}
		})
	}
}
