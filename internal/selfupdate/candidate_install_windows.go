// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build windows

package selfupdate

import (
	"errors"
	"fmt"
	"io/fs"

	"github.com/larksuite/cli/internal/vfs"
)

// install replaces the target in two phases because Windows refuses to
// overwrite a running executable: the target is first moved aside to .old,
// then the candidate is renamed into place. A crash between the two renames
// leaves no usable target; the next run's CleanupStaleFiles recovers it from
// .old (see updater_windows.go).
func (c *Candidate) install() (func(), error) {
	backup := c.target + ".old"
	backedUp := false
	if _, err := vfs.Stat(c.target); err == nil {
		// Drop a stale backup from an interrupted update, then move the current
		// executable aside so a failed promotion can restore it.
		if err := vfs.Remove(backup); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("remove stale binary backup: %w", err)
		}
		if err := vfs.Rename(c.target, backup); err != nil {
			return nil, err
		}
		backedUp = true
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	if err := vfs.Rename(c.path, c.target); err != nil {
		if backedUp {
			// The previous executable is the only known-good binary: a failed
			// restore is reported explicitly and .old is kept for manual
			// recovery instead of being silently swallowed.
			if restoreErr := vfs.Rename(backup, c.target); restoreErr != nil {
				return nil, fmt.Errorf("replace binary: %w (restoring the previous binary also failed: %v; it remains at %s, restore it manually)", err, restoreErr, backup)
			}
		}
		return nil, fmt.Errorf("replace binary: %w", err)
	}
	c.path = ""
	return func() { _ = vfs.Remove(backup) }, nil
}
