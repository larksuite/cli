// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/zalando/go-keyring"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
)

func TestConfigSetDPoPPersistsSelectedProfile(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	config := &core.MultiAppConfig{CurrentApp: "first", Apps: []core.AppConfig{
		{Name: "first", AppId: "app-first", DPoPMode: core.DPoPModeRequired},
		{Name: "second", AppId: "app-second", Users: []core.AppUser{{UserOpenId: "ou_existing"}}},
	}}
	if err := core.SaveMultiAppConfig(config); err != nil {
		t.Fatal(err)
	}
	f, stdout, stderr, _ := cmdutil.TestFactory(t, nil)
	f.Invocation.Profile = "second"

	cmd := NewCmdConfigSet(f)
	cmd.SetArgs([]string{"dpop", "disabled"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	loaded, err := core.LoadMultiAppConfig()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.CurrentApp != "first" || loaded.Apps[0].DPoPMode != core.DPoPModeRequired ||
		loaded.Apps[1].DPoPMode != core.DPoPModeDisabled || len(loaded.Apps[1].Users) != 1 ||
		loaded.Apps[1].Users[0].UserOpenId != "ou_existing" {
		t.Fatal("config set changed another profile or an existing user binding")
	}
	if stdout.Len() != 0 || stderr.Len() == 0 {
		t.Fatalf("output: stdout=%q stderr=%q", stdout, stderr)
	}
}

func TestConfigSetRejectsUnsupportedKey(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, nil)
	cmd := NewCmdConfigSet(f)
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	cmd.SetArgs([]string{"jwt", "required"})

	p, ok := errs.ProblemOf(cmd.Execute())
	if !ok || p.Subtype != errs.SubtypeInvalidArgument {
		t.Fatalf("unsupported key was not rejected with invalid_argument")
	}
}

func TestConfigSetDPoPRequiredChecksKeyStorage(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	blockedRoot := filepath.Join(t.TempDir(), "blocked")
	if err := os.WriteFile(blockedRoot, []byte("not a directory"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", blockedRoot)
	t.Setenv("LARKSUITE_CLI_DATA_DIR", blockedRoot)
	keyring.MockInit()
	if err := core.SaveMultiAppConfig(&core.MultiAppConfig{Apps: []core.AppConfig{{
		AppId: "app-test", DPoPMode: core.DPoPModeDisabled,
	}}}); err != nil {
		t.Fatal(err)
	}
	f, stdout, _, _ := cmdutil.TestFactory(t, nil)

	cmd := NewCmdConfigSet(f)
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	cmd.SetArgs([]string{"dpop", "required"})
	err := cmd.Execute()

	p, ok := errs.ProblemOf(err)
	if !ok || p.Subtype != errs.SubtypeDPoPKeyMissing || p.Hint == "" {
		t.Fatalf("error = %v, want dpop key storage probe failure with hint", err)
	}
	loaded, loadErr := core.LoadMultiAppConfig()
	if loadErr != nil || loaded.Apps[0].DPoPMode != core.DPoPModeDisabled || stdout.Len() != 0 {
		t.Fatalf("failed command changed policy or emitted success: %v", loadErr)
	}
}
