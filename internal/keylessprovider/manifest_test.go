// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package keylessprovider

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"
)

func setupManifestTest(t *testing.T) (optionalPackageFixture, *atomic.Int32) {
	t.Helper()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	fx := newManagedPackageFixture(t, true)
	t.Setenv(openClawStateDirEnv, fx.stateDir)
	data, err := json.Marshal(inspectPluginDocument(fx, pluginID, fx.packageDir))
	if err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	stubInspectFunction(t, func(context.Context, string) ([]byte, error) { calls.Add(1); return data, nil })
	return fx, &calls
}

func TestResolveManifestLifecycle(t *testing.T) {
	fx, calls := setupManifestTest(t)
	path := providerManifestPath()
	if err := os.WriteFile(path, []byte(`{"version":`), 0600); err != nil {
		t.Fatal(err)
	}
	resolve := func(wantCalls int32) {
		t.Helper()
		cmd, err := Resolve(context.Background(), ProviderID)
		if err != nil || cmd == nil || calls.Load() != wantCalls {
			t.Fatalf("resolve: err=%v, inspect calls=%d, want %d", err, calls.Load(), wantCalls)
		}
	}
	resolve(1) // Repair the corrupt manifest.
	resolve(1) // The repaired entry skips inspection.
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != 0600 {
			t.Fatalf("manifest permissions: %v, %v", info, err)
		}
	}
	manifest, err := readProviderManifest()
	if err != nil {
		t.Fatal(err)
	}
	entry := manifest.Providers[ProviderID]
	if entry.PackageDir != fx.packageDir || entry.BinaryPath != fx.binary || entry.SHA256 != fx.binaryDigest || entry.PackageVersion != "1.2.3" {
		t.Fatalf("unexpected manifest entry: %+v", entry)
	}
	if err := os.WriteFile(fx.binary, []byte("TEST optional-package signer"), 0700); err != nil {
		t.Fatal(err)
	}
	resolve(2) // A same-size replacement invalidates the recorded digest.
	resolve(2)
	if err := os.Remove(fx.binary); err != nil {
		t.Fatal(err)
	}
	stubOpenClawInspect(t, nil, errors.New("inspection unavailable"))
	if _, err := Resolve(context.Background(), ProviderID); err == nil {
		t.Fatal("reused stale command")
	}
}

func TestResolve_UnsafeManifestIsIgnoredAndReplaced(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX mode and symlink test")
	}
	_, calls := setupManifestTest(t)
	if _, err := Resolve(context.Background(), ProviderID); err != nil {
		t.Fatal(err)
	}

	path := providerManifestPath()
	if err := os.Chmod(path, 0666); err != nil {
		t.Fatal(err)
	}
	if _, err := Resolve(context.Background(), ProviderID); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 {
		t.Fatalf("unsafe mode did not trigger discovery; calls = %d", calls.Load())
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("repaired manifest permissions = %v, %v", info, err)
	}

	target := filepath.Join(t.TempDir(), "must-not-overwrite.json")
	const targetContents = "outside"
	if err := os.WriteFile(target, []byte(targetContents), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, err := Resolve(context.Background(), ProviderID); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 3 {
		t.Fatalf("manifest symlink did not trigger discovery; calls = %d", calls.Load())
	}
	if info, err := os.Lstat(path); err != nil || info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("manifest symlink was not safely replaced: %v, %v", info, err)
	}
	if contents, err := os.ReadFile(target); err != nil || string(contents) != targetContents {
		t.Fatalf("symlink target changed: %q, %v", contents, err)
	}
}

func stubInspectFunction(t *testing.T, fn func(context.Context, string) ([]byte, error)) {
	t.Helper()
	previous := runOpenClawInspect
	runOpenClawInspect = fn
	t.Cleanup(func() { runOpenClawInspect = previous })
}
