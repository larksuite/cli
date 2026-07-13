// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"time"

	larkauth "github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/internal/vfs"
)

var loginScopeCacheSafeChars = regexp.MustCompile(`[^a-zA-Z0-9._-]`)

type loginScopeCacheRecord struct {
	RequestedScope string `json:"requested_scope"`
}

type pendingLoginRecord struct {
	DeviceCode     string `json:"device_code"`
	AppID          string `json:"app_id"`
	RequestedScope string `json:"requested_scope"`
	ExpiresAt      int64  `json:"expires_at"`
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

func pendingLoginPath(appID string) string {
	return filepath.Join(loginScopeCacheDir(), "latest-"+sanitizeLoginScopeCacheKey(appID)+".json")
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

// savePendingLogin persists the latest split-flow authorization so a later
// agent turn can resume it without copying a device code through the model's
// conversation context. The per-device scope record remains for backwards
// compatibility with the explicit --device-code flow.
func savePendingLogin(deviceCode, appID, requestedScope string, expiresIn int) error {
	if err := saveLoginRequestedScope(deviceCode, requestedScope); err != nil {
		return err
	}
	record := pendingLoginRecord{
		DeviceCode:     deviceCode,
		AppID:          appID,
		RequestedScope: requestedScope,
		ExpiresAt:      time.Now().Add(time.Duration(expiresIn) * time.Second).Unix(),
	}
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	return validate.AtomicWrite(pendingLoginPath(appID), data, 0600)
}

func loadPendingLogin(appID string) (*pendingLoginRecord, error) {
	path := pendingLoginPath(appID)
	data, err := vfs.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var record pendingLoginRecord
	if err := json.Unmarshal(data, &record); err != nil {
		_ = vfs.Remove(path)
		return nil, err
	}
	if record.DeviceCode == "" || record.AppID != appID || record.ExpiresAt <= time.Now().Unix() {
		_ = vfs.Remove(path)
		return nil, os.ErrNotExist
	}
	return &record, nil
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

func removePendingLogin(deviceCode, appID string) error {
	firstErr := removeLoginRequestedScope(deviceCode)
	path := pendingLoginPath(appID)
	if data, err := vfs.ReadFile(path); err == nil {
		var record pendingLoginRecord
		if json.Unmarshal(data, &record) == nil && record.DeviceCode == deviceCode {
			if err := vfs.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) && firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
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
