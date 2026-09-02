// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"path/filepath"
	"testing"
	"time"

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

func TestStoredTokenGenerationGuard(t *testing.T) {
	setupStoredTokenTest(t)
	now := time.Now()
	generation0 := &StoredUAToken{
		AppId:            "cli_generation_guard",
		UserOpenId:       "ou_generation_guard",
		AccessToken:      "access-g0",
		RefreshToken:     "refresh-g0",
		ExpiresAt:        now.Add(-time.Minute).UnixMilli(),
		RefreshExpiresAt: now.Add(time.Hour).UnixMilli(),
	}
	generation1 := *generation0
	generation1.AccessToken = "access-g1"
	generation1.RefreshToken = "refresh-g1"
	generation2 := generation1
	generation2.AccessToken = "access-g2"
	generation2.RefreshToken = "refresh-g2"

	if err := SetStoredToken(generation0); err != nil {
		t.Fatalf("SetStoredToken(generation 0) error = %v", err)
	}
	withLock := func(fn func() error) {
		t.Helper()
		if err := withTokenStorageLock(generation0.AppId, generation0.UserOpenId, fn); err != nil {
			t.Fatalf("withTokenStorageLock() error = %v", err)
		}
	}

	withLock(func() error {
		current, swapped, err := compareAndSwapStoredToken(
			generation0.AppId, generation0.UserOpenId, generation0, &generation1,
		)
		if err == nil && (!swapped || current == nil || current.RefreshToken != generation1.RefreshToken) {
			t.Fatalf("matching swap = (%#v, %v), want generation 1 stored", current, swapped)
		}
		return err
	})
	if err := SetStoredToken(&generation2); err != nil {
		t.Fatalf("SetStoredToken(generation 2) error = %v", err)
	}

	withLock(func() error {
		current, swapped, err := compareAndSwapStoredToken(
			generation0.AppId, generation0.UserOpenId, &generation1, generation0,
		)
		if err == nil && (swapped || current == nil || current.RefreshToken != generation2.RefreshToken) {
			t.Fatalf("stale swap = (%#v, %v), want generation 2 retained", current, swapped)
		}
		return err
	})
	withLock(func() error {
		current, deleted, err := compareAndDeleteStoredToken(
			generation0.AppId, generation0.UserOpenId, &generation1,
		)
		if err == nil && (deleted || current == nil || current.RefreshToken != generation2.RefreshToken) {
			t.Fatalf("stale delete = (%#v, %v), want generation 2 retained", current, deleted)
		}
		return err
	})
	withLock(func() error {
		current, deleted, err := compareAndDeleteStoredToken(
			generation0.AppId, generation0.UserOpenId, &generation2,
		)
		if err == nil && (!deleted || current != nil) {
			t.Fatalf("matching delete = (%#v, %v), want token removed", current, deleted)
		}
		return err
	})
	if current := GetStoredToken(generation0.AppId, generation0.UserOpenId); current != nil {
		t.Fatalf("stored token = %#v, want removed", current)
	}
}
