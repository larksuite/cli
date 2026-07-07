// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/httpmock"
)

func TestAutomationGetExecute_RedactsWebhookToken(t *testing.T) {
	rctx, stdoutBuf, reg := newOpenAPIKeyRCtx(t,
		map[string]string{"app-id": "string", "name": "string"},
		map[string]string{"app-id": "app_x", "name": "wh1"})
	reg.Register(&httpmock.Stub{
		Method: "GET", URL: "/open-apis/apaas/v1/apps/app_x/triggers/wh1",
		Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{
			"name": "wh1", "trigger_type": "webhook", "status": "enabled",
			"trigger_condition": map[string]interface{}{
				"preview_url": "https://p", "runtime_url": "https://r",
				"token_enabled": true, "token_value": "PLAINTEXT_SECRET",
			},
		}},
	})
	if err := AppsAutomationGet.Execute(context.Background(), rctx); err != nil {
		t.Fatalf("Execute() = %v", err)
	}
	out := stdoutBuf.String()
	if strings.Contains(out, "PLAINTEXT_SECRET") {
		t.Errorf("get must never surface plaintext token: %s", out)
	}
	if !strings.Contains(out, "token_enabled") {
		t.Errorf("get must expose token_enabled: %s", out)
	}
}

func TestAutomationGet_MissingName(t *testing.T) {
	rctx, _, _ := newOpenAPIKeyRCtx(t,
		map[string]string{"app-id": "string", "name": "string"},
		map[string]string{"app-id": "app_x"})
	if err := AppsAutomationGet.Validate(context.Background(), rctx); err == nil {
		t.Error("missing --name must fail validation")
	}
}
