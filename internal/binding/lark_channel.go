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
	// Secrets is an optional registry of secret providers — same shape as
	// openclaw's `secrets` block. Lets bridge declare `exec` provider scripts
	// (for AES-encrypted secret backends), `env` allowlists, or `file`
	// indirection rules. Resolved by binding.ResolveSecretInput.
	Secrets *SecretsConfig `json:"secrets,omitempty"`
}

// LarkChannelAccounts is the namespace for credential entries.
// Currently only `app` is defined; left as a struct (not a flat field) so
// future entries (oauth, alternate apps) can be added without re-shaping the
// top-level on disk.
type LarkChannelAccounts struct {
	App LarkChannelApp `json:"app"`
}

// LarkChannelApp is the bot app credential entry.
//
// `Secret` accepts the full SecretInput protocol (string / "${VAR}" template /
// SecretRef object with source env|file|exec) so users can keep secrets out
// of config.json — either by referencing an env var the bridge inherits, a
// chmod-0400 file outside the bridge dir, or an exec script that decrypts a
// local AES-encrypted secret store. Aligns lark-channel with the same secret
// protocol openclaw already uses.
type LarkChannelApp struct {
	ID     string      `json:"id"`
	Secret SecretInput `json:"secret"`
	Tenant string      `json:"tenant"` // "feishu" | "lark"
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
