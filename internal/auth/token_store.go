// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"encoding/json"
	"fmt"
	"time"

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
		return nil, err
	}
	if jsonStr == "" {
		return nil, nil
	}
	var token StoredUAToken
	if err := json.Unmarshal([]byte(jsonStr), &token); err != nil {
		return nil, err
	}
	return &token, nil
}

// SetStoredToken persists a UAT.
func SetStoredToken(token *StoredUAToken) error {
	key := accountKey(token.AppId, token.UserOpenId)
	data, err := json.Marshal(token)
	if err != nil {
		return err
	}
	return keychain.Set(keychain.LarkCliService, key, string(data))
}

// RemoveStoredToken removes a stored UAT.
func RemoveStoredToken(appId, userOpenId string) error {
	return keychain.Remove(keychain.LarkCliService, accountKey(appId, userOpenId))
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

// setStoredTokenIfCurrent stores updated only when expected is still the
// current token generation. It returns the token present after the check and
// whether the update was applied.
func setStoredTokenIfCurrent(expected, updated *StoredUAToken) (*StoredUAToken, bool, error) {
	current, err := readStoredToken(expected.AppId, expected.UserOpenId)
	if err != nil {
		return nil, false, err
	}
	if !isSameStoredTokenGeneration(current, expected) {
		return current, false, nil
	}
	if err := SetStoredToken(updated); err != nil {
		return current, false, err
	}
	return updated, true, nil
}

// removeStoredTokenIfCurrent removes expected only when it is still the
// current token generation. It returns the token retained on a mismatch.
func removeStoredTokenIfCurrent(expected *StoredUAToken) (*StoredUAToken, bool, error) {
	current, err := readStoredToken(expected.AppId, expected.UserOpenId)
	if err != nil {
		return nil, false, err
	}
	if !isSameStoredTokenGeneration(current, expected) {
		return current, false, nil
	}
	if err := RemoveStoredToken(expected.AppId, expected.UserOpenId); err != nil {
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
