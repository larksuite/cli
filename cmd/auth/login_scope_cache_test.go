// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

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

func TestPendingLoginCache_ResumesPerAppAndCleansUp(t *testing.T) {
	setupLoginConfigDir(t)

	if err := savePendingLogin("device-a", "cli_a", "scope:a", 600); err != nil {
		t.Fatalf("savePendingLogin(cli_a) error = %v", err)
	}
	if err := savePendingLogin("device-b", "cli_b", "scope:b", 600); err != nil {
		t.Fatalf("savePendingLogin(cli_b) error = %v", err)
	}

	a, err := loadPendingLogin("cli_a")
	if err != nil || a.DeviceCode != "device-a" || a.RequestedScope != "scope:a" {
		t.Fatalf("loadPendingLogin(cli_a) = (%+v, %v)", a, err)
	}
	b, err := loadPendingLogin("cli_b")
	if err != nil || b.DeviceCode != "device-b" || b.RequestedScope != "scope:b" {
		t.Fatalf("loadPendingLogin(cli_b) = (%+v, %v)", b, err)
	}

	if err := removePendingLogin("device-a", "cli_a"); err != nil {
		t.Fatalf("removePendingLogin(cli_a) error = %v", err)
	}
	if _, err := loadPendingLogin("cli_a"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("loadPendingLogin(cli_a) after cleanup error = %v, want not exist", err)
	}
	if b, err := loadPendingLogin("cli_b"); err != nil || b.DeviceCode != "device-b" {
		t.Fatalf("cli_b pending flow must survive cli_a cleanup: (%+v, %v)", b, err)
	}
}

func TestPendingLoginCache_ExpiredRecordIsRemoved(t *testing.T) {
	setupLoginConfigDir(t)

	record := pendingLoginRecord{
		DeviceCode:     "expired-device",
		AppID:          "cli_expired",
		RequestedScope: "scope:expired",
		ExpiresAt:      time.Now().Add(-time.Second).Unix(),
	}
	data, err := json.Marshal(record)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if err := vfs.MkdirAll(loginScopeCacheDir(), 0700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := vfs.WriteFile(pendingLoginPath(record.AppID), data, 0600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if _, err := loadPendingLogin(record.AppID); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("loadPendingLogin() error = %v, want not exist", err)
	}
	if _, err := vfs.Stat(pendingLoginPath(record.AppID)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Stat(pendingPath) error = %v, want not exist", err)
	}
}

func TestPendingLoginCache_LatestFlowForSameAppWins(t *testing.T) {
	setupLoginConfigDir(t)

	if err := savePendingLogin("device-first", "cli_same", "scope:first", 600); err != nil {
		t.Fatalf("savePendingLogin(first) error = %v", err)
	}
	if err := savePendingLogin("device-second", "cli_same", "scope:second", 600); err != nil {
		t.Fatalf("savePendingLogin(second) error = %v", err)
	}

	got, err := loadPendingLogin("cli_same")
	if err != nil {
		t.Fatalf("loadPendingLogin() error = %v", err)
	}
	if got.DeviceCode != "device-second" || got.RequestedScope != "scope:second" {
		t.Fatalf("loadPendingLogin() = %+v, want latest flow", got)
	}
}
