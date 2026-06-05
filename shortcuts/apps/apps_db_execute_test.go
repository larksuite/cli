// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/internal/output"
)

func TestAppsDBExecute_SingleSELECTJSONEnvelopeWrapsResults(t *testing.T) {
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
	// JSON envelope 应该把 result 字符串 parse 之后放进 data.results
	var env struct {
		Data struct {
			Results []map[string]interface{} `json:"results"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("decode envelope: %v\n%s", err, stdout.String())
	}
	if len(env.Data.Results) != 1 {
		t.Fatalf("data.results = %d items (want 1)", len(env.Data.Results))
	}
	if env.Data.Results[0]["sql_type"] != "SELECT" {
		t.Fatalf("results[0].sql_type = %v", env.Data.Results[0]["sql_type"])
	}
}

func TestAppsDBExecute_DryRunSendsTransactionalFalse(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	if err := runAppsShortcut(t, AppsDBExecute,
		[]string{"+db-execute", "--yes", "--app-id", "app_x", "--sql", "select 1", "--env", "dev", "--dry-run", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("dry-run err=%v", err)
	}
	var env struct {
		API []struct {
			Method string                 `json:"method"`
			URL    string                 `json:"url"`
			Body   map[string]interface{} `json:"body"`
			Params map[string]interface{} `json:"params"`
		} `json:"api"`
	}
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

func TestAppsDBExecute_RejectsEmptySQL(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	err := runAppsShortcut(t, AppsDBExecute,
		[]string{"+db-execute", "--yes", "--app-id", "app_x", "--sql", "   ", "--as", "user"}, factory, stdout)
	if err == nil || !strings.Contains(err.Error(), "--sql or --file") {
		t.Fatalf("expected empty-sql error, got %v", err)
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
		[]string{"+db-execute", "--app-id", "app_x", "--env", "dev", "--file", "m.sql", "--dry-run", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("dry-run err=%v", err)
	}
	var env struct {
		API []struct {
			Body map[string]interface{} `json:"body"`
		} `json:"api"`
	}
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

func TestAppsDBExecute_LegacyWireSingleSelectJSONEnvelope(t *testing.T) {
	// 验证 JSON envelope 也把 legacy result 正确归一化进 data.results
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
		Data struct {
			Results []map[string]interface{} `json:"results"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("decode: %v\n%s", err, stdout.String())
	}
	if len(env.Data.Results) != 1 {
		t.Fatalf("results length = %d, want 1; got: %v", len(env.Data.Results), env.Data.Results)
	}
	if env.Data.Results[0]["sql_type"] != "SELECT" {
		t.Fatalf("results[0].sql_type = %v, want SELECT", env.Data.Results[0]["sql_type"])
	}
	if env.Data.Results[0]["record_count"] != float64(1) {
		t.Fatalf("results[0].record_count = %v, want 1", env.Data.Results[0]["record_count"])
	}
}

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
	// DBA 模式（transactional=false）前序语句已 auto-commit 落地，绝不能误报「rolled back」。
	if strings.Contains(got, "rolled back") {
		t.Errorf("DBA mode must NOT claim rollback (prior statements persisted); got:\n%s", got)
	}
	if strings.Contains(got, "statements executed") {
		t.Errorf("failed run should NOT print success summary; got:\n%s", got)
	}
}

// TestAppsDBExecute_MultiStatementFailureReturnsTypedError 钉死「多语句失败 → partial failure」：
// 逐条结果 + statement_index / error_code / rolled_back / note 作为 ok:false 数据落 stdout，
// 退出信号是 PartialFailureError（非零 exit）。rolled_back=false 因 CLI 永远 DBA 模式
// （真机 boe 实证：失败前的语句已落地）。
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
		t.Fatalf("multi-statement failure must return a partial-failure error; stdout:\n%s", stdout.String())
	}
	// json 失败路径不得打成功 envelope。
	if strings.Contains(stdout.String(), `"ok": true`) {
		t.Errorf("must not emit ok:true success envelope on failure; stdout:\n%s", stdout.String())
	}
	var pfErr *output.PartialFailureError
	if !errors.As(err, &pfErr) {
		t.Fatalf("want *output.PartialFailureError, got %T: %v", err, err)
	}
	if pfErr.Code != output.ExitAPI {
		t.Errorf("exit = %d, want %d (ExitAPI)", pfErr.Code, output.ExitAPI)
	}
	payload := decodePartialFailureData(t, stdout.String())
	if got := payload["statement_index"]; got != float64(1) {
		t.Errorf("statement_index = %v, want 1", got)
	}
	if got := payload["error_code"]; got != float64(1300002) {
		t.Errorf("error_code = %v, want 1300002", got)
	}
	msg, _ := payload["error_message"].(string)
	if !strings.Contains(msg, "(at statement 2 of 2)") {
		t.Errorf("error_message missing statement locator: %q", msg)
	}
	if got := payload["rolled_back"]; got != false {
		t.Errorf("rolled_back = %v, want false (DBA mode persists prior statements)", got)
	}
	results, _ := payload["results"].([]interface{})
	if len(results) != 2 {
		t.Errorf("results length = %d, want 2 (persisted statement + ERROR sentinel)", len(results))
	}
	note, _ := payload["note"].(string)
	if !strings.Contains(note, "already applied") {
		t.Errorf("note should warn prior statements persisted, got %q", note)
	}
}

// decodePartialFailureData 解析 stdout 上 ok:false 的 partial-failure envelope，返回 data 块。
func decodePartialFailureData(t *testing.T, stdoutStr string) map[string]interface{} {
	t.Helper()
	var envelope struct {
		OK   bool                   `json:"ok"`
		Data map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal([]byte(stdoutStr), &envelope); err != nil {
		t.Fatalf("stdout is not a JSON envelope: %v\n%s", err, stdoutStr)
	}
	if envelope.OK {
		t.Fatalf("envelope.ok = true, want false on partial failure")
	}
	if envelope.Data == nil {
		t.Fatalf("envelope.data missing; stdout:\n%s", stdoutStr)
	}
	return envelope.Data
}

// TestAppsDBExecute_SingleErrorReturnsTypedError 单条语句失败（server 也返 code:0 + ERROR 哨兵）
// 同样走 partial failure：statement_index=0、note 说明无语句落地、message 标注 (at statement 1 of 1)。
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
		t.Fatalf("single ERROR sentinel must return a partial-failure error; stdout:\n%s", stdout.String())
	}
	var pfErr *output.PartialFailureError
	if !errors.As(err, &pfErr) {
		t.Fatalf("want *output.PartialFailureError, got %T: %v", err, err)
	}
	payload := decodePartialFailureData(t, stdout.String())
	msg, _ := payload["error_message"].(string)
	if !strings.Contains(msg, "(at statement 1 of 1)") {
		t.Errorf("error_message missing locator: %q", msg)
	}
	if got := payload["statement_index"]; got != float64(0) {
		t.Errorf("statement_index = %v, want 0", got)
	}
	note, _ := payload["note"].(string)
	if !strings.Contains(note, "no statements were applied") {
		t.Errorf("note should say nothing was applied, got %q", note)
	}
}

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

func TestCellString_MarshalFallback(t *testing.T) {
	// complex128 is not switch-handled and json.Marshal rejects it →
	// falls back to fmt.Sprintf("%v", v), which is deterministic for complex.
	if got := cellString(complex(1, 2)); got != "(1+2i)" {
		t.Errorf("cellString(complex)=%q want (1+2i)", got)
	}
}

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

// TestAppsDBExecute_PrettyPartialFailureKeepsStdoutHumanOnly pins the pretty
// contract on a statement failure: stdout carries only the per-statement
// human summary (no JSON envelope stacked after it), and the command still
// exits non-zero via the partial-failure signal.
func TestAppsDBExecute_PrettyPartialFailureKeepsStdoutHumanOnly(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps/app_x/sql_commands",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"result": `[{"sql_type":"ERROR","data":"{\"code\":\"k_dl_000002\",\"message\":\"syntax error\"}"}]`,
			},
		},
	})
	err := runAppsShortcut(t, AppsDBExecute,
		[]string{"+db-execute", "--yes", "--app-id", "app_x", "--sql", "x", "--format", "pretty", "--as", "user"},
		factory, stdout)
	var pfErr *output.PartialFailureError
	if !errors.As(err, &pfErr) {
		t.Fatalf("want *output.PartialFailureError, got %T: %v", err, err)
	}
	out := stdout.String()
	if !strings.Contains(out, "✗") {
		t.Fatalf("pretty summary missing failure marker; stdout:\n%s", out)
	}
	if strings.Contains(out, `"ok"`) {
		t.Fatalf("pretty stdout must not stack a JSON envelope after the summary; stdout:\n%s", out)
	}
}
