// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package config

import (
	"context"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	larkauth "github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
)

// holdLoginLock acquires the SingleUser "login" flock against the test's
// configDir and returns a release func that callers MUST defer. Failures bubble
// as t.Fatal — the lock is the test's witness, so an acquire failure is a test
// failure, not a system-under-test failure.
func holdLoginLock(t *testing.T, configDir string) (release func()) {
	t.Helper()
	root := larkauth.NewLocalRoot(configDir)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	lk, err := root.Locks(larkauth.SingleUser()).Acquire(ctx, "login", 5*time.Second)
	if err != nil {
		cancel()
		t.Fatalf("test setup: acquire login flock: %v", err)
	}
	return func() {
		lk.Release()
		cancel()
	}
}

// configDirForStrictModeTest reuses setupStrictModeTestConfig's wiring while
// returning the dir so the test can root its own flock against it.
func configDirForStrictModeTest(t *testing.T) string {
	t.Helper()
	setupStrictModeTestConfig(t)
	// setupStrictModeTestConfig sets LARKSUITE_CLI_CONFIG_DIR; we read it back
	// rather than re-deriving so the two paths can never drift.
	dir := os.Getenv("LARKSUITE_CLI_CONFIG_DIR")
	if dir == "" {
		t.Fatal("setupStrictModeTestConfig did not set LARKSUITE_CLI_CONFIG_DIR")
	}
	return dir
}

// TestStrictMode_Show_DoesNotBlockOnHeldLock pins the post-fix invariant:
// `config strict-mode` with no args MUST NOT acquire the login flock. Pre-fix,
// a peer holding the lock (typical case: `config bind` sat at its TUI prompt)
// turned an instant status query into a 30s flock-timeout failure.
//
// We assert by holding the lock for the entire test and verifying the show
// command completes well within the 30s acquire deadline — a 2s budget gives
// plenty of CI headroom while still proving the show path is lock-free.
func TestStrictMode_Show_DoesNotBlockOnHeldLock(t *testing.T) {
	dir := configDirForStrictModeTest(t)

	release := holdLoginLock(t, dir)
	defer release()

	f, stdout, _, _ := cmdutil.TestFactory(t, &core.CliConfig{AppID: "test-app", AppSecret: "secret"})
	cmd := NewCmdConfigStrictMode(f)
	cmd.SetArgs([]string{})

	done := make(chan error, 1)
	start := time.Now()
	go func() { done <- cmd.Execute() }()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("strict-mode show with held lock: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("strict-mode show blocked >2s with login flock held; the show path "+
			"is taking the writer lock again — regression of the show-path lock-skip fix. "+
			"elapsed=%v", time.Since(start))
	}

	if !strings.Contains(stdout.String(), "off") {
		t.Errorf("expected 'off' in show output, got: %s", stdout.String())
	}
}

// TestDefaultAs_Show_DoesNotBlockOnHeldLock mirrors the strict-mode test for
// `config default-as` with no args. Same pre-fix bug shape, same post-fix
// contract: the show path is lock-free.
func TestDefaultAs_Show_DoesNotBlockOnHeldLock(t *testing.T) {
	dir := configDirForStrictModeTest(t) // same shape: single-app multi config

	release := holdLoginLock(t, dir)
	defer release()

	f, stdout, _, _ := cmdutil.TestFactory(t, &core.CliConfig{AppID: "test-app", AppSecret: "secret"})
	cmd := NewCmdConfigDefaultAs(f)
	cmd.SetArgs([]string{})

	done := make(chan error, 1)
	start := time.Now()
	go func() { done <- cmd.Execute() }()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("default-as show with held lock: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("default-as show blocked >2s with login flock held; elapsed=%v", time.Since(start))
	}

	// app.DefaultAs is empty in the seed config → the show path prints "auto".
	if !strings.Contains(stdout.String(), "default-as:") {
		t.Errorf("expected 'default-as:' in show output, got: %s", stdout.String())
	}
}

// TestStrictMode_Set_BlocksOnHeldLock is the counter-test: the SET arm MUST
// still acquire the login flock. If a future refactor accidentally hoists the
// Acquire call above the set branch (or removes it entirely), concurrent set
// invocations would race the underlying config.json. We verify by holding the
// lock for ~200ms in a goroutine, then running `strict-mode bot` and asserting
// it took at least that long — i.e. it waited for the lock.
//
// 200ms is far above OS scheduler jitter and far below the 30s acquire wait,
// so the assertion is robust against CI noise without slowing the suite.
func TestStrictMode_Set_BlocksOnHeldLock(t *testing.T) {
	dir := configDirForStrictModeTest(t)

	const holdFor = 200 * time.Millisecond
	const lowerBound = 100 * time.Millisecond // generous floor below holdFor

	root := larkauth.NewLocalRoot(dir)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	lk, err := root.Locks(larkauth.SingleUser()).Acquire(ctx, "login", 5*time.Second)
	if err != nil {
		t.Fatalf("acquire test lock: %v", err)
	}

	// Release after `holdFor` so the system-under-test's 30s acquire eventually
	// wins. WaitGroup ensures the goroutine has actually run before we assert.
	var wg sync.WaitGroup
	wg.Add(1)
	releaseAt := time.Now().Add(holdFor)
	go func() {
		defer wg.Done()
		time.Sleep(holdFor)
		lk.Release()
	}()

	f, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{AppID: "test-app", AppSecret: "secret"})
	cmd := NewCmdConfigStrictMode(f)
	cmd.SetArgs([]string{"bot"})

	start := time.Now()
	if err := cmd.Execute(); err != nil {
		wg.Wait()
		t.Fatalf("strict-mode set: %v", err)
	}
	elapsed := time.Since(start)
	wg.Wait()

	if elapsed < lowerBound {
		t.Errorf("strict-mode set returned in %v, expected >= %v — the set arm "+
			"appears to have skipped the login flock; the lock-skip fix should "+
			"only apply to the no-args show branch.", elapsed, lowerBound)
	}
	if !time.Now().After(releaseAt) {
		// Defensive: shouldn't happen given elapsed >= lowerBound, but explicit
		// is better than implicit.
		t.Errorf("set returned before goroutine released the lock — timing invariant violated")
	}
}
