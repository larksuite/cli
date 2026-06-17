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

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/i18n"
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
	var internalErr *errs.InternalError
	if !errors.As(err, &internalErr) {
		t.Fatalf("error type = %T, want *errs.InternalError; err=%v", err, err)
	}
	if internalErr.Subtype != errs.SubtypeFileIO {
		t.Fatalf("subtype = %q, want %q", internalErr.Subtype, errs.SubtypeFileIO)
	}
	if code := output.ExitCodeOf(err); code != output.ExitInternal {
		t.Fatalf("exit code = %d, want %d (ExitInternal)", code, output.ExitInternal)
	}
}

// TestProfileAddRun_Lang covers the unified --lang contract on profile add:
// short codes and Feishu locales both canonicalize to the same stored locale,
// empty stores no preference, and an unrecognized value errors.
func TestProfileAddRun_Lang(t *testing.T) {
	t.Run("short and locale canonicalize and persist alike", func(t *testing.T) {
		for _, in := range []string{"ja", "ja_jp"} {
			setupProfileConfigDir(t)
			f, _, _, _ := cmdutil.TestFactory(t, nil)
			f.IOStreams.In = strings.NewReader("secret\n")
			if err := profileAddRun(f, "p", "app-p", true, "feishu", in, false); err != nil {
				t.Fatalf("--lang %q: profileAddRun() error = %v", in, err)
			}
			saved, err := core.LoadMultiAppConfig()
			if err != nil {
				t.Fatalf("LoadMultiAppConfig() error = %v", err)
			}
			if app := saved.FindApp("p"); app == nil || app.Lang != i18n.LangJaJP {
				t.Errorf("--lang %q: stored Lang = %v, want %q", in, app, i18n.LangJaJP)
			}
		}
	})

	t.Run("empty stores no preference", func(t *testing.T) {
		setupProfileConfigDir(t)
		f, _, _, _ := cmdutil.TestFactory(t, nil)
		f.IOStreams.In = strings.NewReader("secret\n")
		if err := profileAddRun(f, "p", "app-p", true, "feishu", "", false); err != nil {
			t.Fatalf("profileAddRun() error = %v", err)
		}
		saved, _ := core.LoadMultiAppConfig()
		if app := saved.FindApp("p"); app == nil || app.Lang != "" {
			t.Errorf("stored Lang = %v, want \"\" (unset)", app)
		}
	})

	t.Run("invalid lang errors", func(t *testing.T) {
		setupProfileConfigDir(t)
		f, _, _, _ := cmdutil.TestFactory(t, nil)
		f.IOStreams.In = strings.NewReader("secret\n")
		err := profileAddRun(f, "p", "app-p", true, "feishu", "ZH", false)
		if err == nil {
			t.Fatal("expected validation error for --lang ZH, got nil")
		}
		var valErr *errs.ValidationError
		if !errors.As(err, &valErr) || output.ExitCodeOf(err) != output.ExitValidation {
			t.Fatalf("expected typed validation error with ExitValidation, got %T: %v", err, err)
		}
	})
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

	var internalErr *errs.InternalError
	if !errors.As(err, &internalErr) {
		t.Fatalf("error type = %T, want *errs.InternalError; err=%v", err, err)
	}
	if internalErr.Subtype != errs.SubtypeStorage {
		t.Fatalf("subtype = %q, want %q", internalErr.Subtype, errs.SubtypeStorage)
	}
	if internalErr.Cause == nil {
		t.Fatalf("cause = nil, want wrapped underlying error")
	}
	if !strings.Contains(internalErr.Message, wantMsg) {
		t.Fatalf("message = %q, want contains %q", internalErr.Message, wantMsg)
	}
	if code := output.ExitCodeOf(err); code != output.ExitInternal {
		t.Fatalf("exit code = %d, want %d (ExitInternal)", code, output.ExitInternal)
	}
}

// assertValidationError asserts err is a typed *errs.ValidationError with the
// given subtype, message fragment, and exit code 2.
func assertValidationError(t *testing.T, err error, wantSubtype errs.Subtype, wantMsg string) *errs.ValidationError {
	t.Helper()

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var valErr *errs.ValidationError
	if !errors.As(err, &valErr) {
		t.Fatalf("error type = %T, want *errs.ValidationError; err=%v", err, err)
	}
	if valErr.Subtype != wantSubtype {
		t.Fatalf("subtype = %q, want %q", valErr.Subtype, wantSubtype)
	}
	if !strings.Contains(valErr.Message, wantMsg) {
		t.Fatalf("message = %q, want contains %q", valErr.Message, wantMsg)
	}
	if code := output.ExitCodeOf(err); code != output.ExitValidation {
		t.Fatalf("exit code = %d, want %d (ExitValidation)", code, output.ExitValidation)
	}
	return valErr
}

func saveTwoProfiles(t *testing.T) {
	t.Helper()
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
}

func TestProfileAddRun_ValidationErrors(t *testing.T) {
	t.Run("invalid profile name", func(t *testing.T) {
		setupProfileConfigDir(t)
		f, _, _, _ := cmdutil.TestFactory(t, nil)
		f.IOStreams.In = strings.NewReader("secret\n")
		err := profileAddRun(f, "bad name!", "app-x", true, "feishu", "", false)
		valErr := assertValidationError(t, err, errs.SubtypeInvalidArgument, "")
		if valErr.Param != "--name" {
			t.Fatalf("param = %q, want %q", valErr.Param, "--name")
		}
		if valErr.Cause == nil {
			t.Fatal("cause = nil, want wrapped validation error")
		}
	})

	t.Run("missing app-secret-stdin flag", func(t *testing.T) {
		setupProfileConfigDir(t)
		f, _, _, _ := cmdutil.TestFactory(t, nil)
		err := profileAddRun(f, "p", "app-x", false, "feishu", "", false)
		valErr := assertValidationError(t, err, errs.SubtypeInvalidArgument, "app secret must be provided via stdin")
		if valErr.Param != "--app-secret-stdin" {
			t.Fatalf("param = %q, want %q", valErr.Param, "--app-secret-stdin")
		}
		if valErr.Hint == "" {
			t.Fatal("hint is empty, want actionable hint")
		}
	})

	t.Run("empty stdin", func(t *testing.T) {
		setupProfileConfigDir(t)
		f, _, _, _ := cmdutil.TestFactory(t, nil)
		f.IOStreams.In = strings.NewReader("")
		err := profileAddRun(f, "p", "app-x", true, "feishu", "", false)
		valErr := assertValidationError(t, err, errs.SubtypeInvalidArgument, "stdin is empty")
		if valErr.Param != "--app-secret-stdin" {
			t.Fatalf("param = %q, want %q", valErr.Param, "--app-secret-stdin")
		}
	})

	t.Run("blank secret on stdin", func(t *testing.T) {
		setupProfileConfigDir(t)
		f, _, _, _ := cmdutil.TestFactory(t, nil)
		f.IOStreams.In = strings.NewReader("   \n")
		err := profileAddRun(f, "p", "app-x", true, "feishu", "", false)
		assertValidationError(t, err, errs.SubtypeInvalidArgument, "app secret read from stdin is empty")
	})

	t.Run("duplicate profile name", func(t *testing.T) {
		setupProfileConfigDir(t)
		saveTwoProfiles(t)
		f, _, _, _ := cmdutil.TestFactory(t, nil)
		f.IOStreams.In = strings.NewReader("secret\n")
		err := profileAddRun(f, "default", "app-new", true, "feishu", "", false)
		valErr := assertValidationError(t, err, errs.SubtypeFailedPrecondition, `profile "default" already exists`)
		if valErr.Param != "--name" {
			t.Fatalf("param = %q, want %q", valErr.Param, "--name")
		}
	})

	t.Run("duplicate app-id", func(t *testing.T) {
		setupProfileConfigDir(t)
		saveTwoProfiles(t)
		f, _, _, _ := cmdutil.TestFactory(t, nil)
		f.IOStreams.In = strings.NewReader("secret\n")
		err := profileAddRun(f, "fresh", "app-default", true, "feishu", "", false)
		valErr := assertValidationError(t, err, errs.SubtypeFailedPrecondition, "already used by profile")
		if valErr.Param != "--app-id" {
			t.Fatalf("param = %q, want %q", valErr.Param, "--app-id")
		}
	})
}

func TestProfileUseRun_ValidationErrors(t *testing.T) {
	t.Run("no previous profile for toggle", func(t *testing.T) {
		setupProfileConfigDir(t)
		saveTwoProfiles(t)
		f, _, _, _ := cmdutil.TestFactory(t, nil)
		err := profileUseRun(f, "-")
		valErr := assertValidationError(t, err, errs.SubtypeFailedPrecondition, "no previous profile to switch back to")
		if valErr.Hint == "" {
			t.Fatal("hint is empty, want actionable hint")
		}
	})

	t.Run("profile not found", func(t *testing.T) {
		setupProfileConfigDir(t)
		saveTwoProfiles(t)
		f, _, _, _ := cmdutil.TestFactory(t, nil)
		err := profileUseRun(f, "ghost")
		assertValidationError(t, err, errs.SubtypeInvalidArgument, `profile "ghost" not found`)
	})
}

func TestProfileRenameRun_ValidationErrors(t *testing.T) {
	t.Run("invalid new name", func(t *testing.T) {
		setupProfileConfigDir(t)
		saveTwoProfiles(t)
		f, _, _, _ := cmdutil.TestFactory(t, nil)
		err := profileRenameRun(f, "default", "bad name!")
		valErr := assertValidationError(t, err, errs.SubtypeInvalidArgument, "")
		if valErr.Cause == nil {
			t.Fatal("cause = nil, want wrapped validation error")
		}
	})

	t.Run("old profile not found", func(t *testing.T) {
		setupProfileConfigDir(t)
		saveTwoProfiles(t)
		f, _, _, _ := cmdutil.TestFactory(t, nil)
		err := profileRenameRun(f, "ghost", "fresh")
		assertValidationError(t, err, errs.SubtypeInvalidArgument, `profile "ghost" not found`)
	})

	t.Run("new name already exists", func(t *testing.T) {
		setupProfileConfigDir(t)
		saveTwoProfiles(t)
		f, _, _, _ := cmdutil.TestFactory(t, nil)
		err := profileRenameRun(f, "default", "target")
		valErr := assertValidationError(t, err, errs.SubtypeFailedPrecondition, `profile "target" already exists`)
		if valErr.Hint == "" {
			t.Fatal("hint is empty, want actionable hint")
		}
	})
}

func TestProfileRemoveRun_ValidationErrors(t *testing.T) {
	t.Run("profile not found", func(t *testing.T) {
		setupProfileConfigDir(t)
		saveTwoProfiles(t)
		f, _, _, _ := cmdutil.TestFactory(t, nil)
		err := profileRemoveRun(f, "ghost")
		assertValidationError(t, err, errs.SubtypeInvalidArgument, `profile "ghost" not found`)
	})

	t.Run("cannot remove the only profile", func(t *testing.T) {
		setupProfileConfigDir(t)
		multi := &core.MultiAppConfig{
			CurrentApp: "solo",
			Apps: []core.AppConfig{
				{Name: "solo", AppId: "app-solo", AppSecret: core.PlainSecret("secret-solo"), Brand: core.BrandFeishu},
			},
		}
		if err := core.SaveMultiAppConfig(multi); err != nil {
			t.Fatalf("SaveMultiAppConfig() error = %v", err)
		}
		f, _, _, _ := cmdutil.TestFactory(t, nil)
		err := profileRemoveRun(f, "solo")
		valErr := assertValidationError(t, err, errs.SubtypeFailedPrecondition, "cannot remove the only profile")
		if valErr.Hint == "" {
			t.Fatal("hint is empty, want actionable hint")
		}
	})
}

func TestProfileListRun_InvalidConfigReturnsValidationError(t *testing.T) {
	dir := setupProfileConfigDir(t)
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte("{invalid json"), 0600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	f, _, _, _ := cmdutil.TestFactory(t, nil)
	err := profileListRun(f)
	valErr := assertValidationError(t, err, errs.SubtypeFailedPrecondition, "failed to load config")
	if valErr.Cause == nil {
		t.Fatal("cause = nil, want wrapped load error")
	}
}
