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

func TestImMessagesCardUpdateValidate(t *testing.T) {
	tests := []struct {
		name    string
		flags   map[string]string
		wantErr string
	}{
		{
			name: "accepts card content",
			flags: map[string]string{
				"message-id": "om_123",
				"content":    `{"config":{"update_multi":true},"elements":[{"tag":"div","text":{"tag":"plain_text","content":"updated"}}]}`,
			},
		},
		{
			name: "rejects thread id",
			flags: map[string]string{
				"message-id": "omt_123",
				"content":    `{"config":{"update_multi":true}}`,
			},
			wantErr: "must start with om_",
		},
		{
			name: "rejects missing content",
			flags: map[string]string{
				"message-id": "om_123",
			},
			wantErr: "--content is required",
		},
		{
			name: "rejects invalid json",
			flags: map[string]string{
				"message-id": "om_123",
				"content":    `{"config":`,
			},
			wantErr: "not valid JSON",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt := newCardUpdateTestRuntime(t, tt.flags)
			err := ImMessagesCardUpdate.Validate(context.Background(), rt)
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

func TestImMessagesCardUpdateExecute_PatchesCardRequest(t *testing.T) {
	ctx := context.Background()
	content := `{"config":{"update_multi":true},"elements":[{"tag":"div","text":{"tag":"plain_text","content":"updated"}}]}`
	cmd := newCardUpdateTestCommand(t, map[string]string{
		"message-id": "om_123",
		"content":    content,
	})
	cfg := &core.CliConfig{AppID: "cli_test", AppSecret: "secret", Brand: core.BrandFeishu}
	f, _, _, reg := cmdutil.TestFactory(t, cfg)
	stub := &httpmock.Stub{
		Method: http.MethodPatch,
		URL:    "/open-apis/im/v1/messages/om_123",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{},
		},
	}
	reg.Register(stub)

	rt := common.TestNewRuntimeContextForAPI(ctx, cmd, cfg, f, core.AsBot)
	if err := ImMessagesCardUpdate.Execute(ctx, rt); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(stub.CapturedBody, &body); err != nil {
		t.Fatalf("request body invalid JSON: %v\n%s", err, string(stub.CapturedBody))
	}
	if body["content"] != content {
		t.Fatalf("content = %v, want %s", body["content"], content)
	}
}

func newCardUpdateTestRuntime(t *testing.T, flags map[string]string) *common.RuntimeContext {
	t.Helper()
	return &common.RuntimeContext{Cmd: newCardUpdateTestCommand(t, flags)}
}

func newCardUpdateTestCommand(t *testing.T, flags map[string]string) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{Use: "test"}
	for _, flag := range []string{"message-id", "content"} {
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
