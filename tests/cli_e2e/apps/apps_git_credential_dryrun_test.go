// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestAppsGitCredentialInitDryRun(t *testing.T) {
	setAppsDryRunEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"apps", "+git-credential-init",
			"--app-id", "app_xxx",
			"--dry-run",
		},
		BinaryPath: "../../../lark-cli",
		DefaultAs:  "user",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	assert.Equal(t, "GET", gjson.Get(result.Stdout, "api.0.method").String())
	assert.Equal(t, "/open-apis/spark/v1/apps/app_xxx/git_info", gjson.Get(result.Stdout, "api.0.url").String())
	assert.Equal(t, "app_xxx", gjson.Get(result.Stdout, "api.0.params.app_id").String())
	assert.False(t, gjson.Get(result.Stdout, "api.0.body").Exists())
	assert.Equal(t, "api-plus-local-setup", gjson.Get(result.Stdout, "mode").String())
	assert.Equal(t, "initialize_local_git_credential", gjson.Get(result.Stdout, "action").String())
	assert.True(t, strings.HasSuffix(gjson.Get(result.Stdout, "metadata_file").String(), filepath.Join("spark", "app_xxx", "git.json")))
	assert.Equal(t, int64(3), gjson.Get(result.Stdout, "local_effects.#").Int())
	assert.Equal(t, "save the issued PAT in the local system credential store", gjson.Get(result.Stdout, "local_effects.0").String())
	assert.Equal(t, "write app-scoped git credential metadata", gjson.Get(result.Stdout, "local_effects.1").String())
	assert.Equal(t, "configure a URL-scoped Git credential helper in global git config when possible", gjson.Get(result.Stdout, "local_effects.2").String())
}

func TestAppsGitCredentialListDryRun(t *testing.T) {
	setAppsDryRunEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:       []string{"apps", "+git-credential-list", "--dry-run"},
		BinaryPath: "../../../lark-cli",
		DefaultAs:  "user",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	assert.Equal(t, "Preview local Git credential listing (no API call, read-only local state).", gjson.Get(result.Stdout, "description").String())
	assert.Equal(t, "local-read-only", gjson.Get(result.Stdout, "mode").String())
	assert.Equal(t, "list_local_git_credentials", gjson.Get(result.Stdout, "action").String())
	assert.Equal(t, int64(0), gjson.Get(result.Stdout, "api.#").Int())
	assert.Contains(t, gjson.Get(result.Stdout, "storage_root").String(), filepath.Join("", "spark"))
	assert.Equal(t, "scan app-scoped git credential metadata under the CLI config directory", gjson.Get(result.Stdout, "reads.0").String())
}

func TestAppsGitCredentialRemoveDryRun(t *testing.T) {
	setAppsDryRunEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:       []string{"apps", "+git-credential-remove", "--app-id", "app_xxx", "--dry-run"},
		BinaryPath: "../../../lark-cli",
		DefaultAs:  "user",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	assert.Equal(t, "Preview local Git credential cleanup (no API call; would clean up local-only state).", gjson.Get(result.Stdout, "description").String())
	assert.Equal(t, "local-cleanup-only", gjson.Get(result.Stdout, "mode").String())
	assert.Equal(t, "remove_local_git_credential", gjson.Get(result.Stdout, "action").String())
	assert.Equal(t, "app_xxx", gjson.Get(result.Stdout, "app_id").String())
	assert.Equal(t, int64(0), gjson.Get(result.Stdout, "api.#").Int())
	assert.True(t, strings.HasSuffix(gjson.Get(result.Stdout, "metadata_file").String(), filepath.Join("spark", "app_xxx", "git.json")))
	assert.Equal(t, "read app-scoped git credential metadata", gjson.Get(result.Stdout, "effects.0").String())
}
