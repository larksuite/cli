// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package convertlib

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/larksuite/cli/shortcuts/common"
)

// ParseJSONObject parses a raw JSON string into a map.
func ParseJSONObject(raw string) (map[string]interface{}, error) {
	var v map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return nil, err
	}
	return v, nil
}

func invalidJSONPlaceholder(kind string) string {
	if kind == "" {
		return "[Invalid JSON content]"
	}
	return fmt.Sprintf("[Invalid %s JSON]", kind)
}

// BuildMentionKeyMap builds a key→name lookup from the message "mentions" array.
func BuildMentionKeyMap(mentions []interface{}) map[string]string {
	m := map[string]string{}
	for _, raw := range mentions {
		item, _ := raw.(map[string]interface{})
		key, _ := item["key"].(string)
		name, _ := item["name"].(string)
		if key != "" && name != "" {
			m[key] = name
		}
	}
	return m
}

// ResolveMentionKeys replaces mention keys in text with @name format.
func ResolveMentionKeys(text string, mentionMap map[string]string) string {
	for key, name := range mentionMap {
		text = strings.ReplaceAll(text, key, "@"+name)
	}
	return text
}

// formatTimestamp converts a Unix timestamp string (seconds or milliseconds) to
// "YYYY-MM-DD HH:mm" local time. Values with fewer than 10 digits are treated as
// seconds; larger values are treated as milliseconds.
// Returns empty string if the input is empty or unparseable.
func formatTimestamp(ts string) string {
	if ts == "" {
		return ""
	}
	n, err := strconv.ParseInt(ts, 10, 64)
	if err != nil || n == 0 {
		return ""
	}
	if len(strings.TrimLeft(ts, "+-")) >= 13 { // milliseconds timestamps are typically 13+ digits
		n /= 1000
	}
	return time.Unix(n, 0).Local().Format("2006-01-02 15:04:05")
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

// xmlEscapeBody escapes XML special characters for use in element body content.
var xmlBodyEscaper = strings.NewReplacer(
	"&", "&amp;",
	"<", "&lt;",
	">", "&gt;",
)

func xmlEscapeBody(s string) string {
	return xmlBodyEscaper.Replace(s)
}

// escapeMDLinkText escapes square brackets in Markdown link text to prevent link injection.
func escapeMDLinkText(s string) string {
	s = strings.ReplaceAll(s, `[`, `\[`)
	s = strings.ReplaceAll(s, `]`, `\]`)
	return s
}

// extractPostBlocksText extracts plain text from post-style content blocks ([][]element).
func extractPostBlocksText(blocks []interface{}) string {
	var lines []string
	for _, para := range blocks {
		elems, _ := para.([]interface{})
		var sb strings.Builder
		for _, el := range elems {
			elem, _ := el.(map[string]interface{})
			sb.WriteString(renderPostElem(elem))
		}
		if s := sb.String(); s != "" {
			lines = append(lines, s)
		}
	}
	return strings.Join(lines, "\n")
}
