// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package busdiscover scans the OS process table for live bus processes to detect orphans.
package busdiscover

import (
	"regexp"
	"strings"
	"time"
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
	return newPlatformScanner()
}

// appIDPattern requires the cli_ prefix to avoid matching unrelated --profile values.
var appIDPattern = regexp.MustCompile(`--profile\s+(cli_[a-zA-Z0-9_]+)`)

func parseAppIDFromCmdline(cmdline string) string {
	if !strings.Contains(cmdline, "lark-cli") {
		return ""
	}
	if !strings.Contains(cmdline, "event _bus") {
		return ""
	}
	m := appIDPattern.FindStringSubmatch(cmdline)
	if len(m) < 2 {
		return ""
	}
	return m[1]
}
