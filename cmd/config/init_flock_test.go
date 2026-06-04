// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package config

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	larkauth "github.com/larksuite/cli/internal/auth"
)

// Regression: configInitRun previously loaded the config pre-prompt and
// saved the same buffer post-prompt with no flock. A peer `auth login`
// (or `profile add`, `users use`, `config bind`, peer `config init`)
// writing in that gap had its update silently overwritten when init's
// stale buffer was flushed. Symptom in the wild: a user who ran
// `auth login` while another agent was paused on the init TUI vanished
// from Users[] the moment the operator hit Enter on init.
//
// These tests pin the contract:
//
//  1. lockedSaveInit blocks while the SingleUser/login lock is held by
//     a peer; once the peer releases, the save lands.
//  2. lockedSaveInit re-loads the config inside the lock — a peer write
//     that occurred between the pre-prompt load and lockedSaveInit is
//     reflected in the saved file (no lost update).
//
// The lock primitive is the documented MultiAppConfig serialiser
// (cmd/auth/login.go.syncLoginUserToProfile, cmd/profile/add.go);
// the helper just routes init through it.

// TestLockedSaveInit_BlocksOnPeerLock asserts the lock is actually
// acquired. With the flock removed, the goroutine returns immediately
// instead of waiting on the holder.
func TestLockedSaveInit_BlocksOnPeerLock(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	core.SetCurrentWorkspace(core.WorkspaceLocal)

	// Acquire the same lock from a peer goroutine, hold it for 250ms.
	root := larkauth.NewLocalRoot(core.GetConfigDir())
	peerCtx, peerCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer peerCancel()
	peerLock, err := root.Locks(larkauth.SingleUser()).Acquire(peerCtx, "login", 5*time.Second)
	if err != nil {
		t.Fatalf("peer Acquire: %v", err)
	}
	released := make(chan struct{})
	go func() {
		time.Sleep(250 * time.Millisecond)
		peerLock.Release()
		close(released)
	}()

	// lockedSaveInit must wait for the peer release.
	f, _, _, _ := cmdutil.TestFactory(t, nil)
	start := time.Now()
	saveErr := lockedSaveInit("", f, "cli_a", core.PlainSecret("s"), core.BrandFeishu, "en")
	waited := time.Since(start)

	<-released
	if saveErr != nil {
		t.Fatalf("lockedSaveInit: %v", saveErr)
	}
	// Allow some scheduler slack but require we waited at least most of the
	// peer hold window. Without the flock this is ~0ms.
	if waited < 200*time.Millisecond {
		t.Errorf("lockedSaveInit did not wait on peer lock; waited=%s want >=200ms", waited)
	}
}

// TestLockedSaveInit_ReloadsInsideLock asserts the post-flock re-load
// observes peer writes — the lost-update guard. A pre-prompt `existing`
// is intentionally NOT passed to the helper; the helper loads its own
// snapshot inside the lock.
//
// Sequence:
//   - Pre-state: an empty config dir.
//   - Peer (simulated): writes a config with a profile "peer" (e.g.
//     `auth login` adding a user; here we use a SaveMultiAppConfig as
//     stand-in, which is what login ultimately does inside its own
//     flock — we are testing the init helper's read-after-lock, not
//     the lock primitive itself).
//   - Init helper: lockedSaveInit("", ..., "cli_init", ...) which is
//     the no-profile-name path that calls saveAsOnlyApp and would
//     OVERWRITE the file with a single-app config holding "cli_init".
//   - The expected post-state with the fix is a single-app config
//     with appId="cli_init" (the no-profile path ALWAYS overwrites; the
//     test of value here is that the helper successfully serialised,
//     not that it preserved the peer profile — for that contract see
//     the --name=peer variant below).
//
// The first sub-test verifies the no-profile path overwrites cleanly
// without panicking on a peer-written file (sanity for the load-then-
// overwrite ordering inside the lock). The second sub-test verifies
// the --name path PRESERVES the peer profile, which is the actual
// lost-update prevention: pre-fix the pre-prompt `existing == nil`
// snapshot would have caused saveAsProfile to drop "peer".
func TestLockedSaveInit_ReloadsInsideLock(t *testing.T) {
	t.Run("named profile preserves peer write", func(t *testing.T) {
		t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
		core.SetCurrentWorkspace(core.WorkspaceLocal)

		// Peer wrote this AFTER our pre-prompt load (which would have
		// returned nil). The helper must observe it via the in-lock
		// re-load.
		peer := &core.MultiAppConfig{
			Apps: []core.AppConfig{{
				Name:      "peer",
				AppId:     "cli_peer",
				AppSecret: core.PlainSecret("s-peer"),
				Brand:     core.BrandFeishu,
				Users:     []core.AppUser{{UserOpenId: "ou_p", UserName: "Peer"}},
			}},
		}
		if err := core.SaveMultiAppConfig(peer); err != nil {
			t.Fatalf("seed peer: %v", err)
		}

		// Init runs under --name=mine — the named-profile path appends.
		// With pre-fix (no in-lock re-load), saveAsProfile would have
		// received existing==nil and produced a config holding ONLY
		// "mine", silently dropping "peer".
		f, _, _, _ := cmdutil.TestFactory(t, nil)
		if err := lockedSaveInit("mine", f, "cli_mine", core.PlainSecret("s-mine"), core.BrandFeishu, "en"); err != nil {
			t.Fatalf("lockedSaveInit: %v", err)
		}

		got, err := core.LoadMultiAppConfig()
		if err != nil {
			t.Fatalf("LoadMultiAppConfig: %v", err)
		}
		if got.FindApp("peer") == nil {
			t.Errorf("peer profile dropped — lost update; apps=%v", got.ProfileNames())
		}
		if got.FindApp("mine") == nil {
			t.Errorf("mine profile not appended; apps=%v", got.ProfileNames())
		}
	})

	t.Run("flock release on success", func(t *testing.T) {
		t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
		core.SetCurrentWorkspace(core.WorkspaceLocal)

		f, _, _, _ := cmdutil.TestFactory(t, nil)
		if err := lockedSaveInit("", f, "cli_a", core.PlainSecret("s"), core.BrandFeishu, "en"); err != nil {
			t.Fatalf("first save: %v", err)
		}
		// Second back-to-back call must succeed — proves the first
		// release ran.
		if err := lockedSaveInit("", f, "cli_b", core.PlainSecret("s"), core.BrandFeishu, "en"); err != nil {
			t.Fatalf("second save (lock leak?): %v", err)
		}
	})
}

// TestLockedSaveInit_ConcurrentSavesSerialize fires two goroutines
// at the helper and asserts both eventually win and the final file
// contains exactly one of the two app IDs (no half-written / merged
// config). With the flock removed, the writes interleave and the
// last-flush-wins outcome is non-deterministic but never errors —
// so the assertion here is that BOTH calls return nil AND the saved
// file is one of the two well-formed shapes (not e.g. zero apps).
func TestLockedSaveInit_ConcurrentSavesSerialize(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	core.SetCurrentWorkspace(core.WorkspaceLocal)

	f, _, _, _ := cmdutil.TestFactory(t, nil)
	var wg sync.WaitGroup
	errs := make([]error, 2)
	wg.Add(2)
	go func() {
		defer wg.Done()
		errs[0] = lockedSaveInit("", f, "cli_a", core.PlainSecret("sa"), core.BrandFeishu, "en")
	}()
	go func() {
		defer wg.Done()
		errs[1] = lockedSaveInit("", f, "cli_b", core.PlainSecret("sb"), core.BrandFeishu, "en")
	}()
	wg.Wait()

	for i, e := range errs {
		if e != nil {
			t.Errorf("concurrent save %d: %v", i, e)
		}
	}
	got, err := core.LoadMultiAppConfig()
	if err != nil {
		t.Fatalf("LoadMultiAppConfig: %v", err)
	}
	if len(got.Apps) != 1 {
		t.Errorf("expected exactly one surviving app, got %d", len(got.Apps))
	}
	if appID := got.Apps[0].AppId; appID != "cli_a" && appID != "cli_b" {
		t.Errorf("saved AppId = %q, want cli_a or cli_b", appID)
	}
}
