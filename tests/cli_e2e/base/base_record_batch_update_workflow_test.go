// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestBaseRecordBatchUpdatePerRecordWorkflow(t *testing.T) {
	if os.Getenv("LARKSUITE_CLI_E2E_ENABLE_UPDATE_RECORDS") != "1" {
		t.Skip("update_records is not available in the online backend; set LARKSUITE_CLI_E2E_ENABLE_UPDATE_RECORDS=1 after rollout")
	}

	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	t.Cleanup(cancel)

	baseToken := createBaseWithRetry(t, ctx, "lark-cli-e2e-batch-update-"+clie2e.GenerateSuffix())
	tableID, _, _ := createTableWithRetry(
		t,
		parentT,
		ctx,
		baseToken,
		"Batch Update "+clie2e.GenerateSuffix(),
		`[{"name":"Name","type":"text"},{"name":"Status","type":"select","multiple":false,"options":[{"name":"Open"},{"name":"Done"}]},{"name":"Score","type":"number"}]`,
		`{"name":"Main","type":"grid"}`,
	)

	createResult, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"base", "+record-batch-create",
			"--base-token", baseToken,
			"--table-id", tableID,
			"--json", `{"fields":["Name","Status","Score"],"rows":[["alpha","Open",10],["beta","Open",15]]}`,
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	createResult.AssertExitCode(t, 0)
	createResult.AssertStdoutStatus(t, true)

	firstRecordID := gjson.Get(createResult.Stdout, "data.record_id_list.0").String()
	secondRecordID := gjson.Get(createResult.Stdout, "data.record_id_list.1").String()
	require.NotEmpty(t, firstRecordID, "stdout:\n%s", createResult.Stdout)
	require.NotEmpty(t, secondRecordID, "stdout:\n%s", createResult.Stdout)

	updateBody, err := json.Marshal(map[string]map[string]map[string]any{
		"update_records": {
			firstRecordID:  {"Status": []string{"Done"}},
			secondRecordID: {"Score": 20},
		},
	})
	require.NoError(t, err)

	updateResult, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{
			"base", "+record-batch-update",
			"--base-token", baseToken,
			"--table-id", tableID,
			"--json", string(updateBody),
		},
		DefaultAs: "bot",
	})
	require.NoError(t, err)
	updateResult.AssertExitCode(t, 0)
	updateResult.AssertStdoutStatus(t, true)
	require.ElementsMatch(
		t,
		[]string{firstRecordID, secondRecordID},
		[]string{
			gjson.Get(updateResult.Stdout, "data.record_id_list.0").String(),
			gjson.Get(updateResult.Stdout, "data.record_id_list.1").String(),
		},
		updateResult.Stdout,
	)
	require.Equal(t, "Done", gjson.Get(updateResult.Stdout, "data.update_records."+firstRecordID+".Status.0").String(), updateResult.Stdout)
	require.Equal(t, int64(20), gjson.Get(updateResult.Stdout, "data.update_records."+secondRecordID+".Score").Int(), updateResult.Stdout)

	assertRecordFields := func(recordID, expectedStatus string, expectedScore int64) {
		t.Helper()
		result, runErr := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"base", "+record-get",
				"--base-token", baseToken,
				"--table-id", tableID,
				"--record-id", recordID,
				"--field-id", "Status",
				"--field-id", "Score",
				"--format", "json",
			},
			DefaultAs: "bot",
		})
		require.NoError(t, runErr)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		require.Equal(t, recordID, gjson.Get(result.Stdout, "data.record_id_list.0").String(), result.Stdout)
		require.Equal(t, "Status", gjson.Get(result.Stdout, "data.fields.0").String(), result.Stdout)
		require.Equal(t, "Score", gjson.Get(result.Stdout, "data.fields.1").String(), result.Stdout)
		require.Equal(t, expectedStatus, gjson.Get(result.Stdout, "data.data.0.0.0").String(), result.Stdout)
		require.Equal(t, expectedScore, gjson.Get(result.Stdout, "data.data.0.1").Int(), result.Stdout)
	}

	assertRecordFields(firstRecordID, "Done", 10)
	assertRecordFields(secondRecordID, "Open", 20)
}
