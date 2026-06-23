// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package facts

import (
	"encoding/json"
	"path/filepath"

	"github.com/larksuite/cli/internal/vfs"
)

func (f Facts) WriteFile(path string) error {
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := vfs.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return vfs.WriteFile(path, data, 0o644)
}

func ReadFile(path string) (Facts, error) {
	data, err := vfs.ReadFile(path)
	if err != nil {
		return Facts{}, err
	}
	var f Facts
	if err := json.Unmarshal(data, &f); err != nil {
		return Facts{}, err
	}
	return f, nil
}
