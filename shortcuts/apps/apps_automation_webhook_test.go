// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/httpmock"
)

func automationUpdateFlagDefs() map[string]string {
	return map[string]string{
		"app-id": "string", "name": "string", "trigger-type": "string", "description": "string",
		"cron": "string", "timezone": "string", "white-ip-list": "string",
		"reset-url": "bool", "app-env": "string",
		"enable-token": "bool", "disable-token": "bool", "reset-token": "bool",
	}
}

func TestWebhookResetURL_RequiresAppEnv(t *testing.T) {
	rctx, _, _ := newOpenAPIKeyRCtx(t, automationUpdateFlagDefs(),
		map[string]string{"app-id": "app_x", "name": "wh1", "reset-url": "true"})
	if err := runWebhookURLReset(rctx); err == nil {
		t.Error("--reset-url without --app-env must error")
	}
}

func TestWebhookResetURL_InvalidAppEnv(t *testing.T) {
	rctx, _, _ := newOpenAPIKeyRCtx(t, automationUpdateFlagDefs(),
		map[string]string{"app-id": "app_x", "name": "wh1", "reset-url": "true", "app-env": "prod"})
	if err := runWebhookURLReset(rctx); err == nil {
		t.Error("--app-env must be preview or runtime")
	}
}

func TestWebhookResetURL_PostsAppEnv(t *testing.T) {
	rctx, stdoutBuf, reg := newOpenAPIKeyRCtx(t, automationUpdateFlagDefs(),
		map[string]string{"app-id": "app_x", "name": "wh1", "reset-url": "true", "app-env": "preview"})
	reg.Register(&httpmock.Stub{
		Method: "POST", URL: "/open-apis/apaas/v1/apps/app_x/triggers/wh1/webhook/url/reset",
		Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{"preview_url": "https://new-preview"}},
	})
	if err := runWebhookURLReset(rctx); err != nil {
		t.Fatalf("Execute() = %v", err)
	}
	if !strings.Contains(stdoutBuf.String(), "new-preview") {
		t.Errorf("reset-url must return new URL: %s", stdoutBuf.String())
	}
}

func TestWebhookEnableToken_SurfacesTokenOnce(t *testing.T) {
	rctx, stdoutBuf, reg := newOpenAPIKeyRCtx(t, automationUpdateFlagDefs(),
		map[string]string{"app-id": "app_x", "name": "wh1", "enable-token": "true"})
	reg.Register(&httpmock.Stub{
		Method: "POST", URL: "/open-apis/apaas/v1/apps/app_x/triggers/wh1/webhook/token/status",
		Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{"token_value": "BEARER_ONCE"}},
	})
	if err := runWebhookTokenStatus(rctx, true); err != nil {
		t.Fatalf("Execute() = %v", err)
	}
	out := stdoutBuf.String()
	if !strings.Contains(out, "BEARER_ONCE") {
		t.Errorf("enable-token must surface token once: %s", out)
	}
}
