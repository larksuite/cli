// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build !windows

package busdiscover

import (
	"bufio"
	"bytes"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// lstartLayout is the fixed format that `ps -o lstart` emits on macOS/Linux.
// It is a 24-character field, e.g. "Sun Apr 19 03:03:40 2026".
const lstartLayout = "Mon Jan _2 15:04:05 2006"

func newPlatformScanner() Scanner {
	return &unixScanner{
		runPS: func() ([]byte, error) {
			return exec.Command("ps", "-eo", "pid,lstart,command").Output()
		},
	}
}

// unixScanner shells out to `ps` and parses the output.
// runPS is a field so tests can inject canned output without spawning ps.
type unixScanner struct {
	runPS func() ([]byte, error)
}

func (s *unixScanner) ScanBusProcesses() ([]Process, error) {
	out, err := s.runPS()
	if err != nil {
		return nil, fmt.Errorf("busdiscover: run ps: %w", err)
	}
	return parseUnixPS(out), nil
}

// parseUnixPS parses the output of `ps -eo pid,lstart,command`.
//
// Expected columns (width-based, lstart is always 24 chars):
//
//	PID<spaces>LSTART<space>COMMAND
//
// The first line is a header and is skipped. Lines that don't parse cleanly
// (wrong column count, unparseable lstart, non-bus cmdline) are silently
// dropped — we're scanning the whole system, most lines are not buses.
func parseUnixPS(out []byte) []Process {
	var result []Process
	sc := bufio.NewScanner(bytes.NewReader(out))
	// Allow long lines; commands can exceed the default 64KB token.
	sc.Buffer(make([]byte, 0, 128*1024), 1024*1024)

	first := true
	for sc.Scan() {
		line := sc.Text()
		if first {
			first = false
			continue // header
		}
		p, ok := parseOneUnixPSLine(line)
		if !ok {
			continue
		}
		result = append(result, p)
	}
	return result
}

// parseOneUnixPSLine parses a single ps data line. Returns (_, false) on any
// format error or if the cmdline is not a bus process.
func parseOneUnixPSLine(line string) (Process, bool) {
	// Skip leading whitespace before PID.
	line = strings.TrimLeft(line, " ")
	if line == "" {
		return Process{}, false
	}

	// Extract PID (first whitespace-delimited token).
	sp := strings.IndexByte(line, ' ')
	if sp < 0 {
		return Process{}, false
	}
	pid, err := strconv.Atoi(line[:sp])
	if err != nil {
		return Process{}, false
	}
	rest := strings.TrimLeft(line[sp:], " ")

	// LSTART is exactly 24 chars in format "Mon Jan _2 15:04:05 2006".
	if len(rest) < 24 {
		return Process{}, false
	}
	lstart := rest[:24]
	startTime, err := time.ParseInLocation(lstartLayout, lstart, time.Local)
	if err != nil {
		return Process{}, false
	}
	// Remainder is the command line.
	cmdline := strings.TrimLeft(rest[24:], " ")

	appID := parseAppIDFromCmdline(cmdline)
	if appID == "" {
		return Process{}, false
	}
	return Process{PID: pid, AppID: appID, StartTime: startTime}, true
}
