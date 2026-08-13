// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build extended

package extendedupdate

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/larksuite/cli/internal/vfs"
)

func TestCheckCachedUsesExtendedEditionState(t *testing.T) {
	for _, key := range []string{
		"LARKSUITE_CLI_NO_UPDATE_NOTIFIER",
		"CI",
		"BUILD_NUMBER",
		"RUN_ID",
	} {
		t.Setenv(key, "")
	}
	dir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", dir)

	standardState, err := json.Marshal(cachedRelease{
		LatestVersion: "9.0.0",
		CheckedAt:     time.Now().Unix(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := vfs.WriteFile(filepath.Join(dir, "update-state.json"), standardState, 0o600); err != nil {
		t.Fatal(err)
	}
	if got := CheckCached("1.0.0"); got != nil {
		t.Fatalf("Standard cache leaked into Extended notice: %+v", got)
	}

	extendedState, err := json.Marshal(cachedRelease{
		LatestVersion: "2.0.0",
		CheckedAt:     time.Now().Unix(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := vfs.WriteFile(filepath.Join(dir, extendedStateFile), extendedState, 0o600); err != nil {
		t.Fatal(err)
	}
	got := CheckCached("1.0.0")
	if got == nil || got.Current != "1.0.0" || got.Latest != "2.0.0" {
		t.Fatalf("CheckCached() = %+v, want Extended 1.0.0 -> 2.0.0", got)
	}
}
