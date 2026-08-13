// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"context"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/envvars"
)

func TestStartupProfileSnapshotUsesDetectedWorkspace(t *testing.T) {
	previousWorkspace := core.CurrentWorkspace()
	t.Cleanup(func() { core.SetCurrentWorkspace(previousWorkspace) })

	tests := []struct {
		name          string
		workspace     core.Workspace
		signalName    string
		signalValue   string
		expectedAppID string
		expectedBrand core.LarkBrand
	}{
		{
			name:          "local",
			workspace:     core.WorkspaceLocal,
			expectedAppID: "cli_local",
			expectedBrand: core.BrandFeishu,
		},
		{
			name:          "openclaw",
			workspace:     core.WorkspaceOpenClaw,
			signalName:    "OPENCLAW_CLI",
			signalValue:   "1",
			expectedAppID: "cli_openclaw",
			expectedBrand: core.BrandLark,
		},
		{
			name:          "hermes",
			workspace:     core.WorkspaceHermes,
			signalName:    "HERMES_HOME",
			signalValue:   "/managed/hermes",
			expectedAppID: "cli_hermes",
			expectedBrand: core.BrandLark,
		},
		{
			name:          "lark_channel",
			workspace:     core.WorkspaceLarkChannel,
			signalName:    "LARK_CHANNEL",
			signalValue:   "1",
			expectedAppID: "cli_lark_channel",
			expectedBrand: core.BrandLark,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearWorkspaceSignals(t)
			clearCredentialSignals(t)
			configRoot := t.TempDir()
			t.Setenv("LARKSUITE_CLI_CONFIG_DIR", configRoot)
			t.Setenv(envvars.CliExternalCredentialConfig,
				filepath.Join(configRoot, "missing-external-credential.json"))
			t.Setenv("LARKSUITE_CLI_REMOTE_META", "off")
			if tt.signalName != "" {
				t.Setenv(tt.signalName, tt.signalValue)
			}

			writeWorkspaceProfile(t, core.WorkspaceLocal, "local", "cli_local", core.BrandFeishu)
			if !tt.workspace.IsLocal() {
				writeWorkspaceProfile(t, tt.workspace, tt.name, tt.expectedAppID, tt.expectedBrand)
			}

			// Execute resolves the registry brand before entering
			// buildInternal. Pin that ordering independently.
			core.SetCurrentWorkspace(core.WorkspaceLocal)
			if got := selectInvocationWorkspace(); got != tt.workspace {
				t.Fatalf("selected workspace = %q, want %q", got, tt.workspace)
			}
			if got := ResolveStartupBrand(""); got != tt.expectedBrand {
				t.Fatalf("startup brand = %q, want %q", got, tt.expectedBrand)
			}

			// Build/buildInternal is also a public construction path. Reset the
			// process state to local so the test proves it establishes the
			// workspace before SelectProfile captures the immutable snapshot.
			core.SetCurrentWorkspace(core.WorkspaceLocal)
			factory, _, _ := buildInternal(
				context.Background(),
				cmdutil.InvocationContext{},
				WithIO(strings.NewReader(""), io.Discard, io.Discard),
				WithoutPlugins(),
				WithoutServiceCommands(),
			)
			if got := core.CurrentWorkspace(); got != tt.workspace {
				t.Fatalf("workspace after build = %q, want %q", got, tt.workspace)
			}
			config, err := factory.Config()
			if err != nil {
				t.Fatalf("Factory.Config() error = %v", err)
			}
			if config.AppID != tt.expectedAppID || config.Brand != tt.expectedBrand {
				t.Fatalf("resolved config = app %q (%s), want app %q (%s)",
					config.AppID, config.Brand, tt.expectedAppID, tt.expectedBrand)
			}
		})
	}
}

func clearWorkspaceSignals(t *testing.T) {
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
}

func clearCredentialSignals(t *testing.T) {
	t.Helper()
	for _, name := range []string{
		envvars.CliAppID,
		envvars.CliAppSecret,
		envvars.CliBrand,
		envvars.CliUserAccessToken,
		envvars.CliTenantAccessToken,
		envvars.CliDefaultAs,
		envvars.CliStrictMode,
	} {
		t.Setenv(name, "")
	}
}

func writeWorkspaceProfile(
	t *testing.T,
	workspace core.Workspace,
	name string,
	appID string,
	brand core.LarkBrand,
) {
	t.Helper()
	previous := core.CurrentWorkspace()
	core.SetCurrentWorkspace(workspace)
	defer core.SetCurrentWorkspace(previous)
	if err := core.SaveMultiAppConfig(&core.MultiAppConfig{
		CurrentApp: name,
		Apps: []core.AppConfig{{
			Name:      name,
			AppId:     appID,
			AppSecret: core.PlainSecret("test-secret-" + name),
			Brand:     brand,
			Users:     []core.AppUser{},
		}},
	}); err != nil {
		t.Fatalf("save %s workspace profile: %v", workspace.Display(), err)
	}
}
