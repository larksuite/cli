// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package gitcred manages the lifecycle of app Git credentials.
//
// Lock ordering convention — read this before adding any new lock acquisition:
//
//	ALWAYS acquire lockApp BEFORE lockGlobalConfig, and lockGlobalConfig
//	BEFORE lockURL. Never invert this order.
//
// Rationale:
//   - lockApp is a cross-process file lock with bounded timeout (2s) and a
//     possible setup error; acquiring it first keeps the failure surface
//     outside any in-process lock and avoids holding the in-process mutex
//     while waiting on I/O / another process.
//   - lockGlobalConfig is a cross-process file lock, keyed by the writable
//     global Git config file path, that serializes the read-modify-write of
//     that file across concurrent lark-cli processes. It sits below lockApp
//     (which is finer-grained, per app) and above lockURL.
//   - lockURL is an in-process sync.Mutex that never fails and blocks
//     indefinitely; holding it while waiting on a file lock would risk
//     deadlocking with a concurrent goroutine that held the file lock first.
//
// Paths that only manipulate per-app state (Init, Remove, Erase) take lockApp.
// SetHelper/UnsetHelper additionally take lockGlobalConfig because they perform
// a read-modify-write on the shared global config file. Get() is the only path
// that touches per-URL state in addition to per-app state, so it is the only
// caller that takes lockURL.
package gitcred

import (
	"errors"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/lockfile"
	"github.com/larksuite/cli/internal/vfs" //nolint:depguard // git credential locks live under CLI config dir and are not user file I/O.
	"github.com/larksuite/cli/internal/workspace"
)

var urlLocks sync.Map

var safeLockNameChars = regexp.MustCompile(`[^a-zA-Z0-9._-]`)

// lockURL acquires an in-process, per-URL mutex. It never returns an error
// and blocks until the mutex is available.
//
// Lock ordering: lockURL MUST NOT be held while calling lockApp. See package
// comment for the full convention.
func lockURL(url string) func() {
	actual, _ := urlLocks.LoadOrStore(url, &sync.Mutex{})
	mu := actual.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

// lockApp acquires a cross-process file lock scoped to the given appID. It
// returns an unlock function or an error if the lock directory cannot be
// created or the lock cannot be acquired within the 2s timeout.
//
// Lock ordering: when both lockApp and lockURL are needed, lockApp must be
// taken FIRST. See package comment for the full convention.
func lockApp(appID string) (func(), error) {
	dir := filepath.Join(workspace.GetConfigDir(), "locks")
	if err := vfs.MkdirAll(dir, 0700); err != nil {
		return nil, errs.NewInternalError(errs.SubtypeStorage, "create Git credential lock dir: %v", err).WithCause(err)
	}
	name := "apps_git_credential_" + safeLockNameChars.ReplaceAllString(appID, "_") + ".lock"
	return acquireFileLock(filepath.Join(dir, filepath.Base(name)))
}

// lockGlobalConfig acquires a cross-process file lock that serializes the
// read-modify-write of the writable global Git config file identified by
// origin (a git "file:" origin or a bare path). It is keyed by the config file
// path so concurrent lark-cli processes editing the same global config are
// serialized while edits to different config files proceed independently.
//
// Lock ordering: lockGlobalConfig must be taken AFTER lockApp and BEFORE
// lockURL. See package comment for the full convention.
func lockGlobalConfig(origin string) (func(), error) {
	dir := filepath.Join(workspace.GetConfigDir(), "locks")
	if err := vfs.MkdirAll(dir, 0700); err != nil {
		return nil, errs.NewInternalError(errs.SubtypeStorage, "create Git credential lock dir: %v", err).WithCause(err)
	}
	name := globalConfigLockName(origin)
	unlock, err := acquireFileLock(filepath.Join(dir, filepath.Base(name)))
	if err != nil {
		return nil, errs.NewInternalError(errs.SubtypeStorage, "acquire global Git config lock: %v", err).WithCause(err)
	}
	return unlock, nil
}

// globalConfigLockName derives the cross-process lock file name for a writable
// global config origin. GIT_CONFIG_GLOBAL may be a relative path, which would
// otherwise make the lock name (and therefore the lock file) depend on the
// caller's working directory; joining it onto the current directory ensures
// every process locking the same config file agrees on the lock regardless of
// cwd. It falls back to the raw path if the cwd is unavailable.
func globalConfigLockName(origin string) string {
	path := strings.TrimPrefix(origin, "file:")
	if !filepath.IsAbs(path) {
		if cwd, err := vfs.Getwd(); err == nil {
			path = filepath.Join(cwd, path)
		}
	}
	return "apps_git_credential_globalcfg_" + safeLockNameChars.ReplaceAllString(path, "_") + ".lock"
}

// acquireFileLock takes the cross-process lock at lockPath, retrying on
// contention until a 2s deadline. It returns an unlock function or an error if
// the lock cannot be acquired within the timeout.
func acquireFileLock(lockPath string) (func(), error) {
	lock := lockfile.New(lockPath)
	deadline := time.Now().Add(2 * time.Second)
	for {
		err := lock.TryLock()
		if err == nil {
			return func() { _ = lock.Unlock() }, nil
		}
		if !errors.Is(err, lockfile.ErrHeld) || time.Now().After(deadline) {
			return nil, err
		}
		time.Sleep(50 * time.Millisecond)
	}
}
