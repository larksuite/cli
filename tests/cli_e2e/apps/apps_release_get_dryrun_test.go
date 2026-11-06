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

// TestAppsReleaseGetDryRun pins the request plan for a numeric release id and the
// local rejection of values the gateway can never accept (it types the release id
// as a 64-bit integer).
func TestAppsReleaseGetDryRun(t *testing.T) {
	setAppsDryRunEnv(t)

	t.Run("NumericReleaseID", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"apps", "+release-get", "--app-id", "app_x", "--release-id", "7712345678901234567", "--dry-run"},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		assert.Equal(t, "GET", clie2e.DryRunGet(result.Stdout, "api.0.method").String())
		assert.Equal(t, "/open-apis/spark/v1/apps/app_x/releases/7712345678901234567",
			clie2e.DryRunGet(result.Stdout, "api.0.url").String())
	})

	for _, bad := range []string{"latest", `"7712345678901234567"`, "release_yyy", "9999999999999999999"} {
		t.Run("Rejects/"+bad, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			result, err := clie2e.RunCmd(ctx, clie2e.Request{
				Args:      []string{"apps", "+release-get", "--app-id", "app_x", "--release-id", bad, "--dry-run"},
				DefaultAs: "user",
			})
			require.NoError(t, err)
			assert.Equal(t, 2, result.ExitCode, "stderr:\n%s", result.Stderr)
			assert.Equal(t, "validation", errEnvelopeField(result, "error.type"), "stderr:\n%s", result.Stderr)
			assert.Equal(t, "--release-id", errEnvelopeField(result, "error.param"), "stderr:\n%s", result.Stderr)
			assert.Contains(t, errEnvelopeField(result, "error.hint"), "+release-list", "stderr:\n%s", result.Stderr)
		})
	}
}
