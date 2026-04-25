// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build !windows

package busdiscover

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

const lstartLayout = "Mon Jan _2 15:04:05 2006" // fixed `ps -o lstart` format, 24 chars

func newPlatformScanner() Scanner {
	return &unixScanner{
		runPS: func() ([]byte, error) {
			cmd := exec.Command("ps", "-eo", "pid,lstart,command")
			// Force C locale so lstart emits English names matching lstartLayout.
			cmd.Env = append(os.Environ(), "LC_ALL=C", "LANG=C")
			return cmd.Output()
		},
	}
}

type unixScanner struct {
	runPS func() ([]byte, error) // injectable for tests
}

func (s *unixScanner) ScanBusProcesses() ([]Process, error) {
	out, err := s.runPS()
	if err != nil {
		return nil, fmt.Errorf("busdiscover: run ps: %w", err)
	}
	return parseUnixPS(out), nil
}

// parseUnixPS parses `ps -eo pid,lstart,command`; malformed/non-bus lines silently dropped.
func parseUnixPS(out []byte) []Process {
	var result []Process
	sc := bufio.NewScanner(bytes.NewReader(out))
	sc.Buffer(make([]byte, 0, 128*1024), 1024*1024) // commands can exceed default 64KB

	first := true
	for sc.Scan() {
		line := sc.Text()
		if first {
			first = false
			continue
		}
		p, ok := parseOneUnixPSLine(line)
		if !ok {
			continue
		}
		result = append(result, p)
	}
	return result
}

func parseOneUnixPSLine(line string) (Process, bool) {
	line = strings.TrimLeft(line, " ")
	if line == "" {
		return Process{}, false
	}

	sp := strings.IndexByte(line, ' ')
	if sp < 0 {
		return Process{}, false
	}
	pid, err := strconv.Atoi(line[:sp])
	if err != nil {
		return Process{}, false
	}
	rest := strings.TrimLeft(line[sp:], " ")

	if len(rest) < 24 {
		return Process{}, false
	}
	lstart := rest[:24]
	startTime, err := time.ParseInLocation(lstartLayout, lstart, time.Local)
	if err != nil {
		return Process{}, false
	}
	cmdline := strings.TrimLeft(rest[24:], " ")

	appID := parseAppIDFromCmdline(cmdline)
	if appID == "" {
		return Process{}, false
	}
	return Process{PID: pid, AppID: appID, StartTime: startTime}, true
}
