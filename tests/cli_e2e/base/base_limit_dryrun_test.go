// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestBaseListDryRunRejectsOutOfRangeLimit(t *testing.T) {
	setBaseDryRunConfigEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"base", "+table-list",
			"--base-token", "app_x",
			"--limit", "101",
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 2)

	require.Equal(t, "validation", gjson.Get(result.Stderr, "error.type").String(), result.Stderr)
	require.Equal(t, "invalid_argument", gjson.Get(result.Stderr, "error.subtype").String(), result.Stderr)
	require.Equal(t, "--limit", gjson.Get(result.Stderr, "error.param").String(), result.Stderr)
	require.Contains(t, gjson.Get(result.Stderr, "error.message").String(), "must be between 1 and 100")
	require.Empty(t, result.Stdout)
}

func TestBaseListDryRunAcceptsPageSizeAliasForLimit(t *testing.T) {
	setBaseDryRunConfigEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"base", "+table-list",
			"--base-token", "app_x",
			"--page-size", "40",
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	require.Equal(t, int64(0), clie2e.DryRunGet(result.Stdout, "api.0.params.offset").Int(), result.Stdout)
	require.Equal(t, int64(40), clie2e.DryRunGet(result.Stdout, "api.0.params.limit").Int(), result.Stdout)
	require.False(t, clie2e.DryRunGet(result.Stdout, "api.0.params.page_size").Exists(), result.Stdout)
}

func TestBaseListDryRunUsesLastPaginationSpelling(t *testing.T) {
	setBaseDryRunConfigEnv(t)

	tests := []struct {
		name string
		args []string
		want int64
	}{
		{name: "alias last", args: []string{"--limit", "20", "--page-size", "40"}, want: 40},
		{name: "canonical last", args: []string{"--page-size", "40", "--limit", "20"}, want: 20},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			t.Cleanup(cancel)
			args := []string{"base", "+table-list", "--base-token", "app_x"}
			args = append(args, test.args...)
			args = append(args, "--dry-run")
			result, err := clie2e.RunCmd(ctx, clie2e.Request{Args: args, DefaultAs: "bot"})
			require.NoError(t, err)
			result.AssertExitCode(t, 0)
			require.Equal(t, test.want, clie2e.DryRunGet(result.Stdout, "api.0.params.limit").Int(), result.Stdout)
		})
	}
}

func TestBaseListDryRunAttributesValidationToPageSizeAlias(t *testing.T) {
	setBaseDryRunConfigEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      []string{"base", "+table-list", "--base-token", "app_x", "--page-size", "101", "--dry-run"},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 2)
	require.Equal(t, "--page-size", gjson.Get(result.Stderr, "error.param").String(), result.Stderr)
	require.Contains(t, gjson.Get(result.Stderr, "error.message").String(), "must be between 1 and 100")
}
