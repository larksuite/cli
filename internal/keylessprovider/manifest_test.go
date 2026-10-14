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

func TestResolve_ManifestMissPersistsAndHitSkipsInspect(t *testing.T) {
	configDir := useIsolatedProviderManifest(t)
	fx := newManagedPackageFixture(t, runtime.GOOS, runtime.GOARCH, true)
	t.Setenv(openClawStateDirEnv, fx.stateDir)
	data := marshalInspectDocument(t, inspectPluginDocument(fx, pluginID, fx.packageDir))

	var calls atomic.Int32
	stubInspectFunction(t, func(context.Context, string) ([]byte, error) {
		calls.Add(1)
		return data, nil
	})
	if command, err := Resolve(context.Background(), ProviderID); err != nil || command == nil {
		t.Fatalf("first Resolve = %v, %v", command, err)
	}
	if calls.Load() != 1 {
		t.Fatalf("inspect calls after miss = %d, want 1", calls.Load())
	}

	path := filepath.Join(configDir, providerManifestFileName)
	manifestData, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var manifest providerManifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		t.Fatalf("manifest is not valid JSON: %v", err)
	}
	entry := manifest.Providers[ProviderID]
	if manifest.Version != providerManifestFormatVersion || entry.PackageDir != fx.packageDir ||
		entry.BinaryPath != fx.binary || entry.SHA256 != fx.binaryDigest || entry.PackageVersion != "1.2.3" {
		t.Fatalf("persisted manifest = %#v", manifest)
	}
	if runtime.GOOS != "windows" {
		if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0600 {
			t.Fatalf("manifest permissions = %v, %v", info, err)
		}
	}

	if command, err := Resolve(context.Background(), ProviderID); err != nil || command == nil {
		t.Fatalf("cached Resolve = %v, %v", command, err)
	}
	if calls.Load() != 1 {
		t.Fatalf("manifest hit restarted inspect; calls = %d", calls.Load())
	}
}

func TestResolve_ManifestMutationRefreshes(t *testing.T) {
	useIsolatedProviderManifest(t)
	fx := newManagedPackageFixture(t, runtime.GOOS, runtime.GOARCH, true)
	t.Setenv(openClawStateDirEnv, fx.stateDir)
	data := marshalInspectDocument(t, inspectPluginDocument(fx, pluginID, fx.packageDir))

	var calls atomic.Int32
	stubInspectFunction(t, func(context.Context, string) ([]byte, error) {
		calls.Add(1)
		return data, nil
	})
	if _, err := Resolve(context.Background(), ProviderID); err != nil {
		t.Fatal(err)
	}

	changed := []byte("TEST optional-package signer") // same length, different digest
	if info, err := os.Stat(fx.binary); err != nil || info.Size() != int64(len(changed)) {
		t.Fatalf("fixture length precondition failed: %v, %v", info, err)
	}
	if err := os.WriteFile(fx.binary, changed, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := Resolve(context.Background(), ProviderID); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 {
		t.Fatalf("inspect calls after binary mutation = %d, want 2", calls.Load())
	}

	manifest, err := readProviderManifest()
	if err != nil {
		t.Fatal(err)
	}
	entry := manifest.Providers[ProviderID]
	if entry.SHA256 == fx.binaryDigest {
		t.Fatal("manifest retained the stale signer digest")
	}
}

func TestResolve_CorruptManifestSelfHeals(t *testing.T) {
	configDir := useIsolatedProviderManifest(t)
	fx := newManagedPackageFixture(t, runtime.GOOS, runtime.GOARCH, true)
	t.Setenv(openClawStateDirEnv, fx.stateDir)
	if err := os.WriteFile(filepath.Join(configDir, providerManifestFileName), []byte(`{"version":`), 0600); err != nil {
		t.Fatal(err)
	}
	data := marshalInspectDocument(t, inspectPluginDocument(fx, pluginID, fx.packageDir))
	var calls atomic.Int32
	stubInspectFunction(t, func(context.Context, string) ([]byte, error) {
		calls.Add(1)
		return data, nil
	})
	if _, err := Resolve(context.Background(), ProviderID); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 {
		t.Fatalf("inspect calls = %d, want 1", calls.Load())
	}
	if _, err := readProviderManifest(); err != nil {
		t.Fatalf("manifest was not repaired: %v", err)
	}
}

func TestResolve_UnsafeManifestIsIgnoredAndReplaced(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX mode and symlink test")
	}
	configDir := useIsolatedProviderManifest(t)
	fx := newManagedPackageFixture(t, runtime.GOOS, runtime.GOARCH, true)
	t.Setenv(openClawStateDirEnv, fx.stateDir)
	data := marshalInspectDocument(t, inspectPluginDocument(fx, pluginID, fx.packageDir))
	var calls atomic.Int32
	stubInspectFunction(t, func(context.Context, string) ([]byte, error) {
		calls.Add(1)
		return data, nil
	})
	if _, err := Resolve(context.Background(), ProviderID); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(configDir, providerManifestFileName)
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

func TestResolve_InvalidManifestDoesNotFallBackToStaleCommand(t *testing.T) {
	useIsolatedProviderManifest(t)
	fx := newManagedPackageFixture(t, runtime.GOOS, runtime.GOARCH, true)
	t.Setenv(openClawStateDirEnv, fx.stateDir)
	data := marshalInspectDocument(t, inspectPluginDocument(fx, pluginID, fx.packageDir))
	stubInspectFunction(t, func(context.Context, string) ([]byte, error) { return data, nil })
	if _, err := Resolve(context.Background(), ProviderID); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(fx.binary); err != nil {
		t.Fatal(err)
	}
	stubInspectFunction(t, func(context.Context, string) ([]byte, error) {
		return nil, errors.New("inspection unavailable")
	})
	if _, err := Resolve(context.Background(), ProviderID); err == nil {
		t.Fatal("Resolve reused an invalid manifest entry")
	}
}

func marshalInspectDocument(t *testing.T, document any) []byte {
	t.Helper()
	data, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func stubInspectFunction(t *testing.T, fn func(context.Context, string) ([]byte, error)) {
	t.Helper()
	previous := runOpenClawInspect
	runOpenClawInspect = fn
	t.Cleanup(func() { runOpenClawInspect = previous })
}
