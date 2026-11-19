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
	"sync/atomic"
	"testing"
	"time"
)

type optionalPackageFixture struct {
	stateDir, pluginDir                     string
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

func TestResolveFromInspectManagedPackage(t *testing.T) {
	for _, hoisted := range []bool{false, true} {
		fx := newManagedPackageFixture(t, hoisted)
		got, err := resolveFromInspectDocument(t, fx, inspectPluginDocument(fx, pluginID, fx.packageDir))
		if err != nil || got.binaryPath != fx.binary || got.packageDir != fx.packageDir || got.digest != fx.binaryDigest {
			t.Fatalf("hoisted=%t: resolved %+v, err=%v", hoisted, got, err)
		}
	}
}

func TestResolveFromInspectRejectsUntrustedLayout(t *testing.T) {
	fx := newManagedPackageFixture(t, true)
	for _, tc := range []struct{ name, id, plugin, pkg, want string }{
		{"wrong plugin", "unexpected", fx.pluginDir, fx.packageDir, "unexpected plugin"},
		{"package outside state", pluginID, fx.pluginDir, signerPackageUnder(t.TempDir(), fx.spec), "outside OpenClaw state"},
		{"unrelated package inside state", pluginID, fx.pluginDir, signerPackageUnder(filepath.Join(fx.stateDir, "other", "node_modules"), fx.spec), "neither plugin-local nor managed-project-hoisted"},
		{"plugin outside state", pluginID, filepath.Join(t.TempDir(), pluginID), fx.packageDir, "outside OpenClaw state"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc := inspectPluginDocument(fx, tc.id, tc.pkg)
			doc["plugin"].(map[string]any)["rootDir"] = tc.plugin
			if _, err := resolveFromInspectDocument(t, fx, doc); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected %q, got %v", tc.want, err)
			}
		})
	}
}

func TestResolve_InspectUnavailableUsesOnlyFixedExtensionFallback(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	fx := newOptionalPackageFixture(t)
	t.Setenv(openClawStateDirEnv, fx.stateDir)
	stubOpenClawInspect(t, nil, &inspectUnavailableError{cause: errors.New("not installed")})
	command, err := Resolve(context.Background(), ProviderID)
	if err != nil || command == nil {
		t.Fatalf("Resolve = %v, %v", command, err)
	}
}

func TestResolve_InspectUnavailableDoesNotScanManagedProjects(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	fx := newManagedPackageFixture(t, true)
	t.Setenv(openClawStateDirEnv, fx.stateDir)
	stubOpenClawInspect(t, nil, &inspectUnavailableError{cause: errors.New("not installed")})
	if _, err := Resolve(context.Background(), ProviderID); err == nil || !strings.Contains(err.Error(), "fixed extension fallback failed") {
		t.Fatalf("error = %v", err)
	}
}

func TestResolve_InvalidInspectDoesNotFallBack(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	fx := newOptionalPackageFixture(t)
	t.Setenv(openClawStateDirEnv, fx.stateDir)
	stubOpenClawInspect(t, []byte(`{"plugin":`), nil)
	if _, err := Resolve(context.Background(), ProviderID); err == nil || !strings.Contains(err.Error(), "parse OpenClaw") {
		t.Fatalf("error = %v", err)
	}
}

func TestResolve_InspectCancellationHonorsCallerContext(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	fx := newOptionalPackageFixture(t)
	t.Setenv(openClawStateDirEnv, fx.stateDir)
	stubInspectFunction(t, func(ctx context.Context, _ string) ([]byte, error) {
		<-ctx.Done()
		return nil, &inspectUnavailableError{cause: ctx.Err()}
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Resolve(ctx, ProviderID); !errors.Is(err, context.Canceled) {
		t.Fatalf("Resolve error = %v, want context.Canceled", err)
	}
}

func TestInspectCacheSharesLookupAndHonorsWaiterCancellation(t *testing.T) {
	var cache inspectResultCache
	var calls atomic.Int32
	started, release := make(chan struct{}), make(chan struct{})
	loader := func(context.Context) ([]byte, error) {
		if calls.Add(1) == 1 {
			close(started)
		}
		select {
		case <-release:
			return []byte("result"), nil
		case <-time.After(5 * time.Second):
			return nil, errors.New("waiter did not return")
		}
	}
	done := make(chan error, 1)
	go func() {
		data, err := cache.load(context.Background(), "install", loader)
		if err == nil && string(data) != "result" {
			err = errors.New("wrong lookup result")
		}
		if err == nil {
			data[0] = 'X'
		}
		done <- err
	}()
	<-started
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	_, waitErr := cache.load(ctx, "install", loader)
	close(release)
	if err := <-done; err != nil || !errors.Is(waitErr, context.DeadlineExceeded) {
		t.Fatalf("lookup error=%v, waiter error=%v", err, waitErr)
	}
	data, err := cache.load(context.Background(), "install", loader)
	if err != nil || string(data) != "result" || calls.Load() != 1 {
		t.Fatalf("cached data=%q, err=%v, lookup calls=%d", data, err, calls.Load())
	}
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
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	for _, provider := range []string{"", "evil.provider", " " + ProviderID} {
		if _, err := Resolve(context.Background(), provider); err == nil || !strings.Contains(err.Error(), "unknown") {
			t.Fatalf("provider %q: error = %v", provider, err)
		}
	}
}

func TestOpenClawStateDirPrecedence(t *testing.T) {
	home, openClawHome, explicit := t.TempDir(), t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	for _, tc := range []struct{ state, config, appHome, want string }{
		{explicit, "~/profiles/openclaw.json", openClawHome, explicit},
		{"", "~/profiles/openclaw.json", openClawHome, filepath.Join(openClawHome, "profiles")},
		{"", "", openClawHome, filepath.Join(openClawHome, openClawDirName)},
		{"~/custom", "", "", filepath.Join(home, "custom")},
	} {
		t.Setenv(openClawStateDirEnv, tc.state)
		t.Setenv(openClawConfigEnv, tc.config)
		t.Setenv(openClawHomeEnv, tc.appHome)
		if got, err := openClawStateDir(); err != nil || got != tc.want {
			t.Fatalf("state=%q config=%q home=%q: got %q, %v; want %q", tc.state, tc.config, tc.appHome, got, err, tc.want)
		}
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
			fx := newOptionalPackageFixture(t)
			manifest := validManifest(fx.spec)
			test.mutate(&manifest)
			writeJSON(t, fx.packageJSON, manifest, 0600)
			if _, err := resolveFromStateDir(fx.stateDir, runtime.GOOS, runtime.GOARCH); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestResolveFromStateDirRejectsSymlinks(t *testing.T) {
	for _, name := range []string{"parent", "binary"} {
		t.Run(name, func(t *testing.T) {
			fx := newOptionalPackageFixture(t)
			path := fx.binary
			if name == "parent" {
				path = fx.binDir
			}
			if err := os.Rename(path, path+"-real"); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(path+"-real", path); err != nil {
				t.Skipf("symlink unavailable: %v", err)
			}
			if _, err := resolveFromStateDir(fx.stateDir, runtime.GOOS, runtime.GOARCH); err == nil || !strings.Contains(err.Error(), "symlink") {
				t.Fatalf("expected symlink rejection, got %v", err)
			}
		})
	}
}

func TestResolveFromStateDir_RejectsInsecureModeAndNonExecutable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX mode test")
	}
	t.Run("writable parent", func(t *testing.T) {
		fx := newOptionalPackageFixture(t)
		if err := os.Chmod(fx.packageDir, 0777); err != nil {
			t.Fatal(err)
		}
		if _, err := resolveFromStateDir(fx.stateDir, runtime.GOOS, runtime.GOARCH); err == nil || !strings.Contains(err.Error(), "writable") {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("non executable", func(t *testing.T) {
		fx := newOptionalPackageFixture(t)
		if err := os.Chmod(fx.binary, 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := resolveFromStateDir(fx.stateDir, runtime.GOOS, runtime.GOARCH); err == nil || !strings.Contains(err.Error(), "executable") {
			t.Fatalf("error = %v", err)
		}
	})
}

func newOptionalPackageFixture(t *testing.T) optionalPackageFixture {
	t.Helper()
	spec, err := signerPackageFor(runtime.GOOS, runtime.GOARCH)
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

func newManagedPackageFixture(t *testing.T, hoisted bool) optionalPackageFixture {
	t.Helper()
	spec, err := signerPackageFor(runtime.GOOS, runtime.GOARCH)
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
		stateDir: stateDir, pluginDir: pluginDir, packageDir: packageDir, spec: spec,
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
	stubInspectFunction(t, func(context.Context, string) ([]byte, error) { return data, err })
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
