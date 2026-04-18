// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package convertlib

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/larksuite/cli/shortcuts/common"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
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

// ResolveSenderNames batch-resolves sender open_ids to display names.
// The cache map is used to share already-resolved IDs across calls; newly resolved
// names are written back into it. Pass an empty map if no prior cache exists.
//
// Step 1: extract names from message mentions (free, no API call).
// Step 2: for remaining unresolved IDs, call contact batch API (requires contact:user.base:readonly).
// Silently returns partial results on API error.
//
// [#22] Changed from variadic `cache ...map[string]string` to a required parameter.
// The variadic form was misleading: every caller passed exactly one map, and the function
// body both modified it and returned it, making the dual semantics confusing.
func ResolveSenderNames(runtime *common.RuntimeContext, messages []map[string]interface{}, cache map[string]string) map[string]string {
	nameMap := cache
	if nameMap == nil {
		nameMap = make(map[string]string)
	}

	// Step 1: extract names from mentions (free)
	for _, msg := range messages {
		switch mentions := msg["mentions"].(type) {
		case []interface{}:
			for _, raw := range mentions {
				m, _ := raw.(map[string]interface{})
				id, _ := m["id"].(string)
				name, _ := m["name"].(string)
				if id != "" && name != "" && strings.HasPrefix(id, "ou_") {
					nameMap[id] = name
				}
			}
		case []map[string]interface{}:
			// Backward-compatible path for tests/callers that construct typed slices.
			for _, m := range mentions {
				id, _ := m["id"].(string)
				name, _ := m["name"].(string)
				if id != "" && name != "" && strings.HasPrefix(id, "ou_") {
					nameMap[id] = name
				}
			}
		}
	}

	// Collect sender IDs still missing a name
	seen := make(map[string]bool)
	var missingIDs []string
	for _, msg := range messages {
		sender, ok := msg["sender"].(map[string]interface{})
		if !ok {
			continue
		}
		senderType, _ := sender["sender_type"].(string)
		if senderType != "user" {
			continue
		}
		id, _ := sender["id"].(string)
		if id == "" || !strings.HasPrefix(id, "ou_") || seen[id] || nameMap[id] != "" {
			continue
		}
		seen[id] = true
		missingIDs = append(missingIDs, id)
	}
	if len(missingIDs) == 0 {
		return nameMap
	}

	// Step 2: batch resolve remaining via contact API.
	// Use basic_batch for user identity (lighter permission requirement),
	// full batch for bot identity.
	if runtime.As().IsBot() {
		batchResolveUsers(runtime, missingIDs, nameMap)
	} else {
		batchResolveByBasicContact(runtime, missingIDs, nameMap)
	}

	return nameMap
}

// batchResolveByBasicContact resolves user names via POST /contact/v3/users/basic_batch.
// This API has lighter permission requirements and works with user identity
// even when the target user is not in the app's visible range.
// Response uses "users" (not "items") and "user_id" (not "open_id").
// The basic_batch endpoint caps user_ids at 10 per request.
func batchResolveByBasicContact(runtime *common.RuntimeContext, missingIDs []string, nameMap map[string]string) {
	const batchSize = 10
	for i := 0; i < len(missingIDs); i += batchSize {
		end := i + batchSize
		if end > len(missingIDs) {
			end = len(missingIDs)
		}
		batch := missingIDs[i:end]

		data, err := runtime.DoAPIJSON(http.MethodPost,
			"/open-apis/contact/v3/users/basic_batch",
			larkcore.QueryParams{"user_id_type": []string{"open_id"}},
			map[string]interface{}{"user_ids": batch},
		)
		if err != nil {
			break
		}

		users, _ := data["users"].([]interface{})
		for _, item := range users {
			user, _ := item.(map[string]interface{})
			userID, _ := user["user_id"].(string)
			name, _ := user["name"].(string)
			if userID != "" && name != "" {
				nameMap[userID] = name
			}
		}
	}
}

func batchResolveUsers(runtime *common.RuntimeContext, missingIDs []string, nameMap map[string]string) {
	const batchSize = 50
	for i := 0; i < len(missingIDs); i += batchSize {
		end := i + batchSize
		if end > len(missingIDs) {
			end = len(missingIDs)
		}
		batch := missingIDs[i:end]

		parts := []string{"user_id_type=open_id"}
		for _, uid := range batch {
			parts = append(parts, "user_ids="+url.QueryEscape(uid))
		}
		apiURL := "/open-apis/contact/v3/users/batch?" + strings.Join(parts, "&")

		data, err := runtime.DoAPIJSON(http.MethodGet, apiURL, nil, nil)
		if err != nil {
			break
		}

		items, _ := data["items"].([]interface{})
		for _, item := range items {
			user, _ := item.(map[string]interface{})
			openID, _ := user["open_id"].(string)
			name, _ := user["name"].(string)
			if openID != "" && name != "" {
				nameMap[openID] = name
			}
		}
	}
}

// AttachSenderNames enriches message sender objects with resolved display names.
// Senders whose name could not be resolved are left unchanged (id is preserved).
func AttachSenderNames(messages []map[string]interface{}, nameMap map[string]string) {
	for _, msg := range messages {
		sender, ok := msg["sender"].(map[string]interface{})
		if !ok {
			continue
		}
		id, _ := sender["id"].(string)
		if name, ok := nameMap[id]; ok {
			sender["name"] = name
		}
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
