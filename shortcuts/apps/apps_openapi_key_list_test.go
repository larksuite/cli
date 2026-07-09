// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/spf13/cobra"
)

// newOpenAPIKeyRCtx 构造带指定 flag 的 RuntimeContext。flags 是 name->value，
// bool flag 传 "true"/"false"。被本组所有命令测试复用。
func newOpenAPIKeyRCtx(t *testing.T, flagDefs map[string]string, flags map[string]string) (*common.RuntimeContext, *bytes.Buffer, *httpmock.Registry) {
	t.Helper()
	cfg := &core.CliConfig{
		AppID:      "test-app-" + strings.ToLower(t.Name()),
		AppSecret:  "test-secret",
		Brand:      core.BrandFeishu,
		UserOpenId: "ou_test",
	}
	factory, stdoutBuf, _, reg := cmdutil.TestFactory(t, cfg)
	cmd := &cobra.Command{Use: "test-openapi-key"}
	cmd.SetContext(context.Background())
	for name, typ := range flagDefs {
		switch typ {
		case "bool":
			cmd.Flags().Bool(name, false, "")
		case "int":
			cmd.Flags().Int(name, 0, "")
		case "string_array":
			cmd.Flags().StringArray(name, nil, "")
		default:
			cmd.Flags().String(name, "", "")
		}
	}
	for name, val := range flags {
		_ = cmd.Flags().Set(name, val)
	}
	rctx := common.TestNewRuntimeContextForAPI(context.Background(), cmd, cfg, factory, core.AsUser)
	return rctx, stdoutBuf, reg
}

func TestOpenAPIKeyListMeta(t *testing.T) {
	if AppsOpenAPIKeyList.Command != "+openapi-key-list" || AppsOpenAPIKeyList.Risk != "read" {
		t.Errorf("meta mismatch: %+v", AppsOpenAPIKeyList)
	}
	if len(AppsOpenAPIKeyList.Scopes) != 1 || AppsOpenAPIKeyList.Scopes[0] != "spark:app:read" {
		t.Errorf("scopes = %v", AppsOpenAPIKeyList.Scopes)
	}
}

func TestOpenAPIKeyListExecute_Redacts(t *testing.T) {
	rctx, stdoutBuf, reg := newOpenAPIKeyRCtx(t,
		map[string]string{"app-id": "string", "limit": "int", "offset": "int"},
		map[string]string{"app-id": "app_x"})
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/spark/v1/apps/app_x/oapi_apikeys",
		Body: map[string]interface{}{
			"code": 0, "msg": "",
			"data": map[string]interface{}{
				"infos": []interface{}{
					map[string]interface{}{
						"api_key_id": "k1", "name": "partner-test",
						"api_key": "xxxxxxxxxxxx", "status": float64(1),
					},
				},
			},
		},
	})
	if err := AppsOpenAPIKeyList.Execute(context.Background(), rctx); err != nil {
		t.Fatalf("Execute() = %v", err)
	}
	out := stdoutBuf.String()
	if strings.Contains(out, "xxxxxxxxxxxx") {
		t.Fatalf("list output leaked raw api key: %s", out)
	}
	if !strings.Contains(out, "****xxxx") {
		t.Errorf("expected masked key_preview in output: %s", out)
	}
	_ = json.Valid
}
