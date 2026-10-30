// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/citation"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/spf13/cobra"
)

func newMGetCitationRuntime(t *testing.T, transport http.RoundTripper) *common.RuntimeContext {
	t.Helper()
	rt := newUserShortcutRuntime(t, transport)
	isolateCitationTestFactory(t, rt)
	rt.Cmd = &cobra.Command{Use: "+messages-mget"}
	rt.Cmd.Flags().String("message-ids", "om_root,om_second", "")
	rt.Cmd.Flags().Bool("no-reactions", true, "")
	rt.Format = "json"
	setRuntimeField(t, rt, "citation", ImMessagesMGet.Citation)
	return rt
}

func TestMessagesMGetCitationEnvelope(t *testing.T) {
	for _, identity := range []core.Identity{core.AsUser, core.AsBot} {
		for _, env := range []string{"", "0", "true", "1"} {
			for _, format := range []string{"json", "pretty", "table", "csv", "ndjson"} {
				t.Run(string(identity)+"/"+env+"/"+format, func(t *testing.T) {
					t.Setenv("LARKSUITE_CLI_CITATION", env)
					calls := map[string]int{}
					rt := newMGetCitationRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
						calls[req.URL.Path]++
						if req.Method != http.MethodGet {
							t.Fatalf("unexpected method: %s", req.Method)
						}
						var items []map[string]interface{}
						switch req.URL.Path {
						case "/open-apis/im/v1/messages/mget":
							if got := req.URL.Query()["message_ids"]; !reflect.DeepEqual(got, []string{"om_root", "om_second"}) {
								t.Fatalf("message_ids = %v", got)
							}
							items = []map[string]interface{}{
								{"message_id": "om_root", "chat_id": "oc_one", "chat_type": "group", "chat_default_name": "Group One", "thread_id": "omt_test", "msg_type": "text", "body": map[string]interface{}{"content": `{"text":"root"}`}},
								{"message_id": "om_second", "chat_id": "oc_two", "chat_type": "group", "chat_default_name": "Group Two", "msg_type": "image", "body": map[string]interface{}{"content": `{"image_key":"img_test"}`}},
							}
						case imMessagesListPath:
							if req.URL.Query().Get("container_id") != "omt_test" || req.URL.Query().Get("container_id_type") != "thread" {
								t.Fatalf("thread request: %s", req.URL)
							}
							// The reply omits chat_id and inherits its root's citation context.
							items = []map[string]interface{}{{"message_id": "om_reply", "msg_type": "file", "body": map[string]interface{}{"content": `{"file_key":"file_test","file_name":"test.txt"}`}}}
						default:
							t.Fatalf("unexpected enrichment request: %s", req.URL)
						}
						return shortcutJSONResponse(200, map[string]interface{}{"code": 0, "data": map[string]interface{}{"items": items, "has_more": false}}), nil
					}))
					setRuntimeField(t, rt, "resolvedAs", identity)
					rt.Format = format
					if err := ImMessagesMGet.Execute(context.Background(), rt); err != nil {
						t.Fatal(err)
					}
					if len(calls) != 2 || calls[imMessagesListPath] != 1 {
						t.Fatalf("requests = %v", calls)
					}
					stdout := rt.IO().Out.(*bytes.Buffer).Bytes()
					if format != "json" {
						if bytes.Contains(stdout, []byte(`"citations"`)) {
							t.Fatalf("non-envelope output contains citations: %s", stdout)
						}
						if format == "pretty" && bytes.Contains(stdout, []byte("Group One")) {
							t.Fatalf("citation changed pretty columns: %s", stdout)
						}
						return
					}
					var out struct {
						OK        bool                   `json:"ok"`
						Data      map[string]interface{} `json:"data"`
						Citations []string               `json:"citations"`
					}
					if err := json.Unmarshal(stdout, &out); err != nil {
						t.Fatal(err)
					}
					messages := citationItems(out.Data, "messages")
					if !out.OK || len(messages) != 2 || out.Data["total"] != float64(2) || messages[0]["content"] != "root" {
						t.Fatalf("message payload changed: %s", stdout)
					}
					if _, exists := messages[0]["chat_name"]; exists {
						t.Fatal("citation must not introduce chat_name")
					}
					replies := citationItems(messages[0], "thread_replies")
					if len(replies) != 1 {
						t.Fatalf("replies = %v", replies)
					}
					if env != "1" {
						if _, exists := messages[0]["citation_title"]; exists || len(out.Citations) != 0 {
							t.Fatalf("disabled gate enriched output: %s", stdout)
						}
						if _, exists := replies[0]["citation_title"]; exists {
							t.Fatal("disabled gate enriched replies")
						}
						return
					}
					wantTitles := []string{"Group One", "Group One", "Group Two"}
					if len(out.Citations) != len(wantTitles) || replies[0]["citation_title"] != "Group One" {
						t.Fatalf("citations = %#v; replies = %#v", out.Citations, replies)
					}
					for i, entry := range out.Citations {
						if !strings.Contains(entry, "<title>"+wantTitles[i]+"</title>") || !strings.Contains(entry, "https://") {
							t.Fatalf("citation %d = %#v", i, entry)
						}
					}
				})
			}
		}
	}
}

func TestMessagesMGetCitationChatLookup(t *testing.T) {
	for _, tc := range []struct {
		name, env, wantTitle        string
		batchFails, getFails, empty bool
		wantBatch, wantGet          int
	}{
		{name: "batch", env: "1", wantTitle: "Batch Group", wantBatch: 1},
		{name: "get-fallback", env: "1", batchFails: true, wantTitle: "Topic Group", wantBatch: 1, wantGet: 1},
		{name: "denied", env: "1", batchFails: true, getFails: true, wantBatch: 1, wantGet: 1},
		{name: "disabled"},
		{name: "empty", env: "1", empty: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("LARKSUITE_CLI_CITATION", tc.env)
			batch, get := 0, 0
			rt := newMGetCitationRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				var data map[string]interface{}
				switch req.URL.Path {
				case "/open-apis/im/v1/messages/mget":
					items := []map[string]interface{}{}
					if !tc.empty {
						for _, id := range []string{"om_root", "om_second"} {
							items = append(items, map[string]interface{}{"message_id": id, "chat_id": "oc_test", "msg_type": "text", "body": map[string]interface{}{"content": `{"text":"hello"}`}})
						}
					}
					data = map[string]interface{}{"items": items}
				case "/open-apis/im/v1/chats/batch_query":
					batch++
					if tc.batchFails {
						return shortcutJSONResponse(403, map[string]interface{}{"code": 99991672, "msg": "denied"}), nil
					}
					data = map[string]interface{}{"items": []map[string]interface{}{{"chat_id": "oc_test", "chat_mode": "group", "name": "Batch Group"}}}
				case "/open-apis/im/v1/chats/oc_test":
					get++
					if tc.getFails {
						return shortcutJSONResponse(403, map[string]interface{}{"code": 99991672, "msg": "denied"}), nil
					}
					data = map[string]interface{}{"chat_mode": "topic", "name": "Topic Group"}
				default:
					t.Fatalf("unexpected request: %s", req.URL)
				}
				return shortcutJSONResponse(200, map[string]interface{}{"code": 0, "data": data}), nil
			}))
			if err := ImMessagesMGet.Execute(context.Background(), rt); err != nil {
				t.Fatal(err)
			}
			if batch != tc.wantBatch || get != tc.wantGet {
				t.Fatalf("batch=%d get=%d", batch, get)
			}
			var out struct {
				Data      map[string]interface{} `json:"data"`
				Citations []string               `json:"citations"`
			}
			stdout := rt.IO().Out.(*bytes.Buffer).Bytes()
			if err := json.Unmarshal(stdout, &out); err != nil {
				t.Fatal(err)
			}
			if tc.empty {
				if len(citationItems(out.Data, "messages")) != 0 || bytes.Contains(stdout, []byte(`"citations"`)) {
					t.Fatalf("empty result: %s", stdout)
				}
				return
			}
			if len(citationItems(out.Data, "messages")) != 2 {
				t.Fatalf("lookup failure lost messages: %s", stdout)
			}
			if tc.env == "1" {
				if len(out.Citations) != 2 || !strings.Contains(out.Citations[0], "<title>"+tc.wantTitle+"</title>") {
					t.Fatalf("citations = %#v", out.Citations)
				}
			} else if len(out.Citations) != 0 {
				t.Fatal("disabled citations")
			}
		})
	}
}

func TestMessagesMGetCitationAllTypesAndScannedTitles(t *testing.T) {
	if err := common.ValidateCitationShortcut(ImMessagesMGet); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"text", "image", "file", "post", "interactive", "audio", "video", "media", "sticker", "system", "merge_forward", "share_chat", "share_user", "future_type"} {
		msg := textMessage()
		msg["msg_type"], msg["citation_title"] = kind, "Scanned title"
		msg["thread_replies"] = []map[string]interface{}{{"msg_type": kind, "citation_title": "Scanned reply", "create_time": "2026-07-27 21:26"}}
		entries := messagesMGetCitations(nil, map[string]interface{}{"messages": []map[string]interface{}{msg}})
		if len(entries) != 2 || entries[0].SourceType != citation.SourceMessage || entries[0].Title != "Scanned title" || entries[0].URL != testMessageAppLink || entries[1].Title != "Scanned reply" || !strings.Contains(entries[1].URL, "oc_1") || entries[1].PublishTime == "" {
			t.Fatalf("%s: %#v", kind, entries)
		}
		if msg["chat_name"] != "Example group" {
			t.Fatal("builder overwrote chat_name")
		}
	}
}

func TestMessagesMGetCitationDryRun(t *testing.T) {
	for _, env := range []string{"0", "1"} {
		for _, identity := range []string{"user", "bot"} {
			t.Run(env+"/"+identity, func(t *testing.T) {
				t.Setenv("LARKSUITE_CLI_CITATION", env)
				rt := newMGetCitationRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
					t.Fatalf("dry-run must not call %s", req.URL)
					return nil, nil
				}))
				parent := &cobra.Command{Use: "im"}
				ImMessagesMGet.Mount(parent, rt.Factory)
				parent.SetArgs([]string{"+messages-mget", "--as", identity, "--message-ids", "om_root,om_second", "--no-reactions", "--dry-run", "--format", "json"})
				if err := parent.Execute(); err != nil {
					t.Fatal(err)
				}
				stdout := rt.IO().Out.(*bytes.Buffer).String()
				for _, want := range []string{`"method": "GET"`, "/open-apis/im/v1/messages/mget", "om_root", "om_second"} {
					if !strings.Contains(stdout, want) {
						t.Fatalf("dry-run missing %q: %s", want, stdout)
					}
				}
				if strings.Contains(stdout, "Citation only:") != (env == "1") || strings.Contains(stdout, `"citations"`) {
					t.Fatalf("unexpected citation dry-run output: %s", stdout)
				}
			})
		}
	}
}

func TestMessagesMGetCitationUserP2P(t *testing.T) {
	for _, denied := range []bool{false, true} {
		t.Run(map[bool]string{false: "resolved", true: "name-denied"}[denied], func(t *testing.T) {
			t.Setenv("LARKSUITE_CLI_CITATION", "1")
			contacts := 0
			rt := newMGetCitationRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				var data map[string]interface{}
				switch req.URL.Path {
				case "/open-apis/im/v1/messages/mget":
					data = map[string]interface{}{"items": []map[string]interface{}{{"message_id": "om_root", "chat_id": "oc_p2p", "msg_type": "text", "body": map[string]interface{}{"content": `{"text":"hello"}`}}}}
				case "/open-apis/im/v1/chats/batch_query":
					data = map[string]interface{}{"items": []map[string]interface{}{{"chat_id": "oc_p2p", "chat_mode": "p2p", "p2p_target_id": "ou_peer", "p2p_target_type": "user"}}}
				case "/open-apis/contact/v3/users/basic_batch":
					contacts++
					if req.Method != http.MethodPost || req.URL.Query().Get("user_id_type") != "open_id" {
						t.Fatalf("unexpected contact request: %s %s", req.Method, req.URL)
					}
					if denied {
						return shortcutJSONResponse(403, map[string]interface{}{"code": 99991672, "msg": "denied"}), nil
					}
					data = map[string]interface{}{"users": []map[string]interface{}{{"user_id": "ou_peer", "name": "B"}}}
				default:
					t.Fatalf("unexpected request: %s", req.URL)
				}
				return shortcutJSONResponse(200, map[string]interface{}{"code": 0, "data": data}), nil
			}))
			rt.Config.UserOpenId, rt.Config.UserName = "ou_self", "A"
			setRuntimeScopes(t, rt, "contact:user.basic_profile:readonly")
			if err := ImMessagesMGet.Execute(context.Background(), rt); err != nil {
				t.Fatal(err)
			}
			var out struct {
				Data      map[string]interface{} `json:"data"`
				Citations []string               `json:"citations"`
			}
			if err := json.Unmarshal(rt.IO().Out.(*bytes.Buffer).Bytes(), &out); err != nil {
				t.Fatal(err)
			}
			want := "A和B的会话"
			if denied {
				want = "来自单聊"
			}
			messages := citationItems(out.Data, "messages")
			if contacts != 1 || len(messages) != 1 || messages[0]["content"] != "hello" || messages[0]["citation_title"] != want || len(out.Citations) != 1 || !strings.Contains(out.Citations[0], "<title>"+want+"</title>") {
				t.Fatalf("contacts=%d messages=%v citations=%v", contacts, messages, out.Citations)
			}
			if _, exists := messages[0]["chat_name"]; exists {
				t.Fatal("P2P citation must not introduce chat_name")
			}
		})
	}
}
