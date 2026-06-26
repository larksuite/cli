// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/httpmock"
)

func TestOpenAPIKeyEnableExecute_StatusOne(t *testing.T) {
	rctx, stdoutBuf, reg := newOpenAPIKeyRCtx(t,
		map[string]string{"app-id": "string", "key-id": "string"},
		map[string]string{"app-id": "app_x", "key-id": "1"})
	reg.Register(&httpmock.Stub{
		Method: "PATCH",
		URL:    "/open-apis/spark/v1/apps/app_x/oapi_apikeys/1",
		Body: map[string]interface{}{
			"code": 0, "msg": "",
			"data": map[string]interface{}{
				"info": map[string]interface{}{"api_key_id": "k1", "name": "k", "api_key": "xxxxxxxxxxxx", "status": float64(1)},
			},
		},
	})
	if err := AppsOpenAPIKeyEnable.Execute(context.Background(), rctx); err != nil {
		t.Fatalf("Execute() = %v", err)
	}
	if strings.Contains(stdoutBuf.String(), "xxxxxxxxxxxx") {
		t.Fatalf("enable leaked raw api_key")
	}
}

func TestOpenAPIKeyStatusBody(t *testing.T) {
	if b := openAPIKeyStatusBody(1); b["status"] != 1 {
		t.Errorf("enable body = %v", b)
	}
	if b := openAPIKeyStatusBody(0); b["status"] != 0 {
		t.Errorf("disable body = %v", b)
	}
}
