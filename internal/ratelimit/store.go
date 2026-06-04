// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package ratelimit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/lockfile"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/internal/vfs"
)

const stateVersion = 1

var (
	lockWaitTimeout  = 5 * time.Second
	lockRetryInitial = 10 * time.Millisecond
	lockRetryMax     = 50 * time.Millisecond
	stateGCInterval  = time.Hour
	stateGCGrace     = time.Minute
	stateGCMu        sync.Mutex
	stateGCLastRun   = map[string]time.Time{}
)

type stateFile struct {
	dir string
}

type keyState struct {
	Version int     `json:"version"`
	Entries []int64 `json:"entries"`
}

var stateKeyRe = regexp.MustCompile(`^[a-f0-9]{64}$`)

func defaultStateFile() *stateFile {
	// Runtime dir is workspace-aware by design; local rate limit state is shared
	// across processes in the same lark-cli workspace/profile runtime.
	return newStateFile(filepath.Join(core.GetRuntimeDir(), "ratelimit"))
}

func newStateFile(dir string) *stateFile {
	return &stateFile{dir: dir}
}

func (s *stateFile) WithKeyLock(ctx context.Context, key string, fn func([]int64) ([]int64, error)) error {
	if key == "" {
		return internalStateError("rate limit key is empty")
	}
	if err := vfs.MkdirAll(s.dir, 0700); err != nil {
		return internalStateError("create rate limit state dir: %v", err)
	}
	s.maybeGC(time.Now())
	statePath, lockPath := s.pathsForKey(key)
	lock, err := s.lockKey(ctx, lockPath)
	if err != nil {
		return err
	}
	defer lock.Unlock() //nolint:errcheck // best-effort release; operation result is already decided.

	entries, err := s.loadKeyState(statePath)
	if err != nil {
		return err
	}
	entries, err = fn(entries)
	if err != nil {
		return err
	}
	return s.saveKeyState(statePath, entries)
}

func (s *stateFile) lockKey(ctx context.Context, lockPath string) (*lockfile.LockFile, error) {
	lock := lockfile.New(lockPath)
	lockCtx, cancel := context.WithTimeout(ctx, lockWaitTimeout)
	defer cancel()
	delay := lockRetryInitial
	for {
		if err := lock.TryLock(); err != nil {
			if !errors.Is(err, lockfile.ErrHeld) {
				return nil, internalStateError("lock rate limit state: %v", err)
			}
			select {
			case <-lockCtx.Done():
				if ctx.Err() != nil {
					return nil, ctx.Err()
				}
				return nil, internalStateError("timed out waiting for rate limit state lock")
			case <-time.After(delay):
				if delay < lockRetryMax {
					delay += lockRetryInitial
					if delay > lockRetryMax {
						delay = lockRetryMax
					}
				}
				continue
			}
		}
		return lock, nil
	}
}

func (s *stateFile) pathsForKey(key string) (string, string) {
	safe := key
	if !stateKeyRe.MatchString(key) {
		sum := sha256.Sum256([]byte(key))
		safe = hex.EncodeToString(sum[:])
	}
	return filepath.Join(s.dir, safe+".json"), filepath.Join(s.dir, safe+".lock")
}

func (s *stateFile) maybeGC(now time.Time) {
	stateGCMu.Lock()
	last := stateGCLastRun[s.dir]
	if !last.IsZero() && now.Sub(last) < stateGCInterval {
		stateGCMu.Unlock()
		return
	}
	stateGCLastRun[s.dir] = now
	stateGCMu.Unlock()

	s.gcExpired(now)
}

func (s *stateFile) gcExpired(now time.Time) {
	entries, err := vfs.ReadDir(s.dir)
	if err != nil {
		return
	}
	maxAge := maxRuleWindow(builtinRules) + stateGCGrace
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || filepath.Ext(name) != ".json" {
			continue
		}
		key := strings.TrimSuffix(name, ".json")
		if !stateKeyRe.MatchString(key) {
			continue
		}
		info, err := entry.Info()
		if err != nil || now.Sub(info.ModTime()) < maxAge {
			continue
		}
		statePath, lockPath := s.pathsForKey(key)
		lock := lockfile.New(lockPath)
		if err := lock.TryLock(); err != nil {
			continue
		}
		func() {
			defer lock.Unlock() //nolint:errcheck // best-effort cleanup.
			info, err := vfs.Stat(statePath)
			if err != nil || now.Sub(info.ModTime()) < maxAge {
				return
			}
			_ = vfs.Remove(statePath)
		}()
	}
}

func (s *stateFile) loadKeyState(path string) ([]int64, error) {
	data, err := vfs.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, internalStateError("read rate limit state: %v", err)
	}
	var st keyState
	if err := json.Unmarshal(data, &st); err != nil {
		return nil, corruptStateError(err)
	}
	if st.Version != stateVersion {
		return nil, corruptStateError(fmt.Errorf("unsupported version %d", st.Version))
	}
	return st.Entries, nil
}

func (s *stateFile) saveKeyState(path string, entries []int64) error {
	if len(entries) == 0 {
		if err := vfs.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return internalStateError("remove empty rate limit state: %v", err)
		}
		return nil
	}
	data, err := json.MarshalIndent(keyState{Version: stateVersion, Entries: entries}, "", "  ")
	if err != nil {
		return internalStateError("encode rate limit state: %v", err)
	}
	data = append(data, '\n')
	if err := validate.AtomicWrite(path, data, 0600); err != nil {
		return internalStateError("write rate limit state: %v", err)
	}
	return nil
}

func internalStateError(format string, args ...any) error {
	return output.ErrWithHint(output.ExitInternal, "internal",
		fmt.Sprintf(format, args...),
		"delete ratelimit/*.json under the lark-cli runtime directory and retry")
}

func corruptStateError(err error) error {
	return output.ErrWithHint(output.ExitInternal, "internal",
		fmt.Sprintf("rate limit state is invalid: %v", err),
		"delete ratelimit/*.json under the lark-cli runtime directory and retry")
}
