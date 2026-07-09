// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/httpmock"
)

func TestOpenAPIKeyResetMeta_HighRisk(t *testing.T) {
	if AppsOpenAPIKeyReset.Risk != "high-risk-write" {
		t.Errorf("reset must be high-risk-write, got %q", AppsOpenAPIKeyReset.Risk)
	}
}

func TestOpenAPIKeyResetExecute_ReturnsNewRaw(t *testing.T) {
	rctx, stdoutBuf, reg := newOpenAPIKeyRCtx(t,
		map[string]string{"app-id": "string", "key-id": "string", "yes": "bool"},
		map[string]string{"app-id": "app_x", "key-id": "1", "yes": "true"})
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/apps/app_x/oapi_apikeys/1/refresh",
		Body: map[string]interface{}{
			"code": 0, "msg": "",
			"data": map[string]interface{}{
				"api_key": "xxxxxxxxxxxx",
				"info":    map[string]interface{}{"api_key_id": "k1", "name": "k", "api_key": "xxxxxxxxxxxx", "status": float64(1)},
			},
		},
	})
	if err := AppsOpenAPIKeyReset.Execute(context.Background(), rctx); err != nil {
		t.Fatalf("Execute() = %v", err)
	}
	out := stdoutBuf.String()
	if !strings.Contains(out, "xxxxxxxxxxxx") {
		t.Fatalf("reset must surface the new raw secret once: %s", out)
	}
	if strings.Count(out, "xxxxxxxxxxxx") != 1 {
		t.Errorf("raw key must appear exactly once (top-level only, info must be redacted): %s", out)
	}
	if !strings.Contains(out, "****xxxx") {
		t.Errorf("redacted info must carry key_preview: %s", out)
	}
}
