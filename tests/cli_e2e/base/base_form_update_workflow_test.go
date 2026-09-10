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

func TestBaseFormUpdateDisplayModeWorkflow(t *testing.T) {
	if os.Getenv("LARK_CLI_E2E_BASE_FORM_DISPLAY_MODE_READY") != "1" {
		t.Skip("set LARK_CLI_E2E_BASE_FORM_DISPLAY_MODE_READY=1 after the form display-mode OpenAPI is deployed")
	}
	clie2e.SkipWithoutTenantAccessToken(t)
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	t.Cleanup(cancel)

	suffix := clie2e.GenerateSuffix()
	baseToken := createBaseWithRetry(t, ctx, "lark-cli-e2e-form-display-mode-"+suffix)
	tableID, _, _ := createTableWithRetry(
		t,
		parentT,
		ctx,
		baseToken,
		"Form Display Mode "+suffix,
		`[{"name":"Question","type":"text"}]`,
		`{"name":"Main","type":"grid"}`,
	)

	createResult, err := clie2e.RunCmdWithRetry(ctx, clie2e.Request{
		Args: []string{
			"base", "+form-create",
			"--base-token", baseToken,
			"--table-id", tableID,
			"--name", "Display Mode " + suffix,
		},
		DefaultAs: "bot",
	}, clie2e.RetryOptions{})
	require.NoError(t, err)
	createResult.AssertExitCode(t, 0)
	createResult.AssertStdoutStatus(t, true)
	formID := gjson.Get(createResult.Stdout, "data.id").String()
	if formID == "" {
		formID = gjson.Get(createResult.Stdout, "data.form_id").String()
	}
	require.NotEmpty(t, formID, "stdout:\n%s", createResult.Stdout)

	for _, tt := range []struct {
		mode string
		want int64
	}{
		{mode: "step", want: 2},
		{mode: "list", want: 1},
	} {
		t.Run(tt.mode, func(t *testing.T) {
			updateResult, runErr := clie2e.RunCmdWithRetry(ctx, clie2e.Request{
				Args: []string{
					"base", "+form-update",
					"--base-token", baseToken,
					"--table-id", tableID,
					"--form-id", formID,
					"--display-mode", tt.mode,
				},
				DefaultAs: "bot",
			}, clie2e.RetryOptions{})
			require.NoError(t, runErr)
			updateResult.AssertExitCode(t, 0)
			updateResult.AssertStdoutStatus(t, true)
			require.Equal(t, tt.want, gjson.Get(updateResult.Stdout, "data.display_mode").Int(), "stdout:\n%s", updateResult.Stdout)

			var lastGet *clie2e.Result
			require.Eventually(t, func() bool {
				lastGet, runErr = clie2e.RunCmd(ctx, clie2e.Request{
					Args: []string{
						"base", "+form-get",
						"--base-token", baseToken,
						"--table-id", tableID,
						"--form-id", formID,
					},
					DefaultAs: "bot",
				})
				return runErr == nil && lastGet.ExitCode == 0 && gjson.Get(lastGet.Stdout, "data.display_mode").Int() == tt.want
			}, 30*time.Second, time.Second, "form display_mode did not become %d; last result=%+v err=%v", tt.want, lastGet, runErr)
		})
	}
}
