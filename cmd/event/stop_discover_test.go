// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package event

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/larksuite/cli/internal/event/busdiscover"
)

func TestDiscoverAppIDs_OnlyLiveLockHolders(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", tmp)

	eventsDir := filepath.Join(tmp, "events")

	// Two live buses (lock held until t.Cleanup releases it).
	for _, app := range []string{"cli_XXXXXXXXXXXXXXXX", "cli_YYYYYYYYYYYYYYYY"} {
		appDir := filepath.Join(eventsDir, app)
		h, err := busdiscover.WritePIDFile(appDir, 1234)
		if err != nil {
			t.Fatalf("WritePIDFile %s: %v", app, err)
		}
		t.Cleanup(func() { _ = h.Release() })
	}

	// Dead bus: lock acquired then released → looks like a stale dir on disk.
	deadDir := filepath.Join(eventsDir, "cli_ZZZZZZZZZZZZZZZZ")
	hDead, err := busdiscover.WritePIDFile(deadDir, 9999)
	if err != nil {
		t.Fatalf("WritePIDFile dead: %v", err)
	}
	if err := hDead.Release(); err != nil {
		t.Fatalf("Release dead: %v", err)
	}

	// Stale bus.sock without alive.lock — old behavior would surface it; new must not.
	staleSockDir := filepath.Join(eventsDir, "cli_SSSSSSSSSSSSSSSS")
	if err := os.MkdirAll(staleSockDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staleSockDir, "bus.sock"), nil, 0600); err != nil {
		t.Fatal(err)
	}

	// Stray non-dir file under events/.
	if err := os.WriteFile(filepath.Join(eventsDir, "stray.txt"), nil, 0600); err != nil {
		t.Fatal(err)
	}

	got := discoverAppIDs()
	sort.Strings(got)
	want := []string{"cli_XXXXXXXXXXXXXXXX", "cli_YYYYYYYYYYYYYYYY"}
	if len(got) != len(want) {
		t.Fatalf("discoverAppIDs() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("discoverAppIDs()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestDiscoverAppIDs_MissingEventsDir(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	if got := discoverAppIDs(); len(got) != 0 {
		t.Errorf("discoverAppIDs() on missing events/ = %v, want empty", got)
	}
}
