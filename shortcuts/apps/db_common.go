// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

// ── db 环境 flag：--environment 是唯一受理名；旧名 --env 已移除 ──
//
// 硬改名：标准名 --environment（带默认/枚举）正常注册并受理；旧名 --env 仅注册为隐藏 flag，
// 目的是「传了能被识别并给出清晰报错」而非继续受理——一旦显式传 --env，在 Validate 阶段直接
// 返回 validation 错、指向 --environment。所有 DryRun/Execute 经 dbEnv() 只读 --environment。

// dbEnvFlags 返回环境 flag 对，供各 db 命令 append 进自己的 Flags。
func dbEnvFlags(def string, enum []string, desc string) []common.Flag {
	return []common.Flag{
		{Name: "environment", Default: def, Enum: enum, Desc: desc},
		{Name: "env", Hidden: true, Desc: "removed: use --environment"},
	}
}

// dbEnv 取环境值：只认标准 --environment（含其默认值）；旧名 --env 不再受理（见 rejectLegacyEnvFlag）。
func dbEnv(rctx *common.RuntimeContext) string {
	return rctx.Str("environment")
}

// dbEnvParams 把 env 并入 params：仅当显式指定了环境（非空）才带 env 键；未指定（空）时
// 省略该键，由服务端按应用多环境状态自动选分支（多环境→dev，单环境→online）。与家族对
// 空可选参数的 omit-empty 约定一致——不发空串，wire 上真正不带 env。原样返回同一个 map 便于链式。
func dbEnvParams(rctx *common.RuntimeContext, params map[string]interface{}) map[string]interface{} {
	if env := dbEnv(rctx); env != "" {
		params["env"] = env
	}
	return params
}

// rejectLegacyEnvFlag 在 Validate 阶段拦截已移除的 --env：显式传了就报清晰的 validation 错，指向 --environment。
func rejectLegacyEnvFlag(rctx *common.RuntimeContext) error {
	if rctx.Changed("env") {
		return errs.NewValidationError(errs.SubtypeInvalidArgument,
			"--env is no longer supported; use --environment instead").WithParam("--env")
	}
	return nil
}

// pollUntil 轮询异步任务直到 check 判定终态。async migrate/recovery 用：dataloom 立即返
// task_id/preview_request_id，CLI 自己 poll（避免单连接长挂被网关/SDK 30s 中断）。
// 首次立即 fetch（不睡）；check 返 done→返回；返 err→透传（失败终态）；否则按 interval 间隔重试至 maxWait。
func pollUntil(ctx context.Context, interval, maxWait time.Duration,
	fetch func() (map[string]interface{}, error),
	check func(map[string]interface{}) (done bool, err error)) (map[string]interface{}, error) {
	maxAttempts := int(maxWait / interval)
	if maxAttempts < 1 {
		maxAttempts = 1
	}
	for i := 0; ; i++ {
		data, err := fetch()
		if err != nil {
			return nil, err
		}
		done, cerr := check(data)
		if cerr != nil {
			return nil, cerr
		}
		if done {
			return data, nil
		}
		if i+1 >= maxAttempts {
			// async 任务多半还在服务端推进，poll 超时是可重试的——标 retryable 让 agent 重新轮询而非放弃。
			return nil, errs.NewNetworkError(errs.SubtypeNetworkTimeout, "timed out waiting for completion after %s", maxWait).WithRetryable()
		}
		select {
		case <-ctx.Done():
			return nil, errs.NewNetworkError(errs.SubtypeNetworkTransport, "cancelled while waiting").WithCause(ctx.Err())
		case <-time.After(interval):
		}
	}
}

// URL helpers for the db CLI commands.

// appTablesPath 返回 app db 表列表 URL（复用存量「获取数据表列表」接口）。
func appTablesPath(appID string) string {
	return fmt.Sprintf("%s/apps/%s/tables", apiBasePath, validate.EncodePathSegment(appID))
}

// appTablePath 返回单个 app db 表详情 URL（复用存量「获取数据表详细信息」接口）。
func appTablePath(appID, table string) string {
	return appTablesPath(appID) + "/" + validate.EncodePathSegment(table)
}

// appSQLPath 返回 app db SQL 执行 URL（复用存量「执行 SQL」接口）。
func appSQLPath(appID string) string {
	return fmt.Sprintf("%s/apps/%s/sql_commands", apiBasePath, validate.EncodePathSegment(appID))
}

// appDbEnvCreatePath 返回 app db 环境创建 URL（服务端接口名仍为 db_dev_init）。
func appDbEnvCreatePath(appID string) string {
	return fmt.Sprintf("%s/apps/%s/db_dev_init", apiBasePath, validate.EncodePathSegment(appID))
}

// ── 多环境发布（env diff/migrate）/ 数据恢复（recovery）/ 配额 路由 ──

// appEnvMigratePath 返回 dev→online 发布（预览/落地共用）URL：db/env_migrate。
func appEnvMigratePath(appID string) string {
	return fmt.Sprintf("%s/apps/%s/db/env_migrate", apiBasePath, validate.EncodePathSegment(appID))
}

// appEnvMigrateStatusPath 返回发布异步任务状态查询 URL：db/env_migrate_status。
func appEnvMigrateStatusPath(appID string) string {
	return fmt.Sprintf("%s/apps/%s/db/env_migrate_status", apiBasePath, validate.EncodePathSegment(appID))
}

// appRecoveryPath 返回 PITR 数据恢复（预览/落地共用）URL：db/env_recovery。
func appRecoveryPath(appID string) string {
	return fmt.Sprintf("%s/apps/%s/db/env_recovery", apiBasePath, validate.EncodePathSegment(appID))
}

// appRecoveryDiffStatusPath 返回恢复预览（diff）异步状态查询 URL：db/env_recovery_diff_status。
func appRecoveryDiffStatusPath(appID string) string {
	return fmt.Sprintf("%s/apps/%s/db/env_recovery_diff_status", apiBasePath, validate.EncodePathSegment(appID))
}

// appRecoveryApplyStatusPath 返回恢复落地异步状态查询 URL：db/env_recovery_apply_status。
func appRecoveryApplyStatusPath(appID string) string {
	return fmt.Sprintf("%s/apps/%s/db/env_recovery_apply_status", apiBasePath, validate.EncodePathSegment(appID))
}

// appDbQuotaPath 返回 db 配额查询 URL：db/quota。
func appDbQuotaPath(appID string) string {
	return fmt.Sprintf("%s/apps/%s/db/quota", apiBasePath, validate.EncodePathSegment(appID))
}

// ── 变更追溯（changelog / audit）路由 ──

// appChangelogListPath 返回 DDL 变更记录列表 URL：db/changelog_list。
func appChangelogListPath(appID string) string {
	return fmt.Sprintf("%s/apps/%s/db/changelog_list", apiBasePath, validate.EncodePathSegment(appID))
}

// appAuditStatusPath 返回表审计开关状态查询 URL：db/audit_status。
func appAuditStatusPath(appID string) string {
	return fmt.Sprintf("%s/apps/%s/db/audit_status", apiBasePath, validate.EncodePathSegment(appID))
}

// appAuditSetPath 返回表审计开关设置 URL：db/audit_set。
func appAuditSetPath(appID string) string {
	return fmt.Sprintf("%s/apps/%s/db/audit_set", apiBasePath, validate.EncodePathSegment(appID))
}

// appAuditListPath 返回行级审计事件列表 URL：db/audit_list。
func appAuditListPath(appID string) string {
	return fmt.Sprintf("%s/apps/%s/db/audit_list", apiBasePath, validate.EncodePathSegment(appID))
}

// operatorRef 是 operator 的 {id,name}。后端用 JSON 字符串内嵌透传，CLI parse：
// json 输出还原成对象（下游能区分同名用户），pretty 只取 name。
type operatorRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// parseOperator 解析 operator 字符串：空→nil；非 JSON→{raw,raw}；JSON→{id,name}（name 空兜底 id）。
func parseOperator(raw string) *operatorRef {
	s := strings.TrimSpace(raw)
	if s == "" {
		return nil
	}
	if !strings.HasPrefix(s, "{") {
		return &operatorRef{ID: s, Name: s}
	}
	var o operatorRef
	if json.Unmarshal([]byte(s), &o) != nil {
		return &operatorRef{ID: s, Name: s}
	}
	if o.Name == "" {
		o.Name = o.ID
	}
	return &o
}

// operatorName 取 operator 的展示名（pretty），空用 "—"。
func operatorName(op *operatorRef) string {
	if op == nil || op.Name == "" {
		return "—"
	}
	return op.Name
}

// safeParseJSON 把 before/after 的 JSON 字符串还原成结构化对象供下游消费；失败时透传原始串。
func safeParseJSON(s string) interface{} {
	var v interface{}
	if json.Unmarshal([]byte(s), &v) == nil {
		return v
	}
	return s
}

// appDataImportPath 返回 db 数据导入 URL（新增 db/ 域段路由）。
func appDataImportPath(appID string) string {
	return fmt.Sprintf("%s/apps/%s/db/data_import", apiBasePath, validate.EncodePathSegment(appID))
}

// appDataExportPath 返回 db 数据导出 URL（返原始字节）。
func appDataExportPath(appID string) string {
	return fmt.Sprintf("%s/apps/%s/db/data_export", apiBasePath, validate.EncodePathSegment(appID))
}

// appTableRecordsPath 返回数据表记录列表 URL（复用 GetAppTableRecordList，其 total 即符合条件的记录总数）。
func appTableRecordsPath(appID, table string) string {
	return appTablePath(appID, table) + "/records"
}

// resolveDataFormat 由文件扩展名推断数据格式。lark-cli 的 --format 已被框架占用（输出渲染），
// 故数据格式从文件名推断：import 接受 csv/json，export 还接受 sql。
func resolveDataFormat(ext string, allowSQL bool) (string, error) {
	raw := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(ext)), ".")
	switch raw {
	case "csv", "json":
		return raw, nil
	case "sql":
		if allowSQL {
			return "sql", nil
		}
	}
	if allowSQL {
		return "", errs.NewValidationError(errs.SubtypeInvalidArgument, "unsupported data format %q (file must end in .csv, .json or .sql)", raw)
	}
	return "", errs.NewValidationError(errs.SubtypeInvalidArgument, "unsupported data format %q (file must end in .csv or .json)", raw)
}

// countDataRows 粗估数据行数（用于导入上限校验、导出兜底计数）。
// csv：非空行数 - 1（表头）；json：顶层数组长度，非数组算 1，解析失败算 0。
func countDataRows(body []byte, format string) int {
	if format == "csv" {
		lines := 0
		for _, ln := range strings.Split(string(body), "\n") {
			if strings.TrimRight(ln, "\r") != "" {
				lines++
			}
		}
		if lines > 0 {
			return lines - 1
		}
		return 0
	}
	var arr []json.RawMessage
	if err := json.Unmarshal(body, &arr); err == nil {
		return len(arr)
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(body, &obj); err == nil {
		return 1
	}
	return 0
}

// requireAppID trims --app-id and rejects blank, returning a uniform validation error.
func requireAppID(raw string) (string, error) {
	id := strings.TrimSpace(raw)
	if id == "" {
		return "", errs.NewValidationError(errs.SubtypeInvalidArgument, "--app-id is required").WithParam("--app-id")
	}
	return id, nil
}
