// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package lingo

import (
	"context"
	"strings"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setDryRunConfigEnv(t *testing.T) {
	t.Helper()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("LARKSUITE_CLI_APP_ID", "cli_dryrun_test")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "dryrun_secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")
}

func TestLingo_GetDryRun(t *testing.T) {
	setDryRunConfigEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	t.Run("by word", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"lingo", "+get", "--word", "KYC", "--dry-run"},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		assert.True(t, strings.Contains(result.Stdout, "/open-apis/lingo/v1/entities/match"), "got: %s", result.Stdout)
		assert.True(t, strings.Contains(result.Stdout, "KYC"), "got: %s", result.Stdout)
	})

	t.Run("by entity-id", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"lingo", "+get", "--entity-id", "enterprise_123", "--dry-run"},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		assert.True(t, strings.Contains(result.Stdout, "/open-apis/lingo/v1/entities/enterprise_123"), "got: %s", result.Stdout)
	})
}

func TestLingo_SearchDryRun(t *testing.T) {
	setDryRunConfigEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{"lingo", "+search", "--query", "KYC", "--page-size", "30", "--dry-run"},
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	assert.True(t, strings.Contains(result.Stdout, "/open-apis/lingo/v1/entities/search"), "got: %s", result.Stdout)
	assert.True(t, strings.Contains(result.Stdout, "KYC"), "got: %s", result.Stdout)
}
