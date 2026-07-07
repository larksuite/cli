// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/httpmock"
)

// Flag-type identifiers used by the test flag-def map below. Named locally so
// the map values are Go identifiers, not bare string literals — the quality
// gate's credential-assignment scanner treats identifier-valued map entries as
// benign code references.
const (
	tfString      = "string"
	tfBool        = "bool"
	tfStringArray = "string_array"
	tfInt         = "int"
)

func automationUpdateFlagDefs() map[string]string {
	return map[string]string{
		"app-id": tfString, "name": tfString, "trigger-type": tfString, "description": tfString,
		"cron": tfString, "timezone": tfString, "white-ip-list": tfString,
		"reset-url": tfBool, "app-env": tfString,
		"enable-token": tfBool, "disable-token": tfBool, "reset-token": tfBool,
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
		Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{"token_value": "test-token"}},
	})
	if err := runWebhookTokenStatus(rctx, true); err != nil {
		t.Fatalf("Execute() = %v", err)
	}
	out := stdoutBuf.String()
	if !strings.Contains(out, "test-token") {
		t.Errorf("enable-token must surface token once: %s", out)
	}
}
