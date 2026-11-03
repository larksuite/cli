// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"strings"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestBasePluginSearchDryRun(t *testing.T) {
	setBaseDryRunConfigEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"base", "+plugin-search",
			"--keyword", "Jira issue sync",
			"--plugin-types", "data_connector,workflow",
			"--limit", "7",
			"--dry-run",
		},
		DefaultAs: "user",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	result.AssertStdoutStatus(t, true)
	require.Equal(t, "POST", clie2e.DryRunGet(result.Stdout, "api.0.method").String(), result.Stdout)
	require.Equal(t, "/open-apis/base/v3/plugins/search", clie2e.DryRunGet(result.Stdout, "api.0.url").String(), result.Stdout)
	require.JSONEq(t, `{
  "query": "Jira issue sync",
  "plugin_types": ["data_connector", "workflow"],
  "limit": 7
}`, clie2e.DryRunGet(result.Stdout, "api.0.body").Raw, result.Stdout)
}

func TestBasePluginSearchDryRunOmitsPluginTypes(t *testing.T) {
	result := runBaseDryRun(t, 0,
		"base", "+plugin-search",
		"--keyword", "AI field extension",
	)
	body := clie2e.DryRunGet(result.Stdout, "api.0.body")
	require.False(t, body.Get("plugin_types").Exists(), result.Stdout)
	require.Equal(t, int64(10), body.Get("limit").Int(), result.Stdout)
}

func TestBasePluginSearchDryRunRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		wantParam string
	}{
		{
			name:      "blank keyword",
			args:      []string{"--keyword", "  "},
			wantParam: "--keyword",
		},
		{
			name:      "keyword too long",
			args:      []string{"--keyword", strings.Repeat("界", 201)},
			wantParam: "--keyword",
		},
		{
			name:      "unknown plugin type",
			args:      []string{"--keyword", "jira", "--plugin-types", "unknown"},
			wantParam: "--plugin-types",
		},
		{
			name:      "empty plugin types",
			args:      []string{"--keyword", "jira", "--plugin-types", ""},
			wantParam: "--plugin-types",
		},
		{
			name:      "limit out of range",
			args:      []string{"--keyword", "jira", "--limit", "21"},
			wantParam: "--limit",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			args := append([]string{"base", "+plugin-search"}, test.args...)
			result := runBaseDryRun(t, 2, args...)
			require.Equal(t, "validation", gjson.Get(result.Stderr, "error.type").String(), result.Stderr)
			require.Equal(t, "invalid_argument", gjson.Get(result.Stderr, "error.subtype").String(), result.Stderr)
			require.Equal(t, test.wantParam, gjson.Get(result.Stderr, "error.param").String(), result.Stderr)
			require.Empty(t, result.Stdout)
		})
	}
}

func TestBasePluginSearchHelpGuidesQueryRewrite(t *testing.T) {
	setBaseDryRunConfigEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      []string{"base", "+plugin-search", "--help"},
		DefaultAs: "user",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	for _, guidance := range []string{
		"resolve references",
		"1-2 high-signal terms",
		"synonyms, common names, or alternate phrasing",
		"data synchronization",
		"external source product name",
		"omit generic terms such as sync, connector, or Base",
		"small set of same-intent queries",
		"alternative terms or common names",
		"one rewritten query per call",
		"do not concatenate multiple queries",
	} {
		require.Contains(t, result.Stdout, guidance)
	}
}
