// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
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
//
// Minute field accepted forms:
//   - "N" (single value 0-59)
//   - "N,M,..." (comma list of single values; min pairwise gap incl. wrap >= 30)
//   - "*/N" (step from 0; N must be >= 30)
//
// Anything else (ranges like "N-M", stepped ranges like "N-M/S",
// range shorthands like "0/10", question marks) is rejected up-front with a
// typed --cron error. A previous version accepted "1-59/10" through the
// fallthrough because none of the three matchers claimed it, and the caller
// only found out the interval was 10 minutes when the backend rejected it
// (or worse, silently accepted a schedule the operator did not intend).
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
		if err != nil || n < 1 || n > 59 {
			return appsValidationParamError("--cron",
				"cron minute step %q must be an integer 1..59", minute)
		}
		// */N in cron expands to [0, N, 2N, ...] within 0..59, then wraps to 0
		// of the next hour. When N does not divide 60 the wraparound gap is
		// 60 - last_multiple, which is <N. For the 30-minute floor to hold on
		// every gap (in-hour AND wrap), *only* N=30 works: */30 fires at :00
		// and :30, gaps [30, 30]. */45 fires at :00 and :45, gaps [45, 15] —
		// the 15-min wraparound gap violates the floor. All 31..59 fail the
		// same way (small wraparound remainder); 1..29 fail the in-hour gap.
		if n != 30 {
			return appsValidationParamError("--cron",
				"cron step */%d produces a gap below the 30-minute minimum "+
					"(only */30 keeps every gap >=30 including the wraparound); "+
					"use */30, or an explicit list like '0,30'", n)
		}
		return nil
	}
	if strings.Contains(minute, ",") {
		parts := strings.Split(minute, ",")
		vals := make([]int, 0, len(parts))
		for _, p := range parts {
			p = strings.TrimSpace(p)
			n, err := strconv.Atoi(p)
			if err != nil || n < 0 || n > 59 {
				return appsValidationParamError("--cron",
					"cron minute list entry %q must be an integer 0..59", p)
			}
			vals = append(vals, n)
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
		return nil
	}
	// Bare single value fallthrough. Reject range/step-range/anything else so
	// forms like "1-59/10" (10-min interval) and "0/10" (10-min interval)
	// cannot bypass the 30-minute floor. The backend enforces its own cron
	// rules, but the CLI stays strict about which forms it accepts so callers
	// get an early, unambiguous error.
	if n, err := strconv.Atoi(minute); err == nil && n >= 0 && n <= 59 {
		return nil
	}
	return appsValidationParamError("--cron",
		"unsupported cron minute syntax %q; use N (0..59), N,M,... (min gap >=30), or */N (N>=30)", minute)
}

const defaultCronTimezone = "Asia/Shanghai"

// Local length limits mirrored from the flag help ("--name <=100 chars",
// "--description <=50 chars"). Enforcing here catches a violation before the
// API round-trip and returns a typed --name / --description error, whereas
// hitting the backend surfaces an opaque business error the agent has to
// diagnose. Constants (not magic numbers) so the flag help and the check
// share one source of truth if the backend ever renegotiates the limits.
const (
	automationNameMaxLen        = 100
	automationDescriptionMaxLen = 50
)

// validateAutomationNameLen guards against a --name that would be rejected by
// the backend on length. Empty is intentionally permitted here — the required
// check lives in the create Validate hook (which fires first) and in Update
// the flag is not required at all. Counts runes, not bytes: the flag help
// documents "<=100 chars", and Chinese/emoji names would be silently rejected
// well below the char limit if we counted UTF-8 bytes.
func validateAutomationNameLen(name string) error {
	if n := utf8.RuneCountInString(name); n > automationNameMaxLen {
		return appsValidationParamError("--name",
			"--name must be at most %d chars, got %d", automationNameMaxLen, n)
	}
	return nil
}

// validateAutomationDescriptionLen guards --description length; empty passes.
// Counts runes for the same reason as validateAutomationNameLen.
func validateAutomationDescriptionLen(desc string) error {
	if n := utf8.RuneCountInString(desc); n > automationDescriptionMaxLen {
		return appsValidationParamError("--description",
			"--description must be at most %d chars, got %d", automationDescriptionMaxLen, n)
	}
	return nil
}

// conditionFlagFamily maps each condition-carrying flag to the trigger-type
// family it belongs to. Used by create/update to reject cross-type flag
// combinations up-front (e.g. --trigger-type webhook --cron '0 9 * * *'
// silently dropped --cron before this guard).
//
// --timezone is a modifier on --cron, so it lives in the cron family.
// --description is trigger-type-agnostic and NOT in this map — it can pair
// with any type on create and can appear alone on update.
var conditionFlagFamily = map[string]string{
	"cron":            "cron",
	"timezone":        "cron",
	"table":           "record-change",
	"event":           "record-change",
	"fields":          "record-change",
	"white-ip-list":   "webhook",
	"event-type":      "feishu-approval",
	"instance-status": "feishu-approval",
	"task-status":     "feishu-approval",
	"approval-code":   "feishu-approval",
}

// flagIsSet reports whether a condition-carrying flag has a caller-provided
// value. string and string-array types both need to be probed; a nil / empty
// value counts as unset.
func flagIsSet(rctx *common.RuntimeContext, name string) bool {
	if v := strings.TrimSpace(rctx.Str(name)); v != "" {
		return true
	}
	if arr := rctx.StrArray(name); len(arr) > 0 {
		return true
	}
	return false
}

// familiesInUse returns the set of trigger-type families whose condition flags
// the caller has set on this invocation. A trigger has exactly one type, so
// legitimate condition writes involve at most one family; anything else is a
// user mistake that must not slip through to the backend.
func familiesInUse(rctx *common.RuntimeContext) map[string]string {
	out := map[string]string{}
	for flag, family := range conditionFlagFamily {
		if flagIsSet(rctx, flag) {
			out[family] = flag
		}
	}
	return out
}

// familiesMixedList renders a comma-separated, sorted list of families
// currently in use for inclusion in the multi-family rejection error. Stable
// order keeps the error message deterministic across Go's random map
// iteration.
func familiesMixedList(families map[string]string) string {
	names := make([]string, 0, len(families))
	for name := range families {
		names = append(names, name)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

// rejectCrossFamilyCondFlags rejects any condition flag that does not belong
// to `wantFamily`. Returns a typed --<flag> error naming the first offending
// flag encountered. Deterministic ordering (iterated over a stable slice)
// keeps the error message reproducible for tests.
func rejectCrossFamilyCondFlags(rctx *common.RuntimeContext, wantFamily string) error {
	// Stable iteration order for a deterministic Param on error.
	order := []string{
		"cron", "timezone",
		"table", "event", "fields",
		"white-ip-list",
		"event-type", "instance-status", "task-status", "approval-code",
	}
	for _, flag := range order {
		if conditionFlagFamily[flag] != wantFamily && flagIsSet(rctx, flag) {
			return appsValidationParamError("--"+flag,
				"--%s belongs to trigger-type %q, not %q; drop it or change --trigger-type",
				flag, conditionFlagFamily[flag], wantFamily)
		}
	}
	return nil
}

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
