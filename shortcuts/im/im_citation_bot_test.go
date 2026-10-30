// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/shortcuts/common"
)

const citationBotPath = "/open-apis/bot/v3/bots/basic_batch"
const citationBotScope = "application:bot.basic_info:read"

func TestCitationBotFallbackCommands(t *testing.T) {
	for _, command := range []string{"list", "search", "mget"} {
		for _, tc := range []struct {
			name, env, cachedName, scope, wantTitle string
			fail, empty                             bool
			chatDenied, peerMissing, callerBot      bool
			membersFail                             bool
			senderPeer                              bool
			wantCalls                               int
		}{
			{name: "missing-name", env: "1", scope: citationBotScope, wantTitle: "A和Assistant的会话", wantCalls: 1},
			{name: "sender-name-wins", env: "1", cachedName: "Sender Name", scope: "im:chat:read", wantTitle: "A和Sender Name的会话"},
			{name: "disabled", scope: citationBotScope},
			{name: "not-enabled-by-true", env: "true", scope: citationBotScope},
			{name: "missing-scope", env: "1", scope: "im:chat:read", wantTitle: "来自单聊"},
			{name: "api-denied", env: "1", scope: citationBotScope, fail: true, wantCalls: 1, wantTitle: "来自单聊"},
			{name: "missing-peer-id", env: "1", peerMissing: true, wantTitle: "来自单聊"},
			{name: "user-chat-denied", env: "1", chatDenied: true, wantTitle: "来自单聊"},
			{name: "bot-chat-denied", env: "1", callerBot: true, chatDenied: true, wantTitle: "A和Peer的会话"},
			{name: "bot-sender-peer", env: "1", callerBot: true, chatDenied: true, senderPeer: true, wantTitle: "A和Peer的会话"},
			{name: "bot-sender-peer-disabled", callerBot: true, chatDenied: true, senderPeer: true},
			{name: "bot-members-denied", env: "1", callerBot: true, chatDenied: true, membersFail: true, wantTitle: "来自单聊"},
			{name: "bot-chat-denied-disabled", callerBot: true, chatDenied: true},
			{name: "empty-results", env: "1", scope: citationBotScope, empty: true},
		} {
			t.Run(command+"/"+tc.name, func(t *testing.T) {
				t.Setenv("LARKSUITE_CLI_CITATION", tc.env)
				calls, chatBatchCalls, chatGetCalls, memberCalls := 0, 0, 0, 0
				transport := shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
					var data map[string]interface{}
					message := map[string]interface{}{
						"message_id": "om_test", "chat_id": "oc_test", "msg_type": "image",
						"body":      map[string]interface{}{"content": `{"image_key":"img_test"}`},
						"chat_type": "p2p", "chat_default_name": "",
						"sender": map[string]interface{}{"id": "ou_self", "sender_type": "user", "sender_name": "A"},
					}
					if tc.cachedName != "" {
						message["sender"] = map[string]interface{}{"id": "cli_peer", "sender_type": "app", "open_bot_id": "ou_peer", "sender_name": tc.cachedName}
					}
					switch req.URL.Path {
					case imMessagesListPath, "/open-apis/im/v1/messages/mget":
						items := []interface{}{message}
						if tc.senderPeer {
							message["sender"] = map[string]interface{}{"id": "cli_self", "sender_type": "app", "open_bot_id": "ou_self", "sender_name": "A"}
							items = append(items, map[string]interface{}{
								"message_id": "om_peer", "chat_id": "oc_test", "msg_type": "text",
								"body":   map[string]interface{}{"content": `{"text":"hello"}`},
								"sender": map[string]interface{}{"id": "ou_human", "sender_type": "user", "sender_name": "Peer"},
							})
						}
						if tc.empty {
							items = nil
						}
						data = map[string]interface{}{"items": items, "has_more": false}
					case "/open-apis/im/v1/messages/search":
						items := []interface{}{map[string]interface{}{"meta_data": map[string]interface{}{"message_id": "om_test", "chat_id": "oc_test", "is_p2p_chat": true}}}
						if tc.senderPeer {
							items = append(items, map[string]interface{}{"meta_data": map[string]interface{}{"message_id": "om_peer", "chat_id": "oc_test", "is_p2p_chat": true}})
						}
						if tc.empty {
							items = nil
						}
						data = map[string]interface{}{"items": items}
					case "/open-apis/im/v1/chats/batch_query":
						chatBatchCalls++
						if tc.chatDenied {
							return shortcutJSONResponse(400, map[string]interface{}{"code": 99991663, "msg": "invalid token type"}), nil
						}
						peerID := "ou_peer"
						if tc.peerMissing {
							peerID = ""
						}
						data = map[string]interface{}{"items": []interface{}{map[string]interface{}{"chat_id": "oc_test", "chat_mode": "p2p", "p2p_target_id": peerID, "p2p_target_type": "bot"}}}
					case "/open-apis/im/v1/chats/oc_test":
						chatGetCalls++
						if !tc.chatDenied || tc.env != "1" || req.Method != http.MethodGet || req.URL.Query().Get("user_id_type") != "open_id" {
							t.Fatalf("unexpected chat fallback: %s", req.URL)
						}
						data = map[string]interface{}{"chat_mode": "p2p", "owner_id": "ou_owner"}
					case "/open-apis/im/v1/chats/oc_test/members":
						memberCalls++
						if !tc.callerBot || tc.env != "1" || req.Method != http.MethodGet || req.URL.Query().Get("member_id_type") != "open_id" || req.URL.Query().Get("page_size") != "50" {
							t.Fatalf("unexpected member lookup: %s", req.URL)
						}
						if tc.membersFail {
							return shortcutJSONResponse(403, map[string]interface{}{"code": 99991672, "msg": "denied"}), nil
						}
						data = map[string]interface{}{"items": []interface{}{map[string]interface{}{"member_id": "ou_human", "name": "Peer"}}, "has_more": false}
					case citationBotPath:
						calls++
						if req.Method != http.MethodGet || !reflect.DeepEqual(req.URL.Query()["bot_ids"], []string{"ou_peer"}) {
							t.Fatalf("request = %s %s", req.Method, req.URL)
						}
						if tc.fail {
							return shortcutJSONResponse(403, map[string]interface{}{"code": 99991672, "msg": "denied"}), nil
						}
						data = map[string]interface{}{"bots": map[string]interface{}{"ou_peer": map[string]interface{}{"name": "Assistant"}}}
					default:
						t.Fatalf("unexpected request (bots must not use contact): %s", req.URL)
					}
					return shortcutJSONResponse(200, map[string]interface{}{"code": 0, "data": data}), nil
				})
				rt := newMessagesSearchRuntime(t, map[string]string{"query": "test"}, nil, transport)
				isolateCitationTestFactory(t, rt)
				setRuntimeField(t, rt, "resolvedAs", core.AsUser)
				if tc.callerBot {
					setRuntimeField(t, rt, "resolvedAs", core.AsBot)
					setRuntimeField(t, rt, "botInfoFunc", func() (*common.BotInfo, error) {
						return &common.BotInfo{OpenID: "ou_self", AppName: "A"}, nil
					})
				}
				setRuntimeScopes(t, rt, tc.scope)
				rt.Config.UserOpenId, rt.Config.UserName = "ou_self", "A"
				rt.Cmd.Flags().Bool("no-reactions", true, "")
				rt.Format = "json"
				shortcut := ImMessagesSearch
				if command == "list" {
					shortcut = ImChatMessageList
					rt.Cmd.Flags().String("chat-id", "oc_test", "")
					rt.Cmd.Flags().String("order", "desc", "")
					rt.Cmd.Flags().Int("page-delay", 0, "")
				}
				if command == "mget" {
					shortcut = ImMessagesMGet
					rt.Cmd.Flags().String("message-ids", "om_test", "")
				}
				if err := shortcut.Execute(context.Background(), rt); err != nil {
					t.Fatal(err)
				}
				if calls != tc.wantCalls {
					t.Fatalf("bot requests = %d, want %d", calls, tc.wantCalls)
				}
				wantChatBatches := 0
				if !tc.empty && (command == "search" || (!tc.callerBot && tc.env == "1")) {
					wantChatBatches = 1
				}
				if chatBatchCalls != wantChatBatches {
					t.Fatalf("chat batch requests = %d, want %d", chatBatchCalls, wantChatBatches)
				}
				wantChatGets := 0
				if command == "search" && tc.chatDenied && tc.env == "1" {
					wantChatGets = 1
				}
				if chatGetCalls != wantChatGets {
					t.Fatalf("chat get requests = %d, want %d", chatGetCalls, wantChatGets)
				}
				wantMemberCalls := 0
				if tc.callerBot && tc.env == "1" && !tc.senderPeer {
					wantMemberCalls = 1
				}
				if memberCalls != wantMemberCalls {
					t.Fatalf("member requests = %d, want %d", memberCalls, wantMemberCalls)
				}
				var out struct {
					Data map[string]interface{} `json:"data"`
				}
				if err := json.Unmarshal(rt.IO().Out.(*bytes.Buffer).Bytes(), &out); err != nil {
					t.Fatal(err)
				}
				messages := citationItems(out.Data, "messages")
				if tc.empty {
					if len(messages) != 0 {
						t.Fatal("expected no messages")
					}
					return
				}
				// Conversion output keeps non-text messages even on lookup failure.
				wantMessages := 1
				if tc.senderPeer {
					wantMessages = 2
				}
				if len(messages) != wantMessages || messages[0]["message_id"] != "om_test" || messages[0]["msg_type"] != "image" {
					t.Fatalf("message lost: %#v", out.Data)
				}
				if tc.env == "1" {
					cs := messagesSearchCitations(rt, out.Data)
					if command == "list" {
						cs = chatMessagesListCitations(rt, out.Data)
					}
					if command == "mget" {
						cs = messagesMGetCitations(rt, out.Data)
					}
					if len(cs) != wantMessages || cs[0].Title != tc.wantTitle {
						t.Fatalf("citations = %#v, want title %q", cs, tc.wantTitle)
					}
				}
				if tc.fail || tc.name == "missing-scope" {
					if !strings.Contains(rt.IO().ErrOut.(*bytes.Buffer).String(), "warning:") {
						t.Fatal("missing stderr warning")
					}
				}
			})
		}
	}
}

func TestCitationBotFallbackContinuesAfterBatchFailure(t *testing.T) {
	calls := 0
	rt := newUserShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return shortcutJSONResponse(200, map[string]interface{}{"code": 20006, "msg": "internal error"}), nil
		}
		if got := req.URL.Query()["bot_ids"]; !reflect.DeepEqual(got, []string{"ou_10"}) {
			t.Fatalf("second batch = %v", got)
		}
		return shortcutJSONResponse(200, map[string]interface{}{"code": 0, "data": map[string]interface{}{
			"bots": map[string]interface{}{"ou_10": map[string]interface{}{"name": "Last Bot"}},
		}}), nil
	}))
	isolateCitationTestFactory(t, rt)
	setRuntimeScopes(t, rt, citationBotScope)
	var ids []string
	for i := 0; i < 11; i++ {
		ids = append(ids, fmt.Sprintf("ou_%02d", i))
	}
	names := make(map[string]string)
	fillCitationBotNames(rt, ids, names)
	if calls != 2 || len(names) != 1 || names["ou_10"] != "Last Bot" {
		t.Fatalf("calls = %d, names = %#v", calls, names)
	}
	if !strings.Contains(rt.IO().ErrOut.(*bytes.Buffer).String(), "warning:") {
		t.Fatal("missing batch failure warning")
	}
}

func TestCitationBotFallbackBatchingAndIdentity(t *testing.T) {
	for _, identity := range []core.Identity{core.AsUser, core.AsBot} {
		t.Run(string(identity), func(t *testing.T) {
			var batches [][]string
			rt := newUserShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				ids := req.URL.Query()["bot_ids"]
				if req.Method != http.MethodGet || req.URL.Path != citationBotPath || len(ids) > 10 || len(ids) == 0 || req.ContentLength > 0 {
					t.Fatalf("invalid request: %s %s", req.Method, req.URL)
				}
				batches = append(batches, append([]string(nil), ids...))
				bots := make(map[string]interface{})
				for _, id := range ids {
					bots[id] = map[string]interface{}{"name": "Bot " + id}
				}
				return shortcutJSONResponse(200, map[string]interface{}{"code": 0, "data": map[string]interface{}{"bots": bots}}), nil
			}))
			isolateCitationTestFactory(t, rt)
			setRuntimeField(t, rt, "resolvedAs", identity)
			setRuntimeScopes(t, rt, citationBotScope)
			rt.Config.UserOpenId, rt.Config.UserName = "ou_self", "A"
			setRuntimeField(t, rt, "botInfoFunc", func() (*common.BotInfo, error) { return &common.BotInfo{OpenID: "ou_self", AppName: "A"}, nil })
			chats := map[string]messageChatContext{"group": {kind: "group", name: "Group"}}
			var wantIDs []string
			for i := 0; i < 25; i++ {
				id := fmt.Sprintf("ou_bot_%02d", i)
				wantIDs = append(wantIDs, id)
				chats[id] = messageChatContext{kind: "p2p", partnerID: id, partnerType: "bot"}
				chats[id+"_duplicate"] = chats[id]
			}
			got := messageChatTitles(rt, chats, nil)
			if !reflect.DeepEqual(batches, [][]string{wantIDs[:10], wantIDs[10:20], wantIDs[20:]}) {
				t.Fatalf("batches = %#v", batches)
			}
			for _, id := range wantIDs {
				want := "A和Bot " + id + "的会话"
				if got[id] != want || got[id+"_duplicate"] != want {
					t.Fatalf("title for %s = %q", id, got[id])
				}
			}
			if got["group"] != "Group" {
				t.Fatal("group title changed")
			}
		})
	}
}

func TestCitationBotFallbackFailures(t *testing.T) {
	for _, tc := range []struct {
		name           string
		data           map[string]interface{}
		transportError bool
	}{
		{name: "partial", data: map[string]interface{}{
			"bots":        map[string]interface{}{"ou_a": map[string]interface{}{"name": "Assistant"}},
			"failed_bots": map[string]interface{}{"ou_b": map[string]interface{}{"code": 20003, "reason": "cross tenant denied"}},
		}},
		{name: "not-found", data: map[string]interface{}{"failed_bots": map[string]interface{}{"ou_a": map[string]interface{}{"code": 20002}}}},
		{name: "invalid-id", data: map[string]interface{}{"failed_bots": map[string]interface{}{"ou_a": map[string]interface{}{"code": 20004}}}},
		{name: "empty-name", data: map[string]interface{}{"bots": map[string]interface{}{"ou_a": map[string]interface{}{"name": "", "i18n_names": map[string]interface{}{"en_us": "Not a fallback"}}}}},
		{name: "malformed", data: map[string]interface{}{"bots": "invalid"}},
		{name: "network", transportError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			rt := newUserShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				calls++
				if tc.transportError {
					return nil, fmt.Errorf("test transport failure")
				}
				return shortcutJSONResponse(200, map[string]interface{}{"code": 0, "data": tc.data}), nil
			}))
			isolateCitationTestFactory(t, rt)
			setRuntimeScopes(t, rt, citationBotScope)
			names := map[string]string{"ou_existing": "Existing"}
			fillCitationBotNames(rt, []string{"ou_a", "ou_b"}, names)
			if names["ou_existing"] != "Existing" || names["ou_b"] != "" || (tc.name == "partial" && names["ou_a"] != "Assistant") || (tc.name != "partial" && names["ou_a"] != "") {
				t.Fatalf("names = %#v", names)
			}
			if calls == 0 || !strings.Contains(rt.IO().ErrOut.(*bytes.Buffer).String(), "warning:") {
				t.Fatal("failure must warn without returning an error")
			}
		})
	}
}
