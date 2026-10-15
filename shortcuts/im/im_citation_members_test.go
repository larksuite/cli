// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"net/http"
	"testing"
)

func TestFillBotP2PPartners(t *testing.T) {
	for _, tc := range []struct {
		name, kind, knownID, cachedName  string
		items                            []interface{}
		more, restricted, wantPeer, skip bool
	}{
		{name: "unique-human", kind: "p2p", items: []interface{}{map[string]interface{}{"member_id": "ou_peer", "name": "Peer"}}, wantPeer: true},
		{name: "known-peer-name", kind: "p2p", knownID: "ou_peer", items: []interface{}{map[string]interface{}{"member_id": "ou_peer", "name": "Peer"}}, wantPeer: true},
		{name: "sender-name-wins", kind: "p2p", cachedName: "Sender", items: []interface{}{map[string]interface{}{"member_id": "ou_peer", "name": "Peer"}}, wantPeer: true},
		{name: "already-known", kind: "p2p", knownID: "ou_peer", cachedName: "Sender", skip: true},
		{name: "group", kind: "group", skip: true},
		{name: "unknown", skip: true},
		{name: "empty", kind: "p2p"},
		{name: "multiple", kind: "p2p", items: []interface{}{map[string]interface{}{"member_id": "ou_peer"}, map[string]interface{}{"member_id": "ou_other"}}},
		{name: "paginated", kind: "p2p", more: true, items: []interface{}{map[string]interface{}{"member_id": "ou_peer"}}},
		{name: "restricted", kind: "p2p", restricted: true, items: []interface{}{map[string]interface{}{"member_id": "ou_peer"}}},
		{name: "missing-id", kind: "p2p", items: []interface{}{map[string]interface{}{"name": "Peer"}}},
		{name: "self", kind: "p2p", items: []interface{}{map[string]interface{}{"member_id": "ou_self"}}},
		{name: "conflicting-peer", kind: "p2p", knownID: "ou_known", items: []interface{}{map[string]interface{}{"member_id": "ou_peer"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			rt := newUserShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				calls++
				if req.Method != http.MethodGet || req.URL.Path != "/open-apis/im/v1/chats/oc_test/members" || req.URL.Query().Get("member_id_type") != "open_id" || req.URL.Query().Get("page_size") != "50" {
					t.Fatalf("request = %s %s", req.Method, req.URL)
				}
				return shortcutJSONResponse(200, map[string]interface{}{"code": 0, "data": map[string]interface{}{
					"items": tc.items, "has_more": tc.more, "trigger_security_conf_limit": tc.restricted, "member_total": 0,
				}}), nil
			}))
			isolateCitationTestFactory(t, rt)
			original := messageChatContext{kind: tc.kind, partnerID: tc.knownID}
			chats := map[string]messageChatContext{"oc_test": original}
			names := map[string]string{"ou_peer": tc.cachedName}
			fillBotP2PPartners(rt, chats, names, "ou_self", nil)
			if (calls == 0) != tc.skip || calls > 1 {
				t.Fatalf("calls = %d, skip = %v", calls, tc.skip)
			}
			if !tc.wantPeer {
				if chats["oc_test"] != original || names["ou_peer"] != tc.cachedName {
					t.Fatalf("unexpected enrichment: %#v, %#v", chats, names)
				}
				return
			}
			wantName := "Peer"
			if tc.cachedName != "" {
				wantName = tc.cachedName
			}
			if chats["oc_test"].partnerID != "ou_peer" || chats["oc_test"].partnerType != "user" || names["ou_peer"] != wantName {
				t.Fatalf("enrichment = %#v, %#v", chats, names)
			}
		})
	}
}

func TestFillBotP2PPartnersReusesChatSenders(t *testing.T) {
	for _, tc := range []struct {
		name, selfID, selfName, knownPeer string
		chatIDs                           []string
		multipleChats, ambiguous          bool
		wantCalls                         int
	}{
		{name: "matching-chat", selfID: "ou_self", selfName: "A", chatIDs: []string{"oc_test"}},
		{name: "single-chat-missing-id", selfID: "ou_self", selfName: "A", chatIDs: []string{""}},
		{name: "multi-chat-isolation", selfID: "ou_self", selfName: "A", multipleChats: true, chatIDs: []string{"oc_other", "oc_test"}},
		{name: "other-chat-only", selfID: "ou_self", selfName: "A", chatIDs: []string{"oc_other"}, wantCalls: 1},
		{name: "multi-chat-missing-id", selfID: "ou_self", selfName: "A", multipleChats: true, chatIDs: []string{""}, wantCalls: 1},
		{name: "ambiguous-senders", selfID: "ou_self", selfName: "A", chatIDs: []string{"oc_test"}, ambiguous: true, wantCalls: 1},
		{name: "known-peer-conflict", selfID: "ou_self", selfName: "A", knownPeer: "ou_known", chatIDs: []string{"oc_test"}, wantCalls: 1},
		{name: "missing-self-id", selfName: "A", chatIDs: []string{"oc_test"}, wantCalls: 1},
		{name: "missing-self-name", selfID: "ou_self", chatIDs: []string{"oc_test"}, wantCalls: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			rt := newUserShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				calls++
				if req.URL.Path != "/open-apis/im/v1/chats/oc_test/members" {
					t.Fatalf("unexpected request: %s", req.URL)
				}
				return shortcutJSONResponse(403, map[string]interface{}{"code": 99991672, "msg": "denied"}), nil
			}))
			isolateCitationTestFactory(t, rt)
			chats := map[string]messageChatContext{"oc_test": {kind: "p2p", partnerID: tc.knownPeer}}
			if tc.multipleChats {
				chats["oc_other"] = messageChatContext{kind: "group", name: "Other"}
			}
			names := map[string]string{tc.selfID: tc.selfName, "ou_peer": "B", "ou_other": "C"}
			var messages []map[string]interface{}
			for _, chatID := range tc.chatIDs {
				id := "ou_peer"
				if chatID == "oc_other" {
					id = "ou_other"
				}
				messages = append(messages, map[string]interface{}{"chat_id": chatID, "sender": map[string]interface{}{"id": id, "sender_type": "user", "name": names[id]}})
			}
			if tc.ambiguous {
				messages = append(messages, map[string]interface{}{"chat_id": "oc_test", "sender": map[string]interface{}{"id": "ou_other", "sender_type": "user", "name": "C"}})
			}
			fillBotP2PPartners(rt, chats, names, tc.selfID, messages)
			wantPeer := "ou_peer"
			if tc.wantCalls > 0 {
				wantPeer = tc.knownPeer
			}
			if calls != tc.wantCalls || chats["oc_test"].partnerID != wantPeer {
				t.Fatalf("calls=%d, peer=%q; want calls=%d, peer=%q", calls, chats["oc_test"].partnerID, tc.wantCalls, wantPeer)
			}
		})
	}
}
