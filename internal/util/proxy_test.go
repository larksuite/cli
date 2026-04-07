// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package util

import (
	"bytes"
	"net/http"
	"sync"
	"testing"
)

func TestDetectProxyEnv(t *testing.T) {
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

func TestNewBaseTransport_Default(t *testing.T) {
	t.Setenv(EnvNoProxy, "")
	tr := NewBaseTransport()
	if tr.Proxy == nil {
		t.Error("expected proxy func to be set when LARK_CLI_NO_PROXY is not set")
	}
}

func TestNewBaseTransport_NoProxy(t *testing.T) {
	t.Setenv(EnvNoProxy, "1")
	tr := NewBaseTransport()
	if tr.Proxy != nil {
		t.Error("expected proxy func to be nil when LARK_CLI_NO_PROXY=1")
	}
}

func TestWarnIfProxied_WithProxy(t *testing.T) {
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

func TestWarnIfProxied_WithoutProxy(t *testing.T) {
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

func TestWarnIfProxied_SilentWhenDisabled(t *testing.T) {
	proxyWarningOnce = sync.Once{}

	t.Setenv("HTTPS_PROXY", "http://proxy:8080")
	t.Setenv(EnvNoProxy, "1")

	var buf bytes.Buffer
	WarnIfProxied(&buf)

	if buf.Len() != 0 {
		t.Errorf("expected no warning when proxy is disabled, got: %s", buf.String())
	}
}

func TestWarnIfProxied_OnlyOnce(t *testing.T) {
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

func TestRedactProxyURL(t *testing.T) {
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

func TestWarnIfProxied_RedactsCredentials(t *testing.T) {
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

func TestNewBaseTransport_IsHTTPTransport(t *testing.T) {
	t.Setenv(EnvNoProxy, "")
	tr := NewBaseTransport()

	// Should be a valid *http.Transport that can be used
	var rt http.RoundTripper = tr
	_ = rt

	// Verify it's not the same pointer as DefaultTransport (should be a clone)
	if tr == http.DefaultTransport {
		t.Error("NewBaseTransport should return a clone, not DefaultTransport itself")
	}
}

func TestNewBaseTransport_RespectsNoProxyEnv(t *testing.T) {
	// Simulate: user sets both system proxy and our disable flag
	t.Setenv("HTTPS_PROXY", "http://should-be-ignored:8888")
	t.Setenv(EnvNoProxy, "1")

	tr := NewBaseTransport()
	if tr.Proxy != nil {
		t.Error("LARK_CLI_NO_PROXY should override system proxy settings")
	}

	// Clean up and verify proxy is restored
	t.Setenv(EnvNoProxy, "")
	tr2 := NewBaseTransport()
	if tr2.Proxy == nil {
		t.Error("proxy should be enabled when LARK_CLI_NO_PROXY is unset")
	}
}
