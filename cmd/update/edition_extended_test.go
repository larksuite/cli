// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build extended

package cmdupdate

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/selfupdate"
	"github.com/larksuite/cli/internal/skillscheck"
)

func TestExtendedUpdateUsesExtendedReleaseInstaller(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	oldFetch, oldInstall := fetchExtendedLatest, installExtended
	oldVersion, oldUpdater, oldSync := currentVersion, newUpdater, syncSkills
	t.Cleanup(func() {
		fetchExtendedLatest, installExtended = oldFetch, oldInstall
		currentVersion, newUpdater, syncSkills = oldVersion, oldUpdater, oldSync
	})
	fetchExtendedLatest = func() (string, error) { return "1.2.4", nil }
	currentVersion = func() string { return "1.2.3" }
	installed := ""
	installExtended = func(version string) error {
		installed = version
		return nil
	}
	newUpdater = func() *selfupdate.Updater {
		return &selfupdate.Updater{DetectOverride: func() selfupdate.DetectResult {
			return selfupdate.DetectResult{Method: selfupdate.InstallManual}
		}}
	}
	syncSkills = func(skillscheck.SyncOptions) *skillscheck.SyncResult { return &skillscheck.SyncResult{} }

	var out, errOut bytes.Buffer
	f := cmdutil.NewDefault(cmdutil.NewIOStreams(nil, &out, &errOut), cmdutil.InvocationContext{})
	cmd := NewCmdUpdate(f)
	cmd.SetArgs([]string{"--json"})
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if installed != "1.2.4" {
		t.Fatalf("installed version = %q, want 1.2.4", installed)
	}
	var result map[string]interface{}
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result["edition"] != "extended" || result["action"] != "updated" {
		t.Fatalf("result = %#v", result)
	}
}

func TestExtendedUpdateCheckDoesNotInstall(t *testing.T) {
	oldFetch, oldInstall := fetchExtendedLatest, installExtended
	oldVersion := currentVersion
	t.Cleanup(func() {
		fetchExtendedLatest, installExtended = oldFetch, oldInstall
		currentVersion = oldVersion
	})
	fetchExtendedLatest = func() (string, error) { return "1.2.4", nil }
	currentVersion = func() string { return "1.2.3" }
	installExtended = func(string) error {
		t.Fatal("installer called during --check")
		return nil
	}
	var out bytes.Buffer
	f := cmdutil.NewDefault(cmdutil.NewIOStreams(nil, &out, &bytes.Buffer{}), cmdutil.InvocationContext{})
	cmd := NewCmdUpdate(f)
	cmd.SetArgs([]string{"--json", "--check"})
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err)
	}
}
