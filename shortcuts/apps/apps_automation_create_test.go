// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/httpmock"
)

func automationCreateFlagDefs() map[string]string {
	return map[string]string{
		"app-id": "string", "name": "string", "trigger-type": "string", "description": "string",
		"cron": "string", "timezone": "string",
		"table": "string", "event": "string", "fields": "string",
		"white-ip-list": "string",
		"approval-code": "string", "event-type": "string",
		"instance-status": "string_array", "task-status": "string_array",
	}
}

func TestAutomationCreateCron_BuildsBody(t *testing.T) {
	rctx, stdoutBuf, reg := newOpenAPIKeyRCtx(t, automationCreateFlagDefs(),
		map[string]string{"app-id": "app_x", "name": "daily", "trigger-type": "cron", "cron": "0 9 * * *"})
	reg.Register(&httpmock.Stub{
		Method: "POST", URL: "/open-apis/apaas/v1/apps/app_x/triggers",
		Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{
			"name": "daily", "trigger_type": "cron", "status": "disabled",
		}},
	})
	if err := AppsAutomationCreate.Execute(context.Background(), rctx); err != nil {
		t.Fatalf("Execute() = %v", err)
	}
	if !strings.Contains(stdoutBuf.String(), "daily") {
		t.Errorf("create output must contain trigger name: %s", stdoutBuf.String())
	}
}

func TestAutomationCreate_MissingType(t *testing.T) {
	rctx, _, _ := newOpenAPIKeyRCtx(t, automationCreateFlagDefs(),
		map[string]string{"app-id": "app_x", "name": "n"})
	err := AppsAutomationCreate.Validate(context.Background(), rctx)
	assertValidationParamError(t, err, "--trigger-type")
}

func TestAutomationCreateCron_Sub30MinRejected(t *testing.T) {
	rctx, _, _ := newOpenAPIKeyRCtx(t, automationCreateFlagDefs(),
		map[string]string{"app-id": "app_x", "name": "n", "trigger-type": "cron", "cron": "*/5 * * * *"})
	err := AppsAutomationCreate.Validate(context.Background(), rctx)
	assertValidationParamError(t, err, "--cron")
}

func TestAutomationCreateRecordChange_MissingEvent(t *testing.T) {
	rctx, _, _ := newOpenAPIKeyRCtx(t, automationCreateFlagDefs(),
		map[string]string{"app-id": "app_x", "name": "n", "trigger-type": "record-change", "table": "tbl"})
	err := AppsAutomationCreate.Validate(context.Background(), rctx)
	assertValidationParamError(t, err, "--event")
}

func TestAutomationCreateApproval_CodeOptional(t *testing.T) {
	rctx, _, reg := newOpenAPIKeyRCtx(t, automationCreateFlagDefs(),
		map[string]string{"app-id": "app_x", "name": "n", "trigger-type": "feishu-approval",
			"event-type": "approval_instance", "instance-status": "APPROVED"})
	reg.Register(&httpmock.Stub{
		Method: "POST", URL: "/open-apis/apaas/v1/apps/app_x/triggers",
		Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{"name": "n", "status": "disabled"}},
	})
	if err := AppsAutomationCreate.Validate(context.Background(), rctx); err != nil {
		t.Fatalf("approval without --approval-code must pass validation: %v", err)
	}
	if err := AppsAutomationCreate.Execute(context.Background(), rctx); err != nil {
		t.Fatalf("Execute() = %v", err)
	}
}

// TestAutomationCreateApproval_StatusUppercased asserts that a lowercase status
// passed via --instance-status is normalized to the uppercase enum in the body
// before it reaches the backend (foundation review: buildApprovalCondition stores
// the raw statuses, so create must uppercase them itself).
func TestAutomationCreateApproval_StatusUppercased(t *testing.T) {
	rctx, _, _ := newOpenAPIKeyRCtx(t, automationCreateFlagDefs(),
		map[string]string{"app-id": "app_x", "name": "n", "trigger-type": "feishu-approval",
			"event-type": "approval_instance", "instance-status": "approved"})
	body, err := buildAutomationCreateBody(rctx)
	if err != nil {
		t.Fatalf("buildAutomationCreateBody() = %v", err)
	}
	cond, ok := body["feishu_approval_condition"].(map[string]interface{})
	if !ok {
		t.Fatalf("feishu_approval_condition missing or wrong type: %+v", body)
	}
	statuses, ok := cond["status"].([]string)
	if !ok {
		t.Fatalf("status must be []string: %+v", cond)
	}
	if len(statuses) != 1 || statuses[0] != "APPROVED" {
		t.Errorf("lowercase status must be uppercased to APPROVED, got %v", statuses)
	}
}

// TestAutomationCreate_RedactsWebhookToken covers the bearer-token redaction
// reverse invariant on the create path: CreateTrigger backend re-reads
// GetTriggerModel and returns TriggerInfo, sharing the decrypting
// webhook-condition converter with get/list — theoretically capable of
// returning plaintext bearerToken. Defense-in-depth: CLI create must also
// redact so every read-shaped output path is consistently scrubbed.
func TestAutomationCreate_RedactsWebhookToken(t *testing.T) {
	rctx, stdoutBuf, reg := newOpenAPIKeyRCtx(t, automationCreateFlagDefs(),
		map[string]string{"app-id": "app_x", "name": "wh1", "trigger-type": "webhook"})
	reg.Register(&httpmock.Stub{
		Method: "POST", URL: "/open-apis/apaas/v1/apps/app_x/triggers",
		Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{
			"name": "wh1", "trigger_type": "webhook", "status": "disabled",
			"trigger_condition": map[string]interface{}{
				"preview_url": "https://p", "runtime_url": "https://r",
				"token_enabled": true, "token_value": "PLAINTEXT_CREATE_TOKEN",
			},
		}},
	})
	if err := AppsAutomationCreate.Execute(context.Background(), rctx); err != nil {
		t.Fatalf("Execute() = %v", err)
	}
	out := stdoutBuf.String()
	if strings.Contains(out, "PLAINTEXT_CREATE_TOKEN") {
		t.Errorf("create must never surface plaintext token: %s", out)
	}
}
