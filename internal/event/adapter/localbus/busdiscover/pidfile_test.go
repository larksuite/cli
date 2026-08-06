// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package busdiscover

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/larksuite/cli/internal/lockfile"
)

func TestWritePIDFile_WritesPIDAndTimestamp(t *testing.T) {
	dir := t.TempDir()
	h, err := WritePIDFile(dir, 4242)
	if err != nil {
		t.Fatalf("WritePIDFile: %v", err)
	}
	t.Cleanup(func() { _ = h.Release() })

	data, err := os.ReadFile(filepath.Join(dir, "bus.pid"))
	if err != nil {
		t.Fatalf("read pid file: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d: %q", len(lines), string(data))
	}
	if lines[0] != "4242" {
		t.Errorf("pid line = %q, want %q", lines[0], "4242")
	}
	ts, err := time.Parse(time.RFC3339, lines[1])
	if err != nil {
		t.Errorf("timestamp parse: %v (line: %q)", err, lines[1])
	}
	if time.Since(ts) > time.Minute {
		t.Errorf("timestamp = %v, expected within last minute", ts)
	}
}

func TestWritePIDFile_SecondCallReturnsErrHeld(t *testing.T) {
	dir := t.TempDir()
	h1, err := WritePIDFile(dir, 1111)
	if err != nil {
		t.Fatalf("first WritePIDFile: %v", err)
	}
	t.Cleanup(func() { _ = h1.Release() })

	_, err = WritePIDFile(dir, 2222)
	if !errors.Is(err, lockfile.ErrHeld) {
		t.Errorf("second WritePIDFile err = %v, want lockfile.ErrHeld", err)
	}
}

func TestWritePIDFile_ReleaseAllowsReacquire(t *testing.T) {
	dir := t.TempDir()
	h1, err := WritePIDFile(dir, 1111)
	if err != nil {
		t.Fatalf("first WritePIDFile: %v", err)
	}
	if err := h1.Release(); err != nil {
		t.Fatalf("Release: %v", err)
	}
	h2, err := WritePIDFile(dir, 2222)
	if err != nil {
		t.Fatalf("re-acquire after Release: %v", err)
	}
	t.Cleanup(func() { _ = h2.Release() })
}

func TestScanLiveBuses_ReturnsLiveBusOnly(t *testing.T) {
	root := t.TempDir()

	liveDir := filepath.Join(root, "cli_live")
	hLive, err := WritePIDFile(liveDir, 7777)
	if err != nil {
		t.Fatalf("WritePIDFile live: %v", err)
	}
	t.Cleanup(func() { _ = hLive.Release() })

	deadDir := filepath.Join(root, "cli_dead")
	hDead, err := WritePIDFile(deadDir, 8888)
	if err != nil {
		t.Fatalf("WritePIDFile dead: %v", err)
	}
	if err := hDead.Release(); err != nil {
		t.Fatalf("Release dead: %v", err)
	}

	if err := os.MkdirAll(filepath.Join(root, "empty"), 0700); err != nil {
		t.Fatalf("mkdir empty: %v", err)
	}

	procs, err := scanLiveBuses(root)
	if err != nil {
		t.Fatalf("scanLiveBuses: %v", err)
	}
	if len(procs) != 1 {
		t.Fatalf("expected 1 live proc, got %d: %+v", len(procs), procs)
	}
	if procs[0].AppID != "cli_live" {
		t.Errorf("AppID = %q, want %q", procs[0].AppID, "cli_live")
	}
	if procs[0].PID != 7777 {
		t.Errorf("PID = %d, want 7777", procs[0].PID)
	}
}

func TestScanLiveBuses_MissingDirIsNotError(t *testing.T) {
	procs, err := scanLiveBuses(filepath.Join(t.TempDir(), "does-not-exist"))
	if err != nil {
		t.Errorf("err = %v, want nil", err)
	}
	if len(procs) != 0 {
		t.Errorf("expected empty result, got %+v", procs)
	}
}

func TestScanLiveBuses_LiveBusWithCorruptPIDFileSurfaced(t *testing.T) {
	root := t.TempDir()
	appDir := filepath.Join(root, "cli_corrupt")
	if err := os.MkdirAll(appDir, 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	lock := lockfile.New(filepath.Join(appDir, aliveLockFileName))
	if err := lock.TryLock(); err != nil {
		t.Fatalf("TryLock: %v", err)
	}
	t.Cleanup(func() { _ = lock.Unlock() })
	if err := os.WriteFile(filepath.Join(appDir, pidFileName), []byte("garbage"), 0600); err != nil {
		t.Fatalf("write corrupt pid: %v", err)
	}

	procs, err := scanLiveBuses(root)
	if err != nil {
		t.Fatalf("scanLiveBuses: %v", err)
	}
	if len(procs) != 1 {
		t.Fatalf("expected 1 entry (live bus surfaced anonymously), got %d: %+v", len(procs), procs)
	}
	if procs[0].AppID != "cli_corrupt" {
		t.Errorf("AppID = %q, want %q", procs[0].AppID, "cli_corrupt")
	}
	if procs[0].PID != 0 {
		t.Errorf("PID = %d, want 0 (anonymous)", procs[0].PID)
	}
}
