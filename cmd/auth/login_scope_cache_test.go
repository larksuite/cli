// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"errors"
	"os"
	"testing"

	"github.com/larksuite/cli/internal/vfs"
)

func TestLoginRequestedScopeCache_RoundTrip(t *testing.T) {
	setupLoginConfigDir(t)

	deviceCode := "device/code:123"
	requestedScope := "im:message:send im:message:reply"

	if err := saveLoginRequestedScope(deviceCode, requestedScope); err != nil {
		t.Fatalf("saveLoginRequestedScope() error = %v", err)
	}
	got, err := loadLoginRequestedScope(deviceCode)
	if err != nil {
		t.Fatalf("loadLoginRequestedScope() error = %v", err)
	}
	if got != requestedScope {
		t.Fatalf("requestedScope = %q, want %q", got, requestedScope)
	}
	if _, err := vfs.Stat(loginScopeCachePath(deviceCode)); err != nil {
		t.Fatalf("Stat(cachePath) error = %v", err)
	}
	if err := removeLoginRequestedScope(deviceCode); err != nil {
		t.Fatalf("removeLoginRequestedScope() error = %v", err)
	}
	if _, err := vfs.Stat(loginScopeCachePath(deviceCode)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Stat(cachePath) error = %v, want not exist", err)
	}
}

func TestLoadLoginRequestedScope_MissingReturnsEmpty(t *testing.T) {
	setupLoginConfigDir(t)

	got, err := loadLoginRequestedScope("missing-device-code")
	if err != nil {
		t.Fatalf("loadLoginRequestedScope() error = %v", err)
	}
	if got != "" {
		t.Fatalf("requestedScope = %q, want empty", got)
	}
}
