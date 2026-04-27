// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package binding

import (
	"encoding/json"
	"fmt"

	"github.com/larksuite/cli/internal/vfs"
)

// ReadOpenClawConfig reads and parses an openclaw.json file at the given path.
func ReadOpenClawConfig(path string) (*OpenClawRoot, error) {
	data, err := vfs.ReadFile(path)
	if err != nil {
		return nil, err // caller (bind.go) formats user-facing message with path context
	}

	var root OpenClawRoot
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("invalid JSON in %s: %w", path, err)
	}

	return &root, nil
}
