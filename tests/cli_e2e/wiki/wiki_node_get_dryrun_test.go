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

func TestWikiNodeGetDryRunRejectsTruncatedToken(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("LARKSUITE_CLI_APP_ID", "wiki_node_get_dryrun_test")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "wiki_node_get_dryrun_secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"wiki", "+node-get",
			"--node-token", "PImXw",
			"--dry-run",
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 2)
	require.Empty(t, result.Stdout)
	require.Equal(t, "validation", gjson.Get(result.Stderr, "error.type").String(), result.Stderr)
	require.Equal(t, "invalid_argument", gjson.Get(result.Stderr, "error.subtype").String(), result.Stderr)
	require.Equal(t, "--node-token", gjson.Get(result.Stderr, "error.param").String(), result.Stderr)
	require.Contains(t, gjson.Get(result.Stderr, "error.hint").String(), "complete token", result.Stderr)
}
