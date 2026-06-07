// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/spf13/cobra"
)

func TestImMessagesUpdateValidate(t *testing.T) {
	tests := []struct {
		name    string
		flags   map[string]string
		wantErr string
	}{
		{
			name: "accepts text",
			flags: map[string]string{
				"message-id": "om_123",
				"text":       "updated",
				"msg-type":   "text",
			},
		},
		{
			name: "rejects thread id",
			flags: map[string]string{
				"message-id": "omt_123",
				"text":       "updated",
				"msg-type":   "text",
			},
			wantErr: "must start with om_",
		},
		{
			name: "rejects multiple content inputs",
			flags: map[string]string{
				"message-id": "om_123",
				"text":       "updated",
				"content":    `{"text":"updated"}`,
				"msg-type":   "text",
			},
			wantErr: "exactly one",
		},
		{
			name: "rejects unsupported message type",
			flags: map[string]string{
				"message-id": "om_123",
				"content":    `{"image_key":"img_123"}`,
				"msg-type":   "image",
			},
			wantErr: "text or post",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt := newUpdateMessageTestRuntime(t, tt.flags)
			err := ImMessagesUpdate.Validate(context.Background(), rt)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate() error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Validate() error = %v, want substring %q", err, tt.wantErr)
			}
		})
	}
}

func TestImMessagesUpdateExecute_PutsUpdateRequest(t *testing.T) {
	ctx := context.Background()
	cmd := newUpdateMessageTestCommand(t, map[string]string{
		"message-id": "om_123",
		"text":       "updated <at id=ou_1/>",
		"msg-type":   "text",
	})
	cfg := &core.CliConfig{AppID: "cli_test", AppSecret: "secret", Brand: core.BrandFeishu}
	f, _, _, reg := cmdutil.TestFactory(t, cfg)
	stub := &httpmock.Stub{
		Method: http.MethodPut,
		URL:    "/open-apis/im/v1/messages/om_123",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{
				"message_id":  "om_123",
				"chat_id":     "oc_123",
				"update_time": "1710000000000",
				"updated":     true,
			},
		},
	}
	reg.Register(stub)

	rt := common.TestNewRuntimeContextForAPI(ctx, cmd, cfg, f, core.AsBot)
	if err := ImMessagesUpdate.Execute(ctx, rt); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(stub.CapturedBody, &body); err != nil {
		t.Fatalf("request body invalid JSON: %v\n%s", err, string(stub.CapturedBody))
	}
	if body["msg_type"] != "text" {
		t.Fatalf("msg_type = %v, want text", body["msg_type"])
	}
	content, _ := body["content"].(string)
	if !strings.Contains(content, `<at user_id=\"ou_1\">`) {
		t.Fatalf("content = %q, want normalized mention", content)
	}
}

func TestImMessagesUpdateExecute_DoesNotRewriteRawContentJSON(t *testing.T) {
	ctx := context.Background()
	rawContent := `{"zh_cn":{"content":[[{"tag":"text","text":"keep <at id=ou_1/> literal"}]]}}`
	cmd := newUpdateMessageTestCommand(t, map[string]string{
		"message-id": "om_123",
		"content":    rawContent,
		"msg-type":   "post",
	})
	cfg := &core.CliConfig{AppID: "cli_test", AppSecret: "secret", Brand: core.BrandFeishu}
	f, _, _, reg := cmdutil.TestFactory(t, cfg)
	stub := &httpmock.Stub{
		Method: http.MethodPut,
		URL:    "/open-apis/im/v1/messages/om_123",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{
				"message_id": "om_123",
				"updated":    true,
			},
		},
	}
	reg.Register(stub)

	rt := common.TestNewRuntimeContextForAPI(ctx, cmd, cfg, f, core.AsBot)
	if err := ImMessagesUpdate.Execute(ctx, rt); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(stub.CapturedBody, &body); err != nil {
		t.Fatalf("request body invalid JSON: %v\n%s", err, string(stub.CapturedBody))
	}
	if body["content"] != rawContent {
		t.Fatalf("content = %v, want raw content unchanged", body["content"])
	}
}

func newUpdateMessageTestRuntime(t *testing.T, flags map[string]string) *common.RuntimeContext {
	t.Helper()
	return &common.RuntimeContext{Cmd: newUpdateMessageTestCommand(t, flags)}
}

func newUpdateMessageTestCommand(t *testing.T, flags map[string]string) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{Use: "test"}
	for _, flag := range []string{"message-id", "msg-type", "content", "text", "markdown"} {
		cmd.Flags().String(flag, "", "")
	}
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
