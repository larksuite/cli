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

// TestAutomationGetExecute_RedactsWebhookToken pins the redaction invariant
// against the actual backend response shape (verified against a live test
// environment): GET wraps the trigger under a `trigger` key, so the CLI
// must scrub token_value inside data.trigger.trigger_condition. A previous
// implementation only scrubbed data.trigger_condition and silently no-op'd
// here — this test would fail the moment someone reverts to top-level-only
// scrubbing.
func TestAutomationGetExecute_RedactsWebhookToken(t *testing.T) {
	rctx, stdoutBuf, reg := newOpenAPIKeyRCtx(t,
		map[string]string{"app-id": "string", "name": "string"},
		map[string]string{"app-id": "app_x", "name": "wh1"})
	reg.Register(&httpmock.Stub{
		Method: "GET", URL: "/open-apis/spark/v1/apps/app_x/triggers/wh1",
		Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{
			"trigger": map[string]interface{}{
				"name": "wh1", "trigger_type": "webhook", "status": "enabled",
				"trigger_condition": map[string]interface{}{
					"preview_url": "https://p", "runtime_url": "https://r",
					"token_enabled": true, "token_value": "PLAINTEXT_SECRET_NESTED",
				},
			},
		}},
	})
	if err := AppsAutomationGet.Execute(context.Background(), rctx); err != nil {
		t.Fatalf("Execute() = %v", err)
	}
	out := stdoutBuf.String()
	if strings.Contains(out, "PLAINTEXT_SECRET_NESTED") {
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
	err := AppsAutomationGet.Validate(context.Background(), rctx)
	assertValidationParamError(t, err, "--name")
}

// TestAutomationGet_MissingAppID covers the sibling branch of Validate:
// automationValidateName rejects an empty --app-id before checking --name.
func TestAutomationGet_MissingAppID(t *testing.T) {
	rctx, _, _ := newOpenAPIKeyRCtx(t,
		map[string]string{"app-id": "string", "name": "string"},
		map[string]string{"name": "t1"})
	err := AppsAutomationGet.Validate(context.Background(), rctx)
	assertValidationParamError(t, err, "--app-id")
}

// TestAutomationGet_APIErrorAttachesNotFoundHint covers the failure branch of
// Execute: a business error on GET must surface typed and carry the
// automation-list hint so the caller has a next step.
func TestAutomationGet_APIErrorAttachesNotFoundHint(t *testing.T) {
	rctx, _, reg := newOpenAPIKeyRCtx(t,
		map[string]string{"app-id": "string", "name": "string"},
		map[string]string{"app-id": "app_x", "name": "missing"})
	reg.Register(&httpmock.Stub{
		Method: "GET", URL: "/open-apis/spark/v1/apps/app_x/triggers/missing",
		Body: map[string]interface{}{"code": 400400001, "msg": "trigger not found"},
	})
	err := AppsAutomationGet.Execute(context.Background(), rctx)
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
	if !strings.Contains(p.Hint, "+automation-list") {
		t.Errorf("hint must point at +automation-list, got %q", p.Hint)
	}
}

// TestAutomationGet_DryRunPreview exercises the DryRun closure and pins the
// GET method + URL pattern that agents inspect before committing.
func TestAutomationGet_DryRunPreview(t *testing.T) {
	rctx, _, _ := newOpenAPIKeyRCtx(t,
		map[string]string{"app-id": "string", "name": "string"},
		map[string]string{"app-id": "app_x", "name": "t1"})
	preview := AppsAutomationGet.DryRun(context.Background(), rctx)
	if preview == nil {
		t.Fatal("DryRun returned nil")
	}
	blob, err := preview.MarshalJSON()
	if err != nil {
		t.Fatalf("marshal preview: %v", err)
	}
	got := string(blob)
	if !strings.Contains(got, `"method":"GET"`) ||
		!strings.Contains(got, "/apps/app_x/triggers/t1") {
		t.Errorf("preview missing expected GET/URL fields: %s", got)
	}
}
