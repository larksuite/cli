// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/zalando/go-keyring"
)

func setupStoredTokenTest(t *testing.T) {
	t.Helper()
	root := t.TempDir()
	t.Setenv("HOME", filepath.Join(root, "home"))
	t.Setenv("LARKSUITE_CLI_DATA_DIR", filepath.Join(root, "data"))
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", filepath.Join(root, "config"))
	keyring.MockInit()
}

func compareAndSwapStoredTokenForTest(expected, updated *StoredUAToken) (*StoredUAToken, bool, error) {
	var current *StoredUAToken
	var swapped bool
	err := withTokenStorageLock(expected.AppId, expected.UserOpenId, func() error {
		var err error
		current, swapped, err = compareAndSwapStoredToken(expected.AppId, expected.UserOpenId, expected, updated)
		return err
	})
	return current, swapped, err
}

func compareAndDeleteStoredTokenForTest(expected *StoredUAToken) (*StoredUAToken, bool, error) {
	var current *StoredUAToken
	var deleted bool
	err := withTokenStorageLock(expected.AppId, expected.UserOpenId, func() error {
		var err error
		current, deleted, err = compareAndDeleteStoredToken(expected.AppId, expected.UserOpenId, expected)
		return err
	})
	return current, deleted, err
}

func TestTokenStorageLockPathMatchesLegacyRefreshConvention(t *testing.T) {
	setupStoredTokenTest(t)

	tests := []struct {
		name       string
		appID      string
		userOpenID string
		want       string
	}{
		{
			name:       "safe identifiers",
			appID:      "cli_test-app.1",
			userOpenID: "ou_test-user.1",
			want:       "refresh_cli_test-app.1_ou_test-user.1.lock",
		},
		{
			name:       "unsafe characters are replaced",
			appID:      "cli/test:app",
			userOpenID: "ou/test:user",
			want:       "refresh_cli_test_app_ou_test_user.lock",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := filepath.Base(tokenStorageLockPath(tt.appID, tt.userOpenID)); got != tt.want {
				t.Fatalf("tokenStorageLockPath() filename = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestStoredTokenMutationsRejectDifferentAccount(t *testing.T) {
	setupStoredTokenTest(t)

	foreign := &StoredUAToken{
		AppId:        "cli_foreign",
		UserOpenId:   "ou_foreign",
		AccessToken:  "access-foreign",
		RefreshToken: "refresh-foreign",
	}
	tests := []struct {
		name      string
		operation func() error
	}{
		{
			name: "set",
			operation: func() error {
				return writeStoredToken("cli_locked", "ou_locked", foreign)
			},
		},
		{
			name: "compare and swap",
			operation: func() error {
				_, _, err := compareAndSwapStoredToken("cli_locked", "ou_locked", foreign, foreign)
				return err
			},
		},
		{
			name: "compare and delete",
			operation: func() error {
				_, _, err := compareAndDeleteStoredToken("cli_locked", "ou_locked", foreign)
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := withTokenStorageLock("cli_locked", "ou_locked", tt.operation)
			problem, ok := errs.ProblemOf(err)
			if !ok {
				t.Fatalf("withTokenStorageLock() error = %v, want typed error", err)
			}
			if problem.Category != errs.CategoryInternal || problem.Subtype != errs.SubtypeStorage {
				t.Fatalf("problem = (%q, %q), want (%q, %q)",
					problem.Category, problem.Subtype, errs.CategoryInternal, errs.SubtypeStorage)
			}
		})
	}
	if got := GetStoredToken(foreign.AppId, foreign.UserOpenId); got != nil {
		t.Fatalf("foreign token was stored: %#v", got)
	}
}

func TestStoredTokenCompareAndSwapAndDelete(t *testing.T) {
	setupStoredTokenTest(t)

	generation0 := &StoredUAToken{
		AppId:        "cli_generation_guard_test",
		UserOpenId:   "ou_generation_guard_test",
		AccessToken:  "access-g0",
		RefreshToken: "refresh-g0",
	}
	generation1 := &StoredUAToken{
		AppId:        generation0.AppId,
		UserOpenId:   generation0.UserOpenId,
		AccessToken:  "access-g1",
		RefreshToken: "refresh-g1",
	}
	generation2 := &StoredUAToken{
		AppId:        generation0.AppId,
		UserOpenId:   generation0.UserOpenId,
		AccessToken:  "access-g2",
		RefreshToken: "refresh-g2",
	}

	if err := SetStoredToken(generation0); err != nil {
		t.Fatalf("SetStoredToken(generation 0) error = %v", err)
	}
	current, swapped, err := compareAndSwapStoredTokenForTest(generation0, generation1)
	if err != nil {
		t.Fatalf("compareAndSwap() error = %v", err)
	}
	if !swapped || current == nil || current.RefreshToken != generation1.RefreshToken {
		t.Fatalf("swap result = (%#v, %v), want generation 1 stored", current, swapped)
	}

	if err := SetStoredToken(generation2); err != nil {
		t.Fatalf("SetStoredToken(generation 2) error = %v", err)
	}
	current, swapped, err = compareAndSwapStoredTokenForTest(generation0, generation1)
	if err != nil {
		t.Fatalf("compareAndSwap(stale generation) error = %v", err)
	}
	if swapped || current == nil || current.RefreshToken != generation2.RefreshToken {
		t.Fatalf("stale swap result = (%#v, %v), want generation 2 retained", current, swapped)
	}

	current, deleted, err := compareAndDeleteStoredTokenForTest(generation0)
	if err != nil {
		t.Fatalf("compareAndDelete(stale generation) error = %v", err)
	}
	if deleted || current == nil || current.RefreshToken != generation2.RefreshToken {
		t.Fatalf("stale delete result = (%#v, %v), want generation 2 retained", current, deleted)
	}

	current, deleted, err = compareAndDeleteStoredTokenForTest(generation2)
	if err != nil {
		t.Fatalf("compareAndDelete() error = %v", err)
	}
	if !deleted || current != nil || GetStoredToken(generation2.AppId, generation2.UserOpenId) != nil {
		t.Fatalf("matching delete result = (%#v, %v), want token removed", current, deleted)
	}
}

func TestStoredTokenMutationsUseSharedLock(t *testing.T) {
	setupStoredTokenTest(t)

	tests := []struct {
		name      string
		operation func(expected, updated *StoredUAToken) error
	}{
		{
			name: "set",
			operation: func(_, updated *StoredUAToken) error {
				return SetStoredToken(updated)
			},
		},
		{
			name: "remove",
			operation: func(expected, _ *StoredUAToken) error {
				return RemoveStoredToken(expected.AppId, expected.UserOpenId)
			},
		},
		{
			name: "compare and swap",
			operation: func(expected, updated *StoredUAToken) error {
				_, _, err := compareAndSwapStoredTokenForTest(expected, updated)
				return err
			},
		},
		{
			name: "compare and delete",
			operation: func(expected, _ *StoredUAToken) error {
				_, _, err := compareAndDeleteStoredTokenForTest(expected)
				return err
			},
		},
	}

	for index, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			userOpenID := "ou_shared_lock_" + string(rune('a'+index))
			expected := &StoredUAToken{
				AppId:        "cli_shared_lock_test",
				UserOpenId:   userOpenID,
				AccessToken:  "access-g0",
				RefreshToken: "refresh-g0",
			}
			updated := &StoredUAToken{
				AppId:        expected.AppId,
				UserOpenId:   userOpenID,
				AccessToken:  "access-g1",
				RefreshToken: "refresh-g1",
			}
			if err := SetStoredToken(expected); err != nil {
				t.Fatalf("SetStoredToken(expected) error = %v", err)
			}

			processLock := tokenStorageProcessLock(expected.AppId, expected.UserOpenId)
			processLock.Lock()
			released := false
			defer func() {
				if !released {
					processLock.Unlock()
				}
			}()

			done := make(chan error, 1)
			started := make(chan struct{})
			go func() {
				close(started)
				done <- tt.operation(expected, updated)
			}()
			<-started

			select {
			case err := <-done:
				processLock.Unlock()
				released = true
				t.Fatalf("mutation completed without waiting for shared lock: %v", err)
			case <-time.After(3 * tokenStorageLockRetryDelay):
			}

			processLock.Unlock()
			released = true
			select {
			case err := <-done:
				if err != nil {
					t.Fatalf("mutation error after lock release = %v", err)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("mutation did not finish after shared lock was released")
			}
		})
	}
}

func TestRefreshWithLockRereadsCurrentGeneration(t *testing.T) {
	setupStoredTokenTest(t)
	now := time.Now()
	current := &StoredUAToken{
		AppId:            "cli_refresh_reread_test",
		UserOpenId:       "ou_refresh_reread_test",
		AccessToken:      "access-current",
		RefreshToken:     "refresh-current",
		ExpiresAt:        now.Add(time.Hour).UnixMilli(),
		RefreshExpiresAt: now.Add(24 * time.Hour).UnixMilli(),
	}
	if err := SetStoredToken(current); err != nil {
		t.Fatalf("SetStoredToken() error = %v", err)
	}

	httpCalled := false
	httpClient := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		httpCalled = true
		return nil, errors.New("unexpected refresh request")
	})}
	got, err := refreshWithLock(httpClient, UATCallOptions{
		AppId:      current.AppId,
		UserOpenId: current.UserOpenId,
	})
	if err != nil {
		t.Fatalf("refreshWithLock() error = %v", err)
	}
	if got == nil || got.RefreshToken != current.RefreshToken {
		t.Fatalf("refreshWithLock() = %#v, want current generation", got)
	}
	if httpCalled {
		t.Fatal("refresh endpoint was called for the current valid generation")
	}
}

func TestRefreshWithLockCoalescesConcurrentSuccessfulRefreshes(t *testing.T) {
	setupStoredTokenTest(t)
	now := time.Now()
	stored := &StoredUAToken{
		AppId:            "cli_refresh_coalescing_test",
		UserOpenId:       "ou_refresh_coalescing_test",
		AccessToken:      "access-old",
		RefreshToken:     "refresh-old",
		ExpiresAt:        now.Add(-time.Minute).UnixMilli(),
		RefreshExpiresAt: now.Add(time.Hour).UnixMilli(),
	}
	if err := SetStoredToken(stored); err != nil {
		t.Fatalf("SetStoredToken() error = %v", err)
	}

	var requestCount atomic.Int32
	requestStarted := make(chan struct{})
	releaseRequest := make(chan struct{})
	httpClient := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if requestCount.Add(1) == 1 {
			close(requestStarted)
			<-releaseRequest
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body: io.NopCloser(strings.NewReader(
				`{"code":0,"access_token":"access-new","refresh_token":"refresh-new","expires_in":7200,"refresh_token_expires_in":86400}`)),
			Request: req,
		}, nil
	})}

	const callers = 10
	type refreshCallResult struct {
		token *StoredUAToken
		err   error
	}
	start := make(chan struct{})
	results := make(chan refreshCallResult, callers)
	var ready sync.WaitGroup
	ready.Add(callers)
	for range callers {
		go func() {
			ready.Done()
			<-start
			token, err := refreshWithLock(httpClient, UATCallOptions{
				AppId:      stored.AppId,
				AppSecret:  "test-secret",
				UserOpenId: stored.UserOpenId,
			})
			results <- refreshCallResult{token: token, err: err}
		}()
	}
	ready.Wait()
	close(start)
	select {
	case <-requestStarted:
		close(releaseRequest)
	case <-time.After(2 * time.Second):
		close(releaseRequest)
		t.Fatal("refresh request did not start")
	}

	for range callers {
		result := <-results
		if result.err != nil {
			t.Fatalf("refreshWithLock() error = %v", result.err)
		}
		if result.token == nil || result.token.AccessToken != "access-new" {
			t.Fatalf("refreshWithLock() token = %#v, want refreshed token", result.token)
		}
	}
	if got := requestCount.Load(); got != 1 {
		t.Fatalf("refresh request count = %d, want 1", got)
	}
}
