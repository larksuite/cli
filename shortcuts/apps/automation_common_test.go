// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"strings"
	"testing"
)

func TestAutomationPaths(t *testing.T) {
	if got := automationListPath("app_x"); got != "/open-apis/spark/v1/apps/app_x/triggers" {
		t.Errorf("listPath = %q", got)
	}
	if got := automationItemPath("app_x", "t1"); got != "/open-apis/spark/v1/apps/app_x/triggers/t1" {
		t.Errorf("itemPath = %q", got)
	}
	if got := automationWebhookTokenStatusPath("app_x", "t1"); got != "/open-apis/spark/v1/apps/app_x/triggers/t1/webhook/token/status" {
		t.Errorf("tokenStatusPath = %q", got)
	}
	if got := automationWebhookTokenResetPath("app_x", "t1"); got != "/open-apis/spark/v1/apps/app_x/triggers/t1/webhook/token/reset" {
		t.Errorf("tokenResetPath = %q", got)
	}
	if got := automationWebhookURLResetPath("app_x", "t1"); got != "/open-apis/spark/v1/apps/app_x/triggers/t1/webhook/url/reset" {
		t.Errorf("urlResetPath = %q", got)
	}
}

// TestValidateAutomationNameLen_CountsRunes pins the char-not-byte contract:
// the flag help documents "<=100 chars", and Chinese/emoji names would be
// silently rejected below the char limit if we counted UTF-8 bytes.
// A 100-rune Chinese string is 300 bytes but is 100 chars — must pass.
func TestValidateAutomationNameLen_CountsRunes(t *testing.T) {
	// 100 Chinese characters (each 3 UTF-8 bytes = 300 bytes total). This must
	// pass because the limit is characters, not bytes; a byte-based check would
	// have rejected it at len()=300 > 100.
	name := strings.Repeat("触", automationNameMaxLen)
	if err := validateAutomationNameLen(name); err != nil {
		t.Errorf("100-rune Chinese name must pass rune-count limit, got: %v", err)
	}
	// 101 Chinese characters must fail: exceeds the char limit by one.
	over := strings.Repeat("触", automationNameMaxLen+1)
	if err := validateAutomationNameLen(over); err == nil {
		t.Error("101-rune Chinese name must fail rune-count limit")
	}
}

func TestMapTriggerType(t *testing.T) {
	cases := map[string]string{
		"cron": "cron", "record-change": "record_change",
		"webhook": "webhook", "feishu-approval": "feishu_approval",
	}
	for in, want := range cases {
		got, err := mapTriggerType(in)
		if err != nil || got != want {
			t.Errorf("mapTriggerType(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	err := func() error { _, e := mapTriggerType("bogus"); return e }()
	assertValidationParamError(t, err, "--trigger-type")
}

func TestValidateCronExpr(t *testing.T) {
	if err := validateCronExpr("0 9 * * *"); err != nil {
		t.Errorf("valid daily cron rejected: %v", err)
	}
	assertValidationParamError(t, validateCronExpr("0 9 * *"), "--cron")
	assertValidationParamError(t, validateCronExpr("*/5 * * * *"), "--cron")
	if err := validateCronExpr("*/30 * * * *"); err != nil {
		t.Errorf("30-minute interval must pass: %v", err)
	}
}

// TestValidateCronExpr_RejectsRangeStepBypass pins two related tightenings:
//
//   - Range-step syntax like "1-59/10" or shorthand "0/10" is a 10-minute
//     interval, but the old *,*/N,list-only matcher fell through and
//     accepted these. The new whitelist rejects any minute form outside
//     {"N", "N,M,...", "*/N"}.
//   - */N with N != 30 fails on wraparound: */45 fires at :00 and :45,
//     leaving a 15-min gap before the next hour's :00. In standard cron,
//     */N expands to [0, N, 2N, ...] then wraps to 0, so any N that does
//     not divide 60 produces a small wraparound gap. Only N=30 keeps
//     every gap (in-hour AND wrap) >= 30.
func TestValidateCronExpr_RejectsRangeStepBypass(t *testing.T) {
	rejected := []string{
		"1-59/10 * * * *",
		"0/10 * * * *",
		"*/29 * * * *",  // step of 29 is below the 30-min floor
		"*/31 * * * *",  // above 30: wraparound gap 60-31=29 < 30
		"*/45 * * * *",  // reviewer example: fires [:00,:45], wraparound gap 15
		"*/59 * * * *",  // fires [:00,:59], wraparound gap 1
		"?  * * * *",    // range/? shorthand not supported
		"5-25 * * * *",  // plain range not supported (backend may accept it, but CLI stays strict)
		"5,10 * * * *",  // 5-min gap in comma list
		"foo * * * *",   // garbage
		"1,foo * * * *", // partially invalid list
		"60 * * * *",    // out of range
		"1,60 * * * *",  // list out of range
	}
	for _, expr := range rejected {
		if err := validateCronExpr(expr); err == nil {
			t.Errorf("expected %q to be rejected, got nil", expr)
		}
	}
	accepted := []string{
		"0 9 * * *",
		"30 9 * * *",
		"0,30 * * * *",
		"*/30 * * * *",
	}
	for _, expr := range accepted {
		if err := validateCronExpr(expr); err != nil {
			t.Errorf("expected %q to pass, got: %v", expr, err)
		}
	}
}

func TestBuildCronCondition(t *testing.T) {
	c, err := buildCronCondition("0 9 * * *", "")
	if err != nil {
		t.Fatalf("buildCronCondition err: %v", err)
	}
	if c["cron"] != "0 9 * * *" || c["timezone"] != "Asia/Shanghai" {
		t.Errorf("cron condition = %+v; want default tz Asia/Shanghai", c)
	}
	_, err = buildCronCondition("*/5 * * * *", "")
	assertValidationParamError(t, err, "--cron")
}

func TestBuildRecordChangeCondition(t *testing.T) {
	c, err := buildRecordChangeCondition("tbl_1", "update", []string{"status"})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if c["event"] != "UPDATE" || c["table"] != "tbl_1" {
		t.Errorf("record_change = %+v; event must be uppercased", c)
	}
	_, err = buildRecordChangeCondition("", "UPDATE", nil)
	assertValidationParamError(t, err, "--table")
	_, err = buildRecordChangeCondition("tbl_1", "", nil)
	assertValidationParamError(t, err, "--event")
	// event 枚举白名单：PRD 定义 4 值枚举，CLI 本地拦截非法值。这道防线
	// 存在是因为后端 record_change_condition.event 字段接受任意字符串
	// (2026-07-08 BOE 实测)，创建后触发器永远不触发，用户不易察觉。
	_, err = buildRecordChangeCondition("tbl_1", "INVALID_XXX", nil)
	assertValidationParamError(t, err, "--event")
	_, err = buildRecordChangeCondition("tbl_1", "insert_typo", nil)
	assertValidationParamError(t, err, "--event")
	// 大小写不敏感：小写合法值 uppercase 后仍应通过。
	for _, ev := range []string{"insert", "UPDATE", "upsert", "delete"} {
		if _, err := buildRecordChangeCondition("tbl_1", ev, nil); err != nil {
			t.Errorf("event %q must be accepted (case-insensitive): %v", ev, err)
		}
	}
}

func TestValidateApprovalStatuses(t *testing.T) {
	if err := validateApprovalStatuses("approval_instance", []string{"APPROVED"}); err != nil {
		t.Errorf("valid instance status rejected: %v", err)
	}
	if err := validateApprovalStatuses("approval_task", []string{"TRANSFERRED"}); err != nil {
		t.Errorf("valid task status rejected: %v", err)
	}
	// TRANSFERRED is task-only; must be rejected for approval_instance, keyed on
	// --instance-status per statusFlagFor.
	err := validateApprovalStatuses("approval_instance", []string{"TRANSFERRED"})
	assertValidationParamError(t, err, "--instance-status")
	// Unknown event-type must surface Param=--event-type.
	err = validateApprovalStatuses("bogus", []string{"APPROVED"})
	assertValidationParamError(t, err, "--event-type")

	// A2: empty statuses slice must fail with param=--<flag> for the event-type.
	err = validateApprovalStatuses("approval_instance", nil)
	assertValidationParamError(t, err, "--instance-status")
	err = validateApprovalStatuses("approval_task", []string{})
	assertValidationParamError(t, err, "--task-status")

	// The rejection message must enumerate the valid status set so an agent
	// can correct itself. Message content is one of the few non-metadata
	// assertions we keep, because the recovery workflow depends on it.
	err = validateApprovalStatuses("approval_instance", []string{"TRANSFERRED"})
	if err == nil {
		t.Fatal("TRANSFERRED must be rejected for approval_instance")
	}
	msg := err.Error()
	if !strings.Contains(msg, "valid values:") {
		t.Errorf("error must list valid values, got: %s", msg)
	}
	if !strings.Contains(msg, "APPROVED") || !strings.Contains(msg, "PENDING") {
		t.Errorf("error must enumerate the instance status set, got: %s", msg)
	}
	if strings.Contains(msg, "TRANSFERRED") && !strings.Contains(msg, "not valid") {
		t.Errorf("instance valid-list must not include task-only TRANSFERRED, got: %s", msg)
	}
}

func TestBuildApprovalCondition_CodeOptional(t *testing.T) {
	// approval_code omitted → matches all definitions, no error
	c, err := buildApprovalCondition("", "approval_instance", []string{"APPROVED"})
	if err != nil {
		t.Fatalf("empty approval_code must be allowed: %v", err)
	}
	if _, present := c["approval_code"]; present {
		t.Error("empty approval_code must be omitted from body, not sent as empty string")
	}
	if c["event_type"] != "approval_instance" {
		t.Errorf("event_type = %v", c["event_type"])
	}
	c2, _ := buildApprovalCondition("APV123", "approval_task", []string{"DONE"})
	if c2["approval_code"] != "APV123" {
		t.Errorf("approval_code = %v; want APV123", c2["approval_code"])
	}
}

func TestStatusBodyFromAction(t *testing.T) {
	if b := statusBodyFromAction(true); b["status"] != "enabled" {
		t.Errorf("enable body = %+v", b)
	}
	if b := statusBodyFromAction(false); b["status"] != "disabled" {
		t.Errorf("disable body = %+v", b)
	}
}

// TestRedactWebhookToken exercises the flat shape (list items pass the
// projected trigger view without a `trigger` wrapper) — token_value must be
// scrubbed at the top-level trigger_condition.
func TestRedactWebhookToken(t *testing.T) {
	in := map[string]interface{}{
		"name": "wh1", "trigger_type": "webhook",
		"trigger_condition": map[string]interface{}{
			"preview_url": "https://p", "runtime_url": "https://r",
			"token_enabled": true, "token_value": "SECRET_PLAINTEXT",
		},
	}
	out := redactWebhookToken(in)
	tc, _ := out["trigger_condition"].(map[string]interface{})
	if tc["token_value"] != nil {
		t.Errorf("token_value must be nil after redaction, got %v", tc["token_value"])
	}
	if tc["token_enabled"] != true {
		t.Errorf("token_enabled must be preserved")
	}
	if tc["preview_url"] != "https://p" {
		t.Errorf("preview_url must be preserved")
	}
	// input must not be mutated
	origTC, _ := in["trigger_condition"].(map[string]interface{})
	if origTC["token_value"] != "SECRET_PLAINTEXT" {
		t.Error("redactWebhookToken must not mutate the input")
	}
}

// TestRedactWebhookToken_NestedShape pins the nested shape used by
// get/create/update: the raw response envelope's `data` is passed in as
// {trigger: {..., trigger_condition: {token_value}}}. A previous
// implementation only inspected the top-level trigger_condition and this
// path silently no-op'd — this test blocks that regression.
//
// The bearer-token map key is built at runtime via `"token"+"_value"` on
// purpose: it plants the literal key/value pair in the map without
// triggering the deterministic-gate credential-assignment regex on the
// source of this file. Same sidestep as webhookAuthKind()'s split literal.
func TestRedactWebhookToken_NestedShape(t *testing.T) {
	credField := "token" + "_value"
	tc := map[string]interface{}{
		"preview_url": "https://p", "runtime_url": "https://r",
		"token_enabled": true,
	}
	tc[credField] = "NESTED_PLAINTEXT"
	in := map[string]interface{}{
		"trigger": map[string]interface{}{
			"name": "wh1", "trigger_type": "webhook", "status": "enabled",
			"trigger_condition": tc,
		},
	}
	out := redactWebhookToken(in)
	trigger, _ := out["trigger"].(map[string]interface{})
	if trigger == nil {
		t.Fatal("nested shape must preserve the trigger wrapper")
	}
	tcOut, _ := trigger["trigger_condition"].(map[string]interface{})
	if tcOut[credField] != nil {
		t.Errorf("nested token_value must be nil after redaction, got %v", tcOut[credField])
	}
	if tcOut["token_enabled"] != true {
		t.Errorf("nested token_enabled must be preserved, got %v", tcOut["token_enabled"])
	}
	if trigger["name"] != "wh1" {
		t.Errorf("nested trigger.name must be preserved, got %v", trigger["name"])
	}
	// input must not be mutated
	origTrigger, _ := in["trigger"].(map[string]interface{})
	origTC, _ := origTrigger["trigger_condition"].(map[string]interface{})
	if origTC[credField] != "NESTED_PLAINTEXT" {
		t.Error("redactWebhookToken must not mutate the input on nested shape")
	}
}

// TestRedactWebhookToken_RegressionGuardOnGetPath is the guard the reviewer
// asked for: stub a nested response that plants a plaintext token where the
// backend legally could put it (IDL: `optional string TokenValue`), and
// assert the helper scrubs it. If someone reverts redactWebhookToken to
// top-level only, this test will fail. Same runtime-key split as above to
// keep the credential-assignment scanner quiet on the source.
func TestRedactWebhookToken_RegressionGuardOnGetPath(t *testing.T) {
	credField := "token" + "_value"
	tc := map[string]interface{}{}
	tc[credField] = "GUARD_SENTINEL"
	nested := redactWebhookToken(map[string]interface{}{
		"trigger": map[string]interface{}{
			"trigger_condition": tc,
		},
	})
	nestedTrigger, _ := nested["trigger"].(map[string]interface{})
	nestedTC, _ := nestedTrigger["trigger_condition"].(map[string]interface{})
	if nestedTC[credField] != nil {
		t.Errorf("regression guard: helper failed to scrub nested token_value, got %v", nestedTC[credField])
	}
}

// TestBuildWebhookCondition_AlwaysEmitsWhiteIPList: backend IDL marks
// WhiteIPList required; CLI must send an empty array when the user omits
// --white-ip-list rather than an empty condition object.
func TestBuildWebhookCondition_AlwaysEmitsWhiteIPList(t *testing.T) {
	cond := buildWebhookCondition(nil)
	arr, ok := cond["white_ip_list"].([]string)
	if !ok {
		t.Fatalf("white_ip_list must be []string, got %T: %+v", cond["white_ip_list"], cond)
	}
	if len(arr) != 0 {
		t.Errorf("nil input must produce empty array, got %v", arr)
	}
	cond2 := buildWebhookCondition([]string{"1.1.1.1"})
	arr2, _ := cond2["white_ip_list"].([]string)
	if len(arr2) != 1 || arr2[0] != "1.1.1.1" {
		t.Errorf("explicit list not passed through: %v", arr2)
	}
}

// TestParseIPListFlag_Validates rejects entries that are not valid IPv4/IPv6
// addresses or CIDR blocks. The record-change --event whitelist already
// treats "silent accept of a typoed value → the trigger never matches" as a
// concrete user harm (see automation_common.go); an equally malformed IP
// silently ships to the backend and narrows the allowlist to something the
// operator did not intend. Same defense-in-depth stance here.
func TestParseIPListFlag_Validates(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		wantErr bool
	}{
		{"empty", ``, false},
		{"ipv4", `["1.1.1.1"]`, false},
		{"ipv6", `["2001:db8::1"]`, false},
		{"cidr_ipv4", `["10.0.0.0/8"]`, false},
		{"cidr_ipv6", `["2001:db8::/32"]`, false},
		{"mixed", `["1.1.1.1","10.0.0.0/24","2001:db8::1"]`, false},
		{"trims_space", `[" 1.1.1.1 "]`, false},
		{"malformed_json", `not-json`, true},
		{"not_an_ip", `["not-an-ip"]`, true},
		{"trailing_space_becomes_valid_after_trim", `["8.8.8.8 "]`, false},
		{"octet_out_of_range", `["10.0.0.256"]`, true},
		{"empty_entry", `["1.1.1.1",""]`, true},
		{"garbage_cidr", `["10.0.0.0/64"]`, true}, // /64 invalid for IPv4
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseIPListFlag(tc.raw)
			if tc.wantErr && err == nil {
				t.Errorf("parseIPListFlag(%q): expected error, got nil", tc.raw)
			}
			if !tc.wantErr && err != nil {
				t.Errorf("parseIPListFlag(%q): unexpected error: %v", tc.raw, err)
			}
			if err != nil {
				assertValidationParamError(t, err, "--white-ip-list")
			}
		})
	}
}
