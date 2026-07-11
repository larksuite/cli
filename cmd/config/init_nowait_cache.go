// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package config

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/internal/vfs"
)

// initNoWaitCacheVersion is the schema version of the cached init context.
// Bump it when the record shape changes so stale entries are ignored.
const initNoWaitCacheVersion = 1

// initNoWaitRecord is the context persisted by `config init --new --no-wait` so
// that the later `--device-code` step can complete the app creation. It must
// never hold a secret, verification URL, or full config — only what the resume
// step needs to finish persisting the new app.
type initNoWaitRecord struct {
	Version      int    `json:"version"`
	Brand        string `json:"brand"`
	ProfileName  string `json:"profile_name"`
	Lang         string `json:"lang"`
	LangExplicit bool   `json:"lang_explicit"`
	Interval     int    `json:"interval"`
	ExpiresAt    int64  `json:"expires_at"` // unix seconds; absolute device-code deadline
	ConfigDigest string `json:"config_digest"`
}

// initNoWaitCacheDir returns the directory used to persist config init
// --no-wait context keyed by device_code.
func initNoWaitCacheDir() string {
	return filepath.Join(core.GetConfigDir(), "cache", "config_init_nowait")
}

// initNoWaitCachePath returns the cache file path for a given device_code.
func initNoWaitCachePath(deviceCode string) string {
	return filepath.Join(initNoWaitCacheDir(), initNoWaitCacheKey(deviceCode)+".json")
}

// initNoWaitCacheKey derives a collision-free, filesystem-safe filename token
// from an opaque device_code. A sha256 hex digest avoids the collisions a
// character-replacement sanitizer would cause (e.g. "a/b" and "a:b" both
// mapping to "a_b").
func initNoWaitCacheKey(deviceCode string) string {
	sum := sha256.Sum256([]byte(deviceCode))
	return hex.EncodeToString(sum[:])
}

// saveInitNoWaitRecord persists the resume context for a device_code.
func saveInitNoWaitRecord(deviceCode string, rec initNoWaitRecord) error {
	if err := vfs.MkdirAll(initNoWaitCacheDir(), 0700); err != nil {
		return err
	}
	data, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	return validate.AtomicWrite(initNoWaitCachePath(deviceCode), data, 0600)
}

// loadInitNoWaitRecord loads the resume context for a device_code. It returns
// (nil, nil) when no cache entry exists.
func loadInitNoWaitRecord(deviceCode string) (*initNoWaitRecord, error) {
	data, err := vfs.ReadFile(initNoWaitCachePath(deviceCode))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var rec initNoWaitRecord
	if err := json.Unmarshal(data, &rec); err != nil {
		_ = vfs.Remove(initNoWaitCachePath(deviceCode))
		return nil, err
	}
	if rec.Version != initNoWaitCacheVersion {
		_ = vfs.Remove(initNoWaitCachePath(deviceCode))
		return nil, nil
	}
	return &rec, nil
}

// removeInitNoWaitRecord deletes the cache entry for a device_code.
func removeInitNoWaitRecord(deviceCode string) error {
	err := vfs.Remove(initNoWaitCachePath(deviceCode))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// computeConfigDigest returns a stable digest of the existing config so the
// resume step can detect drift between initiation and completion. The digest
// is a hash of config.json content (app IDs, brands, users, secret references)
// — it contains no plaintext secret and is safe to cache. A nil config and an
// (unexpected) marshal error both map to the empty digest.
func computeConfigDigest(existing *core.MultiAppConfig) string {
	if existing == nil {
		return ""
	}
	data, err := json.Marshal(existing)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
