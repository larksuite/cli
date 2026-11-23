// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package event

// ── Shared helpers for IM event processors ──────────────────────────────────
// These functions are used across multiple processor files to extract common
// fields from Lark event payloads (operator_id, user_ids, base compact fields).

// openID extracts open_id from a nested {"open_id":"ou_xxx"} structure.
// Lark events represent user IDs as objects; this unwraps the outer layer.
func openID(v interface{}) string {
	m, ok := v.(map[string]interface{})
	if !ok {
		return ""
	}
	s, _ := m["open_id"].(string)
	return s
}

// extractUserIDs extracts open_ids from a users array:
// [{"user_id":{"open_id":"ou_xxx"},"name":"..."},...]
func extractUserIDs(users []interface{}) []string {
	var ids []string
	for _, u := range users {
		um, ok := u.(map[string]interface{})
		if !ok {
			continue
		}
		if id := openID(um["user_id"]); id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

// stringField safely extracts a string value from a map.
func stringField(m map[string]interface{}, key string) string {
	v, _ := m[key].(string)
	return v
}

// mentionOpenID extracts open_id from a mention id field (nested object or plain string).
func mentionOpenID(raw interface{}) string {
	switch v := raw.(type) {
	case map[string]interface{}:
		openID, _ := v["open_id"].(string)
		return openID
	case string:
		return v
	default:
		return ""
	}
}

// compactMentions converts the raw mentions array into a compact form with key, id, name.
func compactMentions(mentions []interface{}) []map[string]interface{} {
	if len(mentions) == 0 {
		return nil
	}
	out := make([]map[string]interface{}, 0, len(mentions))
	for _, raw := range mentions {
		item, _ := raw.(map[string]interface{})
		m := map[string]interface{}{}
		if k := stringField(item, "key"); k != "" {
			m["key"] = k
		}
		if id := mentionOpenID(item["id"]); id != "" {
			m["id"] = id
		}
		if n := stringField(item, "name"); n != "" {
			m["name"] = n
		}
		if len(m) > 0 {
			out = append(out, m)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// compactBase builds the common compact output fields shared by all IM event processors.
// Every compact output includes: type (event_type), event_id, and timestamp (header create_time).
func compactBase(raw *RawEvent) map[string]interface{} {
	out := map[string]interface{}{
		"type": raw.Header.EventType,
	}
	if raw.Header.EventID != "" {
		out["event_id"] = raw.Header.EventID
	}
	if raw.Header.CreateTime != "" {
		out["timestamp"] = raw.Header.CreateTime
	}
	return out
}
