// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/spf13/cobra"
)

func TestRenderMessagesConciseMultiChat(t *testing.T) {
	sections := []conciseChatSection{
		{
			ChatID:   "chat_1",
			ChatName: "project_group",
			ChatType: "group",
			Messages: []map[string]interface{}{
				{
					"message_id":       "message_1",
					"thread_id":        "thread_1",
					"msg_type":         "text",
					"create_time":      "2026-09-01 15:17",
					"content":          "message_1 content",
					"updated":          true,
					"message_app_link": "https://example.invalid/message_1",
					"sender": map[string]interface{}{
						"id": "user_1", "name": "user_1", "sender_type": "user",
					},
					"reactions": map[string]interface{}{
						"counts": []interface{}{
							map[string]interface{}{"reaction_type": "THUMBSUP", "count": float64(2)},
							map[string]interface{}{"reaction_type": "DONE", "count": float64(1)},
						},
					},
					"resources": []interface{}{
						map[string]interface{}{"local_path": "lark-im-resources/message_1.pdf"},
						map[string]interface{}{"error": true, "key": "hidden"},
					},
					"thread_replies": []map[string]interface{}{
						{"message_id": "message_1", "content": "duplicate root"},
						{
							"message_id": "thread_reply_1", "thread_id": "thread_1", "msg_type": "post",
							"create_time": "2026-09-01 15:18", "content": "reply line 1\nreply line 2",
							"sender": map[string]interface{}{
								"id": "user_2", "name": "user_2", "sender_type": "app", "open_bot_id": "user_2_bot",
							},
						},
					},
				},
				{
					"message_id": "message_2", "reply_to": "message_1", "msg_type": "image",
					"create_time": "2026-09-01 15:19", "content": "[image]", "thread_has_more": true,
					"sender":    map[string]interface{}{"id": "user_2", "name": "user_2", "sender_type": "app"},
					"resources": []map[string]interface{}{{"local_path": "lark-im-resources/message_2.png"}},
				},
			},
		},
		{
			ChatID: "chat_2", ChatType: "p2p", ChatPartnerID: "user_3",
			Messages: []map[string]interface{}{
				{
					"message_id": "message_3", "msg_type": "text", "create_time": "2026-09-01 16:01",
					"content": "message_3 line 1\nmessage_3 line 2", "reactions_error": true, "thread_replies_error": true,
					"sender": map[string]interface{}{"id": "user_3", "name": "user_3", "sender_type": "user"},
				},
				{
					"message_id": "message_4", "msg_type": "text", "create_time": "2026-09-01 16:02",
					"content": "must not render", "deleted": true,
				},
			},
		},
	}

	var out bytes.Buffer
	if err := renderMessagesConcise(&out, conciseMessageView{
		Type: conciseMessageViewChat, Title: "Chat messages",
		ChatSections: sections, HasMore: true, NextToken: "next_token_1",
	}); err != nil {
		t.Fatalf("renderMessagesConcise() error = %v", err)
	}

	const want = `# Chat messages

## Participants

- user\_1 (` + "`user_1`" + `, user)
- user\_2 (` + "`user_2`" + `, app, bot_open_id: ` + "`user_2_bot`" + `)
- user\_3 (` + "`user_3`" + `, user)

## Messages

### Chat: project\_group

- chat_id: ` + "`chat_1`" + `
- chat_type: ` + "`group`" + `

- ` + "`2026-09-01 15:17`" + ` · **user\_1** (` + "`user_1`" + `) · ` + "`text`" + ` · message_id: ` + "`message_1`" + ` · thread_id: ` + "`thread_1`" + ` · edited
  > message_1 content
  resources: ` + "`lark-im-resources/message_1.pdf`" + `
  resource_failures: 1
  reactions: ` + "`THUMBSUP x2`" + `, ` + "`DONE x1`" + `
  replies:
  - **Reply** · ` + "`2026-09-01 15:18`" + ` · **user\_2** (` + "`user_2`" + `) · ` + "`post`" + ` · message_id: ` + "`thread_reply_1`" + ` · thread_id: ` + "`thread_1`" + `
    > reply line 1
    > reply line 2

- ` + "`2026-09-01 15:19`" + ` · **user\_2** (` + "`user_2`" + `) · ` + "`image`" + ` · message_id: ` + "`message_2`" + ` · reply_to: ` + "`message_1`" + `
  > [image]
  resources: ` + "`lark-im-resources/message_2.png`" + `
  thread_has_more: true (thread replies incomplete)

### Chat: P2P

- chat_id: ` + "`chat_2`" + `
- chat_type: ` + "`p2p`" + `
- chat_partner: ` + "`user_3`" + `

- ` + "`2026-09-01 16:01`" + ` · **user\_3** (` + "`user_3`" + `) · ` + "`text`" + ` · message_id: ` + "`message_3`" + `
  > message_3 line 1
  > message_3 line 2
  reactions: unavailable
  thread_replies_error: true (thread replies unavailable)

- ` + "`2026-09-01 16:02`" + ` · **unknown_sender** · ` + "`text`" + ` · message_id: ` + "`message_4`" + ` · deleted
  > [deleted]

## Summary

- messages: 4
- thread_replies: 1
- threads: 1
- chats: 2
- has_more: true
- next_token: ` + "`next_token_1`" + `
`
	if got := out.String(); got != want {
		t.Fatalf("concise output mismatch\n--- got ---\n%s--- want ---\n%s", got, want)
	}
	for _, forbidden := range []string{"duplicate root", "must not render", "hidden", "example.invalid", "message_app_link"} {
		if strings.Contains(out.String(), forbidden) {
			t.Fatalf("concise output contains %q:\n%s", forbidden, out.String())
		}
	}
}

func TestRenderMessagesConciseAllSectionsEmpty(t *testing.T) {
	var out bytes.Buffer
	if err := renderMessagesConcise(&out, conciseMessageView{
		Type:         conciseMessageViewChat,
		ChatSections: []conciseChatSection{{ChatID: "chat_1"}, {ChatID: "chat_2"}},
	}); err != nil {
		t.Fatalf("renderMessagesConcise() error = %v", err)
	}
	if got := strings.Count(out.String(), "No messages found."); got != 1 {
		t.Fatalf("empty marker count = %d, want 1:\n%s", got, out.String())
	}
	if !strings.Contains(out.String(), "- messages: 0") {
		t.Fatalf("empty output missing summary:\n%s", out.String())
	}
}

func TestCollectConciseParticipantsDeduplicatesAcrossSections(t *testing.T) {
	sender := map[string]interface{}{"id": "user_1", "name": "user_1"}
	participants := collectConciseParticipants([]conciseChatSection{
		{Messages: []map[string]interface{}{{"message_id": "message_1", "sender": sender}}},
		{Messages: []map[string]interface{}{{"message_id": "message_2", "sender": sender, "mentions": []interface{}{map[string]interface{}{"id": "mentioned_only"}}}}},
	})
	if len(participants) != 1 || participants[0].ID != "user_1" {
		t.Fatalf("participants = %#v, want one cross-section sender", participants)
	}
}

func TestRenderMessagesConciseSingleSectionAndSafeMetadata(t *testing.T) {
	const forged = "\n## forged"
	var out bytes.Buffer
	err := renderMessagesConcise(&out, conciseMessageView{
		Type:  conciseMessageViewChat,
		Title: "Messages" + forged,
		ChatSections: []conciseChatSection{{
			ChatID: "chat`" + forged, ThreadID: "thread`" + forged,
			Messages: []map[string]interface{}{{
				"message_id": "message`" + forged, "content": "normal body",
				"sender": map[string]interface{}{"id": "user`" + forged, "name": "\x1b[31muser" + forged + "\u202e"},
			}},
		}},
		HasMore: false, NextToken: "must-not-render",
	})
	if err != nil {
		t.Fatalf("renderMessagesConcise() error = %v", err)
	}
	got := out.String()
	if strings.Contains(got, forged) || strings.Contains(got, "\x1b") || strings.Contains(got, "\u202e") {
		t.Fatalf("unsafe metadata was not sanitized: %q", got)
	}
	for _, want := range []string{"# Messages \\#\\# forged", "chat_id: ``chat` ## forged``", "thread_id: ``thread` ## forged``", "- has_more: false"} {
		if !strings.Contains(got, want) {
			t.Fatalf("concise output missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "next_token:") || strings.Contains(got, "must-not-render") {
		t.Fatalf("completed result exposed next token:\n%s", got)
	}
}

func TestMessageListConciseFlagIsCommandScoped(t *testing.T) {
	for _, shortcut := range []*common.Shortcut{&ImChatMessageList, &ImThreadsMessagesList} {
		runtime, _ := newMountedIMRuntime(t, shortcut)
		if runtime.Cmd.Flags().Lookup("concise") == nil {
			t.Fatalf("%s is missing --concise", shortcut.Command)
		}
		format := runtime.Cmd.Flags().Lookup("format")
		if format == nil || strings.Contains(format.Usage, "concise") {
			t.Fatalf("%s changed the generic format surface: %#v", shortcut.Command, format)
		}
	}

	runtime, _ := newMountedIMRuntime(t, &ImChatList)
	if runtime.Cmd.Flags().Lookup("concise") != nil {
		t.Fatalf("%s unexpectedly exposes --concise", ImChatList.Command)
	}
}

func TestMessageListConciseDryRunKeepsJSONPreview(t *testing.T) {
	tests := []struct {
		name     string
		shortcut common.Shortcut
		args     []string
	}{
		{name: "chat", shortcut: ImChatMessageList, args: []string{"--chat-id", "oc_test"}},
		{name: "thread", shortcut: ImThreadsMessagesList, args: []string{"--thread", "omt_test"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			factory, stdout, _, _ := cmdutil.TestFactory(t, &core.CliConfig{})
			parent := &cobra.Command{Use: "root", SilenceErrors: true, SilenceUsage: true}
			test.shortcut.Mount(parent, factory)
			parent.SetArgs(append([]string{test.shortcut.Command}, append(test.args, "--dry-run", "--concise", "--no-reactions")...))

			if err := parent.Execute(); err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			var envelope map[string]interface{}
			if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
				t.Fatalf("dry-run stdout is not JSON: %v\n%s", err, stdout.String())
			}
			if envelope["ok"] != true || envelope["dry_run"] != true {
				t.Fatalf("dry-run envelope = %#v", envelope)
			}
			if strings.Contains(stdout.String(), "# Chat messages") || strings.Contains(stdout.String(), "# Thread messages") {
				t.Fatalf("dry-run entered concise renderer:\n%s", stdout.String())
			}
		})
	}
}
