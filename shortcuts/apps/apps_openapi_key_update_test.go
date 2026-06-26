// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/httpmock"
)

// updateFlagDefs returns the flag type map for +openapi-key-update tests.
func updateFlagDefs() map[string]string {
	return map[string]string{
		"app-id":        "string",
		"key-id":        "string",
		"name":          "string",
		"scope-all":     "bool",
		"scope-api":     "string_array",
		"scope":         "string",
		"allow-preview": "bool",
	}
}

func TestOpenAPIKeyUpdate_RequiresOneField(t *testing.T) {
	rctx, _, _ := newOpenAPIKeyRCtx(t,
		updateFlagDefs(),
		map[string]string{"app-id": "app_x", "key-id": "1"})
	err := AppsOpenAPIKeyUpdate.Validate(context.Background(), rctx)
	if err == nil {
		t.Errorf("update with no changeable field must fail validation")
	}
	if err != nil && !strings.Contains(err.Error(), "at least one of --name / --scope-all / --scope-api / --scope / --allow-preview is required") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestOpenAPIKeyUpdateExecute_Redacts(t *testing.T) {
	rctx, stdoutBuf, reg := newOpenAPIKeyRCtx(t,
		updateFlagDefs(),
		map[string]string{"app-id": "app_x", "key-id": "1", "name": "partner-prod"})
	reg.Register(&httpmock.Stub{
		Method: "PATCH",
		URL:    "/open-apis/spark/v1/apps/app_x/oapi_apikeys/1",
		Body: map[string]interface{}{
			"code": 0, "msg": "",
			"data": map[string]interface{}{
				"info": map[string]interface{}{
					"api_key_id": "k1", "name": "partner-prod",
					"api_key": "xxxxxxxxxxxx", "status": float64(1),
				},
			},
		},
	})
	if err := AppsOpenAPIKeyUpdate.Execute(context.Background(), rctx); err != nil {
		t.Fatalf("Execute() = %v", err)
	}
	if strings.Contains(stdoutBuf.String(), "xxxxxxxxxxxx") {
		t.Fatalf("update leaked raw api key: %s", stdoutBuf.String())
	}
}
