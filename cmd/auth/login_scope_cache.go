// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"

	larkauth "github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/internal/vfs"
)

var loginScopeCacheSafeChars = regexp.MustCompile(`[^a-zA-Z0-9._-]`)

type loginScopeCacheRecord struct {
	RequestedScope string `json:"requested_scope"`
}

// loginScopeCacheDir returns the directory used to persist auth login --no-wait
// requested scopes keyed by device_code.
func loginScopeCacheDir() string {
	return filepath.Join(core.GetConfigDir(), "cache", "auth_login_scopes")
}

// loginScopeCachePath returns the cache file path for a given device_code.
func loginScopeCachePath(deviceCode string) string {
	return filepath.Join(loginScopeCacheDir(), sanitizeLoginScopeCacheKey(deviceCode)+".json")
}

// sanitizeLoginScopeCacheKey converts a device_code into a safe filename token.
func sanitizeLoginScopeCacheKey(deviceCode string) string {
	sanitized := loginScopeCacheSafeChars.ReplaceAllString(deviceCode, "_")
	if sanitized == "" {
		return "default"
	}
	return sanitized
}

// saveLoginRequestedScope persists the requested scope string for a device_code.
func saveLoginRequestedScope(deviceCode, requestedScope string) error {
	if err := vfs.MkdirAll(loginScopeCacheDir(), 0700); err != nil {
		return err
	}
	data, err := json.Marshal(loginScopeCacheRecord{RequestedScope: requestedScope})
	if err != nil {
		return err
	}
	return validate.AtomicWrite(loginScopeCachePath(deviceCode), data, 0600)
}

// loadLoginRequestedScope loads the cached requested scope string for a device_code.
// It returns an empty string if no cache entry exists.
func loadLoginRequestedScope(deviceCode string) (string, error) {
	data, err := vfs.ReadFile(loginScopeCachePath(deviceCode))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", err
	}
	var record loginScopeCacheRecord
	if err := json.Unmarshal(data, &record); err != nil {
		_ = vfs.Remove(loginScopeCachePath(deviceCode))
		return "", err
	}
	return record.RequestedScope, nil
}

// removeLoginRequestedScope deletes the cache entry for a device_code.
func removeLoginRequestedScope(deviceCode string) error {
	err := vfs.Remove(loginScopeCachePath(deviceCode))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// shouldRemoveLoginRequestedScope indicates whether the requested-scope cache
// should be removed after polling finishes.
func shouldRemoveLoginRequestedScope(result *larkauth.DeviceFlowResult) bool {
	if result == nil {
		return false
	}
	if result.OK || result.Error == "access_denied" {
		return true
	}
	return result.Error == "expired_token" && result.Message != "Polling was cancelled"
}
