// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package convertlib

import (
	"github.com/larksuite/cli/internal/imcontent"
	"github.com/larksuite/cli/shortcuts/common"
)

// BuildMentionKeyMap builds a key→name lookup from the message "mentions" array.
// It stays as the IM shortcut layer's entry point because shortcuts/event builds
// a ConvertContext through this package and should not have to reach past it.
//
// The other pure helpers are called as imcontent.X directly: ResolveMentionKeys,
// FormatTimestamp and ExtractPostBlocksText had no caller here but a test once
// the converters moved down, and forwarding ParseJSONObject only gave one
// function two entry points.
func BuildMentionKeyMap(mentions []interface{}) map[string]string {
	return imcontent.BuildMentionKeyMap(mentions)
}

// pickSenderName returns the server-provided display name from a message sender:
// the plain `sender_name` (the server's default-locale name). Callers wanting a
// specific locale should read the full `sender_i18n_names` map, which is preserved
// on the sender. Returns "" when the server supplied no name, so the caller can
// fall back to the raw id.
func pickSenderName(sender map[string]interface{}) string {
	name, _ := sender["sender_name"].(string)
	return name
}

// ResolveSenderNames harvests the server-provided sender_name for each message
// sender into the shared cache (keyed by sender id), so a sender appearing across
// the render tree (e.g. merge_forward sub-items, thread replies) resolves once.
// The message read API is the single source of truth for names (opt in via
// with_sender_name=true); there is NO contact/mention fallback — a sender the
// server did not name resolves to its id downstream. Pass an empty map if none exists.
func ResolveSenderNames(_ *common.RuntimeContext, messages []map[string]interface{}, cache map[string]string) map[string]string {
	nameMap := cache
	if nameMap == nil {
		nameMap = make(map[string]string)
	}
	for _, msg := range messages {
		sender, ok := msg["sender"].(map[string]interface{})
		if !ok {
			continue
		}
		id, _ := sender["id"].(string)
		if id == "" {
			continue
		}
		if name := pickSenderName(sender); name != "" {
			nameMap[id] = name
		}
	}
	return nameMap
}

// AttachSenderNames enriches message sender objects with a single resolved display
// name in `name`, taken from the server-provided sender_name (via the sender itself
// or the shared cache). Senders the server did not name keep no `name` (id is
// preserved for downstream id fallback) — there is no contact/mention lookup.
//
// The raw `sender_name` is stripped from the output because it exactly duplicates
// `name`; `sender_i18n_names` (the full i18n set, all locales) and `open_bot_id`
// are preserved for consumers that need a specific locale or the id alignment.
func AttachSenderNames(messages []map[string]interface{}, nameMap map[string]string) {
	for _, msg := range messages {
		sender, ok := msg["sender"].(map[string]interface{})
		if !ok {
			continue
		}
		if name := pickSenderName(sender); name != "" {
			sender["name"] = name
		} else if id, _ := sender["id"].(string); id != "" {
			if name, ok := nameMap[id]; ok {
				sender["name"] = name
			}
		}
		// sender_name exactly duplicates `name`; drop it. Keep sender_i18n_names + open_bot_id.
		delete(sender, "sender_name")
	}
}
