// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package wiki

import (
	"context"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWikiMemberAddDryRun(t *testing.T) {
	setWikiNodeCreateDryRunEnv(t)

	t.Run("SupportsAppIDMemberType", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"wiki", "+member-add",
				"--space-id", "space_42",
				"--member-id", "cli_app_123",
				"--member-type", "appid",
				"--member-role", "member",
				"--dry-run",
			},
			DefaultAs: "bot",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)

		assert.Equal(t, "POST", clie2e.DryRunGet(result.Stdout, "api.0.method").String())
		assert.Equal(t, "/open-apis/wiki/v2/spaces/space_42/members", clie2e.DryRunGet(result.Stdout, "api.0.url").String())
		assert.Equal(t, "cli_app_123", clie2e.DryRunGet(result.Stdout, "api.0.body.member_id").String())
		assert.Equal(t, "appid", clie2e.DryRunGet(result.Stdout, "api.0.body.member_type").String())
		assert.Equal(t, "member", clie2e.DryRunGet(result.Stdout, "api.0.body.member_role").String())
	})
}
