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

// SetupBusLogger creates a log.Logger that writes to eventsDir/bus.log
// with simple rotation: when the log exceeds maxLogSize, the current
// log is renamed to bus.log.1 and a fresh file is opened.
//
// LIMITATION: the size check runs exactly once, at bus start. A long-
// running bus (multi-day) will never rotate mid-run — bus.log grows
// unbounded until the next restart. This is adequate for dev/test and
// short (<1 day) production runs, which is the current target. For
// production daemons that stay up indefinitely, replace with a full
// rotation library (e.g. gopkg.in/natefinch/lumberjack.v2) or add a
// background ticker that periodically calls a MaybeRotate() helper.
// The lifecycle of the underlying *os.File is tied to the returned
// logger, so any rotation strategy also needs coordination with bus
// shutdown to close the file cleanly.
func SetupBusLogger(eventsDir string) (*log.Logger, error) {
	if err := vfs.MkdirAll(eventsDir, 0700); err != nil {
		return nil, err
	}

	logPath := filepath.Join(eventsDir, logFileName)
	backupPath := filepath.Join(eventsDir, logBackupName)

	// Rotate if current log exceeds max size.
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
