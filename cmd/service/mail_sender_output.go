// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package service

import "strings"

func serviceSuccessTransformer(schemaPath string) func(interface{}) interface{} {
	if !isMailSenderSchema(schemaPath) {
		return nil
	}
	return func(result interface{}) interface{} {
		switch {
		case strings.HasSuffix(schemaPath, ".list"):
			return projectMailSenderList(result)
		case strings.HasSuffix(schemaPath, ".batch_create"), strings.HasSuffix(schemaPath, ".batch_remove"):
			return projectMailSenderBatch(result)
		default:
			return result
		}
	}
}

func isMailSenderSchema(schemaPath string) bool {
	return strings.HasPrefix(schemaPath, "mail.user_mailbox.allow_senders.") ||
		strings.HasPrefix(schemaPath, "mail.user_mailbox.blocked_senders.")
}

func projectMailSenderList(result interface{}) interface{} {
	data, ok := responseData(result)
	if !ok {
		return result
	}
	out := map[string]interface{}{}
	if items, ok := data["items"].([]interface{}); ok {
		projected := make([]interface{}, 0, len(items))
		for _, item := range items {
			if m, ok := item.(map[string]interface{}); ok {
				projected = append(projected, map[string]interface{}{"address": m["address"]})
			}
		}
		out["items"] = projected
	}
	if token, ok := data["next_page_token"]; ok {
		out["next_page_token"] = token
	}
	return successResponseWithData(result, out)
}

func projectMailSenderBatch(result interface{}) interface{} {
	data, ok := responseData(result)
	if !ok {
		return result
	}
	out := map[string]interface{}{}
	if v, ok := data["submitted_count"]; ok {
		out["submitted_count"] = v
	}
	if v, ok := data["deduplicated_count"]; ok {
		out["deduplicated_count"] = v
	}
	return successResponseWithData(result, out)
}

func responseData(result interface{}) (map[string]interface{}, bool) {
	m, ok := result.(map[string]interface{})
	if !ok {
		return nil, false
	}
	data, ok := m["data"].(map[string]interface{})
	return data, ok
}

func successResponseWithData(result interface{}, data map[string]interface{}) interface{} {
	m, ok := result.(map[string]interface{})
	if !ok {
		return result
	}
	out := make(map[string]interface{}, len(m))
	for k, v := range m {
		out[k] = v
	}
	out["data"] = data
	return out
}
