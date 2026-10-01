// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package convertlib

import (
	"encoding/json"
	"strings"
)

// Upstream sync_to_chat_info.type values, named after the API Explorer
// definition (project=im, version=v1, resource=message, apiName=get).
//
// The upstream constant names describe the role of the *current* message while
// the upstream prose describes the message on the *other* side, which reads as
// a contradiction. Production data confirms the constant-name reading, so this
// is the only place in the CLI that needs to know the numbers: callers use the
// accessors below and never see the enum.
const (
	syncToChatTargetMessage = 1 // current message is the chat-level copy
	syncToChatSourceMessage = 2 // current message is the original thread reply
)

// SyncToChatRelation is the decoded upstream sync_to_chat_info object. It is an
// internal decoding shape only: output surfaces expose the flat, self-describing
// fields produced by the accessors below, never this struct.
type SyncToChatRelation struct {
	Type             int    `json:"type"`
	ThreadID         string `json:"thread_id,omitempty"`
	RelatedMessageID string `json:"related_message_id"`
}

// SyncedFromThreadReply returns the original thread reply's message ID when the
// current message is the chat-level copy, and "" otherwise.
func (r *SyncToChatRelation) SyncedFromThreadReply() string {
	if r == nil || r.Type != syncToChatTargetMessage {
		return ""
	}
	return r.RelatedMessageID
}

// SyncedFromThread returns the thread the original reply lives in, set together
// with SyncedFromThreadReply. Upstream marks thread_id optional, so this can be
// "" even on the chat-level copy.
func (r *SyncToChatRelation) SyncedFromThread() string {
	if r == nil || r.Type != syncToChatTargetMessage {
		return ""
	}
	return r.ThreadID
}

// SyncedToChatMessage returns the chat-level copy's message ID when the current
// message is the original thread reply, and "" otherwise.
func (r *SyncToChatRelation) SyncedToChatMessage() string {
	if r == nil || r.Type != syncToChatSourceMessage {
		return ""
	}
	return r.RelatedMessageID
}

// DecodeSyncToChatRelation projects a relation without letting malformed
// auxiliary data fail the containing message or event. Unknown fields are ignored.
//
// A present-but-unusable relation (an unrecognised type, a non-integer type, a
// missing related_message_id) is dropped whole and silently: output then looks
// exactly like "this message is not synced". That is a deliberate choice, not an
// oversight. Signalling it would mean either a new output field with no
// consumer, or a stderr warning that only the message-API path can emit — the
// event processors get no IO streams — which would leave the two paths behaving
// differently. Agents cannot act on a relation kind they do not know either way.
// TestFormatMessageItemOmitsUnusableSyncToChatInfo and its event-side
// equivalents pin this behaviour, so changing it has to be deliberate.
//
// Revisit if upstream ships a third type that agents must distinguish from
// "not synced"; the shape to add then is an explicit marker, not a silent one.
func DecodeSyncToChatRelation(data []byte) *SyncToChatRelation {
	var relation SyncToChatRelation
	if err := json.Unmarshal(data, &relation); err != nil {
		return nil
	}
	if (relation.Type != syncToChatTargetMessage && relation.Type != syncToChatSourceMessage) ||
		strings.TrimSpace(relation.RelatedMessageID) == "" {
		return nil
	}
	relation.RelatedMessageID = strings.TrimSpace(relation.RelatedMessageID)
	relation.ThreadID = strings.TrimSpace(relation.ThreadID)
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

// ApplySyncToChatFields writes the flat sync-to-chat fields onto dst, which is a
// formatted message or a compact event map. Absent relation, or a relation whose
// side carries no ID, leaves dst untouched — absence means "not synced", never
// "fetch failed".
func ApplySyncToChatFields(dst map[string]interface{}, relation *SyncToChatRelation) {
	if dst == nil || relation == nil {
		return
	}
	if reply := relation.SyncedFromThreadReply(); reply != "" {
		dst["synced_from_thread_reply"] = reply
		if thread := relation.SyncedFromThread(); thread != "" {
			dst["synced_from_thread"] = thread
		}
	}
	if copyID := relation.SyncedToChatMessage(); copyID != "" {
		dst["synced_to_chat_message"] = copyID
	}
}
