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
	jsonStr, err := keychain.Get(keychain.LarkCliService, accountKey(appId, userOpenId))
	if err != nil || jsonStr == "" {
		return nil
	}
	var token StoredUAToken
	if err := json.Unmarshal([]byte(jsonStr), &token); err != nil {
		return nil
	}
	return &token
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
