// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package transport

import (
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
)

func resetProxyPluginState() {
	loadOnce = sync.Once{}
	loadCfg = nil
	loadErr = nil
	proxyPluginTransport = sync.OnceValue(buildProxyPluginTransport)
	cachedBlockedTransport = sync.OnceValue(buildBlockedTransport)
}

func TestPluginTransport_NotConfigured(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	unsetProxyPluginEnv(t)
	resetProxyPluginState()

	tr, ok := pluginTransport()
	if ok {
		t.Fatalf("pluginTransport() ok = true, want false")
	}
	if tr != nil {
		t.Fatalf("pluginTransport() transport = %T, want nil", tr)
	}
}

func TestPluginTransport_EnabledReturnsFixedProxy(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	unsetProxyPluginEnv(t)
	resetProxyPluginState()

	cfgPath := Path()
	writeFile(t, cfgPath, []byte(`{
  "LARKSUITE_CLI_PROXY_ENABLE": true,
  "LARKSUITE_CLI_PROXY_ADDRESS": "http://127.0.0.1:3128",
  "LARKSUITE_CLI_CA_PATH": ""
}`), 0600)

	rt, ok := pluginTransport()
	if !ok {
		t.Fatal("pluginTransport() ok = false, want true")
	}
	tr, ok := rt.(*http.Transport)
	if !ok {
		t.Fatalf("pluginTransport() = %T, want *http.Transport", rt)
	}
	u, err := tr.Proxy(&http.Request{URL: &url.URL{Scheme: "https", Host: "open.feishu.cn"}})
	if err != nil {
		t.Fatalf("Proxy() error = %v", err)
	}
	if u == nil || u.String() != "http://127.0.0.1:3128" {
		t.Fatalf("Proxy() = %v, want http://127.0.0.1:3128", u)
	}
}

func TestPluginTransport_InvalidConfigWithNonTransportDefaultFailsClosed(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	unsetProxyPluginEnv(t)
	resetProxyPluginState()
	restoreDefaultTransport := replaceDefaultTransport(okRoundTripper{})
	defer restoreDefaultTransport()

	writeFile(t, Path(), []byte(`{`), 0600)

	rt, ok := pluginTransport()
	if !ok {
		t.Fatal("pluginTransport() ok = false, want true")
	}
	if rt == http.DefaultTransport {
		t.Fatalf("pluginTransport() returned http.DefaultTransport, want fail-closed transport")
	}
	resp, err := rt.RoundTrip(&http.Request{URL: &url.URL{Scheme: "https", Host: "open.feishu.cn"}})
	if err == nil {
		t.Fatalf("RoundTrip() error = nil, response = %#v; want fail-closed error", resp)
	}
	if resp != nil {
		t.Fatalf("RoundTrip() response = %#v, want nil", resp)
	}
}

func TestPluginTransport_InvalidConfigReturnsCachedInstance(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	unsetProxyPluginEnv(t)
	resetProxyPluginState()

	writeFile(t, Path(), []byte(`{`), 0600)

	a, ok := pluginTransport()
	if !ok {
		t.Fatal("pluginTransport() ok = false, want true")
	}
	b, ok := pluginTransport()
	if !ok {
		t.Fatal("pluginTransport() ok = false, want true")
	}
	if a != b {
		t.Fatalf("pluginTransport() returned different instances on repeated calls; blocked transport must be cached")
	}
}

func TestBuildProxyPluginTransport_InvalidConfigFailsClosed(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	unsetProxyPluginEnv(t)
	resetProxyPluginState()

	writeFile(t, Path(), []byte(`{`), 0600)

	rt := buildProxyPluginTransport()
	if rt == http.DefaultTransport {
		t.Fatalf("buildProxyPluginTransport() returned http.DefaultTransport, want fail-closed transport")
	}
	resp, err := rt.RoundTrip(&http.Request{URL: &url.URL{Scheme: "https", Host: "open.feishu.cn"}})
	if err == nil {
		t.Fatalf("RoundTrip() error = nil, response = %#v; want fail-closed error", resp)
	}
	if resp != nil {
		t.Fatalf("RoundTrip() response = %#v, want nil", resp)
	}
}

func TestBuildProxyPluginTransport_NonTransportDefaultFailsClosed(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	unsetProxyPluginEnv(t)
	resetProxyPluginState()
	restoreDefaultTransport := replaceDefaultTransport(okRoundTripper{})
	defer restoreDefaultTransport()

	rt := buildProxyPluginTransport()
	if rt == http.DefaultTransport {
		t.Fatalf("buildProxyPluginTransport() returned http.DefaultTransport, want fail-closed transport")
	}
	resp, err := rt.RoundTrip(&http.Request{URL: &url.URL{Scheme: "https", Host: "open.feishu.cn"}})
	if err == nil {
		t.Fatalf("RoundTrip() error = nil, response = %#v; want fail-closed error", resp)
	}
	if resp != nil {
		t.Fatalf("RoundTrip() response = %#v, want nil", resp)
	}
}

// TestPluginTransport_InvalidConfigBlockerIsConcreteTransport guards the
// fail-closed invariant that Fallback relies on: even when
// http.DefaultTransport is not an *http.Transport, an invalid proxy config must
// produce a blocked transport that is itself a concrete *http.Transport. If it
// were a bare RoundTripper, Fallback would downcast-fail and
// silently degrade it into a direct-egress transport.
func TestPluginTransport_InvalidConfigBlockerIsConcreteTransport(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	unsetProxyPluginEnv(t)
	resetProxyPluginState()
	restoreDefaultTransport := replaceDefaultTransport(okRoundTripper{})
	defer restoreDefaultTransport()

	writeFile(t, Path(), []byte(`{`), 0600)

	rt, ok := pluginTransport()
	if !ok {
		t.Fatal("pluginTransport() ok = false, want true")
	}
	if _, isTransport := rt.(*http.Transport); !isTransport {
		t.Fatalf("pluginTransport() blocked transport = %T, want *http.Transport so Fallback cannot degrade it to direct egress", rt)
	}
	// Must remain fail-closed.
	resp, err := rt.RoundTrip(&http.Request{URL: &url.URL{Scheme: "https", Host: "open.feishu.cn"}})
	if err == nil {
		t.Fatalf("RoundTrip() error = nil, response = %#v; want fail-closed error", resp)
	}
	if resp != nil {
		t.Fatalf("RoundTrip() response = %#v, want nil", resp)
	}
}

type okRoundTripper struct{}

func (okRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(""))}, nil
}

func replaceDefaultTransport(rt http.RoundTripper) func() {
	original := http.DefaultTransport
	http.DefaultTransport = rt
	return func() {
		http.DefaultTransport = original
	}
}
