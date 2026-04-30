// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"reflect"

	"github.com/larksuite/cli/internal/event/schemas"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
)

// nativeIMKey curates metadata for a Native IM event; fieldOverrides paths are JSON Pointer anchored at the V2-wrapped schema (start with /event/...).
type nativeIMKey struct {
	key            string
	title          string
	description    string
	scopes         []string
	bodyType       reflect.Type
	fieldOverrides map[string]schemas.FieldMeta
}

// userIDOv returns open_id/union_id/user_id overrides for a UserID object at prefix.
func userIDOv(prefix string) map[string]schemas.FieldMeta {
	return map[string]schemas.FieldMeta{
		prefix + "/open_id":  {Kind: "open_id"},
		prefix + "/union_id": {Kind: "union_id"},
		prefix + "/user_id":  {Kind: "user_id"},
	}
}

// mergeOv merges FieldMeta maps left-to-right (later wins).
func mergeOv(ms ...map[string]schemas.FieldMeta) map[string]schemas.FieldMeta {
	out := map[string]schemas.FieldMeta{}
	for _, m := range ms {
		for k, v := range m {
			out[k] = v
		}
	}
	return out
}

var nativeIMKeys = []nativeIMKey{
	{
		key:         "im.message.message_read_v1",
		title:       "Message read",
		description: "Triggered after a user reads a P2P message sent by the bot",
		scopes:      []string{"im:message:readonly", "im:message"},
		bodyType:    reflect.TypeOf(larkim.P2MessageReadV1Data{}),
		fieldOverrides: mergeOv(
			userIDOv("/event/reader/reader_id"),
			map[string]schemas.FieldMeta{
				"/event/reader/read_time":  {Kind: "timestamp_ms"},
				"/event/message_id_list/*": {Kind: "message_id"},
			},
		),
	},
	{
		key:         "im.message.reaction.created_v1",
		title:       "Reaction added",
		description: "Triggered when a reaction is added to a message",
		scopes:      []string{"im:message:readonly", "im:message.reactions:read"},
		bodyType:    reflect.TypeOf(larkim.P2MessageReactionCreatedV1Data{}),
		fieldOverrides: mergeOv(
			userIDOv("/event/user_id"),
			map[string]schemas.FieldMeta{
				"/event/message_id":  {Kind: "message_id"},
				"/event/action_time": {Kind: "timestamp_ms"},
			},
		),
	},
	{
		key:         "im.message.reaction.deleted_v1",
		title:       "Reaction removed",
		description: "Triggered when a reaction is removed from a message",
		scopes:      []string{"im:message:readonly", "im:message.reactions:read"},
		bodyType:    reflect.TypeOf(larkim.P2MessageReactionDeletedV1Data{}),
		fieldOverrides: mergeOv(
			userIDOv("/event/user_id"),
			map[string]schemas.FieldMeta{
				"/event/message_id":  {Kind: "message_id"},
				"/event/action_time": {Kind: "timestamp_ms"},
			},
		),
	},
	{
		key:         "im.chat.member.bot.added_v1",
		title:       "Bot added to chat",
		description: "Triggered when the bot is added to a chat",
		scopes:      []string{"im:chat.members:bot_access"},
		bodyType:    reflect.TypeOf(larkim.P2ChatMemberBotAddedV1Data{}),
		fieldOverrides: mergeOv(
			userIDOv("/event/operator_id"),
			map[string]schemas.FieldMeta{
				"/event/chat_id": {Kind: "chat_id"},
			},
		),
	},
	{
		key:         "im.chat.member.bot.deleted_v1",
		title:       "Bot removed from chat",
		description: "Triggered after the bot is removed from a chat",
		scopes:      []string{"im:chat.members:bot_access"},
		bodyType:    reflect.TypeOf(larkim.P2ChatMemberBotDeletedV1Data{}),
		fieldOverrides: mergeOv(
			userIDOv("/event/operator_id"),
			map[string]schemas.FieldMeta{
				"/event/chat_id": {Kind: "chat_id"},
			},
		),
	},
	{
		key:         "im.chat.member.user.added_v1",
		title:       "User added to chat",
		description: "Triggered when a new user joins a chat (including topic chats)",
		scopes:      []string{"im:chat.members:read"},
		bodyType:    reflect.TypeOf(larkim.P2ChatMemberUserAddedV1Data{}),
		fieldOverrides: mergeOv(
			userIDOv("/event/operator_id"),
			userIDOv("/event/users/*/user_id"),
			map[string]schemas.FieldMeta{
				"/event/chat_id": {Kind: "chat_id"},
			},
		),
	},
	{
		key:         "im.chat.member.user.withdrawn_v1",
		title:       "User invite withdrawn",
		description: "Triggered after a pending user invite is withdrawn",
		scopes:      []string{"im:chat.members:read"},
		bodyType:    reflect.TypeOf(larkim.P2ChatMemberUserWithdrawnV1Data{}),
		fieldOverrides: mergeOv(
			userIDOv("/event/operator_id"),
			userIDOv("/event/users/*/user_id"),
			map[string]schemas.FieldMeta{
				"/event/chat_id": {Kind: "chat_id"},
			},
		),
	},
	{
		key:         "im.chat.member.user.deleted_v1",
		title:       "User left chat",
		description: "Triggered when a user leaves or is removed from a chat",
		scopes:      []string{"im:chat.members:read"},
		bodyType:    reflect.TypeOf(larkim.P2ChatMemberUserDeletedV1Data{}),
		fieldOverrides: mergeOv(
			userIDOv("/event/operator_id"),
			userIDOv("/event/users/*/user_id"),
			map[string]schemas.FieldMeta{
				"/event/chat_id": {Kind: "chat_id"},
			},
		),
	},
	{
		key:         "im.chat.updated_v1",
		title:       "Chat updated",
		description: "Triggered after chat settings (owner, avatar, name, permissions, etc.) are updated",
		scopes:      []string{"im:chat:read"},
		bodyType:    reflect.TypeOf(larkim.P2ChatUpdatedV1Data{}),
		fieldOverrides: mergeOv(
			userIDOv("/event/operator_id"),
			userIDOv("/event/before_change/owner_id"),
			userIDOv("/event/after_change/owner_id"),
			userIDOv("/event/moderator_list/added_member_list/*/user_id"),
			userIDOv("/event/moderator_list/removed_member_list/*/user_id"),
			map[string]schemas.FieldMeta{
				"/event/chat_id": {Kind: "chat_id"},
			},
		),
	},
	{
		key:         "im.chat.disbanded_v1",
		title:       "Chat disbanded",
		description: "Triggered after a chat is disbanded",
		scopes:      []string{"im:chat:read"},
		bodyType:    reflect.TypeOf(larkim.P2ChatDisbandedV1Data{}),
		fieldOverrides: mergeOv(
			userIDOv("/event/operator_id"),
			map[string]schemas.FieldMeta{
				"/event/chat_id": {Kind: "chat_id"},
			},
		),
	},
}
