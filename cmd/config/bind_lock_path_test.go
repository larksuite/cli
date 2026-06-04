// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
)

// TestConfigBindRun_LockPath_FollowsTargetWorkspace pins the post-fix invariant:
// configBindRun's flock MUST land under the TARGET workspace's locks dir, not
// the workspace the process started in. Pre-fix, NewLocalRoot was constructed
// with core.GetConfigDir() before SetCurrentWorkspace, so a `bind --source X`
// invoked from a Local shell took the lock at <base>/locks/login.lock while
// commitBinding wrote <base>/X/config.json — leaving a peer `auth login` in
// workspace X (which takes <base>/X/locks/login.lock) free to race the same
// target file.
//
// We assert the post-fix layout by inspecting the locks/ directory tree after
// the bind run: the TARGET workspace's locks/ MUST exist (gofrs/flock creates
// the file lazily on Acquire and leaves it behind after Release), while the
// SOURCE/<base>/locks/ MUST NOT have a login.lock file from this run.
//
// Hermes is the cleanest fixture because its env-detection path (HERMES_HOME
// + .env) is already exercised by the existing bind suite.
func TestConfigBindRun_LockPath_FollowsTargetWorkspace(t *testing.T) {
	cases := []struct {
		name      string
		source    string
		wantSubdir string // relative to configDir
	}{
		{
			name:       "hermes",
			source:     "hermes",
			wantSubdir: "hermes",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			saveWorkspace(t)
			configDir := t.TempDir()
			t.Setenv("LARKSUITE_CLI_CONFIG_DIR", configDir)
			t.Setenv("HOME", t.TempDir()) // keep keychain mocks isolated
			clearAgentEnv(t)

			hermesHome := t.TempDir()
			t.Setenv("HERMES_HOME", hermesHome)
			if err := os.WriteFile(
				filepath.Join(hermesHome, ".env"),
				[]byte("FEISHU_APP_ID=cli_lockpath\nFEISHU_APP_SECRET=lock_secret\n"),
				0600,
			); err != nil {
				t.Fatalf("write .env: %v", err)
			}

			f, _, _, _ := cmdutil.TestFactory(t, nil)
			if err := configBindRun(&BindOptions{Factory: f, Source: tc.source}); err != nil {
				t.Fatalf("configBindRun: %v", err)
			}

			// gofrs/flock leaves the lock file on disk after Release; presence of
			// login.lock is therefore a faithful witness of where Acquire was rooted.
			targetLock := filepath.Join(configDir, tc.wantSubdir, "locks", "login.lock")
			if _, err := os.Stat(targetLock); err != nil {
				t.Errorf("target-workspace login.lock missing at %q: %v\n"+
					"expected the bind flock to be rooted on the target workspace, "+
					"not the workspace the process started in", targetLock, err)
			}

			// The SOURCE workspace (Local in this fixture) MUST NOT carry a
			// login.lock from this bind run. If it does, the flock was acquired
			// pre-SetCurrentWorkspace and the cross-workspace serialisation
			// guarantee is broken.
			sourceLock := filepath.Join(configDir, "locks", "login.lock")
			if _, err := os.Stat(sourceLock); err == nil {
				t.Errorf("source-workspace login.lock present at %q; "+
					"flock was acquired BEFORE SetCurrentWorkspace, regressing "+
					"the cross-workspace serialisation fix", sourceLock)
			}
		})
	}
}

// TestConfigBindRun_LockPath_OpenClaw mirrors the Hermes test for the OpenClaw
// auto-detect path. Kept separate because the env-detection fixtures differ
// (OPENCLAW_HOME + openclaw.json vs HERMES_HOME + .env), and the failure mode
// for one is not symptomatic of the other.
func TestConfigBindRun_LockPath_OpenClaw(t *testing.T) {
	saveWorkspace(t)
	configDir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", configDir)
	t.Setenv("HOME", t.TempDir())
	clearAgentEnv(t)

	openclawHome := t.TempDir()
	t.Setenv("OPENCLAW_HOME", openclawHome)
	t.Setenv("OPENCLAW_CONFIG_PATH", "")
	t.Setenv("OPENCLAW_STATE_DIR", "")

	openclawDir := filepath.Join(openclawHome, ".openclaw")
	if err := os.MkdirAll(openclawDir, 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	cfg := `{"channels":{"feishu":{"appId":"cli_lockpath_oc","appSecret":"lock_oc_secret","domain":"feishu"}}}`
	if err := os.WriteFile(filepath.Join(openclawDir, "openclaw.json"), []byte(cfg), 0600); err != nil {
		t.Fatalf("write openclaw.json: %v", err)
	}

	f, _, _, _ := cmdutil.TestFactory(t, nil)
	if err := configBindRun(&BindOptions{Factory: f, Source: "openclaw"}); err != nil {
		t.Fatalf("configBindRun: %v", err)
	}

	targetLock := filepath.Join(configDir, "openclaw", "locks", "login.lock")
	if _, err := os.Stat(targetLock); err != nil {
		t.Errorf("openclaw login.lock missing at %q: %v", targetLock, err)
	}

	sourceLock := filepath.Join(configDir, "locks", "login.lock")
	if _, err := os.Stat(sourceLock); err == nil {
		t.Errorf("source-workspace login.lock present at %q; cross-workspace "+
			"serialisation fix has regressed", sourceLock)
	}
}
