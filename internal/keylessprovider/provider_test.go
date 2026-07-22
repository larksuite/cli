// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package keylessprovider

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

type optionalPackageFixture struct {
	stateDir, projectDir, pluginDir         string
	packageDir, packageJSON, binDir, binary string
	spec                                    platformPackage
	binaryDigest                            string
}

func TestSignerPackageFor(t *testing.T) {
	tests := []struct {
		goos, goarch, name, npmOS, npmCPU, binary string
	}{
		{"darwin", "arm64", "@larksuite/lark-keyless-signer-darwin-arm64", "darwin", "arm64", signerBinaryBase},
		{"darwin", "amd64", "@larksuite/lark-keyless-signer-darwin-x64", "darwin", "x64", signerBinaryBase},
		{"linux", "arm64", "@larksuite/lark-keyless-signer-linux-arm64", "linux", "arm64", signerBinaryBase},
		{"linux", "amd64", "@larksuite/lark-keyless-signer-linux-x64", "linux", "x64", signerBinaryBase},
		{"windows", "amd64", "@larksuite/lark-keyless-signer-win32-x64", "win32", "x64", signerBinaryBase + ".exe"},
	}
	for _, test := range tests {
		t.Run(test.goos+"_"+test.goarch, func(t *testing.T) {
			got, err := signerPackageFor(test.goos, test.goarch)
			if err != nil {
				t.Fatal(err)
			}
			if got.name != test.name || got.npmOS != test.npmOS || got.npmCPU != test.npmCPU || got.binaryName != test.binary {
				t.Fatalf("spec = %#v", got)
			}
		})
	}
	for _, unsupported := range [][2]string{{"windows", "arm64"}, {"linux", "riscv64"}, {"freebsd", "amd64"}} {
		if _, err := signerPackageFor(unsupported[0], unsupported[1]); err == nil {
			t.Fatalf("%s/%s unexpectedly supported", unsupported[0], unsupported[1])
		}
	}
}

func TestResolveFromStateDir_OptionalPackage(t *testing.T) {
	fx := newOptionalPackageFixture(t, runtime.GOOS, runtime.GOARCH)
	got, err := resolveFromStateDir(fx.stateDir, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		t.Fatal(err)
	}
	if got.binaryPath != fx.binary || got.packageDir != fx.packageDir || got.digest != fx.binaryDigest {
		t.Fatalf("resolved = %#v, fixture = %#v", got, fx)
	}
}

func TestResolve_UsesOpenClawStateDirAndFixedExtensionFallback(t *testing.T) {
	useIsolatedProviderManifest(t)
	fx := newOptionalPackageFixture(t, runtime.GOOS, runtime.GOARCH)
	t.Setenv(openClawStateDirEnv, fx.stateDir)
	t.Setenv(openClawHomeEnv, filepath.Join(t.TempDir(), "must-not-win"))
	t.Setenv("PATH", "") // force the fixed extensions fallback
	command, err := Resolve(context.Background(), ProviderID)
	if err != nil {
		t.Fatal(err)
	}
	if command == nil {
		t.Fatal("Resolve returned nil command")
	}
}

func TestResolveFromInspect_ManagedProjectPluginLocal(t *testing.T) {
	fx := newManagedPackageFixture(t, runtime.GOOS, runtime.GOARCH, false)
	got, err := resolveFromInspectDocument(t, fx, inspectPluginDocument(fx, pluginID, fx.packageDir))
	if err != nil {
		t.Fatal(err)
	}
	assertResolvedFixture(t, got, fx)
}

func TestResolveFromInspect_ManagedProjectHoisted(t *testing.T) {
	fx := newManagedPackageFixture(t, runtime.GOOS, runtime.GOARCH, true)
	got, err := resolveFromInspectDocument(t, fx, inspectPluginDocument(fx, pluginID, fx.packageDir))
	if err != nil {
		t.Fatal(err)
	}
	assertResolvedFixture(t, got, fx)
}

func TestResolveFromInspect_RejectsWrongPluginAndEscapingPaths(t *testing.T) {
	t.Run("wrong plugin", func(t *testing.T) {
		fx := newManagedPackageFixture(t, runtime.GOOS, runtime.GOARCH, true)
		_, err := resolveFromInspectDocument(t, fx, inspectPluginDocument(fx, "evil-plugin", fx.packageDir))
		if err == nil || !strings.Contains(err.Error(), "unexpected plugin") {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("package outside state", func(t *testing.T) {
		fx := newManagedPackageFixture(t, runtime.GOOS, runtime.GOARCH, true)
		outside := filepath.Join(t.TempDir(), "node_modules", signerPackageScope, strings.TrimPrefix(fx.spec.name, signerPackageScope+"/"))
		_, err := resolveFromInspectDocument(t, fx, inspectPluginDocument(fx, pluginID, outside))
		if err == nil || !strings.Contains(err.Error(), "outside OpenClaw state") {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("unrelated package inside state", func(t *testing.T) {
		fx := newManagedPackageFixture(t, runtime.GOOS, runtime.GOARCH, true)
		unrelated := signerPackageUnder(filepath.Join(fx.stateDir, "other", "node_modules"), fx.spec)
		_, err := resolveFromInspectDocument(t, fx, inspectPluginDocument(fx, pluginID, unrelated))
		if err == nil || !strings.Contains(err.Error(), "neither plugin-local nor managed-project-hoisted") {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("plugin outside state", func(t *testing.T) {
		fx := newManagedPackageFixture(t, runtime.GOOS, runtime.GOARCH, true)
		document := inspectPluginDocument(fx, pluginID, fx.packageDir)
		plugin := document["plugin"].(map[string]any)
		plugin["rootDir"] = filepath.Join(t.TempDir(), "openclaw-lark")
		_, err := resolveFromInspectDocument(t, fx, document)
		if err == nil || !strings.Contains(err.Error(), "outside OpenClaw state") {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestResolve_InspectUnavailableUsesOnlyFixedExtensionFallback(t *testing.T) {
	useIsolatedProviderManifest(t)
	fx := newOptionalPackageFixture(t, runtime.GOOS, runtime.GOARCH)
	t.Setenv(openClawStateDirEnv, fx.stateDir)
	stubOpenClawInspect(t, nil, &inspectUnavailableError{cause: errors.New("not installed")})
	command, err := Resolve(context.Background(), ProviderID)
	if err != nil || command == nil {
		t.Fatalf("Resolve = %v, %v", command, err)
	}
}

func TestResolve_InspectUnavailableDoesNotScanManagedProjects(t *testing.T) {
	useIsolatedProviderManifest(t)
	fx := newManagedPackageFixture(t, runtime.GOOS, runtime.GOARCH, true)
	t.Setenv(openClawStateDirEnv, fx.stateDir)
	stubOpenClawInspect(t, nil, &inspectUnavailableError{cause: errors.New("not installed")})
	if _, err := Resolve(context.Background(), ProviderID); err == nil || !strings.Contains(err.Error(), "fixed extension fallback failed") {
		t.Fatalf("error = %v", err)
	}
}

func TestResolve_InvalidInspectDoesNotFallBack(t *testing.T) {
	useIsolatedProviderManifest(t)
	fx := newOptionalPackageFixture(t, runtime.GOOS, runtime.GOARCH)
	t.Setenv(openClawStateDirEnv, fx.stateDir)
	stubOpenClawInspect(t, []byte(`{"plugin":`), nil)
	if _, err := Resolve(context.Background(), ProviderID); err == nil || !strings.Contains(err.Error(), "parse OpenClaw") {
		t.Fatalf("error = %v", err)
	}
}

func TestResolve_InspectCancellationHonorsCallerContext(t *testing.T) {
	useIsolatedProviderManifest(t)
	fx := newOptionalPackageFixture(t, runtime.GOOS, runtime.GOARCH)
	t.Setenv(openClawStateDirEnv, fx.stateDir)
	previous := runOpenClawInspect
	runOpenClawInspect = func(ctx context.Context, _ string) ([]byte, error) {
		<-ctx.Done()
		return nil, &inspectUnavailableError{cause: ctx.Err()}
	}
	t.Cleanup(func() { runOpenClawInspect = previous })

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Resolve(ctx, ProviderID); !errors.Is(err, context.Canceled) {
		t.Fatalf("Resolve error = %v, want context.Canceled", err)
	}
}

func TestInspectResultCache_ShortLivedAndSingleflight(t *testing.T) {
	var cache inspectResultCache
	var calls atomic.Int32
	started := make(chan struct{})
	release := make(chan struct{})
	loader := func(context.Context) ([]byte, error) {
		if calls.Add(1) == 1 {
			close(started)
		}
		<-release
		return []byte(`{"plugin":{"id":"openclaw-lark"}}`), nil
	}

	const callers = 8
	results := make(chan []byte, callers)
	errs := make(chan error, callers)
	var wg sync.WaitGroup
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			data, err := cache.load(context.Background(), "same-install", loader)
			results <- data
			errs <- err
		}()
	}
	<-started
	close(release)
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	for data := range results {
		if string(data) != `{"plugin":{"id":"openclaw-lark"}}` {
			t.Fatalf("cached data = %q", data)
		}
		if len(data) > 0 {
			data[0] = 'x'
		}
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("loader calls = %d, want 1", got)
	}
	data, err := cache.load(context.Background(), "same-install", loader)
	if err != nil || string(data) != `{"plugin":{"id":"openclaw-lark"}}` || calls.Load() != 1 {
		t.Fatalf("cache hit = %q, %v, calls %d", data, err, calls.Load())
	}
}

func TestInspectResultCache_WaiterHonorsContext(t *testing.T) {
	var cache inspectResultCache
	started := make(chan struct{})
	release := make(chan struct{})
	loader := func(context.Context) ([]byte, error) {
		close(started)
		<-release
		return []byte(`{}`), nil
	}
	go func() {
		_, _ = cache.load(context.Background(), "busy", loader)
	}()
	<-started

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := cache.load(ctx, "busy", loader); !errors.Is(err, context.Canceled) {
		t.Fatalf("cache waiter error = %v, want context.Canceled", err)
	}
	close(release)
}

func TestInspectOutputAndEnvironmentAreBounded(t *testing.T) {
	buffer := &cappedBuffer{limit: 4}
	if n, err := buffer.Write([]byte("123456")); err != nil || n != 6 || !buffer.exceeded || buffer.String() != "1234" {
		t.Fatalf("bounded write = n %d exceeded %v data %q err %v", n, buffer.exceeded, buffer.String(), err)
	}
	t.Setenv("HOME", "/safe-home")
	t.Setenv("NODE_OPTIONS", "--require=/must-not-load")
	t.Setenv(openClawStateDirEnv, "/must-not-win")
	env := strings.Join(openClawInspectEnvironment("/selected-state"), "\n")
	if !strings.Contains(env, "HOME=/safe-home") || !strings.Contains(env, openClawStateDirEnv+"=/selected-state") ||
		strings.Contains(env, "NODE_OPTIONS") || strings.Contains(env, openClawStateDirEnv+"=/must-not-win") {
		t.Fatalf("inspect environment = %q", env)
	}
}

func TestResolve_UnknownProviderFailsClosed(t *testing.T) {
	useIsolatedProviderManifest(t)
	if _, err := Resolve(context.Background(), "evil.provider"); err == nil || !strings.Contains(err.Error(), "unknown") {
		t.Fatalf("error = %v", err)
	}
}

func useIsolatedProviderManifest(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", dir)
	return dir
}

func TestOpenClawStateDirPriorityAndExpansion(t *testing.T) {
	home := t.TempDir()
	state := filepath.Join(t.TempDir(), "custom state")
	t.Setenv("HOME", home)
	t.Setenv(openClawConfigEnv, "")
	t.Setenv(openClawHomeEnv, filepath.Join(t.TempDir(), "must-not-win"))
	t.Setenv(openClawStateDirEnv, state)
	if got, err := openClawStateDir(); err != nil || got != state {
		t.Fatalf("state dir = %q, %v", got, err)
	}

	t.Setenv(openClawStateDirEnv, "")
	openClawHome := filepath.Join(t.TempDir(), "openclaw home")
	t.Setenv(openClawHomeEnv, openClawHome)
	if got, err := openClawStateDir(); err != nil || got != filepath.Join(openClawHome, openClawDirName) {
		t.Fatalf("OPENCLAW_HOME state dir = %q, %v", got, err)
	}

	t.Setenv(openClawHomeEnv, "")
	t.Setenv(openClawStateDirEnv, "~/custom-openclaw")
	if got, err := openClawStateDir(); err != nil || got != filepath.Join(home, "custom-openclaw") {
		t.Fatalf("expanded state dir = %q, %v", got, err)
	}
}

func TestOpenClawStateDir_ConfigPathPrecedesHome(t *testing.T) {
	osHome := t.TempDir()
	openClawHome := filepath.Join(t.TempDir(), "openclaw-home")
	t.Setenv("HOME", osHome)
	t.Setenv(openClawStateDirEnv, "")
	t.Setenv(openClawHomeEnv, openClawHome)
	t.Setenv(openClawConfigEnv, "~/profiles/team/openclaw.json")

	want := filepath.Join(openClawHome, "profiles", "team")
	if got, err := openClawStateDir(); err != nil || got != want {
		t.Fatalf("state dir = %q, %v; want %q", got, err, want)
	}

	explicitState := filepath.Join(t.TempDir(), "state-wins")
	t.Setenv(openClawStateDirEnv, explicitState)
	if got, err := openClawStateDir(); err != nil || got != explicitState {
		t.Fatalf("explicit state dir = %q, %v; want %q", got, err, explicitState)
	}
}

func TestOpenClawStateDirRejectsRelativeOrUncleanPath(t *testing.T) {
	unclean := filepath.Join(t.TempDir(), "child") + string(filepath.Separator) + ".." + string(filepath.Separator) + "state"
	for _, path := range []string{"relative/state", unclean} {
		t.Run(strings.ReplaceAll(path, string(filepath.Separator), "_"), func(t *testing.T) {
			t.Setenv(openClawStateDirEnv, path)
			if _, err := openClawStateDir(); err == nil {
				t.Fatalf("path %q was accepted", path)
			}
		})
	}
}

func TestResolveFromStateDir_ValidatesPackageMetadata(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*packageManifest)
		want   string
	}{
		{"name", func(m *packageManifest) { m.Name = "@evil/signer" }, "name"},
		{"version", func(m *packageManifest) { m.Version = "latest" }, "version"},
		{"os", func(m *packageManifest) { m.OS = []string{"other"} }, "os metadata"},
		{"cpu", func(m *packageManifest) { m.CPU = []string{"other"} }, "cpu metadata"},
		{"multiple os", func(m *packageManifest) { m.OS = append(m.OS, "other") }, "os metadata"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fx := newOptionalPackageFixture(t, runtime.GOOS, runtime.GOARCH)
			manifest := validManifest(fx.spec)
			test.mutate(&manifest)
			writeJSON(t, fx.packageJSON, manifest, 0600)
			if _, err := resolveFromStateDir(fx.stateDir, runtime.GOOS, runtime.GOARCH); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestResolveFromStateDir_RejectsParentAndBinarySymlinks(t *testing.T) {
	t.Run("parent", func(t *testing.T) {
		fx := newOptionalPackageFixture(t, runtime.GOOS, runtime.GOARCH)
		realBin := fx.binDir + "-real"
		if err := os.Rename(fx.binDir, realBin); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(realBin, fx.binDir); err != nil {
			t.Skipf("symlink unavailable: %v", err)
		}
		if _, err := resolveFromStateDir(fx.stateDir, runtime.GOOS, runtime.GOARCH); err == nil || !strings.Contains(err.Error(), "symlink") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("binary", func(t *testing.T) {
		fx := newOptionalPackageFixture(t, runtime.GOOS, runtime.GOARCH)
		realBinary := fx.binary + "-real"
		if err := os.Rename(fx.binary, realBinary); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(realBinary, fx.binary); err != nil {
			t.Skipf("symlink unavailable: %v", err)
		}
		if _, err := resolveFromStateDir(fx.stateDir, runtime.GOOS, runtime.GOARCH); err == nil || !strings.Contains(err.Error(), "symlink") {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestResolveFromStateDir_RejectsInsecureModeAndNonExecutable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX mode test")
	}
	t.Run("writable parent", func(t *testing.T) {
		fx := newOptionalPackageFixture(t, runtime.GOOS, runtime.GOARCH)
		if err := os.Chmod(fx.packageDir, 0777); err != nil {
			t.Fatal(err)
		}
		if _, err := resolveFromStateDir(fx.stateDir, runtime.GOOS, runtime.GOARCH); err == nil || !strings.Contains(err.Error(), "writable") {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("non executable", func(t *testing.T) {
		fx := newOptionalPackageFixture(t, runtime.GOOS, runtime.GOARCH)
		if err := os.Chmod(fx.binary, 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := resolveFromStateDir(fx.stateDir, runtime.GOOS, runtime.GOARCH); err == nil || !strings.Contains(err.Error(), "executable") {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestResolveFromStateDir_MissingPackageFailsClosed(t *testing.T) {
	stateDir := t.TempDir()
	if _, err := resolveFromStateDir(stateDir, runtime.GOOS, runtime.GOARCH); err == nil {
		t.Fatal("missing optional package was accepted")
	}
}

func newOptionalPackageFixture(t *testing.T, goos, goarch string) optionalPackageFixture {
	t.Helper()
	spec, err := signerPackageFor(goos, goarch)
	if err != nil {
		t.Skipf("host platform has no optional package: %v", err)
	}
	stateDir := filepath.Join(t.TempDir(), "openclaw state")
	pluginDir := filepath.Join(stateDir, "extensions", pluginID)
	packageDir := signerPackageUnder(filepath.Join(pluginDir, "node_modules"), spec)
	return writeOptionalPackageFixture(t, optionalPackageFixture{
		stateDir: stateDir, pluginDir: pluginDir, packageDir: packageDir, spec: spec,
	})
}

func newManagedPackageFixture(t *testing.T, goos, goarch string, hoisted bool) optionalPackageFixture {
	t.Helper()
	spec, err := signerPackageFor(goos, goarch)
	if err != nil {
		t.Skipf("host platform has no optional package: %v", err)
	}
	stateDir := filepath.Join(t.TempDir(), "custom OpenClaw state")
	projectDir := filepath.Join(stateDir, "npm", "projects", "larksuite-openclaw-lark-test-generation")
	nodeModules := filepath.Join(projectDir, "node_modules")
	pluginDir := filepath.Join(nodeModules, signerPackageScope, strings.TrimPrefix(pluginPackageName, signerPackageScope+"/"))
	packageNodeModules := filepath.Join(pluginDir, "node_modules")
	if hoisted {
		packageNodeModules = nodeModules
	}
	packageDir := signerPackageUnder(packageNodeModules, spec)
	return writeOptionalPackageFixture(t, optionalPackageFixture{
		stateDir: stateDir, projectDir: projectDir, pluginDir: pluginDir, packageDir: packageDir, spec: spec,
	})
}

func writeOptionalPackageFixture(t *testing.T, fx optionalPackageFixture) optionalPackageFixture {
	t.Helper()
	if err := os.MkdirAll(fx.pluginDir, 0700); err != nil {
		t.Fatal(err)
	}
	packageDir := fx.packageDir
	binDir := filepath.Join(packageDir, "bin")
	if err := os.MkdirAll(binDir, 0700); err != nil {
		t.Fatal(err)
	}
	packageJSON := filepath.Join(packageDir, "package.json")
	writeJSON(t, packageJSON, validManifest(fx.spec), 0600)
	binary := filepath.Join(binDir, fx.spec.binaryName)
	binaryBytes := []byte("test optional-package signer")
	if err := os.WriteFile(binary, binaryBytes, 0700); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(binaryBytes)
	fx.packageJSON, fx.binDir, fx.binary = packageJSON, binDir, binary
	fx.binaryDigest = hex.EncodeToString(digest[:])
	return fx
}

func inspectPluginDocument(fx optionalPackageFixture, inspectedID, resolvedPath string) map[string]any {
	return map[string]any{"plugin": map[string]any{
		"id": inspectedID, "name": pluginPackageName, "packageName": pluginPackageName,
		"rootDir": fx.pluginDir, "status": "loaded",
		"dependencyStatus": map[string]any{"optionalDependencies": []map[string]any{{
			"name": fx.spec.name, "installed": true, "optional": true, "resolvedPath": resolvedPath,
		}}},
	}}
}

func resolveFromInspectDocument(t *testing.T, fx optionalPackageFixture, document any) (resolvedProvider, error) {
	t.Helper()
	data, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	stubOpenClawInspect(t, data, nil)
	return resolveFromInspect(context.Background(), fx.stateDir, runtime.GOOS, runtime.GOARCH)
}

func stubOpenClawInspect(t *testing.T, data []byte, err error) {
	t.Helper()
	previous := runOpenClawInspect
	runOpenClawInspect = func(context.Context, string) ([]byte, error) { return data, err }
	t.Cleanup(func() { runOpenClawInspect = previous })
}

func assertResolvedFixture(t *testing.T, got resolvedProvider, fx optionalPackageFixture) {
	t.Helper()
	if got.binaryPath != fx.binary || got.packageDir != fx.packageDir || got.digest != fx.binaryDigest {
		t.Fatalf("resolved = %#v, fixture = %#v", got, fx)
	}
}

func validManifest(spec platformPackage) packageManifest {
	return packageManifest{Name: spec.name, Version: "1.2.3", OS: []string{spec.npmOS}, CPU: []string{spec.npmCPU}}
}

func writeJSON(t *testing.T, path string, value any, mode os.FileMode) {
	t.Helper()
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, mode); err != nil {
		t.Fatal(err)
	}
}
