// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package wiki

import (
	"context"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestWikiNodeGetDryRunTokenLookup(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("LARKSUITE_CLI_APP_ID", "wiki_node_get_dryrun_test")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "wiki_node_get_dryrun_secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	for _, tt := range []struct {
		name    string
		input   string
		token   string
		objType string
	}{
		{name: "short token reaches server", input: "short", token: "short"},
		{name: "opaque token", input: "opaque_example_token", token: "opaque_example_token"},
		{name: "document URL", input: "https://example.com/docx/docx_example", token: "docx_example"},
		{name: "legacy type is ignored", input: "docx_example", token: "docx_example", objType: "docx"},
		{name: "unknown legacy type is ignored", input: "docx_example", token: "docx_example", objType: "unknown"},
		{name: "legacy type contradicts URL", input: "https://example.com/docx/docx_example", token: "docx_example", objType: "sheet"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			args := []string{"wiki", "+node-get", "--node-token", tt.input, "--space-id", "123", "--dry-run"}
			if tt.objType != "" {
				args = append(args, "--obj-type", tt.objType)
			}
			result, err := clie2e.RunCmd(ctx, clie2e.Request{Args: args, DefaultAs: "bot"})
			require.NoError(t, err)
			result.AssertExitCode(t, 0)
			api := clie2e.DryRunGet(result.Stdout, "api").Array()
			require.Len(t, api, 1, result.Stdout)
			require.Equal(t, "GET", api[0].Get("method").String())
			require.Equal(t, "/open-apis/wiki/v2/spaces/node_by_token", api[0].Get("url").String())
			params := api[0].Get("params").Map()
			require.Len(t, params, 1, result.Stdout)
			require.Equal(t, tt.token, params["token"].String())
			require.False(t, api[0].Get("body").Exists(), result.Stdout)
			require.Empty(t, result.Stderr)
		})
	}
}

func TestWikiNodeGetHelpHidesLegacyObjectType(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	result, err := clie2e.RunCmd(ctx, clie2e.Request{Args: []string{"wiki", "+node-get", "--help"}})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	require.Contains(t, result.Stdout, "--node-token")
	require.NotContains(t, result.Stdout, "--obj-type")
	require.Empty(t, result.Stderr)
}

func TestWikiNodeGetDryRunStillRequiresToken(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("LARKSUITE_CLI_APP_ID", "wiki_node_get_dryrun_test")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "wiki_node_get_dryrun_secret")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	result, err := clie2e.RunCmd(ctx, clie2e.Request{Args: []string{"wiki", "+node-get", "--dry-run"}, DefaultAs: "bot"})
	require.NoError(t, err)
	result.AssertExitCode(t, 2)
	require.Empty(t, result.Stdout)
	require.Equal(t, "validation", gjson.Get(result.Stderr, "error.type").String())
	require.Equal(t, "invalid_argument", gjson.Get(result.Stderr, "error.subtype").String())
	require.Equal(t, "--node-token", gjson.Get(result.Stderr, "error.param").String())
}
