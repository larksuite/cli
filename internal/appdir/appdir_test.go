// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package appdir

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestConfigDir_PrefersExplicitOverride(t *testing.T) {
	base := realPath(t, t.TempDir())
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", filepath.Join(base, "override"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(base, "xdg-config"))
	t.Setenv("HOME", base)

	got := ConfigDir()
	want := filepath.Join(base, "override")
	if got != want {
		t.Fatalf("ConfigDir() = %q, want %q", got, want)
	}
}

func TestConfigDir_UsesLegacyDirWhenPresent(t *testing.T) {
	home := realPath(t, t.TempDir())
	legacy := filepath.Join(home, ".lark-cli")
	if err := osMkdirAll(legacy); err != nil {
		t.Fatalf("create legacy dir: %v", err)
	}
	t.Setenv("HOME", home)
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", "")
	t.Setenv("XDG_CONFIG_HOME", "")

	got := ConfigDir()
	if got != legacy {
		t.Fatalf("ConfigDir() = %q, want %q", got, legacy)
	}
}

func TestConfigDir_UsesXDGWhenLegacyMissing(t *testing.T) {
	home := realPath(t, t.TempDir())
	xdg := filepath.Join(home, "xdg-config")
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", xdg)
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", "")

	got := ConfigDir()
	want := filepath.Join(xdg, appName)
	if got != want {
		t.Fatalf("ConfigDir() = %q, want %q", got, want)
	}
}

func TestCacheDir_PrefersExplicitOverride(t *testing.T) {
	base := realPath(t, t.TempDir())
	cacheDir := filepath.Join(base, "my-cache")
	t.Setenv("LARKSUITE_CLI_CACHE_DIR", cacheDir)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(base, "xdg-cache"))

	if got := CacheDir(); got != cacheDir {
		t.Fatalf("CacheDir() = %q, want %q", got, cacheDir)
	}
}

func TestStateDir_PrefersExplicitOverride(t *testing.T) {
	base := realPath(t, t.TempDir())
	stateDir := filepath.Join(base, "my-state")
	t.Setenv("LARKSUITE_CLI_STATE_DIR", stateDir)
	t.Setenv("XDG_STATE_HOME", filepath.Join(base, "xdg-state"))

	if got := StateDir(); got != stateDir {
		t.Fatalf("StateDir() = %q, want %q", got, stateDir)
	}
	if got, want := LogDir(), filepath.Join(stateDir, "logs"); got != want {
		t.Fatalf("LogDir() = %q, want %q", got, want)
	}
}

func TestCacheStateAndLogDir_UseXDGBaseDirs(t *testing.T) {
	base := realPath(t, t.TempDir())
	t.Setenv("HOME", base)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(base, "xdg-cache"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(base, "xdg-state"))
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", "")
	t.Setenv("LARKSUITE_CLI_CACHE_DIR", "")
	t.Setenv("LARKSUITE_CLI_STATE_DIR", "")
	t.Setenv("LARKSUITE_CLI_LOG_DIR", "")

	if got, want := CacheDir(), filepath.Join(base, "xdg-cache", appName); got != want {
		t.Fatalf("CacheDir() = %q, want %q", got, want)
	}
	if got, want := StateDir(), filepath.Join(base, "xdg-state", appName); got != want {
		t.Fatalf("StateDir() = %q, want %q", got, want)
	}
	if got, want := LogDir(), filepath.Join(base, "xdg-state", appName, "logs"); got != want {
		t.Fatalf("LogDir() = %q, want %q", got, want)
	}
}

func TestLogDir_PrefersExplicitOverride(t *testing.T) {
	base := realPath(t, t.TempDir())
	logDir := filepath.Join(base, "logs")
	t.Setenv("HOME", base)
	t.Setenv("LARKSUITE_CLI_LOG_DIR", logDir)
	t.Setenv("LARKSUITE_CLI_STATE_DIR", filepath.Join(base, "state"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(base, "xdg-state"))

	got := LogDir()
	if got != logDir {
		t.Fatalf("LogDir() = %q, want %q", got, logDir)
	}
}

func TestDataDir_UsesXDGDataHome(t *testing.T) {
	base := realPath(t, t.TempDir())
	t.Setenv("HOME", base)
	t.Setenv("XDG_DATA_HOME", filepath.Join(base, "xdg-data"))
	t.Setenv("LARKSUITE_CLI_DATA_DIR", "")

	got, err := DataDir("svc")
	if err != nil {
		t.Fatalf("DataDir() error = %v", err)
	}
	want := filepath.Join(base, "xdg-data", appName, "svc")
	if got != want {
		t.Fatalf("DataDir() = %q, want %q", got, want)
	}
}

func TestDataDir_DefaultPathIncludesAppNamespace(t *testing.T) {
	base := realPath(t, t.TempDir())
	t.Setenv("HOME", base)
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("LARKSUITE_CLI_DATA_DIR", "")

	got, err := DataDir("svc")
	if err != nil {
		t.Fatalf("DataDir() error = %v", err)
	}

	want := filepath.Join(base, ".local", "share", appName, "svc")
	if runtime.GOOS == "windows" {
		want = filepath.Join(base, ".lark-cli", "data", "svc")
	}
	if got != want {
		t.Fatalf("DataDir() = %q, want %q", got, want)
	}
}

func TestDataDir_PrefersDarwinLegacyDirWhenPresent(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("darwin-only legacy path")
	}

	home := realPath(t, t.TempDir())
	legacy := filepath.Join(home, "Library", "Application Support", "svc")
	if err := osMkdirAll(legacy); err != nil {
		t.Fatalf("create legacy dir: %v", err)
	}
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("LARKSUITE_CLI_DATA_DIR", "")

	got, err := DataDir("svc")
	if err != nil {
		t.Fatalf("DataDir() error = %v", err)
	}
	if got != legacy {
		t.Fatalf("DataDir() = %q, want %q", got, legacy)
	}
}

func TestDataDir_RejectsUnsafeServiceName(t *testing.T) {
	for _, bad := range []string{"", ".", "..", "../etc", "foo/bar", "svc\x00name", "svc\tname", "svc\nname", "svc\rname"} {
		t.Run(bad, func(t *testing.T) {
			_, err := DataDir(bad)
			if err == nil {
				t.Errorf("DataDir(%q) returned no error, want error", bad)
			}
		})
	}
}

func realPath(t *testing.T, path string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatalf("EvalSymlinks(%q): %v", path, err)
	}
	return resolved
}

func osMkdirAll(path string) error {
	return os.MkdirAll(path, 0o700)
}
