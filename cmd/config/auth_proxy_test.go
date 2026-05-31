// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package config

import (
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
)

func TestConfigAuthProxyTrustCmd_RiskAndYesFlag(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, nil)
	cmd := newCmdConfigAuthProxyTrust(f)

	risk, ok := cmdutil.GetRisk(cmd)
	if !ok || risk != cmdutil.RiskHighRiskWrite {
		t.Fatalf("risk = %q, %v; want %q", risk, ok, cmdutil.RiskHighRiskWrite)
	}
	if cmd.Flags().Lookup("yes") == nil {
		t.Fatal("trust command should expose --yes confirmation flag")
	}
}

func TestConfigAuthProxyTrustCmd_RequiresConfirmation(t *testing.T) {
	clearAgentEnv(t)
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, _, _, _ := cmdutil.TestFactory(t, nil)
	cmd := newCmdConfigAuthProxyTrust(f)
	cmd.SetArgs([]string{"https://gate.example.com"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected confirmation error without --yes")
	}
	var confirmErr *errs.ConfirmationRequiredError
	if !errors.As(err, &confirmErr) {
		t.Fatalf("error = %T %v, want *errs.ConfirmationRequiredError", err, err)
	}
	if confirmErr.Risk != cmdutil.RiskHighRiskWrite {
		t.Fatalf("risk = %q, want %q", confirmErr.Risk, cmdutil.RiskHighRiskWrite)
	}
	if confirmErr.Subtype != errs.SubtypeConfirmationRequired {
		t.Fatalf("subtype = %q, want %q", confirmErr.Subtype, errs.SubtypeConfirmationRequired)
	}

	cfg, loadErr := core.LoadAuthProxyConfig()
	if loadErr != nil {
		t.Fatalf("LoadAuthProxyConfig() error = %v", loadErr)
	}
	if len(cfg.TrustedHosts) != 0 {
		t.Fatalf("TrustedHosts = %#v, want empty after refused confirmation", cfg.TrustedHosts)
	}
}

func TestConfigAuthProxyTrustCmd_RefusesAgentWorkspaceEvenWithYes(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
	}{
		{name: "openclaw", env: map[string]string{"OPENCLAW_HOME": "/tmp/openclaw"}},
		{name: "hermes", env: map[string]string{"HERMES_HOME": "/tmp/hermes"}},
		{name: "lark channel", env: map[string]string{"LARK_CHANNEL": "1"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearAgentEnv(t)
			t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
			for k, v := range tt.env {
				t.Setenv(k, v)
			}

			f, _, _, _ := cmdutil.TestFactory(t, nil)
			cmd := newCmdConfigAuthProxyTrust(f)
			cmd.SetArgs([]string{"https://gate.example.com", "--yes"})

			err := cmd.Execute()
			if err == nil {
				t.Fatal("expected trust to be refused in agent workspace")
			}
			if !strings.Contains(err.Error(), "outside the agent environment") {
				t.Fatalf("error = %v, want outside-agent guidance", err)
			}

			cfg, loadErr := core.LoadAuthProxyConfig()
			if loadErr != nil {
				t.Fatalf("LoadAuthProxyConfig() error = %v", loadErr)
			}
			if len(cfg.TrustedHosts) != 0 {
				t.Fatalf("TrustedHosts = %#v, want empty after agent refusal", cfg.TrustedHosts)
			}
		})
	}
}

func TestConfigAuthProxyTrustCmd_ConfirmedLocalTrustsHost(t *testing.T) {
	clearAgentEnv(t)
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, _, _, _ := cmdutil.TestFactory(t, nil)
	cmd := newCmdConfigAuthProxyTrust(f)
	cmd.SetArgs([]string{"https://gate.example.com", "--yes"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("cmd.Execute() error = %v", err)
	}

	cfg, err := core.LoadAuthProxyConfig()
	if err != nil {
		t.Fatalf("LoadAuthProxyConfig() error = %v", err)
	}
	if len(cfg.TrustedHosts) != 1 || cfg.TrustedHosts[0] != "gate.example.com" {
		t.Fatalf("TrustedHosts = %#v, want gate.example.com", cfg.TrustedHosts)
	}
}

func TestConfigAuthProxyTrustRun_PersistsCanonicalTrustedHost(t *testing.T) {
	clearAgentEnv(t)
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, _, _, _ := cmdutil.TestFactory(t, nil)

	if err := runConfigAuthProxyTrust(f, "https://GATE.example.com:443", true); err != nil {
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
	clearAgentEnv(t)
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, _, _, _ := cmdutil.TestFactory(t, nil)

	if err := runConfigAuthProxyTrust(f, "https://gate.example.com", true); err != nil {
		t.Fatalf("first trust error = %v", err)
	}
	if err := runConfigAuthProxyTrust(f, "gate.example.com", true); err != nil {
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
	clearAgentEnv(t)
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, _, _, _ := cmdutil.TestFactory(t, nil)

	if err := runConfigAuthProxyTrust(f, "http://gate.example.com", true); err == nil {
		t.Fatal("expected HTTP auth proxy trust to be rejected")
	}
}

func TestConfigAuthProxyUntrustRun_RemovesTrustedHost(t *testing.T) {
	clearAgentEnv(t)
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, _, _, _ := cmdutil.TestFactory(t, nil)

	if err := runConfigAuthProxyTrust(f, "https://gate.example.com", true); err != nil {
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
