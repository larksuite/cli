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
	if got := gjson.Get(result, "data.messages.0.synced_from_thread_reply").String(); got != "om_source" {
		t.Fatalf("synced_from_thread_reply = %q, stdout: %s", got, result)
	}
	if got := gjson.Get(result, "data.messages.0.synced_from_thread").String(); got != "omt_source" {
		t.Fatalf("synced_from_thread = %q, stdout: %s", got, result)
	}
	if gjson.Get(result, "data.messages.0.synced_to_chat_message").Exists() {
		t.Fatalf("synced_to_chat_message set on the chat copy, stdout: %s", result)
	}
	// The nested upstream object and its unknown fields must not reach stdout.
	if gjson.Get(result, "data.messages.0.sync_to_chat_info").Exists() {
		t.Fatalf("nested sync_to_chat_info reached stdout: %s", result)
	}
	if strings.Contains(result, "future_private") {
		t.Fatalf("unknown relation field leaked, stdout: %s", result)
	}
}

// The thread-reply side goes through the same Cobra boundary: assert it emits
// synced_to_chat_message and nothing from the copy side.
func TestMessagesMGetCommandEmitsThreadReplySideOfRelation(t *testing.T) {
	runtime := newBotShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if !strings.Contains(req.URL.Path, "/open-apis/im/v1/messages/mget") {
			t.Fatalf("unexpected request: %s", req.URL.String())
		}
		return shortcutJSONResponse(http.StatusOK, map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"items": []interface{}{map[string]interface{}{
					"message_id": "om_reply",
					"msg_type":   "text",
					"body":       map[string]interface{}{"content": `{"text":"hello"}`},
					"sync_to_chat_info": map[string]interface{}{
						"type":               2,
						"related_message_id": "om_copy",
					},
				}},
			},
		}), nil
	}))

	root := &cobra.Command{Use: "root"}
	ImMessagesMGet.Mount(root, runtime.Factory)
	root.SetArgs([]string{"+messages-mget", "--message-ids", "om_reply", "--no-reactions", "--format", "json"})
	if err := root.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute +messages-mget: %v", err)
	}

	result := runtime.Factory.IOStreams.Out.(*bytes.Buffer).String()
	if got := gjson.Get(result, "data.messages.0.synced_to_chat_message").String(); got != "om_copy" {
		t.Fatalf("synced_to_chat_message = %q, stdout: %s", got, result)
	}
	for _, key := range []string{"synced_from_thread_reply", "synced_from_thread"} {
		if gjson.Get(result, "data.messages.0."+key).Exists() {
			t.Fatalf("%s set on the thread reply, stdout: %s", key, result)
		}
	}
}
