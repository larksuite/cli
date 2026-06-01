// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package util

import (
	"bytes"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

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
	// Ensure developer machine env doesn't accidentally enable proxy plugin mode
	// and change expectations for SharedTransport().
	unsetEnv(t, envvars.CliProxyEnable)
	unsetEnv(t, envvars.CliProxyAddress)
	unsetEnv(t, envvars.CliCAPath)
}

// TestDetectProxyEnv verifies proxy environment detection priority and empty-state behavior.
func TestDetectProxyEnv(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	unsetProxyPluginEnv(t)

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
	unsetProxyPluginEnv(t)
	t.Setenv(EnvNoProxy, "")
	tr := SharedTransport()
	if tr != http.DefaultTransport {
		t.Error("SharedTransport should return http.DefaultTransport when LARK_CLI_NO_PROXY is unset")
	}
}

// TestSharedTransport_NoProxyReturnsClone verifies that disabling proxying returns a cloned transport.
func TestSharedTransport_NoProxyReturnsClone(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	unsetProxyPluginEnv(t)
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
	unsetProxyPluginEnv(t)
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
	unsetProxyPluginEnv(t)
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
	unsetProxyPluginEnv(t)
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
	unsetProxyPluginEnv(t)
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
	unsetProxyPluginEnv(t)
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
	unsetProxyPluginEnv(t)
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
	unsetProxyPluginEnv(t)
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

// TestWarnIfProxied_ProxyPluginEnabled verifies that when proxy plugin mode is
// enabled, the warning describes the plugin proxy and the correct disable method
// (LARKSUITE_CLI_PROXY_ENABLE=false) instead of the misleading LARK_CLI_NO_PROXY
// instruction — even when env proxy and LARK_CLI_NO_PROXY are also set.
func TestWarnIfProxied_ProxyPluginEnabled(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	unsetProxyPluginEnv(t)
	proxyWarningOnce = sync.Once{}

	old := proxyPluginStatus
	proxyPluginStatus = func() (string, string, bool) { return "http://127.0.0.1:3128", "", true }
	t.Cleanup(func() { proxyPluginStatus = old })

	// Plugin mode overrides these; the warning must still be the plugin one.
	t.Setenv("HTTPS_PROXY", "http://corp-proxy:8080")
	t.Setenv(EnvNoProxy, "1")

	var buf bytes.Buffer
	WarnIfProxied(&buf)
	out := buf.String()

	if !strings.Contains(out, "127.0.0.1:3128") {
		t.Errorf("warning should mention the plugin proxy address, got: %s", out)
	}
	if !strings.Contains(out, envvars.CliProxyEnable) {
		t.Errorf("warning should mention %s as the disable method, got: %s", envvars.CliProxyEnable, out)
	}
	if strings.Contains(out, "Set "+EnvNoProxy+"=1") {
		t.Errorf("warning must NOT give the misleading %s disable instruction when plugin is enabled, got: %s", EnvNoProxy, out)
	}
	// No custom CA configured -> no interception warning.
	if strings.Contains(out, "custom CA") {
		t.Errorf("warning should not mention a custom CA when none is configured, got: %s", out)
	}
}

// TestWarnIfProxied_ProxyPluginCustomCAWarns verifies that when a custom CA is
// trusted, the warning surfaces the TLS-interception capability (V3).
func TestWarnIfProxied_ProxyPluginCustomCAWarns(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	unsetProxyPluginEnv(t)
	proxyWarningOnce = sync.Once{}

	old := proxyPluginStatus
	proxyPluginStatus = func() (string, string, bool) {
		return "http://127.0.0.1:3128", "/etc/lark/extra_ca.pem", true
	}
	t.Cleanup(func() { proxyPluginStatus = old })

	var buf bytes.Buffer
	WarnIfProxied(&buf)
	out := buf.String()

	if !strings.Contains(out, "custom CA") {
		t.Errorf("warning should mention the custom CA, got: %s", out)
	}
	if !strings.Contains(out, "/etc/lark/extra_ca.pem") {
		t.Errorf("warning should include the CA path, got: %s", out)
	}
	if !strings.Contains(out, "intercept") {
		t.Errorf("warning should mention TLS interception, got: %s", out)
	}
}

// TestNewHTTPClient verifies the factory wires the shared proxy-plugin-aware
// transport (instead of a bare client that bypasses proxy plugin mode).
func TestNewHTTPClient(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	unsetProxyPluginEnv(t)
	t.Setenv(EnvNoProxy, "")

	c := NewHTTPClient(7 * time.Second)
	if c.Transport == nil {
		t.Fatal("NewHTTPClient transport is nil; want shared transport")
	}
	if c.Transport != SharedTransport() {
		t.Errorf("NewHTTPClient transport = %v, want SharedTransport()", c.Transport)
	}
	if c.Timeout != 7*time.Second {
		t.Errorf("NewHTTPClient timeout = %v, want 7s", c.Timeout)
	}
}

// TestWarnIfProxied_ProxyPluginEnabledRedactsCredentials verifies the plugin
// warning never leaks credentials embedded in the configured proxy address.
func TestWarnIfProxied_ProxyPluginEnabledRedactsCredentials(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	unsetProxyPluginEnv(t)
	proxyWarningOnce = sync.Once{}

	old := proxyPluginStatus
	proxyPluginStatus = func() (string, string, bool) { return "http://user:s3cret@127.0.0.1:3128", "", true }
	t.Cleanup(func() { proxyPluginStatus = old })

	var buf bytes.Buffer
	WarnIfProxied(&buf)
	out := buf.String()

	if strings.Contains(out, "s3cret") {
		t.Errorf("plugin warning leaked password, got: %s", out)
	}
	if strings.Contains(out, "user:") {
		t.Errorf("plugin warning leaked username, got: %s", out)
	}
	if !strings.Contains(out, "***@127.0.0.1:3128") {
		t.Errorf("plugin warning should contain redacted proxy URL, got: %s", out)
	}
}

// TestRedactProxyURL verifies redaction of proxy credentials across supported formats.
func TestRedactProxyURL(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	unsetProxyPluginEnv(t)
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
	unsetProxyPluginEnv(t)
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
