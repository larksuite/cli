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
// 到 apps 域 (spark/v1) 下，8 个端点全部位于
// /open-apis/spark/v1/apps/:app_id/triggers* 下。这里直接复用同包的
// apiBasePath 而不是自定义前缀，避免误用早期的备选前缀。
const automationBasePath = apiBasePath

func automationListPath(appID string) string {
	return fmt.Sprintf(automationBasePath+"/apps/%s/triggers", validate.EncodePathSegment(appID))
}

func automationItemPath(appID, name string) string {
	return fmt.Sprintf(automationBasePath+"/apps/%s/triggers/%s",
		validate.EncodePathSegment(appID), validate.EncodePathSegment(name))
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
	if strings.Contains(minute, ",") {
		parts := strings.Split(minute, ",")
		vals := make([]int, 0, len(parts))
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if n, err := strconv.Atoi(p); err == nil {
				vals = append(vals, n)
			}
		}
		if len(vals) >= 2 {
			sort.Ints(vals)
			minGap := 60
			for i := 1; i < len(vals); i++ {
				if gap := vals[i] - vals[i-1]; gap < minGap {
					minGap = gap
				}
			}
			if wrapGap := vals[0] + 60 - vals[len(vals)-1]; wrapGap < minGap {
				minGap = wrapGap
			}
			if minGap < 30 {
				return appsValidationParamError("--cron",
					"cron minute list %q has %d-min interval; minimum interval is 30 minutes", minute, minGap)
			}
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

// recordChangeEventSet 是 record-change 触发器合法 event 枚举。
// 4 个值来自需求定义。CLI 本地做白名单校验，
// 避免后端 event 字段校验缺失导致的"接受任意字符串→触发器永不触发"问题。
var recordChangeEventSet = setOf("INSERT", "UPDATE", "UPSERT", "DELETE")

// buildRecordChangeCondition 产出 record_change_condition body；event 大写化。
func buildRecordChangeCondition(table, event string, fields []string) (map[string]interface{}, error) {
	if strings.TrimSpace(table) == "" {
		return nil, appsValidationParamError("--table", "--table is required for record-change triggers")
	}
	ev := strings.ToUpper(strings.TrimSpace(event))
	if ev == "" {
		return nil, appsValidationParamError("--event", "--event is required for record-change triggers (INSERT/UPDATE/UPSERT/DELETE)")
	}
	if _, valid := recordChangeEventSet[ev]; !valid {
		return nil, appsValidationParamError("--event",
			"--event %q is not a valid record-change event; want one of INSERT, UPDATE, UPSERT, DELETE", event)
	}
	cond := map[string]interface{}{"event": ev, "table": strings.TrimSpace(table)}
	if len(fields) > 0 {
		cond["fields"] = fields
	}
	return cond, nil
}

// buildWebhookCondition 产出 webhook_condition body。white_ip_list 在后端契约
// 里是 required，因此当 CLI 侧未传 --white-ip-list 时也发一个空数组，避免后端
// 拒收；显式空数组 `[]` 与"不限来源 IP"语义一致（呼应无鉴权公网回调告警）。
func buildWebhookCondition(ipList []string) map[string]interface{} {
	if ipList == nil {
		ipList = []string{}
	}
	return map[string]interface{}{"white_ip_list": ipList}
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

// redactWebhookToken returns a shallow copy of a trigger view with any
// trigger_condition.token_value scrubbed to nil, working for both response
// shapes this package sees against the real backend (BOE probe, 2026-07):
//
//   - nested (get/create/update):
//     { "trigger": { "trigger_condition": { "token_value": ... } } }
//   - flat (list items):
//     { "trigger_condition": { "token_value": ... } }
//
// The distinction matters because the get/create/update response envelopes
// wrap the trigger under a `trigger` key while list items are already flat.
// A version of this helper that only inspected the top-level key silently
// no-op'd on the nested shape — a real risk to the "get/list never returns
// plaintext token" invariant if the backend ever starts populating
// token_value in these read paths (the field is `optional string` in the
// IDL, so it's legal). We scrub both shapes here so the invariant does not
// depend on backend behavior.
//
// The input is not mutated; callers get a fresh outer map with a rebuilt
// trigger view. Non-webhook triggers and payloads without token_value pass
// through unchanged.
func redactWebhookToken(info map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(info))
	for k, v := range info {
		out[k] = v
	}
	// Nested shape: rebuild info["trigger"] with a scrubbed trigger_condition.
	if wrapped, ok := info["trigger"].(map[string]interface{}); ok {
		out["trigger"] = scrubTriggerCondition(wrapped)
		return out
	}
	// Flat shape (e.g. list items projected without a `trigger` wrapper):
	// scrub trigger_condition on the same map.
	if _, hasFlat := info["trigger_condition"].(map[string]interface{}); hasFlat {
		return scrubTriggerCondition(out)
	}
	return out
}

// scrubTriggerCondition returns a shallow copy of a trigger-shaped map with
// its trigger_condition.token_value replaced by nil. Called by
// redactWebhookToken for each shape it recognizes.
func scrubTriggerCondition(trigger map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(trigger))
	for k, v := range trigger {
		out[k] = v
	}
	tc, ok := out["trigger_condition"].(map[string]interface{})
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
