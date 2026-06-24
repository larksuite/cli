// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/httpmock"
)

func TestOpenAPIKeyCreateExecute_ReturnsRawOnce(t *testing.T) {
	rctx, stdoutBuf, reg := newOpenAPIKeyRCtx(t,
		map[string]string{"app-id": "string", "name": "string", "scope": "string", "allow-preview": "bool"},
		map[string]string{"app-id": "app_x", "name": "partner-test"})
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps/app_x/oapi_apikeys",
		Body: map[string]interface{}{
			"code": 0, "msg": "",
			"data": map[string]interface{}{
				"api_key_id": "1",
				"info": map[string]interface{}{
					"api_key_id": "1", "name": "partner-test",
					"api_key": "mdk_live_9f3a2b8c5f4a", "status": float64(1),
				},
			},
		},
	})
	if err := AppsOpenAPIKeyCreate.Execute(context.Background(), rctx); err != nil {
		t.Fatalf("Execute() = %v", err)
	}
	out := stdoutBuf.String()
	// create surfaces the raw secret ONCE at top-level api_key
	if !strings.Contains(out, "mdk_live_9f3a2b8c5f4a") {
		t.Fatalf("create must surface raw api_key once: %s", out)
	}
	// nested info must be redacted
	if strings.Count(out, "mdk_live_9f3a2b8c5f4a") != 1 {
		t.Errorf("raw key must appear exactly once (top-level only): %s", out)
	}
	if !strings.Contains(out, "****5f4a") {
		t.Errorf("redacted info must carry key_preview: %s", out)
	}
}

func TestOpenAPIKeyCreate_MissingName(t *testing.T) {
	rctx, _, _ := newOpenAPIKeyRCtx(t,
		map[string]string{"app-id": "string", "name": "string", "scope": "string", "allow-preview": "bool"},
		map[string]string{"app-id": "app_x"})
	if err := AppsOpenAPIKeyCreate.Validate(context.Background(), rctx); err == nil {
		t.Errorf("missing --name must fail validation")
	}
}

func TestOpenAPIKeyCreate_InvalidScope(t *testing.T) {
	rctx, _, _ := newOpenAPIKeyRCtx(t,
		map[string]string{"app-id": "string", "name": "string", "scope": "string", "allow-preview": "bool"},
		map[string]string{"app-id": "app_x", "name": "n", "scope": "{bad"})
	if err := AppsOpenAPIKeyCreate.Validate(context.Background(), rctx); err == nil {
		t.Errorf("invalid --scope json must fail validation")
	}
}
