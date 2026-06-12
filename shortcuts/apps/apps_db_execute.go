// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
)

// AppsDBExecute executes SQL against a Miaoda app database.
//
// POST /apps/{app_id}/sql_commands，CLI 永远带 ?transactional=false 进入 DBA 模式
// （不默认包事务、支持 DDL、result 字符串内嵌结构化 JSON）。
//
// pretty 渲染 6 种形态：
//   - 单 SELECT：表格（列间两空格、列对齐填充）
//   - 空 SELECT：`(0 rows)`
//   - 单 DML：`✓ N row(s) <verb>`（verb 跟 sql_type：INSERT→inserted/UPDATE→updated/DELETE→deleted）
//   - 单 DDL：`✓ DDL executed`
//   - 多语句全部成功：逐条 `Statement K: ✓ <summary>` + 末尾 `✓ N statements executed`
//   - 多语句部分失败：`Statement K: ✗ <message> [<code>]` + 末尾「前序语句已落地」提示
//
// 失败语义：server 多语句失败仍返 code:0，把失败语句标成 ERROR 哨兵塞进 result。Execute 检测到哨兵
// 后按 partial failure 上报（exit 非 0）：stdout 输出 ok:false 数据，带 results /
// statement_index / error_code / error_message / rolled_back / note，避免 agent 误判
// ok:true 假成功。CLI 永远 DBA 模式（transactional=false），失败前的语句已 auto-commit
// 落地，故 rolled_back=false（真机 boe 实证）。
//
// JSON envelope（成功路径）：CLI 把 server 返的 result 字符串解出来放进 `data.results` 数组。
//
// Risk: high-risk-write —— SQL 可含 DML/DDL，框架对所有执行强制 --yes 确认关卡（--dry-run 预览豁免）。
//
// SQL 来源二选一：--sql（内联文本，或 - 读 stdin）/ --file（.sql 文件路径，受 CLI 相对路径约束）。
// --file 在 Validate 阶段读出内容、归一化到 --sql，下游统一从 rctx.Str("sql") 取。
var AppsDBExecute = common.Shortcut{
	Service:     appsService,
	Command:     "+db-execute",
	Description: "Execute SQL (SELECT / DML / DDL) against a Miaoda app database",
	Risk:        "high-risk-write",
	Tips: []string{
		`Example: lark-cli apps +db-execute --app-id <app_id> --sql "SELECT * FROM orders LIMIT 10" --yes`,
		`Example: lark-cli apps +db-execute --app-id <app_id> --env dev --file ./migration.sql --yes`,
		"Tip: filter fields with --jq, e.g. -q '.data.results[].sql_type'",
	},
	Scopes:    []string{"spark:app:write"},
	AuthTypes: []string{"user"},
	HasFormat: true,
	Flags: []common.Flag{
		{Name: "app-id", Desc: "Miaoda app id", Required: true},
		{Name: "sql", Desc: "SQL text; use - to read stdin. Mutually exclusive with --file",
			Input: []string{common.Stdin}},
		{Name: "file", Desc: "path to a .sql file (relative to cwd). Mutually exclusive with --sql"},
		{Name: "env", Default: "dev", Enum: []string{"dev", "online"}, Desc: "target db environment (default dev; use --env online for the online environment)"},
	},
	Validate: func(ctx context.Context, rctx *common.RuntimeContext) error {
		if _, err := requireAppID(rctx.Str("app-id")); err != nil {
			return err
		}
		sql := strings.TrimSpace(rctx.Str("sql"))
		file := strings.TrimSpace(rctx.Str("file"))
		if sql != "" && file != "" {
			return appsValidationError("--sql and --file are mutually exclusive").
				WithParams(
					appsInvalidParam("--sql", "mutually exclusive with --file"),
					appsInvalidParam("--file", "mutually exclusive with --sql"),
				)
		}
		if file != "" {
			data, err := cmdutil.ReadInputFile(rctx.FileIO(), file)
			if err != nil {
				return appsValidationParamError("--file", "--file: %v", err).WithCause(err)
			}
			// 归一化：把文件内容写回 --sql，下游（DryRun/Execute）统一从 sql 取。
			rctx.Cmd.Flags().Set("sql", string(data))
			sql = strings.TrimSpace(string(data))
		}
		if sql == "" {
			return appsValidationError("one of --sql or --file is required (use --sql - to read stdin)").
				WithParams(
					appsInvalidParam("--sql", "one of --sql or --file is required"),
					appsInvalidParam("--file", "one of --sql or --file is required"),
				)
		}
		return nil
	},
	DryRun: func(ctx context.Context, rctx *common.RuntimeContext) *common.DryRunAPI {
		appID, _ := requireAppID(rctx.Str("app-id"))
		return common.NewDryRunAPI().
			POST(appSQLPath(appID)).
			Desc("Execute SQL on Miaoda app database").
			Params(buildDBSQLParams(rctx)).
			Body(buildDBSQLBody(rctx))
	},
	Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
		appID, err := requireAppID(rctx.Str("app-id"))
		if err != nil {
			return err
		}
		raw, err := rctx.CallAPITyped("POST", appSQLPath(appID),
			buildDBSQLParams(rctx),
			buildDBSQLBody(rctx))
		if err != nil {
			return withAppsHint(err, "verify table/column names with `lark-cli apps +db-table-get --app-id "+appID+" --table <table>`; for day-to-day debugging target the dev database with `--env dev`")
		}

		// server `result: string` 内嵌结构化数组 —— CLI 解出来放进 envelope 的 data.results，
		// 让 json/pretty 路径都基于同一份反序列化产物渲染。
		stmts := parseSQLResult(common.GetString(raw, "result"))
		// 注意：data.results 在 json（默认）路径下原样透出全部行，CLI 侧不再二次截断。
		// 这不是无界 token 黑洞 —— server 对单条 SELECT 结果集有 1000 行硬上限，超出会直接
		// 返报错（而非静默截断）。需要更大结果集时请在 SQL 里显式 LIMIT/分页，由调用方控制规模。
		data := map[string]interface{}{"results": stmts}

		// 多语句 / 单语句失败：server 仍返 code:0，把失败语句标成 ERROR 哨兵塞进 result。
		// 已落地的前序语句 + 失败语句构成 partial failure：逐条结果作为 ok:false 数据
		// 留在 stdout（机器可读）+ 非零退出信号，别让 agent 误判 ok:true 假成功。
		// pretty 模式 stdout 只打逐条 ✓/✗ 摘要（不再叠一份 JSON envelope），仅返回退出信号。
		if errIdx, errStmt, failed := findErrorSentinel(stmts); failed {
			if rctx.Format == "pretty" {
				renderSQLPretty(rctx.IO().Out, stmts)
				return output.PartialFailure(output.ExitAPI)
			}
			return rctx.OutPartialFailure(sqlStatementFailurePayload(stmts, errIdx, errStmt), nil)
		}

		rctx.OutFormat(data, nil, func(w io.Writer) {
			renderSQLPretty(w, stmts)
		})
		return nil
	},
}

// findErrorSentinel 在 statements 里找 ERROR 哨兵（server 失败时追加在失败语句位置）。
// 返回失败语句下标（0-based）、该 ERROR statement、是否命中。
func findErrorSentinel(stmts []map[string]interface{}) (int, map[string]interface{}, bool) {
	for i, s := range stmts {
		if common.GetString(s, "sql_type") == "ERROR" {
			return i, s, true
		}
	}
	return 0, nil, false
}

// sqlStatementFailurePayload 把 ERROR 哨兵整理成 partial-failure 的 stdout 数据。
//
// CLI 永远 DBA 模式（transactional=false），真机 boe 实证：失败语句之前的语句已逐条 auto-commit
// 落地，不存在外层事务回滚。因此 rolled_back=false、results 含全部逐条结果（ERROR 哨兵在
// 失败位置），note 提示用户别整批重跑（否则会重复写入）。
func sqlStatementFailurePayload(stmts []map[string]interface{}, errIdx int, errStmt map[string]interface{}) map[string]interface{} {
	code, msg := parseErrorSentinel(common.GetString(errStmt, "data"))
	stmtNo := errIdx + 1 // 1-based 给人看
	note := "no statements were applied; fix the SQL and re-run."
	if errIdx > 0 {
		note = fmt.Sprintf(
			"statements 1-%d were already applied (DBA mode auto-commits each statement); fix statement %d and re-run only the remaining statements.",
			errIdx, stmtNo)
	}
	return map[string]interface{}{
		"results":         stmts,
		"statement_index": errIdx,
		"error_code":      code,
		"error_message":   fmt.Sprintf("%s (at statement %d of %d)", msg, stmtNo, len(stmts)),
		"rolled_back":     false,
		"note":            note,
	}
}

// parseErrorSentinel 解析 ERROR 哨兵的 data（`{code,message}` JSON），返回数值 code 与 message。
// code 兼容 int / "k_dl_1300002" / 数字字符串多形态（复用 codeString），解析失败回退 0 / 原文。
func parseErrorSentinel(data string) (int, string) {
	if data == "" {
		return 0, "(unknown error)"
	}
	var e struct {
		Code    interface{} `json:"code"`
		Message string      `json:"message"`
	}
	if err := json.Unmarshal([]byte(data), &e); err != nil {
		return 0, data
	}
	code := 0
	if cs := codeString(e.Code); cs != "" {
		if n, convErr := strconv.Atoi(cs); convErr == nil {
			code = n
		}
	}
	if e.Message == "" {
		return code, "(unknown error)"
	}
	return code, e.Message
}

// buildDBSQLParams 构造 sql 接口的 query：env + 强制 transactional=false（DBA 模式）。
//
// CLI 永远走 DBA 模式，原子性由用户在 SQL 内显式 BEGIN/COMMIT 控制；不暴露 transactional flag 给用户。
func buildDBSQLParams(rctx *common.RuntimeContext) map[string]interface{} {
	return map[string]interface{}{
		"env":           rctx.Str("env"),
		"transactional": false,
	}
}

// buildDBSQLBody 构造 sql 接口的 body：仅 sql（来源由 Validate 归一化到 --sql）。
func buildDBSQLBody(rctx *common.RuntimeContext) map[string]interface{} {
	return map[string]interface{}{
		"sql": rctx.Str("sql"),
	}
}

// parseSQLResult 从 server result 字符串反序列化出 statements 数组，兼容两种 wire 形态：
//
//  1. 结构化形态：`[{"sql_type":"SELECT","data":"[...]","record_count":N}, ...]`
//     —— 每条 statement 含 sql_type / data / record_count / affected_rows 元数据。
//
//  2. 字符串数组形态：`["[{...rows...}]", "", ...]`
//     —— 每条 statement 一个字符串：SELECT 是 rows JSON、DML/DDL 是空串；
//     无 sql_type 元数据，CLI 端按内容形态推断（SELECT vs OK）。
//
// 解析失败时返回单元素 fallback `{sql_type:"RAW", data:resultStr}`，pretty 路径原样打。
func parseSQLResult(resultStr string) []map[string]interface{} {
	if resultStr == "" {
		return nil
	}

	// 形态 1：结构化数组（每元素是 object）
	var structured []map[string]interface{}
	if err := json.Unmarshal([]byte(resultStr), &structured); err == nil && isStructuredResult(structured) {
		return structured
	}

	// 形态 2：字符串数组（每元素是 rows JSON 或 ""）
	var legacy []string
	if err := json.Unmarshal([]byte(resultStr), &legacy); err == nil {
		out := make([]map[string]interface{}, 0, len(legacy))
		for _, rowsJSON := range legacy {
			out = append(out, normalizeLegacyStatement(rowsJSON))
		}
		return out
	}

	return []map[string]interface{}{{"sql_type": "RAW", "data": resultStr}}
}

// isStructuredResult 判断反序列化出来的 []map 是不是新形态：第一条元素含 sql_type 字段。
// 兼容场景：[]map 反序列化 legacy `[""]` 可能也能成（空 map），用 sql_type 存在性区分。
func isStructuredResult(stmts []map[string]interface{}) bool {
	if len(stmts) == 0 {
		return false
	}
	_, ok := stmts[0]["sql_type"]
	return ok
}

// normalizeLegacyStatement 把 legacy wire 一个字符串元素转成跟新形态一致的 map。
// 推断规则：data 是非空 rows 数组 → sql_type=SELECT；空串 / 空数组 → sql_type=OK（DML/DDL 老 wire 不可分）。
func normalizeLegacyStatement(rowsJSON string) map[string]interface{} {
	stmt := map[string]interface{}{
		"sql_type": "OK",
		"data":     rowsJSON,
	}
	trimmed := strings.TrimSpace(rowsJSON)
	if trimmed == "" || trimmed == "null" {
		return stmt
	}
	var rows []interface{}
	if err := json.Unmarshal([]byte(trimmed), &rows); err != nil {
		// 非 JSON 数组（理论上 server 不会返这种），按原样保留 sql_type=OK
		return stmt
	}
	// 是 JSON 数组 → 视作 SELECT，含 record_count
	stmt["sql_type"] = "SELECT"
	stmt["record_count"] = float64(len(rows))
	return stmt
}

// renderSQLPretty 按 statements 数量分单条 / 多条两种渲染路径。
func renderSQLPretty(w io.Writer, stmts []map[string]interface{}) {
	if len(stmts) == 0 {
		fmt.Fprintln(w, "(empty result)")
		return
	}
	if len(stmts) == 1 {
		renderSingleStatementPretty(w, stmts[0])
		return
	}
	renderMultiStatementPretty(w, stmts)
}

// renderSingleStatementPretty 单条 statement pretty（无 Statement header）。
func renderSingleStatementPretty(w io.Writer, s map[string]interface{}) {
	sqlType := common.GetString(s, "sql_type")
	switch {
	case sqlType == "SELECT":
		renderSelectRowsAsTable(w, common.GetString(s, "data"))
	case sqlType == "ERROR":
		// 单条就挂的极端场景：直接打 ERROR 行（跟多语句失败的最后一行格式一致）。
		fmt.Fprintln(w, "✗ "+errorSummary(common.GetString(s, "data")))
	case isDMLType(sqlType):
		// 结构化 wire 下 INSERT / UPDATE / DELETE / MERGE：✓ N row(s) <verb>
		fmt.Fprintln(w, "✓ "+dmlSummary(sqlType, s["affected_rows"]))
	case sqlType == "OK":
		// legacy wire 下 DML / DDL 都映射成 OK（老 wire 不带 sql_type 元数据，无法区分动词 / 行数）
		fmt.Fprintln(w, "✓ ok")
	default:
		// 其余皆 DDL：真机 boe 返细粒度动词 CREATE_TABLE / DROP_TABLE / ALTER_TABLE / TRUNCATE 等。
		fmt.Fprintln(w, "✓ DDL executed")
	}
}

// renderMultiStatementPretty 多条 statement pretty：
//   - 每条用 "Statement K: ✓ <summary>" / "Statement K: ✗ <error> [<code>]"
//   - SELECT 用 "Statement K: SELECT (N row(s))" 头 + 紧跟表格
//   - 末尾汇总：全部成功 "✓ N statements executed"；遇 ERROR 哨兵打「前序语句已落地」提示
//     （DBA 模式不回滚），失败本身由 Execute 升级成 typed error（exit 非 0）
func renderMultiStatementPretty(w io.Writer, stmts []map[string]interface{}) {
	failedIdx := -1
	successCount := 0
	for i, s := range stmts {
		sqlType := common.GetString(s, "sql_type")
		idx := i + 1
		switch {
		case sqlType == "ERROR":
			fmt.Fprintf(w, "Statement %d: ✗ %s\n", idx, errorSummary(common.GetString(s, "data")))
			failedIdx = i
		case sqlType == "SELECT":
			rc := intOrZero(s["record_count"])
			fmt.Fprintf(w, "Statement %d: SELECT (%d row%s)\n", idx, rc, plural(rc))
			renderSelectRowsAsTable(w, common.GetString(s, "data"))
			successCount++
		case isDMLType(sqlType):
			fmt.Fprintf(w, "Statement %d: ✓ %s\n", idx, dmlSummary(sqlType, s["affected_rows"]))
			successCount++
		case sqlType == "OK":
			fmt.Fprintf(w, "Statement %d: ✓ ok\n", idx)
			successCount++
		default:
			// DDL 族：CREATE_TABLE / DROP_TABLE / ALTER_TABLE / TRUNCATE / CREATE_INDEX ...
			fmt.Fprintf(w, "Statement %d: ✓ DDL executed\n", idx)
			successCount++
		}
		if i < len(stmts)-1 {
			fmt.Fprintln(w) // statements 间留空行
		}
	}
	fmt.Fprintln(w)
	if failedIdx >= 0 {
		// CLI 永远 DBA 模式（transactional=false），失败语句之前的语句已 auto-commit 落地，
		// 不存在整批回滚 —— 如实告诉用户，避免整批重跑导致重复写入。
		if successCount > 0 {
			fmt.Fprintf(w, "(statement %d failed; %d statement%s before it already applied — DBA mode auto-commits each)\n",
				failedIdx+1, successCount, plural(int64(successCount)))
		} else {
			fmt.Fprintf(w, "(statement %d failed; no statements applied)\n", failedIdx+1)
		}
	} else {
		fmt.Fprintf(w, "✓ %d statements executed\n", successCount)
	}
}

// renderSelectRowsAsTable 把 SELECT 的 data（rows JSON 数组字符串）解析并渲染成对齐表格。
// 空结果输出 "(0 rows)"。
func renderSelectRowsAsTable(w io.Writer, dataJSON string) {
	if dataJSON == "" || dataJSON == "[]" {
		fmt.Fprintln(w, "(0 rows)")
		return
	}
	var rows []map[string]interface{}
	if err := json.Unmarshal([]byte(dataJSON), &rows); err != nil {
		// 数据不符合预期 schema —— 原样打 fallback。
		fmt.Fprintln(w, dataJSON)
		return
	}
	if len(rows) == 0 {
		fmt.Fprintln(w, "(0 rows)")
		return
	}
	headers := collectColumns(rows)
	cells := make([][]string, 0, len(rows))
	for _, row := range rows {
		line := make([]string, 0, len(headers))
		for _, h := range headers {
			line = append(line, cellString(row[h]))
		}
		cells = append(cells, line)
	}
	renderAlignedTable(w, headers, cells)
}

// collectColumns 按首行字段顺序收集列名；首行 key 顺序由 encoding/json 反序列化决定（map 无序），
// 排序后保证输出稳定。列顺序在示例里跟 SQL SELECT 顺序一致——但 Go encoding/json 反序列化丢列序，
// 这里按字典序保证可重现，agent / 测试可稳定 assert。
func collectColumns(rows []map[string]interface{}) []string {
	set := map[string]struct{}{}
	for _, r := range rows {
		for k := range r {
			set[k] = struct{}{}
		}
	}
	cols := make([]string, 0, len(set))
	for k := range set {
		cols = append(cols, k)
	}
	sort.Strings(cols)
	return cols
}

// cellString 把任意 JSON value 转字符串显示（null → 空串；非字符串/数字 → JSON 编码）。
func cellString(v interface{}) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case bool:
		if x {
			return "true"
		}
		return "false"
	case float64:
		// 整数值不输出小数（id=101 而不是 101.000000）。
		if x == float64(int64(x)) {
			return fmt.Sprintf("%d", int64(x))
		}
		return fmt.Sprintf("%g", x)
	}
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(b)
}

// dmlSummary 把 sql_type + affected_rows 渲染成 "N row(s) <verb>" 字符串。
//
// 动词映射：INSERT → inserted / UPDATE → updated / DELETE → deleted / MERGE → merged。
// 未知 sql_type 默认 "affected"。
func dmlSummary(sqlType string, affectedRows interface{}) string {
	n := intOrZero(affectedRows)
	verb := dmlVerb(sqlType)
	return fmt.Sprintf("%d row%s %s", n, plural(n), verb)
}

// isDMLType 判断 sql_type 是否是行级 DML（带 affected_rows 语义）。
// 真机 boe wire：SELECT 走表格、INSERT/UPDATE/DELETE/MERGE 走行数摘要、其余（CREATE_TABLE /
// DROP_TABLE / ALTER_TABLE / TRUNCATE / CREATE_INDEX ...）一律按 DDL 处理。
func isDMLType(sqlType string) bool {
	switch strings.ToUpper(sqlType) {
	case "INSERT", "UPDATE", "DELETE", "MERGE":
		return true
	}
	return false
}

func dmlVerb(sqlType string) string {
	switch strings.ToUpper(sqlType) {
	case "INSERT":
		return "inserted"
	case "UPDATE":
		return "updated"
	case "DELETE":
		return "deleted"
	case "MERGE":
		return "merged"
	}
	return "affected"
}

func plural(n int64) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// errorSummary 从 ERROR 哨兵的 data 字段（{code, message} JSON）解析出 "message [code]" 形态。
// 解析失败时回退到原文。
func errorSummary(data string) string {
	if data == "" {
		return "(unknown error)"
	}
	var e struct {
		Code    interface{} `json:"code"`
		Message string      `json:"message"`
	}
	if err := json.Unmarshal([]byte(data), &e); err != nil {
		return data
	}
	codeStr := codeString(e.Code)
	if codeStr != "" {
		return fmt.Sprintf("%s [%s]", e.Message, codeStr)
	}
	return e.Message
}

// codeString 处理 code 字段在 wire 上可能是 int / "k_dl_1300015" / 数字字符串等多形态。
func codeString(c interface{}) string {
	switch x := c.(type) {
	case nil:
		return ""
	case string:
		// "k_dl_1300015" → 抽 1300015；纯数字保持原样。
		if strings.HasPrefix(x, "k_dl_") {
			return strings.TrimPrefix(x, "k_dl_")
		}
		return x
	case float64:
		return fmt.Sprintf("%d", int64(x))
	}
	return ""
}

// intOrZero 把 JSON number 转 int64；nil / 类型不匹配返回 0。
func intOrZero(raw interface{}) int64 {
	if n, ok := numericAsFloat(raw); ok {
		return int64(n)
	}
	return 0
}
