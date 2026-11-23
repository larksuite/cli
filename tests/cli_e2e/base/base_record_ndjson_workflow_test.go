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

func TestBaseRecordDefaultNDJSONWorkflow(t *testing.T) {
	clie2e.SkipWithoutTenantAccessToken(t)
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	t.Cleanup(cancel)
	// Check the cleanup verifier's permission before creating tenant resources.
	// A test app without Drive metadata access cannot prove the Base was deleted.
	probe, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args:      []string{"api", "post", "/open-apis/drive/v1/metas/batch_query"},
		DefaultAs: "bot",
		Data:      map[string]any{"request_docs": []map[string]string{{"doc_token": "base_ndjson_cleanup_probe", "doc_type": "bitable"}}},
	})
	require.NoError(t, err)
	if gjson.Get(probe.Stderr, "error.subtype").String() == "app_scope_not_applied" {
		t.Skip("live NDJSON workflow requires Drive cleanup scopes; the configured test app has not applied for them")
	}
	probe.AssertExitCode(t, 0)
	baseToken := createBaseWithRetry(t, ctx, "lark-cli-e2e-ndjson-"+clie2e.GenerateSuffix())
	tableID, _, _ := createTableWithRetry(t, t, ctx, baseToken, "NDJSON Records", `[{"name":"Name","type":"text"}]`, `{"name":"Main","type":"grid"}`)
	created, err := clie2e.RunCmd(ctx, clie2e.Request{DefaultAs: "bot", Args: []string{"base", "+record-batch-create", "--base-token", baseToken, "--table-id", tableID, "--json", `{"create_records":[{"Name":"ndjson-alpha"},{"Name":"ndjson-beta"}]}`}})
	require.NoError(t, err)
	created.AssertExitCode(t, 0)
	recordID := gjson.Get(created.Stdout, "data.record_id_list.0").String()
	require.NotEmpty(t, recordID, created.Stdout)
	for _, command := range []string{"+record-list", "+record-search", "+record-get"} {
		t.Run(command, func(t *testing.T) {
			args := []string{"base", command, "--base-token", baseToken, "--table-id", tableID, "--field-id", "Name"}
			expectedCount := int64(2)
			if command == "+record-search" {
				args = append(args, "--keyword", "ndjson", "--search-field", "Name")
			}
			if command == "+record-get" {
				args = append(args, "--record-id", recordID)
				expectedCount = 1
			}
			dir := t.TempDir()
			result, err := clie2e.RunCmd(ctx, clie2e.Request{Args: args, DefaultAs: "bot", WorkDir: dir})
			require.NoError(t, err)
			result.AssertExitCode(t, 0)
			require.Equal(t, expectedCount, gjson.Get(result.Stdout, "records_count").Int(), result.Stdout)
			require.False(t, gjson.Get(result.Stdout, "has_more").Bool(), result.Stdout)
			if command != "+record-get" {
				require.Equal(t, int64(2000), gjson.Get(result.Stdout, "requested_limit").Int(), result.Stdout)
			}
			raw, err := os.ReadFile(gjson.Get(result.Stdout, "record_file").String())
			require.NoError(t, err)
			rows := strings.Split(strings.TrimSpace(string(raw)), "\n")
			require.Len(t, rows, int(expectedCount))
			for _, row := range rows {
				require.True(t, gjson.Valid(row), row)
				require.NotEmpty(t, gjson.Get(row, "record_id").String(), row)
				require.Contains(t, gjson.Get(row, "Name").String(), "ndjson-")
			}
			_, err = os.Stat(gjson.Get(result.Stdout, "manifest_file").String())
			require.NoError(t, err)
		})
	}
}
