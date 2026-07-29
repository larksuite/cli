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

func TestRiskControlWorkspacePolicy(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	config := &core.MultiAppConfig{Apps: []core.AppConfig{{
		AppId: "cli_test", AppSecret: core.PlainSecret("secret"), Brand: core.BrandFeishu,
	}}}
	if err := core.SaveMultiAppConfig(config); err != nil {
		t.Fatal(err)
	}

	f, stdout, stderr, _ := cmdutil.TestFactory(t, nil)
	cmd := NewCmdConfigRiskControl(f)
	cmd.SetArgs([]string{"off"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("set off: %v", err)
	}
	loaded, err := core.LoadMultiAppConfig()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.RiskControl == nil || *loaded.RiskControl {
		t.Fatalf("RiskControl = %v, want explicit false", loaded.RiskControl)
	}
	if !strings.Contains(stderr.String(), "set to off") {
		t.Fatalf("stderr = %q", stderr.String())
	}

	stdout.Reset()
	cmd = NewCmdConfigRiskControl(f)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("show: %v", err)
	}
	if got := stdout.String(); got != "risk-control: off (source: workspace)\n" {
		t.Fatalf("stdout = %q", got)
	}

	cmd = NewCmdConfigRiskControl(f)
	cmd.SetArgs([]string{"on"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("set on: %v", err)
	}
	loaded, err = core.LoadMultiAppConfig()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.RiskControl == nil || !*loaded.RiskControl {
		t.Fatalf("RiskControl = %v, want explicit true", loaded.RiskControl)
	}

	cmd = NewCmdConfigRiskControl(f)
	cmd.SetArgs([]string{"default"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("reset default: %v", err)
	}
	loaded, err = core.LoadMultiAppConfig()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.RiskControl != nil {
		t.Fatalf("RiskControl = %v, want nil", loaded.RiskControl)
	}

	stdout.Reset()
	cmd = NewCmdConfigRiskControl(f)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("show default: %v", err)
	}
	if got := stdout.String(); got != "risk-control: on (source: default)\n" {
		t.Fatalf("stdout = %q", got)
	}
}

func TestRiskControlWorkspacePolicyRejectsInvalidValue(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	if err := core.SaveMultiAppConfig(&core.MultiAppConfig{Apps: []core.AppConfig{{
		AppId: "cli_test", AppSecret: core.PlainSecret("secret"), Brand: core.BrandFeishu,
	}}}); err != nil {
		t.Fatal(err)
	}

	f, _, _, _ := cmdutil.TestFactory(t, nil)
	cmd := NewCmdConfigRiskControl(f)
	cmd.SetArgs([]string{"invalid"})
	err := cmd.Execute()
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("error = %T %v, want *errs.ValidationError", err, err)
	}
	if validationErr.Subtype != errs.SubtypeInvalidArgument {
		t.Fatalf("subtype = %q, want %q", validationErr.Subtype, errs.SubtypeInvalidArgument)
	}
}

func TestRiskControlWorkspacePolicyAllowedWithExternalCredentials(t *testing.T) {
	f := newConfigFactoryWithExternalProvider(t)
	config := &core.MultiAppConfig{Apps: []core.AppConfig{{
		AppId: "cli_test", AppSecret: core.PlainSecret("secret"), Brand: core.BrandFeishu,
	}}}
	if err := core.SaveMultiAppConfig(config); err != nil {
		t.Fatal(err)
	}

	cmd := NewCmdConfig(f)
	cmd.SetArgs([]string{"risk-control", "off"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("set off with external credentials: %v", err)
	}

	loaded, err := core.LoadMultiAppConfig()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.RiskControl == nil || *loaded.RiskControl {
		t.Fatalf("RiskControl = %v, want explicit false", loaded.RiskControl)
	}
}
