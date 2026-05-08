// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package binding

import (
	"encoding/json"
	"fmt"

	"github.com/larksuite/cli/internal/vfs"
)

// LarkChannelRoot captures ~/.lark-channel/config.json.
// Schema mirrors lark-channel-bridge/src/config/schema.ts:AppConfig.
// Unknown fields are ignored — forward-compatible with future bridge versions.
type LarkChannelRoot struct {
	Accounts LarkChannelAccounts `json:"accounts"`
}

// LarkChannelAccounts is the namespace for credential entries.
// Currently only `app` is defined; left as a struct (not a flat field) so
// future entries (oauth, alternate apps) can be added without re-shaping the
// top-level on disk.
type LarkChannelAccounts struct {
	App LarkChannelApp `json:"app"`
}

// LarkChannelApp is the bot app credential entry.
// Bridge stores the secret as plain text — secret-resolve indirection
// (${VAR} / file: / exec:) is intentionally not supported here, matching
// the bridge's on-disk format.
type LarkChannelApp struct {
	ID     string `json:"id"`
	Secret string `json:"secret"`
	Tenant string `json:"tenant"` // "feishu" | "lark"
}

// ReadLarkChannelConfig reads and parses ~/.lark-channel/config.json.
func ReadLarkChannelConfig(path string) (*LarkChannelRoot, error) {
	data, err := vfs.ReadFile(path)
	if err != nil {
		return nil, err // caller formats user-facing message with path context
	}

	var root LarkChannelRoot
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("invalid JSON in %s: %w", path, err)
	}

	return &root, nil
}
