// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/larksuite/cli/internal/validate"
)

// automationBasePath 是触发器公网 OpenAPI 前缀。后端把触发器公网端点统一
// 到 apps 域 (spark/v1) 下，见后端方案 §接口行为——8 个端点全部位于
// /open-apis/spark/v1/apps/:app_id/triggers* 下。这里直接复用同包的
// apiBasePath 而不是自定义前缀，避免误用旧 apaas 域。
const automationBasePath = apiBasePath

func automationListPath(appID string) string {
	return fmt.Sprintf(automationBasePath+"/apps/%s/triggers", validate.EncodePathSegment(appID))
}

func automationItemPath(appID, name string) string {
	return fmt.Sprintf(automationBasePath+"/apps/%s/triggers/%s",
		validate.EncodePathSegment(appID), validate.EncodePathSegment(name))
}

func automationStatusPath(appID, name string) string {
	return automationItemPath(appID, name) + "/status"
}

func automationWebhookTokenStatusPath(appID, name string) string {
	return automationItemPath(appID, name) + "/webhook/token/status"
}

func automationWebhookTokenResetPath(appID, name string) string {
	return automationItemPath(appID, name) + "/webhook/token/reset"
}

func automationWebhookURLResetPath(appID, name string) string {
	return automationItemPath(appID, name) + "/webhook/url/reset"
}

// mapTriggerType 把 CLI 面向 Agent 的 kebab-case 类型转成 OpenAPI 的 snake_case。
func mapTriggerType(cliType string) (string, error) {
	switch cliType {
	case "cron":
		return "cron", nil
	case "record-change":
		return "record_change", nil
	case "webhook":
		return "webhook", nil
	case "feishu-approval":
		return "feishu_approval", nil
	default:
		return "", appsValidationParamError("--trigger-type",
			"unknown --trigger-type %q; want one of cron, record-change, webhook, feishu-approval", cliType)
	}
}

// validateCronExpr 校验五段式 cron 表达式，并兜底最小间隔 30 分钟。
// 这是给 Agent 的即时提示；后端 OpenAPI 层也会校验（ErrInvalidCronTab /
// ErrCronIntervalTooSmall），CLI 本地拦截只为更快反馈。
func validateCronExpr(expr string) error {
	fields := strings.Fields(strings.TrimSpace(expr))
	if len(fields) != 5 {
		return appsValidationParamError("--cron",
			"cron must have 5 fields (minute hour day month weekday), got %d in %q", len(fields), expr)
	}
	// 兜底 30 分钟下限：仅拦截可本地判定的高频分钟字段（"*" 或 "*/n" n<30）。
	minute := fields[0]
	if minute == "*" {
		return appsValidationParamError("--cron",
			"cron minute field '*' means every minute; minimum interval is 30 minutes")
	}
	if strings.HasPrefix(minute, "*/") {
		n, err := strconv.Atoi(strings.TrimPrefix(minute, "*/"))
		if err == nil && n < 30 {
			return appsValidationParamError("--cron",
				"cron interval */%d minutes is below the 30-minute minimum", n)
		}
	}
	return nil
}

const defaultCronTimezone = "Asia/Shanghai"

// approvalStatusSets 是 feishu-approval 两种 event-type 各自的合法状态集合。
// 后端 OpenAPI 不逐值校验 status，CLI 本地分桶校验是唯一保障。
var approvalStatusSets = map[string]map[string]struct{}{
	"approval_instance": setOf("PENDING", "APPROVED", "REJECTED", "CANCELED", "DELETED", "REVERTED", "OVERTIME_CLOSE", "OVERTIME_RECOVER"),
	"approval_task":     setOf("REVERTED", "PENDING", "APPROVED", "REJECTED", "TRANSFERRED", "ROLLBACK", "DONE", "OVERTIME_CLOSE", "OVERTIME_RECOVER"),
}

func setOf(items ...string) map[string]struct{} {
	m := make(map[string]struct{}, len(items))
	for _, it := range items {
		m[it] = struct{}{}
	}
	return m
}

// buildCronCondition 产出 OpenAPI 层 cron_condition body。缺省时区补 Asia/Shanghai。
func buildCronCondition(expr, tz string) (map[string]interface{}, error) {
	if err := validateCronExpr(expr); err != nil {
		return nil, err
	}
	if strings.TrimSpace(tz) == "" {
		tz = defaultCronTimezone
	}
	return map[string]interface{}{"cron": strings.TrimSpace(expr), "timezone": tz}, nil
}

// buildRecordChangeCondition 产出 record_change_condition body；event 大写化。
func buildRecordChangeCondition(table, event string, fields []string) (map[string]interface{}, error) {
	if strings.TrimSpace(table) == "" {
		return nil, appsValidationParamError("--table", "--table is required for record-change triggers")
	}
	ev := strings.ToUpper(strings.TrimSpace(event))
	if ev == "" {
		return nil, appsValidationParamError("--event", "--event is required for record-change triggers (INSERT/UPDATE/UPSERT/DELETE)")
	}
	cond := map[string]interface{}{"event": ev, "table": strings.TrimSpace(table)}
	if len(fields) > 0 {
		cond["fields"] = fields
	}
	return cond, nil
}

// buildWebhookCondition 产出 webhook_condition body（IP 白名单，可空）。
func buildWebhookCondition(ipList []string) map[string]interface{} {
	cond := map[string]interface{}{}
	if ipList != nil {
		cond["white_ip_list"] = ipList
	}
	return cond
}

// validateApprovalStatuses 按 event-type 分桶校验状态枚举合法性。
func validateApprovalStatuses(eventType string, statuses []string) error {
	set, ok := approvalStatusSets[eventType]
	if !ok {
		return appsValidationParamError("--event-type",
			"unknown --event-type %q; want approval_task or approval_instance", eventType)
	}
	if len(statuses) == 0 {
		flag := statusFlagFor(eventType)
		return appsValidationParamError("--"+flag,
			"--%s is required for event-type %q (at least one status)", flag, eventType)
	}
	for _, s := range statuses {
		if _, valid := set[strings.ToUpper(strings.TrimSpace(s))]; !valid {
			// 列出该 event-type 的合法状态集合，便于 Agent 修正。
			return appsValidationParamError("--"+statusFlagFor(eventType),
				"status %q is not valid for event-type %q; valid values: %s",
				s, eventType, sortedStatusList(set))
		}
	}
	return nil
}

// sortedStatusList 返回状态集合的稳定排序、逗号分隔字符串，用于错误提示。
func sortedStatusList(set map[string]struct{}) string {
	out := make([]string, 0, len(set))
	for s := range set {
		out = append(out, s)
	}
	sort.Strings(out)
	return strings.Join(out, ", ")
}

func statusFlagFor(eventType string) string {
	if eventType == "approval_task" {
		return "task-status"
	}
	return "instance-status"
}

// buildApprovalCondition 产出 feishu_approval_condition body。approval_code 可选：
// 空则省略（匹配所有审批定义），不发空串。
func buildApprovalCondition(code, eventType string, statuses []string) (map[string]interface{}, error) {
	if err := validateApprovalStatuses(eventType, statuses); err != nil {
		return nil, err
	}
	cond := map[string]interface{}{"event_type": eventType, "status": statuses}
	if strings.TrimSpace(code) != "" {
		cond["approval_code"] = strings.TrimSpace(code)
	}
	return cond, nil
}

// statusBodyFromAction 把 enable/disable 命令映射到同一 status 端点的 body。
func statusBodyFromAction(enable bool) map[string]interface{} {
	if enable {
		return map[string]interface{}{"status": "enabled"}
	}
	return map[string]interface{}{"status": "disabled"}
}

// redactWebhookToken 返回触发器详情的副本，把 trigger_condition.token_value 抹成 nil。
// 内部后端读结构可能解密返回明文 token，脱敏是 CLI/公网层职责，
// 不修改入参。非 webhook 或无 token_value 时原样返回浅拷贝。
func redactWebhookToken(info map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(info))
	for k, v := range info {
		out[k] = v
	}
	tc, ok := info["trigger_condition"].(map[string]interface{})
	if !ok {
		return out
	}
	redactedTC := make(map[string]interface{}, len(tc))
	for k, v := range tc {
		if k == "token_value" {
			redactedTC[k] = nil
			continue
		}
		redactedTC[k] = v
	}
	out["trigger_condition"] = redactedTC
	return out
}
