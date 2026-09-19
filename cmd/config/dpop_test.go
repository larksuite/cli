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

type dpopProbeFailure struct{ err error }

func (k dpopProbeFailure) Get(string, string) (string, error) { return "", k.err }
func (k dpopProbeFailure) Set(string, string, string) error   { return k.err }
func (k dpopProbeFailure) Remove(string, string) error        { return k.err }

func TestDPoPConfigPersistsOnlySelectedProfile(t *testing.T) {
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
	probeErr := errors.New("storage must not be probed for preferred or disabled")
	f.Keychain = dpopProbeFailure{err: probeErr}
	if err := NewCmdConfigDPoP(f).Execute(); err != nil {
		t.Fatal(err)
	}
	if stdout.String() != "dpop: preferred (profile \"second\", scope local)\n" || stderr.Len() != 0 {
		t.Fatalf("view output: stdout=%q stderr=%q", stdout, stderr)
	}
	for _, mode := range []string{"disabled", "preferred"} {
		stdout.Reset()
		stderr.Reset()
		cmd := NewCmdConfigDPoP(f)
		cmd.SetArgs([]string{mode})
		if err := cmd.Execute(); err != nil {
			t.Fatal(err)
		}
		loaded, err := core.LoadMultiAppConfig()
		if err != nil {
			t.Fatal(err)
		}
		if loaded.CurrentApp != "first" || loaded.Apps[0].DPoPMode != core.DPoPModeRequired ||
			loaded.Apps[1].DPoPMode != core.DPoPMode(mode) || len(loaded.Apps[1].Users) != 1 ||
			loaded.Apps[1].Users[0].UserOpenId != "ou_existing" {
			t.Fatal("policy update changed another profile or an existing user binding")
		}
		if stdout.Len() != 0 || !strings.Contains(stderr.String(), "DPoP set to "+mode) {
			t.Fatal("update output is misplaced")
		}
	}
}

func TestDPoPConfigRejectsInvalidInputAndFailedRequiredProbe(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	if err := core.SaveMultiAppConfig(&core.MultiAppConfig{Apps: []core.AppConfig{{AppId: "app-test", DPoPMode: core.DPoPModeDisabled}}}); err != nil {
		t.Fatal(err)
	}
	f, stdout, _, _ := cmdutil.TestFactory(t, nil)
	probeErr := errors.New("injected metadata store locked")
	f.Keychain = dpopProbeFailure{err: probeErr}
	for _, input := range []string{"REQUIRED", "required ", "on", "required"} {
		cmd := NewCmdConfigDPoP(f)
		cmd.SilenceUsage = true
		cmd.SilenceErrors = true
		cmd.SetArgs([]string{input})
		err := cmd.Execute()
		p, ok := errs.ProblemOf(err)
		want := errs.SubtypeInvalidArgument
		if input == "required" {
			want = errs.SubtypeDPoPKeyMissing
		}
		if !ok || p.Subtype != want {
			t.Fatalf("%q: error = %v", input, err)
		}
		if input == "required" && (p.Hint == "" || !errors.Is(err, probeErr)) {
			t.Fatalf("lost probe cause or recovery: %v", err)
		}
		loaded, loadErr := core.LoadMultiAppConfig()
		if loadErr != nil || loaded.Apps[0].DPoPMode != core.DPoPModeDisabled || stdout.Len() != 0 {
			t.Fatalf("failed command changed policy or emitted success: %v", loadErr)
		}
	}
}
