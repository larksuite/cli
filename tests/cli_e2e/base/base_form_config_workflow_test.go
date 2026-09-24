// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"os"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestBaseFormConfigWorkflow(t *testing.T) {
	if os.Getenv("LARK_CLI_E2E_FORM_CONFIG_READY") != "1" {
		t.Skip("set LARK_CLI_E2E_FORM_CONFIG_READY=1 after the form configuration OpenAPI is deployed")
	}
	clie2e.SkipWithoutTenantAccessToken(t)

	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	t.Cleanup(cancel)
	baseToken := createBaseWithRetry(t, ctx, "lark-cli-e2e-form-config-"+clie2e.GenerateSuffix())
	tableID, _, _ := createTableWithRetry(
		t,
		t,
		ctx,
		baseToken,
		"Form config "+clie2e.GenerateSuffix(),
		`[{"name":"Name","type":"text"}]`,
		`{"name":"Main","type":"grid"}`,
	)

	formCreate, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      []string{"base", "+form-create", "--base-token", baseToken, "--table-id", tableID, "--name", "Config form " + clie2e.GenerateSuffix()},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	formCreate.AssertExitCode(t, 0)
	formCreate.AssertStdoutStatus(t, true)
	formID := gjson.Get(formCreate.Stdout, "data.id").String()
	require.NotEmpty(t, formID, formCreate.Stdout)

	commonArgs := []string{"--base-token", baseToken, "--table-id", tableID, "--form-id", formID}
	run := func(command string, extraArgs ...string) *clie2e.Result {
		t.Helper()
		args := append([]string{"base", command}, commonArgs...)
		args = append(args, extraArgs...)
		result, runErr := clie2e.RunCmd(ctx, clie2e.Request{Args: args, DefaultAs: "bot"})
		require.NoError(t, runErr)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		return result
	}

	run("+form-submission-settings-get")
	run("+form-submission-settings-update", "--allow-modify-submission=true")
	submissionSettings := run("+form-submission-settings-get")
	allowModify := gjson.Get(submissionSettings.Stdout, "data.allow_modify_submission")
	require.True(t, allowModify.Exists(), submissionSettings.Stdout)
	require.True(t, allowModify.Bool(), submissionSettings.Stdout)
	run("+form-notifications-get")
	run("+form-notifications-update", "--type", "on-submission", "--enabled=false")
	notifications := run("+form-notifications-get")
	onSubmissionEnabled := gjson.Get(notifications.Stdout, "data.on_submission.enabled")
	require.True(t, onSubmissionEnabled.Exists(), notifications.Stdout)
	require.False(t, onSubmissionEnabled.Bool(), notifications.Stdout)
	run("+form-submit-actions-get")
	run("+form-submit-actions-update", "--type", "redirect", "--enabled=false")
	submitActions := run("+form-submit-actions-get")
	redirectEnabled := gjson.Get(submitActions.Stdout, "data.redirect.enabled")
	require.True(t, redirectEnabled.Exists(), submitActions.Stdout)
	require.False(t, redirectEnabled.Bool(), submitActions.Stdout)
	run("+form-lottery-get")
	run("+form-lottery-action", "--action", "disable")
	run("+form-lottery-get")
}
