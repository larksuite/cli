// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package busdiscover enumerates live bus daemons via per-AppID PID files protected by a process-lifetime advisory lock.
package busdiscover

import (
	"path/filepath"
	"time"

	"github.com/larksuite/cli/internal/core"
)

type Process struct {
	PID       int
	AppID     string
	StartTime time.Time
}

type Scanner interface {
	ScanBusProcesses() ([]Process, error)
}

func Default() Scanner {
	return &fsScanner{eventsDir: filepath.Join(core.GetConfigDir(), "events")}
}

type fsScanner struct {
	eventsDir string
}

func (s *fsScanner) ScanBusProcesses() ([]Process, error) {
	return scanLiveBuses(s.eventsDir)
}
