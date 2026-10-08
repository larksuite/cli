// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"strings"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAppsReleaseCreateDryRun pins the public create-release request contract:
// app_id belongs in the path, a supplied reason is forwarded verbatim as
// apply_reason, and omitted optional fields stay omitted. Validation failures
// must stop before a request plan is emitted.
func TestAppsReleaseCreateDryRun(t *testing.T) {
	setAppsDryRunEnv(t)

	run := func(t *testing.T, args ...string) *clie2e.Result {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)
		result, err := clie2e.RunCmd(ctx, clie2e.Request{Args: args, DefaultAs: "user"})
		require.NoError(t, err)
		return result
	}

	t.Run("ForwardsConfirmedReasonAndTrimmedRoutingFields", func(t *testing.T) {
		reason := `  deploy release approval: $HOME; $(printf no)  `
		result := run(t,
			"apps", "+release-create",
			"--app-id", "  app_x  ",
			"--branch", "  sprint/default  ",
			"--apply-reason", reason,
			"--dry-run",
		)
		result.AssertExitCode(t, 0)

		assert.Equal(t, "POST", clie2e.DryRunGet(result.Stdout, "api.0.method").String())
		assert.Equal(t, "/open-apis/spark/v1/apps/app_x/releases", clie2e.DryRunGet(result.Stdout, "api.0.url").String())
		assert.Equal(t, "sprint/default", clie2e.DryRunGet(result.Stdout, "api.0.body.branch").String())
		assert.Equal(t, reason, clie2e.DryRunGet(result.Stdout, "api.0.body.apply_reason").String())
		assert.False(t, clie2e.DryRunGet(result.Stdout, "api.0.body.applyReason").Exists())
		assert.False(t, clie2e.DryRunGet(result.Stdout, "api.0.body.app_id").Exists())
	})

	t.Run("OmitsEmptyBranch", func(t *testing.T) {
		result := run(t,
			"apps", "+release-create",
			"--app-id", "app_x",
			"--apply-reason", "deploy the reviewed change",
			"--dry-run",
		)
		result.AssertExitCode(t, 0)

		assert.Equal(t, "deploy the reviewed change", clie2e.DryRunGet(result.Stdout, "api.0.body.apply_reason").String())
		assert.False(t, clie2e.DryRunGet(result.Stdout, "api.0.body.branch").Exists())
	})

	t.Run("OmitsReasonForHTMLCompatibleFlow", func(t *testing.T) {
		result := run(t,
			"apps", "+release-create",
			"--app-id", "app_x",
			"--dry-run",
		)
		result.AssertExitCode(t, 0)
		assert.Equal(t, "POST", clie2e.DryRunGet(result.Stdout, "api.0.method").String())
		assert.False(t, clie2e.DryRunGet(result.Stdout, "api.0.body.apply_reason").Exists())
		assert.False(t, clie2e.DryRunGet(result.Stdout, "api.0.body.branch").Exists())
	})

	t.Run("RejectsBlankReason", func(t *testing.T) {
		result := run(t,
			"apps", "+release-create",
			"--app-id", "app_x",
			"--apply-reason", " \t ",
			"--dry-run",
		)
		result.AssertExitCode(t, 2)
		assert.Contains(t, validateErrorMessage(result), "--apply-reason must not be empty")
		assert.False(t, clie2e.DryRunGet(result.Stdout, "api.0.method").Exists())
	})

	t.Run("RejectsDangerousUnicode", func(t *testing.T) {
		result := run(t,
			"apps", "+release-create",
			"--app-id", "app_x",
			"--apply-reason", "deploy\u202enow",
			"--dry-run",
		)
		result.AssertExitCode(t, 2)
		assert.Contains(t, validateErrorMessage(result), "must not contain dangerous Unicode characters")
		assert.False(t, clie2e.DryRunGet(result.Stdout, "api.0.method").Exists())
	})

	t.Run("AcceptsExactlyOneThousandCodePoints", func(t *testing.T) {
		reason := strings.Repeat("界", 1000)
		result := run(t,
			"apps", "+release-create",
			"--app-id", "app_x",
			"--apply-reason", reason,
			"--dry-run",
		)
		result.AssertExitCode(t, 0)
		assert.Equal(t, reason, clie2e.DryRunGet(result.Stdout, "api.0.body.apply_reason").String())
	})

	t.Run("RejectsMoreThanOneThousandCodePoints", func(t *testing.T) {
		result := run(t,
			"apps", "+release-create",
			"--app-id", "app_x",
			"--apply-reason", strings.Repeat("界", 1001),
			"--dry-run",
		)
		result.AssertExitCode(t, 2)
		assert.Contains(t, validateErrorMessage(result), "must be at most 1000 characters")
		assert.False(t, clie2e.DryRunGet(result.Stdout, "api.0.method").Exists())
	})
}
