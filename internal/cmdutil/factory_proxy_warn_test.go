// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdutil

import (
	"io"
	"testing"

	_ "github.com/larksuite/cli/extension/credential/env" // registers the env-backed account provider
	"github.com/larksuite/cli/internal/envvars"
)

// installProxyWarnSpy swaps the package-level proxy-warning seams for one test:
// stderrIsTerminal is forced to `terminal`, and warnIfProxied is replaced with a
// counter. The real implementations are restored on cleanup. Returns a pointer
// to the call count so the caller can assert how many times the warning fired.
func installProxyWarnSpy(t *testing.T, terminal bool) *int {
	t.Helper()
	prevWarn, prevTTY := warnIfProxied, stderrIsTerminal
	t.Cleanup(func() {
		warnIfProxied, stderrIsTerminal = prevWarn, prevTTY
	})
	calls := 0
	warnIfProxied = func(io.Writer) { calls++ }
	stderrIsTerminal = func(*IOStreams) bool { return terminal }
	return &calls
}

var proxyWarnGateCases = []struct {
	name     string
	terminal bool
	want     int
}{
	{"terminal stderr warns once", true, 1},
	{"non-terminal stderr stays silent", false, 0},
}

// TestCachedHttpClientFunc_ProxyWarnGate verifies the http-client init path
// invokes WarnIfProxied only when stderr is an interactive terminal.
func TestCachedHttpClientFunc_ProxyWarnGate(t *testing.T) {
	for _, tc := range proxyWarnGateCases {
		t.Run(tc.name, func(t *testing.T) {
			calls := installProxyWarnSpy(t, tc.terminal)

			fn := cachedHttpClientFunc(&Factory{IOStreams: &IOStreams{ErrOut: io.Discard}})
			if _, err := fn(); err != nil {
				t.Fatalf("http client init: %v", err)
			}

			if *calls != tc.want {
				t.Errorf("WarnIfProxied calls = %d, want %d", *calls, tc.want)
			}
		})
	}
}

// TestCachedLarkClientFunc_ProxyWarnGate verifies the lark-client init path
// invokes WarnIfProxied only when stderr is an interactive terminal. The gate
// runs after ResolveAccount, so an env-backed credential is wired up to let
// account resolution succeed without network or config files.
func TestCachedLarkClientFunc_ProxyWarnGate(t *testing.T) {
	for _, tc := range proxyWarnGateCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(envvars.CliAppID, "env-app")
			t.Setenv(envvars.CliAppSecret, "env-secret")
			t.Setenv(envvars.CliDefaultAs, "")
			t.Setenv(envvars.CliUserAccessToken, "")
			t.Setenv(envvars.CliTenantAccessToken, "")
			t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

			calls := installProxyWarnSpy(t, tc.terminal)

			f := NewDefault(&IOStreams{ErrOut: io.Discard}, InvocationContext{})
			if _, err := cachedLarkClientFunc(f)(); err != nil {
				t.Fatalf("lark client init: %v", err)
			}

			if *calls != tc.want {
				t.Errorf("WarnIfProxied calls = %d, want %d", *calls, tc.want)
			}
		})
	}
}
