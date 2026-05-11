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

	// Step 1: extract user names from mentions (free)
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

	// Collect sender IDs still missing a name.
	// - user senders: resolve via contact API (open_id → name)
	// - bot/app senders: resolve via application API (app_id → app_name)
	seenUsers := make(map[string]bool)
	seenApps := make(map[string]bool)
	var missingUserIDs []string
	var missingAppIDs []string
	for _, msg := range messages {
		sender, ok := msg["sender"].(map[string]interface{})
		if !ok {
			continue
		}
		senderType, _ := sender["sender_type"].(string)
		id, _ := sender["id"].(string)
		if id == "" || nameMap[id] != "" {
			continue
		}

		switch senderType {
		case "user":
			if !strings.HasPrefix(id, "ou_") || seenUsers[id] {
				continue
			}
			seenUsers[id] = true
			missingUserIDs = append(missingUserIDs, id)
		case "app", "bot":
			if seenApps[id] {
				continue
			}
			seenApps[id] = true
			missingAppIDs = append(missingAppIDs, id)
		}
	}

	if len(missingUserIDs) == 0 && len(missingAppIDs) == 0 {
		return nameMap
	}

	// Step 2: batch resolve remaining user senders via contact API.
	// Use basic_batch for user identity (lighter permission requirement),
	// full batch for bot identity.
	if len(missingUserIDs) > 0 {
		if runtime.As().IsBot() {
			batchResolveUsers(runtime, missingUserIDs, nameMap)
		} else {
			batchResolveByBasicContact(runtime, missingUserIDs, nameMap)
		}
	}

	// Step 3: resolve bot/app sender names via application API.
	if len(missingAppIDs) > 0 {
		batchResolveApps(runtime, missingAppIDs, nameMap)
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

func batchResolveApps(runtime *common.RuntimeContext, appIDs []string, nameMap map[string]string) {
	query := larkcore.QueryParams{"lang": []string{"zh_cn"}}
	for _, appID := range appIDs {
		data, err := doAPIJSONAsBotIfPossible(runtime, http.MethodGet, "/open-apis/application/v6/applications/"+url.PathEscape(appID), query, nil)
		if err != nil {
			continue
		}
		app, _ := data["app"].(map[string]any)
		name, _ := app["app_name"].(string)
		if name == "" {
			name, _ = data["app_name"].(string)
		}
		if name != "" {
			nameMap[appID] = name
		}
	}
}

func doAPIJSONAsBotIfPossible(runtime *common.RuntimeContext, method, apiPath string, query larkcore.QueryParams, body any) (map[string]any, error) {
	if runtime == nil {
		return nil, fmt.Errorf("runtime is nil")
	}
	if runtime.Config == nil || !runtime.Config.CanBot() {
		return runtime.DoAPIJSON(method, apiPath, query, body)
	}

	req := &larkcore.ApiReq{
		HttpMethod:  method,
		ApiPath:     apiPath,
		QueryParams: query,
	}
	if body != nil {
		req.Body = body
	}
	resp, err := runtime.DoAPIAsBot(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	if len(resp.RawBody) == 0 {
		return nil, fmt.Errorf("empty response body")
	}
	var envelope struct {
		Code int            `json:"code"`
		Msg  string         `json:"msg"`
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(resp.RawBody, &envelope); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}
	if envelope.Code != 0 {
		return nil, fmt.Errorf("[%d] %s", envelope.Code, envelope.Msg)
	}
	return envelope.Data, nil
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
