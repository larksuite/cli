// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package config

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// clearAgentEnv removes every env var that DetectWorkspaceFromEnv treats as
// an Agent signal (OPENCLAW_* / HERMES_* / LARK_CHANNEL). Prefix-based so the
// helper stays correct when DetectWorkspaceFromEnv adds new signals; tests
// no longer drift silently. Mirrors cmd/config/bind_test.go's helper.
func clearAgentEnv(t *testing.T) {
	t.Helper()
	for _, kv := range os.Environ() {
		idx := strings.IndexByte(kv, '=')
		if idx < 0 {
			continue
		}
		k := kv[:idx]
		if strings.HasPrefix(k, "OPENCLAW_") ||
			strings.HasPrefix(k, "HERMES_") ||
			k == "LARK_CHANNEL" {
			t.Setenv(k, "")
		}
	}
}

// setupTempConfig creates a temp config dir and sets LARKSUITE_CLI_CONFIG_DIR.
func setupTempConfig(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", dir)
	return dir
}

// writeHermesEnv creates a fake ~/.hermes/.env with feishu credentials.
func writeHermesEnv(t *testing.T, hermesHome, appID, appSecret, domain string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(hermesHome, 0700))
	content := "FEISHU_APP_ID=" + appID + "\nFEISHU_APP_SECRET=" + appSecret + "\n"
	if domain != "" {
		content += "FEISHU_DOMAIN=" + domain + "\n"
	}
	require.NoError(t, os.WriteFile(filepath.Join(hermesHome, ".env"), []byte(content), 0600))
}

// writeOpenClawConfig creates a fake openclaw.json with a single feishu account.
func writeOpenClawConfig(t *testing.T, openclawHome, appID, appSecret, brand string) {
	t.Helper()
	dir := filepath.Join(openclawHome, ".openclaw")
	require.NoError(t, os.MkdirAll(dir, 0700))
	content := `{"channels":{"feishu":{"appId":"` + appID + `","appSecret":"` + appSecret + `","domain":"` + brand + `"}}}`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "openclaw.json"), []byte(content), 0600))
}

// writeLarkChannelConfig creates a fake ~/.lark-channel/config.json under
// fakeHome (caller is responsible for setting HOME=fakeHome via t.Setenv).
func writeLarkChannelConfig(t *testing.T, fakeHome, appID, appSecret, tenant string) {
	t.Helper()
	dir := filepath.Join(fakeHome, ".lark-channel")
	require.NoError(t, os.MkdirAll(dir, 0700))
	content := `{"accounts":{"app":{"id":"` + appID + `","secret":"` + appSecret + `","tenant":"` + tenant + `"}}}`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.json"), []byte(content), 0600))
}

// assertStderrError verifies the structured error JSON envelope in stderr.
// Checks error.type and error.message exactly. hint is checked if non-empty.
func assertStderrError(t *testing.T, result *clie2e.Result, wantExitCode int, wantType, wantMessage, wantHint string) {
	t.Helper()
	assert.Equal(t, wantExitCode, result.ExitCode, "exit code mismatch\nstdout:\n%s\nstderr:\n%s", result.Stdout, result.Stderr)

	errJSON := gjson.Get(result.Stderr, "error")
	require.True(t, errJSON.Exists(), "stderr missing 'error' JSON envelope\nstderr:\n%s", result.Stderr)

	assert.Equal(t, wantType, errJSON.Get("type").String(),
		"error.type mismatch\nstderr:\n%s", result.Stderr)
	assert.Equal(t, wantMessage, errJSON.Get("message").String(),
		"error.message mismatch\nstderr:\n%s", result.Stderr)
	if wantHint != "" {
		assert.Equal(t, wantHint, errJSON.Get("hint").String(),
			"error.hint mismatch\nstderr:\n%s", result.Stderr)
	}
}

func TestBind_InvalidSource(t *testing.T) {
	setupTempConfig(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{"config", "bind", "--source", "invalid"},
	})
	require.NoError(t, err)
	assertStderrError(t, result, 2, "validation",
		`invalid --source "invalid"; valid values: openclaw, hermes, lark-channel`, "")
}

func TestBind_MissingSource_NonTTY(t *testing.T) {
	setupTempConfig(t)
	// Clear Agent env so DetectWorkspaceFromEnv returns WorkspaceLocal and
	// finalizeSource hits the "cannot determine Agent source" branch instead
	// of silently auto-detecting whichever Agent the CI runner happens to
	// inherit env from.
	clearAgentEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:  []string{"config", "bind"},
		Stdin: []byte{}, // force non-TTY via explicit empty stdin
	})
	require.NoError(t, err)
	assertStderrError(t, result, 2, "bind",
		"cannot determine Agent source: no --source flag and no Agent environment detected",
		"pass --source openclaw|hermes|lark-channel, or run this command inside the corresponding Agent context")
}

func TestBind_Hermes_Success(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	configDir := setupTempConfig(t)
	hermesHome := filepath.Join(t.TempDir(), ".hermes-test")
	t.Setenv("HERMES_HOME", hermesHome)
	writeHermesEnv(t, hermesHome, "cli_e2e_test", "e2e_secret", "lark")

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{"config", "bind", "--source", "hermes"},
	})
	require.NoError(t, err)

	if result.ExitCode == 0 {
		// Success path: verify stdout JSON envelope exactly
		stdout := result.Stdout
		assert.True(t, gjson.Get(stdout, "ok").Bool(), "stdout:\n%s", stdout)
		assert.Equal(t, "hermes", gjson.Get(stdout, "workspace").String(), "stdout:\n%s", stdout)
		assert.Equal(t, "cli_e2e_test", gjson.Get(stdout, "app_id").String(), "stdout:\n%s", stdout)

		expectedConfigPath := filepath.Join(configDir, "hermes", "config.json")
		assert.Equal(t, expectedConfigPath, gjson.Get(stdout, "config_path").String(), "stdout:\n%s", stdout)

		// Verify config file content exactly
		data, readErr := os.ReadFile(expectedConfigPath)
		require.NoError(t, readErr)
		assert.Equal(t, "cli_e2e_test", gjson.GetBytes(data, "apps.0.appId").String())
		assert.Equal(t, "lark", gjson.GetBytes(data, "apps.0.brand").String())
	} else {
		// Keychain failure is acceptable in CI — verify error type is keychain-related.
		// Note: exact message depends on OS keychain error (platform-dependent), so we
		// check the structured type field instead of message text.
		errType := gjson.Get(result.Stderr, "error.type").String()
		assert.Equal(t, "hermes", errType,
			"non-zero exit should be from hermes bind path\nstderr:\n%s", result.Stderr)
	}
}

func TestBind_Hermes_MissingEnvFile(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	setupTempConfig(t)
	hermesHome := filepath.Join(t.TempDir(), "nonexistent")
	t.Setenv("HERMES_HOME", hermesHome)

	envPath := filepath.Join(hermesHome, ".env")
	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{"config", "bind", "--source", "hermes"},
	})
	require.NoError(t, err)
	assertStderrError(t, result, 2, "hermes",
		"failed to read Hermes config: open "+envPath+": no such file or directory",
		"verify Hermes is installed and configured at "+envPath)
}

func TestBind_Hermes_MissingAppID(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	setupTempConfig(t)
	hermesHome := filepath.Join(t.TempDir(), ".hermes-test")
	t.Setenv("HERMES_HOME", hermesHome)
	require.NoError(t, os.MkdirAll(hermesHome, 0700))
	require.NoError(t, os.WriteFile(
		filepath.Join(hermesHome, ".env"),
		[]byte("FEISHU_APP_SECRET=secret_only\n"),
		0600,
	))

	envPath := filepath.Join(hermesHome, ".env")
	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{"config", "bind", "--source", "hermes"},
	})
	require.NoError(t, err)
	assertStderrError(t, result, 2, "hermes",
		"FEISHU_APP_ID not found in "+envPath,
		"run 'hermes setup' to configure Feishu credentials")
}

func TestBind_FlagMode_Overwrite(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	configDir := setupTempConfig(t)
	hermesHome := filepath.Join(t.TempDir(), ".hermes-test")
	t.Setenv("HERMES_HOME", hermesHome)
	writeHermesEnv(t, hermesHome, "cli_e2e_new", "e2e_new_secret", "feishu")

	// Pre-create config to simulate existing binding
	hermesDir := filepath.Join(configDir, "hermes")
	require.NoError(t, os.MkdirAll(hermesDir, 0700))
	configPath := filepath.Join(hermesDir, "config.json")
	require.NoError(t, os.WriteFile(configPath, []byte(`{"apps":[{"appId":"old_app"}]}`), 0600))

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{"config", "bind", "--source", "hermes"},
	})
	require.NoError(t, err)

	if result.ExitCode == 0 {
		// Flag mode silently overwrites and flags replaced=true in stdout.
		assert.True(t, gjson.Get(result.Stdout, "ok").Bool(), "stdout:\n%s", result.Stdout)
		assert.True(t, gjson.Get(result.Stdout, "replaced").Bool(), "stdout:\n%s", result.Stdout)
		assert.Equal(t, "cli_e2e_new", gjson.Get(result.Stdout, "app_id").String(), "stdout:\n%s", result.Stdout)
		// Rebind is now signalled only by `replaced:true` in the stdout
		// envelope (checked above). stderr only carries the standard
		// success header; sanity-check its prefix is present.
		assert.Contains(t, result.Stderr, "配置成功",
			"stderr should carry the bind-success header\nstderr:\n%s", result.Stderr)
	} else {
		// Keychain failure acceptable in CI
		errType := gjson.Get(result.Stderr, "error.type").String()
		assert.Equal(t, "hermes", errType,
			"non-zero exit should be from hermes bind path\nstderr:\n%s", result.Stderr)
	}
}

func TestBind_ConfigShow_WorkspaceField(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	configDir := setupTempConfig(t)
	// Test asserts workspace == "local"; clear Agent signals so an inherited
	// LARK_CHANNEL=1 / OPENCLAW_* / HERMES_* doesn't reroute to a workspace
	// where the local config we just wrote is invisible.
	clearAgentEnv(t)
	require.NoError(t, os.WriteFile(
		filepath.Join(configDir, "config.json"),
		[]byte(`{"apps":[{"appId":"cli_local","appSecret":"secret","brand":"feishu"}]}`),
		0600,
	))

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{"config", "show"},
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	assert.Equal(t, "local", gjson.Get(result.Stdout, "workspace").String())
	assert.Equal(t, "cli_local", gjson.Get(result.Stdout, "appId").String())
}

func TestBind_ConfigShow_UnboundWorkspace(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	setupTempConfig(t)
	t.Setenv("OPENCLAW_CLI", "1") // force openclaw workspace

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{"config", "show"},
	})
	require.NoError(t, err)
	assertStderrError(t, result, 2, "openclaw",
		"openclaw context detected but lark-cli is not bound to it",
		"read `lark-cli config bind --help`, then ask the user to confirm intent and identity preset (bot-only or user-default); only after both are confirmed, run `lark-cli config bind`")
}

func TestBind_OpenClaw_MissingFile(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	setupTempConfig(t)
	openclawHome := filepath.Join(t.TempDir(), "nonexistent")
	t.Setenv("OPENCLAW_HOME", openclawHome)
	t.Setenv("OPENCLAW_CONFIG_PATH", "")
	t.Setenv("OPENCLAW_STATE_DIR", "")

	configPath := filepath.Join(openclawHome, ".openclaw", "openclaw.json")
	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{"config", "bind", "--source", "openclaw"},
	})
	require.NoError(t, err)
	assertStderrError(t, result, 2, "openclaw",
		"cannot read "+configPath+": open "+configPath+": no such file or directory",
		"verify OpenClaw is installed and configured")
}

func TestBind_OpenClaw_Success(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	configDir := setupTempConfig(t)
	openclawHome := t.TempDir()
	t.Setenv("OPENCLAW_HOME", openclawHome)
	t.Setenv("OPENCLAW_CONFIG_PATH", "")
	t.Setenv("OPENCLAW_STATE_DIR", "")
	writeOpenClawConfig(t, openclawHome, "cli_oc_test", "oc_secret", "feishu")

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{"config", "bind", "--source", "openclaw"},
	})
	require.NoError(t, err)

	if result.ExitCode == 0 {
		// Success path: verify stdout JSON envelope exactly
		stdout := result.Stdout
		assert.True(t, gjson.Get(stdout, "ok").Bool(), "stdout:\n%s", stdout)
		assert.Equal(t, "openclaw", gjson.Get(stdout, "workspace").String(), "stdout:\n%s", stdout)
		assert.Equal(t, "cli_oc_test", gjson.Get(stdout, "app_id").String(), "stdout:\n%s", stdout)

		expectedConfigPath := filepath.Join(configDir, "openclaw", "config.json")
		assert.Equal(t, expectedConfigPath, gjson.Get(stdout, "config_path").String(), "stdout:\n%s", stdout)

		// Verify config file content exactly
		data, readErr := os.ReadFile(expectedConfigPath)
		require.NoError(t, readErr)
		assert.Equal(t, "cli_oc_test", gjson.GetBytes(data, "apps.0.appId").String())
		assert.Equal(t, "feishu", gjson.GetBytes(data, "apps.0.brand").String())
	} else {
		// Keychain failure acceptable in CI
		errType := gjson.Get(result.Stderr, "error.type").String()
		assert.Equal(t, "openclaw", errType,
			"non-zero exit should be from openclaw bind path\nstderr:\n%s", result.Stderr)
	}
}

// TestBind_LarkChannel_Success exercises the full end-to-end happy path:
// fake bridge config under HOME → bind reads it → workspace config written
// to LARKSUITE_CLI_CONFIG_DIR/lark-channel/config.json with brand from tenant.
func TestBind_LarkChannel_Success(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	configDir := setupTempConfig(t)
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)
	writeLarkChannelConfig(t, fakeHome, "cli_lc_e2e", "lc_secret", "lark")

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{"config", "bind", "--source", "lark-channel"},
	})
	require.NoError(t, err)

	if result.ExitCode == 0 {
		stdout := result.Stdout
		assert.True(t, gjson.Get(stdout, "ok").Bool(), "stdout:\n%s", stdout)
		assert.Equal(t, "lark-channel", gjson.Get(stdout, "workspace").String(), "stdout:\n%s", stdout)
		assert.Equal(t, "cli_lc_e2e", gjson.Get(stdout, "app_id").String(), "stdout:\n%s", stdout)

		expectedConfigPath := filepath.Join(configDir, "lark-channel", "config.json")
		assert.Equal(t, expectedConfigPath, gjson.Get(stdout, "config_path").String(), "stdout:\n%s", stdout)

		data, readErr := os.ReadFile(expectedConfigPath)
		require.NoError(t, readErr)
		assert.Equal(t, "cli_lc_e2e", gjson.GetBytes(data, "apps.0.appId").String())
		assert.Equal(t, "lark", gjson.GetBytes(data, "apps.0.brand").String())
	} else {
		// Keychain failure acceptable in CI; verify the error came from the
		// lark-channel binder (i.e. routing was correct) rather than another path.
		errType := gjson.Get(result.Stderr, "error.type").String()
		assert.Equal(t, "lark-channel", errType,
			"non-zero exit should be from lark-channel bind path\nstderr:\n%s", result.Stderr)
	}
}

// TestBind_LarkChannel_MissingFile verifies the routed error path when the
// bridge has not been configured: hint must point at bridge setup, not at
// `config init` (which would silently create a parallel local app and waste
// the user's existing bridge credentials).
func TestBind_LarkChannel_MissingFile(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	setupTempConfig(t)
	fakeHome := t.TempDir() // empty — no .lark-channel/config.json
	t.Setenv("HOME", fakeHome)

	configPath := filepath.Join(fakeHome, ".lark-channel", "config.json")
	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{"config", "bind", "--source", "lark-channel"},
	})
	require.NoError(t, err)
	assertStderrError(t, result, 2, "lark-channel",
		"cannot read "+configPath+": open "+configPath+": no such file or directory",
		"verify lark-channel-bridge is installed and configured")
}

// TestBind_LarkChannel_AutoDetect verifies LARK_CHANNEL=1 alone routes the
// no-flag bind into the lark-channel workspace (matches the bridge's actual
// runtime — it sets the env, not --source).
func TestBind_LarkChannel_AutoDetect(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	setupTempConfig(t)
	// Clear other agent env so OpenClaw/Hermes signals from the host shell
	// don't preempt the lark-channel detection.
	clearAgentEnv(t)
	t.Setenv("LARK_CHANNEL", "1")

	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)
	writeLarkChannelConfig(t, fakeHome, "cli_lc_auto", "auto_secret", "feishu")

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{"config", "bind"}, // no --source
	})
	require.NoError(t, err)

	if result.ExitCode == 0 {
		assert.Equal(t, "lark-channel", gjson.Get(result.Stdout, "workspace").String(),
			"stdout:\n%s", result.Stdout)
	} else {
		errType := gjson.Get(result.Stderr, "error.type").String()
		assert.Equal(t, "lark-channel", errType,
			"non-zero exit should be from lark-channel bind path\nstderr:\n%s", result.Stderr)
	}
}
