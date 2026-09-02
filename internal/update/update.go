// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package update

import (
	"context"
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
	"github.com/larksuite/cli/internal/urlrewrite"
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
	src, err := distribution.ResolveSource(context.Background())
	if err != nil || shouldSkip(currentVersion, src.ManifestMode()) {
		return nil
	}
	state, _ := loadState()
	if state == nil || state.LatestVersion == "" || state.Source != src.Identity() {
		return nil
	}
	if src.ManifestMode() {
		if state.LatestVersion == currentVersion {
			return nil
		}
		return &UpdateInfo{Current: currentVersion, Latest: state.LatestVersion, Source: "manifest"}
	}
	if !versioncheck.IsNewer(state.LatestVersion, currentVersion) {
		return nil
	}
	return &UpdateInfo{Current: currentVersion, Latest: state.LatestVersion}
}

// RefreshCache fetches the configured target and updates the local cache.
// No-op if the cache is still fresh (< 24h). Safe to call from a goroutine.
func RefreshCache(currentVersion string) {
	src, err := distribution.ResolveSource(context.Background())
	if err != nil || shouldSkip(currentVersion, src.ManifestMode()) {
		return
	}
	state, _ := loadState()
	if state != nil && state.Source == src.Identity() && time.Since(time.Unix(state.CheckedAt, 0)) < cacheTTL {
		return // cache is fresh
	}
	version, fetchErr := fetchTargetVersion(context.Background(), src)
	if fetchErr != nil {
		return
	}
	_ = saveState(&updateState{
		LatestVersion: version,
		CheckedAt:     time.Now().Unix(),
		Source:        src.Identity(),
	})
}

// shouldSkip suppresses the notifier in CI, when opted out, or without a
// usable version. The npm flow additionally only tracks published releases;
// a manifest distribution may target development builds.
func shouldSkip(version string, manifestMode bool) bool {
	if os.Getenv("LARKSUITE_CLI_NO_UPDATE_NOTIFIER") != "" || versioncheck.IsCIEnv() || version == "" {
		return true
	}
	if manifestMode {
		return false
	}
	// Skip local dev builds (e.g. v1.0.0-12-g9b933f1-dirty from git describe).
	return version == "DEV" || version == "dev" || !versioncheck.IsRelease(version)
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
	return versioncheck.IsNewer(t.Version, current)
}

// FetchTarget synchronously queries the active update source. It is intended
// for explicit checks such as update and doctor.
func FetchTarget() (Target, error) {
	ctx := context.Background()
	src, err := distribution.ResolveSource(ctx)
	if err != nil {
		return Target{}, err
	}
	version, fetchErr := fetchTargetVersion(ctx, src)
	if fetchErr != nil {
		return Target{}, fetchErr
	}
	return Target{Version: version, Exact: src.ManifestMode()}, nil
}

func fetchTargetVersion(ctx context.Context, src distribution.Source) (string, error) {
	if src.ManifestMode() {
		manifest, err := src.FetchManifest(ctx)
		if err != nil {
			return "", err
		}
		return manifest.Version, nil
	}
	return fetchLatestVersion()
}

// --- npm registry ---

type npmLatestResponse struct {
	Version string `json:"version"`
}

func fetchLatestVersion() (string, error) {
	resp, err := httpClient().Get(urlrewrite.Rewrite(registryURL))
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
