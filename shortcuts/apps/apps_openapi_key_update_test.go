// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/httpmock"
)

func TestOpenAPIKeyUpdate_RequiresOneField(t *testing.T) {
	rctx, _, _ := newOpenAPIKeyRCtx(t,
		map[string]string{"app-id": "string", "key-id": "string", "name": "string", "scope": "string", "allow-preview": "bool"},
		map[string]string{"app-id": "app_x", "key-id": "1"})
	if err := AppsOpenAPIKeyUpdate.Validate(context.Background(), rctx); err == nil {
		t.Errorf("update with no changeable field must fail validation")
	}
}

func TestOpenAPIKeyUpdateExecute_Redacts(t *testing.T) {
	rctx, stdoutBuf, reg := newOpenAPIKeyRCtx(t,
		map[string]string{"app-id": "string", "key-id": "string", "name": "string", "scope": "string", "allow-preview": "bool"},
		map[string]string{"app-id": "app_x", "key-id": "1", "name": "partner-prod"})
	reg.Register(&httpmock.Stub{
		Method: "PATCH",
		URL:    "/open-apis/spark/v1/apps/app_x/oapi_apikeys/1",
		Body: map[string]interface{}{
			"code": 0, "msg": "",
			"data": map[string]interface{}{
				"info": map[string]interface{}{
					"api_key_id": "1", "name": "partner-prod",
					"api_key": "mdk_live_9f3a2b8c5f4a", "status": float64(1),
				},
			},
		},
	})
	if err := AppsOpenAPIKeyUpdate.Execute(context.Background(), rctx); err != nil {
		t.Fatalf("Execute() = %v", err)
	}
	if strings.Contains(stdoutBuf.String(), "mdk_live_9f3a2b8c5f4a") {
		t.Fatalf("update leaked raw api_key: %s", stdoutBuf.String())
	}
}
