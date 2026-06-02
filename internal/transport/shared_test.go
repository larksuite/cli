// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package transport

import (
	"net/http"
	"net/url"
	"testing"
	"time"
)

// TestShared_DefaultReturnsStdlibSingleton verifies the default shared transport.
func TestShared_DefaultReturnsStdlibSingleton(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	unsetProxyPluginEnv(t)
	resetProxyPluginState()
	t.Setenv(EnvNoProxy, "")
	if Shared() != http.DefaultTransport {
		t.Error("Shared should return http.DefaultTransport when LARK_CLI_NO_PROXY is unset")
	}
}

// TestShared_NoProxyReturnsClone verifies that disabling proxying returns a cloned transport.
func TestShared_NoProxyReturnsClone(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	unsetProxyPluginEnv(t)
	resetProxyPluginState()
	t.Setenv(EnvNoProxy, "1")
	tr := Shared()
	if tr == http.DefaultTransport {
		t.Fatal("Shared should return a clone, not DefaultTransport, when LARK_CLI_NO_PROXY is set")
	}
	ht, ok := tr.(*http.Transport)
	if !ok {
		t.Fatalf("expected *http.Transport, got %T", tr)
	}
	if ht.Proxy != nil {
		t.Error("no-proxy transport should have Proxy == nil")
	}
}

// TestShared_NoProxyIsCachedSingleton verifies singleton caching for the no-proxy transport.
func TestShared_NoProxyIsCachedSingleton(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	unsetProxyPluginEnv(t)
	resetProxyPluginState()
	t.Setenv(EnvNoProxy, "1")
	if Shared() != Shared() {
		t.Error("repeated Shared calls with LARK_CLI_NO_PROXY set must return the same instance")
	}
}

// TestShared_EnvUnsetAfterSetFallsBackToDefault verifies fallback to the stdlib
// transport after unsetting LARK_CLI_NO_PROXY.
func TestShared_EnvUnsetAfterSetFallsBackToDefault(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	unsetProxyPluginEnv(t)
	resetProxyPluginState()
	// Simulate a process that first runs with LARK_CLI_NO_PROXY=1 (populating
	// the no-proxy singleton), then unsets it. Subsequent calls must return
	// http.DefaultTransport, NOT the cached no-proxy clone.
	t.Setenv(EnvNoProxy, "1")
	if Shared() == http.DefaultTransport {
		t.Fatal("precondition: first call with env set should not return DefaultTransport")
	}

	t.Setenv(EnvNoProxy, "")
	if after := Shared(); after != http.DefaultTransport {
		t.Errorf("after unsetting LARK_CLI_NO_PROXY, Shared must return http.DefaultTransport, got %T", after)
	}
}

// TestShared_NoProxyOverridesSystemProxy verifies that LARK_CLI_NO_PROXY disables system proxies.
func TestShared_NoProxyOverridesSystemProxy(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	unsetProxyPluginEnv(t)
	resetProxyPluginState()
	t.Setenv("HTTPS_PROXY", "http://should-be-ignored:8888")
	t.Setenv(EnvNoProxy, "1")

	ht, ok := Shared().(*http.Transport)
	if !ok {
		t.Fatalf("expected *http.Transport, got %T", Shared())
	}
	if ht.Proxy != nil {
		t.Error("LARK_CLI_NO_PROXY should override system proxy settings")
	}
}

// TestNewHTTPClient verifies the factory wires the shared proxy-plugin-aware
// transport (instead of a bare client that bypasses proxy plugin mode).
func TestNewHTTPClient(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	unsetProxyPluginEnv(t)
	resetProxyPluginState()
	t.Setenv(EnvNoProxy, "")

	c := NewHTTPClient(7 * time.Second)
	if c.Transport == nil {
		t.Fatal("NewHTTPClient transport is nil; want shared transport")
	}
	if c.Transport != Shared() {
		t.Errorf("NewHTTPClient transport = %v, want Shared()", c.Transport)
	}
	if c.Timeout != 7*time.Second {
		t.Errorf("NewHTTPClient timeout = %v, want 7s", c.Timeout)
	}
}

// TestShared_PluginOverridesNoProxy locks the contract that proxy-plugin mode wins
// over LARK_CLI_NO_PROXY: even with NO_PROXY set, an enabled plugin forces the proxy.
func TestShared_PluginOverridesNoProxy(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	unsetProxyPluginEnv(t)
	t.Setenv(EnvNoProxy, "1") // NO_PROXY set, but the plugin must win
	resetProxyPluginState()

	writeFile(t, Path(), []byte(`{
  "LARKSUITE_CLI_PROXY_ENABLE": true,
  "LARKSUITE_CLI_PROXY_ADDRESS": "http://127.0.0.1:3128"
}`), 0600)

	tr, ok := Shared().(*http.Transport)
	if !ok {
		t.Fatalf("Shared() = %T, want proxy *http.Transport, not the NO_PROXY clone", tr)
	}
	u, err := tr.Proxy(&http.Request{URL: &url.URL{Scheme: "https", Host: "open.feishu.cn"}})
	if err != nil || u == nil || u.String() != "http://127.0.0.1:3128" {
		t.Fatalf("Proxy() = %v, %v; plugin must override NO_PROXY with the fixed proxy", u, err)
	}
}

// TestShared_MalformedConfigFailsClosedEvenWithNoProxy locks the most dangerous
// invariant of the fold: a malformed proxy_config.json must FAIL CLOSED, never
// fall through to direct egress — not even to the LARK_CLI_NO_PROXY clone.
func TestShared_MalformedConfigFailsClosedEvenWithNoProxy(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	unsetProxyPluginEnv(t)
	t.Setenv(EnvNoProxy, "1")
	resetProxyPluginState()

	writeFile(t, Path(), []byte(`{`), 0600) // malformed

	rt := Shared()
	if rt == http.DefaultTransport {
		t.Fatal("malformed config returned http.DefaultTransport — fail OPEN")
	}
	if rt == noProxyTransport() {
		t.Fatal("malformed config fell through to the NO_PROXY direct-egress clone — fail OPEN")
	}
	resp, err := rt.RoundTrip(&http.Request{URL: &url.URL{Scheme: "https", Host: "open.feishu.cn"}})
	if err == nil {
		t.Fatalf("RoundTrip() err = nil (resp=%v); malformed config must fail closed", resp)
	}
}
