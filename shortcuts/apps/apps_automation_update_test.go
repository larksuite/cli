// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/httpmock"
)

func TestAutomationUpdate_PatchCronOnly(t *testing.T) {
	rctx, stdoutBuf, reg := newOpenAPIKeyRCtx(t, automationUpdateFlagDefs(),
		map[string]string{"app-id": "app_x", "name": "t1", "trigger-type": "cron", "cron": "0 10 * * *"})
	reg.Register(&httpmock.Stub{
		Method: "PUT", URL: "/open-apis/spark/v1/apps/app_x/triggers/t1",
		Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{"name": "t1", "trigger_type": "cron"}},
	})
	if err := runAutomationUpdate(rctx); err != nil {
		t.Fatalf("Execute() = %v", err)
	}
	if !strings.Contains(stdoutBuf.String(), "t1") {
		t.Errorf("update output = %s", stdoutBuf.String())
	}
}

// TestAutomationUpdate_MutuallyExclusiveWebhookFlags exercises the mutex check
// on webhook action flags. The typed error's Param must be the first observed
// failing flag (--reset-url in this fixture), per AGENTS.md: Param names only
// actual failed user input.
func TestAutomationUpdate_MutuallyExclusiveWebhookFlags(t *testing.T) {
	rctx, _, _ := newOpenAPIKeyRCtx(t, automationUpdateFlagDefs(),
		map[string]string{"app-id": "app_x", "name": "t1", "reset-url": "true", "reset-token": "true"})
	err := AppsAutomationUpdate.Validate(context.Background(), rctx)
	assertValidationParamError(t, err, "--reset-url")
}

func TestAutomationUpdate_WhiteIPListPatch(t *testing.T) {
	rctx, _, reg := newOpenAPIKeyRCtx(t, automationUpdateFlagDefs(),
		map[string]string{"app-id": "app_x", "name": "wh1", "trigger-type": "webhook", "white-ip-list": `["1.1.1.1"]`})
	reg.Register(&httpmock.Stub{
		Method: "PUT", URL: "/open-apis/spark/v1/apps/app_x/triggers/wh1",
		Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{"name": "wh1"}},
	})
	if err := runAutomationUpdate(rctx); err != nil {
		t.Fatalf("Execute() = %v", err)
	}
}

func TestAutomationUpdate_InvalidCronRejected(t *testing.T) {
	rctx, _, _ := newOpenAPIKeyRCtx(t, automationUpdateFlagDefs(),
		map[string]string{"app-id": "app_x", "name": "t1", "trigger-type": "cron", "cron": "*/5 * * * *"})
	err := runAutomationUpdate(rctx)
	assertValidationParamError(t, err, "--cron")
}

func TestAutomationUpdate_InvalidWhiteIPListRejected(t *testing.T) {
	rctx, _, _ := newOpenAPIKeyRCtx(t, automationUpdateFlagDefs(),
		map[string]string{"app-id": "app_x", "name": "wh1", "trigger-type": "webhook", "white-ip-list": "{bad json"})
	err := runAutomationUpdate(rctx)
	assertValidationParamError(t, err, "--white-ip-list")
}

// TestAutomationUpdate_NoFieldsRejected covers the empty-PATCH guard: at least
// one condition-carrying flag or a webhook action flag must be present. The
// error is intentionally Param-less (no single user flag failed); recovery
// candidates are structured in Params + Hint, matching the +update precedent.
func TestAutomationUpdate_NoFieldsRejected(t *testing.T) {
	rctx, _, _ := newOpenAPIKeyRCtx(t, automationUpdateFlagDefs(),
		map[string]string{"app-id": "app_x", "name": "t1"})
	err := runAutomationUpdate(rctx)
	if err == nil {
		t.Fatal("empty PATCH must be rejected")
	}
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected *errs.ValidationError, got %T: %v", err, err)
	}
	if ve.Category != errs.CategoryValidation {
		t.Errorf("category = %s, want %s", ve.Category, errs.CategoryValidation)
	}
	if ve.Subtype != errs.SubtypeInvalidArgument {
		t.Errorf("subtype = %s, want %s", ve.Subtype, errs.SubtypeInvalidArgument)
	}
	if ve.Param != "" {
		t.Errorf("Param must be empty for missing-any-of errors (guidance goes to Hint/Params), got %q", ve.Param)
	}
	if ve.Hint == "" {
		t.Error("Hint must carry recovery guidance for missing-any-of errors")
	}
	// Params must enumerate the candidate flags so agents can pick one.
	if len(ve.Params) < 5 {
		t.Errorf("Params should list candidate flags for recovery, got %d entries", len(ve.Params))
	}
}

// TestAutomationUpdate_ResetURLRequiresAppEnv exercises the Validate-time check
// that --reset-url requires --app-env.
func TestAutomationUpdate_ResetURLRequiresAppEnv(t *testing.T) {
	rctx, _, _ := newOpenAPIKeyRCtx(t, automationUpdateFlagDefs(),
		map[string]string{"app-id": "app_x", "name": "wh1", "reset-url": "true"})
	err := AppsAutomationUpdate.Validate(context.Background(), rctx)
	assertValidationParamError(t, err, "--app-env")
}

// TestAutomationUpdate_PatchRecordChange covers A5: --trigger-type record-change
// with --table/--event dispatches to record_change_condition rebuild.
func TestAutomationUpdate_PatchRecordChange(t *testing.T) {
	rctx, stdoutBuf, reg := newOpenAPIKeyRCtx(t, automationUpdateFlagDefs(),
		map[string]string{
			"app-id": "app_x", "name": "rc1", "trigger-type": "record-change",
			"table": "tbl_1", "event": "UPDATE", "fields": `["fld1"]`,
		})
	reg.Register(&httpmock.Stub{
		Method: "PUT", URL: "/open-apis/spark/v1/apps/app_x/triggers/rc1",
		Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{"name": "rc1", "trigger_type": "record_change"}},
	})
	if err := runAutomationUpdate(rctx); err != nil {
		t.Fatalf("Execute() = %v", err)
	}
	if !strings.Contains(stdoutBuf.String(), "rc1") {
		t.Errorf("update output = %s", stdoutBuf.String())
	}
}

// TestAutomationUpdate_PatchRecordChange_MissingEvent covers A5 error path:
// --table without --event surfaces a typed error keyed on --event.
func TestAutomationUpdate_PatchRecordChange_MissingEvent(t *testing.T) {
	rctx, _, _ := newOpenAPIKeyRCtx(t, automationUpdateFlagDefs(),
		map[string]string{
			"app-id": "app_x", "name": "rc1", "trigger-type": "record-change",
			"table": "tbl_1",
		})
	err := runAutomationUpdate(rctx)
	assertValidationParamError(t, err, "--event")
}

// TestAutomationUpdate_PatchRecordChange_InvalidFieldsJSON covers A5: bad JSON
// in --fields is rejected up-front by parseFieldsFlag with Param=--fields.
func TestAutomationUpdate_PatchRecordChange_InvalidFieldsJSON(t *testing.T) {
	rctx, _, _ := newOpenAPIKeyRCtx(t, automationUpdateFlagDefs(),
		map[string]string{
			"app-id": "app_x", "name": "rc1", "trigger-type": "record-change",
			"table": "tbl_1", "event": "UPDATE", "fields": "{bad json",
		})
	err := runAutomationUpdate(rctx)
	assertValidationParamError(t, err, "--fields")
}

// TestAutomationUpdate_PatchApproval covers A5: feishu-approval dispatch.
func TestAutomationUpdate_PatchApproval(t *testing.T) {
	rctx, stdoutBuf, reg := newOpenAPIKeyRCtx(t, automationUpdateFlagDefs(),
		map[string]string{
			"app-id": "app_x", "name": "apv", "trigger-type": "feishu-approval",
			"event-type": "approval_instance", "instance-status": "approved",
		})
	reg.Register(&httpmock.Stub{
		Method: "PUT", URL: "/open-apis/spark/v1/apps/app_x/triggers/apv",
		Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{"name": "apv", "trigger_type": "feishu_approval"}},
	})
	if err := runAutomationUpdate(rctx); err != nil {
		t.Fatalf("Execute() = %v", err)
	}
	if !strings.Contains(stdoutBuf.String(), "apv") {
		t.Errorf("update output = %s", stdoutBuf.String())
	}
}

// TestAutomationUpdate_PatchApproval_TaskEventStatuses verifies that
// approval_task pulls its statuses from --task-status (not --instance-status).
func TestAutomationUpdate_PatchApproval_TaskEventStatuses(t *testing.T) {
	rctx, _, reg := newOpenAPIKeyRCtx(t, automationUpdateFlagDefs(),
		map[string]string{
			"app-id": "app_x", "name": "apv", "trigger-type": "feishu-approval",
			"event-type": "approval_task", "task-status": "DONE",
		})
	reg.Register(&httpmock.Stub{
		Method: "PUT", URL: "/open-apis/spark/v1/apps/app_x/triggers/apv",
		Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{"name": "apv"}},
	})
	if err := runAutomationUpdate(rctx); err != nil {
		t.Fatalf("Execute() = %v", err)
	}
}

// TestAutomationUpdate_PatchApproval_MissingStatuses: --event-type without
// --instance-status / --task-status surfaces a typed error keyed on the status
// flag matching the event-type.
func TestAutomationUpdate_PatchApproval_MissingStatuses(t *testing.T) {
	rctx, _, _ := newOpenAPIKeyRCtx(t, automationUpdateFlagDefs(),
		map[string]string{
			"app-id": "app_x", "name": "apv", "trigger-type": "feishu-approval",
			"event-type": "approval_instance",
		})
	err := runAutomationUpdate(rctx)
	assertValidationParamError(t, err, "--instance-status")
}

// TestAutomationUpdate_PatchRedactsWebhookToken covers the bearer-token
// redaction reverse invariant on the update-patch path: PATCH response is a
// re-read of GetTriggerModel and may carry a decrypted bearer token; the CLI
// must redact it before stdout, mirroring get/list behaviour. Without this
// test a regression could leak plaintext.
func TestAutomationUpdate_PatchRedactsWebhookToken(t *testing.T) {
	rctx, stdoutBuf, reg := newOpenAPIKeyRCtx(t, automationUpdateFlagDefs(),
		map[string]string{
			"app-id": "app_x", "name": "wh1", "trigger-type": "webhook",
			"white-ip-list": `["1.1.1.1"]`,
		})
	reg.Register(&httpmock.Stub{
		Method: "PUT", URL: "/open-apis/spark/v1/apps/app_x/triggers/wh1",
		Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{
			"name": "wh1", "trigger_type": "webhook", "status": "enabled",
			"trigger_condition": map[string]interface{}{
				"preview_url": "https://p", "runtime_url": "https://r",
				"token_enabled": true, "token_value": "PLAINTEXT_PATCH_TOKEN",
			},
		}},
	})
	if err := runAutomationUpdate(rctx); err != nil {
		t.Fatalf("Execute() = %v", err)
	}
	out := stdoutBuf.String()
	if strings.Contains(out, "PLAINTEXT_PATCH_TOKEN") {
		t.Errorf("update PATCH must never surface plaintext token: %s", out)
	}
	if !strings.Contains(out, "token_enabled") {
		t.Errorf("update PATCH must still expose token_enabled: %s", out)
	}
}

// TestAutomationUpdate_WebhookActionRejectsConditionFlag: combining a webhook
// action flag with a condition flag would silently drop the condition (e.g.
// `--reset-token --cron '0 9 * * *'` used to just rotate the token). Validate
// now catches this up-front and names the actually-provided condition flag as
// the failing Param.
func TestAutomationUpdate_WebhookActionRejectsConditionFlag(t *testing.T) {
	rctx, _, _ := newOpenAPIKeyRCtx(t, automationUpdateFlagDefs(),
		map[string]string{
			"app-id": "app_x", "name": "wh1",
			"reset-token": "true", "cron": "0 9 * * *",
		})
	err := AppsAutomationUpdate.Validate(context.Background(), rctx)
	assertValidationParamError(t, err, "--cron")
}

func TestAutomationUpdateMeta_HighRisk(t *testing.T) {
	if AppsAutomationUpdate.Risk != "high-risk-write" {
		t.Errorf("update must be high-risk-write, got %q", AppsAutomationUpdate.Risk)
	}
}
