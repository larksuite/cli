// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package core

import (
	"path/filepath"
	"testing"
)

func TestDetectWorkspaceFromEnv(t *testing.T) {
	tests := []struct {
		name   string
		env    map[string]string
		expect Workspace
	}{
		{
			name:   "no agent env → local",
			env:    map[string]string{},
			expect: WorkspaceLocal,
		},
		{
			name:   "OPENCLAW_CLI=1 → openclaw",
			env:    map[string]string{"OPENCLAW_CLI": "1"},
			expect: WorkspaceOpenClaw,
		},
		{
			name:   "OPENCLAW_CLI=true → local (strict ==1 check)",
			env:    map[string]string{"OPENCLAW_CLI": "true"},
			expect: WorkspaceLocal,
		},
		{
			name:   "OPENCLAW_CLI=yes → local",
			env:    map[string]string{"OPENCLAW_CLI": "yes"},
			expect: WorkspaceLocal,
		},
		{
			name:   "OPENCLAW_CLI=0 → local",
			env:    map[string]string{"OPENCLAW_CLI": "0"},
			expect: WorkspaceLocal,
		},
		{
			name:   "OPENCLAW_CLI empty → local",
			env:    map[string]string{"OPENCLAW_CLI": ""},
			expect: WorkspaceLocal,
		},
		{
			name:   "OPENCLAW_CLI=1 with trailing space → local (strict)",
			env:    map[string]string{"OPENCLAW_CLI": "1 "},
			expect: WorkspaceLocal,
		},
		{
			name:   "generic FEISHU_APP_ID + SECRET → local (not a Hermes signal)",
			env:    map[string]string{"FEISHU_APP_ID": "cli_abc", "FEISHU_APP_SECRET": "xxx"},
			expect: WorkspaceLocal,
		},
		{
			name:   "HERMES_HOME set → hermes",
			env:    map[string]string{"HERMES_HOME": "/Users/me/.hermes"},
			expect: WorkspaceHermes,
		},
		{
			name:   "HERMES_QUIET=1 → hermes (set by gateway)",
			env:    map[string]string{"HERMES_QUIET": "1"},
			expect: WorkspaceHermes,
		},
		{
			name:   "HERMES_EXEC_ASK=1 → hermes",
			env:    map[string]string{"HERMES_EXEC_ASK": "1"},
			expect: WorkspaceHermes,
		},
		{
			name:   "HERMES_GATEWAY_TOKEN set → hermes",
			env:    map[string]string{"HERMES_GATEWAY_TOKEN": "69ce6b...6065"},
			expect: WorkspaceHermes,
		},
		{
			name:   "HERMES_SESSION_KEY set → hermes",
			env:    map[string]string{"HERMES_SESSION_KEY": "agent:main:feishu:dm:oc_xxx"},
			expect: WorkspaceHermes,
		},
		{
			name:   "HERMES_QUIET=0 alone → local (strict ==1 check)",
			env:    map[string]string{"HERMES_QUIET": "0"},
			expect: WorkspaceLocal,
		},
		{
			name:   "OPENCLAW_CLI=1 + HERMES_HOME both set → openclaw wins (priority)",
			env:    map[string]string{"OPENCLAW_CLI": "1", "HERMES_HOME": "/Users/me/.hermes"},
			expect: WorkspaceOpenClaw,
		},
		{
			name:   "FEISHU_APP_ID + HERMES_HOME → hermes (HERMES_ signals suffice)",
			env:    map[string]string{"FEISHU_APP_ID": "cli_abc", "FEISHU_APP_SECRET": "xxx", "HERMES_HOME": "/Users/me/.hermes"},
			expect: WorkspaceHermes,
		},
		{
			name:   "OPENCLAW_HOME set → openclaw (older OpenClaw builds without subprocess marker)",
			env:    map[string]string{"OPENCLAW_HOME": "/Users/me/.openclaw"},
			expect: WorkspaceOpenClaw,
		},
		{
			name:   "OPENCLAW_STATE_DIR set → openclaw",
			env:    map[string]string{"OPENCLAW_STATE_DIR": "/srv/openclaw/state"},
			expect: WorkspaceOpenClaw,
		},
		{
			name:   "OPENCLAW_CONFIG_PATH set → openclaw",
			env:    map[string]string{"OPENCLAW_CONFIG_PATH": "/etc/openclaw/openclaw.json"},
			expect: WorkspaceOpenClaw,
		},
		{
			name:   "OPENCLAW_HOME + FEISHU both set → openclaw wins (priority)",
			env:    map[string]string{"OPENCLAW_HOME": "/Users/me/.openclaw", "FEISHU_APP_ID": "cli_abc", "FEISHU_APP_SECRET": "xxx"},
			expect: WorkspaceOpenClaw,
		},
		{
			name:   "LARKSUITE_CLI_APP_ID does not affect workspace",
			env:    map[string]string{"LARKSUITE_CLI_APP_ID": "cli_local", "LARKSUITE_CLI_APP_SECRET": "local_secret"},
			expect: WorkspaceLocal,
		},
		{
			name:   "LARK_CHANNEL=1 → lark-channel",
			env:    map[string]string{"LARK_CHANNEL": "1"},
			expect: WorkspaceLarkChannel,
		},
		{
			name:   "LARK_CHANNEL=true → local (strict ==1 check)",
			env:    map[string]string{"LARK_CHANNEL": "true"},
			expect: WorkspaceLocal,
		},
		{
			name:   "LARK_CHANNEL=0 → local",
			env:    map[string]string{"LARK_CHANNEL": "0"},
			expect: WorkspaceLocal,
		},
		{
			name:   "OPENCLAW_CLI=1 + LARK_CHANNEL=1 → openclaw wins (priority)",
			env:    map[string]string{"OPENCLAW_CLI": "1", "LARK_CHANNEL": "1"},
			expect: WorkspaceOpenClaw,
		},
		{
			name:   "HERMES_HOME + LARK_CHANNEL=1 → hermes wins (priority over lark-channel)",
			env:    map[string]string{"HERMES_HOME": "/Users/me/.hermes", "LARK_CHANNEL": "1"},
			expect: WorkspaceHermes,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			getenv := func(key string) string { return tt.env[key] }
			got := DetectWorkspaceFromEnv(getenv)
			if got != tt.expect {
				t.Errorf("DetectWorkspaceFromEnv() = %q, want %q", got, tt.expect)
			}
		})
	}
}

func TestWorkspaceDisplay(t *testing.T) {
	tests := []struct {
		ws     Workspace
		expect string
	}{
		{WorkspaceLocal, "local"},
		{Workspace(""), "local"},
		{WorkspaceOpenClaw, "openclaw"},
		{WorkspaceHermes, "hermes"},
		{WorkspaceLarkChannel, "lark-channel"},
	}
	for _, tt := range tests {
		if got := tt.ws.Display(); got != tt.expect {
			t.Errorf("Workspace(%q).Display() = %q, want %q", tt.ws, got, tt.expect)
		}
	}
}

func TestWorkspaceIsLocal(t *testing.T) {
	if !WorkspaceLocal.IsLocal() {
		t.Error("WorkspaceLocal.IsLocal() should be true")
	}
	if !Workspace("").IsLocal() {
		t.Error(`Workspace("").IsLocal() should be true`)
	}
	if WorkspaceOpenClaw.IsLocal() {
		t.Error("WorkspaceOpenClaw.IsLocal() should be false")
	}
}

func TestSetCurrentWorkspace(t *testing.T) {
	orig := CurrentWorkspace()
	defer SetCurrentWorkspace(orig)

	SetCurrentWorkspace(WorkspaceOpenClaw)
	if got := CurrentWorkspace(); got != WorkspaceOpenClaw {
		t.Errorf("CurrentWorkspace() = %q, want %q", got, WorkspaceOpenClaw)
	}

	SetCurrentWorkspace(WorkspaceLocal)
	if got := CurrentWorkspace(); got != WorkspaceLocal {
		t.Errorf("CurrentWorkspace() = %q, want %q", got, WorkspaceLocal)
	}
}

func TestGetRuntimeDir(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", tmp)

	orig := CurrentWorkspace()
	defer SetCurrentWorkspace(orig)

	// Local → base dir (same as pre-workspace behavior)
	SetCurrentWorkspace(WorkspaceLocal)
	if got := GetRuntimeDir(); got != tmp {
		t.Errorf("local: GetRuntimeDir() = %q, want %q", got, tmp)
	}
	if got := GetConfigDir(); got != tmp {
		t.Errorf("local: GetConfigDir() = %q, want %q", got, tmp)
	}

	// OpenClaw → base/openclaw
	SetCurrentWorkspace(WorkspaceOpenClaw)
	want := filepath.Join(tmp, "openclaw")
	if got := GetRuntimeDir(); got != want {
		t.Errorf("openclaw: GetRuntimeDir() = %q, want %q", got, want)
	}

	// Hermes → base/hermes
	SetCurrentWorkspace(WorkspaceHermes)
	want = filepath.Join(tmp, "hermes")
	if got := GetRuntimeDir(); got != want {
		t.Errorf("hermes: GetRuntimeDir() = %q, want %q", got, want)
	}

	// LarkChannel → base/lark-channel
	SetCurrentWorkspace(WorkspaceLarkChannel)
	want = filepath.Join(tmp, "lark-channel")
	if got := GetRuntimeDir(); got != want {
		t.Errorf("lark-channel: GetRuntimeDir() = %q, want %q", got, want)
	}
}

func TestGetConfigPath(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", tmp)

	orig := CurrentWorkspace()
	defer SetCurrentWorkspace(orig)

	SetCurrentWorkspace(WorkspaceLocal)
	want := filepath.Join(tmp, "config.json")
	if got := GetConfigPath(); got != want {
		t.Errorf("local: GetConfigPath() = %q, want %q", got, want)
	}

	SetCurrentWorkspace(WorkspaceOpenClaw)
	want = filepath.Join(tmp, "openclaw", "config.json")
	if got := GetConfigPath(); got != want {
		t.Errorf("openclaw: GetConfigPath() = %q, want %q", got, want)
	}
}
