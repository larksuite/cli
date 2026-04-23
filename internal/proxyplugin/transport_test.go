// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package proxyplugin

import (
	"net/http"
	"net/url"
	"sync"
	"testing"
)

func resetProxyPluginState() {
	loadOnce = sync.Once{}
	loadCfg = nil
	loadErr = nil
	proxyPluginTransport = sync.OnceValue(buildProxyPluginTransport)
}

func TestSharedTransport_NotConfigured(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	unsetProxyPluginEnv(t)
	resetProxyPluginState()

	tr, ok := SharedTransport()
	if ok {
		t.Fatalf("SharedTransport() ok = true, want false")
	}
	if tr != nil {
		t.Fatalf("SharedTransport() transport = %T, want nil", tr)
	}
}

func TestSharedTransport_EnabledReturnsFixedProxy(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	unsetProxyPluginEnv(t)
	resetProxyPluginState()

	cfgPath := Path()
	writeFile(t, cfgPath, []byte(`{
  "LARKSUITE_CLI_PROXY_ENABLE": true,
  "LARKSUITE_CLI_PROXY_ADDRESS": "http://127.0.0.1:3128",
  "LARKSUITE_CLI_CA_PATH": ""
}`), 0600)

	rt, ok := SharedTransport()
	if !ok {
		t.Fatal("SharedTransport() ok = false, want true")
	}
	tr, ok := rt.(*http.Transport)
	if !ok {
		t.Fatalf("SharedTransport() = %T, want *http.Transport", rt)
	}
	u, err := tr.Proxy(&http.Request{URL: &url.URL{Scheme: "https", Host: "open.feishu.cn"}})
	if err != nil {
		t.Fatalf("Proxy() error = %v", err)
	}
	if u == nil || u.String() != "http://127.0.0.1:3128" {
		t.Fatalf("Proxy() = %v, want http://127.0.0.1:3128", u)
	}
}
