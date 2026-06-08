// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package transport

import (
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
	"sync"

	"github.com/larksuite/cli/internal/envvars"
)

// Proxy environment constants control shared transport proxy behavior.
const (
	// EnvNoProxy disables automatic proxy support when set to any non-empty value.
	EnvNoProxy = "LARK_CLI_NO_PROXY"
)

// proxyEnvKeys lists environment variables that Go's ProxyFromEnvironment reads.
var proxyEnvKeys = []string{
	"HTTPS_PROXY", "https_proxy",
	"HTTP_PROXY", "http_proxy",
	"ALL_PROXY", "all_proxy",
}

// DetectProxyEnv returns the first proxy-related environment variable that is set,
// or empty strings if none are configured.
func DetectProxyEnv() (key, value string) {
	for _, k := range proxyEnvKeys {
		if v := os.Getenv(k); v != "" {
			return k, v
		}
	}
	return "", ""
}

// proxyWarningOnce ensures proxy environment warnings are emitted at most once.
var proxyWarningOnce sync.Once

// proxyPluginStatus reports the configured proxy plugin address, the extra
// trusted CA path (if any), and whether proxy plugin mode is enabled. It is
// indirected through a package variable so tests can simulate plugin-enabled
// mode without the process-global Load() sync.Once cache.
var proxyPluginStatus = func() (addr, caPath string, enabled bool) {
	cfg, err := Load()
	if err != nil || !cfg.Enabled() {
		return "", "", false
	}
	return cfg.Proxy, cfg.CAPath, true
}

// redactProxyURL masks userinfo (username:password) in a proxy URL.
// Handles both scheme-prefixed ("http://user:pass@host") and bare ("user:pass@host") formats.
func redactProxyURL(raw string) string {
	// Try standard url.Parse first (works when scheme is present)
	u, err := url.Parse(raw)
	if err == nil && u.User != nil {
		return u.Scheme + "://***@" + u.Host + u.RequestURI()
	}

	// Fallback: handle bare URLs without scheme (e.g. "user:pass@proxy:8080")
	if at := strings.LastIndex(raw, "@"); at > 0 {
		return "***@" + raw[at+1:]
	}

	return raw
}

// WarnIfProxied prints a one-time warning to w when a proxy environment variable
// is detected and proxy is not disabled via LARK_CLI_NO_PROXY. Proxy credentials
// are redacted. Safe to call multiple times; only the first call prints.
func WarnIfProxied(w io.Writer) {
	proxyWarningOnce.Do(func() {
		// Proxy plugin mode overrides env proxies and LARK_CLI_NO_PROXY (see
		// Shared), so its warning and disable instructions take precedence.
		// Emitting the env-proxy warning here would be misleading: it tells the
		// user to set LARK_CLI_NO_PROXY=1, which does NOT disable the plugin proxy.
		if addr, caPath, enabled := proxyPluginStatus(); enabled {
			fmt.Fprintf(w, "[lark-cli] [WARN] proxy plugin enabled: all requests (including credentials) are forced through %s. To disable, set %s=false or remove %s.\n",
				redactProxyURL(addr), envvars.CliProxyEnable, Path())
			if strings.TrimSpace(caPath) != "" {
				// A custom CA means upstream TLS can be intercepted/inspected by
				// the proxy (MITM). Surface it so the operator is aware traffic
				// (including Bearer tokens) is decryptable on this host.
				fmt.Fprintf(w, "[lark-cli] [WARN] proxy plugin trusts a custom CA (%s); TLS to upstreams can be intercepted/inspected by this proxy.\n",
					caPath)
			}
			return
		}
		if os.Getenv(EnvNoProxy) != "" {
			return
		}
		key, val := DetectProxyEnv()
		if key == "" {
			return
		}
		fmt.Fprintf(w, "[lark-cli] [WARN] proxy detected: %s=%s — requests (including credentials) will transit through this proxy. Set %s=1 to disable proxy.\n",
			key, redactProxyURL(val), EnvNoProxy)
	})
}
