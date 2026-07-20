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

// The automation shortcuts (+automation-list / get / create / update /
// enable / disable) live under the same spark/v1 namespace as the rest of
// the apps domain and are UAT-only (Scopes: spark:app:{read,write},
// AuthTypes: [user]). These dry-run tests pin the request shape (method,
// URL, body/params) so a schema drift is caught before it reaches a real
// backend, and pin the Validate-stage rejection surface for the type / flag
// guards that make the write shortcuts safe to expose to Agents.

const (
	automationDryRunAppID   = "app_dryrun_x"
	automationDryRunTrigger = "t1"
)

// TestAppsAutomationListDryRun exercises the read collection path. The
// list command supports two orthogonal query features (page cursor +
// server-side --trigger-type filter), so both are pinned here.
func TestAppsAutomationListDryRun(t *testing.T) {
	setAppsDryRunEnv(t)

	t.Run("HappyPath_MinimalArgs", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"apps", "+automation-list",
				"--app-id", automationDryRunAppID,
				"--dry-run",
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)

		assert.Equal(t, "GET", clie2e.DryRunGet(result.Stdout, "api.0.method").String())
		assert.Equal(t,
			"/open-apis/spark/v1/apps/"+automationDryRunAppID+"/triggers",
			clie2e.DryRunGet(result.Stdout, "api.0.url").String())
		// No filter / no page-token → params object omitted, not sent empty.
		assert.False(t, clie2e.DryRunGet(result.Stdout, "api.0.params.trigger_type").Exists())
		assert.False(t, clie2e.DryRunGet(result.Stdout, "api.0.params.page_token").Exists())
	})

	t.Run("TriggerTypeFilter_PushedDown", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"apps", "+automation-list",
				"--app-id", automationDryRunAppID,
				"--trigger-type", "record-change",
				"--page-token", "cursor-abc",
				"--dry-run",
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)

		// --trigger-type is CLI-facing kebab-case; the backend expects the
		// snake_case wire form. Use "record-change" (not "webhook", which is
		// identical in both forms) so a raw-pass-through regression fails
		// this assertion.
		assert.Equal(t, "record_change", clie2e.DryRunGet(result.Stdout, "api.0.params.trigger_type").String())
		assert.Equal(t, "cursor-abc", clie2e.DryRunGet(result.Stdout, "api.0.params.page_token").String())
	})

	t.Run("RejectsInvalidTriggerType", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"apps", "+automation-list",
				"--app-id", automationDryRunAppID,
				"--trigger-type", "bogus",
				"--dry-run",
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 2)
		msg := validateErrorMessage(result)
		assert.Contains(t, msg, "--trigger-type")
	})
}

// TestAppsAutomationGetDryRun pins the singular read path — GET on the item
// URL, no body, no params.
func TestAppsAutomationGetDryRun(t *testing.T) {
	setAppsDryRunEnv(t)

	t.Run("HappyPath", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"apps", "+automation-get",
				"--app-id", automationDryRunAppID,
				"--name", automationDryRunTrigger,
				"--dry-run",
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)

		assert.Equal(t, "GET", clie2e.DryRunGet(result.Stdout, "api.0.method").String())
		assert.Equal(t,
			"/open-apis/spark/v1/apps/"+automationDryRunAppID+"/triggers/"+automationDryRunTrigger,
			clie2e.DryRunGet(result.Stdout, "api.0.url").String())
	})

	t.Run("RejectsMissingName", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"apps", "+automation-get",
				"--app-id", automationDryRunAppID,
				"--dry-run",
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 2)
		// Cobra's Required:true fires first with a canonical message; assert on
		// the flag name substring rather than a specific wording.
		assert.Contains(t, validateErrorMessage(result), "name")
	})
}

// TestAppsAutomationCreateDryRun pins the create request shape across all
// four trigger types plus the cross-family guard (F1) that keeps flags from
// one type from silently landing on a trigger of another type.
func TestAppsAutomationCreateDryRun(t *testing.T) {
	setAppsDryRunEnv(t)

	t.Run("Cron_BuildsCronCondition", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"apps", "+automation-create",
				"--app-id", automationDryRunAppID,
				"--name", "daily",
				"--trigger-type", "cron",
				"--cron", "0 9 * * *",
				"--dry-run",
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)

		assert.Equal(t, "POST", clie2e.DryRunGet(result.Stdout, "api.0.method").String())
		assert.Equal(t,
			"/open-apis/spark/v1/apps/"+automationDryRunAppID+"/triggers",
			clie2e.DryRunGet(result.Stdout, "api.0.url").String())
		// trigger_type is CLI-facing kebab, backend-facing snake.
		assert.Equal(t, "cron", clie2e.DryRunGet(result.Stdout, "api.0.body.trigger_type").String())
		assert.Equal(t, "0 9 * * *", clie2e.DryRunGet(result.Stdout, "api.0.body.cron_condition.cron").String())
		// Default timezone applied when --timezone is omitted.
		assert.Equal(t, "Asia/Shanghai", clie2e.DryRunGet(result.Stdout, "api.0.body.cron_condition.timezone").String())
		// The other condition_* keys must not appear.
		assert.False(t, clie2e.DryRunGet(result.Stdout, "api.0.body.webhook_condition").Exists())
		assert.False(t, clie2e.DryRunGet(result.Stdout, "api.0.body.record_change_condition").Exists())
		assert.False(t, clie2e.DryRunGet(result.Stdout, "api.0.body.feishu_approval_condition").Exists())
	})

	t.Run("RecordChange_BuildsRecordChangeCondition", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"apps", "+automation-create",
				"--app-id", automationDryRunAppID,
				"--name", "onUpd",
				"--trigger-type", "record-change",
				"--table", "tbl_1",
				"--event", "update",
				"--fields", `["fld1"]`,
				"--dry-run",
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)

		assert.Equal(t, "record_change", clie2e.DryRunGet(result.Stdout, "api.0.body.trigger_type").String())
		// event is uppercased regardless of input case (backend contract).
		assert.Equal(t, "UPDATE", clie2e.DryRunGet(result.Stdout, "api.0.body.record_change_condition.event").String())
		assert.Equal(t, "tbl_1", clie2e.DryRunGet(result.Stdout, "api.0.body.record_change_condition.table").String())
		assert.Equal(t, "fld1", clie2e.DryRunGet(result.Stdout, "api.0.body.record_change_condition.fields.0").String())
	})

	t.Run("Webhook_BuildsWebhookCondition", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"apps", "+automation-create",
				"--app-id", automationDryRunAppID,
				"--name", "hook",
				"--trigger-type", "webhook",
				"--white-ip-list", `["1.1.1.1","10.0.0.0/8"]`,
				"--dry-run",
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)

		assert.Equal(t, "webhook", clie2e.DryRunGet(result.Stdout, "api.0.body.trigger_type").String())
		assert.Equal(t, "1.1.1.1", clie2e.DryRunGet(result.Stdout, "api.0.body.webhook_condition.white_ip_list.0").String())
		assert.Equal(t, "10.0.0.0/8", clie2e.DryRunGet(result.Stdout, "api.0.body.webhook_condition.white_ip_list.1").String())
	})

	t.Run("FeishuApproval_UppercasesStatus", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"apps", "+automation-create",
				"--app-id", automationDryRunAppID,
				"--name", "apv",
				"--trigger-type", "feishu-approval",
				"--event-type", "approval_instance",
				"--instance-status", "approved",
				"--dry-run",
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)

		assert.Equal(t, "feishu_approval", clie2e.DryRunGet(result.Stdout, "api.0.body.trigger_type").String())
		assert.Equal(t, "approval_instance", clie2e.DryRunGet(result.Stdout, "api.0.body.feishu_approval_condition.event_type").String())
		assert.Equal(t, "APPROVED", clie2e.DryRunGet(result.Stdout, "api.0.body.feishu_approval_condition.status.0").String())
	})

	t.Run("RejectsCrossFamilyFlags", func(t *testing.T) {
		// F1 regression guard: --trigger-type webhook + --cron used to
		// silently drop --cron (buildAutomationCreateBody's switch only
		// visited one branch). Validate now rejects up-front.
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"apps", "+automation-create",
				"--app-id", automationDryRunAppID,
				"--name", "n",
				"--trigger-type", "webhook",
				"--cron", "0 9 * * *",
				"--dry-run",
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 2)
		assert.Contains(t, validateErrorMessage(result), "--cron")
	})

	t.Run("RejectsSub30MinCron", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"apps", "+automation-create",
				"--app-id", automationDryRunAppID,
				"--name", "fast",
				"--trigger-type", "cron",
				"--cron", "*/5 * * * *",
				"--dry-run",
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 2)
		// The Validate error message names the constraint ("30-minute minimum")
		// rather than repeating the flag; the flag name is on params[].name.
		assert.Contains(t, validateErrorMessage(result), "30-minute minimum")
	})
}

// TestAppsAutomationUpdateDryRun pins the update PUT shape plus the four
// webhook action dispatches (--reset-url / --enable-token / --disable-token
// / --reset-token) that share the same +automation-update command.
func TestAppsAutomationUpdateDryRun(t *testing.T) {
	setAppsDryRunEnv(t)

	t.Run("Cron_PatchOnly", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"apps", "+automation-update",
				"--app-id", automationDryRunAppID,
				"--name", automationDryRunTrigger,
				"--trigger-type", "cron",
				"--cron", "0 10 * * *",
				"--yes",
				"--dry-run",
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)

		assert.Equal(t, "PUT", clie2e.DryRunGet(result.Stdout, "api.0.method").String())
		assert.Equal(t,
			"/open-apis/spark/v1/apps/"+automationDryRunAppID+"/triggers/"+automationDryRunTrigger,
			clie2e.DryRunGet(result.Stdout, "api.0.url").String())
		assert.Equal(t, "0 10 * * *", clie2e.DryRunGet(result.Stdout, "api.0.body.cron_condition.cron").String())
		// Update body must not carry trigger_type — it's a partial update; the
		// trigger's type is immutable and lives server-side.
		assert.False(t, clie2e.DryRunGet(result.Stdout, "api.0.body.trigger_type").Exists())
	})

	t.Run("ResetURL_DispatchesToWebhookURLEndpoint", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"apps", "+automation-update",
				"--app-id", automationDryRunAppID,
				"--name", automationDryRunTrigger,
				"--reset-url",
				"--app-env", "preview",
				"--yes",
				"--dry-run",
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)

		assert.Equal(t, "POST", clie2e.DryRunGet(result.Stdout, "api.0.method").String())
		assert.Equal(t,
			"/open-apis/spark/v1/apps/"+automationDryRunAppID+"/triggers/"+automationDryRunTrigger+"/webhook/url/reset",
			clie2e.DryRunGet(result.Stdout, "api.0.url").String())
		assert.Equal(t, "preview", clie2e.DryRunGet(result.Stdout, "api.0.body.app_env").String())
	})

	t.Run("EnableToken_DispatchesToTokenStatusEndpoint", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"apps", "+automation-update",
				"--app-id", automationDryRunAppID,
				"--name", automationDryRunTrigger,
				"--enable-token",
				"--yes",
				"--dry-run",
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)

		assert.Equal(t, "PATCH", clie2e.DryRunGet(result.Stdout, "api.0.method").String())
		assert.Equal(t,
			"/open-apis/spark/v1/apps/"+automationDryRunAppID+"/triggers/"+automationDryRunTrigger+"/webhook/token/status",
			clie2e.DryRunGet(result.Stdout, "api.0.url").String())
		assert.Equal(t, "enabled", clie2e.DryRunGet(result.Stdout, "api.0.body.status").String())
	})

	t.Run("ResetToken_DispatchesToTokenResetEndpoint", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"apps", "+automation-update",
				"--app-id", automationDryRunAppID,
				"--name", automationDryRunTrigger,
				"--reset-token",
				"--yes",
				"--dry-run",
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)

		assert.Equal(t, "POST", clie2e.DryRunGet(result.Stdout, "api.0.method").String())
		assert.Equal(t,
			"/open-apis/spark/v1/apps/"+automationDryRunAppID+"/triggers/"+automationDryRunTrigger+"/webhook/token/reset",
			clie2e.DryRunGet(result.Stdout, "api.0.url").String())
	})

	t.Run("RejectsMultiFamilyConditionMix", func(t *testing.T) {
		// F2 regression guard: --cron + --white-ip-list without
		// --trigger-type used to compose a PUT with both cron_condition
		// AND webhook_condition. A trigger has exactly one type, so this
		// is nonsensical regardless of what backend does with it.
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"apps", "+automation-update",
				"--app-id", automationDryRunAppID,
				"--name", automationDryRunTrigger,
				"--cron", "0 9 * * *",
				"--white-ip-list", `["1.1.1.1"]`,
				"--yes",
				"--dry-run",
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 2)
		assert.Contains(t, validateErrorMessage(result), "--trigger-type")
	})

	t.Run("RejectsMutuallyExclusiveWebhookActions", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"apps", "+automation-update",
				"--app-id", automationDryRunAppID,
				"--name", automationDryRunTrigger,
				"--reset-url",
				"--reset-token",
				"--yes",
				"--dry-run",
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 2)
		assert.Contains(t, validateErrorMessage(result), "--reset-url")
	})

	t.Run("RejectsInvalidAppEnvValue", func(t *testing.T) {
		// --app-env value validation moved into Validate so --dry-run and
		// Execute agree; the earlier behavior printed a body preview with
		// app_env: "invalid" that a real invocation would reject.
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"apps", "+automation-update",
				"--app-id", automationDryRunAppID,
				"--name", automationDryRunTrigger,
				"--reset-url",
				"--app-env", "invalid",
				"--yes",
				"--dry-run",
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 2)
		assert.Contains(t, validateErrorMessage(result), "preview or runtime")
	})

	t.Run("RejectsAppEnvWithoutResetURL", func(t *testing.T) {
		// --app-env only pairs with --reset-url. Under any other webhook
		// action or a condition update it was silently dropped; --dry-run
		// happily printed the request that DID reach the backend, without
		// the flag.
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"apps", "+automation-update",
				"--app-id", automationDryRunAppID,
				"--name", automationDryRunTrigger,
				"--enable-token",
				"--app-env", "preview",
				"--yes",
				"--dry-run",
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 2)
		assert.Contains(t, validateErrorMessage(result), "--reset-url")
	})
}

// TestAppsAutomationEnableDisableDryRun pins the enable/disable dispatch to
// the shared status endpoint (PATCH parent resource with status body). The
// two shortcuts share runAutomationStatus; DryRun paths are separate closures
// that coverage tracks independently.
func TestAppsAutomationEnableDisableDryRun(t *testing.T) {
	setAppsDryRunEnv(t)

	t.Run("Enable", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"apps", "+automation-enable",
				"--app-id", automationDryRunAppID,
				"--name", automationDryRunTrigger,
				"--dry-run",
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)

		assert.Equal(t, "PATCH", clie2e.DryRunGet(result.Stdout, "api.0.method").String())
		assert.Equal(t,
			"/open-apis/spark/v1/apps/"+automationDryRunAppID+"/triggers/"+automationDryRunTrigger,
			clie2e.DryRunGet(result.Stdout, "api.0.url").String())
		assert.Equal(t, "enabled", clie2e.DryRunGet(result.Stdout, "api.0.body.status").String())
	})

	t.Run("Disable", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"apps", "+automation-disable",
				"--app-id", automationDryRunAppID,
				"--name", automationDryRunTrigger,
				"--dry-run",
			},
			DefaultAs: "user",
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)

		assert.Equal(t, "PATCH", clie2e.DryRunGet(result.Stdout, "api.0.method").String())
		assert.Equal(t,
			"/open-apis/spark/v1/apps/"+automationDryRunAppID+"/triggers/"+automationDryRunTrigger,
			clie2e.DryRunGet(result.Stdout, "api.0.url").String())
		assert.Equal(t, "disabled", clie2e.DryRunGet(result.Stdout, "api.0.body.status").String())
	})
}
