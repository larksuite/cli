// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"bytes"
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/tidwall/gjson"
)

func TestMessagesMGetCommandPreservesSyncToChatRelationInJSONOutput(t *testing.T) {
	runtime := newBotShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if !strings.Contains(req.URL.Path, "/open-apis/im/v1/messages/mget") {
			t.Fatalf("unexpected request: %s", req.URL.String())
		}
		return shortcutJSONResponse(http.StatusOK, map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"items": []interface{}{map[string]interface{}{
					"message_id": "om_target",
					"msg_type":   "text",
					"body":       map[string]interface{}{"content": `{"text":"hello"}`},
					"sync_to_chat_info": map[string]interface{}{
						"type":               1,
						"thread_id":          "omt_source",
						"related_message_id": "om_source",
						"future_private":     "must-not-leak",
					},
				}},
			},
		}), nil
	}))

	root := &cobra.Command{Use: "root"}
	ImMessagesMGet.Mount(root, runtime.Factory)
	root.SetArgs([]string{"+messages-mget", "--message-ids", "om_target", "--no-reactions", "--format", "json"})
	if err := root.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute +messages-mget: %v", err)
	}

	out := runtime.Factory.IOStreams.Out.(*bytes.Buffer)
	result := out.String()
	if got := gjson.Get(result, "data.messages.0.sync_to_chat_info.type").Int(); got != 1 {
		t.Fatalf("relation type = %d, stdout: %s", got, result)
	}
	if got := gjson.Get(result, "data.messages.0.sync_to_chat_info.thread_id").String(); got != "omt_source" {
		t.Fatalf("relation thread_id = %q, stdout: %s", got, result)
	}
	if got := gjson.Get(result, "data.messages.0.sync_to_chat_info.related_message_id").String(); got != "om_source" {
		t.Fatalf("related_message_id = %q, stdout: %s", got, result)
	}
	if gjson.Get(result, "data.messages.0.sync_to_chat_info.future_private").Exists() {
		t.Fatalf("unknown relation field leaked, stdout: %s", result)
	}
}
