// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package minutes

import (
	"context"
	"net/http"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestMinutesSearchDryRunSupportsUserAndBotIdentity(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("LARKSUITE_CLI_APP_ID", "app")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	for _, identity := range []string{"user", "bot"} {
		t.Run(identity, func(t *testing.T) {
			args := []string{"minutes", "+search", "--query", "roadmap", "--page-size", "5", "--dry-run"}
			if identity == "bot" {
				args = append(args, "--owner-ids", "ou_owner")
			}
			result, err := clie2e.RunCmd(ctx, clie2e.Request{
				Args:      args,
				DefaultAs: identity,
			})
			require.NoError(t, err)
			result.AssertExitCode(t, 0)

			require.Equal(t, identity, clie2e.DryRunGet(result.Stdout, "identity").String(), "stdout:\n%s", result.Stdout)
			require.Equal(t, http.MethodPost, clie2e.DryRunGet(result.Stdout, "api.0.method").String(), "stdout:\n%s", result.Stdout)
			require.Equal(t, "/open-apis/minutes/v1/minutes/search", clie2e.DryRunGet(result.Stdout, "api.0.url").String(), "stdout:\n%s", result.Stdout)
			require.Equal(t, "5", clie2e.DryRunGet(result.Stdout, "api.0.params.page_size").String(), "stdout:\n%s", result.Stdout)
			require.Equal(t, "roadmap", clie2e.DryRunGet(result.Stdout, "api.0.body.query").String(), "stdout:\n%s", result.Stdout)
			if identity == "bot" {
				require.Equal(t, "ou_owner", clie2e.DryRunGet(result.Stdout, "api.0.body.filter.owner_ids.0").String(), "stdout:\n%s", result.Stdout)
			}
		})
	}
}

func TestMinutesSearchDryRunRejectsMeForBotIdentity(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("LARKSUITE_CLI_APP_ID", "app")
	t.Setenv("LARKSUITE_CLI_APP_SECRET", "secret")
	t.Setenv("LARKSUITE_CLI_BRAND", "feishu")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      []string{"minutes", "+search", "--owner-ids", "me", "--dry-run"},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 2)
	require.Equal(t, "invalid_argument", gjson.Get(result.Stderr, "error.subtype").String(), "stderr:\n%s", result.Stderr)
	require.Equal(t, "--owner-ids", gjson.Get(result.Stderr, "error.param").String(), "stderr:\n%s", result.Stderr)
	require.Contains(t, gjson.Get(result.Stderr, "error.hint").String(), "ou_xxx", "stderr:\n%s", result.Stderr)
}
