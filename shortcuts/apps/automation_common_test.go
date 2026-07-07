// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"strings"
	"testing"
)

func TestAutomationPaths(t *testing.T) {
	if got := automationListPath("app_x"); got != "/open-apis/apaas/v1/apps/app_x/triggers" {
		t.Errorf("listPath = %q", got)
	}
	if got := automationItemPath("app_x", "t1"); got != "/open-apis/apaas/v1/apps/app_x/triggers/t1" {
		t.Errorf("itemPath = %q", got)
	}
	if got := automationStatusPath("app_x", "t1"); got != "/open-apis/apaas/v1/apps/app_x/triggers/t1/status" {
		t.Errorf("statusPath = %q", got)
	}
	if got := automationWebhookTokenStatusPath("app_x", "t1"); got != "/open-apis/apaas/v1/apps/app_x/triggers/t1/webhook/token/status" {
		t.Errorf("tokenStatusPath = %q", got)
	}
	if got := automationWebhookTokenResetPath("app_x", "t1"); got != "/open-apis/apaas/v1/apps/app_x/triggers/t1/webhook/token/reset" {
		t.Errorf("tokenResetPath = %q", got)
	}
	if got := automationWebhookURLResetPath("app_x", "t1"); got != "/open-apis/apaas/v1/apps/app_x/triggers/t1/webhook/url/reset" {
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
	if _, err := mapTriggerType("bogus"); err == nil {
		t.Error("unknown trigger type must error")
	}
}

func TestValidateCronExpr(t *testing.T) {
	if err := validateCronExpr("0 9 * * *"); err != nil {
		t.Errorf("valid daily cron rejected: %v", err)
	}
	if err := validateCronExpr("0 9 * *"); err == nil {
		t.Error("4-field cron must be rejected (needs 5 fields)")
	}
	if err := validateCronExpr("*/5 * * * *"); err == nil {
		t.Error("5-minute interval must be rejected (< 30 min floor)")
	}
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
	if _, err := buildCronCondition("*/5 * * * *", ""); err == nil {
		t.Error("sub-30min cron must error")
	}
}

func TestBuildRecordChangeCondition(t *testing.T) {
	c, err := buildRecordChangeCondition("tbl_1", "update", []string{"status"})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if c["event"] != "UPDATE" || c["table"] != "tbl_1" {
		t.Errorf("record_change = %+v; event must be uppercased", c)
	}
	if _, err := buildRecordChangeCondition("", "UPDATE", nil); err == nil {
		t.Error("missing table must error")
	}
	if _, err := buildRecordChangeCondition("tbl_1", "", nil); err == nil {
		t.Error("missing event must error")
	}
}

func TestValidateApprovalStatuses(t *testing.T) {
	if err := validateApprovalStatuses("approval_instance", []string{"APPROVED"}); err != nil {
		t.Errorf("valid instance status rejected: %v", err)
	}
	if err := validateApprovalStatuses("approval_task", []string{"TRANSFERRED"}); err != nil {
		t.Errorf("valid task status rejected: %v", err)
	}
	// TRANSFERRED only valid for task, not instance
	if err := validateApprovalStatuses("approval_instance", []string{"TRANSFERRED"}); err == nil {
		t.Error("TRANSFERRED must be rejected for approval_instance")
	}
	if err := validateApprovalStatuses("bogus", []string{"APPROVED"}); err == nil {
		t.Error("unknown event-type must error")
	}
	// spec Error-004: rejection message must list the valid status set for the
	// event-type so the agent can correct itself.
	err := validateApprovalStatuses("approval_instance", []string{"TRANSFERRED"})
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
	// TRANSFERRED is task-only; it must NOT appear in the instance valid list.
	if strings.Contains(msg, "TRANSFERRED") && !strings.Contains(msg, "not valid") {
		t.Errorf("instance valid-list must not include task-only TRANSFERRED, got: %s", msg)
	}
}

func TestBuildApprovalCondition_CodeOptional(t *testing.T) {
	// approval_code omitted → matches all definitions, no error (Rule-6-2)
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
