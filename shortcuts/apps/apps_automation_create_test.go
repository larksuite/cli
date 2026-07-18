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
		"status": "string",
	}
}

func TestAutomationCreateCron_BuildsBody(t *testing.T) {
	rctx, stdoutBuf, reg := newOpenAPIKeyRCtx(t, automationCreateFlagDefs(),
		map[string]string{"app-id": "app_x", "name": "daily", "trigger-type": "cron", "cron": "0 9 * * *"})
	// Real backend response wraps the created trigger under `trigger` (a live
	// test-env probe confirmed the shape, same as GET/PUT). The Execute pretty
	// path reads trigger["name"]/["trigger_type"]/["status"] from that key —
	// a flat fixture makes the pretty path print `<nil>` and only passes via
	// the JSON envelope, which hides regressions in the pretty branch.
	reg.Register(&httpmock.Stub{
		Method: "POST", URL: "/open-apis/spark/v1/apps/app_x/triggers",
		Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{
			"trigger": map[string]interface{}{
				"name": "daily", "trigger_type": "cron", "status": "disabled",
			},
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

// TestAutomationCreate_CrossFamilyFlagsRejected pins the F1 guard: a condition
// flag from a family other than --trigger-type used to be silently dropped by
// buildAutomationCreateBody's single-branch switch, so
// `--trigger-type webhook --cron '0 9 * * *'` created a webhook with no cron
// but returned success. Validate now rejects the cross-family flag up-front.
func TestAutomationCreate_CrossFamilyFlagsRejected(t *testing.T) {
	cases := []struct {
		name      string
		flags     map[string]string
		wantParam string
	}{
		{"webhook_with_cron",
			map[string]string{
				"app-id": "app_x", "name": "n", "trigger-type": "webhook",
				"cron": "0 9 * * *",
			}, "--cron"},
		{"cron_with_white_ip_list",
			map[string]string{
				"app-id": "app_x", "name": "n", "trigger-type": "cron",
				"cron": "0 9 * * *", "white-ip-list": `["1.1.1.1"]`,
			}, "--white-ip-list"},
		{"record_change_with_event_type",
			map[string]string{
				"app-id": "app_x", "name": "n", "trigger-type": "record-change",
				"table": "tbl", "event": "UPDATE", "event-type": "approval_instance",
			}, "--event-type"},
		{"feishu_approval_with_table",
			map[string]string{
				"app-id": "app_x", "name": "n", "trigger-type": "feishu-approval",
				"event-type": "approval_instance", "instance-status": "APPROVED",
				"table": "tbl",
			}, "--table"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rctx, _, _ := newOpenAPIKeyRCtx(t, automationCreateFlagDefs(), tc.flags)
			err := AppsAutomationCreate.Validate(context.Background(), rctx)
			assertValidationParamError(t, err, tc.wantParam)
		})
	}
}

// TestAutomationCreate_UnknownTriggerTypeRejected: --trigger-type must be one
// of the four supported kebab-case values. A typo used to sneak past Validate
// (buildAutomationCreateBody caught it, but only after the cross-family guard
// would otherwise fire with a misleading "belongs to type" message).
func TestAutomationCreate_UnknownTriggerTypeRejected(t *testing.T) {
	rctx, _, _ := newOpenAPIKeyRCtx(t, automationCreateFlagDefs(),
		map[string]string{"app-id": "app_x", "name": "n", "trigger-type": "bogus"})
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
		Method: "POST", URL: "/open-apis/spark/v1/apps/app_x/triggers",
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
// reverse invariant on the create path against the real response shape (a
// live test-env probe confirmed POST wraps the trigger under a `trigger`
// key, same as GET/PUT). The backend create path re-reads the freshly
// created trigger and returns it through the same read-path converter used
// by get/list — theoretically capable of returning a plaintext bearer
// token. Defense-in-depth: CLI create must also redact so every read-shaped
// output path is consistently scrubbed.
func TestAutomationCreate_RedactsWebhookToken(t *testing.T) {
	rctx, stdoutBuf, reg := newOpenAPIKeyRCtx(t, automationCreateFlagDefs(),
		map[string]string{"app-id": "app_x", "name": "wh1", "trigger-type": "webhook"})
	reg.Register(&httpmock.Stub{
		Method: "POST", URL: "/open-apis/spark/v1/apps/app_x/triggers",
		Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{
			"trigger": map[string]interface{}{
				"name": "wh1", "trigger_type": "webhook", "status": "disabled",
				"trigger_condition": map[string]interface{}{
					"preview_url": "https://p", "runtime_url": "https://r",
					"token_enabled": true, "token_value": "PLAINTEXT_CREATE_TOKEN",
				},
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

// TestAutomationCreate_StatusPassthrough verifies --status is included in the
// POST body when set. Backend supports create+enable in one call via the
// optional status field; CLI passes it through unchanged.
func TestAutomationCreate_StatusPassthrough(t *testing.T) {
	rctx, _, _ := newOpenAPIKeyRCtx(t, automationCreateFlagDefs(),
		map[string]string{
			"app-id": "app_x", "name": "n", "trigger-type": "cron",
			"cron": "0 9 * * *", "status": "enabled",
		})
	body, err := buildAutomationCreateBody(rctx)
	if err != nil {
		t.Fatalf("buildBody: %v", err)
	}
	if body["status"] != "enabled" {
		t.Errorf("status = %v; want enabled", body["status"])
	}
}

// TestAutomationCreate_StatusInvalid: only enabled/disabled accepted.
func TestAutomationCreate_StatusInvalid(t *testing.T) {
	rctx, _, _ := newOpenAPIKeyRCtx(t, automationCreateFlagDefs(),
		map[string]string{
			"app-id": "app_x", "name": "n", "trigger-type": "cron",
			"cron": "0 9 * * *", "status": "bogus",
		})
	_, err := buildAutomationCreateBody(rctx)
	assertValidationParamError(t, err, "--status")
}

// TestAutomationCreate_StatusOmitted: when --status is not set, body must not
// carry a status field — backend applies its default (disabled).
func TestAutomationCreate_StatusOmitted(t *testing.T) {
	rctx, _, _ := newOpenAPIKeyRCtx(t, automationCreateFlagDefs(),
		map[string]string{
			"app-id": "app_x", "name": "n", "trigger-type": "cron",
			"cron": "0 9 * * *",
		})
	body, err := buildAutomationCreateBody(rctx)
	if err != nil {
		t.Fatalf("buildBody: %v", err)
	}
	if _, present := body["status"]; present {
		t.Errorf("status must be omitted when --status not set, got %v", body["status"])
	}
}

// TestAutomationCreate_NameTooLong: --name > 100 chars is rejected locally with
// a typed --name error, sparing the round trip to the backend.
func TestAutomationCreate_NameTooLong(t *testing.T) {
	rctx, _, _ := newOpenAPIKeyRCtx(t, automationCreateFlagDefs(),
		map[string]string{
			"app-id": "app_x", "name": strings.Repeat("n", automationNameMaxLen+1),
			"trigger-type": "cron", "cron": "0 9 * * *",
		})
	_, err := buildAutomationCreateBody(rctx)
	assertValidationParamError(t, err, "--name")
}

// TestAutomationCreate_DescriptionTooLong: --description > 50 chars is rejected
// locally with a typed --description error.
func TestAutomationCreate_DescriptionTooLong(t *testing.T) {
	rctx, _, _ := newOpenAPIKeyRCtx(t, automationCreateFlagDefs(),
		map[string]string{
			"app-id": "app_x", "name": "n", "trigger-type": "cron",
			"cron": "0 9 * * *", "description": strings.Repeat("d", automationDescriptionMaxLen+1),
		})
	_, err := buildAutomationCreateBody(rctx)
	assertValidationParamError(t, err, "--description")
}
