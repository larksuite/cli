// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/httpmock"
)

// createFlagDefs returns the flag type map for +openapi-key-create tests.
func createFlagDefs() map[string]string {
	return map[string]string{
		"app-id":        "string",
		"name":          "string",
		"scope-all":     "bool",
		"scope-api":     "string_array",
		"scope":         "string",
		"allow-preview": "bool",
	}
}

func TestOpenAPIKeyCreateExecute_ReturnsRawOnce(t *testing.T) {
	rctx, stdoutBuf, reg := newOpenAPIKeyRCtx(t,
		createFlagDefs(),
		map[string]string{"app-id": "app_x", "name": "partner-test"})
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps/app_x/oapi_apikeys",
		Body: map[string]interface{}{
			"code": 0, "msg": "",
			"data": map[string]interface{}{
				"api_key_id": "k1",
				"info": map[string]interface{}{
					"api_key_id": "k1", "name": "partner-test",
					"api_key": "xxxxxxxxxxxx", "status": float64(1),
				},
			},
		},
	})
	if err := AppsOpenAPIKeyCreate.Execute(context.Background(), rctx); err != nil {
		t.Fatalf("Execute() = %v", err)
	}
	out := stdoutBuf.String()
	// create surfaces the raw secret ONCE at top-level api_key
	if !strings.Contains(out, "xxxxxxxxxxxx") {
		t.Fatalf("create must surface raw api_key once: %s", out)
	}
	// nested info must be redacted — raw key appears exactly once (top-level only)
	if strings.Count(out, "xxxxxxxxxxxx") != 1 {
		t.Errorf("raw key must appear exactly once (top-level only): %s", out)
	}
	if !strings.Contains(out, "****xxxx") {
		t.Errorf("redacted info must carry key_preview: %s", out)
	}
}

func TestOpenAPIKeyCreate_MissingName(t *testing.T) {
	rctx, _, _ := newOpenAPIKeyRCtx(t,
		createFlagDefs(),
		map[string]string{"app-id": "app_x"})
	if err := AppsOpenAPIKeyCreate.Validate(context.Background(), rctx); err == nil {
		t.Errorf("missing --name must fail validation")
	}
}

func TestOpenAPIKeyCreate_InvalidScope(t *testing.T) {
	rctx, _, _ := newOpenAPIKeyRCtx(t,
		createFlagDefs(),
		map[string]string{"app-id": "app_x", "name": "n", "scope": "{bad"})
	if err := AppsOpenAPIKeyCreate.Validate(context.Background(), rctx); err == nil {
		t.Errorf("invalid --scope json must fail validation")
	}
}

func TestOpenAPIKeyCreate_ScopeRawAndFriendlyMutuallyExclusive(t *testing.T) {
	rctx, _, _ := newOpenAPIKeyRCtx(t,
		createFlagDefs(),
		map[string]string{"app-id": "app_x", "name": "n", "scope": `{"allowAll":true}`, "scope-all": "true"})
	if err := AppsOpenAPIKeyCreate.Validate(context.Background(), rctx); err == nil {
		t.Errorf("--scope + --scope-all must fail validation")
	}
}
