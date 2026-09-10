// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/dpop"
	"github.com/larksuite/cli/internal/keychain"
	"github.com/larksuite/cli/internal/recovery"
)

const (
	StoredTokenTypeBearer = "Bearer"
	StoredTokenTypeDPoP   = dpop.TokenType
)

// StoredUAToken represents a stored user access token.
type StoredUAToken struct {
	UserOpenId           string `json:"userOpenId"`
	AppId                string `json:"appId"`
	AccessToken          string `json:"accessToken"`
	RefreshToken         string `json:"refreshToken"`
	ExpiresAt            int64  `json:"expiresAt"`        // Unix ms
	RefreshExpiresAt     int64  `json:"refreshExpiresAt"` // Unix ms
	Scope                string `json:"scope"`
	GrantedAt            int64  `json:"grantedAt"` // Unix ms
	TokenType            string `json:"tokenType,omitempty"`
	DPoPKeyID            string `json:"dpopKeyId,omitempty"`
	DPoPJKT              string `json:"dpopJkt,omitempty"`
	DPoPKeySecurityLevel string `json:"dpopKeySecurityLevel,omitempty"`
	ClockOffsetMs        int64  `json:"clockOffsetMs,omitempty"`
	ClockSyncedAtMs      int64  `json:"clockSyncedAtMs,omitempty"`
}

const refreshAheadMs = 5 * 60 * 1000 // 5 minutes

var errStoredTokenCorrupt = errors.New("stored token data is corrupt")

// accountKey generates a unique key for an account based on its AppID and UserOpenID.
func accountKey(appId, userOpenId string) string {
	return fmt.Sprintf("%s:%s", appId, userOpenId)
}

// MaskToken masks a token for safe logging.
func MaskToken(token string) string {
	if len(token) <= 8 {
		return "****"
	}
	return "****" + token[len(token)-4:]
}

// GetStoredToken reads the stored UAT and preserves storage and decode errors.
func GetStoredToken(appId, userOpenId string) (*StoredUAToken, error) {
	token, err := readStoredToken(appId, userOpenId)
	if errors.Is(err, errStoredTokenCorrupt) {
		return nil, withCorruptTokenRecovery(err)
	}
	return token, err
}

func readStoredToken(appId, userOpenId string) (*StoredUAToken, error) {
	jsonStr, err := keychain.Get(keychain.LarkCliService, accountKey(appId, userOpenId))
	if err != nil {
		return nil, err
	}
	if jsonStr == "" {
		return nil, nil
	}
	var token StoredUAToken
	if err := json.Unmarshal([]byte(jsonStr), &token); err != nil {
		return nil, errs.NewInternalError(errs.SubtypeStorage,
			"failed to decode stored token: %v", err).
			WithCause(errors.Join(errStoredTokenCorrupt, err))
	}
	if err := validateStoredToken(&token, appId, userOpenId); err != nil {
		return nil, err
	}
	if token.TokenType == "" {
		token.TokenType = StoredTokenTypeBearer
	}
	return &token, nil
}

// withCorruptTokenRecovery attaches re-authorization guidance to a read-side
// corruption error: a new login overwrites the damaged entry, so it is the
// recovery step. The write-side validator stays hint-free because a rejected
// write leaves nothing on disk to re-authorize.
func withCorruptTokenRecovery(err error) error {
	return recovery.Attach(err, recovery.UserAuthorization())
}

// ResolveDPoPBindingContext restores a stored DPoP binding using the caller's
// cancellation and deadline for platform key access.
func ResolveDPoPBindingContext(ctx context.Context, token *StoredUAToken, store *dpop.KeyStore) (*dpop.Binding, error) {
	if token == nil || token.TokenType == "" || token.TokenType == StoredTokenTypeBearer {
		return nil, nil
	}
	if token.TokenType != StoredTokenTypeDPoP || token.DPoPKeyID == "" || token.DPoPJKT == "" {
		return nil, recovery.Attach(errs.NewAuthenticationError(errs.SubtypeDPoPKeyMissing,
			"stored DPoP token is missing its key binding"), dpopReauthorizationHint(
			"run `lark-cli auth login` to re-authorize this profile"))
	}
	if store == nil {
		store = dpop.NewKeyStore(nil)
	}
	key, err := store.LoadContext(ctx, token.DPoPKeyID)
	if err != nil {
		return nil, recovery.Attach(errs.NewAuthenticationError(errs.SubtypeDPoPKeyMissing,
			"stored DPoP key is unavailable: %v", err).
			WithCause(err), dpopKeyUnavailableHint())
	}
	// The token record is the atomic owner of the clock domain used for this
	// token's expiry and proofs. Always override key metadata, including with a
	// zero state, so a partially committed metadata update cannot leak into an
	// older token generation.
	key.Clock().RestoreState(dpop.ClockState{
		OffsetMillis:   token.ClockOffsetMs,
		SyncedAtMillis: token.ClockSyncedAtMs,
	})
	binding, err := dpop.RestoreBinding(token.AccessToken, token.DPoPKeyID, token.DPoPJKT, key)
	if err != nil {
		return nil, recovery.Attach(errs.NewAuthenticationError(errs.SubtypeDPoPBindingMismatch,
			"stored DPoP token and key do not match").
			WithCause(err), dpopReauthorizationHint(
			"run `lark-cli auth login` to re-authorize this profile"))
	}
	if token.DPoPKeySecurityLevel != "" &&
		token.DPoPKeySecurityLevel != string(binding.KeyStoreSecureLevel) {
		return nil, recovery.Attach(errs.NewAuthenticationError(errs.SubtypeDPoPBindingMismatch,
			"stored DPoP token and key protection level do not match"),
			dpopReauthorizationHint("run `lark-cli auth login` to re-authorize this profile"))
	}
	return binding, nil
}

func dpopReauthorizationHint(text string) recovery.Hint {
	return recovery.Join("", recovery.Command(recovery.TargetAuthLogin, text)).
		WithFallback("re-authorize this profile through the distribution's supported authorization flow")
}

func dpopKeyUnavailableHint() recovery.Hint {
	return recovery.Join("; ",
		recovery.Text(dpop.KeyAccessUnavailableHint),
		recovery.Command(recovery.TargetAuthLogin,
			"if the key remains unavailable outside the sandbox, run `lark-cli auth login` to re-authorize"),
	).WithFallback("restore access to the platform key store outside the sandbox; if the key is lost, re-authorize this profile through the distribution's supported authorization flow")
}

// SetStoredToken persists a UAT.
func SetStoredToken(token *StoredUAToken) error {
	if token == nil {
		return errs.NewInternalError(errs.SubtypeStorage,
			"cannot store a nil token")
	}
	return withTokenStorageLock(token.AppId, token.UserOpenId, func() error {
		return writeStoredToken(token.AppId, token.UserOpenId, token)
	})
}

// writeStoredToken persists token for the supplied account. The caller must
// hold that account's token storage lock.
func writeStoredToken(appID, userOpenID string, token *StoredUAToken) error {
	if token == nil {
		return errs.NewInternalError(errs.SubtypeStorage,
			"cannot store a nil token")
	}
	if err := validateStoredToken(token, appID, userOpenID); err != nil {
		return err
	}
	key := accountKey(appID, userOpenID)
	previous, _ := readStoredToken(appID, userOpenID)
	data, err := json.Marshal(token)
	if err != nil {
		return err
	}
	if err := keychain.Set(keychain.LarkCliService, key, string(data)); err != nil {
		return err
	}
	// The token now owns its new key. Best-effort cleanup of the superseded key
	// happens only after the token write, so a cleanup failure cannot roll back
	// into a token record whose new key was deleted by the caller.
	if previous != nil && previous.DPoPKeyID != "" && previous.DPoPKeyID != token.DPoPKeyID {
		if err := dpop.NewKeyStore(nil).Delete(previous.DPoPKeyID); err != nil {
			keychain.LogAuthError("dpop", "cleanup-superseded-key", err)
		}
	}
	return nil
}

func validateStoredToken(token *StoredUAToken, appID, userOpenID string) error {
	var reason error
	switch {
	case token == nil:
		reason = errors.New("stored token is nil")
	case token.AppId == "" || token.UserOpenId == "":
		reason = errors.New("stored token account binding is incomplete")
	case token.AppId != appID || token.UserOpenId != userOpenID:
		reason = errors.New("stored token account binding does not match its storage key")
	case token.AccessToken == "":
		reason = errors.New("stored token has no access token")
	default:
		return nil
	}
	return errs.NewInternalError(errs.SubtypeStorage,
		"stored token data failed semantic validation").
		WithCause(errors.Join(errStoredTokenCorrupt, reason))
}

// RemoveStoredToken removes a stored UAT.
func RemoveStoredToken(appId, userOpenId string) error {
	return withTokenStorageLock(appId, userOpenId, func() error {
		return deleteStoredToken(appId, userOpenId)
	})
}

// deleteStoredToken removes the supplied account's token. The caller must hold
// that account's token storage lock.
func deleteStoredToken(appID, userOpenID string) error {
	current, err := readStoredToken(appID, userOpenID)
	if err != nil && !errors.Is(err, errStoredTokenCorrupt) {
		return err
	}
	if err := keychain.Remove(keychain.LarkCliService, accountKey(appID, userOpenID)); err != nil {
		return err
	}
	if current != nil && current.DPoPKeyID != "" {
		if err := dpop.NewKeyStore(nil).Delete(current.DPoPKeyID); err != nil {
			// Keep the key reference discoverable so cleanup can be retried
			// after a temporary signer or metadata-store failure.
			cleanupErr := errs.NewInternalError(errs.SubtypeStorage,
				"failed to delete DPoP key: %v", err).WithCause(err)
			if restoreErr := writeStoredToken(appID, userOpenID, current); restoreErr != nil {
				return errs.NewInternalError(errs.SubtypeStorage,
					"failed to delete DPoP key and restore its token record").
					WithCause(errors.Join(cleanupErr, restoreErr))
			}
			return cleanupErr
		}
	}
	return nil
}

// isSameStoredTokenGeneration reports whether two snapshots represent the same
// refresh-token generation. Access tokens are used only for case that does not
// contain a refresh token.
func isSameStoredTokenGeneration(current, expected *StoredUAToken) bool {
	if current == nil || expected == nil ||
		current.AppId != expected.AppId ||
		current.UserOpenId != expected.UserOpenId {
		return false
	}
	if current.RefreshToken != "" || expected.RefreshToken != "" {
		return current.RefreshToken == expected.RefreshToken
	}
	return current.AccessToken == expected.AccessToken
}

// compareAndSwapStoredToken replaces expected with updated when the stored
// token generation still matches expected. The caller must hold the storage
// lock for appID and userOpenID.
func compareAndSwapStoredToken(appID, userOpenID string, expected, updated *StoredUAToken) (*StoredUAToken, bool, error) {
	if expected == nil || updated == nil {
		return nil, false, errs.NewInternalError(errs.SubtypeStorage,
			"cannot compare and swap a nil stored token")
	}
	if expected.AppId != appID || expected.UserOpenId != userOpenID ||
		updated.AppId != appID || updated.UserOpenId != userOpenID {
		return nil, false, errs.NewInternalError(errs.SubtypeStorage,
			"cannot compare and swap stored tokens for different accounts")
	}

	current, err := GetStoredToken(appID, userOpenID)
	if err != nil {
		return nil, false, err
	}
	if !isSameStoredTokenGeneration(current, expected) {
		return current, false, nil
	}
	if err := writeStoredToken(appID, userOpenID, updated); err != nil {
		return current, false, err
	}
	return updated, true, nil
}

// compareAndDeleteStoredToken removes expected when the stored token generation
// still matches it. The caller must hold the storage lock for appID and
// userOpenID.
func compareAndDeleteStoredToken(appID, userOpenID string, expected *StoredUAToken) (*StoredUAToken, bool, error) {
	if expected == nil {
		return nil, false, errs.NewInternalError(errs.SubtypeStorage,
			"cannot compare and delete a nil stored token")
	}
	if expected.AppId != appID || expected.UserOpenId != userOpenID {
		return nil, false, errs.NewInternalError(errs.SubtypeStorage,
			"cannot compare and delete a stored token for a different account")
	}

	current, err := GetStoredToken(appID, userOpenID)
	if err != nil {
		return nil, false, err
	}
	if !isSameStoredTokenGeneration(current, expected) {
		return current, false, nil
	}
	if err := deleteStoredToken(appID, userOpenID); err != nil {
		return current, false, err
	}
	return nil, true, nil
}

// TokenStatus determines the freshness of a stored token.
func TokenStatus(token *StoredUAToken) string {
	now := tokenNow(token).UnixMilli()
	if now < token.ExpiresAt-refreshAheadMs {
		return "valid"
	}
	if now < token.RefreshExpiresAt {
		return "needs_refresh"
	}
	return "expired"
}

func tokenNow(token *StoredUAToken) time.Time {
	now := time.Now()
	if token == nil || token.ClockSyncedAtMs <= 0 {
		return now
	}
	return now.Add(time.Duration(token.ClockOffsetMs) * time.Millisecond)
}

func applyClockState(token *StoredUAToken, clock *dpop.AdjustableClock) {
	if token == nil || clock == nil {
		return
	}
	state := clock.State()
	token.ClockOffsetMs = state.OffsetMillis
	token.ClockSyncedAtMs = state.SyncedAtMillis
}
