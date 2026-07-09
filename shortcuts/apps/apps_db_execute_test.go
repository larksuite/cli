// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/internal/output"
)

// TestAppsDBExecute_SingleSELECTJSONIsRowArray 断言单条 SELECT 的 JSON data 直接是行数组（不再透传 result 字符串）。
func TestAppsDBExecute_SingleSELECTJSONIsRowArray(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps/app_x/sql_commands",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				// DBA 模式 result：结构化数组 JSON 字符串
				"result": `[{"sql_type":"SELECT","data":"[{\"id\":101,\"total_cents\":2500}]","record_count":1}]`,
			},
		},
	})
	if err := runAppsShortcut(t, AppsDBExecute,
		[]string{"+db-execute", "--yes", "--app-id", "app_x", "--sql", "select 1", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	// PRD 单 SELECT：data 直接是行数组（不再是 data.results[].data 字符串）
	var env struct {
		Data []map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("decode envelope: %v\n%s", err, stdout.String())
	}
	if len(env.Data) != 1 {
		t.Fatalf("data = %d rows (want 1)\n%s", len(env.Data), stdout.String())
	}
	if env.Data[0]["id"] != float64(101) || env.Data[0]["total_cents"] != float64(2500) {
		t.Fatalf("data[0] = %v, want {id:101,total_cents:2500}", env.Data[0])
	}
}

// TestAppsDBExecute_SingleDMLJSONShape 断言单条 DML 的 JSON data 形如 {command, rows_affected}。
func TestAppsDBExecute_SingleDMLJSONShape(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps/app_x/sql_commands",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"result": `[{"sql_type":"INSERT","data":"","affected_rows":3}]`,
			},
		},
	})
	if err := runAppsShortcut(t, AppsDBExecute,
		[]string{"+db-execute", "--yes", "--app-id", "app_x", "--sql", "insert", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	// PRD 单 DML：data = {command, rows_affected}
	var env struct {
		Data struct {
			Command      string `json:"command"`
			RowsAffected int    `json:"rows_affected"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("decode: %v\n%s", err, stdout.String())
	}
	if env.Data.Command != "INSERT" || env.Data.RowsAffected != 3 {
		t.Fatalf("data = %+v, want {command:INSERT, rows_affected:3}", env.Data)
	}
}

// TestAppsDBExecute_SingleDDLJSONShape 断言单条 DDL 的 JSON data 形如 {command}。
func TestAppsDBExecute_SingleDDLJSONShape(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps/app_x/sql_commands",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"result": `[{"sql_type":"CREATE_TABLE","data":"[]"}]`,
			},
		},
	})
	if err := runAppsShortcut(t, AppsDBExecute,
		[]string{"+db-execute", "--yes", "--app-id", "app_x", "--sql", "create", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	// PRD 单 DDL：data = {command}
	var env struct {
		Data struct {
			Command string `json:"command"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("decode: %v\n%s", err, stdout.String())
	}
	if env.Data.Command != "CREATE_TABLE" {
		t.Fatalf("data.command = %q, want CREATE_TABLE", env.Data.Command)
	}
}

// TestAppsDBExecute_MultiStatementJSONShape 断言多语句的 JSON data 是元素数组，且 SELECT 包成 {command:"SELECT", rows:[...]}。
func TestAppsDBExecute_MultiStatementJSONShape(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps/app_x/sql_commands",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"result": `[` +
					`{"sql_type":"INSERT","data":"","affected_rows":1},` +
					`{"sql_type":"SELECT","data":"[{\"id\":999}]","record_count":1}` +
					`]`,
			},
		},
	})
	if err := runAppsShortcut(t, AppsDBExecute,
		[]string{"+db-execute", "--yes", "--app-id", "app_x", "--sql", "x", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	// PRD 多语句：data 是元素数组；SELECT 包成 {command:"SELECT", rows:[...]}
	var env struct {
		Data []map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("decode: %v\n%s", err, stdout.String())
	}
	if len(env.Data) != 2 {
		t.Fatalf("data = %d elements (want 2)\n%s", len(env.Data), stdout.String())
	}
	if env.Data[0]["command"] != "INSERT" || env.Data[0]["rows_affected"] != float64(1) {
		t.Fatalf("data[0] = %v, want {command:INSERT, rows_affected:1}", env.Data[0])
	}
	if env.Data[1]["command"] != "SELECT" {
		t.Fatalf("data[1].command = %v, want SELECT", env.Data[1]["command"])
	}
	rows, ok := env.Data[1]["rows"].([]interface{})
	if !ok || len(rows) != 1 {
		t.Fatalf("data[1].rows = %v, want 1 row", env.Data[1]["rows"])
	}
}

// TestAppsDBExecute_DryRunSendsTransactionalFalse 断言 dry-run 发出的请求是 POST、params 带 transactional=false（DBA 模式）且 transactional 不在 body 里。
func TestAppsDBExecute_DryRunSendsTransactionalFalse(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	if err := runAppsShortcut(t, AppsDBExecute,
		[]string{"+db-execute", "--yes", "--app-id", "app_x", "--sql", "select 1", "--environment", "dev", "--dry-run", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("dry-run err=%v", err)
	}
	var env dryRunAPIEnvelope
	if err := json.Unmarshal([]byte(stdout.String()), &env); err != nil {
		t.Fatalf("decode: %v\n%s", err, stdout.String())
	}
	if env.API[0].Method != "POST" || env.API[0].URL != "/open-apis/spark/v1/apps/app_x/sql_commands" {
		t.Fatalf("method/url = %s %s", env.API[0].Method, env.API[0].URL)
	}
	if env.API[0].Body["sql"] != "select 1" {
		t.Fatalf("body.sql = %v", env.API[0].Body["sql"])
	}
	if env.API[0].Params["env"] != "dev" {
		t.Fatalf("params.env = %v", env.API[0].Params["env"])
	}
	if env.API[0].Params["transactional"] != false {
		t.Fatalf("params.transactional = %v (want false, CLI is DBA mode)", env.API[0].Params["transactional"])
	}
	if _, ok := env.API[0].Body["transactional"]; ok {
		t.Fatalf("transactional should NOT be in body, got body=%v", env.API[0].Body)
	}
}

// TestAppsDBExecute_RejectsEmptySQL 断言 --sql 全空白时校验报错（提示需要 --sql 或 --file）。
func TestAppsDBExecute_RejectsEmptySQL(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	err := runAppsShortcut(t, AppsDBExecute,
		[]string{"+db-execute", "--yes", "--app-id", "app_x", "--sql", "   ", "--as", "user"}, factory, stdout)
	if err == nil || !strings.Contains(err.Error(), "--sql or --file") {
		t.Fatalf("expected empty-sql error, got %v", err)
	}
}

// TestAppsDBExecute_LegacyEnvFlagRejected 钉死：旧名 --env 已移除，显式传入报 validation 错并指向 --environment。
func TestAppsDBExecute_LegacyEnvFlagRejected(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	err := runAppsShortcut(t, AppsDBExecute,
		[]string{"+db-execute", "--yes", "--app-id", "app_x", "--sql", "select 1", "--env", "dev", "--as", "user"}, factory, stdout)
	if err == nil {
		t.Fatalf("--env should be rejected; stdout:\n%s", stdout.String())
	}
	p, ok := errs.ProblemOf(err)
	if !ok || p.Category != errs.CategoryValidation {
		t.Fatalf("want a typed validation error, got %T: %v", err, err)
	}
	if !strings.Contains(p.Message, "--environment") {
		t.Errorf("message should point to --environment: %q", p.Message)
	}
}

// --sql 与 --file 互斥
func TestAppsDBExecute_RejectsSQLAndFileTogether(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	err := runAppsShortcut(t, AppsDBExecute,
		[]string{"+db-execute", "--yes", "--app-id", "app_x", "--sql", "SELECT 1", "--file", "x.sql", "--as", "user"}, factory, stdout)
	if err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("expected mutual-exclusion error, got %v", err)
	}
}

// --file 读取相对路径 .sql 文件 → 内容进 body.sql（dry-run 验证）
func TestAppsDBExecute_FileReadsSQLIntoBody(t *testing.T) {
	dir := t.TempDir()
	sqlPath := filepath.Join(dir, "m.sql")
	if err := os.WriteFile(sqlPath, []byte("SELECT 42 AS answer;\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// 切到临时目录，使相对路径校验通过（CLI 仅接受 cwd 内相对路径）。
	// 用 os.Chdir + 还原而非 t.Chdir：后者要 Go 1.24，本仓库 go.mod 为 1.23。
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWD) })

	factory, stdout, _ := newAppsExecuteFactory(t)
	if err := runAppsShortcut(t, AppsDBExecute,
		[]string{"+db-execute", "--app-id", "app_x", "--environment", "dev", "--file", "m.sql", "--dry-run", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("dry-run err=%v", err)
	}
	var env dryRunAPIEnvelope
	if err := json.Unmarshal([]byte(stdout.String()), &env); err != nil {
		t.Fatalf("decode: %v\n%s", err, stdout.String())
	}
	if env.API[0].Body["sql"] != "SELECT 42 AS answer;\n" {
		t.Fatalf("body.sql = %v, want file content", env.API[0].Body["sql"])
	}
}

// ============================================================================
// legacy wire 形态测试 —— BOE server 实测返这种 ["rows-json-string", ...]
// 形态而非 spec 里的 [{sql_type, data, ...}]，CLI 端必须兼容。
// 输入用 BOE 真实抓包数据（test_scripts/boe_e2e/run.log）。
// ============================================================================

// TestAppsDBExecute_LegacyWireSingleSelect 断言 legacy 字符串数组 wire 的单 SELECT 能正常渲染表格、不回退到 RAW。
func TestAppsDBExecute_LegacyWireSingleSelect(t *testing.T) {
	// BOE 实测：SELECT 1 AS x  →  result: "[\"[{\\\"x\\\":1}]\"]"
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps/app_x/sql_commands",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"result": `["[{\"x\":1}]"]`,
			},
		},
	})
	if err := runAppsShortcut(t, AppsDBExecute,
		[]string{"+db-execute", "--yes", "--app-id", "app_x", "--sql", "SELECT 1 AS x", "--format", "pretty", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	got := stdout.String()
	if !strings.Contains(got, "x") {
		t.Errorf("missing header 'x':\n%s", got)
	}
	if !strings.Contains(got, "1") {
		t.Errorf("missing value row '1':\n%s", got)
	}
	// 不应回退到 RAW
	if strings.Contains(got, "RAW") || strings.Contains(got, "[\\\"") {
		t.Errorf("should not fall back to RAW or raw-string passthrough:\n%s", got)
	}
}

// TestAppsDBExecute_LegacyWireSingleSelectJSONIsRowArray 断言 legacy wire 的 SELECT 同样归一化成 PRD 行数组形态。
func TestAppsDBExecute_LegacyWireSingleSelectJSONIsRowArray(t *testing.T) {
	// 验证 legacy wire 的 SELECT 也归一化成 PRD 行数组形态（data 直接是行）
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps/app_x/sql_commands",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"result": `["[{\"x\":1}]"]`,
			},
		},
	})
	if err := runAppsShortcut(t, AppsDBExecute,
		[]string{"+db-execute", "--yes", "--app-id", "app_x", "--sql", "SELECT 1 AS x", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	var env struct {
		Data []map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("decode: %v\n%s", err, stdout.String())
	}
	if len(env.Data) != 1 {
		t.Fatalf("data length = %d, want 1; got: %v", len(env.Data), env.Data)
	}
	if env.Data[0]["x"] != float64(1) {
		t.Fatalf("data[0].x = %v, want 1", env.Data[0]["x"])
	}
}

// TestAppsDBExecute_LegacyWireMultiSelect 断言 legacy wire 多 SELECT 输出带 Statement N header 与末尾 "✓ N statements executed" 汇总。
func TestAppsDBExecute_LegacyWireMultiSelect(t *testing.T) {
	// BOE 实测：SELECT 1; SELECT 2  →  result: "[\"[{\\\"?column?\\\":1}]\",\"[{\\\"?column?\\\":2}]\"]"
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps/app_x/sql_commands",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"result": `["[{\"?column?\":1}]","[{\"?column?\":2}]"]`,
			},
		},
	})
	if err := runAppsShortcut(t, AppsDBExecute,
		[]string{"+db-execute", "--yes", "--app-id", "app_x", "--sql", "SELECT 1; SELECT 2;", "--format", "pretty", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	got := stdout.String()
	// 多语句应有 Statement N: header
	if !strings.Contains(got, "Statement 1: SELECT") || !strings.Contains(got, "Statement 2: SELECT") {
		t.Errorf("missing Statement headers:\n%s", got)
	}
	// 末尾应有 ✓ N statements executed
	if !strings.Contains(got, "✓ 2 statements executed") {
		t.Errorf("missing summary line:\n%s", got)
	}
}

// TestAppsDBExecute_LegacyWireDDLEmptyResult 断言 result 为空字符串时（legacy DDL）pretty 输出 "(empty result)"。
func TestAppsDBExecute_LegacyWireDDLEmptyResult(t *testing.T) {
	// BOE 实测：CREATE TABLE  →  result: "" （空字符串，无 rows）
	// 老 wire 不区分 DDL/DML/无返回，统一标 "ok"
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps/app_x/sql_commands",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"result": ``, // 空字符串
			},
		},
	})
	if err := runAppsShortcut(t, AppsDBExecute,
		[]string{"+db-execute", "--yes", "--app-id", "app_x", "--sql", "CREATE TABLE foo (id INT)", "--format", "pretty", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	got := stdout.String()
	// result="" 触发 parseSQLResult 返 nil → renderSQLPretty 输出 "(empty result)"
	if !strings.Contains(got, "(empty result)") {
		t.Errorf("expected '(empty result)' for empty result string, got:\n%s", got)
	}
}

// TestAppsDBExecute_LegacyWireMultiSelectWithRealTable 断言含 CJK / uuid / int 字段的真实表行能正确显示在 pretty 表格里。
func TestAppsDBExecute_LegacyWireMultiSelectWithRealTable(t *testing.T) {
	// BOE 实测真实表抓包（course 表第一行）：复杂 JSON 含 CJK / timestamp / uuid 字段
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps/app_x/sql_commands",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"result": `["[{\"id\":\"abc-123\",\"title\":\"高效沟通\",\"capacity\":30}]"]`,
			},
		},
	})
	if err := runAppsShortcut(t, AppsDBExecute,
		[]string{"+db-execute", "--yes", "--app-id", "app_x", "--sql", "SELECT id,title,capacity FROM course LIMIT 1", "--format", "pretty", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	got := stdout.String()
	// 验证 CJK / uuid / int 都能正确显示在表格里
	for _, want := range []string{"id", "title", "capacity", "abc-123", "高效沟通", "30"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in pretty output:\n%s", want, got)
		}
	}
}

// pretty 单 SELECT：表格输出，列间两空格，无 Statement header。
func TestAppsDBExecute_PrettySingleSelectTable(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps/app_x/sql_commands",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"result": `[{"sql_type":"SELECT","data":"[{\"id\":101,\"total_cents\":2500},{\"id\":102,\"total_cents\":1800}]","record_count":2}]`,
			},
		},
	})
	if err := runAppsShortcut(t, AppsDBExecute,
		[]string{"+db-execute", "--yes", "--app-id", "app_x", "--sql", "select", "--format", "pretty", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	got := stdout.String()
	if strings.Contains(got, "Statement 1:") {
		t.Errorf("single statement pretty should NOT have Statement header\noutput:\n%s", got)
	}
	// 列按字典序排序：id / total_cents
	if !strings.Contains(got, "id   total_cents") {
		t.Errorf("missing header row\noutput:\n%s", got)
	}
	if !strings.Contains(got, "101  2500") || !strings.Contains(got, "102  1800") {
		t.Errorf("missing data rows\noutput:\n%s", got)
	}
}

// TestAppsDBExecute_PrettyEmptySelect 断言空 SELECT 的 pretty 输出为 "(0 rows)"。
func TestAppsDBExecute_PrettyEmptySelect(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps/app_x/sql_commands",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"result": `[{"sql_type":"SELECT","data":"[]","record_count":0}]`,
			},
		},
	})
	if err := runAppsShortcut(t, AppsDBExecute,
		[]string{"+db-execute", "--yes", "--app-id", "app_x", "--sql", "select", "--format", "pretty", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	if !strings.Contains(stdout.String(), "(0 rows)") {
		t.Fatalf("empty SELECT should print (0 rows), got:\n%s", stdout.String())
	}
}

// TestAppsDBExecute_PrettySingleDMLAndDDL 断言单条 DML 渲染 "✓ N row(s) <verb>"、各类 DDL（含细粒度动词）渲染 "✓ DDL executed"。
func TestAppsDBExecute_PrettySingleDMLAndDDL(t *testing.T) {
	cases := []struct {
		name    string
		result  string
		wantStr string
	}{
		{"INSERT_1_row", `[{"sql_type":"INSERT","data":"","affected_rows":1}]`, "✓ 1 row inserted"},
		{"UPDATE_5_rows", `[{"sql_type":"UPDATE","data":"","affected_rows":5}]`, "✓ 5 rows updated"},
		{"DELETE_0_rows", `[{"sql_type":"DELETE","data":"","affected_rows":0}]`, "✓ 0 rows deleted"},
		{"DDL", `[{"sql_type":"DDL","data":"","affected_rows":0}]`, "✓ DDL executed"},
		// 真机 boe 实测：DDL 的 sql_type 是细粒度动词（CREATE_TABLE / DROP_TABLE / ALTER_TABLE...），
		// data 是 "[]"、无 affected_rows。必须识别为 DDL，而不是落到 dmlSummary 渲染成 "0 rows affected"。
		{"CREATE_TABLE", `[{"sql_type":"CREATE_TABLE","data":"[]"}]`, "✓ DDL executed"},
		{"DROP_TABLE", `[{"sql_type":"DROP_TABLE","data":"[]"}]`, "✓ DDL executed"},
		{"ALTER_TABLE", `[{"sql_type":"ALTER_TABLE","data":"[]"}]`, "✓ DDL executed"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			factory, stdout, reg := newAppsExecuteFactory(t)
			reg.Register(&httpmock.Stub{
				Method: "POST",
				URL:    "/open-apis/spark/v1/apps/app_x/sql_commands",
				Body:   map[string]interface{}{"code": 0, "data": map[string]interface{}{"result": c.result}},
			})
			if err := runAppsShortcut(t, AppsDBExecute,
				[]string{"+db-execute", "--yes", "--app-id", "app_x", "--sql", "x", "--format", "pretty", "--as", "user"},
				factory, stdout); err != nil {
				t.Fatalf("execute err=%v", err)
			}
			if !strings.Contains(stdout.String(), c.wantStr) {
				t.Errorf("want %q\ngot:\n%s", c.wantStr, stdout.String())
			}
		})
	}
}

// TestAppsDBExecute_PrettyMultiStatementsAllSuccess 断言多语句全成功时逐条 Statement 摘要 + 末尾 "✓ N statements executed"。
func TestAppsDBExecute_PrettyMultiStatementsAllSuccess(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps/app_x/sql_commands",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"result": `[` +
					`{"sql_type":"INSERT","data":"","affected_rows":1},` +
					`{"sql_type":"UPDATE","data":"","affected_rows":1},` +
					`{"sql_type":"SELECT","data":"[{\"id\":999}]","record_count":1}` +
					`]`,
			},
		},
	})
	if err := runAppsShortcut(t, AppsDBExecute,
		[]string{"+db-execute", "--yes", "--app-id", "app_x", "--sql", "x", "--format", "pretty", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	got := stdout.String()
	for _, line := range []string{
		"Statement 1: ✓ 1 row inserted",
		"Statement 2: ✓ 1 row updated",
		"Statement 3: SELECT (1 row)",
		"✓ 3 statements executed",
	} {
		if !strings.Contains(got, line) {
			t.Errorf("missing %q in pretty output\nfull:\n%s", line, got)
		}
	}
}

// TestAppsDBExecute_PrettyMultiStatementsDDL 钉住真机 boe 多语句 DDL 的 wire：
// CREATE_TABLE / DROP_TABLE（data="[]"、无 affected_rows）须渲染成 "✓ DDL executed"，
// 不能落到 dmlSummary 变成 "0 rows affected"。
func TestAppsDBExecute_PrettyMultiStatementsDDL(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps/app_x/sql_commands",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"result": `[{"sql_type":"CREATE_TABLE","data":"[]"},{"sql_type":"DROP_TABLE","data":"[]"}]`,
			},
		},
	})
	if err := runAppsShortcut(t, AppsDBExecute,
		[]string{"+db-execute", "--yes", "--app-id", "app_x", "--sql", "x", "--format", "pretty", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	got := stdout.String()
	for _, line := range []string{
		"Statement 1: ✓ DDL executed",
		"Statement 2: ✓ DDL executed",
		"✓ 2 statements executed",
	} {
		if !strings.Contains(got, line) {
			t.Errorf("missing %q in pretty output\nfull:\n%s", line, got)
		}
	}
	if strings.Contains(got, "rows affected") {
		t.Errorf("DDL must not render as 'rows affected'\nfull:\n%s", got)
	}
}

// TestAppsDBExecute_PrettyMultiStatementsPartialFailureWithErrorSentinel 断言多语句部分失败时 pretty 仍打逐条 ✓/✗ 摘要、声明前序已 commit 未回滚，且返回 typed error、不打成功汇总。
func TestAppsDBExecute_PrettyMultiStatementsPartialFailureWithErrorSentinel(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps/app_x/sql_commands",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"result": `[` +
					`{"sql_type":"INSERT","data":"","affected_rows":1},` +
					`{"sql_type":"ERROR","data":"{\"code\":1300015,\"message\":\"syntax error at or near 'SELEC'\"}"}` +
					`]`,
			},
		},
	})
	// pretty 失败路径：逐条 ✓/✗ 摘要照打到 stdout（人看），同时返回 typed error（exit 非 0）。
	err := runAppsShortcut(t, AppsDBExecute,
		[]string{"+db-execute", "--yes", "--app-id", "app_x", "--sql", "x", "--format", "pretty", "--as", "user"},
		factory, stdout)
	if err == nil {
		t.Fatalf("pretty multi-statement failure must still return a typed error; stdout:\n%s", stdout.String())
	}
	got := stdout.String()
	for _, line := range []string{
		"Statement 1: ✓ 1 row inserted",
		"Statement 2: ✗ syntax error at or near 'SELEC' [1300015]",
	} {
		if !strings.Contains(got, line) {
			t.Errorf("missing %q in pretty output\nfull:\n%s", line, got)
		}
	}
	// 非事务（transactional=false）前序语句已逐条 commit 落地，须如实说明「committed and not rolled back」，
	// 绝不能误报整批回滚。
	if !strings.Contains(got, "committed and not rolled back") {
		t.Errorf("non-tx failure must state prior statements committed & not rolled back; got:\n%s", got)
	}
	if strings.Contains(got, "statements executed") {
		t.Errorf("failed run should NOT print success summary; got:\n%s", got)
	}
}

// TestAppsDBExecute_MultiStatementFailureReturnsTypedError 钉死「多语句失败 → typed errs.APIError」：
// json 默认不再打 ok:true 假成功，而是返回 typed errs.* 错误（type=api / subtype=server_error、
// exit=1）。失败位置在 message 的 "(at statement N of M)"，前序是否落地/是否回滚写在 hint。
// 本例无 BEGIN → 前序逐条 commit、未回滚（hint 含 "committed and not rolled back"）。
func TestAppsDBExecute_MultiStatementFailureReturnsTypedError(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps/app_x/sql_commands",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"result": `[` +
					`{"sql_type":"INSERT","data":"","affected_rows":1},` +
					`{"sql_type":"ERROR","data":"{\"code\":\"k_dl_1300002\",\"message\":\"duplicate key value violates unique constraint\"}"}` +
					`]`,
			},
		},
	})
	err := runAppsShortcut(t, AppsDBExecute,
		[]string{"+db-execute", "--yes", "--app-id", "app_x", "--sql", "x", "--as", "user"},
		factory, stdout)
	if err == nil {
		t.Fatalf("multi-statement failure must return a typed error; stdout:\n%s", stdout.String())
	}
	// json 失败路径不得打成功 envelope。
	if strings.Contains(stdout.String(), `"ok": true`) {
		t.Errorf("must not emit ok:true success envelope on failure; stdout:\n%s", stdout.String())
	}
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("want a typed errs.* error, got %T: %v", err, err)
	}
	if p.Category != errs.CategoryAPI || p.Subtype != errs.SubtypeServerError {
		t.Errorf("category/subtype = %s/%s, want api/server_error", p.Category, p.Subtype)
	}
	if p.Code != 1300002 {
		t.Errorf("code = %d, want 1300002", p.Code)
	}
	if !strings.Contains(p.Message, "(at statement 2 of 2)") {
		t.Errorf("message missing statement locator: %q", p.Message)
	}
	// 无 BEGIN → 前序逐条 commit、未回滚，语义写在 hint。
	if !strings.Contains(p.Hint, "committed and not rolled back") {
		t.Errorf("hint should state prior statements committed & not rolled back: %q", p.Hint)
	}
	if output.ExitCodeOf(err) != output.ExitAPI {
		t.Errorf("exit = %d, want %d (ExitAPI)", output.ExitCodeOf(err), output.ExitAPI)
	}
}

// TestAppsDBExecute_SingleErrorReturnsTypedError 单条语句失败（server 也返 code:0 + ERROR 哨兵）
// 同样升级成 typed error：statement_index=0、completed 空、message 标注 (at statement 1 of 1)。
func TestAppsDBExecute_SingleErrorReturnsTypedError(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps/app_x/sql_commands",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"result": `[{"sql_type":"ERROR","data":"{\"code\":\"k_dl_000002\",\"message\":\"syntax error at or near 'SELEC'\"}"}]`,
			},
		},
	})
	err := runAppsShortcut(t, AppsDBExecute,
		[]string{"+db-execute", "--yes", "--app-id", "app_x", "--sql", "x", "--as", "user"},
		factory, stdout)
	if err == nil {
		t.Fatalf("single ERROR sentinel must return a typed error; stdout:\n%s", stdout.String())
	}
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("want a typed errs.* error, got %T: %v", err, err)
	}
	if p.Category != errs.CategoryAPI || p.Subtype != errs.SubtypeServerError {
		t.Errorf("category/subtype = %s/%s, want api/server_error", p.Category, p.Subtype)
	}
	if !strings.Contains(p.Message, "(at statement 1 of 1)") {
		t.Errorf("message missing locator: %q", p.Message)
	}
	// 第一条就失败、无落地 的语义写在 hint。
	if !strings.Contains(p.Hint, "No statements were applied") {
		t.Errorf("hint should state nothing applied: %q", p.Hint)
	}
}

// TestAppsDBExecute_TransactionFailureRolledBack 钉死「显式事务内失败 → 整批回滚」：
// 实测后端把 BEGIN 也作为 statement 返回；completed 含未配对 BEGIN → inferRolledBack 判定回滚。
// 回滚语义现写在 hint（miaoda 原句 "Transaction rolled back; no changes persisted."），失败位置在 message。
func TestAppsDBExecute_TransactionFailureRolledBack(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps/app_x/sql_commands",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				// BOE 实测 wire：BEGIN; CREATE; INSERT(ok); INSERT(dup→ERROR)
				"result": `[` +
					`{"sql_type":"BEGIN","data":"[]"},` +
					`{"sql_type":"CREATE_TABLE","data":"[]"},` +
					`{"sql_type":"INSERT","data":"[{\"rowCount\":1}]","affected_rows":1},` +
					`{"sql_type":"ERROR","data":"{\"code\":\"k_dl_1300002\",\"message\":\"duplicate key value violates unique constraint\"}"}` +
					`]`,
			},
		},
	})
	err := runAppsShortcut(t, AppsDBExecute,
		[]string{"+db-execute", "--yes", "--app-id", "app_x", "--sql", "x", "--as", "user"},
		factory, stdout)
	if err == nil {
		t.Fatalf("transaction failure must return a typed error; stdout:\n%s", stdout.String())
	}
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("want a typed errs.* error, got %T: %v", err, err)
	}
	if p.Category != errs.CategoryAPI || p.Subtype != errs.SubtypeServerError {
		t.Errorf("category/subtype = %s/%s, want api/server_error", p.Category, p.Subtype)
	}
	if !strings.Contains(p.Message, "(at statement 4 of 4)") {
		t.Errorf("message missing statement locator: %q", p.Message)
	}
	// 事务整批回滚 / 前序未落库 的语义写在 hint（miaoda 原句）。
	if !strings.Contains(p.Hint, "Transaction rolled back; no changes persisted.") {
		t.Errorf("hint should state transaction rolled back & nothing persisted: %q", p.Hint)
	}
}

// TestInferRolledBack_Cases 断言 inferRolledBack 按 BEGIN/COMMIT/ROLLBACK 计数判定失败时事务是否仍开着（即整批回滚）。
func TestInferRolledBack_Cases(t *testing.T) {
	stmt := func(t string) map[string]interface{} { return map[string]interface{}{"sql_type": t} }
	cases := []struct {
		name      string
		completed []map[string]interface{}
		want      bool
	}{
		{"empty", nil, false},
		{"autocommit single", []map[string]interface{}{stmt("INSERT")}, false},
		{"open tx (unmatched BEGIN)", []map[string]interface{}{stmt("BEGIN"), stmt("CREATE_TABLE"), stmt("INSERT")}, true},
		{"closed tx (BEGIN+COMMIT)", []map[string]interface{}{stmt("BEGIN"), stmt("INSERT"), stmt("COMMIT")}, false},
		{"reopened tx", []map[string]interface{}{stmt("BEGIN"), stmt("COMMIT"), stmt("BEGIN"), stmt("INSERT")}, true},
		{"rollback closes tx", []map[string]interface{}{stmt("BEGIN"), stmt("INSERT"), stmt("ROLLBACK")}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := inferRolledBack(c.completed); got != c.want {
				t.Errorf("inferRolledBack(%s) = %v, want %v", c.name, got, c.want)
			}
		})
	}
}

// TestCellString_AllKinds 断言 cellString 对 nil/string/bool/整数/小数/对象各类型的字符串化结果。
func TestCellString_AllKinds(t *testing.T) {
	cases := []struct {
		name string
		in   interface{}
		want string
	}{
		{"nil", nil, ""},
		{"string", "hello", "hello"},
		{"bool true", true, "true"},
		{"bool false", false, "false"},
		{"int float", float64(101), "101"},
		{"fractional", float64(1.25), "1.25"},
		{"object", map[string]interface{}{"a": float64(1)}, `{"a":1}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := cellString(c.in); got != c.want {
				t.Errorf("cellString(%v)=%q want %q", c.in, got, c.want)
			}
		})
	}
}

// TestCodeString_Forms 断言 codeString 处理 nil / "k_dl_xxx" / 纯数字串 / float64 / 不支持类型各形态。
func TestCodeString_Forms(t *testing.T) {
	cases := []struct {
		name string
		in   interface{}
		want string
	}{
		{"nil", nil, ""},
		{"k_dl prefix", "k_dl_1300015", "1300015"},
		{"plain string", "1300015", "1300015"},
		{"float64", float64(42), "42"},
		{"unsupported", []int{1}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := codeString(c.in); got != c.want {
				t.Errorf("codeString(%v)=%q want %q", c.in, got, c.want)
			}
		})
	}
}

// TestDmlVerb_AllVerbs 断言 dmlVerb 对 INSERT/UPDATE/DELETE/MERGE 的动词映射（大小写不敏感），非 DML 返回 affected。
func TestDmlVerb_AllVerbs(t *testing.T) {
	cases := map[string]string{
		"INSERT":       "inserted",
		"update":       "updated",
		"DELETE":       "deleted",
		"Merge":        "merged",
		"CREATE_TABLE": "affected",
	}
	for in, want := range cases {
		if got := dmlVerb(in); got != want {
			t.Errorf("dmlVerb(%q)=%q want %q", in, got, want)
		}
	}
}

// TestIntOrZero_Cases 断言 intOrZero 对 JSON number 取整、对非数字 / nil 返回 0。
func TestIntOrZero_Cases(t *testing.T) {
	if got := intOrZero(float64(5)); got != 5 {
		t.Errorf("intOrZero(5)=%d want 5", got)
	}
	if got := intOrZero("x"); got != 0 {
		t.Errorf("intOrZero(non-numeric)=%d want 0", got)
	}
	if got := intOrZero(nil); got != 0 {
		t.Errorf("intOrZero(nil)=%d want 0", got)
	}
}

// TestErrorSummary_Cases 断言 errorSummary 对空 / 非法 JSON / 带 code / 无 code 各情形生成 "message [code]" 文案。
func TestErrorSummary_Cases(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"empty", "", "(unknown error)"},
		{"malformed json", "not json", "not json"},
		{"with code", `{"code":"k_dl_1300015","message":"boom"}`, "boom [1300015]"},
		{"no code", `{"message":"plain"}`, "plain"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := errorSummary(c.in); got != c.want {
				t.Errorf("errorSummary(%q)=%q want %q", c.in, got, c.want)
			}
		})
	}
}

// TestParseErrorSentinel_Cases 断言 parseErrorSentinel 解析 ERROR 哨兵 data 得到数值 code 与 message（含空 / 非法 / 空 message 回退）。
func TestParseErrorSentinel_Cases(t *testing.T) {
	cases := []struct {
		name, in string
		wantCode int
		wantMsg  string
	}{
		{"empty", "", 0, "(unknown error)"},
		{"malformed", "xyz", 0, "xyz"},
		{"code+msg", `{"code":"1300015","message":"boom"}`, 1300015, "boom"},
		{"empty msg", `{"code":"1300015","message":""}`, 1300015, "(unknown error)"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			code, msg := parseErrorSentinel(c.in)
			if code != c.wantCode || msg != c.wantMsg {
				t.Errorf("parseErrorSentinel(%q)=%d,%q want %d,%q", c.in, code, msg, c.wantCode, c.wantMsg)
			}
		})
	}
}

// TestIsStructuredResult_Cases 断言 isStructuredResult 仅在首元素含 sql_type 时判为新结构化形态。
func TestIsStructuredResult_Cases(t *testing.T) {
	if !isStructuredResult([]map[string]interface{}{{"sql_type": "SELECT"}}) {
		t.Error("expected structured=true when sql_type present")
	}
	if isStructuredResult([]map[string]interface{}{{}}) {
		t.Error("expected structured=false when sql_type absent")
	}
	if isStructuredResult(nil) {
		t.Error("expected structured=false for empty")
	}
}

// TestNormalizeLegacyStatement_Cases 断言 normalizeLegacyStatement 把空 / null / 非 JSON 标为 OK、把 rows 数组标为 SELECT 并带 record_count。
func TestNormalizeLegacyStatement_Cases(t *testing.T) {
	t.Run("empty -> OK", func(t *testing.T) {
		got := normalizeLegacyStatement("")
		if got["sql_type"] != "OK" {
			t.Errorf("got sql_type=%v want OK", got["sql_type"])
		}
	})
	t.Run("null -> OK", func(t *testing.T) {
		got := normalizeLegacyStatement("null")
		if got["sql_type"] != "OK" {
			t.Errorf("got sql_type=%v want OK", got["sql_type"])
		}
	})
	t.Run("rows -> SELECT", func(t *testing.T) {
		got := normalizeLegacyStatement(`[{"id":1}]`)
		if got["sql_type"] != "SELECT" {
			t.Errorf("got sql_type=%v want SELECT", got["sql_type"])
		}
		if got["record_count"] != float64(1) {
			t.Errorf("got record_count=%v want 1", got["record_count"])
		}
	})
	t.Run("non-json kept as OK", func(t *testing.T) {
		got := normalizeLegacyStatement(`notjson`)
		if got["sql_type"] != "OK" {
			t.Errorf("got sql_type=%v want OK", got["sql_type"])
		}
	})
}

// TestCellString_MarshalFallback 断言 cellString 对 json.Marshal 拒绝的类型（如 complex）回退到 fmt %v。
func TestCellString_MarshalFallback(t *testing.T) {
	// complex128 is not switch-handled and json.Marshal rejects it →
	// falls back to fmt.Sprintf("%v", v), which is deterministic for complex.
	if got := cellString(complex(1, 2)); got != "(1+2i)" {
		t.Errorf("cellString(complex)=%q want (1+2i)", got)
	}
}

// TestRenderSingleStatementPretty_Branches 断言 renderSingleStatementPretty 对 SELECT/ERROR/DML/legacy OK/DDL 各分支的输出。
func TestRenderSingleStatementPretty_Branches(t *testing.T) {
	cases := []struct {
		name   string
		stmt   map[string]interface{}
		substr string
	}{
		{"select empty", map[string]interface{}{"sql_type": "SELECT", "data": "[]"}, "(0 rows)"},
		{"error", map[string]interface{}{"sql_type": "ERROR", "data": `{"message":"boom"}`}, "✗ boom"},
		{"dml insert", map[string]interface{}{"sql_type": "INSERT", "affected_rows": float64(3)}, "✓ 3 rows inserted"},
		{"legacy ok", map[string]interface{}{"sql_type": "OK"}, "✓ ok"},
		{"ddl default", map[string]interface{}{"sql_type": "CREATE_TABLE"}, "✓ DDL executed"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var b strings.Builder
			renderSingleStatementPretty(&b, c.stmt)
			if !strings.Contains(b.String(), c.substr) {
				t.Errorf("output %q does not contain %q", b.String(), c.substr)
			}
		})
	}
}

// TestRenderSelectRowsAsTable_Branches 断言 renderSelectRowsAsTable 对空串 / 空数组 / 非法 JSON 回退 / 正常 rows 各分支的输出。
func TestRenderSelectRowsAsTable_Branches(t *testing.T) {
	cases := []struct {
		name   string
		data   string
		substr string
	}{
		{"empty string", "", "(0 rows)"},
		{"empty array", "[]", "(0 rows)"},
		{"malformed fallback", "{bad", "{bad"},
		{"rows", `[{"id":1}]`, "id"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var b strings.Builder
			renderSelectRowsAsTable(&b, c.data)
			if !strings.Contains(b.String(), c.substr) {
				t.Errorf("output %q does not contain %q", b.String(), c.substr)
			}
		})
	}
}
