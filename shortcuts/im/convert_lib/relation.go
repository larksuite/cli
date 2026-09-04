// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package convertlib

import (
	"encoding/json"
	"strings"
)

// SyncToChatRelation links a source thread reply with the message synchronized to chat.
type SyncToChatRelation struct {
	Type             int    `json:"type" desc:"Role of this message: 1 (SYNC_TO_CHAT_TARGET_MESSAGE) means this message is the chat-level copy; 2 (SYNC_TO_CHAT_SOURCE_MESSAGE) means this message is the original thread reply"`
	ThreadID         string `json:"thread_id,omitempty" desc:"Root thread ID of the original reply; normally present when this message is the chat-level copy"`
	RelatedMessageID string `json:"related_message_id" desc:"Message ID on the other side: the original thread reply for type 1, or the chat-level copy for type 2" kind:"message_id"`
}

// DecodeSyncToChatRelation projects a relation without letting malformed
// auxiliary data fail the containing message or event. Unknown fields are ignored.
func DecodeSyncToChatRelation(data []byte) *SyncToChatRelation {
	var relation SyncToChatRelation
	if err := json.Unmarshal(data, &relation); err != nil {
		return nil
	}
	if (relation.Type != 1 && relation.Type != 2) || strings.TrimSpace(relation.RelatedMessageID) == "" {
		return nil
	}
	return &relation
}

// ProjectSyncToChatRelation converts a loose API value into the typed relation.
func ProjectSyncToChatRelation(value interface{}) *SyncToChatRelation {
	if value == nil {
		return nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	return DecodeSyncToChatRelation(data)
}
