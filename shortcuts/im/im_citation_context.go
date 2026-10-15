// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"fmt"
	"net/http"
	"net/url"
	"sort"

	"github.com/larksuite/cli/shortcuts/common"
)

// messageChatContext holds the API fields needed for citation titles.
type messageChatContext struct {
	kind        string // group or p2p
	name        string // Original name; used directly for groups only.
	partnerID   string // P2P peer relative to the caller, not a message sender.
	partnerType string // Peer type; bot IDs must not go to user contact endpoints.
}

type citationUserInfo struct {
	id, name string
}

// projectCitationUser follows contact +get-user's ID/name fallbacks.
func projectCitationUser(raw map[string]interface{}) citationUserInfo {
	user := citationUserInfo{id: asString(raw["open_id"])}
	if user.id == "" {
		user.id = asString(raw["user_id"])
	}
	for _, key := range []string{"name", "user_name", "display_name", "employee_name", "cn_name"} {
		if user.name = asString(raw[key]); user.name != "" {
			break
		}
	}
	return user
}

// projectMessageChatContexts adapts chats/batch_query fields, keyed by chat_id.
func projectMessageChatContexts(raw map[string]map[string]interface{}) map[string]messageChatContext {
	chats := make(map[string]messageChatContext, len(raw))
	for id, item := range raw {
		chats[id] = projectMessageChatContext(item)
	}
	return chats
}

func projectMessageChatContext(item map[string]interface{}) messageChatContext {
	kind := asString(item["chat_mode"])
	if kind != "" && kind != "p2p" {
		kind = "group"
	}
	return messageChatContext{kind, asString(item["name"]), asString(item["p2p_target_id"]), asString(item["p2p_target_type"])}
}

// fillMissingMessageChatContexts falls back to chats.get when batch lookup has no entry.
// Callers gate this extra traffic on citation. A P2P owner_id is not a peer ID.
func fillMissingMessageChatContexts(rt *common.RuntimeContext, chats map[string]messageChatContext, ids []string) {
	seen := make(map[string]bool)
	for _, id := range ids {
		if id == "" || seen[id] || chats[id].kind != "" {
			continue
		}
		seen[id] = true
		data, err := rt.CallAPITyped(http.MethodGet, "/open-apis/im/v1/chats/"+url.PathEscape(id), map[string]interface{}{"user_id_type": "open_id"}, nil)
		if err != nil {
			fmt.Fprintf(rt.IO().ErrOut, "warning: citation chat lookup failed: %v\n", err)
			continue
		}
		chats[id] = projectMessageChatContext(data)
	}
}

// messageChatTitles uses group names or joins the caller's and P2P peer's names.
// Lookups are best-effort; incomplete P2P names use a generic title.
func messageChatTitles(rt *common.RuntimeContext, chats map[string]messageChatContext, messages []map[string]interface{}) map[string]string {
	titles := make(map[string]string, len(chats))
	needsP2P := false
	for id, chat := range chats {
		if chat.kind == "p2p" {
			needsP2P = true
		} else if chat.kind == "group" {
			titles[id] = chat.name
		}
	}
	if !needsP2P {
		// Group-only results need no identity or contact lookups.
		return titles
	}
	// Reuse direct sender names, keyed by ID rather than display name.
	names := make(map[string]string)
	for _, msg := range messages {
		if sender, ok := msg["sender"].(map[string]interface{}); ok {
			// Bot senders and P2P peers may use different ID representations.
			identity := struct{ id, openBotID, name string }{
				asString(sender["id"]), asString(sender["open_bot_id"]), asString(sender["name"]),
			}
			if identity.name != "" {
				for _, id := range []string{identity.id, identity.openBotID} {
					if id != "" {
						names[id] = identity.name
					}
				}
			}
		}
	}
	// Identify the caller independently of message senders.
	selfID, selfName := "", ""
	if rt.IsBot() {
		if bot, err := rt.BotInfo(); err == nil {
			selfID, selfName = bot.OpenID, bot.AppName
		}
	} else {
		selfID, selfName = rt.Config.UserOpenId, rt.Config.UserName
		if selfID == "" || (selfName == "" && names[selfID] == "") {
			if data, err := rt.CallAPITyped(http.MethodGet, "/open-apis/authen/v1/user_info", nil, nil); err == nil {
				user := projectCitationUser(data)
				selfID, selfName = user.id, user.name
			}
		}
	}
	if selfID != "" && names[selfID] == "" {
		// Preserve existing sender names; config/user_info only fills gaps.
		names[selfID] = selfName
	}
	if rt.IsBot() {
		fillBotP2PPartners(rt, chats, names, selfID, messages)
	}
	// Keep bot Open IDs out of contact user lookups.
	missing := make(map[string]bool)
	var userIDs, botIDs []string
	for _, chat := range chats {
		if chat.kind != "p2p" || chat.partnerID == "" || names[chat.partnerID] != "" || missing[chat.partnerID] {
			continue
		}
		missing[chat.partnerID] = true
		if chat.partnerType == "bot" {
			botIDs = append(botIDs, chat.partnerID)
		} else {
			userIDs = append(userIDs, chat.partnerID)
		}
	}
	sort.Strings(userIDs) // Keep request batches deterministic despite map iteration order.
	sort.Strings(botIDs)
	fillCitationUserNames(rt, userIDs, names)
	fillCitationBotNames(rt, botIDs, names)
	// Require both names; never substitute message text or IDs.
	for id, chat := range chats {
		if chat.kind != "p2p" {
			continue
		}
		if a, b := names[selfID], names[chat.partnerID]; a != "" && b != "" {
			titles[id] = a + "和" + b + "的会话"
		} else {
			titles[id] = "来自单聊"
			fmt.Fprintln(rt.IO().ErrOut, "warning: using generic P2P citation title: missing participant name or identity")
		}
	}
	return titles
}

// members.get excludes bots, so a complete bot P2P result has one human peer.
func fillBotP2PPartners(rt *common.RuntimeContext, chats map[string]messageChatContext, names map[string]string, selfID string, messages []map[string]interface{}) {
	for id, chat := range chats {
		if chat.kind != "p2p" || chat.partnerType == "bot" || (chat.partnerID != "" && names[chat.partnerID] != "") {
			continue
		}
		// Reuse the unique human sender from this P2P chat before querying members.
		peerID := chat.partnerID
		for _, msg := range messages {
			msgChatID := asString(msg["chat_id"])
			if msgChatID != id && (msgChatID != "" || len(chats) != 1) {
				continue
			}
			raw, _ := msg["sender"].(map[string]interface{})
			sender := struct{ id, kind string }{asString(raw["id"]), asString(raw["sender_type"])}
			if sender.kind != "user" || sender.id == "" || sender.id == selfID {
				continue
			}
			if peerID != "" && peerID != sender.id {
				peerID = ""
				break
			}
			peerID = sender.id
		}
		if selfID != "" && peerID != "" && names[selfID] != "" && names[peerID] != "" {
			chat.partnerID, chat.partnerType = peerID, "user"
			chats[id] = chat
			continue
		}
		data, err := rt.CallAPITyped(http.MethodGet, "/open-apis/im/v1/chats/"+url.PathEscape(id)+"/members", map[string]interface{}{"member_id_type": "open_id", "page_size": 50}, nil)
		if err != nil {
			fmt.Fprintf(rt.IO().ErrOut, "warning: citation P2P member lookup failed: %v\n", err)
			continue
		}
		page := struct {
			items               []map[string]interface{}
			hasMore, restricted bool
		}{items: citationItems(data, "items")}
		page.hasMore, _ = data["has_more"].(bool)
		page.restricted, _ = data["trigger_security_conf_limit"].(bool)
		// Never infer a peer from a partial or ambiguous membership list.
		if page.hasMore || page.restricted || len(page.items) != 1 {
			continue
		}
		peer := citationUserInfo{asString(page.items[0]["member_id"]), asString(page.items[0]["name"])}
		if peer.id == "" || peer.id == selfID || (chat.partnerID != "" && chat.partnerID != peer.id) {
			continue
		}
		chat.partnerID, chat.partnerType = peer.id, "user"
		chats[id] = chat
		if names[peer.id] == "" {
			names[peer.id] = peer.name
		}
	}
}

// fillCitationBotNames resolves missing P2P peers, even if they have never sent a message.
func fillCitationBotNames(rt *common.RuntimeContext, ids []string, names map[string]string) {
	if len(ids) == 0 {
		return
	}
	if !rt.IsBot() {
		if err := checkFlagRequiredScopes(rt.Ctx(), rt, []string{"application:bot.basic_info:read"}); err != nil {
			fmt.Fprintf(rt.IO().ErrOut, "warning: citation bot name lookup skipped: %v\n", err)
			return
		}
	}
	// Both identities use bot Open IDs and repeated bot_ids query keys (max 10).
	for _, batch := range chunkStrings(ids, 10) {
		data, err := rt.CallAPITyped(http.MethodGet, "/open-apis/bot/v3/bots/basic_batch", map[string]interface{}{"bot_ids": batch}, nil)
		if err != nil {
			fmt.Fprintf(rt.IO().ErrOut, "warning: citation bot name lookup failed: %v\n", err)
			continue
		}
		bots, _ := data["bots"].(map[string]interface{})
		failed, _ := data["failed_bots"].(map[string]interface{})
		for _, id := range batch {
			raw, _ := bots[id].(map[string]interface{})
			bot := citationBotResult{name: asString(raw["name"])}
			_, bot.failed = failed[id]
			if bot.failed || bot.name == "" {
				fmt.Fprintf(rt.IO().ErrOut, "warning: citation bot name unavailable for %s\n", id)
				continue
			}
			names[id] = bot.name
		}
	}
}

type citationBotResult struct {
	name   string
	failed bool
}

// fillCitationUserNames matches contact +get-user's identity-specific endpoints.
// IDs are deduplicated by the caller; lookup failures only warn.
func fillCitationUserNames(rt *common.RuntimeContext, ids []string, names map[string]string) {
	if len(ids) == 0 {
		return
	}
	if !rt.IsBot() {
		// Optional enrichment must not require a new authorization flow.
		if err := checkFlagRequiredScopes(rt.Ctx(), rt, []string{"contact:user.basic_profile:readonly"}); err != nil {
			fmt.Fprintf(rt.IO().ErrOut, "warning: citation name lookup skipped: %v\n", err)
			return
		}
	}
	if rt.IsBot() {
		for _, id := range ids {
			data, err := rt.CallAPITyped(http.MethodGet, "/open-apis/contact/v3/users/"+url.PathEscape(id), map[string]interface{}{"user_id_type": "open_id"}, nil)
			if err == nil {
				user, _ := data["user"].(map[string]interface{})
				names[id] = projectCitationUser(user).name
			} else {
				fmt.Fprintf(rt.IO().ErrOut, "warning: citation name lookup failed: %v\n", err)
			}
		}
		return
	}
	for _, batch := range chunkStrings(ids, 50) {
		data, err := rt.CallAPITyped(http.MethodPost, "/open-apis/contact/v3/users/basic_batch", map[string]interface{}{"user_id_type": "open_id"}, map[string]interface{}{"user_ids": batch})
		if err != nil {
			fmt.Fprintf(rt.IO().ErrOut, "warning: citation name lookup failed: %v\n", err)
			continue
		}
		for _, raw := range citationItems(data, "users") {
			user := projectCitationUser(raw)
			if user.id != "" {
				names[user.id] = user.name
			}
		}
	}
}
