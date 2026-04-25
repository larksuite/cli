// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package bus

import (
	"log"
	"os"
	"path/filepath"

	"github.com/larksuite/cli/internal/vfs"
)

const (
	maxLogSize    = 5 * 1024 * 1024 // 5 MB
	logFileName   = "bus.log"
	logBackupName = "bus.log.1"
)

// SetupBusLogger writes to eventsDir/bus.log with one-shot size-based rotation at startup only.
func SetupBusLogger(eventsDir string) (*log.Logger, error) {
	if err := vfs.MkdirAll(eventsDir, 0700); err != nil {
		return nil, err
	}

	logPath := filepath.Join(eventsDir, logFileName)
	backupPath := filepath.Join(eventsDir, logBackupName)

	if info, err := vfs.Stat(logPath); err == nil && info.Size() > maxLogSize {
		_ = vfs.Remove(backupPath)
		_ = vfs.Rename(logPath, backupPath)
	}

	f, err := vfs.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return nil, err
	}

	return log.New(f, "", log.LstdFlags), nil
}
