// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"errors"
	"net/url"
	"os"
	"path/filepath"

	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/vfs" //nolint:depguard // Git credential list scans CLI config-dir state; it is not user file I/O.
)

type gitCredentialAppStorage struct{}

func (gitCredentialAppStorage) Read(appID, key string) ([]byte, error) {
	return Read(appID, key)
}

func (gitCredentialAppStorage) Write(appID, key string, data []byte) error {
	return Write(appID, key, data)
}

func (gitCredentialAppStorage) Delete(appID, key string) error {
	return Delete(appID, key)
}

func (gitCredentialAppStorage) ListAppIDs() ([]string, error) {
	root := filepath.Join(core.GetConfigDir(), storageRoot)
	entries, err := vfs.ReadDir(root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []string{}, nil
		}
		return nil, appsStorageError(err, "apps storage: read root: %v", err)
	}
	appIDs := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		appID, err := url.PathUnescape(e.Name())
		if err != nil {
			continue
		}
		if err := checkSeg(appID, "appID"); err != nil {
			continue
		}
		appIDs = append(appIDs, appID)
	}
	return appIDs, nil
}
