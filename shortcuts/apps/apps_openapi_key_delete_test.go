// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/httpmock"
)

func TestOpenAPIKeyDeleteMeta_HighRisk(t *testing.T) {
	if AppsOpenAPIKeyDelete.Risk != "high-risk-write" {
		t.Errorf("delete must be high-risk-write, got %q", AppsOpenAPIKeyDelete.Risk)
	}
}

func TestOpenAPIKeyDeleteExecute(t *testing.T) {
	rctx, stdoutBuf, reg := newOpenAPIKeyRCtx(t,
		map[string]string{"app-id": "string", "key-id": "string", "yes": "bool"},
		map[string]string{"app-id": "app_x", "key-id": "1", "yes": "true"})
	reg.Register(&httpmock.Stub{
		Method: "DELETE",
		URL:    "/open-apis/spark/v1/apps/app_x/oapi_apikeys/1",
		Body:   map[string]interface{}{"code": 0, "msg": "", "data": map[string]interface{}{}},
	})
	if err := AppsOpenAPIKeyDelete.Execute(context.Background(), rctx); err != nil {
		t.Fatalf("Execute() = %v", err)
	}
	if !strings.Contains(stdoutBuf.String(), "\"deleted\"") && !strings.Contains(stdoutBuf.String(), "deleted") {
		t.Errorf("expected deleted marker: %s", stdoutBuf.String())
	}
}
