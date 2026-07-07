// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/httpmock"
)

func automationListFlagDefs() map[string]string {
	return map[string]string{
		"app-id": "string", "trigger-type": "string",
		"page-size": "int", "page-token": "string", "all": "bool",
	}
}

func TestAutomationListMeta(t *testing.T) {
	if AppsAutomationList.Command != "+automation-list" || AppsAutomationList.Risk != "read" {
		t.Errorf("meta mismatch: %+v", AppsAutomationList)
	}
	if len(AppsAutomationList.Scopes) != 1 || AppsAutomationList.Scopes[0] != "spark:app:read" {
		t.Errorf("scopes = %v", AppsAutomationList.Scopes)
	}
}

func TestAutomationListExecute_SinglePage(t *testing.T) {
	rctx, stdoutBuf, reg := newOpenAPIKeyRCtx(t, automationListFlagDefs(),
		map[string]string{"app-id": "app_x"})
	reg.Register(&httpmock.Stub{
		Method: "GET", URL: "/open-apis/apaas/v1/apps/app_x/triggers",
		Body: map[string]interface{}{"code": 0, "msg": "", "data": map[string]interface{}{
			"items": []interface{}{
				map[string]interface{}{"name": "t_cron", "trigger_type": "cron", "status": "disabled"},
				map[string]interface{}{"name": "t_wh", "trigger_type": "webhook", "status": "enabled"},
			},
			"has_more": false, "page_token": "",
		}},
	})
	if err := AppsAutomationList.Execute(context.Background(), rctx); err != nil {
		t.Fatalf("Execute() = %v", err)
	}
	out := stdoutBuf.String()
	if !strings.Contains(out, "t_cron") || !strings.Contains(out, "t_wh") {
		t.Errorf("list must contain both triggers: %s", out)
	}
}

// --all aggregates every page until has_more=false. httpmock.Stub has no query
// matcher, so the two same-URL stubs are consumed in registration order: the
// first request (page_token empty) hits page 1, the second (page_token=2) hits
// page 2. See registry.match — a matched non-reusable stub is not reused.
func TestAutomationListExecute_AllAggregatesPages(t *testing.T) {
	rctx, stdoutBuf, reg := newOpenAPIKeyRCtx(t, automationListFlagDefs(),
		map[string]string{"app-id": "app_x", "all": "true"})
	// page 1: has_more=true, page_token="2"
	reg.Register(&httpmock.Stub{
		Method: "GET", URL: "/open-apis/apaas/v1/apps/app_x/triggers",
		Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{
			"items":    []interface{}{map[string]interface{}{"name": "p1", "trigger_type": "cron", "status": "disabled"}},
			"has_more": true, "page_token": "2",
		}},
	})
	// page 2: has_more=false
	reg.Register(&httpmock.Stub{
		Method: "GET", URL: "/open-apis/apaas/v1/apps/app_x/triggers",
		Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{
			"items":    []interface{}{map[string]interface{}{"name": "p2", "trigger_type": "webhook", "status": "enabled"}},
			"has_more": false, "page_token": "",
		}},
	})
	if err := AppsAutomationList.Execute(context.Background(), rctx); err != nil {
		t.Fatalf("Execute() = %v", err)
	}
	out := stdoutBuf.String()
	if !strings.Contains(out, "p1") || !strings.Contains(out, "p2") {
		t.Errorf("--all must aggregate both pages: %s", out)
	}
}

func TestAutomationListParams_TriggerTypePushdown(t *testing.T) {
	rctx, _, _ := newOpenAPIKeyRCtx(t, automationListFlagDefs(),
		map[string]string{"app-id": "app_x", "trigger-type": "webhook"})
	params := buildAutomationListParams(rctx)
	if params["trigger_type"] != "webhook" {
		t.Errorf("trigger_type must be pushed to query: %+v", params)
	}
}

// list/get 恒不返回明文 Bearer Token（spec Rule-2-2）。webhook item 的
// trigger_condition.token_value 必须逐条脱敏，token_enabled 保留。
func TestAutomationListExecute_RedactsWebhookToken(t *testing.T) {
	rctx, stdoutBuf, reg := newOpenAPIKeyRCtx(t, automationListFlagDefs(),
		map[string]string{"app-id": "app_x"})
	reg.Register(&httpmock.Stub{
		Method: "GET", URL: "/open-apis/apaas/v1/apps/app_x/triggers",
		Body: map[string]interface{}{"code": 0, "msg": "", "data": map[string]interface{}{
			"items": []interface{}{
				map[string]interface{}{
					"name": "t_wh", "trigger_type": "webhook", "status": "enabled",
					"trigger_condition": map[string]interface{}{
						"preview_url": "https://p", "runtime_url": "https://r",
						"token_enabled": true, "token_value": "PLAINTEXT_LIST_TOKEN",
					},
				},
			},
			"has_more": false, "page_token": "",
		}},
	})
	if err := AppsAutomationList.Execute(context.Background(), rctx); err != nil {
		t.Fatalf("Execute() = %v", err)
	}
	out := stdoutBuf.String()
	if strings.Contains(out, "PLAINTEXT_LIST_TOKEN") {
		t.Errorf("list must never surface plaintext token: %s", out)
	}
	if !strings.Contains(out, "token_enabled") {
		t.Errorf("list must expose token_enabled: %s", out)
	}
}
