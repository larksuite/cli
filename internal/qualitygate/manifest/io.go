// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package manifest

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/larksuite/cli/internal/vfs"
	"github.com/larksuite/cli/internal/vfs/localfileio"
)

func ReadFile(path, kind string) (Manifest, error) {
	data, err := vfs.ReadFile(path)
	if err != nil {
		return Manifest{}, err
	}
	return readBytes(filepath.Base(path), data, kind)
}

func ReadBytes(data []byte, kind string) (Manifest, error) {
	return readBytes(kind, data, kind)
}

func readBytes(label string, data []byte, kind string) (Manifest, error) {
	if len(data) > MaxManifestBytes {
		return Manifest{}, fmt.Errorf("%s is too large: %d bytes", label, len(data))
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return Manifest{}, fmt.Errorf("decode %s: %w", label, err)
	}
	if err := m.Validate(kind); err != nil {
		return Manifest{}, err
	}
	return m, nil
}

func WriteFile(path, kind string, m Manifest) error {
	if err := m.Validate(kind); err != nil {
		return err
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return localfileio.AtomicWrite(path, data, 0o644)
}
