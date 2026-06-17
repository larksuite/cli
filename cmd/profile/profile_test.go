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

func saveProfileTestConfig(t *testing.T) {
	t.Helper()
	if err := core.SaveMultiAppConfig(&core.MultiAppConfig{
		CurrentApp: "bytedance",
		Apps: []core.AppConfig{
			{Name: "bytedance", AppId: "app_bytedance", AppSecret: core.PlainSecret("secret"), Brand: core.BrandFeishu},
			{Name: "team-prod", AppId: "app_team", AppSecret: core.PlainSecret("secret"), Brand: core.BrandLark},
			{Name: "lark-boe", AppId: "app_lark_boe", AppSecret: core.PlainSecret("secret"), Brand: core.BrandLark},
		},
	}); err != nil {
		t.Fatalf("SaveMultiAppConfig() error = %v", err)
	}
}

func writeProfileProjectConfig(t *testing.T, dir, body string) string {
	t.Helper()
	path := core.ProjectConfigPath(dir)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatalf("MkdirAll(project config dir): %v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatalf("WriteFile(project config): %v", err)
	}
	return path
}

func TestProfileBindRun_WritesProjectConfigAtGitRoot(t *testing.T) {
	setupProfileConfigDir(t)
	saveProfileTestConfig(t)

	repo := t.TempDir()
	if err := os.Mkdir(filepath.Join(repo, ".git"), 0700); err != nil {
		t.Fatalf("Mkdir(.git): %v", err)
	}
	sub := filepath.Join(repo, "sub")
	if err := os.MkdirAll(sub, 0700); err != nil {
		t.Fatalf("MkdirAll(sub): %v", err)
	}
	cmdutil.TestChdir(t, sub)

	f, _, _, _ := cmdutil.TestFactory(t, nil)
	if err := profileBindRun(f, "team-prod"); err != nil {
		t.Fatalf("profileBindRun() error = %v", err)
	}

	data, err := os.ReadFile(core.ProjectConfigPath(repo))
	if err != nil {
		t.Fatalf("ReadFile(project config): %v", err)
	}
	var cfg core.ProjectConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("Unmarshal(project config): %v", err)
	}
	if cfg.Profile != "team-prod" {
		t.Fatalf("project profile = %q, want team-prod", cfg.Profile)
	}
}

func TestProfileUnbindRun_RemovesNearestProjectConfig(t *testing.T) {
	setupProfileConfigDir(t)
	repo := t.TempDir()
	if err := os.Mkdir(filepath.Join(repo, ".git"), 0700); err != nil {
		t.Fatalf("Mkdir(.git): %v", err)
	}
	path := writeProfileProjectConfig(t, repo, `{"profile":"bytedance"}`)
	sub := filepath.Join(repo, "sub")
	if err := os.MkdirAll(sub, 0700); err != nil {
		t.Fatalf("MkdirAll(sub): %v", err)
	}
	cmdutil.TestChdir(t, sub)

	f, _, _, _ := cmdutil.TestFactory(t, nil)
	if err := profileUnbindRun(f); err != nil {
		t.Fatalf("profileUnbindRun() error = %v", err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("project config still exists, Stat err = %v", err)
	}
}

func TestProfileBindRun_InvalidProfileReturnsTypedError(t *testing.T) {
	setupProfileConfigDir(t)
	f, _, _, _ := cmdutil.TestFactory(t, nil)
	err := profileBindRun(f, "")
	if err == nil {
		t.Fatal("profileBindRun() error = nil, want validation error")
	}
	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("ProblemOf() ok = false, err = %T", err)
	}
	if problem.Category != errs.CategoryValidation || problem.Subtype != errs.SubtypeInvalidArgument {
		t.Fatalf("problem = %#v", problem)
	}
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("error type = %T, want *errs.ValidationError", err)
	}
	if validationErr.Param != "<profile>" {
		t.Fatalf("Param = %q, want <profile>", validationErr.Param)
	}
	if errors.Unwrap(err) == nil {
		t.Fatal("validation error cause is nil")
	}
}

func TestProfileCurrentRun_CLIProfileMissingReturnsValidationError(t *testing.T) {
	setupProfileConfigDir(t)
	saveProfileTestConfig(t)

	f, _, _, _ := cmdutil.TestFactory(t, nil)
	f.Invocation = cmdutil.InvocationContext{
		Profile:       "missing",
		ProfileSource: core.ProfileSourceCLI,
	}
	err := profileCurrentRun(f)
	if err == nil {
		t.Fatal("profileCurrentRun() error = nil, want validation error")
	}
	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("ProblemOf() ok = false, err = %T", err)
	}
	if problem.Category != errs.CategoryValidation || problem.Subtype != errs.SubtypeInvalidArgument {
		t.Fatalf("problem = %#v", problem)
	}
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("error type = %T, want *errs.ValidationError", err)
	}
	if validationErr.Param != "--profile" {
		t.Fatalf("Param = %q, want --profile", validationErr.Param)
	}
}

func TestProfileBindRun_MissingProfileReturnsTypedError(t *testing.T) {
	setupProfileConfigDir(t)
	saveProfileTestConfig(t)
	f, _, _, _ := cmdutil.TestFactory(t, nil)
	err := profileBindRun(f, "missing")
	if err == nil {
		t.Fatal("profileBindRun() error = nil, want validation error")
	}
	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("ProblemOf() ok = false, err = %T", err)
	}
	if problem.Category != errs.CategoryValidation || problem.Subtype != errs.SubtypeInvalidArgument {
		t.Fatalf("problem = %#v", problem)
	}
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("error type = %T, want *errs.ValidationError", err)
	}
	if validationErr.Param != "<profile>" {
		t.Fatalf("Param = %q, want <profile>", validationErr.Param)
	}
}

func TestProfileCurrentRun_ProjectSource(t *testing.T) {
	configDir := setupProfileConfigDir(t)
	saveProfileTestConfig(t)
	projectPath := core.ProjectConfigPath(t.TempDir())

	f, stdout, _, _ := cmdutil.TestFactory(t, nil)
	f.Invocation = cmdutil.InvocationContext{
		Profile:           "team-prod",
		ProfileSource:     core.ProfileSourceProject,
		ProfileConfigPath: projectPath,
	}
	if err := profileCurrentRun(f); err != nil {
		t.Fatalf("profileCurrentRun() error = %v", err)
	}

	var got currentProfileOutput
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("Unmarshal(stdout): %v\nstdout=%s", err, stdout.String())
	}
	if got.Profile != "team-prod" || got.Source != "project" || got.Config != projectPath || got.AppID != "app_team" {
		t.Fatalf("current profile = %#v", got)
	}
	if got.Config == filepath.Join(configDir, "config.json") {
		t.Fatalf("project source should report project config, got global path %q", got.Config)
	}
}

func TestProfileCurrentRun_CLISourceLeavesConfigEmpty(t *testing.T) {
	setupProfileConfigDir(t)
	saveProfileTestConfig(t)

	f, stdout, _, _ := cmdutil.TestFactory(t, nil)
	f.Invocation = cmdutil.InvocationContext{
		Profile:       "bytedance",
		ProfileSource: core.ProfileSourceCLI,
	}
	if err := profileCurrentRun(f); err != nil {
		t.Fatalf("profileCurrentRun() error = %v", err)
	}

	var got currentProfileOutput
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("Unmarshal(stdout): %v\nstdout=%s", err, stdout.String())
	}
	if got.Profile != "bytedance" || got.Source != "cli" || got.Config != "" || got.AppID != "app_bytedance" {
		t.Fatalf("current profile = %#v", got)
	}
}

func TestProfileCurrentRun_ProjectProfileMissingReturnsConfigError(t *testing.T) {
	setupProfileConfigDir(t)
	saveProfileTestConfig(t)
	projectPath := core.ProjectConfigPath(t.TempDir())

	f, _, _, _ := cmdutil.TestFactory(t, nil)
	f.Invocation = cmdutil.InvocationContext{
		Profile:           "missing",
		ProfileSource:     core.ProfileSourceProject,
		ProfileConfigPath: projectPath,
	}
	err := profileCurrentRun(f)
	if err == nil {
		t.Fatal("profileCurrentRun() error = nil, want project profile not found")
	}
	var cfgErr *core.ConfigError
	if !errors.As(err, &cfgErr) {
		t.Fatalf("error type = %T, want *core.ConfigError", err)
	}
	wantMsg := `profile "missing" is configured by project but not found`
	wantHint := "project config: " + projectPath + "; run: lark-cli profile list; available profiles: bytedance, team-prod, lark-boe"
	if cfgErr.Code != 3 || cfgErr.Type != "config" || cfgErr.Message != wantMsg || cfgErr.Hint != wantHint {
		t.Fatalf("ConfigError = %#v", cfgErr)
	}
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
		exitErr, ok := err.(*output.ExitError)
		if !ok || exitErr.Code != output.ExitValidation {
			t.Fatalf("expected ExitValidation, got %T: %v", err, err)
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
