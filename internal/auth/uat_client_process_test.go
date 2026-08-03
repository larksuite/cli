// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build darwin || linux

package auth

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gofrs/flock"
)

func TestRefreshLockIsSharedAcrossProcesses(t *testing.T) {
	if mode := os.Getenv("LARK_CLI_REFRESH_LOCK_HELPER"); mode != "" {
		runRefreshLockHelper(t, mode)
		return
	}

	tempDir := t.TempDir()
	marker := filepath.Join(tempDir, "locked")
	release := filepath.Join(tempDir, "release")
	holderResult := filepath.Join(tempDir, "holder-result")
	probeResult := filepath.Join(tempDir, "probe-result")
	holder := refreshLockHelperCommand(t, "hold", filepath.Join(tempDir, "config-a"), tempDir, marker, release, holderResult)
	if err := holder.Start(); err != nil {
		t.Fatalf("AC6: start lock holder: %v", err)
	}
	defer func() { _ = holder.Process.Kill() }()

	deadline := time.Now().Add(3 * time.Second)
	for {
		if _, err := os.Stat(marker); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("AC6: lock holder did not acquire credential-scoped lock")
		}
		time.Sleep(10 * time.Millisecond)
	}

	probe := refreshLockHelperCommand(t, "probe", filepath.Join(tempDir, "config-b"), tempDir, marker, release, probeResult)
	if output, err := probe.CombinedOutput(); err != nil {
		t.Fatalf("AC6: lock probe failed: %v\n%s", err, output)
	}
	if err := os.WriteFile(release, []byte("release"), 0600); err != nil {
		t.Fatalf("AC6: release lock holder: %v", err)
	}
	if err := holder.Wait(); err != nil {
		t.Fatalf("AC6: lock holder failed: %v", err)
	}

	holderValue, err := os.ReadFile(holderResult)
	if err != nil {
		t.Fatalf("AC6: read holder result: %v", err)
	}
	probeValue, err := os.ReadFile(probeResult)
	if err != nil {
		t.Fatalf("AC6: read probe result: %v", err)
	}
	if string(probeValue) != "blocked:"+string(holderValue) {
		t.Fatalf("AC6: config-isolated processes did not contend on one lock: holder=%q probe=%q", holderValue, probeValue)
	}
}

func refreshLockHelperCommand(t *testing.T, mode, configDir, homeDir, marker, release, result string) *exec.Cmd {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestRefreshLockIsSharedAcrossProcesses$")
	env := make([]string, 0, len(os.Environ())+6)
	for _, value := range os.Environ() {
		if strings.HasPrefix(value, "HOME=") ||
			strings.HasPrefix(value, "LARKSUITE_CLI_CONFIG_DIR=") ||
			strings.HasPrefix(value, "LARKSUITE_CLI_DATA_DIR=") ||
			strings.HasPrefix(value, "LARK_CLI_REFRESH_LOCK_HELPER=") {
			continue
		}
		env = append(env, value)
	}
	cmd.Env = append(env,
		"HOME="+homeDir,
		"LARKSUITE_CLI_CONFIG_DIR="+configDir,
		"LARK_CLI_REFRESH_LOCK_HELPER="+mode,
		"LARK_CLI_REFRESH_LOCK_MARKER="+marker,
		"LARK_CLI_REFRESH_LOCK_RELEASE="+release,
		"LARK_CLI_REFRESH_LOCK_RESULT="+result,
	)
	return cmd
}

func runRefreshLockHelper(t *testing.T, mode string) {
	t.Helper()
	lockPath := refreshLockPath("cli_process_test", "ou_process_test")
	lock := flock.New(lockPath)
	if err := os.MkdirAll(filepath.Dir(lockPath), 0700); err != nil {
		t.Fatal(err)
	}
	locked, err := lock.TryLock()
	if err != nil {
		t.Fatal(err)
	}
	result := os.Getenv("LARK_CLI_REFRESH_LOCK_RESULT")
	switch mode {
	case "hold":
		if !locked {
			t.Fatal("holder could not acquire lock")
		}
		defer lock.Unlock()
		if err := os.WriteFile(result, []byte(lockPath), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(os.Getenv("LARK_CLI_REFRESH_LOCK_MARKER"), []byte("ready"), 0600); err != nil {
			t.Fatal(err)
		}
		deadline := time.Now().Add(5 * time.Second)
		for {
			if _, err := os.Stat(os.Getenv("LARK_CLI_REFRESH_LOCK_RELEASE")); err == nil {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("holder timed out waiting for release")
			}
			time.Sleep(10 * time.Millisecond)
		}
	case "probe":
		if locked {
			_ = lock.Unlock()
			t.Fatal("probe acquired a lock held by another config-isolated process")
		}
		if err := os.WriteFile(result, []byte(fmt.Sprintf("blocked:%s", lockPath)), 0600); err != nil {
			t.Fatal(err)
		}
	default:
		t.Fatalf("unknown helper mode %q", mode)
	}
}
