// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

const dashboardPushReceiverEnv = "LARK_CLI_E2E_BASE_DASHBOARD_PUSH_RECEIVER"

func skipWithoutDashboardPushLiveFixture(t *testing.T) string {
	t.Helper()
	if os.Getenv("LARK_CLI_E2E_BASE_DASHBOARD_PUSH_READY") != "1" {
		t.Skip("set LARK_CLI_E2E_BASE_DASHBOARD_PUSH_READY=1 after Dashboard image workflow segments are deployed")
	}
	receiver := strings.TrimSpace(os.Getenv(dashboardPushReceiverEnv))
	if receiver == "" {
		t.Skipf("set %s to a tenant-valid ou_ user or oc_ group ID", dashboardPushReceiverEnv)
	}
	if !strings.HasPrefix(receiver, "ou_") && !strings.HasPrefix(receiver, "oc_") {
		t.Fatalf("%s must start with ou_ or oc_", dashboardPushReceiverEnv)
	}
	clie2e.SkipWithoutTenantAccessToken(t)
	return receiver
}

// TestBaseDashboardPushWorkflow creates every owned resource and removes the
// temporary Base at cleanup, which also removes its dashboard and workflow.
// The explicit workflow disable runs first to prevent a scheduled send while
// the remaining resources are being cleaned up.
func TestBaseDashboardPushWorkflow(t *testing.T) {
	receiver := skipWithoutDashboardPushLiveFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	t.Cleanup(cancel)

	suffix := clie2e.GenerateSuffix()
	baseToken := createBaseWithRetry(t, ctx, "lark-cli-e2e-dashboard-push-"+suffix)
	dashboardCreate, err := clie2e.RunCmdWithRetry(ctx, clie2e.Request{
		Args: []string{
			"base", "+dashboard-create",
			"--base-token", baseToken,
			"--name", "Dashboard Push " + suffix,
		},
		DefaultAs: "bot",
	}, clie2e.RetryOptions{})
	require.NoError(t, err)
	dashboardCreate.AssertExitCode(t, 0)
	dashboardCreate.AssertStdoutStatus(t, true)
	dashboardID := gjson.Get(dashboardCreate.Stdout, "data.dashboard.dashboard_id").String()
	if dashboardID == "" {
		dashboardID = gjson.Get(dashboardCreate.Stdout, "data.dashboard.id").String()
	}
	require.NotEmpty(t, dashboardID, dashboardCreate.Stdout)
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := clie2e.CleanupContext()
		defer cleanupCancel()
		result, cleanupErr := clie2e.RunCmd(cleanupCtx, clie2e.Request{
			Args:      []string{"base", "+dashboard-delete", "--base-token", baseToken, "--dashboard-id", dashboardID, "--yes"},
			DefaultAs: "bot",
		})
		clie2e.ReportCleanupFailure(t, "delete dashboard "+dashboardID, result, cleanupErr)
	})

	// The Base timezone is Asia/Shanghai (see createBaseWithRetry). Keep a
	// one-time send comfortably in the future so the fixture never fires while
	// the workflow is being read back and disabled.
	sendAt := time.Now().UTC().Add(25 * time.Hour).In(time.FixedZone("Asia/Shanghai", 8*60*60)).Format("2006-01-02 15:04")
	create, err := clie2e.RunCmdWithRetry(ctx, clie2e.Request{
		Args: []string{
			"base", "+dashboard-push-create",
			"--base-token", baseToken,
			"--dashboard-id", dashboardID,
			"--title", "Dashboard Push " + suffix,
			"--send-at", sendAt,
			"--repeat", "NO_REPEAT",
			"--receiver", receiver,
			"--client-token", "lark-cli-e2e-dashboard-push-" + suffix,
			"--content-mode", "image",
		},
		DefaultAs: "bot",
	}, clie2e.RetryOptions{})
	require.NoError(t, err)
	create.AssertExitCode(t, 0)
	create.AssertStdoutStatus(t, true)
	workflowID := gjson.Get(create.Stdout, "data.workflow_id").String()
	require.True(t, strings.HasPrefix(workflowID, "wkf"), create.Stdout)
	require.True(t, gjson.Get(create.Stdout, "data.created").Bool(), create.Stdout)
	require.True(t, gjson.Get(create.Stdout, "data.enabled").Bool(), create.Stdout)
	require.Equal(t, "confirmed", gjson.Get(create.Stdout, "data.enable_outcome").String(), create.Stdout)

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := clie2e.CleanupContext()
		defer cleanupCancel()
		result, cleanupErr := clie2e.RunCmd(cleanupCtx, clie2e.Request{
			Args:      []string{"base", "+workflow-disable", "--base-token", baseToken, "--workflow-id", workflowID},
			DefaultAs: "bot",
		})
		clie2e.ReportCleanupFailure(t, "disable dashboard push workflow "+workflowID, result, cleanupErr)
	})

	get, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      []string{"base", "+workflow-get", "--base-token", baseToken, "--workflow-id", workflowID},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	get.AssertExitCode(t, 0)
	get.AssertStdoutStatus(t, true)
	require.Equal(t, workflowID, gjson.Get(get.Stdout, "data.workflow_id").String(), get.Stdout)
	require.Equal(t, "Dashboard Push "+suffix, gjson.Get(get.Stdout, "data.title").String(), get.Stdout)
	require.Len(t, gjson.Get(get.Stdout, "data.steps").Array(), 2, get.Stdout)
	require.Equal(t, "TimerTrigger", gjson.Get(get.Stdout, "data.steps.0.type").String(), get.Stdout)
	require.Equal(t, "NO_REPEAT", gjson.Get(get.Stdout, "data.steps.0.data.rule").String(), get.Stdout)
	require.Equal(t, sendAt, gjson.Get(get.Stdout, "data.steps.0.data.start_time").String(), get.Stdout)
	require.False(t, gjson.Get(get.Stdout, "data.steps.0.data.is_never_end").Bool(), get.Stdout)
	require.Equal(t, "LarkMessageAction", gjson.Get(get.Stdout, "data.steps.1.type").String(), get.Stdout)
	require.Equal(t, receiver, gjson.Get(get.Stdout, "data.steps.1.data.receiver.0.value.id").String(), get.Stdout)
	require.Equal(t, "$.dashboard.image", gjson.Get(get.Stdout, "data.steps.1.data.content.0.value").String(), get.Stdout)
	require.Equal(t, dashboardID, gjson.Get(get.Stdout, "data.steps.1.data.content.0.extra_info.dashboard_name").String(), get.Stdout)
}
