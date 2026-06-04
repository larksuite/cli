// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package ratelimit

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/larksuite/cli/internal/lockfile"
	"github.com/larksuite/cli/internal/output"
)

func TestStoreWritesReadableStateWithRestrictivePermissions(t *testing.T) {
	dir := t.TempDir()
	rule := testRule()
	req := testRequest()
	limiter := NewLimiterForDir(dir, []Rule{rule}, time.Now)
	if err := limiter.Allow(context.Background(), req); err != nil {
		t.Fatalf("check err = %v", err)
	}
	statePath := testStatePath(dir, rule, req)
	data, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("expected non-empty state")
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(statePath)
		if err != nil {
			t.Fatalf("stat state: %v", err)
		}
		if got := info.Mode().Perm(); got != 0600 {
			t.Fatalf("state mode = %o, want 0600", got)
		}
	}
}

func TestStoreCorruptJSONReturnsInternalError(t *testing.T) {
	dir := t.TempDir()
	rule := testRule()
	req := testRequest()
	if err := os.WriteFile(testStatePath(dir, rule, req), []byte("{bad"), 0600); err != nil {
		t.Fatalf("write corrupt state: %v", err)
	}
	limiter := NewLimiterForDir(dir, []Rule{rule}, time.Now)
	err := limiter.Allow(context.Background(), req)
	if err == nil {
		t.Fatal("expected error")
	}
	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected ExitError, got %T", err)
	}
	if exitErr.Code != output.ExitInternal || exitErr.Detail == nil || exitErr.Detail.Type != "internal" {
		t.Fatalf("unexpected detail: %#v", exitErr.Detail)
	}
}

func TestTwoLimitersShareStateFile(t *testing.T) {
	dir := t.TempDir()
	now := time.Unix(100, 0)
	rule := testRule()
	rule.Limit = 1
	first := NewLimiterForDir(dir, []Rule{rule}, func() time.Time { return now })
	second := NewLimiterForDir(dir, []Rule{rule}, func() time.Time { return now })

	if err := first.Allow(context.Background(), testRequest()); err != nil {
		t.Fatalf("first check err = %v", err)
	}
	if err := second.Allow(context.Background(), testRequest()); err == nil {
		t.Fatal("expected second limiter to see shared state")
	}
}

func TestStoreDeletedStateFileStartsFreshWindow(t *testing.T) {
	dir := t.TempDir()
	now := time.Unix(100, 0)
	rule := testRule()
	rule.Limit = 1
	req := testRequest()
	limiter := NewLimiterForDir(dir, []Rule{rule}, func() time.Time { return now })

	if err := limiter.Allow(context.Background(), req); err != nil {
		t.Fatalf("first check err = %v", err)
	}
	statePath := testStatePath(dir, rule, req)
	if err := os.Remove(statePath); err != nil {
		t.Fatalf("remove state: %v", err)
	}
	if err := limiter.Allow(context.Background(), req); err != nil {
		t.Fatalf("check after deleted state err = %v", err)
	}
	if _, err := os.Stat(statePath); err != nil {
		t.Fatalf("state file should be recreated: %v", err)
	}
}

func TestDifferentKeysUseDifferentStateFiles(t *testing.T) {
	dir := t.TempDir()
	now := time.Unix(100, 0)
	rule := testRule()
	limiter := NewLimiterForDir(dir, []Rule{rule}, func() time.Time { return now })
	first := testRequest()
	second := first
	second.AppID = "app-2"

	if err := limiter.Allow(context.Background(), first); err != nil {
		t.Fatalf("first check err = %v", err)
	}
	if err := limiter.Allow(context.Background(), second); err != nil {
		t.Fatalf("second check err = %v", err)
	}

	if _, err := os.Stat(testStatePath(dir, rule, first)); err != nil {
		t.Fatalf("first state file missing: %v", err)
	}
	if _, err := os.Stat(testStatePath(dir, rule, second)); err != nil {
		t.Fatalf("second state file missing: %v", err)
	}
}

func testStatePath(dir string, rule Rule, req Request) string {
	return filepath.Join(dir, buildKey(req.Brand, req.AppID, rule.Method, rule.CanonicalPath)+".json")
}

func TestStoreGCOldKeyStateFiles(t *testing.T) {
	dir := t.TempDir()
	rule := testRule()
	oldReq := testRequest()
	freshReq := oldReq
	freshReq.AppID = "app-2"
	oldPath := testStatePath(dir, rule, oldReq)
	freshPath := testStatePath(dir, rule, freshReq)
	writeTestKeyState(t, oldPath, []int64{1})
	writeTestKeyState(t, freshPath, []int64{2})

	now := time.Now()
	oldTime := now.Add(-maxRuleWindow(builtinRules) - stateGCGrace - time.Second)
	if err := os.Chtimes(oldPath, oldTime, oldTime); err != nil {
		t.Fatalf("chtimes old state: %v", err)
	}

	newStateFile(dir).gcExpired(now)

	if _, err := os.Stat(oldPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("old state file stat err = %v, want not exist", err)
	}
	if _, err := os.Stat(freshPath); err != nil {
		t.Fatalf("fresh state file should remain: %v", err)
	}
}

func TestStoreLockTimesOut(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	store := newStateFile(dir)
	key := buildKey(testRequest().Brand, testRequest().AppID, testRule().Method, testRule().CanonicalPath)
	_, lockPath := store.pathsForKey(key)
	lock := lockfile.New(lockPath)
	if err := lock.TryLock(); err != nil {
		t.Fatalf("hold lock: %v", err)
	}
	defer lock.Unlock()

	oldTimeout := lockWaitTimeout
	lockWaitTimeout = 20 * time.Millisecond
	defer func() { lockWaitTimeout = oldTimeout }()

	err := store.WithKeyLock(context.Background(), key, func(entries []int64) ([]int64, error) {
		t.Fatal("lock callback should not run")
		return entries, nil
	})
	if err == nil {
		t.Fatal("expected lock timeout")
	}
	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != output.ExitInternal {
		t.Fatalf("err = %v, want internal ExitError", err)
	}
	if !strings.Contains(err.Error(), "timed out waiting") {
		t.Fatalf("err = %v, want timeout message", err)
	}
}

func writeTestKeyState(t *testing.T, path string, entries []int64) {
	t.Helper()
	data, err := json.Marshal(keyState{Version: stateVersion, Entries: entries})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatalf("write key state: %v", err)
	}
}
