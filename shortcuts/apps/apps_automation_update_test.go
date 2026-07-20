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

// TestAutomationUpdate_NoFieldsRejected covers the empty-update guard: at
// least one condition-carrying flag or a webhook action flag must be present.
// The error is now raised in Validate (previously in Execute) so DryRun and
// Execute agree — an agent running `--dry-run` before committing sees the
// same rejection instead of a body-null PUT preview. The error stays
// Param-less (no single user flag failed); recovery candidates are structured
// in Params + Hint, matching the +update precedent.
func TestAutomationUpdate_NoFieldsRejected(t *testing.T) {
	rctx, _, _ := newOpenAPIKeyRCtx(t, automationUpdateFlagDefs(),
		map[string]string{"app-id": "app_x", "name": "t1"})
	err := AppsAutomationUpdate.Validate(context.Background(), rctx)
	if err == nil {
		t.Fatal("empty update must be rejected")
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

// TestAutomationUpdate_AppEnvRequiresResetURL: --app-env is only consumed by
// --reset-url. Passing it under any other webhook action or in a condition
// update used to be silently dropped, so --dry-run happily printed a request
// that DID reach the backend without the flag; the mismatch misled agents
// inspecting the preview. Validate now rejects up-front.
func TestAutomationUpdate_AppEnvRequiresResetURL(t *testing.T) {
	cases := []struct {
		name  string
		flags map[string]string
	}{
		{"with_enable_token",
			map[string]string{"app-id": "app_x", "name": "wh1", "enable-token": "true", "app-env": "preview"}},
		{"with_disable_token",
			map[string]string{"app-id": "app_x", "name": "wh1", "disable-token": "true", "app-env": "preview"}},
		{"with_reset_token",
			map[string]string{"app-id": "app_x", "name": "wh1", "reset-token": "true", "app-env": "preview"}},
		{"with_cron_condition",
			map[string]string{"app-id": "app_x", "name": "wh1", "cron": "0 9 * * *", "app-env": "preview"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rctx, _, _ := newOpenAPIKeyRCtx(t, automationUpdateFlagDefs(), tc.flags)
			err := AppsAutomationUpdate.Validate(context.Background(), rctx)
			assertValidationParamError(t, err, "--app-env")
		})
	}
}

// TestAutomationUpdate_AppEnvInvalidValueRejected: --app-env must be
// preview|runtime. Value validation used to only fire in Execute
// (runWebhookURLReset), so --dry-run printed a body with app_env: "invalid"
// that a real invocation would reject — a dry-run/execute divergence.
// Validate now catches invalid values so dry-run and execute agree.
func TestAutomationUpdate_AppEnvInvalidValueRejected(t *testing.T) {
	rctx, _, _ := newOpenAPIKeyRCtx(t, automationUpdateFlagDefs(),
		map[string]string{"app-id": "app_x", "name": "wh1", "reset-url": "true", "app-env": "invalid"})
	err := AppsAutomationUpdate.Validate(context.Background(), rctx)
	assertValidationParamError(t, err, "--app-env")
	if !strings.Contains(err.Error(), "preview or runtime") {
		t.Errorf("expected preview/runtime guidance, got %q", err.Error())
	}
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
// redaction reverse invariant on the update-patch path against the real
// response shape (a live test-env probe confirmed PUT wraps the trigger
// under a `trigger` key, same as GET/create). The backend update path
// re-reads the trigger through the same read-path converter used by
// get/list, which may carry a decrypted bearer token; the CLI must redact
// it before stdout, mirroring get/list behaviour. Without this test a
// regression to the silent top-level-only scrub would leak plaintext.
func TestAutomationUpdate_PatchRedactsWebhookToken(t *testing.T) {
	rctx, stdoutBuf, reg := newOpenAPIKeyRCtx(t, automationUpdateFlagDefs(),
		map[string]string{
			"app-id": "app_x", "name": "wh1", "trigger-type": "webhook",
			"white-ip-list": `["1.1.1.1"]`,
		})
	reg.Register(&httpmock.Stub{
		Method: "PUT", URL: "/open-apis/spark/v1/apps/app_x/triggers/wh1",
		Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{
			"trigger": map[string]interface{}{
				"name": "wh1", "trigger_type": "webhook", "status": "enabled",
				"trigger_condition": map[string]interface{}{
					"preview_url": "https://p", "runtime_url": "https://r",
					"token_enabled": true, "token_value": "PLAINTEXT_PATCH_TOKEN",
				},
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

// TestAutomationUpdate_SubordinateFlagsRequireParent pins the inert-flag
// contract: a subordinate flag (--timezone / --instance-status /
// --task-status / --approval-code) is rejected with a "requires --<parent>"
// error, not the generic "no update fields" whose Hint used to loop the
// agent back to the same subordinate flag. Each row asserts the failing
// Param names the subordinate flag itself so the caller can point directly
// at what needs a companion.
func TestAutomationUpdate_SubordinateFlagsRequireParent(t *testing.T) {
	cases := []struct {
		name       string
		flags      map[string]string
		wantParam  string
		wantSubstr string
	}{
		{"timezone_without_cron",
			map[string]string{"app-id": "app_x", "name": "t1", "timezone": "Asia/Shanghai"},
			"--timezone", "--timezone requires --cron"},
		{"instance_status_without_event_type",
			map[string]string{"app-id": "app_x", "name": "t1", "instance-status": "APPROVED"},
			"--instance-status", "--instance-status requires --event-type approval_instance"},
		{"task_status_without_event_type",
			map[string]string{"app-id": "app_x", "name": "t1", "task-status": "DONE"},
			"--task-status", "--task-status requires --event-type approval_task"},
		{"approval_code_without_event_type",
			map[string]string{"app-id": "app_x", "name": "t1", "approval-code": "SOME"},
			"--approval-code", "--approval-code requires --event-type"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rctx, _, _ := newOpenAPIKeyRCtx(t, automationUpdateFlagDefs(), tc.flags)
			err := AppsAutomationUpdate.Validate(context.Background(), rctx)
			assertValidationParamError(t, err, tc.wantParam)
			if !strings.Contains(err.Error(), tc.wantSubstr) {
				t.Errorf("expected message containing %q, got %q", tc.wantSubstr, err.Error())
			}
		})
	}
}

// TestAutomationUpdate_MismatchedStatusArrayWithEventType pins the reverse
// inert-flag branch: --event-type is set, but the caller also passes the
// wrong status-array flag (e.g. --event-type approval_instance --task-status).
// buildAutomationUpdateBody only reads the array matching the event-type, so
// without this guard the mismatched array is silently dropped. Reject with a
// typed error naming the mismatched flag.
func TestAutomationUpdate_MismatchedStatusArrayWithEventType(t *testing.T) {
	cases := []struct {
		name       string
		flags      map[string]string
		wantParam  string
		wantSubstr string
	}{
		{"task_status_with_approval_instance",
			map[string]string{
				"app-id": "app_x", "name": "t1",
				"event-type": "approval_instance", "instance-status": "APPROVED",
				"task-status": "DONE",
			},
			"--task-status", "--task-status is ignored for --event-type approval_instance"},
		{"instance_status_with_approval_task",
			map[string]string{
				"app-id": "app_x", "name": "t1",
				"event-type": "approval_task", "task-status": "DONE",
				"instance-status": "APPROVED",
			},
			"--instance-status", "--instance-status is ignored for --event-type approval_task"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rctx, _, _ := newOpenAPIKeyRCtx(t, automationUpdateFlagDefs(), tc.flags)
			err := AppsAutomationUpdate.Validate(context.Background(), rctx)
			assertValidationParamError(t, err, tc.wantParam)
			if !strings.Contains(err.Error(), tc.wantSubstr) {
				t.Errorf("expected message containing %q, got %q", tc.wantSubstr, err.Error())
			}
		})
	}
}

// TestAutomationUpdate_DescriptionTooLong: --description > 50 chars is
// rejected in Validate with a typed --description error.
// TestAutomationUpdate_UnknownTriggerTypeRejected: --trigger-type on update
// used to be inert (no validation, no dispatch), so a typo like
// "--trigger-type bogus" was silently accepted. Validate now runs mapTriggerType
// on any non-empty --trigger-type.
func TestAutomationUpdate_UnknownTriggerTypeRejected(t *testing.T) {
	rctx, _, _ := newOpenAPIKeyRCtx(t, automationUpdateFlagDefs(),
		map[string]string{
			"app-id": "app_x", "name": "t1", "trigger-type": "bogus",
			"cron": "0 9 * * *",
		})
	err := AppsAutomationUpdate.Validate(context.Background(), rctx)
	assertValidationParamError(t, err, "--trigger-type")
}

// TestAutomationUpdate_CrossFamilyConditionFlagsRejected pins the F2 guard:
// when --trigger-type is set, only that family's condition flags may be
// passed. Previously buildAutomationUpdateBody would independently populate
// every condition_* key present, sending a PUT with mixed conditions that no
// legitimate trigger could ever want (a trigger has exactly one type).
func TestAutomationUpdate_CrossFamilyConditionFlagsRejected(t *testing.T) {
	rctx, _, _ := newOpenAPIKeyRCtx(t, automationUpdateFlagDefs(),
		map[string]string{
			"app-id": "app_x", "name": "t1", "trigger-type": "cron",
			"cron": "0 9 * * *", "white-ip-list": `["1.1.1.1"]`,
		})
	err := AppsAutomationUpdate.Validate(context.Background(), rctx)
	assertValidationParamError(t, err, "--white-ip-list")
}

// TestAutomationUpdate_MultiFamilyWithoutTriggerTypeRejected: when
// --trigger-type is absent but flags from more than one family are set, the
// Validate hook should refuse rather than dispatch a mixed-condition PUT.
// Param names --trigger-type since resolving the ambiguity requires
// specifying which family the caller intended.
func TestAutomationUpdate_MultiFamilyWithoutTriggerTypeRejected(t *testing.T) {
	rctx, _, _ := newOpenAPIKeyRCtx(t, automationUpdateFlagDefs(),
		map[string]string{
			"app-id": "app_x", "name": "t1",
			"cron": "0 9 * * *", "white-ip-list": `["1.1.1.1"]`,
		})
	err := AppsAutomationUpdate.Validate(context.Background(), rctx)
	assertValidationParamError(t, err, "--trigger-type")
	if !strings.Contains(err.Error(), "multiple trigger types") {
		t.Errorf("expected multi-family error message, got %q", err.Error())
	}
}

func TestAutomationUpdate_DescriptionTooLong(t *testing.T) {
	rctx, _, _ := newOpenAPIKeyRCtx(t, automationUpdateFlagDefs(),
		map[string]string{
			"app-id": "app_x", "name": "t1",
			"description": strings.Repeat("d", automationDescriptionMaxLen+1),
		})
	err := AppsAutomationUpdate.Validate(context.Background(), rctx)
	assertValidationParamError(t, err, "--description")
}

func TestAutomationUpdateMeta_HighRisk(t *testing.T) {
	if AppsAutomationUpdate.Risk != "high-risk-write" {
		t.Errorf("update must be high-risk-write, got %q", AppsAutomationUpdate.Risk)
	}
}
