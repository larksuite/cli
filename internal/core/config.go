// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/larksuite/cli/internal/keychain"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/internal/vfs"
)

// Identity represents the caller identity for API requests.
type Identity string

const (
	AsUser Identity = "user"
	AsBot  Identity = "bot"
	AsAuto Identity = "auto"
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
	Name       string      `json:"name,omitempty"`
	AppId      string      `json:"appId"`
	AppSecret  SecretInput `json:"appSecret"`
	Brand      LarkBrand   `json:"brand"`
	Lang       string      `json:"lang,omitempty"`
	DefaultAs  Identity    `json:"defaultAs,omitempty"` // AsUser | AsBot | AsAuto
	StrictMode *StrictMode `json:"strictMode,omitempty"`
	Users      []AppUser   `json:"users"`
}

// ProfileName returns the display name for this app config.
// If Name is set, returns Name; otherwise falls back to AppId.
func (a *AppConfig) ProfileName() string {
	if a.Name != "" {
		return a.Name
	}
	return a.AppId
}

// MultiAppConfig is the multi-app config file format.
type MultiAppConfig struct {
	StrictMode  StrictMode  `json:"strictMode,omitempty"`
	CurrentApp  string      `json:"currentApp,omitempty"`
	PreviousApp string      `json:"previousApp,omitempty"`
	Apps        []AppConfig `json:"apps"`
}

// CurrentAppConfig returns the currently active app config.
// Resolution priority: profileOverride > CurrentApp field > Apps[0].
func (m *MultiAppConfig) CurrentAppConfig(profileOverride string) *AppConfig {
	if profileOverride != "" {
		if app := m.FindApp(profileOverride); app != nil {
			return app
		}
		return nil
	}
	if m.CurrentApp != "" {
		if app := m.FindApp(m.CurrentApp); app != nil {
			return app
		}
		return nil // explicit currentApp not found; don't silently fallback
	}
	if len(m.Apps) > 0 {
		return &m.Apps[0]
	}
	return nil
}

// FindApp looks up an app by name, then by appId. Returns nil if not found.
// Name match takes priority: if profile A has Name "X" and profile B has AppId "X",
// FindApp("X") returns profile A.
func (m *MultiAppConfig) FindApp(name string) *AppConfig {
	// First pass: match by Name
	for i := range m.Apps {
		if m.Apps[i].Name != "" && m.Apps[i].Name == name {
			return &m.Apps[i]
		}
	}
	// Second pass: match by AppId
	for i := range m.Apps {
		if m.Apps[i].AppId == name {
			return &m.Apps[i]
		}
	}
	return nil
}

// FindAppIndex looks up an app index by name, then by appId. Returns -1 if not found.
func (m *MultiAppConfig) FindAppIndex(name string) int {
	for i := range m.Apps {
		if m.Apps[i].Name != "" && m.Apps[i].Name == name {
			return i
		}
	}
	for i := range m.Apps {
		if m.Apps[i].AppId == name {
			return i
		}
	}
	return -1
}

// ProfileNames returns all profile names (Name if set, otherwise AppId).
func (m *MultiAppConfig) ProfileNames() []string {
	names := make([]string, len(m.Apps))
	for i := range m.Apps {
		names[i] = m.Apps[i].ProfileName()
	}
	return names
}

// ValidateProfileName checks that a profile name is valid.
// Rejects empty names, whitespace, control characters, and shell-problematic characters,
// but allows Unicode letters (e.g. Chinese, Japanese) for localized profile names.
func ValidateProfileName(name string) error {
	if name == "" {
		return fmt.Errorf("profile name cannot be empty")
	}
	if utf8.RuneCountInString(name) > 64 {
		return fmt.Errorf("profile name %q is too long (max 64 characters)", name)
	}
	for _, r := range name {
		if r <= 0x1F || r == 0x7F { // control characters
			return fmt.Errorf("invalid profile name %q: contains control characters", name)
		}
		switch r {
		case ' ', '\t', '/', '\\', '"', '\'', '`', '$', '#', '!', '&', '|', ';', '(', ')', '{', '}', '[', ']', '<', '>', '?', '*', '~':
			return fmt.Errorf("invalid profile name %q: contains invalid character %q", name, r)
		}
	}
	return nil
}

// CliConfig is the resolved single-app config used by downstream code.
type CliConfig struct {
	ProfileName         string
	AppID               string
	AppSecret           string
	Brand               LarkBrand
	DefaultAs           Identity // AsUser | AsBot | AsAuto | "" (from config file)
	UserOpenId          string
	UserName            string
	SupportedIdentities uint8 `json:"-"` // bitflag: 1=user, 2=bot; set by credential provider
}

// GetConfigDir returns the config directory path.
// If the home directory cannot be determined, it falls back to a relative path
// and prints a warning to stderr.
func GetConfigDir() string {
	if dir := os.Getenv("LARKSUITE_CLI_CONFIG_DIR"); dir != "" {
		return dir
	}
	home, err := vfs.UserHomeDir()
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
	data, err := vfs.ReadFile(GetConfigPath())
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
	if err := vfs.MkdirAll(dir, 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	return validate.AtomicWrite(GetConfigPath(), append(data, '\n'), 0600)
}

// RequireConfig loads the single-app config using the default profile resolution.
func RequireConfig(kc keychain.KeychainAccess) (*CliConfig, error) {
	return RequireConfigForProfile(kc, "")
}

// RequireConfigForProfile loads the single-app config for a specific profile.
// Resolution priority: profileOverride > config.CurrentApp > Apps[0].
func RequireConfigForProfile(kc keychain.KeychainAccess, profileOverride string) (*CliConfig, error) {
	raw, err := LoadMultiAppConfig()
	if err != nil || raw == nil || len(raw.Apps) == 0 {
		return nil, &ConfigError{Code: 2, Type: "config", Message: "not configured", Hint: "run `lark-cli config init --new` in the background. It blocks and outputs a verification URL — retrieve the URL and open it in a browser to complete setup."}
	}
	return ResolveConfigFromMulti(raw, kc, profileOverride)
}

// ResolveConfigFromMulti resolves a single-app config from an already-loaded MultiAppConfig.
// This avoids re-reading the config file when the caller has already loaded it.
func ResolveConfigFromMulti(raw *MultiAppConfig, kc keychain.KeychainAccess, profileOverride string) (*CliConfig, error) {
	app := raw.CurrentAppConfig(profileOverride)
	if app == nil {
		return nil, &ConfigError{
			Code:    2,
			Type:    "config",
			Message: fmt.Sprintf("profile %q not found", profileOverride),
			Hint:    fmt.Sprintf("available profiles: %s", formatProfileNames(raw.ProfileNames())),
		}
	}

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
		ProfileName: app.ProfileName(),
		AppID:       app.AppId,
		AppSecret:   secret,
		Brand:       app.Brand,
		DefaultAs:   app.DefaultAs,
	}
	if len(app.Users) > 0 {
		cfg.UserOpenId = app.Users[0].UserOpenId
		cfg.UserName = app.Users[0].UserName
	}
	return cfg, nil
}

// RequireAuth loads config and ensures a user is logged in.
func RequireAuth(kc keychain.KeychainAccess) (*CliConfig, error) {
	return RequireAuthForProfile(kc, "")
}

// RequireAuthForProfile loads config for a profile and ensures a user is logged in.
func RequireAuthForProfile(kc keychain.KeychainAccess, profileOverride string) (*CliConfig, error) {
	cfg, err := RequireConfigForProfile(kc, profileOverride)
	if err != nil {
		return nil, err
	}
	if cfg.UserOpenId == "" {
		return nil, &ConfigError{Code: 3, Type: "auth", Message: "not logged in", Hint: "run `lark-cli auth login` in the background. It blocks and outputs a verification URL — retrieve the URL and open it in a browser to complete login."}
	}
	return cfg, nil
}

// formatProfileNames joins profile names for display.
func formatProfileNames(names []string) string {
	if len(names) == 0 {
		return "(none)"
	}
	return strings.Join(names, ", ")
}
