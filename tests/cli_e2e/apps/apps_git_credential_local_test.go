// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestAppsGitCredentialListLocalE2E(t *testing.T) {
	env := setupAppsGitCredentialLocalEnv(t)
	seedAppsGitCredentialMetadata(t, env.configDir, "app_a", "https://example.com/git/u/a.git", "pat-ref-a")
	seedAppsGitCredentialMetadata(t, env.configDir, "app_b", "https://example.com/git/u/b.git", "pat-ref-b")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      []string{"apps", "+git-credential-list"},
		DefaultAs: "user",
		Env:       env.vars,
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	assert.Equal(t, int64(2), gjson.Get(result.Stdout, "data.count").Int())
	assert.Equal(t, "app_a", gjson.Get(result.Stdout, "data.credentials.0.app_id").String())
	assert.Equal(t, "https://example.com/git/u/a.git", gjson.Get(result.Stdout, "data.credentials.0.repository_url").String())
	assert.Equal(t, "missing_secret", gjson.Get(result.Stdout, "data.credentials.0.status").String())
	assert.Equal(t, "app_b", gjson.Get(result.Stdout, "data.credentials.1.app_id").String())
	assert.False(t, gjson.Get(result.Stdout, "data.credentials.0.expires_at").Exists())
	assert.False(t, gjson.Get(result.Stdout, "data.credentials.0.expired").Exists())
}

func TestAppsGitCredentialRemoveLocalE2E(t *testing.T) {
	env := setupAppsGitCredentialLocalEnv(t)
	metadataPath := seedAppsGitCredentialMetadata(t, env.configDir, "app_xxx", "https://example.com/git/u/app.git", "pat-ref-remove")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      []string{"apps", "+git-credential-remove", "--app-id", "app_xxx"},
		DefaultAs: "user",
		Env:       env.vars,
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	assert.Equal(t, "app_xxx", gjson.Get(result.Stdout, "data.app_id").String())
	assert.True(t, gjson.Get(result.Stdout, "data.removed").Bool())
	assert.NoFileExists(t, metadataPath)
}

type appsGitCredentialLocalEnv struct {
	configDir string
	vars      map[string]string
}

func setupAppsGitCredentialLocalEnv(t *testing.T) appsGitCredentialLocalEnv {
	t.Helper()
	configDir := t.TempDir()
	homeDir := t.TempDir()
	gitConfig := filepath.Join(t.TempDir(), ".gitconfig")
	return appsGitCredentialLocalEnv{
		configDir: configDir,
		vars: map[string]string{
			"LARKSUITE_CLI_CONFIG_DIR":         configDir,
			"LARKSUITE_CLI_APP_ID":             "apps_local_test",
			"LARKSUITE_CLI_APP_SECRET":         "apps_local_secret",
			"LARKSUITE_CLI_BRAND":              "feishu",
			"LARKSUITE_CLI_NO_UPDATE_NOTIFIER": "1",
			"LARKSUITE_CLI_NO_SKILLS_NOTIFIER": "1",
			"LARKSUITE_CLI_DATA_DIR":           filepath.Join(homeDir, ".local", "share"),
			"HOME":                             homeDir,
			"GIT_CONFIG_GLOBAL":                gitConfig,
			"GIT_CONFIG_NOSYSTEM":              "1",
			"GIT_TERMINAL_PROMPT":              "0",
		},
	}
}

func seedAppsGitCredentialMetadata(t *testing.T, configDir, appID, gitHTTPURL, patRef string) string {
	t.Helper()
	dir := filepath.Join(configDir, "spark", url.PathEscape(appID))
	require.NoError(t, os.MkdirAll(dir, 0700))
	path := filepath.Join(dir, "git.json")
	payload := map[string]any{
		"version":        1,
		"app_id":         appID,
		"git_http_url":   gitHTTPURL,
		"profile":        "default",
		"profile_app_id": "apps_local_test",
		"user_open_id":   "ou_local_test",
		"username":       "x-access-token",
		"pat_ref":        patRef,
		"status":         "confirmed",
		"expires_at":     time.Now().Add(24 * time.Hour).Unix(),
		"updated_at":     time.Now().Unix(),
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, append(data, '\n'), 0600))
	return path
}
