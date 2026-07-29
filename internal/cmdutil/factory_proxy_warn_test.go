// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdutil

import (
	"io"
	"testing"

	_ "github.com/larksuite/cli/extension/credential/env" // registers the env-backed account provider
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/envvars"
)

// installProxyWarnSpy replaces warnIfProxied with a counter for one test and
// restores it on cleanup. Returns a pointer to the call count so the caller can
// assert how many times the warning fired. The terminal state is controlled via
// the IOStreams.StderrIsTerminal field, not a seam.
func installProxyWarnSpy(t *testing.T) *int {
	t.Helper()
	prevWarn := warnIfProxied
	t.Cleanup(func() { warnIfProxied = prevWarn })
	calls := 0
	warnIfProxied = func(io.Writer) { calls++ }
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
	isEnabled := false
	for _, tc := range proxyWarnGateCases {
		t.Run(tc.name, func(t *testing.T) {
			calls := installProxyWarnSpy(t)

			f, _, _, _ := TestFactory(t, &core.CliConfig{AppID: "test-app"})
			f.IOStreams.ErrOut = io.Discard
			f.IOStreams.StderrIsTerminal = tc.terminal
			fn := cachedHttpClientFunc(f, staticWorkspaceConfig{config: &core.MultiAppConfig{RiskControl: &isEnabled}})
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

			calls := installProxyWarnSpy(t)

			// normalizeStreams copies the struct (out := *s), so the
			// StderrIsTerminal field survives into f.IOStreams.
			f := NewDefault(&IOStreams{ErrOut: io.Discard, StderrIsTerminal: tc.terminal}, InvocationContext{})
			if _, err := cachedLarkClientFunc(f, nil)(); err != nil {
				t.Fatalf("lark client init: %v", err)
			}

			if *calls != tc.want {
				t.Errorf("WarnIfProxied calls = %d, want %d", *calls, tc.want)
			}
		})
	}
}
