// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

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

func lockURL(url string) func() {
	actual, _ := urlLocks.LoadOrStore(url, &sync.Mutex{})
	mu := actual.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

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
