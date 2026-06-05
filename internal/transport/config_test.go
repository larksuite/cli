// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package transport

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/larksuite/cli/internal/envvars"
)

// unsetEnv clears key for the duration of the test and restores its original value.
func unsetEnv(t *testing.T, key string) {
	t.Helper()
	old, had := os.LookupEnv(key)
	_ = os.Unsetenv(key)
	t.Cleanup(func() {
		if had {
			_ = os.Setenv(key, old)
		} else {
			_ = os.Unsetenv(key)
		}
	})
}

// unsetProxyPluginEnv clears proxy-related environment variables for deterministic tests.
func unsetProxyPluginEnv(t *testing.T) {
	t.Helper()
	unsetEnv(t, envvars.CliProxyEnable)
	unsetEnv(t, envvars.CliProxyAddress)
	unsetEnv(t, envvars.CliCAPath)
}

// writeFile creates parent directories and writes test data for fixtures.
func writeFile(t *testing.T, path string, data []byte, perm os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, data, perm); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

// TestLoad_MissingFileReturnsNil verifies that Load reports no config when no file
// or proxy environment overrides exist.
func TestLoad_MissingFileReturnsNil(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	loadOnce = sync.Once{}
	loadCfg = nil
	loadErr = nil
	unsetProxyPluginEnv(t)
	// TestLoad_MissingFileReturnsNil must reset loadOnce, loadCfg, and loadErr
	// because multiple tests in this package share the package-level Load()
	// cache via sync.Once.
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg != nil {
		t.Fatalf("Load() = %#v, want nil (missing file)", cfg)
	}
}

// TestApplyToTransport_SetsProxy verifies that a valid proxy config installs a fixed proxy.
func TestApplyToTransport_SetsProxy(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	loadOnce = sync.Once{}
	loadCfg = nil
	loadErr = nil
	unsetProxyPluginEnv(t)

	cfgPath := Path()
	writeFile(t, cfgPath, []byte(`{
  "LARKSUITE_CLI_PROXY_ENABLE": true,
  "LARKSUITE_CLI_PROXY_ADDRESS": "http://127.0.0.1:3128",
  "LARKSUITE_CLI_CA_PATH": ""
}`), 0600)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg == nil || !cfg.Enabled() {
		t.Fatalf("cfg.Enabled() = %v, want true", cfg)
	}

	base := http.DefaultTransport.(*http.Transport)
	tr, err := cfg.ApplyToTransport(base)
	if err != nil {
		t.Fatalf("ApplyToTransport() error = %v", err)
	}
	if tr.Proxy == nil {
		t.Fatal("Proxy func is nil, want fixed proxy")
	}
	u, err := tr.Proxy(&http.Request{URL: &url.URL{Scheme: "https", Host: "open.feishu.cn"}})
	if err != nil {
		t.Fatalf("Proxy() error = %v", err)
	}
	if u == nil || u.String() != "http://127.0.0.1:3128" {
		t.Fatalf("Proxy() = %v, want http://127.0.0.1:3128", u)
	}
}

// TestLoad_RejectsNonLoopbackProxy verifies that proxy mode rejects non-loopback proxies.
func TestLoad_RejectsNonLoopbackProxy(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	loadOnce = sync.Once{}
	loadCfg = nil
	loadErr = nil
	unsetProxyPluginEnv(t)

	cfgPath := Path()
	writeFile(t, cfgPath, []byte(`{
  "LARKSUITE_CLI_PROXY_ENABLE": true,
  "LARKSUITE_CLI_PROXY_ADDRESS": "http://10.0.0.1:3128",
  "LARKSUITE_CLI_CA_PATH": ""
}`), 0600)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg == nil || !cfg.Enabled() {
		t.Fatalf("cfg.Enabled() = %v, want true", cfg)
	}
	_, err = cfg.ApplyToTransport(http.DefaultTransport.(*http.Transport))
	if err == nil {
		t.Fatal("ApplyToTransport() error = nil, want invalid proxy host error")
	}
}

// TestConfig_ProxyURLRejectsUnsupportedParts verifies the configured proxy validator
// rejects URLs with missing ports, paths, queries, and fragments.
func TestConfig_ProxyURLRejectsUnsupportedParts(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "missing explicit port",
			raw:  "http://127.0.0.1",
			want: "explicit port is required",
		},
		{
			name: "trailing slash path",
			raw:  "http://127.0.0.1:3128/",
			want: "path is not allowed",
		},
		{
			name: "query string",
			raw:  "http://127.0.0.1:3128?foo=bar",
			want: "query is not allowed",
		},
		{
			name: "fragment",
			raw:  "http://127.0.0.1:3128#frag",
			want: "fragment is not allowed",
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			_, err := (&Config{Proxy: tt.raw}).proxyURL()
			if err == nil {
				t.Fatalf("proxyURL() error = nil, want substring %q", tt.want)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("proxyURL() error = %q, want substring %q", err, tt.want)
			}
		})
	}
}

// TestLoad_EnvOnlyConfig verifies that proxy settings can come entirely from environment variables.
func TestLoad_EnvOnlyConfig(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	loadOnce = sync.Once{}
	loadCfg = nil
	loadErr = nil

	t.Setenv(envvars.CliProxyEnable, "true")
	t.Setenv(envvars.CliProxyAddress, "http://127.0.0.1:7777")
	t.Setenv(envvars.CliCAPath, "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg == nil || !cfg.Enabled() {
		t.Fatalf("cfg.Enabled() = %v, want true", cfg)
	}
	tr, err := cfg.ApplyToTransport(http.DefaultTransport.(*http.Transport))
	if err != nil {
		t.Fatalf("ApplyToTransport() error = %v", err)
	}
	u, err := tr.Proxy(&http.Request{URL: &url.URL{Scheme: "https", Host: "open.feishu.cn"}})
	if err != nil {
		t.Fatalf("Proxy() error = %v", err)
	}
	if u == nil || u.String() != "http://127.0.0.1:7777" {
		t.Fatalf("Proxy() = %v, want http://127.0.0.1:7777", u)
	}
}

// TestLoad_EnvOverridesFile verifies that proxy environment variables override file values.
func TestLoad_EnvOverridesFile(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	loadOnce = sync.Once{}
	loadCfg = nil
	loadErr = nil

	// File enables with one proxy.
	cfgPath := Path()
	writeFile(t, cfgPath, []byte(`{
  "LARKSUITE_CLI_PROXY_ENABLE": true,
  "LARKSUITE_CLI_PROXY_ADDRESS": "http://127.0.0.1:3128",
  "LARKSUITE_CLI_CA_PATH": ""
}`), 0600)

	// Env overrides: disable + different proxy (should be irrelevant once disabled).
	t.Setenv(envvars.CliProxyEnable, "false")
	t.Setenv(envvars.CliProxyAddress, "http://127.0.0.1:9999")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg == nil {
		t.Fatalf("Load() = nil, want non-nil (file exists)")
	}
	if cfg.Enabled() {
		t.Fatalf("cfg.Enabled() = true, want false (env override)")
	}
}

// TestConfig_ProxyURLMalformedDoesNotLeakUserinfo verifies that a malformed proxy
// URL containing credentials does not leak those credentials in the error string.
// url.Parse error strings embed the original URL, so wrapping them with %w would
// expose user:password.
func TestConfig_ProxyURLMalformedDoesNotLeakUserinfo(t *testing.T) {
	// Invalid percent-encoding in host makes url.Parse fail while userinfo is present.
	raw := "http://user:s3cret@%zz"
	_, err := (&Config{Proxy: raw}).proxyURL()
	if err == nil {
		t.Fatal("proxyURL() error = nil, want malformed URL error")
	}
	if strings.Contains(err.Error(), "s3cret") {
		t.Fatalf("proxyURL() error leaks password: %q", err)
	}
	if strings.Contains(err.Error(), "user:") {
		t.Fatalf("proxyURL() error leaks username: %q", err)
	}
	if !strings.Contains(err.Error(), "malformed URL") {
		t.Fatalf("proxyURL() error = %q, want it to mention malformed URL", err)
	}
	// The redacted form should still be present for diagnostics.
	if !strings.Contains(err.Error(), "***") {
		t.Fatalf("proxyURL() error = %q, want redacted userinfo marker", err)
	}
}

// resetLoadState resets the package-level Load() cache for deterministic tests.
func resetLoadState() {
	loadOnce = sync.Once{}
	loadCfg = nil
	loadErr = nil
}

// TestLoad_RejectsWorldWritableConfig verifies that a world-writable proxy config
// is rejected rather than silently trusted (it could be tampered with by other
// local processes to redirect credential traffic).
func TestLoad_RejectsWorldWritableConfig(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission semantics")
	}
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	resetLoadState()
	unsetProxyPluginEnv(t)

	p := Path()
	writeFile(t, p, []byte(`{"LARKSUITE_CLI_PROXY_ENABLE":true,"LARKSUITE_CLI_PROXY_ADDRESS":"http://127.0.0.1:3128"}`), 0600)
	// Chmod (not WriteFile perm) so umask cannot strip the world-writable bit.
	if err := os.Chmod(p, 0o666); err != nil {
		t.Fatalf("Chmod: %v", err)
	}

	_, err := Load()
	if err == nil {
		t.Fatal("Load() error = nil, want unsafe-config error for world-writable file")
	}
	if !strings.Contains(err.Error(), "world-writable") {
		t.Fatalf("Load() error = %q, want world-writable rejection", err)
	}
}

// TestLoad_RejectsGroupWritableConfig verifies group-writable configs are rejected.
func TestLoad_RejectsGroupWritableConfig(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission semantics")
	}
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	resetLoadState()
	unsetProxyPluginEnv(t)

	p := Path()
	writeFile(t, p, []byte(`{"LARKSUITE_CLI_PROXY_ENABLE":true,"LARKSUITE_CLI_PROXY_ADDRESS":"http://127.0.0.1:3128"}`), 0600)
	if err := os.Chmod(p, 0o660); err != nil {
		t.Fatalf("Chmod: %v", err)
	}

	_, err := Load()
	if err == nil {
		t.Fatal("Load() error = nil, want unsafe-config error for group-writable file")
	}
	if !strings.Contains(err.Error(), "group-writable") {
		t.Fatalf("Load() error = %q, want group-writable rejection", err)
	}
}

// TestLoad_RejectsSymlinkConfig verifies that a symlinked proxy config is rejected,
// preventing redirection of the trusted config path to an attacker-controlled file.
func TestLoad_RejectsSymlinkConfig(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation is privileged on Windows")
	}
	dir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", dir)
	resetLoadState()
	unsetProxyPluginEnv(t)

	// Real file lives elsewhere; the config path is a symlink to it.
	real := filepath.Join(dir, "real_proxy_config.json")
	writeFile(t, real, []byte(`{"LARKSUITE_CLI_PROXY_ENABLE":true,"LARKSUITE_CLI_PROXY_ADDRESS":"http://127.0.0.1:3128"}`), 0600)
	if err := os.Symlink(real, Path()); err != nil {
		t.Fatalf("Symlink: %v", err)
	}

	_, err := Load()
	if err == nil {
		t.Fatal("Load() error = nil, want unsafe-config error for symlinked file")
	}
	if !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("Load() error = %q, want symlink rejection", err)
	}
}

// TestLoad_AcceptsSecureConfig verifies the audit does not break the normal case:
// an owner-only 0600 config still loads.
func TestLoad_AcceptsSecureConfig(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	resetLoadState()
	unsetProxyPluginEnv(t)

	writeFile(t, Path(), []byte(`{"LARKSUITE_CLI_PROXY_ENABLE":true,"LARKSUITE_CLI_PROXY_ADDRESS":"http://127.0.0.1:3128"}`), 0600)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v, want nil for secure 0600 config", err)
	}
	if cfg == nil || !cfg.Enabled() {
		t.Fatalf("cfg.Enabled() = %v, want true", cfg)
	}
}
