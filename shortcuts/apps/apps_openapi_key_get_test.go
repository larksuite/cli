// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/httpmock"
)

func TestOpenAPIKeyGetExecute_Redacts(t *testing.T) {
	rctx, stdoutBuf, reg := newOpenAPIKeyRCtx(t,
		map[string]string{"app-id": "string", "key-id": "string"},
		map[string]string{"app-id": "app_x", "key-id": "1"})
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/spark/v1/apps/app_x/oapi_apikeys/1",
		Body: map[string]interface{}{
			"code": 0, "msg": "",
			"data": map[string]interface{}{
				"info": map[string]interface{}{
					"api_key_id": "k1", "name": "partner-test",
					"api_key": "xxxxxxxxxxxx", "status": float64(1),
				},
			},
		},
	})
	if err := AppsOpenAPIKeyGet.Execute(context.Background(), rctx); err != nil {
		t.Fatalf("Execute() = %v", err)
	}
	if strings.Contains(stdoutBuf.String(), "xxxxxxxxxxxx") {
		t.Fatalf("get output leaked raw api key: %s", stdoutBuf.String())
	}
	if !strings.Contains(stdoutBuf.String(), "****xxxx") {
		t.Errorf("expected key_preview: %s", stdoutBuf.String())
	}
}

func TestOpenAPIKeyGetExecute_MissingKeyID(t *testing.T) {
	rctx, _, _ := newOpenAPIKeyRCtx(t,
		map[string]string{"app-id": "string", "key-id": "string"},
		map[string]string{"app-id": "app_x"})
	if err := AppsOpenAPIKeyGet.Validate(context.Background(), rctx); err == nil {
		t.Errorf("missing --key-id must fail validation")
	}
}
