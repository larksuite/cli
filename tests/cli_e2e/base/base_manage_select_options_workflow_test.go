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

func TestBaseManageSelectOptionsWorkflow(t *testing.T) {
	if os.Getenv("LARK_CLI_E2E_BASE_MANAGE_SELECT_OPTIONS_READY") != "1" {
		t.Skip("set LARK_CLI_E2E_BASE_MANAGE_SELECT_OPTIONS_READY=1 after manage-select-options OpenAPI is deployed")
	}
	clie2e.SkipWithoutTenantAccessToken(t)

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	t.Cleanup(cancel)
	baseToken := createBaseWithRetry(t, ctx, "lark-cli-e2e-manage-options-"+clie2e.GenerateSuffix())
	tableID, _, _ := createTableWithRetry(t, t, ctx, baseToken, "Options "+clie2e.GenerateSuffix(),
		`[{"name":"Name","type":"text"},{"name":"Status","type":"select","multiple":false,"options":[{"name":"Todo"},{"name":"Old"}]}]`,
		`{"name":"Main","type":"grid"}`)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"base", "+manage-select-options", "--base-token", baseToken, "--table-id", tableID, "--field-id", "Status",
			"--json", `{"operations":[{"operation":"add","name":"Done"},{"operation":"rename","name":"Todo","new_name":"Doing"},{"operation":"delete","name":"Old"},{"operation":"delete","name":"Missing"}]}`,
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	result.AssertStdoutStatus(t, true)
	require.True(t, gjson.Get(result.Stdout, "data.results.0.success").Bool(), result.Stdout)
	require.True(t, gjson.Get(result.Stdout, "data.results.1.success").Bool(), result.Stdout)
	require.True(t, gjson.Get(result.Stdout, "data.results.2.success").Bool(), result.Stdout)
	require.False(t, gjson.Get(result.Stdout, "data.results.3.success").Bool(), result.Stdout)
	require.NotEmpty(t, gjson.Get(result.Stdout, "data.results.3.error_message").String(), result.Stdout)

	require.Eventually(t, func() bool {
		readback, readErr := clie2e.RunCmd(ctx, clie2e.Request{
			Args:      []string{"base", "+field-search-options", "--base-token", baseToken, "--table-id", tableID, "--field-id", "Status", "--limit", "200"},
			DefaultAs: "bot",
		})
		if readErr != nil || readback.ExitCode != 0 {
			return false
		}
		names := gjson.Get(readback.Stdout, "data.options.#.name").Array()
		return len(names) == 2 && names[0].String() == "Doing" && names[1].String() == "Done"
	}, 20*time.Second, time.Second)
}
