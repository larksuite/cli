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

// --- Dry-run E2E tests (no real API calls, no secrets needed) ---

func setDryRunConfigEnv(t *testing.T) {
	t.Helper()
	t.Setenv("LARKSUITE_CLI_APP_ID", "cli_dryrun_test")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "dryrun_secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")
}

func TestLingo_SearchDryRun(t *testing.T) {
	setDryRunConfigEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"lingo", "+search",
			"--query", "KYC",
			"--page-size", "30",
			"--dry-run",
		},
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	out := result.Stdout
	assert.True(t, strings.Contains(out, "/open-apis/lingo/v1/entities/search"), "dry-run should contain API path, got: %s", out)
	assert.True(t, strings.Contains(out, "POST"), "dry-run should be POST, got: %s", out)
	assert.True(t, strings.Contains(out, "KYC"), "dry-run should contain query, got: %s", out)
}

func TestLingo_MatchDryRun(t *testing.T) {
	setDryRunConfigEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"lingo", "+match",
			"--word", "KYC",
			"--dry-run",
		},
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	out := result.Stdout
	assert.True(t, strings.Contains(out, "/open-apis/lingo/v1/entities/match"), "dry-run should contain API path, got: %s", out)
	assert.True(t, strings.Contains(out, "KYC"), "dry-run should contain word, got: %s", out)
}

func TestLingo_GetDryRun(t *testing.T) {
	setDryRunConfigEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"lingo", "+get",
			"--entity-id", "ent-1",
			"--dry-run",
		},
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	out := result.Stdout
	assert.True(t, strings.Contains(out, "/open-apis/lingo/v1/entities/ent-1"), "dry-run should contain resolved path, got: %s", out)
	assert.True(t, strings.Contains(out, "GET"), "dry-run should be GET, got: %s", out)
}

func TestLingo_CreateDryRun(t *testing.T) {
	setDryRunConfigEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"lingo", "+create",
			"--main-key", "KYC",
			"--aliases", "Know Your Customer",
			"--description", "AML monitoring concept",
			"--dry-run",
		},
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	out := result.Stdout
	assert.True(t, strings.Contains(out, "/open-apis/lingo/v1/entities"), "dry-run should contain API path, got: %s", out)
	assert.True(t, strings.Contains(out, "POST"), "dry-run should be POST, got: %s", out)
	assert.True(t, strings.Contains(out, "main_keys"), "dry-run body should contain main_keys, got: %s", out)
	assert.True(t, strings.Contains(out, "Know Your Customer"), "dry-run should contain alias, got: %s", out)
	assert.True(t, strings.Contains(out, "AML monitoring concept"), "dry-run should contain description, got: %s", out)
}

func TestLingo_UpdateDryRun(t *testing.T) {
	setDryRunConfigEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"lingo", "+update",
			"--entity-id", "ent-1",
			"--main-key", "KYC",
			"--description", "updated",
			"--dry-run",
		},
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	out := result.Stdout
	assert.True(t, strings.Contains(out, "/open-apis/lingo/v1/entities/ent-1"), "dry-run should contain resolved path, got: %s", out)
	assert.True(t, strings.Contains(out, "PUT"), "dry-run should be PUT, got: %s", out)
	assert.True(t, strings.Contains(out, "updated"), "dry-run should contain description, got: %s", out)
}

func TestLingo_DeleteDryRun(t *testing.T) {
	setDryRunConfigEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"lingo", "+delete",
			"--entity-id", "ent-1",
			"--dry-run",
		},
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	out := result.Stdout
	assert.True(t, strings.Contains(out, "/open-apis/lingo/v1/entities/ent-1"), "dry-run should contain resolved path, got: %s", out)
	assert.True(t, strings.Contains(out, "DELETE"), "dry-run should be DELETE, got: %s", out)
}
