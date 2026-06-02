// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package util

import (
	"bytes"
	"net/http"
	"os"
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
	// Ensure developer machine env doesn't accidentally enable SEC plugin mode
	// and change expectations for SharedTransport().
	unsetEnv(t, envvars.CliSecEnable)
	unsetEnv(t, envvars.CliSecProxy)
	unsetEnv(t, envvars.CliSecCA)
	unsetEnv(t, envvars.CliSecAuth)
}

// TestDetectProxyEnv verifies proxy environment detection priority and empty-state behavior.
func TestDetectProxyEnv(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	unsetSecPluginEnv(t)

	// Clear all proxy env vars first
	for _, k := range proxyEnvKeys {
		t.Setenv(k, "")
	}

	key, val := DetectProxyEnv()
	if key != "" || val != "" {
		t.Errorf("expected no proxy, got %s=%s", key, val)
	}

	t.Setenv("HTTPS_PROXY", "http://proxy:8888")
	key, val = DetectProxyEnv()
	if key != "HTTPS_PROXY" || val != "http://proxy:8888" {
		t.Errorf("expected HTTPS_PROXY=http://proxy:8888, got %s=%s", key, val)
	}
}

// TestSharedTransport_DefaultReturnsStdlibSingleton verifies the default shared transport.
func TestSharedTransport_DefaultReturnsStdlibSingleton(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	unsetSecPluginEnv(t)
	t.Setenv(EnvNoProxy, "")
	tr := SharedTransport()
	if tr != http.DefaultTransport {
		t.Error("SharedTransport should return http.DefaultTransport when LARK_CLI_NO_PROXY is unset")
	}
}

// TestSharedTransport_NoProxyReturnsClone verifies that disabling proxying returns a cloned transport.
func TestSharedTransport_NoProxyReturnsClone(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	unsetSecPluginEnv(t)
	t.Setenv(EnvNoProxy, "1")
	tr := SharedTransport()
	if tr == http.DefaultTransport {
		t.Fatal("SharedTransport should return a clone, not DefaultTransport, when LARK_CLI_NO_PROXY is set")
	}
	ht, ok := tr.(*http.Transport)
	if !ok {
		t.Fatalf("expected *http.Transport, got %T", tr)
	}
	if ht.Proxy != nil {
		t.Error("no-proxy transport should have Proxy == nil")
	}
}

// TestSharedTransport_NoProxyIsCachedSingleton verifies singleton caching for the no-proxy transport.
func TestSharedTransport_NoProxyIsCachedSingleton(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	unsetSecPluginEnv(t)
	t.Setenv(EnvNoProxy, "1")
	a := SharedTransport()
	b := SharedTransport()
	if a != b {
		t.Error("repeated SharedTransport calls with LARK_CLI_NO_PROXY set must return the same instance")
	}
}

// TestSharedTransport_EnvUnsetAfterSetFallsBackToDefault verifies fallback to the stdlib transport after unsetting EnvNoProxy.
func TestSharedTransport_EnvUnsetAfterSetFallsBackToDefault(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	unsetSecPluginEnv(t)
	// Simulate a process that first runs with LARK_CLI_NO_PROXY=1 (populating
	// the no-proxy singleton), then unsets it. Subsequent calls must return
	// http.DefaultTransport, NOT the cached no-proxy clone.
	t.Setenv(EnvNoProxy, "1")
	noProxy := SharedTransport()
	if noProxy == http.DefaultTransport {
		t.Fatal("precondition: first call with env set should not return DefaultTransport")
	}

	t.Setenv(EnvNoProxy, "")
	after := SharedTransport()
	if after != http.DefaultTransport {
		t.Errorf("after unsetting LARK_CLI_NO_PROXY, SharedTransport must return http.DefaultTransport, got %T (%p)", after, after)
	}
}

// TestSharedTransport_NoProxyOverridesSystemProxy verifies that EnvNoProxy disables system proxies.
func TestSharedTransport_NoProxyOverridesSystemProxy(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	unsetSecPluginEnv(t)
	t.Setenv("HTTPS_PROXY", "http://should-be-ignored:8888")
	t.Setenv(EnvNoProxy, "1")

	ht, ok := SharedTransport().(*http.Transport)
	if !ok {
		t.Fatalf("expected *http.Transport, got %T", SharedTransport())
	}
	if ht.Proxy != nil {
		t.Error("LARK_CLI_NO_PROXY should override system proxy settings")
	}
}

// TestWarnIfProxied_WithProxy verifies that proxy detection emits a warning.
func TestWarnIfProxied_WithProxy(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	unsetSecPluginEnv(t)
	// Reset the once guard for this test
	proxyWarningOnce = sync.Once{}

	t.Setenv("HTTPS_PROXY", "http://corp-proxy:3128")

	var buf bytes.Buffer
	WarnIfProxied(&buf)

	out := buf.String()
	if out == "" {
		t.Error("expected warning output when proxy is set")
	}
	if !bytes.Contains([]byte(out), []byte("HTTPS_PROXY")) {
		t.Errorf("warning should mention HTTPS_PROXY, got: %s", out)
	}
	if !bytes.Contains([]byte(out), []byte(EnvNoProxy)) {
		t.Errorf("warning should mention %s, got: %s", EnvNoProxy, out)
	}
}

// TestWarnIfProxied_WithoutProxy verifies that no warning is emitted without proxy settings.
func TestWarnIfProxied_WithoutProxy(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	unsetSecPluginEnv(t)
	proxyWarningOnce = sync.Once{}

	for _, k := range proxyEnvKeys {
		t.Setenv(k, "")
	}

	var buf bytes.Buffer
	WarnIfProxied(&buf)

	if buf.Len() != 0 {
		t.Errorf("expected no output when no proxy is set, got: %s", buf.String())
	}
}

// TestWarnIfProxied_SilentWhenDisabled verifies that EnvNoProxy suppresses warnings.
func TestWarnIfProxied_SilentWhenDisabled(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	unsetSecPluginEnv(t)
	proxyWarningOnce = sync.Once{}

	t.Setenv("HTTPS_PROXY", "http://proxy:8080")
	t.Setenv(EnvNoProxy, "1")

	var buf bytes.Buffer
	WarnIfProxied(&buf)

	if buf.Len() != 0 {
		t.Errorf("expected no warning when proxy is disabled, got: %s", buf.String())
	}
}

// TestWarnIfProxied_OnlyOnce verifies that proxy warnings are emitted only once.
func TestWarnIfProxied_OnlyOnce(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	unsetSecPluginEnv(t)
	proxyWarningOnce = sync.Once{}

	t.Setenv("HTTP_PROXY", "http://proxy:1234")

	var buf bytes.Buffer
	WarnIfProxied(&buf)
	first := buf.String()

	WarnIfProxied(&buf)
	second := buf.String()

	if first == "" {
		t.Error("expected warning on first call")
	}
	if second != first {
		t.Error("expected no additional output on second call")
	}
}

// TestRedactProxyURL verifies redaction of proxy credentials across supported formats.
func TestRedactProxyURL(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	unsetSecPluginEnv(t)
	tests := []struct {
		input string
		want  string
	}{
		{"http://proxy:8080", "http://proxy:8080"},
		{"http://user:pass@proxy:8080", "http://***@proxy:8080/"},
		{"http://user:p%40ss@proxy:8080/path", "http://***@proxy:8080/path"},
		{"http://user@proxy:8080", "http://***@proxy:8080/"},
		{"socks5://admin:secret@10.0.0.1:1080", "socks5://***@10.0.0.1:1080/"},
		{"user:pass@proxy:8080", "***@proxy:8080"},
		{"admin:s3cret@10.0.0.1:3128", "***@10.0.0.1:3128"},
		{"not-a-url", "not-a-url"},
		{"", ""},
	}
	for _, tt := range tests {
		got := redactProxyURL(tt.input)
		if got != tt.want {
			t.Errorf("redactProxyURL(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// TestWarnIfProxied_RedactsCredentials verifies that warning output never leaks credentials.
func TestWarnIfProxied_RedactsCredentials(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	unsetSecPluginEnv(t)
	proxyWarningOnce = sync.Once{}

	t.Setenv("HTTPS_PROXY", "http://admin:s3cret@proxy:8080")

	var buf bytes.Buffer
	WarnIfProxied(&buf)

	out := buf.String()
	if bytes.Contains([]byte(out), []byte("s3cret")) {
		t.Errorf("warning should not contain proxy password, got: %s", out)
	}
	if bytes.Contains([]byte(out), []byte("admin")) {
		t.Errorf("warning should not contain proxy username, got: %s", out)
	}
	if !bytes.Contains([]byte(out), []byte("***@proxy:8080")) {
		t.Errorf("warning should contain redacted proxy URL, got: %s", out)
	}
}
