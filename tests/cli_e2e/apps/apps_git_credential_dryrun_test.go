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

	assert.Equal(t, "GET", clie2e.DryRunGet(result.Stdout, "api.0.method").String())
	assert.Equal(t, "/open-apis/spark/v1/apps/app_xxx/git_info", clie2e.DryRunGet(result.Stdout, "api.0.url").String())
	assert.Equal(t, "app_xxx", clie2e.DryRunGet(result.Stdout, "api.0.params.app_id").String())
	assert.False(t, clie2e.DryRunGet(result.Stdout, "api.0.body").Exists())
	assert.Equal(t, "api-plus-local-setup", clie2e.DryRunGet(result.Stdout, "mode").String())
	assert.Equal(t, "initialize_local_git_credential", clie2e.DryRunGet(result.Stdout, "action").String())
	assert.True(t, strings.HasSuffix(clie2e.DryRunGet(result.Stdout, "metadata_file").String(), filepath.Join("spark", "app_xxx", "git.json")))
	assert.Equal(t, int64(3), clie2e.DryRunGet(result.Stdout, "local_effects.#").Int())
	assert.Equal(t, "save the issued PAT in the local system credential store", clie2e.DryRunGet(result.Stdout, "local_effects.0").String())
	assert.Equal(t, "write app-scoped git credential metadata", clie2e.DryRunGet(result.Stdout, "local_effects.1").String())
	assert.Equal(t, "configure a URL-scoped Git credential helper in global git config when possible", clie2e.DryRunGet(result.Stdout, "local_effects.2").String())
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

	assert.Equal(t, "Preview local Git credential listing (no API call, read-only local state).", clie2e.DryRunGet(result.Stdout, "description").String())
	assert.Equal(t, "local-read-only", clie2e.DryRunGet(result.Stdout, "mode").String())
	assert.Equal(t, "list_local_git_credentials", clie2e.DryRunGet(result.Stdout, "action").String())
	assert.Equal(t, int64(0), clie2e.DryRunGet(result.Stdout, "api.#").Int())
	assert.Contains(t, clie2e.DryRunGet(result.Stdout, "storage_root").String(), filepath.Join("", "spark"))
	assert.Equal(t, "scan app-scoped git credential metadata under the CLI config directory", clie2e.DryRunGet(result.Stdout, "reads.0").String())
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

	assert.Equal(t, "Preview local Git credential cleanup (no API call; would clean up local-only state).", clie2e.DryRunGet(result.Stdout, "description").String())
	assert.Equal(t, "local-cleanup-only", clie2e.DryRunGet(result.Stdout, "mode").String())
	assert.Equal(t, "remove_local_git_credential", clie2e.DryRunGet(result.Stdout, "action").String())
	assert.Equal(t, "app_xxx", clie2e.DryRunGet(result.Stdout, "app_id").String())
	assert.Equal(t, int64(0), clie2e.DryRunGet(result.Stdout, "api.#").Int())
	assert.True(t, strings.HasSuffix(clie2e.DryRunGet(result.Stdout, "metadata_file").String(), filepath.Join("spark", "app_xxx", "git.json")))
	assert.Equal(t, "read app-scoped git credential metadata", clie2e.DryRunGet(result.Stdout, "effects.0").String())
}
