// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package secplugin

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
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

// unsetSecPluginEnv clears SEC-related environment variables for deterministic tests.
func unsetSecPluginEnv(t *testing.T) {
	t.Helper()
	unsetEnv(t, envvars.CliSecEnable)
	unsetEnv(t, envvars.CliSecProxy)
	unsetEnv(t, envvars.CliSecCA)
	unsetEnv(t, envvars.CliSecAuth)
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
// or SEC environment overrides exist.
func TestLoad_MissingFileReturnsNil(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	loadOnce = sync.Once{}
	loadCfg = nil
	loadErr = nil
	unsetSecPluginEnv(t)
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

// TestApplyToTransport_SetsProxy verifies that a valid SEC config installs a fixed proxy.
func TestApplyToTransport_SetsProxy(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	loadOnce = sync.Once{}
	loadCfg = nil
	loadErr = nil
	unsetSecPluginEnv(t)

	cfgPath := Path()
	writeFile(t, cfgPath, []byte(`{
  "LARKSUITE_CLI_SEC_ENABLE": true,
  "LARKSUITE_CLI_SEC_PROXY": "http://127.0.0.1:3128",
  "LARKSUITE_CLI_SEC_CA": "",
  "LARKSUITE_CLI_SEC_AUTH": false
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

// TestLoad_RejectsNonLoopbackProxy verifies that SEC mode rejects non-loopback proxies.
func TestLoad_RejectsNonLoopbackProxy(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	loadOnce = sync.Once{}
	loadCfg = nil
	loadErr = nil
	unsetSecPluginEnv(t)

	cfgPath := Path()
	writeFile(t, cfgPath, []byte(`{
  "LARKSUITE_CLI_SEC_ENABLE": true,
  "LARKSUITE_CLI_SEC_PROXY": "http://10.0.0.1:3128",
  "LARKSUITE_CLI_SEC_CA": "",
  "LARKSUITE_CLI_SEC_AUTH": false
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

// TestConfig_ProxyURLRejectsUnsupportedParts verifies the SEC proxy validator
// rejects URLs with missing ports, queries, and fragments.
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

// TestLoad_EnvOnlyConfig verifies that SEC settings can come entirely from environment variables.
func TestLoad_EnvOnlyConfig(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	loadOnce = sync.Once{}
	loadCfg = nil
	loadErr = nil

	t.Setenv(envvars.CliSecEnable, "true")
	t.Setenv(envvars.CliSecProxy, "http://127.0.0.1:7777")
	t.Setenv(envvars.CliSecCA, "")
	t.Setenv(envvars.CliSecAuth, "true")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg == nil || !cfg.Enabled() {
		t.Fatalf("cfg.Enabled() = %v, want true", cfg)
	}
	if !cfg.AuthEnabled() {
		t.Fatalf("cfg.AuthEnabled() = false, want true")
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

// TestLoad_EnvOverridesFile verifies that SEC environment variables override file values.
func TestLoad_EnvOverridesFile(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	loadOnce = sync.Once{}
	loadCfg = nil
	loadErr = nil

	// File enables with one proxy.
	cfgPath := Path()
	writeFile(t, cfgPath, []byte(`{
  "LARKSUITE_CLI_SEC_ENABLE": true,
  "LARKSUITE_CLI_SEC_PROXY": "http://127.0.0.1:3128",
  "LARKSUITE_CLI_SEC_CA": "",
  "LARKSUITE_CLI_SEC_AUTH": false
}`), 0600)

	// Env overrides: disable + different proxy (should be irrelevant once disabled).
	t.Setenv(envvars.CliSecEnable, "false")
	t.Setenv(envvars.CliSecProxy, "http://127.0.0.1:9999")

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
