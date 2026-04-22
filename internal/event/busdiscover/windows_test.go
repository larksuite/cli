// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build windows

package busdiscover

import (
	"errors"
	"testing"
	"time"
)

func TestWindowsScanner_ParsesBusProcesses(t *testing.T) {
	// Canned output for the 2+ results case where ConvertTo-Json emits a JSON
	// array of objects, each with Pid, CreationDate (ISO 8601 after .ToString('o')),
	// CommandLine.
	canned := []byte(`[
  {
    "Pid": 4711,
    "CreationDate": "2026-04-19T03:03:40.1234567+08:00",
    "CommandLine": "\"C:\\Program Files\\lark-cli\\lark-cli.exe\" event _bus --profile cli_b1c2d3e4f5g6h7i8 --domain https://open.larksuite.com"
  },
  {
    "Pid": 4712,
    "CreationDate": "2026-04-19T10:00:00.0000000+08:00",
    "CommandLine": "\"C:\\Windows\\System32\\notepad.exe\""
  }
]`)
	s := &windowsScanner{runPS: func() ([]byte, error) { return canned, nil }}

	procs, err := s.ScanBusProcesses()
	if err != nil {
		t.Fatalf("ScanBusProcesses: %v", err)
	}
	if len(procs) != 1 {
		t.Fatalf("got %d processes, want 1: %+v", len(procs), procs)
	}
	if procs[0].PID != 4711 {
		t.Errorf("PID = %d, want 4711", procs[0].PID)
	}
	if procs[0].AppID != "cli_b1c2d3e4f5g6h7i8" {
		t.Errorf("AppID = %q, want cli_b1c2d3e4f5g6h7i8", procs[0].AppID)
	}
	// Compare in UTC to side-step local-timezone flakiness on CI runners.
	wantUTC := time.Date(2026, 4, 18, 19, 3, 40, 123456700, time.UTC)
	if !procs[0].StartTime.UTC().Equal(wantUTC) {
		t.Errorf("StartTime UTC = %v, want %v", procs[0].StartTime.UTC(), wantUTC)
	}
}

func TestWindowsScanner_EmptyArray(t *testing.T) {
	s := &windowsScanner{runPS: func() ([]byte, error) { return []byte(`[]`), nil }}
	procs, err := s.ScanBusProcesses()
	if err != nil {
		t.Fatalf("ScanBusProcesses: %v", err)
	}
	if len(procs) != 0 {
		t.Errorf("expected 0 processes, got %d", len(procs))
	}
}

func TestWindowsScanner_PSFailurePropagates(t *testing.T) {
	s := &windowsScanner{runPS: func() ([]byte, error) {
		return nil, errors.New("powershell not found")
	}}
	_, err := s.ScanBusProcesses()
	if err == nil {
		t.Fatal("expected error from runPS failure")
	}
}

func TestWindowsScanner_SingleObjectFallback(t *testing.T) {
	// PowerShell's ConvertTo-Json without -AsArray collapses a single-item array
	// into a bare object. Scanner must handle both.
	canned := []byte(`{
  "Pid": 4711,
  "CreationDate": "2026-04-19T03:03:40.0000000+00:00",
  "CommandLine": "lark-cli.exe event _bus --profile cli_xyz"
}`)
	s := &windowsScanner{runPS: func() ([]byte, error) { return canned, nil }}
	procs, err := s.ScanBusProcesses()
	if err != nil {
		t.Fatalf("ScanBusProcesses: %v", err)
	}
	if len(procs) != 1 {
		t.Fatalf("got %d processes, want 1", len(procs))
	}
	if procs[0].AppID != "cli_xyz" {
		t.Errorf("AppID = %q, want cli_xyz", procs[0].AppID)
	}
}

func TestWindowsScanner_UnexpectedOutput(t *testing.T) {
	s := &windowsScanner{runPS: func() ([]byte, error) {
		return []byte(`not json at all`), nil
	}}
	_, err := s.ScanBusProcesses()
	if err == nil {
		t.Fatal("expected error for non-JSON output")
	}
}
