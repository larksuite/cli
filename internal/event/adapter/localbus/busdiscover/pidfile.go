// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package busdiscover

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/larksuite/cli/internal/lockfile"
	"github.com/larksuite/cli/internal/vfs"
)

const (
	pidFileName       = "bus.pid"
	aliveLockFileName = "bus.alive.lock"
)

// Handle keeps the lifetime lock fd alive; OS releases on process exit.
type Handle struct {
	lock *lockfile.LockFile
}

// Release is for tests only; production lets process exit release the lock.
func (h *Handle) Release() error {
	if h == nil || h.lock == nil {
		return nil
	}
	return h.lock.Unlock()
}

// WritePIDFile takes the alive lock and atomically writes pid + RFC3339 start time.
// Returns lockfile.ErrHeld if another bus holds the lock.
func WritePIDFile(eventsDir string, pid int) (*Handle, error) {
	if err := vfs.MkdirAll(eventsDir, 0700); err != nil {
		return nil, fmt.Errorf("busdiscover: mkdir %s: %w", eventsDir, err)
	}
	lock := lockfile.New(filepath.Join(eventsDir, aliveLockFileName))
	if err := lock.TryLock(); err != nil {
		return nil, err
	}
	pidPath := filepath.Join(eventsDir, pidFileName)
	tmpPath := pidPath + ".tmp"
	payload := fmt.Sprintf("%d\n%s\n", pid, time.Now().UTC().Format(time.RFC3339))
	if err := vfs.WriteFile(tmpPath, []byte(payload), 0600); err != nil {
		_ = lock.Unlock()
		return nil, fmt.Errorf("busdiscover: write pid tmp: %w", err)
	}
	if err := vfs.Rename(tmpPath, pidPath); err != nil {
		_ = vfs.Remove(tmpPath)
		_ = lock.Unlock()
		return nil, fmt.Errorf("busdiscover: rename pid file: %w", err)
	}
	return &Handle{lock: lock}, nil
}

func readPIDFile(eventsDir string) (int, time.Time, error) {
	pidPath := filepath.Join(eventsDir, pidFileName)
	data, err := vfs.ReadFile(pidPath)
	if err != nil {
		return 0, time.Time{}, err
	}
	lines := strings.SplitN(strings.TrimSpace(string(data)), "\n", 2)
	if len(lines) < 2 {
		return 0, time.Time{}, fmt.Errorf("busdiscover: malformed pid file %s", pidPath)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(lines[0]))
	if err != nil {
		return 0, time.Time{}, fmt.Errorf("busdiscover: malformed pid in %s: %w", pidPath, err)
	}
	startTime, err := time.Parse(time.RFC3339, strings.TrimSpace(lines[1]))
	if err != nil {
		return 0, time.Time{}, fmt.Errorf("busdiscover: malformed timestamp in %s: %w", pidPath, err)
	}
	return pid, startTime, nil
}

// isBusAlive: try-lock the alive file. ErrHeld = live holder; success = stale (release immediately).
func isBusAlive(appDir string) bool {
	lockPath := filepath.Join(appDir, aliveLockFileName)
	if _, err := vfs.Stat(lockPath); err != nil {
		return false
	}
	probe := lockfile.New(lockPath)
	err := probe.TryLock()
	if errors.Is(err, lockfile.ErrHeld) {
		return true
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "[busdiscover] probe %s: %v\n", lockPath, err) //nolint:forbidigo // internal diagnostic; scanner has no IOStreams plumbing
		return false
	}
	_ = probe.Unlock()
	return false
}

func scanLiveBuses(eventsDir string) ([]Process, error) {
	entries, err := vfs.ReadDir(eventsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("busdiscover: read events dir: %w", err)
	}
	var result []Process
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		appID := e.Name()
		appDir := filepath.Join(eventsDir, appID)
		if !isBusAlive(appDir) {
			continue
		}
		pid, startTime, err := readPIDFile(appDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[busdiscover] live bus at %s but pid file unreadable: %v\n", appDir, err) //nolint:forbidigo // internal diagnostic; scanner has no IOStreams plumbing
			result = append(result, Process{PID: 0, AppID: appID})
			continue
		}
		result = append(result, Process{PID: pid, AppID: appID, StartTime: startTime})
	}
	return result, nil
}
