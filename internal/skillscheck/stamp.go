// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package skillscheck

import (
	"errors"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/internal/vfs"
)

const stampFile = "skills.stamp"

// stampPath returns ~/.lark-cli/skills.stamp.
// Uses the BASE config dir (not workspace-aware) because skills install
// globally via `npx -g`; per-workspace tracking would produce false
// drift signals when switching workspaces.
func stampPath() string {
	return filepath.Join(core.GetBaseConfigDir(), stampFile)
}

// ReadStamp returns the version recorded in the stamp file. Returns
// ("", nil) when the file does not exist (interpreted as "never synced").
// Other I/O errors are returned as-is so callers can fail closed.
func ReadStamp() (string, error) {
	data, err := vfs.ReadFile(stampPath())
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

// WriteStamp records `version` as the last successfully synced skills
// version. Atomic via tmp + rename (validate.AtomicWrite). Creates
// the base config directory if it does not exist.
func WriteStamp(version string) error {
	if err := vfs.MkdirAll(core.GetBaseConfigDir(), 0o700); err != nil {
		return err
	}
	return validate.AtomicWrite(stampPath(), []byte(version), 0o644)
}
