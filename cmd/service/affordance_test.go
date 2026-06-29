// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package service

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/meta"
)

func TestRenderAffordance(t *testing.T) {
	raw := json.RawMessage(`{
		"use_when": ["发送文本消息"],
		"do_not_use_when": ["群已解散"],
		"prerequisites": ["已获取 chat_id"],
		"examples": [
			{"description":"发一条文本","command":"lark-cli im messages create --params '{...}'"},
			{"command":"lark-cli im messages list"},
			{"description":"no command, skipped","command":""}
		],
		"related": ["im.messages.list"]
	}`)
	out := renderAffordance(meta.Method{Affordance: raw})
	for _, want := range []string{
		"When to use:", "发送文本消息",
		"Avoid when:", "群已解散",
		"Prerequisites:", "已获取 chat_id",
		"Examples:", "发一条文本", "lark-cli im messages create --params '{...}'",
		"lark-cli im messages list", // example with no description -> bare command line
		"Related:", "im.messages.list",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("renderAffordance missing %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "no command, skipped") {
		t.Errorf("example with empty command should be skipped:\n%s", out)
	}

	// Absent or empty affordance renders nothing (so methods without an overlay
	// add nothing to their help).
	if renderAffordance(meta.Method{}) != "" || renderAffordance(meta.Method{Affordance: json.RawMessage(`{}`)}) != "" {
		t.Error("empty affordance should render nothing")
	}
}

func TestServiceMethod_AffordanceInLong(t *testing.T) {
	withAff := map[string]interface{}{
		"path": "messages", "httpMethod": "POST", "description": "发送消息",
		"affordance": map[string]interface{}{
			"examples": []interface{}{
				map[string]interface{}{"description": "发文本", "command": "lark-cli im messages create ..."},
			},
		},
	}
	f, _, _, _ := cmdutil.TestFactory(t, testConfig)
	cmd := NewCmdServiceMethod(f, imSpec(), meta.FromMap(withAff), "create", "messages", nil)
	if !strings.Contains(cmd.Long, "Examples:") || !strings.Contains(cmd.Long, "lark-cli im messages create ...") {
		t.Errorf("affordance examples not in command Long:\n%s", cmd.Long)
	}

	// A method with no affordance adds no guidance block.
	plain := map[string]interface{}{"path": "x", "httpMethod": "GET", "description": "d"}
	cmd2 := NewCmdServiceMethod(f, imSpec(), meta.FromMap(plain), "list", "x", nil)
	if strings.Contains(cmd2.Long, "Examples:") {
		t.Errorf("no-affordance method should have no Examples in Long:\n%s", cmd2.Long)
	}
}
