// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build !windows

package busdiscover

import (
	"errors"
	"testing"
	"time"
)

func TestUnixScanner_ParsesBusProcesses(t *testing.T) {
	// Header line + three processes: one bus, one vim, one lark-cli consume
	// (not a bus). Only the bus should be returned.
	canned := []byte(`  PID                  LSTART COMMAND
70926 Sun Apr 19 03:03:40 2026 /Users/bytedance/go/src/github/cli/lark-cli event _bus --profile cli_XXXXXXXXXXXXXXXX --domain https://open.feishu.cn
12345 Mon Apr 20 10:00:00 2026 /usr/bin/vim /tmp/foo.txt
54321 Mon Apr 20 11:00:00 2026 /usr/local/bin/lark-cli event consume im.message.receive_v1
`)
	s := &unixScanner{runPS: func() ([]byte, error) { return canned, nil }}

	procs, err := s.ScanBusProcesses()
	if err != nil {
		t.Fatalf("ScanBusProcesses: %v", err)
	}
	if len(procs) != 1 {
		t.Fatalf("got %d processes, want 1: %+v", len(procs), procs)
	}
	if procs[0].PID != 70926 {
		t.Errorf("PID = %d, want 70926", procs[0].PID)
	}
	if procs[0].AppID != "cli_XXXXXXXXXXXXXXXX" {
		t.Errorf("AppID = %q, want cli_XXXXXXXXXXXXXXXX", procs[0].AppID)
	}
	wantTime := time.Date(2026, 4, 19, 3, 3, 40, 0, time.Local)
	if !procs[0].StartTime.Equal(wantTime) {
		t.Errorf("StartTime = %v, want %v", procs[0].StartTime, wantTime)
	}
}

func TestUnixScanner_HandlesEmptyPS(t *testing.T) {
	canned := []byte(`  PID                  LSTART COMMAND
`)
	s := &unixScanner{runPS: func() ([]byte, error) { return canned, nil }}

	procs, err := s.ScanBusProcesses()
	if err != nil {
		t.Fatalf("ScanBusProcesses: %v", err)
	}
	if len(procs) != 0 {
		t.Errorf("expected 0 processes, got %d", len(procs))
	}
}

func TestUnixScanner_PSFailurePropagates(t *testing.T) {
	s := &unixScanner{runPS: func() ([]byte, error) {
		return nil, errors.New("ps not found")
	}}

	_, err := s.ScanBusProcesses()
	if err == nil {
		t.Fatal("expected error from runPS failure")
	}
}

func TestUnixScanner_SkipsMalformedLines(t *testing.T) {
	// Line too short to contain an lstart field should be silently skipped.
	canned := []byte(`  PID                  LSTART COMMAND
70926 Sun Apr 19 03:03:40 2026 /usr/local/bin/lark-cli event _bus --profile cli_XXXXXXXXXXXXXXXX
short_line
99999 bogus_date_format /usr/local/bin/lark-cli event _bus --profile cli_xyz
`)
	s := &unixScanner{runPS: func() ([]byte, error) { return canned, nil }}

	procs, err := s.ScanBusProcesses()
	if err != nil {
		t.Fatalf("ScanBusProcesses: %v", err)
	}
	// The malformed lines should be dropped; the valid one kept.
	if len(procs) != 1 {
		t.Fatalf("got %d processes, want 1 (malformed lines must be skipped): %+v", len(procs), procs)
	}
	if procs[0].PID != 70926 {
		t.Errorf("PID = %d, want 70926", procs[0].PID)
	}
}
