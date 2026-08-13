// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build extended

package extendedupdate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/update"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/internal/vfs"
)

const (
	extendedStateFile = "update-state-extended.json"
	extendedCacheTTL  = 24 * time.Hour
)

type cachedRelease struct {
	LatestVersion string `json:"latest_version"`
	CheckedAt     int64  `json:"checked_at"`
}

// CheckCached reads only the Extended release cache. Standard and Extended
// deliberately use different files so one edition can never advertise the
// other edition's release channel.
func CheckCached(currentVersion string) *update.UpdateInfo {
	if skipUpdateNotice(currentVersion) {
		return nil
	}
	state, err := loadCachedRelease()
	if err != nil || state.LatestVersion == "" ||
		!update.IsNewer(state.LatestVersion, currentVersion) {
		return nil
	}
	return &update.UpdateInfo{Current: currentVersion, Latest: state.LatestVersion}
}

// RefreshCache refreshes the Extended GitHub-release cache when stale. It is
// intentionally best-effort because callers run it from the notice goroutine.
func RefreshCache(currentVersion string) {
	if skipUpdateNotice(currentVersion) {
		return
	}
	state, _ := loadCachedRelease()
	if state != nil && time.Since(time.Unix(state.CheckedAt, 0)) < extendedCacheTTL {
		return
	}
	latest, err := FetchLatest()
	if err != nil {
		return
	}
	_ = saveCachedRelease(&cachedRelease{
		LatestVersion: latest,
		CheckedAt:     time.Now().Unix(),
	})
}

func skipUpdateNotice(version string) bool {
	if os.Getenv("LARKSUITE_CLI_NO_UPDATE_NOTIFIER") != "" || update.IsCIEnv() {
		return true
	}
	return !update.IsRelease(version)
}

func extendedStatePath() string {
	return filepath.Join(core.GetConfigDir(), extendedStateFile)
}

func loadCachedRelease() (*cachedRelease, error) {
	data, err := vfs.ReadFile(extendedStatePath())
	if err != nil {
		return nil, err
	}
	var state cachedRelease
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	return &state, nil
}

func saveCachedRelease(state *cachedRelease) error {
	dir := core.GetConfigDir()
	if err := vfs.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return validate.AtomicWrite(extendedStatePath(), data, 0o600)
}
