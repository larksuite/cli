// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package profile

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/vfs"
)

type failRenameFS struct {
	vfs.OsFs
	err error
}

func (fs *failRenameFS) Rename(oldpath, newpath string) error {
	return fs.err
}

func setupProfileConfigDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", dir)
	return dir
}

func TestProfileAddRun_InvalidExistingConfigReturnsError(t *testing.T) {
	dir := setupProfileConfigDir(t)
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte("{invalid json"), 0600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	f, _, _, _ := cmdutil.TestFactory(t, nil)
	f.IOStreams.In = strings.NewReader("secret\n")

	err := profileAddRun(f, "test", "app-test", true, "feishu", "zh", false)
	if err == nil {
		t.Fatal("expected error for invalid existing config")
	}
	if !strings.Contains(err.Error(), "failed to load config") {
		t.Fatalf("error = %v, want failed to load config", err)
	}
}

func TestProfileAddRun_UseAfterUpdatesCurrentAndPrevious(t *testing.T) {
	setupProfileConfigDir(t)
	multi := &core.MultiAppConfig{
		CurrentApp: "default",
		Apps: []core.AppConfig{
			{Name: "default", AppId: "app-default", AppSecret: core.PlainSecret("secret-default"), Brand: core.BrandFeishu},
		},
	}
	if err := core.SaveMultiAppConfig(multi); err != nil {
		t.Fatalf("SaveMultiAppConfig() error = %v", err)
	}

	f, _, _, _ := cmdutil.TestFactory(t, nil)
	f.IOStreams.In = strings.NewReader("secret-new\n")

	if err := profileAddRun(f, "target", "app-target", true, "lark", "en", true); err != nil {
		t.Fatalf("profileAddRun() error = %v", err)
	}

	saved, err := core.LoadMultiAppConfig()
	if err != nil {
		t.Fatalf("LoadMultiAppConfig() error = %v", err)
	}
	if saved.CurrentApp != "target" {
		t.Fatalf("CurrentApp = %q, want %q", saved.CurrentApp, "target")
	}
	if saved.PreviousApp != "default" {
		t.Fatalf("PreviousApp = %q, want %q", saved.PreviousApp, "default")
	}
	if len(saved.Apps) != 2 {
		t.Fatalf("len(Apps) = %d, want 2", len(saved.Apps))
	}
}

func TestProfileRemoveRun_RemovesCurrentProfileAndSwitchesToFirstRemaining(t *testing.T) {
	setupProfileConfigDir(t)
	multi := &core.MultiAppConfig{
		CurrentApp:  "target",
		PreviousApp: "default",
		Apps: []core.AppConfig{
			{Name: "default", AppId: "app-default", AppSecret: core.PlainSecret("secret-default"), Brand: core.BrandFeishu},
			{Name: "target", AppId: "app-target", AppSecret: core.PlainSecret("secret-target"), Brand: core.BrandLark},
		},
	}
	if err := core.SaveMultiAppConfig(multi); err != nil {
		t.Fatalf("SaveMultiAppConfig() error = %v", err)
	}

	f, _, _, _ := cmdutil.TestFactory(t, nil)
	if err := profileRemoveRun(f, "target"); err != nil {
		t.Fatalf("profileRemoveRun() error = %v", err)
	}

	saved, err := core.LoadMultiAppConfig()
	if err != nil {
		t.Fatalf("LoadMultiAppConfig() error = %v", err)
	}
	if saved.CurrentApp != "default" {
		t.Fatalf("CurrentApp = %q, want %q", saved.CurrentApp, "default")
	}
	if saved.PreviousApp != "default" {
		t.Fatalf("PreviousApp = %q, want %q", saved.PreviousApp, "default")
	}
	if len(saved.Apps) != 1 || saved.Apps[0].ProfileName() != "default" {
		t.Fatalf("remaining apps = %#v, want only default", saved.Apps)
	}
}

func TestProfileRenameRun_UpdatesCurrentAndPreviousReferences(t *testing.T) {
	setupProfileConfigDir(t)
	multi := &core.MultiAppConfig{
		CurrentApp:  "old",
		PreviousApp: "old",
		Apps: []core.AppConfig{{
			Name:      "old",
			AppId:     "app-old",
			AppSecret: core.PlainSecret("secret-old"),
			Brand:     core.BrandFeishu,
		}},
	}
	if err := core.SaveMultiAppConfig(multi); err != nil {
		t.Fatalf("SaveMultiAppConfig() error = %v", err)
	}

	f, _, _, _ := cmdutil.TestFactory(t, nil)
	if err := profileRenameRun(f, "old", "new"); err != nil {
		t.Fatalf("profileRenameRun() error = %v", err)
	}

	saved, err := core.LoadMultiAppConfig()
	if err != nil {
		t.Fatalf("LoadMultiAppConfig() error = %v", err)
	}
	if saved.CurrentApp != "new" {
		t.Fatalf("CurrentApp = %q, want %q", saved.CurrentApp, "new")
	}
	if saved.PreviousApp != "new" {
		t.Fatalf("PreviousApp = %q, want %q", saved.PreviousApp, "new")
	}
	if saved.Apps[0].ProfileName() != "new" {
		t.Fatalf("ProfileName() = %q, want %q", saved.Apps[0].ProfileName(), "new")
	}
}

func TestProfileRenameRun_AllowsRenameToOwnAppID(t *testing.T) {
	setupProfileConfigDir(t)
	multi := &core.MultiAppConfig{
		CurrentApp:  "old",
		PreviousApp: "old",
		Apps: []core.AppConfig{{
			Name:      "old",
			AppId:     "app-old",
			AppSecret: core.PlainSecret("secret-old"),
			Brand:     core.BrandFeishu,
		}},
	}
	if err := core.SaveMultiAppConfig(multi); err != nil {
		t.Fatalf("SaveMultiAppConfig() error = %v", err)
	}

	f, _, _, _ := cmdutil.TestFactory(t, nil)
	if err := profileRenameRun(f, "old", "app-old"); err != nil {
		t.Fatalf("profileRenameRun() error = %v", err)
	}

	saved, err := core.LoadMultiAppConfig()
	if err != nil {
		t.Fatalf("LoadMultiAppConfig() error = %v", err)
	}
	if saved.CurrentApp != "app-old" {
		t.Fatalf("CurrentApp = %q, want %q", saved.CurrentApp, "app-old")
	}
	if saved.PreviousApp != "app-old" {
		t.Fatalf("PreviousApp = %q, want %q", saved.PreviousApp, "app-old")
	}
	if saved.Apps[0].Name != "app-old" {
		t.Fatalf("Name = %q, want %q", saved.Apps[0].Name, "app-old")
	}
}

func TestProfileUseRun_ToggleBackUsesPreviousProfile(t *testing.T) {
	setupProfileConfigDir(t)
	multi := &core.MultiAppConfig{
		CurrentApp:  "default",
		PreviousApp: "target",
		Apps: []core.AppConfig{
			{Name: "default", AppId: "app-default", AppSecret: core.PlainSecret("secret-default"), Brand: core.BrandFeishu},
			{Name: "target", AppId: "app-target", AppSecret: core.PlainSecret("secret-target"), Brand: core.BrandLark},
		},
	}
	if err := core.SaveMultiAppConfig(multi); err != nil {
		t.Fatalf("SaveMultiAppConfig() error = %v", err)
	}

	f, _, _, _ := cmdutil.TestFactory(t, nil)
	if err := profileUseRun(f, "-"); err != nil {
		t.Fatalf("profileUseRun() error = %v", err)
	}

	saved, err := core.LoadMultiAppConfig()
	if err != nil {
		t.Fatalf("LoadMultiAppConfig() error = %v", err)
	}
	if saved.CurrentApp != "target" {
		t.Fatalf("CurrentApp = %q, want %q", saved.CurrentApp, "target")
	}
	if saved.PreviousApp != "default" {
		t.Fatalf("PreviousApp = %q, want %q", saved.PreviousApp, "default")
	}
}

func TestProfileListRun_OutputsProfiles(t *testing.T) {
	setupProfileConfigDir(t)
	multi := &core.MultiAppConfig{
		CurrentApp: "default",
		Apps: []core.AppConfig{
			{Name: "default", AppId: "app-default", AppSecret: core.PlainSecret("secret-default"), Brand: core.BrandFeishu},
			{Name: "target", AppId: "app-target", AppSecret: core.PlainSecret("secret-target"), Brand: core.BrandLark},
		},
	}
	if err := core.SaveMultiAppConfig(multi); err != nil {
		t.Fatalf("SaveMultiAppConfig() error = %v", err)
	}

	f, stdout, _, _ := cmdutil.TestFactory(t, nil)
	if err := profileListRun(f); err != nil {
		t.Fatalf("profileListRun() error = %v", err)
	}

	var got []profileListItem
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("Unmarshal() error = %v; output=%s", err, stdout.String())
	}
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2", len(got))
	}
	if got[0].Name != "default" || !got[0].Active {
		t.Fatalf("got[0] = %#v, want active default profile", got[0])
	}
	if got[1].Name != "target" || got[1].Active {
		t.Fatalf("got[1] = %#v, want inactive target profile", got[1])
	}
}

func TestProfileListRun_NotConfiguredReturnsEmptyList(t *testing.T) {
	setupProfileConfigDir(t)

	f, stdout, stderr, _ := cmdutil.TestFactory(t, nil)
	if err := profileListRun(f); err != nil {
		t.Fatalf("profileListRun() error = %v", err)
	}

	var got []profileListItem
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("Unmarshal() error = %v; output=%s", err, stdout.String())
	}
	if len(got) != 0 {
		t.Fatalf("len(got) = %d, want 0", len(got))
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestProfileRemoveRun_SaveFailureReturnsStructuredError(t *testing.T) {
	setupProfileConfigDir(t)
	multi := &core.MultiAppConfig{
		CurrentApp: "target",
		Apps: []core.AppConfig{
			{Name: "default", AppId: "app-default", AppSecret: core.PlainSecret("secret-default"), Brand: core.BrandFeishu},
			{Name: "target", AppId: "app-target", AppSecret: core.PlainSecret("secret-target"), Brand: core.BrandLark},
		},
	}
	if err := core.SaveMultiAppConfig(multi); err != nil {
		t.Fatalf("SaveMultiAppConfig() error = %v", err)
	}

	restoreFS := vfs.DefaultFS
	vfs.DefaultFS = &failRenameFS{err: errors.New("rename boom")}
	t.Cleanup(func() { vfs.DefaultFS = restoreFS })

	f, _, _, _ := cmdutil.TestFactory(t, nil)
	err := profileRemoveRun(f, "target")
	if err == nil {
		t.Fatal("expected save error")
	}
	assertInternalExitError(t, err, "failed to save config")
}

func TestProfileRenameRun_SaveFailureReturnsStructuredError(t *testing.T) {
	setupProfileConfigDir(t)
	multi := &core.MultiAppConfig{
		CurrentApp: "old",
		Apps: []core.AppConfig{{
			Name:      "old",
			AppId:     "app-old",
			AppSecret: core.PlainSecret("secret-old"),
			Brand:     core.BrandFeishu,
		}},
	}
	if err := core.SaveMultiAppConfig(multi); err != nil {
		t.Fatalf("SaveMultiAppConfig() error = %v", err)
	}

	restoreFS := vfs.DefaultFS
	vfs.DefaultFS = &failRenameFS{err: errors.New("rename boom")}
	t.Cleanup(func() { vfs.DefaultFS = restoreFS })

	f, _, _, _ := cmdutil.TestFactory(t, nil)
	err := profileRenameRun(f, "old", "new")
	if err == nil {
		t.Fatal("expected save error")
	}
	assertInternalExitError(t, err, "failed to save config")
}

func TestProfileUseRun_SaveFailureReturnsStructuredError(t *testing.T) {
	setupProfileConfigDir(t)
	multi := &core.MultiAppConfig{
		CurrentApp: "default",
		Apps: []core.AppConfig{
			{Name: "default", AppId: "app-default", AppSecret: core.PlainSecret("secret-default"), Brand: core.BrandFeishu},
			{Name: "target", AppId: "app-target", AppSecret: core.PlainSecret("secret-target"), Brand: core.BrandLark},
		},
	}
	if err := core.SaveMultiAppConfig(multi); err != nil {
		t.Fatalf("SaveMultiAppConfig() error = %v", err)
	}

	restoreFS := vfs.DefaultFS
	vfs.DefaultFS = &failRenameFS{err: errors.New("rename boom")}
	t.Cleanup(func() { vfs.DefaultFS = restoreFS })

	f, _, _, _ := cmdutil.TestFactory(t, nil)
	err := profileUseRun(f, "target")
	if err == nil {
		t.Fatal("expected save error")
	}
	assertInternalExitError(t, err, "failed to save config")
}

func assertInternalExitError(t *testing.T, err error, wantMsg string) {
	t.Helper()

	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("error type = %T, want *output.ExitError; err=%v", err, err)
	}
	if exitErr.Code != output.ExitInternal {
		t.Fatalf("exit code = %d, want %d", exitErr.Code, output.ExitInternal)
	}
	if exitErr.Detail == nil || exitErr.Detail.Type != "internal" {
		t.Fatalf("detail = %#v, want internal detail", exitErr.Detail)
	}
	if !strings.Contains(exitErr.Detail.Message, wantMsg) {
		t.Fatalf("message = %q, want contains %q", exitErr.Detail.Message, wantMsg)
	}
}
