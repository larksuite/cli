// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/larksuite/cli/internal/configdir"
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

var tokenKeychain = keychain.Default()

// accountKey builds the logical credential key for a user token.
func accountKey(appId, userOpenId string) string {
	return fmt.Sprintf("%s:%s", appId, userOpenId)
}

// tokenConfigDir returns the config directory used by token-store compatibility paths.
func tokenConfigDir() string {
	// Keep config dir resolution centralized in internal/configdir.
	// New code should reuse configdir.Get() instead of duplicating env/home logic.
	return configdir.Get()
}

// legacyManagedTokenFilePath returns the old plaintext managed-token file path kept for migration reads and cleanup.
func legacyManagedTokenFilePath(appId, userOpenId string) string {
	return filepath.Join(tokenConfigDir(), "tokens", sanitizeID(accountKey(appId, userOpenId))+".json")
}

// readLegacyManagedToken loads a token from the legacy plaintext fallback file if it still exists.
func readLegacyManagedToken(appId, userOpenId string) *StoredUAToken {
	data, err := os.ReadFile(legacyManagedTokenFilePath(appId, userOpenId))
	if err != nil {
		return nil
	}
	var token StoredUAToken
	if err := json.Unmarshal(data, &token); err != nil {
		return nil
	}
	return &token
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
	jsonStr, err := tokenKeychain.Get(keychain.LarkCliService, accountKey(appId, userOpenId))
	if err == nil && jsonStr != "" {
		var token StoredUAToken
		if err := json.Unmarshal([]byte(jsonStr), &token); err == nil {
			return &token
		}
	}
	jsonStr, err = keychain.GetFallbackWithError(keychain.LarkCliService, accountKey(appId, userOpenId))
	if err != nil {
		if !os.IsNotExist(err) {
			return nil
		}
		return readLegacyManagedToken(appId, userOpenId)
	}
	var token StoredUAToken
	if err := json.Unmarshal([]byte(jsonStr), &token); err != nil {
		return nil
	}
	return &token
}

// SetStoredToken persists a UAT.
func SetStoredToken(token *StoredUAToken) (bool, error) {
	key := accountKey(token.AppId, token.UserOpenId)
	data, err := json.Marshal(token)
	if err != nil {
		return false, err
	}
	if err := tokenKeychain.Set(keychain.LarkCliService, key, string(data)); err == nil {
		_ = keychain.RemoveFallback(keychain.LarkCliService, key)
		_ = os.Remove(legacyManagedTokenFilePath(token.AppId, token.UserOpenId))
		return false, nil
	} else if !keychain.ShouldUseFallback(err) {
		return false, fmt.Errorf("store token in keychain: %w", err)
	}
	if err := keychain.SetFallback(keychain.LarkCliService, key, string(data)); err != nil {
		return false, fmt.Errorf("store token encrypted fallback: %w", err)
	}
	_ = os.Remove(legacyManagedTokenFilePath(token.AppId, token.UserOpenId))
	return true, nil
}

// RemoveStoredToken removes a stored UAT.
func RemoveStoredToken(appId, userOpenId string) error {
	key := accountKey(appId, userOpenId)
	var errs []error
	if err := keychain.RemoveFallback(keychain.LarkCliService, key); err != nil {
		errs = append(errs, err)
	}
	if err := tokenKeychain.Remove(keychain.LarkCliService, key); err != nil {
		errs = append(errs, err)
	}
	if err := os.Remove(legacyManagedTokenFilePath(appId, userOpenId)); err != nil && !os.IsNotExist(err) {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
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
