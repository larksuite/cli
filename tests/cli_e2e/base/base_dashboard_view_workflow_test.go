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

// Enable only after the dashboard endpoint supports view create/patch/readback.
// The workflow owns its Base, table, dashboard, and every component it mutates.
func TestBaseDashboardViewWorkflow(t *testing.T) {
	if os.Getenv("LARK_CLI_E2E_VIEW_READY") != "1" {
		t.Skip("set LARK_CLI_E2E_VIEW_READY=1 after dashboard view create, patch and readback are deployed")
	}
	clie2e.SkipWithoutTenantAccessToken(t)
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	t.Cleanup(cancel)
	baseToken := createBaseWithRetry(t, ctx, "lark-cli-e2e-view-"+clie2e.GenerateSuffix())
	tableID, _, _ := createTableWithRetry(t, t, ctx, baseToken, "Tasks", `[{"name":"Title","type":"text"},{"name":"Status","type":"text"},{"name":"Start","type":"datetime"},{"name":"End","type":"datetime"},{"name":"Cover","type":"attachment"}]`, `{"name":"Main","type":"grid"}`)
	run := func(t *testing.T, args ...string) *clie2e.Result {
		t.Helper()
		result, err := clie2e.RunCmd(ctx, clie2e.Request{Args: args, DefaultAs: "bot"})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		return result
	}
	fields := run(t, "base", "+field-list", "--base-token", baseToken, "--table-id", tableID)
	fieldIDs := make(map[string]string)
	for _, field := range gjson.Get(fields.Stdout, "data.fields").Array() {
		fieldIDs[field.Get("name").String()] = field.Get("id").String()
	}
	for _, name := range []string{"Title", "Status", "End"} {
		require.NotEmpty(t, fieldIDs[name], "missing fixture field %s", name)
	}
	dashboard := run(t, "base", "+dashboard-create", "--base-token", baseToken, "--name", "View workflow")
	dashboardID := gjson.Get(dashboard.Stdout, "data.dashboard.dashboard_id").String()
	require.NotEmpty(t, dashboardID)
	t.Cleanup(func() {
		cleanupCtx, stop := clie2e.CleanupContext()
		defer stop()
		result, err := clie2e.RunCmd(cleanupCtx, clie2e.Request{Args: []string{"base", "+dashboard-delete", "--base-token", baseToken, "--dashboard-id", dashboardID, "--yes"}, DefaultAs: "bot"})
		clie2e.ReportCleanupFailure(t, "delete view dashboard", result, err)
	})
	for _, tc := range []struct {
		kind, config string
		patches      []string
	}{
		{"grid", `{"table_name":"Tasks","view_type":"grid","filter":{"conjunction":"and","conditions":[{"field_name":"Status","operator":"is","value":"Doing"}]},"group_by":[{"field_name":"Status","mode":"integrated","sort":{"type":"group","order":"asc"}}],"sort_by":[{"field_name":"End","order":"asc"}],"visible_fields":["Title","Status","End"]}`, []string{
			`{"sort_by":[{"field_name":"End","order":"desc"}]}`,
			`{"visible_fields":["Title","End"]}`,
			`{"group_by":[{"field_name":"Status","mode":"integrated","sort":{"type":"group","order":"desc"}}]}`,
			`{"filter":{"conjunction":"and","conditions":[{"field_name":"Status","operator":"is","value":"Done"}]}}`,
			`{}`, `{"filter":null}`, `{"group_by":[]}`, `{"sort_by":[]}`,
		}},
		{"gantt", `{"table_name":"Tasks","view_type":"gantt"}`, nil},
		{"gallery", `{"table_name":"Tasks","view_type":"gallery"}`, nil},
	} {
		if !t.Run(tc.kind, func(st *testing.T) {
			created := run(st, "base", "+dashboard-block-create", "--base-token", baseToken, "--dashboard-id", dashboardID, "--name", tc.kind+" task view", "--type", "view", "--data-config", tc.config)
			blockID := gjson.Get(created.Stdout, "data.block.block_id").String()
			require.NotEmpty(st, blockID)
			deleted := false
			t.Cleanup(func() {
				if deleted {
					return
				}
				cleanupCtx, stop := clie2e.CleanupContext()
				defer stop()
				result, err := clie2e.RunCmd(cleanupCtx, clie2e.Request{Args: []string{"base", "+dashboard-block-delete", "--base-token", baseToken, "--dashboard-id", dashboardID, "--block-id", blockID, "--yes"}, DefaultAs: "bot"})
				clie2e.ReportCleanupFailure(t, "delete view component", result, err)
			})
			assertReference := func(config gjson.Result) string {
				st.Helper()
				// Existing ID shapes are independent of the string viewType contract.
				tableRef := config.Get("tableId")
				if tableRef.IsArray() {
					require.Len(st, tableRef.Array(), 1)
					tableRef = tableRef.Array()[0]
				}
				require.Equal(st, tableID, tableRef.String())
				require.NotEmpty(st, config.Get("viewId").String())
				require.Equal(st, gjson.String, config.Get("viewType").Type)
				require.Equal(st, tc.kind, config.Get("viewType").String())
				return config.Get("viewId").String()
			}
			read := func() string {
				st.Helper()
				result := run(st, "base", "+dashboard-block-get", "--base-token", baseToken, "--dashboard-id", dashboardID, "--block-id", blockID)
				require.Equal(st, blockID, gjson.Get(result.Stdout, "data.block.block_id").String())
				require.Equal(st, "view", gjson.Get(result.Stdout, "data.block.type").String())
				return assertReference(gjson.Get(result.Stdout, "data.block.data_config"))
			}
			viewID := read()
			if config := gjson.Get(created.Stdout, "data.block.data_config"); config.Exists() {
				require.Equal(st, viewID, assertReference(config))
			}
			expected := gjson.Parse(tc.config).Map()
			assertProperties := func() {
				st.Helper()
				assertField := func(name string, actual gjson.Result) {
					st.Helper()
					require.Equal(st, gjson.String, actual.Type)
					require.Contains(st, []string{name, fieldIDs[name]}, actual.String())
				}
				// Persisted properties have their own read protocol; dashboard GET
				// only supplies the references used by these commands.
				for _, property := range []struct{ input, output, command string }{
					{"filter", "filter", "+view-get-filter"},
					{"group_by", "group", "+view-get-group"},
					{"sort_by", "sort", "+view-get-sort"},
					{"visible_fields", "visible_fields", "+view-get-visible-fields"},
				} {
					want, configured := expected[property.input]
					if !configured {
						continue
					}
					result := run(st, "base", property.command, "--base-token", baseToken, "--table-id", tableID, "--view-id", viewID)
					actual, present := gjson.Get(result.Stdout, "data").Map()[property.output]
					require.True(st, present, "%s response is missing %s", property.command, property.output)
					switch property.input {
					case "filter":
						conditions := want.Get("conditions").Array()
						require.Len(st, actual.Get("conditions").Array(), len(conditions), property.input)
						if len(conditions) > 0 {
							require.Equal(st, want.Get("conjunction").String(), actual.Get("logic").String())
						}
						for i, condition := range conditions {
							// These fixtures use text equality: dashboard "is" reads as "==".
							tuple := actual.Get("conditions").Array()[i].Array()
							require.Len(st, tuple, 3)
							assertField(condition.Get("field_name").String(), tuple[0])
							require.Equal(st, "==", tuple[1].String())
							require.JSONEq(st, condition.Get("value").Raw, tuple[2].Raw)
						}
					case "group_by", "sort_by":
						require.True(st, actual.IsArray() || actual.Type == gjson.Null, property.input)
						require.Len(st, actual.Array(), len(want.Array()), property.input)
						for i, item := range want.Array() {
							got := actual.Array()[i]
							assertField(item.Get("field_name").String(), got.Get("field"))
							order := item.Get("order")
							if property.input == "group_by" {
								order = item.Get("sort.order")
							}
							require.Equal(st, order.String() == "desc", got.Get("desc").Bool(), property.input)
						}
					case "visible_fields":
						require.True(st, actual.IsArray())
						require.Len(st, actual.Array(), len(want.Array()), property.input)
						for i, field := range want.Array() {
							assertField(field.String(), actual.Array()[i])
						}
					}
				}
			}
			assertProperties()
			for _, patch := range tc.patches {
				updated := run(st, "base", "+dashboard-block-update", "--base-token", baseToken, "--dashboard-id", dashboardID, "--block-id", blockID, "--data-config", patch)
				require.Equal(st, viewID, read(), "a configuration patch must preserve the view reference")
				if config := gjson.Get(updated.Stdout, "data.block.data_config"); config.Exists() {
					require.Equal(st, viewID, assertReference(config))
				}
				for key, value := range gjson.Parse(patch).Map() {
					expected[key] = value
				}
				assertProperties()
			}
			removed := run(st, "base", "+dashboard-block-delete", "--base-token", baseToken, "--dashboard-id", dashboardID, "--block-id", blockID, "--yes")
			require.True(st, gjson.Get(removed.Stdout, "data.deleted").Bool())
			require.Equal(st, blockID, gjson.Get(removed.Stdout, "data.block_id").String())
			deleted = true
		}) {
			return
		}
	}
}
