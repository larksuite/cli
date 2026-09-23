// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"encoding/csv"
	"encoding/json"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/stretchr/testify/require"
)

func TestSQLQueryOutputFormats(t *testing.T) {
	const row = `{"empty":"","large":9007199254740993,"missing":null,"people":[{"name":"Alice","userId":"ou_example"}]}`
	for _, format := range []string{"json", "pretty", "ndjson", "csv", "table"} {
		t.Run(format, func(t *testing.T) {
			factory, stdout, registry := newExecuteFactory(t)
			registry.Register(&httpmock.Stub{
				Method: "POST", URL: "/open-apis/base/v3/bases/app_x/basesql/query",
				BodyFilter: func(body []byte) bool {
					var request map[string]string
					return json.Unmarshal(body, &request) == nil && len(request) == 1 && request["sql"] == " SELECT rec_id FROM tasks\n"
				},
				Body: json.RawMessage("{\"code\":0,\"data\":{\"rows\":[" + row + "]}}"),
			})
			require.NoError(t, runShortcut(t, BaseSQLQuery, []string{"+sql-query", "--base-token", "app_x", "--sql", " SELECT rec_id FROM tasks\n", "--format", format}, factory, stdout))
			switch format {
			case "json", "pretty":
				var envelope struct {
					OK   bool `json:"ok"`
					Data struct {
						Rows []json.RawMessage `json:"rows"`
					} `json:"data"`
				}
				require.NoError(t, json.Unmarshal(stdout.Bytes(), &envelope))
				require.True(t, envelope.OK)
				require.Len(t, envelope.Data.Rows, 1)
				require.JSONEq(t, row, string(envelope.Data.Rows[0]))
				require.Contains(t, string(envelope.Data.Rows[0]), "9007199254740993")
			case "ndjson":
				require.JSONEq(t, row, strings.TrimSpace(stdout.String()))
				require.Contains(t, stdout.String(), "9007199254740993")
			case "csv":
				records, err := csv.NewReader(strings.NewReader(stdout.String())).ReadAll()
				require.NoError(t, err)
				require.Len(t, records, 2)
				require.Contains(t, records[0], "large")
				require.Contains(t, records[1], "9007199254740993")
				require.Contains(t, records[1], "")
			case "table":
				require.Contains(t, stdout.String(), "9007199254740993")
				require.Contains(t, stdout.String(), "Alice")
			}
		})
	}
}

func TestSQLQueryEmptyRows(t *testing.T) {
	for _, format := range []string{"json", "ndjson"} {
		t.Run(format, func(t *testing.T) {
			factory, stdout, registry := newExecuteFactory(t)
			registry.Register(&httpmock.Stub{Method: "POST", URL: "/basesql/query", Body: json.RawMessage(`{"code":0,"data":{"rows":[]}}`)})
			require.NoError(t, runShortcut(t, BaseSQLQuery, []string{"+sql-query", "--base-token", "app_x", "--sql", "SELECT rec_id FROM tasks", "--format", format}, factory, stdout))
			if format == "ndjson" {
				require.Empty(t, stdout.String())
			} else {
				var envelope struct {
					Data struct {
						Rows []any `json:"rows"`
					} `json:"data"`
				}
				require.NoError(t, json.Unmarshal(stdout.Bytes(), &envelope))
				require.NotNil(t, envelope.Data.Rows)
				require.Empty(t, envelope.Data.Rows)
			}
		})
	}
}

func TestSQLCatalogMetadataAndProjection(t *testing.T) {
	const data = `{"supportBaseSQLExecute":false,"tables":[{"baseToken":"app_x","id":"tbl_tasks","name":"Tasks","fields":{"Links":{"sqlType":{"kind":"COLLECTION","element":{"kind":"RECORD_ID"}},"nullable":true,"reference":{"baseToken":"app_x","tableId":"tbl_projects"}},"Status":{"sqlType":{"kind":"OPTION","options":[{"id":"opt_a","name":"Todo"}]},"nullable":false}}},{"baseToken":"app_x","id":"tbl_projects","name":"Projects","fields":{}}]}`
	for _, format := range []string{"json", "pretty", "ndjson", "csv", "table"} {
		t.Run(format, func(t *testing.T) {
			factory, stdout, registry := newExecuteFactory(t)
			registry.Register(&httpmock.Stub{Method: "GET", URL: "/open-apis/base/v3/bases/app_x/basesql/catalog", Body: json.RawMessage(`{"code":0,"data":` + data + `}`)})
			require.NoError(t, runShortcut(t, BaseSQLCatalog, []string{"+sql-catalog", "--base-token", "app_x", "--format", format}, factory, stdout))
			switch format {
			case "json", "pretty":
				var envelope struct {
					OK   bool            `json:"ok"`
					Data json.RawMessage `json:"data"`
				}
				require.NoError(t, json.Unmarshal(stdout.Bytes(), &envelope))
				require.True(t, envelope.OK)
				require.JSONEq(t, data, string(envelope.Data))
			case "ndjson":
				lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
				require.Len(t, lines, 2)
				require.JSONEq(t, `{"table":"Tasks","field":"Links","type":"COLLECTION<RECORD_ID>","nullable":true,"reference":"Projects"}`, lines[0])
				require.JSONEq(t, `{"table":"Tasks","field":"Status","type":"OPTION","nullable":false,"reference":""}`, lines[1])
			case "csv":
				rows, err := csv.NewReader(strings.NewReader(stdout.String())).ReadAll()
				require.NoError(t, err)
				require.Len(t, rows, 3)
				require.Contains(t, rows[1], "COLLECTION<RECORD_ID>")
				require.Contains(t, rows[1], "Projects")
			case "table":
				require.Contains(t, stdout.String(), "COLLECTION<RECORD_ID>")
				require.Contains(t, stdout.String(), "Projects")
			}
		})
	}
}

func TestSQLInvalidResponses(t *testing.T) {
	for _, tc := range []struct {
		name     string
		shortcut common.Shortcut
		method   string
		path     string
		args     []string
		data     string
	}{
		{"query missing rows", BaseSQLQuery, "POST", "/basesql/query", []string{"--sql", "SELECT rec_id FROM tasks"}, `{}`},
		{"query null rows", BaseSQLQuery, "POST", "/basesql/query", []string{"--sql", "SELECT rec_id FROM tasks"}, `{"rows":null}`},
		{"query invalid rows", BaseSQLQuery, "POST", "/basesql/query", []string{"--sql", "SELECT rec_id FROM tasks"}, `{"rows":"invalid"}`},
		{"catalog missing tables", BaseSQLCatalog, "GET", "/basesql/catalog", []string{"--format", "table"}, `{}`},
		{"catalog invalid tables", BaseSQLCatalog, "GET", "/basesql/catalog", []string{"--format", "ndjson"}, `{"tables":"invalid"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			factory, stdout, registry := newExecuteFactory(t)
			registry.Register(&httpmock.Stub{Method: tc.method, URL: tc.path, Body: json.RawMessage(`{"code":0,"data":` + tc.data + `}`)})
			args := append([]string{tc.shortcut.Command, "--base-token", "app_x"}, tc.args...)
			err := runShortcut(t, tc.shortcut, args, factory, stdout)
			problem, ok := errs.ProblemOf(err)
			require.True(t, ok)
			require.Equal(t, errs.CategoryInternal, problem.Category)
			require.Equal(t, errs.SubtypeInvalidResponse, problem.Subtype)
			require.Empty(t, stdout.String())
		})
	}
}
