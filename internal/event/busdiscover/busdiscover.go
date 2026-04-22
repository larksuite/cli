// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package busdiscover scans the OS process table for live `lark-cli event _bus`
// processes. It exists so `event status` can detect orphan bus processes —
// processes still alive but whose IPC socket has gone missing, rendering them
// invisible to the socket-based status path.
//
// Truth model: we treat the process table as primary truth. A bus process is
// detected iff (a) its command line contains "lark-cli" AND "event _bus", and
// (b) its --profile flag value can be parsed as an AppID. This two-gate filter
// also naturally excludes pid-reused processes: after a bus dies, if the OS
// reassigns its pid to an unrelated process, that process's cmdline won't match
// the gate and will be silently filtered out.
package busdiscover

import (
	"regexp"
	"strings"
	"time"
)

// Process describes one live bus process discovered on this machine.
type Process struct {
	PID       int
	AppID     string    // parsed from --profile flag
	StartTime time.Time // process creation time
}

// Scanner discovers live bus processes. Implementations are platform-specific.
type Scanner interface {
	ScanBusProcesses() ([]Process, error)
}

// Default returns a production scanner for the current platform.
func Default() Scanner {
	return newPlatformScanner()
}

// appIDPattern captures the AppID value following a --profile flag.
// AppIDs are canonically of the form cli_<alnum_underscore> — we require that
// prefix to avoid accidentally matching unrelated flag values.
var appIDPattern = regexp.MustCompile(`--profile\s+(cli_[a-zA-Z0-9_]+)`)

// parseAppIDFromCmdline returns the AppID from a bus command line, or "" if
// the command line does not belong to a lark-cli event _bus process.
//
// Gate 1: cmdline must contain both "lark-cli" and "event _bus" substrings.
// Gate 2: --profile flag must be present with a value matching cli_* pattern.
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
