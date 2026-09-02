// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build !windows

package selfupdate

import (
	"github.com/larksuite/cli/internal/vfs"
)

// install promotes the staged candidate with a single atomic rename. Unix
// permits renaming over a running executable (inode semantics, same contract
// as updater_unix.go), so there is no window where the target is missing and
// no backup to roll back.
func (c *Candidate) install() (func(), error) {
	_ = vfs.Remove(c.target + ".old") // stale backup from an older two-phase update
	if err := vfs.Rename(c.path, c.target); err != nil {
		return nil, err
	}
	c.path = ""
	return func() {}, nil
}
