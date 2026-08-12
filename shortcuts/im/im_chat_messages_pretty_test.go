// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"bytes"
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestRenderChatMessagesPrettyConversation(t *testing.T) {
	root := map[string]interface{}{
		"message_id":  "om_root",
		"msg_type":    "post",
		"create_time": "2026-08-11 21:37",
		"sender":      map[string]interface{}{"name": "豆包"},
		"content":     "第一行\n\n```go\nfmt.Println(\"hello\")\n```",
		"thread_id":   "omt_topic",
		"reactions": map[string]interface{}{
			"counts": []interface{}{
				map[string]interface{}{"reaction_type": "THUMBSUP", "count": float64(2)},
				map[string]interface{}{"reaction_type": "DONE", "count": 1},
				map[string]interface{}{"reaction_type": "EMPTY", "count": 0},
			},
		},
		"thread_has_more": true,
	}
	reply := map[string]interface{}{
		"message_id":  "om_reply",
		"msg_type":    "text",
		"create_time": "2026-08-11 21:38",
		"sender":      map[string]interface{}{"name": "boe 的"},
		"content":     "Completed\n第二行",
		"reactions": map[string]interface{}{
			"counts": []map[string]interface{}{
				{"reaction_type": "SMILE", "count": 1},
			},
		},
	}
	crossDayRecalled := map[string]interface{}{
		"message_id":  "om_recalled",
		"msg_type":    "text",
		"create_time": "2026-08-12 00:03",
		"sender":      map[string]interface{}{"id": "ou_user"},
		"content":     "This message was recalled",
		"deleted":     true,
	}
	root["thread_replies"] = []interface{}{root, reply, reply, crossDayRecalled}

	ordinaryReply := map[string]interface{}{
		"message_id":  "om_plain_reply",
		"msg_type":    "text",
		"create_time": "2026-08-11 22:00",
		"sender":      map[string]interface{}{"sender_type": "anonymous"},
		"content":     "普通回复",
		"reply_to":    "om_parent",
	}

	var out bytes.Buffer
	renderChatMessagesPretty(&out, []map[string]interface{}{root, ordinaryReply}, true, "next-token")

	want := `2026-08-11 21:37 · 豆包
> 第一行\n\n` + "```go" + `\nfmt.Println("hello")\n` + "```" + `

message: om_root · thread: omt_topic
表情：THUMBSUP×2、DONE×1

  ↳ 21:38 · boe 的
    > Completed\n第二行
    表情：SMILE×1

  ↳ 2026-08-12 00:03 · ou_user
    > 已撤回

  ↳ 还有更多话题回复 · thread: omt_topic
────────────────────────

2026-08-11 22:00 · anonymous
> 普通回复

message: om_plain_reply · reply_to: om_parent
────────────────────────

2 messages · 2 thread replies · has_more: true · page_token: next-token
`
	if out.String() != want {
		t.Fatalf("pretty transcript mismatch\n--- got ---\n%s--- want ---\n%s", out.String(), want)
	}
}

func TestRenderChatMessagesPrettyThreadStatesAndFallbacks(t *testing.T) {
	messages := []map[string]interface{}{
		{
			"message_id":           "om_failed",
			"msg_type":             "interactive",
			"create_time":          "2026-08-11 20:00",
			"thread_id":            "omt_failed",
			"thread_replies_error": true,
		},
		{
			"message_id":  "om_omitted",
			"msg_type":    "system",
			"create_time": "2026-08-11 20:01",
			"thread_id":   "omt_omitted",
		},
		{
			"message_id":  "om_empty_text",
			"msg_type":    "text",
			"create_time": "2026-08-11 20:02",
		},
	}

	var out bytes.Buffer
	renderChatMessagesPretty(&out, messages, false, "")
	got := out.String()
	for _, want := range []string{
		"2026-08-11 20:00 · 未知发送者\n> [interactive]",
		"话题回复获取失败 · thread: omt_failed",
		"2026-08-11 20:01 · 系统\n> [system]",
		"话题回复未展开 · thread: omt_omitted",
		"2026-08-11 20:02 · 未知发送者\n> [空消息]",
		"3 messages · 0 thread replies · has_more: false · page_token: -",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("pretty output missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "> [text]") || strings.Contains(got, "> [post]") {
		t.Fatalf("ordinary message type leaked into pretty output:\n%s", got)
	}
}

func TestRenderChatMessagesPrettyEmpty(t *testing.T) {
	var out bytes.Buffer
	renderChatMessagesPretty(&out, nil, false, "")
	want := "No messages in this time range.\n\n0 messages · 0 thread replies · has_more: false · page_token: -\n"
	if out.String() != want {
		t.Fatalf("empty pretty output = %q, want %q", out.String(), want)
	}
}

func TestEscapePrettyContent(t *testing.T) {
	input := "line 1\r\nline 2\tC:\\tmp"
	want := `line 1\r\nline 2\tC:\\tmp`
	if got := escapePrettyContent(input); got != want {
		t.Fatalf("escapePrettyContent() = %q, want %q", got, want)
	}
}

func TestImChatMessageListExecuteUsesConversationPrettyRenderer(t *testing.T) {
	transport := shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != imMessagesListPath {
			t.Fatalf("unexpected request: %s", req.URL.String())
		}
		if req.URL.Query().Get("container_id_type") == "thread" {
			return shortcutJSONResponse(http.StatusOK, map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{
					"items": []interface{}{
						map[string]interface{}{
							"message_id": "om_root", "thread_id": "omt_topic", "msg_type": "text", "create_time": "root-time",
							"sender": map[string]interface{}{"id": "cli_root", "sender_type": "app", "sender_name": "豆包"},
							"body":   map[string]interface{}{"content": `{"text":"a long root message that must not be truncated after forty characters"}`},
						},
						map[string]interface{}{
							"message_id": "om_reply", "thread_id": "omt_topic", "msg_type": "post", "create_time": "reply-time",
							"sender": map[string]interface{}{"id": "cli_reply", "sender_type": "app", "sender_name": "boe 的"},
							"body":   map[string]interface{}{"content": `{"title":"","content_v2":[[{"tag":"md","text":"reply line 1\nreply line 2"}]]}`},
						},
					},
					"has_more": false,
				},
			}), nil
		}
		return shortcutJSONResponse(http.StatusOK, map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{
						"message_id": "om_root", "thread_id": "omt_topic", "msg_type": "text", "create_time": "root-time",
						"sender": map[string]interface{}{"id": "cli_root", "sender_type": "app", "sender_name": "豆包"},
						"body":   map[string]interface{}{"content": `{"text":"a long root message that must not be truncated after forty characters"}`},
					},
				},
				"has_more":   true,
				"page_token": "next-token",
			},
		}), nil
	})

	runtime := newBotShortcutRuntime(t, transport)
	runtime.Cmd = newListPageAllCommand(t, ImChatMessageList, map[string]string{
		"chat-id":      "oc_test",
		"page-size":    "1",
		"no-reactions": "true",
	})
	runtime.Format = "pretty"

	if err := ImChatMessageList.Execute(context.Background(), runtime); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	out, _ := runtime.IO().Out.(*bytes.Buffer)
	got := out.String()
	for _, want := range []string{
		"a long root message that must not be truncated after forty characters",
		"  ↳ reply-time · boe 的",
		"    > reply line 1\\nreply line 2",
		"1 messages · 1 thread replies · has_more: true · page_token: next-token",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("pretty Execute output missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "message: om_reply") || strings.Contains(got, "tip: use --format json") || strings.Contains(got, "Pagination:") {
		t.Fatalf("legacy table/footer leaked into conversation pretty output:\n%s", got)
	}
}
