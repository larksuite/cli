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
