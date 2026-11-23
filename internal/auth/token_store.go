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

func accountKeyV2(appID, userOpenID string) string {
	return "uat:v2:" + accountKey(appID, userOpenID)
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
	token, _, err := readStoredToken(appId, userOpenId)
	return token, err
}

func readStoredToken(appID, userOpenID string) (*StoredUAToken, string, error) {
	account := accountKeyV2(appID, userOpenID)
	token, err := readStoredTokenAt(appID, userOpenID, account)
	if err != nil || token != nil {
		return token, account, err
	}
	account = accountKey(appID, userOpenID)
	token, err = readStoredTokenAt(appID, userOpenID, account)
	return token, account, err
}

func readStoredTokenAt(appID, userOpenID, account string) (*StoredUAToken, error) {
	raw, err := keychain.Get(keychain.LarkCliService, account)
	if err != nil {
		return nil, err
	}
	if raw == "" {
		return nil, nil
	}
	var token *StoredUAToken
	if err := json.Unmarshal([]byte(raw), &token); err != nil {
		return nil, withCorruptTokenRecovery(errs.NewInternalError(errs.SubtypeStorage,
			"failed to decode stored token: %v", err).
			WithCause(errors.Join(errStoredTokenCorrupt, err)))
	}
	if token != nil || account == accountKey(appID, userOpenID) {
		if err := validateStoredToken(token, appID, userOpenID); err != nil {
			return nil, withCorruptTokenRecovery(err)
		}
		if token.TokenType == "" {
			token.TokenType = StoredTokenTypeBearer
		}
	}
	return token, nil
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
		if problem, ok := errs.ProblemOf(err); ok && problem.Hint != "" {
			return nil, err
		}
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

// SetStoredToken persists a newly authorized UAT in v2.
func SetStoredToken(token *StoredUAToken) error {
	if token == nil {
		return errs.NewInternalError(errs.SubtypeStorage,
			"cannot store a nil token")
	}
	return withTokenStorageLock(token.AppId, token.UserOpenId, func() error {
		return writeStoredToken(token.AppId, token.UserOpenId, accountKeyV2(token.AppId, token.UserOpenId), token)
	})
}

// writeStoredToken persists token at account. The caller must hold the account's
// token storage lock.
func writeStoredToken(appID, userOpenID, account string, token *StoredUAToken) error {
	if token == nil {
		return errs.NewInternalError(errs.SubtypeStorage,
			"cannot store a nil token")
	}
	if err := validateStoredToken(token, appID, userOpenID); err != nil {
		return err
	}
	// A new authorization can replace an unreadable credential at its destination.
	previous, _ := readStoredTokenAt(appID, userOpenID, account)
	data, err := json.Marshal(token)
	if err != nil {
		return errs.NewInternalError(errs.SubtypeStorage, "failed to encode stored token").WithCause(err)
	}
	if err := keychain.Set(keychain.LarkCliService, account, string(data)); err != nil {
		return err
	}
	// The token now owns its new key. Best-effort cleanup of the superseded key
	// happens only after the token write, so a cleanup failure cannot roll back
	// into a token record whose new key was deleted by the caller.
	if previous != nil && previous.DPoPKeyID != token.DPoPKeyID {
		cleanupStoredTokenKey(previous)
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

// deleteStoredToken clears both entries. The caller must hold the account lock.
func deleteStoredToken(appID, userOpenID string) error {
	current, _ := readStoredTokenAt(appID, userOpenID, accountKeyV2(appID, userOpenID))
	legacy, _ := readStoredTokenAt(appID, userOpenID, accountKey(appID, userOpenID))
	// Remove the fallback first so a partial failure cannot revive it through v2.
	if err := keychain.Remove(keychain.LarkCliService, accountKey(appID, userOpenID)); err != nil {
		return err
	}
	defer cleanupStoredTokenKey(legacy)
	if err := keychain.Remove(keychain.LarkCliService, accountKeyV2(appID, userOpenID)); err != nil {
		return err
	}
	cleanupStoredTokenKey(current)
	return nil
}

func cleanupStoredTokenKey(token *StoredUAToken) {
	if token == nil || token.DPoPKeyID == "" {
		return
	}
	// A retained legacy session may still own the key being replaced in v2.
	for _, account := range []string{accountKeyV2(token.AppId, token.UserOpenId), accountKey(token.AppId, token.UserOpenId)} {
		current, err := readStoredTokenAt(token.AppId, token.UserOpenId, account)
		if err != nil {
			keychain.LogAuthError("dpop", "cleanup-token-key", err)
			return
		}
		if current != nil && current.DPoPKeyID == token.DPoPKeyID {
			return
		}
	}
	if err := dpop.NewKeyStore(nil).Delete(token.DPoPKeyID); err != nil {
		keychain.LogAuthError("dpop", "cleanup-token-key", err)
	}
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

// compareAndSwapStoredToken replaces expected with updated in the same entry
// when the stored token generation still matches expected. The caller must
// hold the storage lock for appID and userOpenID.
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

	current, account, err := readStoredToken(appID, userOpenID)
	if err != nil {
		return nil, false, err
	}
	if !isSameStoredTokenGeneration(current, expected) {
		return current, false, nil
	}
	if err := writeStoredToken(appID, userOpenID, account, updated); err != nil {
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

	current, _, err := readStoredToken(appID, userOpenID)
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
	if token.RefreshToken == "" {
		if now < token.ExpiresAt {
			return "valid"
		}
		return "expired"
	}
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
