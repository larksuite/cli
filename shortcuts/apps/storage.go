// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"errors"
	"net/url"
	"os"
	"path/filepath"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/internal/vfs" //nolint:depguard // existing apps storage persists CLI config-dir state; it is not user file I/O.
)

// storageRoot is the per-domain local-storage directory name under the config dir.
const storageRoot = "spark"

// checkSeg validates a value used as a single path segment (appID or key).
// It rejects empty, "..", "." , URL metacharacters, control and dangerous
// Unicode via validate.ResourceName — defense-in-depth alongside the
// EncodePathSegment escaping applied when building the path, so neither value
// can traverse out of the storage directory.
func checkSeg(name, what string) error {
	if err := validate.ResourceName(name, what); err != nil {
		return appsStorageError(err, "apps storage: %v", err)
	}
	if name == "." {
		return errs.NewInternalError(errs.SubtypeStorage, "apps storage: %s must not be \".\"", what)
	}
	return nil
}

// appDir returns the storage directory for one app: ~/.lark-cli/spark/<esc(appID)>/
// (workspace-aware).
func appDir(appID string) string {
	return filepath.Join(core.GetConfigDir(), storageRoot, validate.EncodePathSegment(appID))
}

// appKeyPath returns the file path for one (appID, key).
func appKeyPath(appID, key string) string {
	return filepath.Join(appDir(appID), validate.EncodePathSegment(key))
}

// Read returns the bytes stored under (appID, key). A missing file returns
// (nil, nil). Content is opaque — callers own the format. Note: an empty stored
// value is indistinguishable from a missing key (both yield nil), so this store
// is unsuitable as an existence flag.
func Read(appID, key string) ([]byte, error) {
	if err := checkSeg(appID, "appID"); err != nil {
		return nil, err
	}
	if err := checkSeg(key, "key"); err != nil {
		return nil, err
	}
	data, err := vfs.ReadFile(appKeyPath(appID, key))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, appsStorageError(err, "apps storage: read: %v", err)
	}
	return data, nil
}

// Write atomically stores data under (appID, key): file 0600, dir 0700. It is a
// create-or-replace upsert for that key; content is written verbatim in
// plaintext. 0600 only guards against other local OS users — it does not protect
// against this user's processes, backups, or synced folders. appID and key are
// opaque strings: any "/" is escaped into a single path segment, never treated
// as a directory separator.
func Write(appID, key string, data []byte) error {
	if err := checkSeg(appID, "appID"); err != nil {
		return err
	}
	if err := checkSeg(key, "key"); err != nil {
		return err
	}
	if err := vfs.MkdirAll(appDir(appID), 0700); err != nil {
		return appsStorageError(err, "apps storage: create dir: %v", err)
	}
	if err := validate.AtomicWrite(appKeyPath(appID, key), data, 0600); err != nil {
		return appsStorageError(err, "apps storage: write: %v", err)
	}
	return nil
}

// Delete removes the file under (appID, key). A missing file is not an error.
func Delete(appID, key string) error {
	if err := checkSeg(appID, "appID"); err != nil {
		return err
	}
	if err := checkSeg(key, "key"); err != nil {
		return err
	}
	if err := vfs.Remove(appKeyPath(appID, key)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return appsStorageError(err, "apps storage: delete: %v", err)
	}
	return nil
}

// List returns the keys stored under appID, skipping subdirectories and names
// that fail to unescape or validate after decoding. A missing app directory
// yields an empty list.
func List(appID string) ([]string, error) {
	if err := checkSeg(appID, "appID"); err != nil {
		return nil, err
	}
	entries, err := vfs.ReadDir(appDir(appID))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []string{}, nil
		}
		return nil, appsStorageError(err, "apps storage: read dir: %v", err)
	}
	keys := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		key, err := url.PathUnescape(e.Name())
		if err != nil {
			continue
		}
		if err := checkSeg(key, "key"); err != nil {
			continue
		}
		keys = append(keys, key)
	}
	return keys, nil
}
