// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/distribution"
	"github.com/larksuite/cli/internal/transport"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/internal/versioncheck"
	"github.com/larksuite/cli/internal/vfs"
)

const (
	registryURL  = "https://registry.npmjs.org/@larksuite/cli/latest"
	cacheTTL     = 24 * time.Hour
	fetchTimeout = 15 * time.Second
	stateFile    = "update-state.json"
	maxBody      = 256 << 10 // 256 KB

)

// UpdateInfo holds version update information.
type UpdateInfo struct {
	Current string `json:"current"`
	Latest  string `json:"latest"`
	Source  string `json:"source,omitempty"`
}

// Message returns a concise update notification including the canonical
// fix command. Aligned with skillscheck.StaleNotice.Message style so
// AI agents can parse a unified "run: lark-cli update" hint across
// both notice types.
func (u *UpdateInfo) Message() string {
	if u.Source != "" {
		return fmt.Sprintf("lark-cli target %s configured, current %s, run: lark-cli update", u.Latest, u.Current)
	}
	return fmt.Sprintf("lark-cli %s available, current %s, run: lark-cli update", u.Latest, u.Current)
}

// pending stores the latest update info for the current process.
var pending atomic.Pointer[UpdateInfo]

// SetPending stores the update info for consumption by output decorators.
func SetPending(info *UpdateInfo) { pending.Store(info) }

// GetPending returns the pending update info, or nil.
func GetPending() *UpdateInfo { return pending.Load() }

// DefaultClient is the HTTP client used for npm registry requests.
// Override in tests with an httptest server client.
var DefaultClient *http.Client

func httpClient() *http.Client {
	if DefaultClient != nil {
		return DefaultClient
	}
	return transport.NewExternalHTTPClient(fetchTimeout)
}

// updateState is persisted to disk for caching.
type updateState struct {
	LatestVersion string `json:"latest_version"`
	CheckedAt     int64  `json:"checked_at"`
	Source        string `json:"source,omitempty"`
}

// CheckCached checks the local cache only (no network). Always fast.
func CheckCached(currentVersion string) *UpdateInfo {
	manifestURL, manifestMode, sourceErr := distribution.ResolveManifestURL(context.Background())
	if sourceErr != nil {
		return nil
	}
	if shouldSkipForMode(currentVersion, manifestMode) {
		return nil
	}
	state, _ := loadState()
	if state == nil || state.LatestVersion == "" {
		return nil
	}
	if manifestMode {
		if state.Source != manifestSourceKey(manifestURL) || state.LatestVersion == currentVersion {
			return nil
		}
		return &UpdateInfo{Current: currentVersion, Latest: state.LatestVersion, Source: "manifest"}
	}
	if state.Source != "" || !IsNewer(state.LatestVersion, currentVersion) {
		return nil
	}
	return &UpdateInfo{Current: currentVersion, Latest: state.LatestVersion}
}

// RefreshCache fetches the configured target and updates the local cache.
// No-op if the cache is still fresh (< 24h). Safe to call from a goroutine.
func RefreshCache(currentVersion string) {
	manifestURL, manifestMode, sourceErr := distribution.ResolveManifestURL(context.Background())
	if sourceErr != nil {
		return
	}
	if shouldSkipForMode(currentVersion, manifestMode) {
		return
	}
	state, _ := loadState()
	identityMatches := !manifestMode && state != nil && state.Source == ""
	if manifestMode {
		identityMatches = state != nil && state.Source == manifestSourceKey(manifestURL)
	}
	if identityMatches && time.Since(time.Unix(state.CheckedAt, 0)) < cacheTTL {
		return // cache is fresh
	}
	target, err := fetchTarget(context.Background(), manifestURL, manifestMode)
	if err != nil {
		return
	}
	sourceKey := ""
	if manifestMode {
		sourceKey = manifestSourceKey(manifestURL)
	}
	_ = saveState(&updateState{
		LatestVersion: target.Version,
		CheckedAt:     time.Now().Unix(),
		Source:        sourceKey,
	})
}

func manifestSourceKey(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return "manifest:" + hex.EncodeToString(sum[:])
}

func shouldSkipForMode(version string, manifestMode bool) bool {
	if manifestMode {
		if os.Getenv("LARKSUITE_CLI_NO_UPDATE_NOTIFIER") != "" || IsCIEnv() {
			return true
		}
		return version == ""
	}
	return shouldSkip(version)
}

func shouldSkip(version string) bool {
	if os.Getenv("LARKSUITE_CLI_NO_UPDATE_NOTIFIER") != "" {
		return true
	}
	// Suppress in CI environments.
	if IsCIEnv() {
		return true
	}
	// No version info at all — can't compare.
	if version == "DEV" || version == "dev" || version == "" {
		return true
	}
	// Skip local dev builds (e.g. v1.0.0-12-g9b933f1-dirty from git describe).
	// Only released versions (clean X.Y.Z) should check for updates.
	if !isRelease(version) {
		return true
	}
	return false
}

// isRelease returns true for published versions: clean semver (1.0.0)
// and npm prerelease (1.0.0-beta.1, 1.0.0-rc.1).
// Returns false for git describe dev builds (v1.0.0-12-g9b933f1-dirty).
func isRelease(version string) bool { return versioncheck.IsRelease(version) }

// IsRelease reports whether version looks like a clean published release
// (semver "1.0.0", or npm prerelease "1.0.0-beta.1") and not a git-describe
// dev build like "1.0.0-12-g9b933f1-dirty". Exported so internal/skillscheck
// can apply the same release-only gating without duplicating the regex.
func IsRelease(version string) bool { return isRelease(version) }

// IsCIEnv returns true when any of the standard CI environment variables
// is set. Exported for internal/skillscheck so its skip rules track the
// same CI-suppression behavior as the update notifier.
func IsCIEnv() bool {
	return versioncheck.IsCIEnv()
}

// --- state file I/O ---

func statePath() string {
	return filepath.Join(core.GetConfigDir(), stateFile)
}

func loadState() (*updateState, error) {
	data, err := vfs.ReadFile(statePath())
	if err != nil {
		return nil, err
	}
	var s updateState
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func saveState(s *updateState) error {
	dir := core.GetConfigDir()
	if err := vfs.MkdirAll(dir, 0700); err != nil {
		return err
	}
	data, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return validate.AtomicWrite(statePath(), data, 0644)
}

// Target describes the active source's desired CLI version.
type Target struct {
	Version string
	Exact   bool
}

// Available reports whether the target should be offered for current.
func (t Target) Available(current string) bool {
	if t.Exact {
		return t.Version != "" && t.Version != current
	}
	return IsNewer(t.Version, current)
}

// FetchTarget synchronously queries the active update source. It is intended
// for explicit checks such as update and doctor.
func FetchTarget() (Target, error) {
	manifestURL, manifestMode, err := distribution.ResolveManifestURL(context.Background())
	if err != nil {
		return Target{}, err
	}
	return fetchTarget(context.Background(), manifestURL, manifestMode)
}

func fetchTarget(ctx context.Context, manifestURL string, manifestMode bool) (Target, error) {
	if manifestMode {
		manifest, err := distribution.FetchManifest(ctx, manifestURL)
		if err != nil {
			return Target{}, err
		}
		return Target{Version: manifest.Version, Exact: true}, nil
	}
	latest, err := fetchLatestVersion()
	if err != nil {
		return Target{}, err
	}
	return Target{Version: latest}, nil
}

// --- npm registry ---

type npmLatestResponse struct {
	Version string `json:"version"`
}

func fetchLatestVersion() (string, error) {
	resp, err := httpClient().Get(registryURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("npm registry: HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return "", err
	}

	var result npmLatestResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return "", err
	}
	if result.Version == "" {
		return "", fmt.Errorf("npm registry: empty version")
	}
	return result.Version, nil
}

// --- semver helpers ---

// IsNewer returns true if version a should be considered an update over b.
//
// When both parse as semver, standard comparison applies.
// When b cannot be parsed (e.g. bare commit hash "9b933f1"), any valid a
// is considered newer — an unparseable local version is assumed outdated.
// When a cannot be parsed, returns false (can't confirm it's newer).
func IsNewer(a, b string) bool {
	return versioncheck.IsNewer(a, b)
}

// ParseVersion parses "X.Y.Z" (with optional "v" prefix and pre-release suffix)
// into [major, minor, patch]. Returns nil on invalid input.
func ParseVersion(v string) []int {
	return versioncheck.Parse(v)
}
