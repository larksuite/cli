// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package vc

import (
	"context"
	"net/http"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
)

func TestVCSearchDryRunSupportsUserAndBotIdentity(t *testing.T) {
	setVCDryRunEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	for _, identity := range []string{"user", "bot"} {
		t.Run(identity, func(t *testing.T) {
			result, err := clie2e.RunCmd(ctx, clie2e.Request{
				Args: []string{
					"vc", "+search", "--query", "roadmap", "--page-size", "5",
					"--page-token", "next", "--dry-run",
				},
				DefaultAs: identity,
			})
			require.NoError(t, err)
			result.AssertExitCode(t, 0)

			out := result.Stdout
			require.Equal(t, identity, clie2e.DryRunGet(out, "identity").String(), "stdout:\n%s", out)
			require.Equal(t, http.MethodPost, clie2e.DryRunGet(out, "api.0.method").String(), "stdout:\n%s", out)
			require.Equal(t, "/open-apis/vc/v1/meetings/search", clie2e.DryRunGet(out, "api.0.url").String(), "stdout:\n%s", out)
			require.Equal(t, "roadmap", clie2e.DryRunGet(out, "api.0.body.query").String(), "stdout:\n%s", out)
			require.Equal(t, "5", clie2e.DryRunGet(out, "api.0.params.page_size").String(), "stdout:\n%s", out)
			require.Equal(t, "next", clie2e.DryRunGet(out, "api.0.params.page_token").String(), "stdout:\n%s", out)
		})
	}

	help, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      []string{"vc", "+search", "--help"},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	help.AssertExitCode(t, 0)
	require.Contains(t, help.Stdout, "identity type: user | bot")
}
