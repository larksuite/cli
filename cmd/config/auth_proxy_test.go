// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package config

import (
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
)

func TestConfigAuthProxyTrustRun_PersistsCanonicalTrustedHost(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, _, _, _ := cmdutil.TestFactory(t, nil)

	if err := runConfigAuthProxyTrust(f, "https://GATE.example.com:443"); err != nil {
		t.Fatalf("runConfigAuthProxyTrust() error = %v", err)
	}

	cfg, err := core.LoadAuthProxyConfig()
	if err != nil {
		t.Fatalf("LoadAuthProxyConfig() error = %v", err)
	}
	if len(cfg.TrustedHosts) != 1 || cfg.TrustedHosts[0] != "gate.example.com" {
		t.Fatalf("TrustedHosts = %#v, want gate.example.com", cfg.TrustedHosts)
	}
}

func TestConfigAuthProxyTrustRun_IsIdempotent(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, _, _, _ := cmdutil.TestFactory(t, nil)

	if err := runConfigAuthProxyTrust(f, "https://gate.example.com"); err != nil {
		t.Fatalf("first trust error = %v", err)
	}
	if err := runConfigAuthProxyTrust(f, "gate.example.com"); err != nil {
		t.Fatalf("second trust error = %v", err)
	}

	cfg, err := core.LoadAuthProxyConfig()
	if err != nil {
		t.Fatalf("LoadAuthProxyConfig() error = %v", err)
	}
	if len(cfg.TrustedHosts) != 1 || cfg.TrustedHosts[0] != "gate.example.com" {
		t.Fatalf("TrustedHosts = %#v, want one gate.example.com", cfg.TrustedHosts)
	}
}

func TestConfigAuthProxyTrustRun_RejectsHTTP(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, _, _, _ := cmdutil.TestFactory(t, nil)

	if err := runConfigAuthProxyTrust(f, "http://gate.example.com"); err == nil {
		t.Fatal("expected HTTP auth proxy trust to be rejected")
	}
}

func TestConfigAuthProxyUntrustRun_RemovesTrustedHost(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, _, _, _ := cmdutil.TestFactory(t, nil)

	if err := runConfigAuthProxyTrust(f, "https://gate.example.com"); err != nil {
		t.Fatalf("trust error = %v", err)
	}
	if err := runConfigAuthProxyUntrust(f, "gate.example.com"); err != nil {
		t.Fatalf("untrust error = %v", err)
	}

	cfg, err := core.LoadAuthProxyConfig()
	if err != nil {
		t.Fatalf("LoadAuthProxyConfig() error = %v", err)
	}
	if len(cfg.TrustedHosts) != 0 {
		t.Fatalf("TrustedHosts = %#v, want empty", cfg.TrustedHosts)
	}
}
