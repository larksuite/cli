// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/larksuite/cli/internal/core"
)

func TestMessagesSearchChatGetFallback(t *testing.T) {
	for _, tc := range []struct {
		name, mode, env, wantTitle string
		batchOK, getFails          bool
		wantGets                   int
	}{
		{name: "group", mode: "group", env: "1", wantTitle: "Group", wantGets: 1},
		{name: "topic", mode: "topic", env: "1", wantTitle: "Group", wantGets: 1},
		{name: "batch-success", mode: "group", env: "1", batchOK: true, wantTitle: "Batch Group"},
		{name: "get-failure", mode: "group", env: "1", getFails: true, wantGets: 1},
		{name: "disabled", mode: "group", env: "0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("LARKSUITE_CLI_CITATION", tc.env)
			gets := 0
			rt := newMessagesSearchRuntime(t, map[string]string{"query": "test"}, nil, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				var data map[string]interface{}
				switch req.URL.Path {
				case "/open-apis/im/v1/messages/search":
					data = map[string]interface{}{"items": []interface{}{
						map[string]interface{}{"meta_data": map[string]interface{}{"message_id": "om_1", "chat_id": "oc_test", "is_p2p_chat": false}},
						map[string]interface{}{"meta_data": map[string]interface{}{"message_id": "om_2", "chat_id": "oc_test", "is_p2p_chat": false}},
					}}
				case "/open-apis/im/v1/messages/mget":
					var items []interface{}
					for _, id := range []string{"om_1", "om_2"} {
						items = append(items, map[string]interface{}{"message_id": id, "chat_id": "oc_test", "msg_type": "image", "body": map[string]interface{}{"content": `{"image_key":"img_test"}`}})
					}
					data = map[string]interface{}{"items": items}
				case "/open-apis/im/v1/chats/batch_query":
					if !tc.batchOK {
						return shortcutJSONResponse(400, map[string]interface{}{"code": 99991663, "msg": "invalid token type"}), nil
					}
					data = map[string]interface{}{"items": []interface{}{map[string]interface{}{"chat_id": "oc_test", "chat_mode": tc.mode, "name": "Batch Group"}}}
				case "/open-apis/im/v1/chats/oc_test":
					gets++
					if req.Method != http.MethodGet || req.URL.Query().Get("user_id_type") != "open_id" {
						t.Fatalf("bad fallback request: %s %s", req.Method, req.URL)
					}
					if tc.getFails {
						return shortcutJSONResponse(403, map[string]interface{}{"code": 230002, "msg": "denied"}), nil
					}
					// chats.get's chat_type is privacy, not group versus P2P.
					data = map[string]interface{}{"chat_mode": tc.mode, "chat_type": "private", "name": "Group"}
				default:
					t.Fatalf("unexpected API: %s", req.URL)
				}
				return shortcutJSONResponse(200, map[string]interface{}{"code": 0, "data": data}), nil
			}))
			isolateCitationTestFactory(t, rt)
			setRuntimeField(t, rt, "resolvedAs", core.AsBot)
			rt.Cmd.Flags().Bool("no-reactions", true, "")
			rt.Format = "json"
			if err := ImMessagesSearch.Execute(context.Background(), rt); err != nil {
				t.Fatal(err)
			}
			var out struct {
				Data map[string]interface{} `json:"data"`
			}
			if err := json.Unmarshal(rt.IO().Out.(*bytes.Buffer).Bytes(), &out); err != nil {
				t.Fatal(err)
			}
			messages := citationItems(out.Data, "messages")
			if len(messages) != 2 || gets != tc.wantGets {
				t.Fatalf("messages=%d, gets=%d, want=%d", len(messages), gets, tc.wantGets)
			}
			if tc.env == "1" {
				for _, c := range messagesSearchCitations(rt, out.Data) {
					if c.Title != tc.wantTitle {
						t.Fatalf("title=%q, want=%q", c.Title, tc.wantTitle)
					}
				}
			} else if _, ok := messages[0]["chat_name"]; ok {
				t.Fatal("citation disabled must not add a title")
			}
		})
	}
}

func TestFillMissingMessageChatContexts(t *testing.T) {
	calls := map[string]int{}
	rt := newUserShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls[req.URL.Path]++
		return shortcutJSONResponse(200, map[string]interface{}{"code": 0, "data": map[string]interface{}{"chat_mode": "p2p", "owner_id": "ou_owner"}}), nil
	}))
	isolateCitationTestFactory(t, rt)
	chats := map[string]messageChatContext{"oc_existing": {kind: "group", name: "Existing"}}
	fillMissingMessageChatContexts(rt, chats, []string{"", "oc_existing", "oc_missing", "oc_missing"})
	if len(calls) != 1 || calls["/open-apis/im/v1/chats/oc_missing"] != 1 || chats["oc_existing"].name != "Existing" || chats["oc_missing"].kind != "p2p" || chats["oc_missing"].partnerID != "" {
		t.Fatalf("calls=%v, chats=%v", calls, chats)
	}
}
