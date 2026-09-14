// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAppsMCPDryRun(t *testing.T) {
	setAppsDryRunEnv(t)
	for _, tc := range []struct{ command, method, resource string }{
		{"+mcp-get", "GET", "connection"},
		{"+mcp-key-create", "POST", "credential"},
	} {
		t.Run(tc.command, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			result, err := clie2e.RunCmd(ctx, clie2e.Request{Args: []string{"apps", tc.command, "--app-id", "app_example", "--include-secret", "--dry-run"}, DefaultAs: "user"})
			require.NoError(t, err)
			result.AssertExitCode(t, 0)
			assert.Equal(t, tc.method, clie2e.DryRunGet(result.Stdout, "api.0.method").String())
			assert.Equal(t, "/open-apis/spark/v1/apps/app_example/mcp/"+tc.resource, clie2e.DryRunGet(result.Stdout, "api.0.url").String())
			assert.False(t, clie2e.DryRunGet(result.Stdout, "api.0.body").Exists())
			assert.False(t, clie2e.DryRunGet(result.Stdout, "api.0.params").Exists())
			missing, err := clie2e.RunCmd(ctx, clie2e.Request{Args: []string{"apps", tc.command, "--dry-run"}, DefaultAs: "user"})
			require.NoError(t, err)
			missing.AssertExitCode(t, 2)
		})
	}
}
