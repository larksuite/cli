// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestBaseContactFieldEnrichmentWorkflow(t *testing.T) {
	clie2e.SkipWithoutTenantAccessToken(t)
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	t.Cleanup(cancel)

	baseToken := createBaseWithRetry(t, ctx, "lark-cli-e2e-contact-enrichment-"+clie2e.GenerateSuffix())
	tableResult, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"base", "+table-create",
			"--base-token", baseToken,
			"--name", "Contact Enrichment",
			"--fields", `[{"name":"Name","type":"text"},{"name":"People","type":"user","multiple":false}]`,
			"--view", `{"name":"Main","type":"grid"}`,
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	tableResult.AssertExitCode(t, 0)
	tableResult.AssertStdoutStatus(t, true)
	tableID := gjson.Get(tableResult.Stdout, "data.table.id").String()
	require.NotEmpty(t, tableID, tableResult.Stdout)

	createResult, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"base", "+contact-field-enrichment-create",
			"--base-token", baseToken,
			"--table-id", tableID,
			"--json", `{"enrichments":[{"source_field":"People","info_type":"department"},{"source_field":"People","info_type":"work_email"}]}`,
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	createResult.AssertExitCode(t, 0)
	createResult.AssertStdoutStatus(t, true)
	require.Equal(t, int64(2), gjson.Get(createResult.Stdout, "data.total").Int(), createResult.Stdout)
	require.Len(t, gjson.Get(createResult.Stdout, "data.enrichments").Array(), 2, createResult.Stdout)

	listResult, err := clie2e.RunCmdWithRetry(ctx, clie2e.Request{
		Args: []string{
			"base", "+contact-field-enrichment-list",
			"--base-token", baseToken,
			"--table-id", tableID,
		},
		DefaultAs: "bot",
	}, clie2e.RetryOptions{})
	require.NoError(t, err)
	listResult.AssertExitCode(t, 0)
	listResult.AssertStdoutStatus(t, true)
	require.Equal(t, int64(2), gjson.Get(listResult.Stdout, "data.total").Int(), listResult.Stdout)
	require.NotEmpty(t, gjson.Get(listResult.Stdout, `data.enrichments.#(info_type=="department").field.id`).String(), listResult.Stdout)
	require.NotEmpty(t, gjson.Get(listResult.Stdout, `data.enrichments.#(info_type=="work_email").field.id`).String(), listResult.Stdout)
}
