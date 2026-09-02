// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/keychain"
)

// StoredUAToken represents a stored user access token.
type StoredUAToken struct {
	UserOpenId       string `json:"userOpenId"`
	AppId            string `json:"appId"`
	AccessToken      string `json:"accessToken"`
	RefreshToken     string `json:"refreshToken"`
	ExpiresAt        int64  `json:"expiresAt"`        // Unix ms
	RefreshExpiresAt int64  `json:"refreshExpiresAt"` // Unix ms
	Scope            string `json:"scope"`
	GrantedAt        int64  `json:"grantedAt"` // Unix ms
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

// GetStoredToken reads the stored UAT for a given (appId, userOpenId) pair.
func GetStoredToken(appId, userOpenId string) *StoredUAToken {
	token, _ := readStoredToken(appId, userOpenId)
	return token
}

func readStoredToken(appId, userOpenId string) (*StoredUAToken, error) {
	jsonStr, err := keychain.Get(keychain.LarkCliService, accountKey(appId, userOpenId))
	if err != nil {
		storageErr := errs.NewInternalError(errs.SubtypeStorage,
			"failed to read stored token: %v", err).
			WithCause(err)
		if problem, ok := errs.ProblemOf(err); ok && problem.Hint != "" {
			storageErr.WithHint("%s", problem.Hint)
		}
		return nil, storageErr
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
	return &token, nil
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
	if token.AppId != appID || token.UserOpenId != userOpenID {
		return errs.NewInternalError(errs.SubtypeStorage,
			"cannot store a token for a different account")
	}
	key := accountKey(appID, userOpenID)
	data, err := json.Marshal(token)
	if err != nil {
		return err
	}
	return keychain.Set(keychain.LarkCliService, key, string(data))
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
	return keychain.Remove(keychain.LarkCliService, accountKey(appID, userOpenID))
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

	current, err := readStoredToken(appID, userOpenID)
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

	current, err := readStoredToken(appID, userOpenID)
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
	now := time.Now().UnixMilli()
	if now < token.ExpiresAt-refreshAheadMs {
		return "valid"
	}
	if now < token.RefreshExpiresAt {
		return "needs_refresh"
	}
	return "expired"
}
