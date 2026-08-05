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
			result, err := clie2e.RunCmd(ctx, clie2e.Request{
				Args:      []string{"minutes", "+search", "--query", "roadmap", "--page-size", "5", "--dry-run"},
				DefaultAs: identity,
			})
			require.NoError(t, err)
			result.AssertExitCode(t, 0)

			require.Equal(t, http.MethodPost, clie2e.DryRunGet(result.Stdout, "api.0.method").String(), "stdout:\n%s", result.Stdout)
			require.Equal(t, "/open-apis/minutes/v1/minutes/search", clie2e.DryRunGet(result.Stdout, "api.0.url").String(), "stdout:\n%s", result.Stdout)
			require.Equal(t, "5", clie2e.DryRunGet(result.Stdout, "api.0.params.page_size").String(), "stdout:\n%s", result.Stdout)
			require.Equal(t, "roadmap", clie2e.DryRunGet(result.Stdout, "api.0.body.query").String(), "stdout:\n%s", result.Stdout)
		})
	}
}
