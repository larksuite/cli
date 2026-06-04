// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/larksuite/cli/internal/i18n"
	"github.com/larksuite/cli/internal/keychain"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/internal/vfs"
)

// Identity is the caller identity for API requests.
type Identity string

const (
	AsUser Identity = "user"
	AsBot  Identity = "bot"
	AsAuto Identity = "auto"
)

func (id Identity) IsBot() bool { return id == AsBot }

// AppUser is a logged-in user record stored in config.
//
// All multi-user fields are `omitempty` so legacy config.json files
// load losslessly. Time fields are *time.Time because encoding/json
// does not honor omitempty on time.Time structs — pointer typing
// keeps zero-value timestamps from polluting saved files.
//
// CachedAt, LastUsed and LastScopes are a denormalised cache of
// user_index.json (the source of truth) — duplicated here so
// `lark config show` need not load the index. If they disagree,
// trust the index; the migrator reconciles on upgrade.
type AppUser struct {
	UserOpenId  string     `json:"userOpenId"`
	UserName    string     `json:"userName"`
	UnionId     string     `json:"unionId,omitempty"`
	CachedAt    *time.Time `json:"cachedAt,omitempty"`
	FirstAuthAt *time.Time `json:"firstAuthAt,omitempty"`
	LastUsed    *time.Time `json:"lastUsed,omitempty"`
	LastScopes  string     `json:"lastScopes,omitempty"`
}

// AppConfig is a per-app configuration entry (stored format — secrets may be unresolved).
//
// CurrentUser names the active user within Users[] for this app; empty
// means fall back to Users[0] so legacy config.json files resolve
// identically. Resolution order is --user > CurrentUser > Users[0].
type AppConfig struct {
	Name        string      `json:"name,omitempty"`
	AppId       string      `json:"appId"`
	AppSecret   SecretInput `json:"appSecret"`
	Brand       LarkBrand   `json:"brand"`
	Lang        i18n.Lang   `json:"lang,omitempty"`
	DefaultAs   Identity    `json:"defaultAs,omitempty"` // AsUser | AsBot | AsAuto
	StrictMode  *StrictMode `json:"strictMode,omitempty"`
	CurrentUser string      `json:"currentUser,omitempty"`
	Users       []AppUser   `json:"users"`
}

// ProfileName returns the display name for this app config (Name, or AppId fallback).
func (a *AppConfig) ProfileName() string {
	if a.Name != "" {
		return a.Name
	}
	return a.AppId
}

// FindUser looks up a user in this profile by UserOpenId, then UserName.
// Returns nil if not found.
//
// OpenId-first (vs FindApp's Name-first) is deliberate: open_ids are
// issuance-unambiguous, while display names are server-provided and
// can collide within a tenant — matching by name first would risk
// resolving `auth users use ou_xxx` to a same-named user.
//
// Empty input returns nil so AppUsers with empty UserName cannot be
// matched by passing "".
func (a *AppConfig) FindUser(idOrName string) *AppUser {
	if idOrName == "" {
		return nil
	}
	for i := range a.Users {
		if a.Users[i].UserOpenId == idOrName {
			return &a.Users[i]
		}
	}
	for i := range a.Users {
		if a.Users[i].UserName != "" && a.Users[i].UserName == idOrName {
			return &a.Users[i]
		}
	}
	return nil
}

// FindUserIndex returns the index of the matching user, or -1 if not
// found. Same OpenId-first two-pass policy as FindUser.
func (a *AppConfig) FindUserIndex(idOrName string) int {
	if idOrName == "" {
		return -1
	}
	for i := range a.Users {
		if a.Users[i].UserOpenId == idOrName {
			return i
		}
	}
	for i := range a.Users {
		if a.Users[i].UserName != "" && a.Users[i].UserName == idOrName {
			return i
		}
	}
	return -1
}

// UserNames returns "name (open_id)" for each user in this profile, or
// just the open_id if name is empty. Order matches Users[] insertion order.
func (a *AppConfig) UserNames() []string {
	out := make([]string, len(a.Users))
	for i := range a.Users {
		if a.Users[i].UserName != "" {
			out[i] = a.Users[i].UserName + " (" + a.Users[i].UserOpenId + ")"
		} else {
			out[i] = a.Users[i].UserOpenId
		}
	}
	return out
}

// CurrentSchemaVersion is the schema-version stamp written into every
// MultiAppConfig saved by this binary. Bump only for non-additive
// changes that need a migrator dispatch.
//
//   - 0: legacy (pre-multi-user; default zero-value).
//   - 1: multi-user fields (AppUser.UnionId/CachedAt/FirstAuthAt/
//     LastUsed/LastScopes, AppConfig.CurrentUser, user_index.json +
//     user_profile.json on disk).
//
// Additive `omitempty` changes do NOT bump this; a bump should be
// reserved for renames/retypes that older binaries cannot read losslessly.
const CurrentSchemaVersion = 1

// MultiAppConfig is the multi-app config file format.
//
// SchemaVersion is omitempty so legacy files load to zero; the migrator
// triggers on `cfg.SchemaVersion < CurrentSchemaVersion`.
type MultiAppConfig struct {
	SchemaVersion int         `json:"schemaVersion,omitempty"`
	StrictMode    StrictMode  `json:"strictMode,omitempty"`
	CurrentApp    string      `json:"currentApp,omitempty"`
	PreviousApp   string      `json:"previousApp,omitempty"`
	Apps          []AppConfig `json:"apps"`
}

// CurrentAppConfig returns the active app config.
// Resolution priority: profileOverride > CurrentApp > Apps[0].
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

// FindApp looks up an app by Name first, then AppId. Name match wins
// when both collide, matching the user-chosen-uniqueness contract on
// profile names. Returns nil if not found.
func (m *MultiAppConfig) FindApp(name string) *AppConfig {
	for i := range m.Apps {
		if m.Apps[i].Name != "" && m.Apps[i].Name == name {
			return &m.Apps[i]
		}
	}
	for i := range m.Apps {
		if m.Apps[i].AppId == name {
			return &m.Apps[i]
		}
	}
	return nil
}

// FindAppIndex is the index-returning sibling of FindApp. Returns -1 if not found.
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
// Allows Unicode letters (Chinese, Japanese, etc.) but rejects empty,
// over-long, control, and shell-problematic characters.
func ValidateProfileName(name string) error {
	if name == "" {
		return fmt.Errorf("profile name cannot be empty")
	}
	if utf8.RuneCountInString(name) > 64 {
		return fmt.Errorf("profile name %q is too long (max 64 characters)", name)
	}
	for _, r := range name {
		if r <= 0x1F || r == 0x7F {
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
	Lang                i18n.Lang
	SupportedIdentities uint8 `json:"-"` // bitflag: 1=user, 2=bot; set by credential provider
}

// identityBotBit is the bit flag for bot identity in SupportedIdentities.
// Must match extension/credential.SupportsBot.
const identityBotBit uint8 = 1 << 1

// CanBot reports whether the current credential context supports bot identity.
// Returns true when SupportedIdentities is unset (0, unknown) or includes the bot bit.
func (c *CliConfig) CanBot() bool {
	return c.SupportedIdentities == 0 || c.SupportedIdentities&identityBotBit != 0
}

// GetConfigDir returns the config directory path for the current workspace.
// Local workspace returns LARKSUITE_CLI_CONFIG_DIR or ~/.lark-cli (fully
// backward-compatible); openclaw/hermes return base/openclaw or base/hermes.
func GetConfigDir() string {
	return GetRuntimeDir()
}

// GetConfigPath returns the config file path for the current workspace.
func GetConfigPath() string {
	return filepath.Join(GetConfigDir(), "config.json")
}

// LoadMultiAppConfig loads multi-app config from disk.
//
// Refuses to load a SchemaVersion higher than CurrentSchemaVersion: a
// future-binary file may carry fields we'd silently drop on re-save.
// Same-version or legacy 0 always loads (additive omitempty evolution).
func LoadMultiAppConfig() (*MultiAppConfig, error) {
	data, err := vfs.ReadFile(GetConfigPath())
	if err != nil {
		return nil, err
	}

	var multi MultiAppConfig
	if err := json.Unmarshal(data, &multi); err != nil {
		return nil, fmt.Errorf("invalid config format: %w", err)
	}
	if multi.SchemaVersion > CurrentSchemaVersion {
		return nil, &ConfigError{
			Code: 3,
			Type: "config",
			Message: fmt.Sprintf(
				"config.json was written by a newer lark-cli (schemaVersion %d > supported %d)",
				multi.SchemaVersion, CurrentSchemaVersion),
			Hint: "upgrade lark-cli, or use a different --profile to avoid overwriting fields the newer binary populated",
		}
	}
	if len(multi.Apps) == 0 {
		return nil, fmt.Errorf("invalid config format: no apps")
	}
	return &multi, nil
}

// SaveMultiAppConfig saves config to disk.
//
// Stamps SchemaVersion = CurrentSchemaVersion if lower or zero. Never
// downgrades a higher SchemaVersion — only the migrator may touch the
// version field downward.
func SaveMultiAppConfig(config *MultiAppConfig) error {
	dir := GetConfigDir()
	if err := vfs.MkdirAll(dir, 0700); err != nil {
		return err
	}
	if config.SchemaVersion < CurrentSchemaVersion {
		config.SchemaVersion = CurrentSchemaVersion
	}
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	return validate.AtomicWrite(GetConfigPath(), append(data, '\n'), 0600)
}

// RequireConfig loads the single-app config using the default profile resolution.
// Backward-compatible thin forwarder.
func RequireConfig(kc keychain.KeychainAccess) (*CliConfig, error) {
	return RequireConfigForProfileAndUser(kc, "", "")
}

// RequireConfigForProfile loads the single-app config for a specific profile.
// Resolution priority: profileOverride > config.CurrentApp > Apps[0].
// Backward-compatible thin forwarder around the *AndUser entry point.
func RequireConfigForProfile(kc keychain.KeychainAccess, profileOverride string) (*CliConfig, error) {
	return RequireConfigForProfileAndUser(kc, profileOverride, "")
}

// RequireConfigForProfileAndUser is the multi-user-aware variant of
// RequireConfigForProfile. userOverride is the resolved user identity
// (--user / LARKSUITE_CLI_OPEN_ID); empty means "no override".
func RequireConfigForProfileAndUser(kc keychain.KeychainAccess, profileOverride, userOverride string) (*CliConfig, error) {
	raw, err := LoadMultiAppConfig()
	if err != nil {
		return nil, PassThroughOrNotConfigured(err)
	}
	if raw == nil || len(raw.Apps) == 0 {
		return nil, NotConfiguredError()
	}
	return ResolveConfigFromMulti(raw, kc, profileOverride, userOverride)
}

// ResolveConfigFromMulti resolves a single-app config from an already-loaded
// MultiAppConfig. User selection follows a three-rung fallback:
//
//	rung 1: userOverride (--user / LARKSUITE_CLI_OPEN_ID)
//	rung 2: AppConfig.CurrentUser (set by `auth users use`)
//	rung 3: Users[0] (legacy single-user compatibility)
//
// Both explicit-selector rungs ERROR on miss rather than silently
// falling through — preventing wrong-user data leaks (e.g. `--user
// ou_alice` against a profile holding only ou_bob must not dispatch
// as ou_bob). Only the empty/empty case picks Users[0].
//
// userOverride matches via FindUser (UserOpenId-first, then UserName).
func ResolveConfigFromMulti(raw *MultiAppConfig, kc keychain.KeychainAccess, profileOverride, userOverride string) (*CliConfig, error) {
	app := raw.CurrentAppConfig(profileOverride)
	if app == nil {
		return nil, &ConfigError{
			Code:    3,
			Type:    "config",
			Message: fmt.Sprintf("profile %q not found", profileOverride),
			Hint:    fmt.Sprintf("available profiles: %s", formatProfileNames(raw.ProfileNames())),
			Rung:    RungProfile,
		}
	}

	if err := ValidateSecretKeyMatch(app.AppId, app.AppSecret); err != nil {
		return nil, &ConfigError{Code: 3, Type: "config",
			Message: "appId and appSecret keychain key are out of sync",
			Hint:    err.Error()}
	}

	secret, err := ResolveSecretInput(app.AppSecret, kc)
	if err != nil {
		// Deprecated: legacy *output.ExitError passthrough; removed after typed migration.
		var exitErr *output.ExitError
		if errors.As(err, &exitErr) {
			return nil, exitErr
		}
		return nil, &ConfigError{Code: 3, Type: "config", Message: err.Error()}
	}
	cfg := &CliConfig{
		ProfileName: app.ProfileName(),
		AppID:       app.AppId,
		AppSecret:   secret,
		Brand:       app.Brand,
		DefaultAs:   app.DefaultAs,
		Lang:        app.Lang,
	}

	// Three-rung user fallback. Both explicit-selector rungs error
	// rather than fall through; only empty/empty picks Users[0].
	profile := app.ProfileName()
	var picked *AppUser
	switch {
	case userOverride != "":
		picked = app.FindUser(userOverride)
		if picked == nil {
			return nil, userResolutionError(profile, userOverride, app.Users, false /* drift */)
		}
	case app.CurrentUser != "":
		picked = app.FindUser(app.CurrentUser)
		if picked == nil {
			return nil, userResolutionError(profile, app.CurrentUser, app.Users, true /* drift */)
		}
	case len(app.Users) > 0:
		picked = &app.Users[0]
	}
	if picked != nil {
		cfg.UserOpenId = picked.UserOpenId
		cfg.UserName = picked.UserName
	}
	return cfg, nil
}

// ResolveProfileConfigForLogin resolves the profile-rung config WITHOUT
// enforcing the strict user-rung selector. It is the entry point for
// `auth login` (and any future "add a user to a profile" command),
// where the operator-supplied --user / env may legitimately name a
// brand-new open_id that is not in app.Users yet.
//
// Concretely: ResolveConfigFromMulti errors when --user names an
// unknown user (correct for "use this user to make an API call");
// here that strictness is wrong (the very point is to ADD that user).
// We return the AppId/AppSecret/Brand/Lang/DefaultAs fields needed to
// drive the device flow; UserOpenId/UserName are left to the caller's
// post-authorization holder verification (cmd/auth/login_holder.go).
//
// Profile-rung errors still surface verbatim (typed *ConfigError with
// RungProfile) so the dispatcher routes profile typos as InvalidArgument
// rather than "not configured".
func ResolveProfileConfigForLogin(raw *MultiAppConfig, kc keychain.KeychainAccess, profileOverride string) (*CliConfig, error) {
	app := raw.CurrentAppConfig(profileOverride)
	if app == nil {
		return nil, &ConfigError{
			Code:    3,
			Type:    "config",
			Message: fmt.Sprintf("profile %q not found", profileOverride),
			Hint:    fmt.Sprintf("available profiles: %s", formatProfileNames(raw.ProfileNames())),
			Rung:    RungProfile,
		}
	}

	if err := ValidateSecretKeyMatch(app.AppId, app.AppSecret); err != nil {
		return nil, &ConfigError{Code: 3, Type: "config",
			Message: "appId and appSecret keychain key are out of sync",
			Hint:    err.Error()}
	}

	secret, err := ResolveSecretInput(app.AppSecret, kc)
	if err != nil {
		var exitErr *output.ExitError
		if errors.As(err, &exitErr) {
			return nil, exitErr
		}
		return nil, &ConfigError{Code: 3, Type: "config", Message: err.Error()}
	}
	return &CliConfig{
		ProfileName: app.ProfileName(),
		AppID:       app.AppId,
		AppSecret:   secret,
		Brand:       app.Brand,
		DefaultAs:   app.DefaultAs,
		Lang:        app.Lang,
		// UserOpenId / UserName intentionally left empty; login resolves
		// them post-authorization via the upstream open_id, then
		// verifyHolder reconciles against --user / env / CurrentUser.
	}, nil
}

// RequireAuth loads config and ensures a user is logged in.
// Backward-compatible thin forwarder; the user-selection chain now
// honours AppConfig.CurrentUser when set by `auth users use`.
func RequireAuth(kc keychain.KeychainAccess) (*CliConfig, error) {
	return RequireAuthForProfileAndUser(kc, "", "")
}

// RequireAuthForProfile loads config for a profile and ensures a user is logged in.
// Backward-compatible thin forwarder around the *AndUser entry point.
func RequireAuthForProfile(kc keychain.KeychainAccess, profileOverride string) (*CliConfig, error) {
	return RequireAuthForProfileAndUser(kc, profileOverride, "")
}

// RequireAuthForProfileAndUser loads config for a profile + user and
// ensures a user is logged in. The not-logged-in error envelope
// (Code=3, Type="auth") is unchanged from RequireAuthForProfile so
// operator runbook greps keep working.
func RequireAuthForProfileAndUser(kc keychain.KeychainAccess, profileOverride, userOverride string) (*CliConfig, error) {
	cfg, err := RequireConfigForProfileAndUser(kc, profileOverride, userOverride)
	if err != nil {
		return nil, err
	}
	if cfg.UserOpenId == "" {
		return nil, &ConfigError{Code: 3, Type: "auth", Message: "not logged in", Hint: "run `lark-cli auth login` in the background. It blocks and outputs a verification URL — retrieve the URL and open it in a browser to complete login."}
	}
	return cfg, nil
}

// userResolutionError builds the override-miss / current-user-stale
// ConfigError. Single helper so both rungs share hint formatting.
//
// drift=false: operator passed --user / env that does not match
// anyone in Users[]. drift=true: AppConfig.CurrentUser names a user
// no longer in Users[] (config.json hand-edited or logout removed
// the user) — the hint also offers a one-shot --user override.
func userResolutionError(profile, requested string, users []AppUser, drift bool) error {
	available := formatUserDisplay(users)
	if drift {
		return &ConfigError{
			Code: 3, Type: "config",
			Message: fmt.Sprintf("current user %q in profile %q is no longer present in users list", requested, profile),
			Hint:    fmt.Sprintf("this usually means config.json was hand-edited or a logout removed the user; pass --user <name|open_id> to override (available: %s), or run `lark-cli auth login` to re-establish the user", available),
			Rung:    RungUser,
		}
	}
	return &ConfigError{
		Code: 3, Type: "config",
		Message: fmt.Sprintf("user %q not found in profile %q", requested, profile),
		Hint:    fmt.Sprintf("available users in this profile: %s; run `lark-cli auth login` to add a new user, or `lark-cli auth users list` to see all known users", available),
		Rung:    RungUser,
	}
}

// formatProfileNames joins profile names for display.
func formatProfileNames(names []string) string {
	if len(names) == 0 {
		return "(none)"
	}
	return strings.Join(names, ", ")
}

// formatUserDisplay renders one line per AppUser as "name (open_id-prefix)".
// open_ids longer than 12 characters are truncated with "…" so hints
// stay terminal-readable. Returns "(none)" for an empty slice.
func formatUserDisplay(users []AppUser) string {
	if len(users) == 0 {
		return "(none)"
	}
	parts := make([]string, 0, len(users))
	for _, u := range users {
		prefix := u.UserOpenId
		if len(prefix) > 12 {
			prefix = prefix[:12] + "…"
		}
		if u.UserName != "" {
			parts = append(parts, fmt.Sprintf("%s (%s)", u.UserName, prefix))
		} else {
			parts = append(parts, prefix)
		}
	}
	return strings.Join(parts, ", ")
}
