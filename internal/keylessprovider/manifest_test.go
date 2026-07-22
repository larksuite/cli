// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package keylessprovider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sync"
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

func TestRefresh_BypassesValidManifest(t *testing.T) {
	useIsolatedProviderManifest(t)
	oldFx := newManagedPackageFixture(t, runtime.GOOS, runtime.GOARCH, true)
	newFx := newManagedPackageFixtureInState(t, oldFx.stateDir, runtime.GOOS, runtime.GOARCH, true)
	t.Setenv(openClawStateDirEnv, oldFx.stateDir)

	var current = oldFx
	var calls atomic.Int32
	stubInspectFunction(t, func(context.Context, string) ([]byte, error) {
		calls.Add(1)
		return marshalInspectDocument(t, inspectPluginDocument(current, pluginID, current.packageDir)), nil
	})
	if _, err := Resolve(context.Background(), ProviderID); err != nil {
		t.Fatal(err)
	}
	current = newFx
	if _, err := Resolve(context.Background(), ProviderID); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 {
		t.Fatalf("normal Resolve should retain valid binding; calls = %d", calls.Load())
	}
	if _, err := Refresh(context.Background(), ProviderID); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 {
		t.Fatalf("Refresh did not inspect; calls = %d", calls.Load())
	}
	manifest, err := readProviderManifest()
	if err != nil {
		t.Fatal(err)
	}
	if got := manifest.Providers[ProviderID].PackageDir; got != newFx.packageDir {
		t.Fatalf("refreshed packageDir = %q, want %q", got, newFx.packageDir)
	}
}

func TestPrepareRefresh_AbandonedCommitPreservesManifestBytes(t *testing.T) {
	configDir := useIsolatedProviderManifest(t)
	oldFx := newManagedPackageFixture(t, runtime.GOOS, runtime.GOARCH, true)
	newFx := newManagedPackageFixtureInState(t, oldFx.stateDir, runtime.GOOS, runtime.GOARCH, true)
	t.Setenv(openClawStateDirEnv, oldFx.stateDir)

	current := oldFx
	stubInspectFunction(t, func(context.Context, string) ([]byte, error) {
		return marshalInspectDocument(t, inspectPluginDocument(current, pluginID, current.packageDir)), nil
	})
	if _, err := Resolve(context.Background(), ProviderID); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(configDir, providerManifestFileName)
	before, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}

	current = newFx
	command, commit, err := PrepareRefresh(context.Background(), ProviderID)
	if err != nil || command == nil || commit == nil {
		t.Fatalf("PrepareRefresh = %v, commit %v, %v", command, commit != nil, err)
	}
	afterPrepare, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(afterPrepare, before) {
		t.Fatal("PrepareRefresh changed the manifest before its commit callback ran")
	}

	const committers = 8
	errs := make(chan error, committers)
	var wg sync.WaitGroup
	for i := 0; i < committers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- commit()
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	manifest, err := readProviderManifest()
	if err != nil {
		t.Fatal(err)
	}
	if got := manifest.Providers[ProviderID].PackageDir; got != newFx.packageDir {
		t.Fatalf("committed packageDir = %q, want %q", got, newFx.packageDir)
	}
}

func TestPrepareRefresh_CommitReturnsFirstErrorWithoutRetry(t *testing.T) {
	configDir := useIsolatedProviderManifest(t)
	fx := newManagedPackageFixture(t, runtime.GOOS, runtime.GOARCH, true)
	t.Setenv(openClawStateDirEnv, fx.stateDir)
	stubInspectFunction(t, func(context.Context, string) ([]byte, error) {
		return marshalInspectDocument(t, inspectPluginDocument(fx, pluginID, fx.packageDir)), nil
	})
	_, commit, err := PrepareRefresh(context.Background(), ProviderID)
	if err != nil || commit == nil {
		t.Fatalf("PrepareRefresh commit %v, error %v", commit != nil, err)
	}

	if err := os.RemoveAll(configDir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configDir, []byte("block manifest directory"), 0600); err != nil {
		t.Fatal(err)
	}
	firstErr := commit()
	if firstErr == nil {
		t.Fatal("first commit unexpectedly succeeded")
	}
	if err := os.Remove(configDir); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(configDir, 0700); err != nil {
		t.Fatal(err)
	}
	secondErr := commit()
	if secondErr != firstErr {
		t.Fatalf("second commit error = %v; want cached first error %v", secondErr, firstErr)
	}
	if _, err := os.Stat(filepath.Join(configDir, providerManifestFileName)); !os.IsNotExist(err) {
		t.Fatalf("second commit retried manifest write: %v", err)
	}
}

func TestPrepareRefresh_CommitRejectsSameStampBinaryMutation(t *testing.T) {
	configDir := useIsolatedProviderManifest(t)
	oldFx := newManagedPackageFixture(t, runtime.GOOS, runtime.GOARCH, true)
	newFx := newManagedPackageFixtureInState(t, oldFx.stateDir, runtime.GOOS, runtime.GOARCH, true)
	t.Setenv(openClawStateDirEnv, oldFx.stateDir)

	current := oldFx
	stubInspectFunction(t, func(context.Context, string) ([]byte, error) {
		return marshalInspectDocument(t, inspectPluginDocument(current, pluginID, current.packageDir)), nil
	})
	if _, err := Resolve(context.Background(), ProviderID); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(configDir, providerManifestFileName)
	before, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}

	current = newFx
	_, commit, err := PrepareRefresh(context.Background(), ProviderID)
	if err != nil || commit == nil {
		t.Fatalf("PrepareRefresh commit %v, error %v", commit != nil, err)
	}
	info, err := os.Stat(newFx.binary)
	if err != nil {
		t.Fatal(err)
	}
	changed := []byte("TEST optional-package signer")
	if info.Size() != int64(len(changed)) {
		t.Fatalf("fixture size = %d, mutated size = %d", info.Size(), len(changed))
	}
	if err := os.WriteFile(newFx.binary, changed, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(newFx.binary, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	mutatedInfo, err := os.Stat(newFx.binary)
	if err != nil {
		t.Fatal(err)
	}
	if mutatedInfo.Size() != info.Size() || !mutatedInfo.ModTime().Equal(info.ModTime()) {
		t.Fatalf("mutation did not preserve size/mtime: before %d/%v, after %d/%v",
			info.Size(), info.ModTime(), mutatedInfo.Size(), mutatedInfo.ModTime())
	}

	if err := commit(); err == nil {
		t.Fatal("commit accepted a signer binary changed after PrepareRefresh")
	}
	after, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, before) {
		t.Fatal("failed commit changed the previous manifest")
	}
}

func TestRefresh_BypassesLiveInspectResultCache(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test fixture uses a POSIX shell executable")
	}
	useIsolatedProviderManifest(t)
	oldFx := newManagedPackageFixture(t, runtime.GOOS, runtime.GOARCH, true)
	newFx := newManagedPackageFixtureInState(t, oldFx.stateDir, runtime.GOOS, runtime.GOARCH, true)
	t.Setenv(openClawStateDirEnv, oldFx.stateDir)
	t.Setenv(openClawConfigEnv, "")

	inspectOutput := filepath.Join(oldFx.stateDir, "inspect-output.json")
	writeInspectOutput := func(fx optionalPackageFixture) {
		t.Helper()
		data := marshalInspectDocument(t, inspectPluginDocument(fx, pluginID, fx.packageDir))
		if err := os.WriteFile(inspectOutput, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	writeInspectOutput(oldFx)

	binDir := t.TempDir()
	openClawPath := filepath.Join(binDir, "openclaw")
	const script = "#!/bin/sh\n/bin/cat \"$OPENCLAW_STATE_DIR/inspect-output.json\"\n"
	if err := os.WriteFile(openClawPath, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir)

	previousCached, previousFresh := runOpenClawInspect, runOpenClawInspectFresh
	runOpenClawInspect, runOpenClawInspectFresh = executeOpenClawInspect, executeOpenClawInspectFresh
	t.Cleanup(func() {
		runOpenClawInspect, runOpenClawInspectFresh = previousCached, previousFresh
	})

	// This normal resolution populates the real process cache with oldFx. The
	// cache key is unchanged when only inspect-output.json is replaced.
	if _, err := Resolve(context.Background(), ProviderID); err != nil {
		t.Fatal(err)
	}
	writeInspectOutput(newFx)
	cachedData, err := executeOpenClawInspect(context.Background(), oldFx.stateDir)
	if err != nil {
		t.Fatal(err)
	}
	cachedPlugin, err := decodeInspectedPlugin(cachedData)
	if err != nil {
		t.Fatal(err)
	}
	if cachedPlugin.RootDir != oldFx.pluginDir {
		t.Fatalf("cache precondition failed: rootDir = %q, want old %q", cachedPlugin.RootDir, oldFx.pluginDir)
	}
	if _, err := Refresh(context.Background(), ProviderID); err != nil {
		t.Fatal(err)
	}
	manifest, err := readProviderManifest()
	if err != nil {
		t.Fatal(err)
	}
	if got := manifest.Providers[ProviderID].PackageDir; got != newFx.packageDir {
		t.Fatalf("Refresh reused cached packageDir %q; want fresh %q", got, newFx.packageDir)
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
	previousCached, previousFresh := runOpenClawInspect, runOpenClawInspectFresh
	runOpenClawInspect, runOpenClawInspectFresh = fn, fn
	t.Cleanup(func() {
		runOpenClawInspect, runOpenClawInspectFresh = previousCached, previousFresh
	})
}

func newManagedPackageFixtureInState(t *testing.T, stateDir, goos, goarch string, hoisted bool) optionalPackageFixture {
	t.Helper()
	spec, err := signerPackageFor(goos, goarch)
	if err != nil {
		t.Skipf("host platform has no optional package: %v", err)
	}
	projectDir := filepath.Join(stateDir, "npm", "projects", "larksuite-openclaw-lark-next-generation")
	nodeModules := filepath.Join(projectDir, "node_modules")
	pluginDir := filepath.Join(nodeModules, signerPackageScope, pluginPackageName[len(signerPackageScope)+1:])
	packageNodeModules := filepath.Join(pluginDir, "node_modules")
	if hoisted {
		packageNodeModules = nodeModules
	}
	return writeOptionalPackageFixture(t, optionalPackageFixture{
		stateDir: stateDir, projectDir: projectDir, pluginDir: pluginDir,
		packageDir: signerPackageUnder(packageNodeModules, spec), spec: spec,
	})
}
