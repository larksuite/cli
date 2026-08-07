// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sparkstore

import (
	"errors"
	"net/url"
	"os"

	"github.com/larksuite/cli/internal/vfs"
)

// AppStorage adapts Spark local state to the Git credential store interface.
type AppStorage struct{}

func (AppStorage) Read(appID, key string) ([]byte, error) {
	return Read(appID, key)
}

func (AppStorage) Write(appID, key string, data []byte) error {
	return Write(appID, key, data)
}

func (AppStorage) Delete(appID, key string) error {
	return Delete(appID, key)
}

func (AppStorage) ListAppIDs() ([]string, error) {
	entries, err := vfs.ReadDir(Root())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []string{}, nil
		}
		return nil, storageError(err, "apps storage: read root: %v", err)
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
