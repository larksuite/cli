// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/larksuite/cli/internal/keychain"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/validate"
)

// Identity represents the caller identity for API requests.
type Identity string

const (
	AsUser Identity = "user"
	AsBot  Identity = "bot"
)

// IsBot returns true if the identity is bot.
func (id Identity) IsBot() bool { return id == AsBot }

// AppUser is a logged-in user record stored in config.
type AppUser struct {
	UserOpenId string `json:"userOpenId"`
	UserName   string `json:"userName"`
}

// AppConfig is a per-app configuration entry (stored format — secrets may be unresolved).
type AppConfig struct {
	AppId     string      `json:"appId"`
	AppSecret SecretInput `json:"appSecret"`
	Brand     LarkBrand   `json:"brand"`
	Lang      string      `json:"lang,omitempty"`
	DefaultAs string      `json:"defaultAs,omitempty"` // "user" | "bot" | "auto"
	Users     []AppUser   `json:"users"`
}

// MultiAppConfig is the multi-app config file format.
type MultiAppConfig struct {
	Apps []AppConfig `json:"apps"`
}

// CliConfig is the resolved single-app config used by downstream code.
type CliConfig struct {
	AppID      string
	AppSecret  string
	Brand      LarkBrand
	DefaultAs  string // "user" | "bot" | "auto" | "" (from config file)
	UserOpenId string
	UserName   string
}

// GetConfigDir returns the config directory path.
// If the home directory cannot be determined, it falls back to a relative path
// and prints a warning to stderr.
func GetConfigDir() string {
	if dir := os.Getenv("LARKSUITE_CLI_CONFIG_DIR"); dir != "" {
		return dir
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		fmt.Fprintf(os.Stderr, "warning: unable to determine home directory: %v\n", err)
	}
	return filepath.Join(home, ".lark-cli")
}

// GetConfigPath returns the config file path.
func GetConfigPath() string {
	return filepath.Join(GetConfigDir(), "config.json")
}

// LoadMultiAppConfig loads multi-app config from disk.
func LoadMultiAppConfig() (*MultiAppConfig, error) {
	data, err := os.ReadFile(GetConfigPath())
	if err != nil {
		return nil, err
	}

	var multi MultiAppConfig
	if err := json.Unmarshal(data, &multi); err != nil {
		return nil, fmt.Errorf("invalid config format: %w", err)
	}
	if len(multi.Apps) == 0 {
		return nil, fmt.Errorf("invalid config format: no apps")
	}
	return &multi, nil
}

// SaveMultiAppConfig saves config to disk.
func SaveMultiAppConfig(config *MultiAppConfig) error {
	dir := GetConfigDir()
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	return validate.AtomicWrite(GetConfigPath(), append(data, '\n'), 0600)
}

// RequireConfig loads the single-app config. Takes Apps[0] directly.
func RequireConfig(kc keychain.KeychainAccess) (*CliConfig, error) {
	raw, err := LoadMultiAppConfig()
	if err != nil || raw == nil || len(raw.Apps) == 0 {
		return nil, &ConfigError{Code: 2, Type: "config", Message: "not configured", Hint: "run `lark-cli config init --new` in the background. It blocks and outputs a verification URL — retrieve the URL and open it in a browser to complete setup."}
	}
	app := raw.Apps[0]
	secret, err := ResolveSecretInput(app.AppSecret, kc)
	if err != nil {
		// If the error comes from the keychain, it will already be wrapped as an ExitError.
		// For other errors (e.g. file read errors, unknown sources), wrap them as ConfigError.
		var exitErr *output.ExitError
		if errors.As(err, &exitErr) {
			return nil, exitErr
		}
		return nil, &ConfigError{Code: 2, Type: "config", Message: err.Error()}
	}
	cfg := &CliConfig{
		AppID:     app.AppId,
		AppSecret: secret,
		Brand:     app.Brand,
		DefaultAs: app.DefaultAs,
	}
	if len(app.Users) > 0 {
		cfg.UserOpenId = app.Users[0].UserOpenId
		cfg.UserName = app.Users[0].UserName
	}
	return cfg, nil
}

// RequireAuth loads config and ensures a user is logged in.
func RequireAuth(kc keychain.KeychainAccess) (*CliConfig, error) {
	cfg, err := RequireConfig(kc)
	if err != nil {
		return nil, err
	}
	if cfg.UserOpenId == "" {
		return nil, &ConfigError{Code: 3, Type: "auth", Message: "not logged in", Hint: "run `lark-cli auth login` in the background. It blocks and outputs a verification URL — retrieve the URL and open it in a browser to complete login."}
	}
	return cfg, nil
}
