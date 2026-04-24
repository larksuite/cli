// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package event

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// TestDiscoverAppIDs_OnlyDirsWithSocket verifies `--all` discovery returns
// exactly the appID subdirs that have a bus.sock file — skipping dirs
// without a socket (stale bus.log / bus.fork.lock leftovers) and ignoring
// non-dir entries. Relies on vfs.ReadDir / vfs.Stat so swapping os.* at
// line 258/268 in stop.go was a hard prerequisite.
func TestDiscoverAppIDs_OnlyDirsWithSocket(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", tmp)

	eventsDir := filepath.Join(tmp, "events")
	if err := os.MkdirAll(eventsDir, 0700); err != nil {
		t.Fatal(err)
	}

	// Two app dirs with a socket: should be returned.
	for _, app := range []string{"cli_XXXXXXXXXXXXXXXX", "cli_YYYYYYYYYYYYYYYY"} {
		appDir := filepath.Join(eventsDir, app)
		if err := os.MkdirAll(appDir, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(appDir, "bus.sock"), nil, 0600); err != nil {
			t.Fatal(err)
		}
	}

	// App dir with log but no socket: skipped (stopped bus leftover).
	stoppedDir := filepath.Join(eventsDir, "cli_ZZZZZZZZZZZZZZZZ")
	if err := os.MkdirAll(stoppedDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stoppedDir, "bus.log"), []byte("stale"), 0600); err != nil {
		t.Fatal(err)
	}

	// A stray file under events/ (not a dir): skipped.
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

// TestDiscoverAppIDs_MissingEventsDir verifies a clean install (no events/
// dir yet) returns nil without error — `event stop --all` on a fresh
// machine should print "No event bus instances found.", not crash.
func TestDiscoverAppIDs_MissingEventsDir(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	if got := discoverAppIDs(); got != nil {
		t.Errorf("discoverAppIDs() on missing events/ = %v, want nil", got)
	}
}
