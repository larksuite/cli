// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package gitcred manages the lifecycle of app Git credentials.
//
// Lock ordering convention — read this before adding any new lock acquisition:
//
//	ALWAYS acquire lockApp BEFORE lockURL. Never invert this order.
//
// Rationale:
//   - lockApp is a cross-process file lock with bounded timeout (2s) and a
//     possible setup error; acquiring it first keeps the failure surface
//     outside any in-process lock and avoids holding the in-process mutex
//     while waiting on I/O / another process.
//   - lockURL is an in-process sync.Mutex that never fails and blocks
//     indefinitely; holding it while waiting on lockApp would risk
//     deadlocking with a concurrent goroutine that held lockApp first.
//
// Paths that only manipulate per-app state (Init, Remove, Erase) only need
// lockApp. Get() is the only path that touches per-URL state in addition to
// per-app state, so it is the only caller that takes both locks.
package gitcred

import (
	"errors"
	"path/filepath"
	"regexp"
	"sync"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/lockfile"
	"github.com/larksuite/cli/internal/vfs" //nolint:depguard // git credential locks live under CLI config dir and are not user file I/O.
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
	dir := filepath.Join(core.GetConfigDir(), "locks")
	if err := vfs.MkdirAll(dir, 0700); err != nil {
		return nil, errs.NewInternalError(errs.SubtypeStorage, "create Git credential lock dir: %v", err).WithCause(err)
	}
	name := "apps_git_credential_" + safeLockNameChars.ReplaceAllString(appID, "_") + ".lock"
	lock := lockfile.New(filepath.Join(dir, filepath.Base(name)))
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
