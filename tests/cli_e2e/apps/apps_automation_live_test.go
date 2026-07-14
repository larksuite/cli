// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAppsAutomationLiveWorkflow drives the full automation trigger
// lifecycle against the real backend (spark/v1) to catch regressions the
// dry-run tests cannot: URL/method path resolution, response envelope shape,
// enum name mapping (kebab->snake in the request, snake->stable in the
// response), and the redaction contract on read paths against the real
// response wrapping.
//
// Gating:
//   - Opt-in via LARK_CLI_AUTOMATION_LIVE_APP_ID (a miaoda app_id the
//     current user identity has spark:app:write on). No default: automation
//     resources cannot be deleted (backend has no delete verb; see
//     Story-7 / product PRD), so this test intentionally does NOT fall back
//     to a hardcoded app, to keep resource accumulation opt-in.
//   - Requires a live user token (SkipWithoutUserToken).
//
// Cleanup posture:
//   - Backend has no delete API for triggers; every trigger created here
//     survives the test. The lifecycle ends with +automation-disable so the
//     trigger cannot fire, matching the "left disabled" semantic used
//     elsewhere in the family.
//   - Names are prefixed `_e2e_<epoch>` so accumulated debris is easy to
//     recognize and sweep manually if it approaches the 50-per-app cap
//     (Error-006). The test does not create a fresh app per run because
//     +apps-create is a separately-scoped write and would compound the
//     resource cost.
func TestAppsAutomationLiveWorkflow(t *testing.T) {
	appID := strings.TrimSpace(os.Getenv("LARK_CLI_AUTOMATION_LIVE_APP_ID"))
	if appID == "" {
		t.Skip("skipped: set LARK_CLI_AUTOMATION_LIVE_APP_ID to a miaoda app_id the current user has spark:app:write on to run this test")
	}
	clie2e.SkipWithoutUserToken(t)

	// Unique-per-run trigger name so re-runs don't collide on the
	// ErrNameAlreadyExists / ErrDuplicateTrigger surface. `_e2e_` prefix
	// tags this as test debris for the manual sweep.
	name := fmt.Sprintf("_e2e_cron_%d", time.Now().Unix())

	t.Run("Create_Cron", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		t.Cleanup(cancel)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"apps", "+automation-create",
				"--app-id", appID,
				"--name", name,
				"--trigger-type", "cron",
				"--cron", "0 9 * * *",
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)

		// The read path wraps the trigger under `data.trigger` (verified on
		// the real backend; the redactWebhookToken helper depends on this).
		assert.Equal(t, name, clie2e.DryRunGet(result.Stdout, "trigger.name").String())
		assert.Equal(t, "cron", clie2e.DryRunGet(result.Stdout, "trigger.trigger_type").String())
		// Rule-3-2: new triggers are created disabled.
		assert.Equal(t, "disabled", clie2e.DryRunGet(result.Stdout, "trigger.status").String())
		assert.Equal(t, "0 9 * * *", clie2e.DryRunGet(result.Stdout, "trigger.trigger_condition.cron").String())
		// Rule-3-1: --timezone omitted, CLI supplies Asia/Shanghai default.
		assert.Equal(t, "Asia/Shanghai", clie2e.DryRunGet(result.Stdout, "trigger.trigger_condition.timezone").String())
	})

	t.Run("Get_ReturnsFullConfig", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		t.Cleanup(cancel)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"apps", "+automation-get",
				"--app-id", appID,
				"--name", name,
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		assert.Equal(t, name, clie2e.DryRunGet(result.Stdout, "trigger.name").String())
		assert.Equal(t, "disabled", clie2e.DryRunGet(result.Stdout, "trigger.status").String())
	})

	t.Run("List_IncludesCreatedTrigger", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		t.Cleanup(cancel)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"apps", "+automation-list",
				"--app-id", appID,
				"--all",
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		// Confirm the just-created trigger appears in the collection. Use
		// a substring check against the raw stdout instead of matching a
		// specific array index: --all aggregates pages so the position is
		// not stable across runs.
		assert.Contains(t, result.Stdout, name)
	})

	t.Run("Update_PatchesCronOnly", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		t.Cleanup(cancel)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"apps", "+automation-update",
				"--app-id", appID,
				"--name", name,
				"--trigger-type", "cron",
				"--cron", "0 10 * * *",
				"--yes",
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		assert.Equal(t, "0 10 * * *", clie2e.DryRunGet(result.Stdout, "trigger.trigger_condition.cron").String())
	})

	t.Run("Enable_ThenGetShowsEnabled", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		t.Cleanup(cancel)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"apps", "+automation-enable",
				"--app-id", appID,
				"--name", name,
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		// Status endpoint returns {"success": true}; verify via a fresh get.
		assert.True(t, clie2e.DryRunGet(result.Stdout, "success").Bool())

		verifyCtx, verifyCancel := context.WithTimeout(context.Background(), 60*time.Second)
		t.Cleanup(verifyCancel)

		verify, err := clie2e.RunCmd(verifyCtx, clie2e.Request{
			Args: []string{
				"apps", "+automation-get",
				"--app-id", appID,
				"--name", name,
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		verify.AssertExitCode(t, 0)
		assert.Equal(t, "enabled", clie2e.DryRunGet(verify.Stdout, "trigger.status").String())
	})

	t.Run("Disable_LeavesTriggerNonFiring", func(t *testing.T) {
		// This is the cleanup step. Backend has no delete API, so leaving
		// the trigger disabled is the safest terminal state.
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		t.Cleanup(cancel)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"apps", "+automation-disable",
				"--app-id", appID,
				"--name", name,
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		assert.True(t, clie2e.DryRunGet(result.Stdout, "success").Bool())

		verifyCtx, verifyCancel := context.WithTimeout(context.Background(), 60*time.Second)
		t.Cleanup(verifyCancel)

		verify, err := clie2e.RunCmd(verifyCtx, clie2e.Request{
			Args: []string{
				"apps", "+automation-get",
				"--app-id", appID,
				"--name", name,
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		verify.AssertExitCode(t, 0)
		assert.Equal(t, "disabled", clie2e.DryRunGet(verify.Stdout, "trigger.status").String())
	})
}

// TestAppsAutomationLiveWebhookRedaction pins Rule-2-2 against a real backend
// response: the CLI must scrub plaintext token from get/list output even when
// the token is enabled. Creates a webhook trigger, enables its bearer token
// (which is the only path that surfaces plaintext, once, on stderr), then
// re-reads via +automation-get and asserts the plaintext token does NOT
// appear in stdout.
//
// Gated identically to TestAppsAutomationLiveWorkflow. Trigger left in
// "token disabled" state so subsequent runs of the token-lifecycle are
// idempotent-ish (backend allows re-enable to rotate).
func TestAppsAutomationLiveWebhookRedaction(t *testing.T) {
	appID := strings.TrimSpace(os.Getenv("LARK_CLI_AUTOMATION_LIVE_APP_ID"))
	if appID == "" {
		t.Skip("skipped: set LARK_CLI_AUTOMATION_LIVE_APP_ID to run this test")
	}
	clie2e.SkipWithoutUserToken(t)

	name := fmt.Sprintf("_e2e_hook_%d", time.Now().Unix())

	t.Run("Create_Webhook", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		t.Cleanup(cancel)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"apps", "+automation-create",
				"--app-id", appID,
				"--name", name,
				"--trigger-type", "webhook",
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		assert.Equal(t, "webhook", clie2e.DryRunGet(result.Stdout, "trigger.trigger_type").String())
		// Rule-5-1: preview and runtime URLs both present on create.
		assert.NotEmpty(t, clie2e.DryRunGet(result.Stdout, "trigger.trigger_condition.preview_url").String())
		assert.NotEmpty(t, clie2e.DryRunGet(result.Stdout, "trigger.trigger_condition.runtime_url").String())
	})

	var plaintextToken string
	t.Run("EnableToken_SurfacesPlaintextOnce", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		t.Cleanup(cancel)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"apps", "+automation-update",
				"--app-id", appID,
				"--name", name,
				"--enable-token",
				"--yes",
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		// Rule-5-3: plaintext token appears in the create/enable response
		// once, with a one-time stderr warning.
		plaintextToken = clie2e.DryRunGet(result.Stdout, "token_value").String()
		require.NotEmpty(t, plaintextToken, "enable-token must surface plaintext token once")
		assert.Contains(t, result.Stderr, "shown only once")
	})

	t.Run("Get_RedactsPlaintextToken", func(t *testing.T) {
		// Rule-2-2: the plaintext token must NOT reappear on any read path.
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		t.Cleanup(cancel)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"apps", "+automation-get",
				"--app-id", appID,
				"--name", name,
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		require.NotEmpty(t, plaintextToken, "prior sub-test must have captured the plaintext token")
		assert.NotContains(t, result.Stdout, plaintextToken,
			"get must never re-surface plaintext bearer token")
		// token_enabled must still be visible so operators know the token
		// is active without seeing its value.
		assert.True(t, clie2e.DryRunGet(result.Stdout, "trigger.trigger_condition.token_enabled").Bool())
	})

	t.Run("DisableToken_Cleanup", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		t.Cleanup(cancel)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"apps", "+automation-update",
				"--app-id", appID,
				"--name", name,
				"--disable-token",
				"--yes",
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
	})
}
