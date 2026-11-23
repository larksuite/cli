// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/dpop"
	"github.com/larksuite/cli/internal/keychain"
	"github.com/larksuite/cli/internal/keysigner"
	"github.com/larksuite/cli/internal/vfs"
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
	legacy := newRefreshTestToken()
	legacy.AppId, legacy.UserOpenId = appID, userOpenID
	seedLegacyToken(t, legacy)
	if writeErr := keychain.Set(keychain.LarkCliService, accountKeyV2(appID, userOpenID), malformed); writeErr != nil {
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
	stored, readErr := keychain.Get(keychain.LarkCliService, accountKeyV2(token.AppId, token.UserOpenId))
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

	legacy := *generation0
	legacy.AccessToken, legacy.RefreshToken = "legacy-access", "legacy-refresh"
	legacy.TokenType = StoredTokenTypeBearer
	seedLegacyToken(t, &legacy)
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
	if got, err := readStoredTokenAt(legacy.AppId, legacy.UserOpenId, accountKey(legacy.AppId, legacy.UserOpenId)); err != nil || got == nil || *got != legacy {
		t.Fatalf("v2 refresh changed the legacy entry: %v", err)
	}
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

func TestStoredTokenRecoversFromCorruptOrUnreadableEntry(t *testing.T) {
	setupStoredTokenTest(t)

	const (
		appID      = "cli_replace"
		userOpenID = "ou_replace"
	)
	account := accountKeyV2(appID, userOpenID)
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
	if runtime.GOOS != "windows" { // Windows credentials use HKCU, not FileIO.
		seedLegacyToken(t, fresh)
		cause := errors.New("credential decryption unavailable")
		failRead := true
		useAuthFSStub(t, authFSStub{readFile: func(path string) ([]byte, error) {
			if failRead && filepath.Base(path) == sanitizeID(account)+".enc" {
				return nil, cause
			}
			return (vfs.OsFs{}).ReadFile(path)
		}})
		if _, err := GetStoredToken(appID, userOpenID); !errors.Is(err, cause) {
			t.Fatalf("unreadable v2 fell back to legacy: %v", err)
		}
		if err := RemoveStoredToken(appID, userOpenID); err != nil {
			t.Fatalf("remove unreadable token: %v", err)
		}
		failRead = false
		if current := mustGetStoredToken(t, appID, userOpenID); current != nil {
			t.Fatalf("unreadable token was not removed: %#v", current)
		}
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
		ClockSyncedAtMs:      time.Now().UnixMilli(),
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

func seedLegacyToken(t *testing.T, token *StoredUAToken) {
	t.Helper()
	data, err := json.Marshal(token)
	if err != nil {
		t.Fatal(err)
	}
	if err := keychain.Set(keychain.LarkCliService, accountKey(token.AppId, token.UserOpenId), string(data)); err != nil {
		t.Fatal(err)
	}
}

func newPersistedDPoPToken(t *testing.T) (*dpop.KeyStore, *StoredUAToken) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("software unlock credentials use shared HKCU on Windows")
	}
	// Match the production software layout so cleanup is real, without native signers.
	directory := filepath.Join(os.Getenv("LARKSUITE_CLI_DATA_DIR"), keychain.LarkCliService)
	if runtime.GOOS == "darwin" {
		directory = filepath.Join(os.Getenv("HOME"), "Library", "Application Support", keychain.LarkCliService)
	}
	secret := make([]byte, 32)
	if err := keychain.Set(keychain.LarkCliService, "dpop:software:unlock:v1", base64.StdEncoding.EncodeToString(secret)); err != nil {
		t.Fatal(err)
	}
	signer, err := keysigner.NewSoftwareSigner(filepath.Join(directory, "keysigner", keysigner.SoftwareSignerName),
		func(context.Context) ([]byte, error) { return append([]byte(nil), secret...), nil })
	if err != nil {
		t.Fatal(err)
	}
	store := dpop.NewKeyStoreWithSigner(nil, signer)
	key, err := store.EnsureContext(context.Background(), "stored-user-key")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveContext(context.Background(), key); err != nil {
		t.Fatal(err)
	}
	jkt, err := key.Thumbprint()
	if err != nil {
		t.Fatal(err)
	}
	token := newRefreshTestToken()
	token.ExpiresAt = time.Now().Add(time.Hour).UnixMilli()
	token.TokenType = StoredTokenTypeDPoP
	token.DPoPKeyID, token.DPoPJKT = key.ID(), jkt
	token.DPoPKeySecurityLevel = string(key.SecurityLevel())
	return store, token
}

func TestStoredTokenCompatibility(t *testing.T) {
	for _, tokenType := range []string{StoredTokenTypeBearer, StoredTokenTypeDPoP} {
		t.Run(tokenType, func(t *testing.T) {
			setupStoredTokenTest(t)
			token := newRefreshTestToken()
			if tokenType == StoredTokenTypeDPoP {
				_, token = newPersistedDPoPToken(t)
			}
			seedLegacyToken(t, token)
			legacy, err := keychain.Get(keychain.LarkCliService, accountKey(token.AppId, token.UserOpenId))
			if err != nil {
				t.Fatal(err)
			}
			token.TokenType = tokenType
			v2 := accountKeyV2(token.AppId, token.UserOpenId)
			for _, raw := range []string{"", "null"} {
				if raw != "" {
					if err := keychain.Set(keychain.LarkCliService, v2, raw); err != nil {
						t.Fatal(err)
					}
				}
				got, err := GetStoredToken(token.AppId, token.UserOpenId)
				if err != nil || got == nil || *got != *token {
					t.Fatalf("legacy fallback = (%+v, %v)", got, err)
				}
				if after, err := keychain.Get(keychain.LarkCliService, v2); err != nil || after != raw {
					t.Fatalf("reading changed v2: %v", err)
				}
			}
			token.AccessToken, token.RefreshToken = "v2-access", "v2-refresh"
			if err := SetStoredToken(token); err != nil {
				t.Fatal(err)
			}
			if got := mustGetStoredToken(t, token.AppId, token.UserOpenId); got == nil || *got != *token {
				t.Fatal("new v2 login did not take precedence")
			}
			if got, err := keychain.Get(keychain.LarkCliService, accountKey(token.AppId, token.UserOpenId)); err != nil || got != legacy {
				t.Fatalf("new login changed the legacy entry: %v", err)
			}
			if err := RemoveStoredToken(token.AppId, token.UserOpenId); err != nil {
				t.Fatal(err)
			}
			for _, account := range []string{accountKey(token.AppId, token.UserOpenId), v2} {
				if raw, err := keychain.Get(keychain.LarkCliService, account); err != nil || raw != "" {
					t.Fatalf("logout retained a credential or marker: %v", err)
				}
			}
		})
	}
}

func TestStoredDPoPTokenKeyCleanup(t *testing.T) {
	for _, action := range []string{"replace", "remove-failure"} {
		t.Run(action, func(t *testing.T) {
			setupStoredTokenTest(t)
			store, legacy := newPersistedDPoPToken(t)
			seedLegacyToken(t, legacy)
			token := *legacy
			token.AccessToken, token.RefreshToken = "v2-access", "v2-refresh"
			if action == "replace" {
				key, err := store.EnsureContext(context.Background(), "new-login-key")
				if err != nil {
					t.Fatal(err)
				}
				if err := store.SaveContext(context.Background(), key); err != nil {
					t.Fatal(err)
				}
				token.DPoPKeyID = key.ID()
				token.DPoPJKT, err = key.Thumbprint()
				if err != nil {
					t.Fatal(err)
				}
			}
			if err := SetStoredToken(&token); err != nil {
				t.Fatal(err)
			}
			if action == "replace" {
				if err := SetStoredToken(newRefreshTestToken()); err != nil {
					t.Fatal(err)
				}
				if _, err := store.LoadContext(context.Background(), token.DPoPKeyID); !errors.Is(err, dpop.ErrKeyNotFound) {
					t.Fatalf("replaced v2 key was not removed: %v", err)
				}
			} else {
				cause := errors.New("v2 removal interrupted")
				failRemove := true
				useAuthFSStub(t, authFSStub{remove: func(path string) error {
					if failRemove && filepath.Base(path) == sanitizeID(accountKeyV2(token.AppId, token.UserOpenId))+".enc" {
						return cause
					}
					return (vfs.OsFs{}).Remove(path)
				}})
				err := RemoveStoredToken(token.AppId, token.UserOpenId)
				requireRefreshProblem(t, err, errs.CategoryInternal, errs.SubtypeStorage, false)
				if !errors.Is(err, cause) {
					t.Fatalf("logout lost removal error: %v", err)
				}
				if got := mustGetStoredToken(t, token.AppId, token.UserOpenId); got == nil || *got != token {
					t.Fatal("failed removal changed the retained v2 credential")
				}
				if raw, err := keychain.Get(keychain.LarkCliService, accountKey(token.AppId, token.UserOpenId)); err != nil || raw != "" {
					t.Fatalf("partial logout retained the fallback: %v", err)
				}
				failRemove = false
			}
			if _, err := store.LoadContext(context.Background(), legacy.DPoPKeyID); err != nil {
				t.Fatalf("removed a key still referenced by a token: %v", err)
			}
			if err := RemoveStoredToken(token.AppId, token.UserOpenId); err != nil {
				t.Fatal(err)
			}
			if _, err := store.LoadContext(context.Background(), legacy.DPoPKeyID); !errors.Is(err, dpop.ErrKeyNotFound) {
				t.Fatalf("unreferenced key was not removed: %v", err)
			}
		})
	}
}
