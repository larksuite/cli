// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/keychain"
)

const (
	aatKeySuffix       = ":aat"
	maxAATRefreshAhead = 5 * time.Minute
)

var errStoredAATCorrupt = errors.New("stored AAT data is corrupt")

// StoredAAToken is the verified Agent Access Token cache payload. Unlike a
// StoredUAToken it deliberately has no refresh token: an AAT is renewed by
// minting another client_credentials token and validating it with
// Introspection.
type StoredAAToken struct {
	AppID       string `json:"appId"`
	UserOpenID  string `json:"userOpenId"`
	AccessToken string `json:"accessToken"`
	ExpiresAt   int64  `json:"expiresAt"` // Unix ms
	GrantedAt   int64  `json:"grantedAt"` // Unix ms
	// Scope is the space-separated grant reported at mint time. Empty on records
	// written before the field existed.
	Scope string `json:"scope"`
}

// AATAccountKey returns the one-to-one app → Agent Employee keychain key.
func AATAccountKey(appID string) string {
	return appID + aatKeySuffix
}

// GetStoredAAT reads and decodes the verified AAT cache for appID.
func GetStoredAAT(kc keychain.KeychainAccess, appID string) (*StoredAAToken, error) {
	if kc == nil {
		return nil, errs.NewInternalError(errs.SubtypeStorage, "AAT keychain is unavailable")
	}
	value, err := kc.Get(keychain.LarkCliService, AATAccountKey(appID))
	if err != nil {
		if errors.Is(err, keychain.ErrNotFound) {
			return nil, nil
		}
		return nil, errs.NewInternalError(errs.SubtypeStorage,
			"failed to read stored AAT: %v", err).WithCause(err)
	}
	if value == "" {
		return nil, nil
	}
	var token StoredAAToken
	if err := json.Unmarshal([]byte(value), &token); err != nil {
		return nil, errs.NewInternalError(errs.SubtypeStorage,
			"failed to decode stored AAT: %v", err).
			WithCause(errors.Join(errStoredAATCorrupt, err))
	}
	return &token, nil
}

// SetStoredAAT persists a verified AAT under <appId>:aat.
func SetStoredAAT(kc keychain.KeychainAccess, token *StoredAAToken) error {
	if token == nil {
		return errs.NewInternalError(errs.SubtypeStorage, "cannot store a nil AAT")
	}
	if err := validateStoredAAT(token, token.AppID, token.UserOpenID); err != nil {
		return err
	}
	return withTokenStorageLock(token.AppID, "aat", func() error {
		return writeStoredAAT(kc, token.AppID, token)
	})
}

func writeStoredAAT(kc keychain.KeychainAccess, appID string, token *StoredAAToken) error {
	if kc == nil {
		return errs.NewInternalError(errs.SubtypeStorage, "AAT keychain is unavailable")
	}
	if token == nil || token.AppID != appID {
		return errs.NewInternalError(errs.SubtypeStorage,
			"cannot store an AAT for a different app")
	}
	data, err := json.Marshal(token)
	if err != nil {
		return errs.NewInternalError(errs.SubtypeStorage,
			"failed to encode stored AAT: %v", err).WithCause(err)
	}
	if err := kc.Set(keychain.LarkCliService, AATAccountKey(appID), string(data)); err != nil {
		return errs.NewInternalError(errs.SubtypeStorage,
			"failed to store AAT: %v", err).WithCause(err)
	}
	return nil
}

// RemoveStoredAAT removes the CLI-owned AAT cache for appID.
func RemoveStoredAAT(kc keychain.KeychainAccess, appID string) error {
	if kc == nil {
		return errs.NewInternalError(errs.SubtypeStorage, "AAT keychain is unavailable")
	}
	return withTokenStorageLock(appID, "aat", func() error {
		if err := kc.Remove(keychain.LarkCliService, AATAccountKey(appID)); err != nil && !errors.Is(err, keychain.ErrNotFound) {
			return errs.NewInternalError(errs.SubtypeStorage,
				"failed to remove stored AAT: %v", err).WithCause(err)
		}
		return nil
	})
}

type aatCacheState uint8

const (
	aatCacheInvalid aatCacheState = iota
	aatCacheFresh
	aatCacheNeedsRefresh
	aatCacheExpired
)

func storedAATState(token *StoredAAToken, appID, userOpenID string, now time.Time) aatCacheState {
	if token == nil || token.AppID != appID || token.UserOpenID != userOpenID ||
		token.AccessToken == "" || token.GrantedAt <= 0 || token.ExpiresAt <= token.GrantedAt {
		return aatCacheInvalid
	}
	nowMS := now.UnixMilli()
	if nowMS >= token.ExpiresAt {
		return aatCacheExpired
	}
	ttlMS := token.ExpiresAt - token.GrantedAt
	refreshAheadMS := ttlMS / 10
	if refreshAheadMS > maxAATRefreshAhead.Milliseconds() {
		refreshAheadMS = maxAATRefreshAhead.Milliseconds()
	}
	if nowMS >= token.ExpiresAt-refreshAheadMS {
		return aatCacheNeedsRefresh
	}
	return aatCacheFresh
}

func validateStoredAAT(token *StoredAAToken, appID, userOpenID string) error {
	if token == nil || token.AppID != appID || token.UserOpenID != userOpenID ||
		token.AccessToken == "" || token.GrantedAt <= 0 || token.ExpiresAt <= token.GrantedAt {
		return errs.NewInternalError(errs.SubtypeInvalidResponse,
			"refusing to store an invalid AAT cache payload")
	}
	return nil
}

// AAT cache status labels reported by InspectStoredAAT.
const (
	AATStatusAbsent   = "absent"
	AATStatusValid    = "valid"
	AATStatusExpiring = "expiring"
	AATStatusExpired  = "expired"
	AATStatusMismatch = "mismatch"
	// AATStatusUnreadable means the cache could not be read at all — a locked
	// keychain or a corrupt payload. Reporting that as "absent" would tell the
	// user a token will be minted on first use, while the next real call fails
	// on storage instead.
	AATStatusUnreadable = "unreadable"
)

// InspectStoredAAT reports what the CLI-owned cache currently holds for appID
// without minting, verifying, renewing or writing anything.
//
// Diagnostics need to answer "is a credential in hand and how long does it
// last" without becoming a credential acquisition themselves — otherwise
// checking your status would change it.
func InspectStoredAAT(kc keychain.KeychainAccess, appID, userOpenID string, now time.Time) (*StoredAAToken, string) {
	token, err := GetStoredAAT(kc, appID)
	if err != nil {
		return nil, AATStatusUnreadable
	}
	if token == nil {
		return nil, AATStatusAbsent
	}
	switch storedAATState(token, appID, userOpenID, now) {
	case aatCacheFresh:
		return token, AATStatusValid
	case aatCacheNeedsRefresh:
		return token, AATStatusExpiring
	case aatCacheExpired:
		return token, AATStatusExpired
	default:
		// Present but bound to a different app or principal: it can never be
		// reused, so reporting it as available would be misleading.
		return nil, AATStatusMismatch
	}
}

// AATRefreshFunc mints and Introspects one new AAT. It must return only a token
// whose app and verified Agent Employee open ID match the requested profile.
type AATRefreshFunc func(context.Context) (*StoredAAToken, error)

// AATCacheResult contains a usable AAT and, when a retryable early refresh
// failed, the failure that caused the still-valid cached token to be used.
type AATCacheResult struct {
	Token *StoredAAToken
	// Refreshed reports whether this call minted the returned token rather than
	// reusing a cached one.
	Refreshed    bool
	RefreshError error
}

// ResolveStoredAAT returns a valid AAT, renewing it under an app-scoped
// process/cross-process lock when necessary. A retryable renewal failure may
// use the old token only before its hard expiry and only when provenance still
// matches the current profile.
func ResolveStoredAAT(
	ctx context.Context,
	kc keychain.KeychainAccess,
	appID, userOpenID string,
	now func() time.Time,
	refresh AATRefreshFunc,
) (*AATCacheResult, error) {
	if appID == "" || userOpenID == "" {
		return nil, errs.NewConfigError(errs.SubtypeInvalidConfig,
			"AAT profile is missing app or principal identity")
	}
	if refresh == nil {
		return nil, errs.NewInternalError(errs.SubtypeSDKError,
			"AAT refresh function is unavailable")
	}
	if now == nil {
		now = time.Now
	}

	cached, err := GetStoredAAT(kc, appID)
	if err != nil && !errors.Is(err, errStoredAATCorrupt) {
		return nil, err
	}
	if storedAATState(cached, appID, userOpenID, now()) == aatCacheFresh {
		return &AATCacheResult{Token: cached}, nil
	}

	var result *AATCacheResult
	err = withTokenStorageLock(appID, "aat", func() error {
		current, readErr := GetStoredAAT(kc, appID)
		if readErr != nil && !errors.Is(readErr, errStoredAATCorrupt) {
			return readErr
		}
		state := storedAATState(current, appID, userOpenID, now())
		if state == aatCacheFresh {
			result = &AATCacheResult{Token: current}
			return nil
		}

		refreshed, refreshErr := refresh(ctx)
		if refreshErr != nil {
			problem, typed := errs.ProblemOf(refreshErr)
			if state == aatCacheNeedsRefresh && typed && problem.Retryable && ctx.Err() == nil {
				result = &AATCacheResult{Token: current, RefreshError: refreshErr}
				return nil
			}
			return refreshErr
		}
		if err := validateStoredAAT(refreshed, appID, userOpenID); err != nil {
			return err
		}
		if !time.UnixMilli(refreshed.ExpiresAt).After(now()) {
			return errs.NewAuthenticationError(errs.SubtypeTokenExpired,
				"newly minted AAT is already expired")
		}
		if err := writeStoredAAT(kc, appID, refreshed); err != nil {
			return err
		}
		result = &AATCacheResult{Token: refreshed, Refreshed: true}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if result == nil || result.Token == nil {
		return nil, errs.NewInternalError(errs.SubtypeSDKError,
			"AAT cache resolution completed without a token")
	}
	return result, nil
}
