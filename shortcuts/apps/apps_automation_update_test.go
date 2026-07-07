// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/httpmock"
)

func TestAutomationUpdate_PatchCronOnly(t *testing.T) {
	rctx, stdoutBuf, reg := newOpenAPIKeyRCtx(t, automationUpdateFlagDefs(),
		map[string]string{"app-id": "app_x", "name": "t1", "trigger-type": "cron", "cron": "0 10 * * *"})
	reg.Register(&httpmock.Stub{
		Method: "PATCH", URL: "/open-apis/apaas/v1/apps/app_x/triggers/t1",
		Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{"name": "t1", "trigger_type": "cron"}},
	})
	if err := runAutomationUpdate(rctx); err != nil {
		t.Fatalf("Execute() = %v", err)
	}
	if !strings.Contains(stdoutBuf.String(), "t1") {
		t.Errorf("update output = %s", stdoutBuf.String())
	}
}

func TestAutomationUpdate_MutuallyExclusiveWebhookFlags(t *testing.T) {
	rctx, _, _ := newOpenAPIKeyRCtx(t, automationUpdateFlagDefs(),
		map[string]string{"app-id": "app_x", "name": "t1", "reset-url": "true", "reset-token": "true"})
	if err := AppsAutomationUpdate.Validate(context.Background(), rctx); err == nil {
		t.Error("two webhook action flags at once must fail validation")
	}
}

func TestAutomationUpdate_WhiteIPListPatch(t *testing.T) {
	rctx, _, reg := newOpenAPIKeyRCtx(t, automationUpdateFlagDefs(),
		map[string]string{"app-id": "app_x", "name": "wh1", "trigger-type": "webhook", "white-ip-list": `["1.1.1.1"]`})
	reg.Register(&httpmock.Stub{
		Method: "PATCH", URL: "/open-apis/apaas/v1/apps/app_x/triggers/wh1",
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
	if err == nil {
		t.Fatal("illegal --cron (below 30-minute minimum) must be rejected up-front")
	}
	msg := err.Error()
	if !strings.Contains(msg, "cron") && !strings.Contains(msg, "30") {
		t.Errorf("error should be cron-specific, got %q", msg)
	}
}

func TestAutomationUpdate_InvalidWhiteIPListRejected(t *testing.T) {
	rctx, _, _ := newOpenAPIKeyRCtx(t, automationUpdateFlagDefs(),
		map[string]string{"app-id": "app_x", "name": "wh1", "trigger-type": "webhook", "white-ip-list": "{bad json"})
	if err := runAutomationUpdate(rctx); err == nil {
		t.Fatal("illegal --white-ip-list must be rejected up-front")
	}
}

func TestAutomationUpdateMeta_HighRisk(t *testing.T) {
	if AppsAutomationUpdate.Risk != "high-risk-write" {
		t.Errorf("update must be high-risk-write, got %q", AppsAutomationUpdate.Risk)
	}
}
