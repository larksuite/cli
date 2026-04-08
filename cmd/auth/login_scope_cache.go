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

func loginScopeCacheDir() string {
	return filepath.Join(core.GetConfigDir(), "cache", "auth_login_scopes")
}

func loginScopeCachePath(deviceCode string) string {
	return filepath.Join(loginScopeCacheDir(), sanitizeLoginScopeCacheKey(deviceCode)+".json")
}

func sanitizeLoginScopeCacheKey(deviceCode string) string {
	sanitized := loginScopeCacheSafeChars.ReplaceAllString(deviceCode, "_")
	if sanitized == "" {
		return "default"
	}
	return sanitized
}

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

func removeLoginRequestedScope(deviceCode string) error {
	err := vfs.Remove(loginScopeCachePath(deviceCode))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func shouldRemoveLoginRequestedScope(result *larkauth.DeviceFlowResult) bool {
	if result == nil {
		return false
	}
	if result.OK || result.Error == "access_denied" {
		return true
	}
	return result.Error == "expired_token" && result.Message != "Polling was cancelled"
}
