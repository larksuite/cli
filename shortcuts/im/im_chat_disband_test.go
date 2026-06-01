// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/spf13/cobra"
)

func TestImChatDisbandExecute_DeletesChat(t *testing.T) {
	ctx := context.Background()
	cmd := newChatDisbandTestCommand(t, map[string]string{
		"chat-id": "oc_123",
	})
	cfg := &core.CliConfig{AppID: "cli_test", AppSecret: "secret", Brand: core.BrandFeishu}
	f, _, _, reg := cmdutil.TestFactory(t, cfg)
	stub := &httpmock.Stub{
		Method: http.MethodDelete,
		URL:    "/open-apis/im/v1/chats/oc_123",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{},
		},
	}
	reg.Register(stub)

	rt := common.TestNewRuntimeContextForAPI(ctx, cmd, cfg, f, core.AsBot)
	if err := ImChatDisband.Execute(ctx, rt); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if len(stub.CapturedBody) != 0 {
		var body map[string]interface{}
		if err := json.Unmarshal(stub.CapturedBody, &body); err != nil {
			t.Fatalf("request body invalid JSON: %v\n%s", err, string(stub.CapturedBody))
		}
		if len(body) != 0 {
			t.Fatalf("request body = %#v, want empty", body)
		}
	}
}

func newChatDisbandTestCommand(t *testing.T, flags map[string]string) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("chat-id", "", "")
	if err := cmd.ParseFlags(nil); err != nil {
		t.Fatalf("ParseFlags() error = %v", err)
	}
	for name, value := range flags {
		if err := cmd.Flags().Set(name, value); err != nil {
			t.Fatalf("Flags().Set(%q) error = %v", name, err)
		}
	}
	return cmd
}
