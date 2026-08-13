// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build !extended

package runtimebootstrap

import (
	"errors"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/build"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/envvars"
	"github.com/larksuite/cli/internal/vfs"
)

type lstatErrorFS struct {
	vfs.FS
	err error
}

func (f lstatErrorFS) Lstat(string) (fs.FileInfo, error) {
	return nil, f.err
}

func useStandardDevConfigPath(t *testing.T, configDir string) string {
	t.Helper()
	for _, name := range []string{
		"OPENCLAW_CLI",
		"OPENCLAW_HOME",
		"OPENCLAW_STATE_DIR",
		"OPENCLAW_CONFIG_PATH",
		"OPENCLAW_SERVICE_MARKER",
		"OPENCLAW_SERVICE_VERSION",
		"OPENCLAW_GATEWAY_PORT",
		"OPENCLAW_SHELL",
		"HERMES_HOME",
		"HERMES_QUIET",
		"HERMES_EXEC_ASK",
		"HERMES_GATEWAY_TOKEN",
		"HERMES_SESSION_KEY",
		"LARK_CHANNEL",
	} {
		t.Setenv(name, "")
	}
	core.SetCurrentWorkspace(core.WorkspaceLocal)
	originalVersion := build.Version
	build.Version = "DEV"
	t.Cleanup(func() { build.Version = originalVersion })
	systemPath := filepath.Join(configDir, "external-credential.json")
	t.Setenv(envvars.CliExternalCredentialConfig, systemPath)
	return systemPath
}

func TestStandardResolveCapturesOrdinaryProfile(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", configDir)
	useStandardDevConfigPath(t, configDir)
	if err := vfs.WriteFile(core.GetConfigPath(), []byte(`{
	  "currentApp": "default",
	  "apps": [{
	    "name": "default",
	    "appId": "cli_standard",
	    "appSecret": "standard-secret",
	    "brand": "feishu",
	    "users": []
	  }]
	}`), 0o600); err != nil {
		t.Fatal(err)
	}

	result := Resolve("")
	if err := result.Plan.StartupError(); err != nil {
		t.Fatalf("Resolve() startup error = %v", err)
	}
	if result.ProfileConfig == nil {
		t.Fatal("Resolve() did not capture the ordinary Profile")
	}
	app := result.ProfileConfig.CurrentAppConfig("")
	if app == nil || app.AppId != "cli_standard" {
		t.Fatalf("captured app = %#v, want cli_standard", app)
	}
	if !result.Plan.AllowsRemoteMetadata() {
		t.Fatal("ordinary Standard runtime unexpectedly disabled remote metadata")
	}
}

func TestStandardResolvePreservesProviderFallbackForUnreadableProfile(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", configDir)
	useStandardDevConfigPath(t, configDir)
	if err := vfs.MkdirAll(core.GetConfigPath(), 0o700); err != nil {
		t.Fatal(err)
	}

	result := Resolve("")
	if err := result.Plan.StartupError(); err != nil {
		t.Fatalf("Resolve() startup error = %v", err)
	}
	if result.ProfileConfig != nil {
		t.Fatalf("ProfileConfig = %#v, want nil provider fallback snapshot", result.ProfileConfig)
	}
}

func TestStandardResolveRejectsSystemConfigWithoutParsingItsProtocol(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", configDir)
	systemPath := useStandardDevConfigPath(t, configDir)
	if err := vfs.WriteFile(systemPath, []byte(`not-json-and-not-a-mode`), 0o600); err != nil {
		t.Fatal(err)
	}

	result := Resolve("")
	startupErr := result.Plan.StartupError()
	var validationErr *errs.ValidationError
	if !errors.As(startupErr, &validationErr) ||
		validationErr.Subtype != errs.SubtypeFailedPrecondition {
		t.Fatalf("startup error = %T %v, want validation/failed_precondition", startupErr, startupErr)
	}
	if result.Plan.AllowsRemoteMetadata() {
		t.Fatal("system configuration sentinel must block remote metadata before command construction")
	}
	if result.ProfileConfig != nil {
		t.Fatalf("ProfileConfig = %#v, want no Profile read after sentinel", result.ProfileConfig)
	}
}

func TestStandardResolveReportsSystemConfigInspectionFailure(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", configDir)
	systemPath := useStandardDevConfigPath(t, configDir)

	permissionErr := &fs.PathError{Op: "lstat", Path: systemPath, Err: fs.ErrPermission}
	previousFS := vfs.DefaultFS
	vfs.DefaultFS = lstatErrorFS{FS: previousFS, err: permissionErr}
	t.Cleanup(func() { vfs.DefaultFS = previousFS })

	result := Resolve("")
	startupErr := result.Plan.StartupError()
	var configErr *errs.ConfigError
	if !errors.As(startupErr, &configErr) ||
		configErr.Subtype != errs.SubtypeInvalidConfig {
		t.Fatalf("startup error = %T %v, want config/invalid_config", startupErr, startupErr)
	}
	if !errors.Is(startupErr, fs.ErrPermission) {
		t.Fatalf("startup error = %v, want preserved permission cause", startupErr)
	}
	if !strings.Contains(configErr.Message, "cannot inspect system external credential configuration") {
		t.Fatalf("message = %q, want inspection failure", configErr.Message)
	}
	if strings.Contains(configErr.Message, "requires the lark-cli Extended edition") {
		t.Fatalf("message = %q, must not misreport an edition mismatch", configErr.Message)
	}
	if configErr.Hint != "ask the system administrator to restore access to the configuration path and its parent directories" {
		t.Fatalf("hint = %q, want actionable access repair guidance", configErr.Hint)
	}
	if result.Plan.AllowsRemoteMetadata() {
		t.Fatal("system configuration inspection failure must block remote metadata")
	}
	if result.ProfileConfig != nil {
		t.Fatalf("ProfileConfig = %#v, want no Profile read after inspection failure", result.ProfileConfig)
	}
}
