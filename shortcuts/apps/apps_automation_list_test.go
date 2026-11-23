// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/httpmock"
)

func automationListFlagDefs() map[string]string {
	return map[string]string{
		"app-id": "string", "trigger-type": "string",
		"page-size": "int", "page-token": "string", "all": "bool",
	}
}

// TestAutomationList_InvalidTriggerTypeFilter covers Validate's mapTriggerType
// error branch: an unknown --trigger-type is rejected before any API call, with
// a typed error naming the failing flag.
func TestAutomationList_InvalidTriggerTypeFilter(t *testing.T) {
	rctx, _, _ := newOpenAPIKeyRCtx(t, automationListFlagDefs(),
		map[string]string{"app-id": "app_x", "trigger-type": "bogus"})
	err := AppsAutomationList.Validate(context.Background(), rctx)
	assertValidationParamError(t, err, "--trigger-type")
}

// TestAutomationListExecute_APIErrorAttachesAppIDHint covers the non-`--all`
// error branch: a business error is surfaced typed and carries appIDListHint,
// which points at +list rather than +automation-list because the recovery for
// a failing collection GET is "check your app-id", not "check trigger names".
func TestAutomationListExecute_APIErrorAttachesAppIDHint(t *testing.T) {
	rctx, _, reg := newOpenAPIKeyRCtx(t, automationListFlagDefs(),
		map[string]string{"app-id": "app_x"})
	reg.Register(&httpmock.Stub{
		Method: "GET", URL: "/open-apis/spark/v1/apps/app_x/triggers",
		Body: map[string]interface{}{"code": 400400002, "msg": "app not accessible"},
	})
	err := AppsAutomationList.Execute(context.Background(), rctx)
	if err == nil {
		t.Fatal("expected typed api error, got nil")
	}
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed problem, got %T: %v", err, err)
	}
	if p.Category != errs.CategoryAPI {
		t.Errorf("category = %q, want %q", p.Category, errs.CategoryAPI)
	}
	if p.Subtype == "" {
		t.Error("subtype must be populated on typed API errors")
	}
	if !strings.Contains(p.Hint, "apps +list") {
		t.Errorf("hint must point at `lark-cli apps +list`, got %q", p.Hint)
	}
}

// TestAutomationList_DryRunPreview exercises the DryRun closure — pins the GET
// method + collection URL + trigger_type param pushdown.
func TestAutomationList_DryRunPreview(t *testing.T) {
	rctx, _, _ := newOpenAPIKeyRCtx(t, automationListFlagDefs(),
		map[string]string{"app-id": "app_x", "trigger-type": "webhook"})
	preview := AppsAutomationList.DryRun(context.Background(), rctx)
	if preview == nil {
		t.Fatal("DryRun returned nil")
	}
	blob, err := preview.MarshalJSON()
	if err != nil {
		t.Fatalf("marshal preview: %v", err)
	}
	got := string(blob)
	if !strings.Contains(got, `"method":"GET"`) ||
		!strings.Contains(got, "/apps/app_x/triggers") ||
		!strings.Contains(got, `"trigger_type":"webhook"`) {
		t.Errorf("preview missing expected GET/URL/params: %s", got)
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
		Method: "GET", URL: "/open-apis/spark/v1/apps/app_x/triggers",
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
		Method: "GET", URL: "/open-apis/spark/v1/apps/app_x/triggers",
		Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{
			"items":    []interface{}{map[string]interface{}{"name": "p1", "trigger_type": "cron", "status": "disabled"}},
			"has_more": true, "page_token": "2",
		}},
	})
	// page 2: has_more=false
	reg.Register(&httpmock.Stub{
		Method: "GET", URL: "/open-apis/spark/v1/apps/app_x/triggers",
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

// list/get 恒不返回明文 Bearer Token。webhook item 的
// trigger_condition.token_value 必须逐条脱敏，token_enabled 保留。
func TestAutomationListExecute_RedactsWebhookToken(t *testing.T) {
	rctx, stdoutBuf, reg := newOpenAPIKeyRCtx(t, automationListFlagDefs(),
		map[string]string{"app-id": "app_x"})
	reg.Register(&httpmock.Stub{
		Method: "GET", URL: "/open-apis/spark/v1/apps/app_x/triggers",
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

// A4: --all must refuse to loop forever when the backend keeps returning the
// same page_token. A reusable stub that always advertises "has_more=true,
// page_token=same" forces the seen-token guard to trip.
func TestAutomationListExecute_All_DetectsRepeatedPageToken(t *testing.T) {
	rctx, _, reg := newOpenAPIKeyRCtx(t, automationListFlagDefs(),
		map[string]string{"app-id": "app_x", "all": "true"})
	reg.Register(&httpmock.Stub{
		Method: "GET", URL: "/open-apis/spark/v1/apps/app_x/triggers",
		Reusable: true,
		Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{
			"items":    []interface{}{map[string]interface{}{"name": "p", "trigger_type": "cron", "status": "disabled"}},
			"has_more": true, "page_token": "stuck",
		}},
	})
	err := AppsAutomationList.Execute(context.Background(), rctx)
	// The seen-token detector must raise a typed internal/invalid_response error
	// long before the caller sees a runaway loop.
	assertInternalError(t, err, errs.SubtypeInvalidResponse)
}

// A4: --all must also refuse to loop forever when the backend keeps issuing new
// distinct page_tokens without ever setting has_more=false. The page-cap kicks
// in at automationListAllMaxPages. Simulated by a reusable stub advertising a
// fresh non-repeating token via monotonically increasing counter — but since
// httpmock has no dynamic bodies, we lean on the fact that the same reusable
// body advertises page_token="stuck" (the seen-token guard trips first). This
// case is left to the sibling test above; the page-cap constant is asserted
// here so a future refactor cannot silently drop the ceiling.
func TestAutomationListAll_PageCapConstant(t *testing.T) {
	if automationListAllMaxPages <= 0 || automationListAllMaxPages > 1000 {
		t.Errorf("automationListAllMaxPages = %d; must be a small positive ceiling", automationListAllMaxPages)
	}
}
