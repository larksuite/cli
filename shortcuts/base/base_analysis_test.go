// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/spf13/cobra"
)

func TestAnalysisShortcutMetadata(t *testing.T) {
	tests := []struct {
		name     string
		shortcut common.Shortcut
		scope    string
	}{
		{name: "context", shortcut: BaseSchemaContextGet, scope: "base:table:read"},
		{name: "sql", shortcut: BaseDataSQLQuery, scope: "base:record:read"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.shortcut.Risk != "read" {
				t.Fatalf("Risk=%q want=read", tt.shortcut.Risk)
			}
			if !reflect.DeepEqual(tt.shortcut.Scopes, []string{tt.scope}) {
				t.Fatalf("Scopes=%v want=%v", tt.shortcut.Scopes, []string{tt.scope})
			}
			if !reflect.DeepEqual(tt.shortcut.AuthTypes, []string{"user", "bot"}) {
				t.Fatalf("AuthTypes=%v", tt.shortcut.AuthTypes)
			}
			var format *common.Flag
			for i := range tt.shortcut.Flags {
				if tt.shortcut.Flags[i].Name == "format" {
					format = &tt.shortcut.Flags[i]
				}
			}
			if format == nil || !format.Hidden || format.Default != "json" || len(format.Enum) != 0 {
				t.Fatalf("format flag=%+v", format)
			}
		})
	}
}

func TestAnalysisShortcutsHideOutputFormatSurface(t *testing.T) {
	for _, shortcut := range []common.Shortcut{BaseSchemaContextGet, BaseDataSQLQuery} {
		t.Run(shortcut.Command, func(t *testing.T) {
			parent := &cobra.Command{Use: "base"}
			shortcut.Mount(parent, &cmdutil.Factory{})
			cmd, _, err := parent.Find([]string{shortcut.Command})
			if err != nil {
				t.Fatal(err)
			}
			if flag := cmd.Flags().Lookup("format"); flag == nil || !flag.Hidden {
				t.Fatalf("format flag=%+v", flag)
			}
			if flag := cmd.Flags().Lookup("json"); flag != nil {
				t.Fatalf("unexpected --json flag=%+v", flag)
			}
		})
	}
}

func TestAnalysisRequestBodiesAndDryRun(t *testing.T) {
	ctx := context.Background()
	contextRuntime := newBaseTestRuntimeWithArrays(
		map[string]string{"base-token": "app_x", "language": "en", "format": "json"},
		map[string][]string{"table-id": {" tbl_a ", "tbl_b"}}, nil, nil,
	)
	assertDryRunContains(t, dryRunSchemaContextGet(ctx, contextRuntime),
		"POST /open-apis/base/v3/bases/app_x/data/context",
		`"format":"markdown"`, `"language":"en"`, `"table_ids":["tbl_a","tbl_b"]`)
	allTablesRuntime := newBaseTestRuntimeWithArrays(
		map[string]string{"base-token": "app_x", "language": "zh", "format": "json"},
		map[string][]string{"table-id": nil}, nil, nil,
	)
	if output := dryRunSchemaContextGet(ctx, allTablesRuntime).Format(); strings.Contains(output, "table_ids") {
		t.Fatalf("omitted table IDs must query all visible tables:\n%s", output)
	}

	sqlRuntime := newBaseTestRuntime(
		map[string]string{"base-token": "app_x", "sql": " SELECT 1 ", "language": "ja", "format": "json"},
		nil, map[string]int{"timeout-ms": 2500},
	)
	assertDryRunContains(t, dryRunDataSQLQuery(ctx, sqlRuntime),
		"POST /open-apis/base/v3/bases/app_x/data/sql",
		`"sql":"SELECT 1"`, `"language":"ja"`, `"timeout_ms":2500`)
	if output := dryRunDataSQLQuery(ctx, sqlRuntime).Format(); strings.Contains(output, `"format"`) {
		t.Fatalf("SQL request must not send format:\n%s", output)
	}

	sqlRuntime = newBaseTestRuntime(
		map[string]string{"base-token": "app_x", "sql": "SELECT 1", "language": "zh", "format": "json"},
		nil, map[string]int{"timeout-ms": -1},
	)
	if output := dryRunDataSQLQuery(ctx, sqlRuntime).Format(); strings.Contains(output, "timeout_ms") {
		t.Fatalf("non-positive timeout must be omitted:\n%s", output)
	}
}

func TestDataSQLQueryDryRunResolvesFileAndStdin(t *testing.T) {
	dir := t.TempDir()
	withBaseWorkingDir(t, dir)
	queryPath := filepath.Join(dir, "query.sql")
	if err := os.WriteFile(queryPath, []byte("SELECT 42"), 0o600); err != nil {
		t.Fatal(err)
	}

	factory, stdout, _ := newExecuteFactory(t)
	err := runShortcut(t, BaseDataSQLQuery, []string{
		"+data-sql-query", "--base-token", "app_x", "--sql", "@query.sql", "--dry-run",
	}, factory, stdout)
	if err != nil {
		t.Fatalf("file dry-run err=%v", err)
	}
	if got := stdout.String(); !strings.Contains(got, `"SELECT 42"`) {
		t.Fatalf("file dry-run=%s", got)
	}

	factory, stdout, _ = newExecuteFactory(t)
	factory.IOStreams.In = strings.NewReader("SELECT 43")
	err = runShortcut(t, BaseDataSQLQuery, []string{
		"+data-sql-query", "--base-token", "app_x", "--sql", "-", "--dry-run",
	}, factory, stdout)
	if err != nil {
		t.Fatalf("stdin dry-run err=%v", err)
	}
	if got := stdout.String(); !strings.Contains(got, `"SELECT 43"`) {
		t.Fatalf("stdin dry-run=%s", got)
	}
}

func TestSchemaContextGetCommandDryRun(t *testing.T) {
	factory, stdout, _ := newExecuteFactory(t)
	err := runShortcut(t, BaseSchemaContextGet, []string{
		"+schema-context-get", "--base-token", "app_x",
		"--table-id", "tbl_a", "--table-id", "tbl_b",
		"--language", "en", "--dry-run",
	}, factory, stdout)
	if err != nil {
		t.Fatalf("dry-run err=%v", err)
	}
	output := stdout.String()
	for _, want := range []string{
		`"method": "POST"`,
		`"url": "/open-apis/base/v3/bases/app_x/data/context"`,
		`"format": "markdown"`,
		`"language": "en"`,
		`"tbl_a"`,
		`"tbl_b"`,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("dry-run missing %q: %s", want, output)
		}
	}
}

func TestValidateBaseSQLInput(t *testing.T) {
	valid := []string{
		"SELECT id, amount FROM orders",
		"SELECT * FROM orders",
		"SELECT DISTINCT * FROM orders",
		"WITH orders AS (SELECT id FROM source) SELECT * FROM orders",
		"SELECT DISTINCT id FROM orders",
		"SELECT ALL id FROM orders",
		"SELECT count(*) AS total FROM orders",
		"WITH totals AS (SELECT sum(amount) AS total FROM orders) SELECT total FROM totals",
		"UPDATE orders SET amount=0",
		"SELECT 1; SELECT 2",
		"SELECT id FROM orders FOR UPDATE",
		"SELECT 'SELECT * FROM hidden' AS note",
		`SELECT "select *" FROM records`,
		"-- SELECT * FROM hidden\nSELECT id FROM orders",
		"/* SELECT * FROM hidden */ SELECT id FROM orders",
		"SELECT orders.* FROM orders",
		"SELECT select_star FROM records",
		"SELECT 'unterminated",
	}
	for _, sql := range valid {
		if err := validateBaseSQLInput(sql); err != nil {
			t.Errorf("valid SQL %q: %v", sql, err)
		}
	}

	invalid := []string{"", " \n\t "}
	for _, sql := range invalid {
		if err := validateBaseSQLInput(sql); err == nil {
			t.Errorf("invalid SQL accepted: %q", sql)
		}
	}
}

func TestDataSQLQueryCommandValidationStopsBeforeDryRun(t *testing.T) {
	tests := []struct {
		name string
		sql  string
		want string
	}{{name: "empty", sql: "   ", want: "cannot be empty"}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			factory, stdout, _ := newExecuteFactory(t)
			err := runShortcut(t, BaseDataSQLQuery, []string{
				"+data-sql-query", "--base-token", "app_x", "--sql", tt.sql, "--dry-run",
			}, factory, stdout)
			problem, ok := errs.ProblemOf(err)
			if !ok || problem.Category != errs.CategoryValidation || problem.Subtype != errs.SubtypeInvalidArgument {
				t.Fatalf("err=%v problem=%+v", err, problem)
			}
			if !strings.Contains(problem.Message, tt.want) {
				t.Fatalf("message=%q want substring %q", problem.Message, tt.want)
			}
			if stdout.Len() != 0 {
				t.Fatalf("rejected SQL must not emit dry-run output: %s", stdout.String())
			}
		})
	}
}

func TestDataSQLQueryDryRunLeavesServerValidationToServer(t *testing.T) {
	for _, sql := range []string{"UPDATE orders SET amount=0", "SELECT * FROM orders"} {
		factory, stdout, _ := newExecuteFactory(t)
		err := runShortcut(t, BaseDataSQLQuery, []string{
			"+data-sql-query", "--base-token", "app_x", "--sql", sql, "--dry-run",
		}, factory, stdout)
		if err != nil {
			t.Fatalf("dry-run SQL %q err=%v", sql, err)
		}
		if got := stdout.String(); !strings.Contains(got, `"sql": `+strconv.Quote(sql)) {
			t.Fatalf("dry-run must preserve SQL for server validation: %s", got)
		}
	}
}

func TestAnalysisExecuteSuccess(t *testing.T) {
	t.Run("context", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		stub := registerAnalysisResponse(reg, "/open-apis/base/v3/bases/app_x/data/context", map[string]interface{}{
			"success": true,
			"data":    map[string]interface{}{"format": "markdown", "content": "# Base", "truncated": false},
		})
		err := runShortcut(t, BaseSchemaContextGet, []string{
			"+schema-context-get", "--base-token", "app_x", "--table-id", "tbl_a", "--table-id", "tbl_b",
		}, factory, stdout)
		if err != nil {
			t.Fatal(err)
		}
		assertJSONBodyField(t, stub.CapturedBody, "format", "markdown")
		assertOutputDataField(t, stdout.String(), "content", "# Base")
	})

	t.Run("sql", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		stub := registerAnalysisResponse(reg, "/open-apis/base/v3/bases/app_x/data/sql", map[string]interface{}{
			"success": true,
			"data": map[string]interface{}{
				"columns": []interface{}{"total"}, "rows": []interface{}{map[string]interface{}{"total": 42}},
				"row_count": 1, "truncated": false, "feedback": "",
			},
		})
		err := runShortcut(t, BaseDataSQLQuery, []string{
			"+data-sql-query", "--base-token", "app_x", "--sql", "SELECT 42 AS total", "--timeout-ms", "3000",
		}, factory, stdout)
		if err != nil {
			t.Fatal(err)
		}
		assertJSONBodyField(t, stub.CapturedBody, "timeout_ms", float64(3000))
		assertOutputDataField(t, stdout.String(), "row_count", float64(1))
	})
}

func TestAnalysisExecuteBusinessAndMalformedFailures(t *testing.T) {
	tests := []struct {
		name     string
		shortcut common.Shortcut
		path     string
		args     []string
		payload  interface{}
		category errs.Category
		subtype  errs.Subtype
		message  string
	}{
		{
			name: "business failure", shortcut: BaseDataSQLQuery, path: "/open-apis/base/v3/bases/app_x/data/sql",
			args:     []string{"+data-sql-query", "--base-token", "app_x", "--sql", "SELECT 1"},
			payload:  map[string]interface{}{"success": false, "error_msg": "query rejected"},
			category: errs.CategoryAPI, subtype: errs.SubtypeUnknown, message: "query rejected",
		},
		{
			name: "missing success", shortcut: BaseSchemaContextGet, path: "/open-apis/base/v3/bases/app_x/data/context",
			args: []string{"+schema-context-get", "--base-token", "app_x"}, payload: map[string]interface{}{"data": map[string]interface{}{}},
			category: errs.CategoryInternal, subtype: errs.SubtypeInvalidResponse, message: "success",
		},
		{
			name: "success and truncated", shortcut: BaseSchemaContextGet, path: "/open-apis/base/v3/bases/app_x/data/context",
			args:     []string{"+schema-context-get", "--base-token", "app_x"},
			payload:  map[string]interface{}{"success": true, "data": map[string]interface{}{"format": "markdown", "content": "partial", "truncated": true}},
			category: errs.CategoryInternal, subtype: errs.SubtypeInvalidResponse, message: "conflicts",
		},
		{
			name: "empty business error", shortcut: BaseDataSQLQuery, path: "/open-apis/base/v3/bases/app_x/data/sql",
			args:     []string{"+data-sql-query", "--base-token", "app_x", "--sql", "SELECT 1"},
			payload:  map[string]interface{}{"success": false, "error_msg": ""},
			category: errs.CategoryInternal, subtype: errs.SubtypeInvalidResponse, message: "error_msg",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			factory, stdout, reg := newExecuteFactory(t)
			registerAnalysisResponse(reg, tt.path, tt.payload)
			err := runShortcut(t, tt.shortcut, tt.args, factory, stdout)
			problem, ok := errs.ProblemOf(err)
			if !ok {
				t.Fatalf("err=%v is not typed", err)
			}
			if problem.Category != tt.category || problem.Subtype != tt.subtype || !strings.Contains(problem.Message, tt.message) {
				t.Fatalf("problem=%+v", problem)
			}
			if stdout.Len() != 0 {
				t.Fatalf("failure must not emit partial data: %s", stdout.String())
			}
		})
	}
}

func TestAnalysisExecuteTruncatedPartialFailures(t *testing.T) {
	tests := []struct {
		name     string
		shortcut common.Shortcut
		path     string
		args     []string
		data     map[string]interface{}
		field    string
		want     interface{}
	}{
		{
			name: "context", shortcut: BaseSchemaContextGet, path: "/open-apis/base/v3/bases/app_x/data/context",
			args:  []string{"+schema-context-get", "--base-token", "app_x"},
			data:  map[string]interface{}{"format": "markdown", "content": "# Partial Base", "truncated": true},
			field: "content", want: "# Partial Base",
		},
		{
			name: "sql", shortcut: BaseDataSQLQuery, path: "/open-apis/base/v3/bases/app_x/data/sql",
			args: []string{"+data-sql-query", "--base-token", "app_x", "--sql", "SELECT * FROM orders"},
			data: map[string]interface{}{
				"columns":   []interface{}{"id"},
				"rows":      []interface{}{map[string]interface{}{"id": json.Number("9007199254740993")}},
				"row_count": json.Number("9007199254740993"), "truncated": true,
			},
			field: "row_count", want: json.Number("9007199254740993"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			factory, stdout, reg := newExecuteFactory(t)
			registerAnalysisResponse(reg, tt.path, map[string]interface{}{
				"success": false, "error_msg": "result truncated", "data": tt.data,
			})
			err := runShortcut(t, tt.shortcut, tt.args, factory, stdout)
			var partialErr *output.PartialFailureError
			if !errors.As(err, &partialErr) || partialErr.Code != output.ExitAPI {
				t.Fatalf("err=%T %v want partial failure exit %d", err, err, output.ExitAPI)
			}

			decoder := json.NewDecoder(strings.NewReader(stdout.String()))
			decoder.UseNumber()
			var envelope struct {
				OK   bool                   `json:"ok"`
				Data map[string]interface{} `json:"data"`
			}
			if err := decoder.Decode(&envelope); err != nil {
				t.Fatalf("decode partial output: %v; output=%s", err, stdout.String())
			}
			if envelope.OK {
				t.Fatalf("partial output must set ok=false: %s", stdout.String())
			}
			if got := envelope.Data[tt.field]; !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("data[%q]=%v (%T) want=%v (%T)", tt.field, got, got, tt.want, tt.want)
			}
			if envelope.Data["truncated"] != true || envelope.Data["error_msg"] != "result truncated" {
				t.Fatalf("partial output missing truncation details: %s", stdout.String())
			}
		})
	}
}

func TestAnalysisRejectsNonJSONFormat(t *testing.T) {
	for _, tc := range []struct {
		shortcut common.Shortcut
		args     []string
	}{
		{BaseSchemaContextGet, []string{"+schema-context-get", "--base-token", "app_x", "--format", "pretty"}},
		{BaseDataSQLQuery, []string{"+data-sql-query", "--base-token", "app_x", "--sql", "SELECT 1", "--format", "csv"}},
	} {
		factory, stdout, _ := newExecuteFactory(t)
		err := runShortcut(t, tc.shortcut, tc.args, factory, stdout)
		problem, ok := errs.ProblemOf(err)
		if !ok || problem.Category != errs.CategoryValidation || problem.Subtype != errs.SubtypeInvalidArgument {
			t.Fatalf("err=%v problem=%+v", err, problem)
		}
	}
}

func registerAnalysisResponse(reg *httpmock.Registry, path string, payload interface{}) *httpmock.Stub {
	stub := &httpmock.Stub{
		Method: "POST", URL: path,
		Body: map[string]interface{}{"code": 0, "data": payload},
	}
	reg.Register(stub)
	return stub
}

func assertJSONBodyField(t *testing.T, body []byte, key string, want interface{}) {
	t.Helper()
	var decoded map[string]interface{}
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("decode body: %v; body=%s", err, body)
	}
	if got := decoded[key]; !reflect.DeepEqual(got, want) {
		t.Fatalf("body[%q]=%v want=%v; body=%s", key, got, want, body)
	}
}

func assertOutputDataField(t *testing.T, output, key string, want interface{}) {
	t.Helper()
	var envelope map[string]interface{}
	if err := json.Unmarshal([]byte(output), &envelope); err != nil {
		t.Fatalf("decode output: %v; output=%s", err, output)
	}
	data, ok := envelope["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("output data=%T; output=%s", envelope["data"], output)
	}
	if got := data[key]; !reflect.DeepEqual(got, want) {
		t.Fatalf("output data[%q]=%v want=%v; output=%s", key, got, want, output)
	}
}
