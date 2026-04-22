// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package update

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/util"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/internal/vfs"
)

const (
	registryURL  = "https://registry.npmjs.org/@larksuite/cli/latest"
	cacheTTL     = 24 * time.Hour
	fetchTimeout = 5 * time.Second
	stateFile    = "update-state.json"
	maxBody      = 256 << 10 // 256 KB

)

// UpdateInfo holds version update information.
type UpdateInfo struct {
	Current string `json:"current"`
	Latest  string `json:"latest"`
}

// Message returns a concise update notification.
func (u *UpdateInfo) Message() string {
	return fmt.Sprintf("lark-cli %s available, current %s", u.Latest, u.Current)
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
	return &http.Client{
		Timeout:   fetchTimeout,
		Transport: util.SharedTransport(),
	}
}

// updateState is persisted to disk for caching.
type updateState struct {
	LatestVersion string `json:"latest_version"`
	CheckedAt     int64  `json:"checked_at"`
}

// CheckCached checks the local cache only (no network). Always fast.
func CheckCached(currentVersion string) *UpdateInfo {
	if shouldSkip(currentVersion) {
		return nil
	}
	state, _ := loadState()
	if state == nil || state.LatestVersion == "" {
		return nil
	}
	if !IsNewer(state.LatestVersion, currentVersion) {
		return nil
	}
	return &UpdateInfo{Current: currentVersion, Latest: state.LatestVersion}
}

// RefreshCache fetches the latest version from npm and updates the local cache.
// No-op if the cache is still fresh (< 24h). Safe to call from a goroutine.
func RefreshCache(currentVersion string) {
	if shouldSkip(currentVersion) {
		return
	}
	state, _ := loadState()
	if state != nil && time.Since(time.Unix(state.CheckedAt, 0)) < cacheTTL {
		return // cache is fresh
	}
	latest, err := fetchLatestVersion()
	if err != nil {
		return
	}
	_ = saveState(&updateState{
		LatestVersion: latest,
		CheckedAt:     time.Now().Unix(),
	})
}

func shouldSkip(version string) bool {
	if os.Getenv("LARKSUITE_CLI_NO_UPDATE_NOTIFIER") != "" {
		return true
	}
	// Suppress in CI environments.
	for _, key := range []string{"CI", "BUILD_NUMBER", "RUN_ID"} {
		if os.Getenv(key) != "" {
			return true
		}
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
var gitDescribePattern = regexp.MustCompile(`-\d+-g[0-9a-f]{7,}`)

func isRelease(version string) bool {
	v := strings.TrimPrefix(version, "v")
	if ParseVersion(v) == nil {
		return false
	}
	return !gitDescribePattern.MatchString(v)
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

// FetchLatest queries the npm registry and returns the latest published version.
// This is a synchronous call with timeout, intended for diagnostic commands (doctor).
func FetchLatest() (string, error) {
	return fetchLatestVersion()
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
	ap := parseVersionDetail(a)
	bp := parseVersionDetail(b)
	if ap == nil {
		return false // can't confirm remote is newer
	}
	if bp == nil {
		return true // local version unparseable → assume outdated
	}
	for i := 0; i < 3; i++ {
		if ap.core[i] > bp.core[i] {
			return true
		}
		if ap.core[i] < bp.core[i] {
			return false
		}
	}
	return comparePrerelease(ap.prerelease, bp.prerelease) > 0
}

// ParseVersion parses "X.Y.Z" (with optional "v" prefix and pre-release suffix)
// into [major, minor, patch]. Returns nil on invalid input.
func ParseVersion(v string) []int {
	parsed := parseVersionDetail(v)
	if parsed == nil {
		return nil
	}
	return []int{parsed.core[0], parsed.core[1], parsed.core[2]}
}

type parsedVersion struct {
	core       [3]int
	prerelease string
}

// validPrerelease matches semver pre-release identifiers (dot-separated).
// Each identifier is either: "0", a non-zero-leading numeric, or alphanumeric with at least one letter/hyphen.
// Rejects empty identifiers ("1.0.0-"), leading-zero numerics ("1.0.0-01"), etc.
var validPrerelease = regexp.MustCompile(
	`^(?:0|[1-9]\d*|[0-9]*[a-zA-Z-][0-9a-zA-Z-]*)` +
		`(?:\.(?:0|[1-9]\d*|[0-9]*[a-zA-Z-][0-9a-zA-Z-]*))*$`)

func parseVersionDetail(v string) *parsedVersion {
	v = strings.TrimPrefix(v, "v")
	if idx := strings.Index(v, "+"); idx >= 0 {
		v = v[:idx]
	}
	prerelease := ""
	if idx := strings.Index(v, "-"); idx >= 0 {
		prerelease = v[idx+1:]
		v = v[:idx]
		if prerelease == "" || !validPrerelease.MatchString(prerelease) {
			return nil
		}
	}
	parts := strings.SplitN(v, ".", 3)
	if len(parts) != 3 {
		return nil
	}
	var nums [3]int
	for i, p := range parts {
		if len(p) > 1 && p[0] == '0' {
			return nil // leading zero in core part (e.g. "01.0.0")
		}
		n, err := strconv.Atoi(p)
		if err != nil {
			return nil
		}
		nums[i] = n
	}
	return &parsedVersion{core: nums, prerelease: prerelease}
}

func comparePrerelease(a, b string) int {
	if a == "" && b == "" {
		return 0
	}
	if a == "" {
		return 1
	}
	if b == "" {
		return -1
	}
	ap := strings.Split(a, ".")
	bp := strings.Split(b, ".")
	for i := 0; i < len(ap) && i < len(bp); i++ {
		cmp := comparePrereleaseIdentifier(ap[i], bp[i])
		if cmp != 0 {
			return cmp
		}
	}
	switch {
	case len(ap) > len(bp):
		return 1
	case len(ap) < len(bp):
		return -1
	default:
		return 0
	}
}

func comparePrereleaseIdentifier(a, b string) int {
	an, aErr := strconv.Atoi(a)
	bn, bErr := strconv.Atoi(b)
	aNumeric := aErr == nil
	bNumeric := bErr == nil
	switch {
	case aNumeric && bNumeric:
		if an > bn {
			return 1
		}
		if an < bn {
			return -1
		}
		return 0
	case aNumeric:
		return -1
	case bNumeric:
		return 1
	default:
		return strings.Compare(a, b)
	}
}
