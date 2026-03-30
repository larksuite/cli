// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package update

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/larksuite/cli/internal/build"
	"github.com/larksuite/cli/internal/core"
)

const (
	// checkInterval is the minimum time between remote version checks.
	checkInterval = 24 * time.Hour

	// registryURL is the npm registry endpoint for the latest version.
	registryURL = "https://registry.npmjs.org/@larksuite/cli/latest"

	// stateFile is the file name for persisting last-check state.
	stateFile = "update-state.json"
)

// state persists between CLI invocations to avoid checking on every run.
type state struct {
	CheckedAt     time.Time `json:"checked_at"`
	LatestVersion string    `json:"latest_version"`
}

// Result holds the outcome of a version check.
type Result struct {
	Current string
	Latest  string
}

// IsNewer returns true if the latest version is newer than current.
func (r *Result) IsNewer() bool {
	return r != nil && r.Latest != "" && r.Latest != r.Current && compareVersions(r.Latest, r.Current) > 0
}

// UpdateCommand returns the recommended update command string.
func (r *Result) UpdateCommand() string {
	return "npm update -g @larksuite/cli && npx skills add larksuite/cli --all -y"
}

// CheckForUpdate checks whether a newer version is available.
// It reads cached state to avoid hitting the network on every invocation.
// Returns nil if no update is available, the version is DEV, or checking is disabled.
func CheckForUpdate(ctx context.Context, httpClient *http.Client) *Result {
	if isDisabled() {
		return nil
	}
	current := build.Version
	if current == "DEV" || current == "" {
		return nil
	}

	stPath := statePath()

	// Try to use cached state first.
	if s, err := readState(stPath); err == nil {
		if time.Since(s.CheckedAt) < checkInterval {
			r := &Result{Current: current, Latest: s.LatestVersion}
			if r.IsNewer() {
				return r
			}
			return nil
		}
	}

	// Fetch latest version from GitHub.
	latest, err := fetchLatestVersion(ctx, httpClient)
	if err != nil {
		return nil
	}

	// Persist state.
	_ = writeState(stPath, &state{
		CheckedAt:     time.Now(),
		LatestVersion: latest,
	})

	r := &Result{Current: current, Latest: latest}
	if r.IsNewer() {
		return r
	}
	return nil
}

// isDisabled returns true if the user has opted out of update checks.
func isDisabled() bool {
	if v := os.Getenv("LARKSUITE_CLI_NO_UPDATE_NOTIFIER"); v != "" {
		return true
	}
	// Suppress in common CI environments.
	for _, key := range []string{"CI", "BUILD_NUMBER", "RUN_ID"} {
		if os.Getenv(key) != "" {
			return true
		}
	}
	return false
}

func statePath() string {
	return filepath.Join(core.GetConfigDir(), stateFile)
}

func readState(path string) (*state, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var s state
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func writeState(path string, s *state) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	data, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

// npmPackage is the subset of npm registry JSON we care about.
type npmPackage struct {
	Version string `json:"version"`
}

func fetchLatestVersion(ctx context.Context, httpClient *http.Client) (string, error) {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 5 * time.Second}
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, registryURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("npm registry: status %d", resp.StatusCode)
	}

	var pkg npmPackage
	if err := json.NewDecoder(resp.Body).Decode(&pkg); err != nil {
		return "", err
	}
	return pkg.Version, nil
}

// compareVersions compares two semver strings (without "v" prefix).
// Returns >0 if a > b, <0 if a < b, 0 if equal.
func compareVersions(a, b string) int {
	ap := parseVersion(a)
	bp := parseVersion(b)
	for i := 0; i < 3; i++ {
		if ap[i] != bp[i] {
			return ap[i] - bp[i]
		}
	}
	return 0
}

func parseVersion(v string) [3]int {
	v = strings.TrimPrefix(v, "v")
	parts := strings.SplitN(v, ".", 3)
	var result [3]int
	for i := 0; i < len(parts) && i < 3; i++ {
		// Strip pre-release suffix (e.g. "1-beta" → "1").
		num := strings.SplitN(parts[i], "-", 2)[0]
		result[i], _ = strconv.Atoi(num)
	}
	return result
}
