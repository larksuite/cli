// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/spf13/cobra"
)

// Reuse existing request fixtures but isolate all remaining factory state.
func isolateCitationTestFactory(t *testing.T, rt *common.RuntimeContext) {
	t.Helper()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	previous := rt.Factory
	factory, _, _, _ := cmdutil.TestFactory(t, rt.Config)
	factory.HttpClient, factory.LarkClient = previous.HttpClient, previous.LarkClient
	rt.Factory = factory
}

func TestChatMessageListCitationRequestAndOutput(t *testing.T) {
	for _, format := range []string{"json", "table", "csv", "ndjson"} {
		for _, tc := range []struct {
			enabled           bool
			kind, name, title string
		}{
			{false, "group", "Group", "Group"}, {true, "group", "Group", "Group"},
			{false, "p2p", "Stored name", "A和B的会话"}, {true, "p2p", "Stored name", "A和B的会话"},
			{false, "p2p", "", "A和B的会话"}, {true, "p2p", "", "A和B的会话"},
		} {
			enabled := tc.enabled
			t.Run(fmt.Sprintf("%s/%s/%v", format, tc.kind, enabled), func(t *testing.T) {
				t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
				t.Setenv("LARKSUITE_CLI_CITATION", map[bool]string{true: "1", false: ""}[enabled])
				calls := 0
				rt := newUserShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
					calls++
					if enabled && tc.kind == "p2p" {
						switch req.URL.Path {
						case "/open-apis/im/v1/chats/batch_query":
							return shortcutJSONResponse(200, map[string]interface{}{"code": 0, "data": map[string]interface{}{"items": []interface{}{map[string]interface{}{"chat_id": "oc_test", "chat_mode": "p2p", "p2p_target_id": "ou_peer", "p2p_target_type": "user"}}}}), nil
						case "/open-apis/contact/v3/users/basic_batch":
							return shortcutJSONResponse(200, map[string]interface{}{"code": 0, "data": map[string]interface{}{"users": []interface{}{map[string]interface{}{"user_id": "ou_peer", "name": "B"}}}}), nil
						}
					}
					if req.URL.Path != imMessagesListPath || req.Method != http.MethodGet {
						t.Fatalf("extra request: %s", req.URL)
					}
					if req.URL.Query().Has("with_chat_info") {
						t.Fatal("chat metadata must not require a query parameter")
					}
					data := map[string]interface{}{"items": []interface{}{map[string]interface{}{
						"message_id": "om_test", "chat_id": "oc_test", "msg_type": "image", "body": map[string]interface{}{"content": `{"image_key":"img_test"}`},
						"chat_type": tc.kind, "chat_default_name": tc.name,
					}}, "has_more": false}
					return shortcutJSONResponse(200, map[string]interface{}{"code": 0, "data": data}), nil
				}))
				rt.Cmd = &cobra.Command{Use: "test"}
				isolateCitationTestFactory(t, rt)
				setRuntimeField(t, rt, "resolvedAs", core.AsUser)
				rt.Config.UserOpenId, rt.Config.UserName = "ou_self", "A"
				rt.Cmd.Flags().String("chat-id", "oc_test", "")
				rt.Cmd.Flags().String("page-size", "20", "")
				rt.Cmd.Flags().String("order", "desc", "")
				rt.Cmd.Flags().Bool("no-reactions", true, "")
				rt.Cmd.Flags().Bool("page-all", false, "")
				rt.Cmd.Flags().Int("page-limit", 10, "")
				rt.Cmd.Flags().Int("page-delay", 0, "")
				rt.Format = format
				if err := ImChatMessageList.Execute(context.Background(), rt); err != nil {
					t.Fatal(err)
				}
				wantCalls := 1
				if enabled && tc.kind == "p2p" {
					wantCalls = 3
				}
				if calls != wantCalls {
					t.Fatalf("calls = %d", calls)
				}
				// The environment gate controls lookups regardless of output format.
				if format != "json" {
					return
				}
				var out struct {
					Data map[string]interface{} `json:"data"`
				}
				if err := json.Unmarshal(rt.Factory.IOStreams.Out.(*bytes.Buffer).Bytes(), &out); err != nil {
					t.Fatal(err)
				}
				for _, key := range []string{"chat_type", "chat_default_name"} {
					if _, exists := out.Data[key]; exists {
						t.Fatalf("metadata must live on messages, not data: %#v", out.Data)
					}
				}
				messages := citationItems(out.Data, "messages")
				if len(messages) != 1 || messages[0]["chat_id"] != "oc_test" || messages[0]["chat_type"] != tc.kind || messages[0]["chat_default_name"] != tc.name {
					t.Fatalf("missing message metadata: %#v", out.Data)
				}
				if _, exists := out.Data["chat_name"]; exists != enabled {
					t.Fatalf("output = %#v", out.Data)
				}
				if enabled {
					cs := chatMessagesListCitations(rt, out.Data)
					if len(cs) != 1 || cs[0].Title != tc.title {
						t.Fatalf("citations = %#v", cs)
					}
				}
			})
		}
	}
}

func TestMessagesSearchCitationRequestAndOutput(t *testing.T) {
	for _, format := range []string{"json", "table", "csv", "ndjson"} {
		for _, enabled := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/%v", format, enabled), func(t *testing.T) {
				t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
				t.Setenv("LARKSUITE_CLI_CITATION", map[bool]string{true: "1", false: ""}[enabled])
				calls := make(map[string]int)
				rt := newMessagesSearchRuntime(t, map[string]string{"query": "test"}, nil, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
					calls[req.URL.Path]++
					var data map[string]interface{}
					switch req.URL.Path {
					case "/open-apis/im/v1/messages/search":
						data = map[string]interface{}{"items": []interface{}{map[string]interface{}{"meta_data": map[string]interface{}{"message_id": "om_test"}}}}
					case "/open-apis/im/v1/messages/mget":
						data = map[string]interface{}{"items": []interface{}{map[string]interface{}{"message_id": "om_test", "chat_id": "oc_test", "msg_type": "text", "body": map[string]interface{}{"content": `{"text":"body"}`}, "sender": map[string]interface{}{"id": "ou_self", "id_type": "open_id", "sender_type": "user", "sender_name": "A"}}}}
					case "/open-apis/im/v1/chats/batch_query":
						data = map[string]interface{}{"items": []interface{}{map[string]interface{}{"chat_id": "oc_test", "chat_mode": "p2p", "p2p_target_id": "ou_peer", "p2p_target_type": "user"}}}
					case "/open-apis/contact/v3/users/basic_batch":
						if !enabled {
							t.Fatal("extra name request with gate disabled")
						}
						data = map[string]interface{}{"users": []interface{}{map[string]interface{}{"user_id": "ou_peer", "name": "B"}}}
					default:
						t.Fatalf("unexpected API: %s", req.URL)
					}
					return shortcutJSONResponse(200, map[string]interface{}{"code": 0, "data": data}), nil
				}))
				setRuntimeField(t, rt, "resolvedAs", core.AsUser)
				isolateCitationTestFactory(t, rt)
				rt.Cmd.Flags().Bool("no-reactions", true, "")
				rt.Config.UserOpenId, rt.Config.UserName = "ou_self", "A"
				rt.Format = format
				if err := ImMessagesSearch.Execute(context.Background(), rt); err != nil {
					t.Fatal(err)
				}
				if calls["/open-apis/im/v1/chats/batch_query"] != 1 {
					t.Fatal("existing lookup must run exactly once regardless of gate")
				}
				wantNameCalls := 0
				if enabled {
					wantNameCalls = 1
				}
				if got := calls["/open-apis/contact/v3/users/basic_batch"]; got != wantNameCalls {
					t.Fatalf("name requests = %d, want %d", got, wantNameCalls)
				}
				if format != "json" {
					return
				}
				var out struct {
					Data map[string]interface{} `json:"data"`
				}
				if err := json.Unmarshal(rt.Factory.IOStreams.Out.(*bytes.Buffer).Bytes(), &out); err != nil {
					t.Fatal(err)
				}
				messages := citationItems(out.Data, "messages")
				if len(messages) != 1 {
					t.Fatalf("data = %#v", out.Data)
				}
				if _, exists := messages[0]["chat_name"]; exists != enabled {
					t.Fatalf("message = %#v", messages[0])
				}
				if enabled && messagesSearchCitations(rt, out.Data)[0].Title != "A和B的会话" {
					t.Fatal("wrong P2P title")
				}
			})
		}
	}
}

func TestMessageCitationDryRunEnvGate(t *testing.T) {
	for _, tc := range []struct {
		env, format, jq string
		concise, want   bool
	}{
		{"", "json", "", false, false}, {"0", "json", "", false, false}, {"true", "json", "", false, false},
		{"1", "json", "", false, true}, {"1", "", "", false, true},
		{"1", "pretty", "", false, true}, {"1", "table", "", false, true},
		{"1", "csv", "", false, true}, {"1", "ndjson", "", false, true},
		{"1", "pretty", ".data", false, true}, {"1", "json", "", true, true},
		{"", "pretty", ".data", false, false},
	} {
		t.Run(fmt.Sprintf("%s/%s/%s/%v", tc.env, tc.format, tc.jq, tc.concise), func(t *testing.T) {
			t.Setenv("LARKSUITE_CLI_CITATION", tc.env)
			rt := newMsgListTestRT(t, nil)
			isolateCitationTestFactory(t, rt)
			rt.Cmd.Flags().Bool("concise", tc.concise, "")
			rt.Format, rt.JqExpr = tc.format, tc.jq
			params, err := buildChatMessageListRequest(rt, "oc_test")
			if err != nil {
				t.Fatal(err)
			}
			if _, exists := params["with_chat_info"]; exists {
				t.Fatal("unexpected chat metadata query parameter")
			}
			dry := mustMarshalDryRun(t, ImChatMessageList.DryRun(context.Background(), rt))
			if strings.Contains(dry, "with_chat_info") || strings.Contains(dry, "Citation only:") != tc.want {
				t.Fatalf("dry-run gate: %s", dry)
			}
			if strings.Contains(dry, "/open-apis/bot/v3/bots/basic_batch") != tc.want {
				t.Fatalf("bot fallback dry-run gate: %s", dry)
			}
			if strings.Contains(dry, "user P2P may additionally query chats/batch_query") != tc.want || strings.Contains(dry, "falling back to GET /open-apis/im/v1/chats/{chat_id}") {
				t.Fatalf("user-only chat lookup dry-run gate: %s", dry)
			}
			searchRT := newMessagesSearchRuntime(t, map[string]string{"query": "test"}, nil, nil)
			isolateCitationTestFactory(t, searchRT)
			searchRT.Cmd.Flags().Bool("concise", tc.concise, "")
			searchRT.Format, searchRT.JqExpr = tc.format, tc.jq
			searchDry := mustMarshalDryRun(t, ImMessagesSearch.DryRun(context.Background(), searchRT))
			for _, plan := range []string{dry, searchDry} {
				if strings.Contains(plan, "/open-apis/im/v1/chats/{chat_id}/members") != tc.want {
					t.Fatalf("members fallback dry-run gate: %s", plan)
				}
			}
			if strings.Contains(searchDry, "Citation only:") != tc.want {
				t.Fatalf("search dry-run gate: %s", searchDry)
			}
			if strings.Contains(searchDry, "/open-apis/bot/v3/bots/basic_batch") != tc.want {
				t.Fatalf("search bot fallback dry-run gate: %s", searchDry)
			}
			if strings.Contains(searchDry, "GET /open-apis/im/v1/chats/{chat_id}") != tc.want {
				t.Fatalf("search chat fallback dry-run gate: %s", searchDry)
			}
		})
	}
}

func TestMessageChatTitlesMissingParticipantBatched(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	for _, fail := range []bool{false, true} {
		t.Run(fmt.Sprint(fail), func(t *testing.T) {
			calls := 0
			rt := newUserShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				calls++
				if req.Method != "POST" || req.URL.Path != "/open-apis/contact/v3/users/basic_batch" {
					t.Fatalf("unexpected request: %s", req.URL)
				}
				var body struct {
					IDs []string `json:"user_ids"`
				}
				if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
					t.Fatal(err)
				}
				if len(body.IDs) != 1 || body.IDs[0] != "ou_peer" || req.URL.Query().Get("user_id_type") != "open_id" {
					t.Fatalf("request = %#v", body)
				}
				if fail {
					return shortcutJSONResponse(403, map[string]interface{}{"code": 99991672, "msg": "denied"}), nil
				}
				return shortcutJSONResponse(200, map[string]interface{}{"code": 0, "data": map[string]interface{}{"users": []interface{}{map[string]interface{}{"user_id": "ou_peer", "name": "A"}}}}), nil
			}))
			rt.Cmd = &cobra.Command{Use: "test"}
			rt.Config.UserOpenId, rt.Config.UserName = "ou_self", "A"
			isolateCitationTestFactory(t, rt)
			chats := map[string]messageChatContext{
				// Multiple chats referencing one partner still make one name lookup.
				"oc_1": {kind: "p2p", partnerID: "ou_peer"}, "oc_2": {kind: "p2p", partnerID: "ou_peer"},
				"oc_group": {kind: "group", name: "Group"},
			}
			// Only self has spoken; peer must not be inferred from message order.
			messages := []map[string]interface{}{{"sender": map[string]interface{}{"id": "ou_self", "name": "A"}}}
			got := messageChatTitles(rt, chats, messages)
			want := "A和A的会话"
			if fail {
				want = "来自单聊"
			}
			if calls != 1 || got["oc_1"] != want || got["oc_2"] != want || got["oc_group"] != "Group" {
				t.Fatalf("titles = %#v, calls = %d", got, calls)
			}
		})
	}
}

func TestMessageChatTitlesIncompleteParticipants(t *testing.T) {
	for _, tc := range []struct {
		name, selfID, selfName, peerID, peerName string
	}{
		{"missing-peer-id", "ou_self", "A", "", ""},
		{"missing-peer-name", "ou_self", "A", "ou_peer", ""},
		{"missing-self-name", "ou_self", "", "ou_peer", "B"},
		{"missing-self-id", "", "A", "ou_peer", "B"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rt := newUserShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				return shortcutJSONResponse(403, map[string]interface{}{"code": 99991672, "msg": "denied"}), nil
			}))
			isolateCitationTestFactory(t, rt)
			rt.Config.UserOpenId, rt.Config.UserName = tc.selfID, tc.selfName
			chats := map[string]messageChatContext{
				"p2p":           {kind: "p2p", partnerID: tc.peerID},
				"group":         {kind: "group", name: "Group"},
				"unnamed-group": {kind: "group"},
				"unknown":       {},
			}
			messages := []map[string]interface{}{{"sender": map[string]interface{}{"id": tc.peerID, "name": tc.peerName}}}
			got := messageChatTitles(rt, chats, messages)
			if got["p2p"] != "来自单聊" || got["group"] != "Group" || got["unnamed-group"] != "" || got["unknown"] != "" {
				t.Fatalf("titles = %#v", got)
			}
		})
	}

}

func TestMessageChatTitlesReuseSenderNames(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	rt := newUserShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		t.Fatalf("unexpected API call: %s", req.URL)
		return nil, nil
	}))
	rt.Config.UserOpenId = "ou_self"
	isolateCitationTestFactory(t, rt)
	chats := map[string]messageChatContext{"oc_1": {kind: "p2p", partnerID: "ou_peer"}}
	for _, reverse := range []bool{false, true} {
		messages := []map[string]interface{}{
			{"sender": map[string]interface{}{"id": "ou_self", "name": "A"}},
			{"sender": map[string]interface{}{"id": "ou_peer", "name": "B"}},
		}
		if reverse {
			messages[0], messages[1] = messages[1], messages[0]
		}
		if got := messageChatTitles(rt, chats, messages)["oc_1"]; got != "A和B的会话" {
			t.Fatalf("title = %s", got)
		}
	}
}

func TestMessageListMetadataSurvivesEmptyPage(t *testing.T) {
	result := &imMapListResult{}
	_ = result.AddPage(imMapListPage{Items: []map[string]interface{}{{"chat_type": "group", "chat_default_name": "Group"}}, HasMore: true})
	_ = result.AddPage(imMapListPage{})
	if len(result.items) != 1 || result.items[0]["chat_type"] != "group" || result.items[0]["chat_default_name"] != "Group" {
		t.Fatal("metadata lost")
	}
}

func TestMessageChatTitlesBotIdentityAndTarget(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	rt := newUserShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		t.Fatalf("unexpected request: %s", req.URL)
		return nil, nil
	}))
	isolateCitationTestFactory(t, rt)
	rt.Config.UserOpenId, rt.Config.UserName = "ou_user", "A"
	messages := []map[string]interface{}{
		{"sender": map[string]interface{}{"id": "cli_bot", "open_bot_id": "ou_bot", "name": "Assistant"}},
		{"sender": map[string]interface{}{"id": "ou_user", "name": "A"}},
	}
	got := messageChatTitles(rt, map[string]messageChatContext{"oc_1": {kind: "p2p", partnerID: "ou_bot", partnerType: "bot"}}, messages)
	if got["oc_1"] != "A和Assistant的会话" {
		t.Fatalf("user view: %#v", got)
	}
	setRuntimeField(t, rt, "resolvedAs", core.AsBot)
	setRuntimeField(t, rt, "botInfoFunc", func() (*common.BotInfo, error) { return &common.BotInfo{OpenID: "ou_bot", AppName: "Assistant"}, nil })
	got = messageChatTitles(rt, map[string]messageChatContext{"oc_1": {kind: "p2p", partnerID: "ou_user", partnerType: "user"}}, messages)
	if got["oc_1"] != "Assistant和A的会话" {
		t.Fatalf("bot view: %#v", got)
	}
}

func TestMessageCitationLeavesURLValidationToFramework(t *testing.T) {
	msg := textMessage()
	delete(msg, "chat_name")
	c, ok := messageCitation(citationBrand(nil), msg, "")
	if !ok || c.Title != "" {
		t.Fatal("must not use content as title")
	}
	delete(msg, "chat_id")
	for _, link := range []string{"", "http://example.com", "javascript:alert(1)", "https:///missing-host"} {
		msg["message_app_link"] = link
		if c, ok := messageCitation(citationBrand(nil), msg, ""); !ok || c.URL != link {
			t.Fatalf("must leave URL %q for framework validation: citation=%+v, ok=%v", link, c, ok)
		}
	}
}
