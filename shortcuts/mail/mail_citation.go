// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"strings"

	"github.com/larksuite/cli/internal/citation"
	"github.com/larksuite/cli/shortcuts/common"
)

func mailMessageCitations(_ *common.RuntimeContext, data any) []citation.Citation {
	message, ok := data.(map[string]interface{})
	if !ok {
		return nil
	}
	return []citation.Citation{mailCitationFromMessage(message, false)}
}

func mailTriageCitations(runtime *common.RuntimeContext, data any) []citation.Citation {
	out, ok := data.(map[string]interface{})
	if !ok {
		return nil
	}
	search := mailTriageCitationUsesSearch(runtime)
	messages := mailCitationMessages(out["messages"])
	items := make([]citation.Citation, 0, len(messages))
	for _, message := range messages {
		items = append(items, mailCitationFromMessage(message, search))
	}
	return items
}

func mailTriageCitationUsesSearch(runtime *common.RuntimeContext) bool {
	if runtime == nil {
		return false
	}
	query := strings.TrimSpace(runtime.Str("query"))
	filter, err := buildTriageFilter(runtime)
	if err != nil {
		return query != ""
	}
	page, err := parseTriagePageToken(runtime.Str("page-token"))
	if err != nil {
		return query != ""
	}
	search, err := resolveTriagePath(page, query, filter)
	return err == nil && search
}

func mailCitationMessages(value any) []map[string]interface{} {
	switch messages := value.(type) {
	case []map[string]interface{}:
		return messages
	case []interface{}:
		result := make([]map[string]interface{}, 0, len(messages))
		for _, value := range messages {
			if message, ok := value.(map[string]interface{}); ok {
				result = append(result, message)
			}
		}
		return result
	default:
		return nil
	}
}

func mailCitationFromMessage(message map[string]interface{}, search bool) citation.Citation {
	title := common.GetString(message, "subject")
	internalDate, hasInternalDate := message["internal_date"]
	publishTime := common.GetStringLoose(message, "internal_date")
	if search {
		title = firstMailCitationString(message, "title", "subject")
		if !hasInternalDate || mailCitationTimeMissing(internalDate) {
			publishTime = common.GetStringLoose(message, "create_time")
		}
	}
	return citation.Citation{
		SourceType:  citation.SourceMail,
		URL:         common.GetString(message, "applink"),
		Title:       title,
		PublishTime: citation.Time(publishTime),
	}
}

func mailCitationTimeMissing(value any) bool {
	if value == nil {
		return true
	}
	text, ok := value.(string)
	return ok && strings.TrimSpace(text) == ""
}

func firstMailCitationString(message map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if value := common.GetString(message, key); value != "" {
			return value
		}
	}
	return ""
}

// copyTriageCitationFields preserves only fields already present in the API
// payload. This keeps citation construction a pure projection of final output.
func copyTriageCitationFields(dst, src map[string]interface{}, keys ...string) {
	for _, key := range keys {
		if value, ok := src[key]; ok {
			dst[key] = value
		}
	}
}
