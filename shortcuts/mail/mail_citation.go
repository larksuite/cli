// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"github.com/larksuite/cli/internal/citation"
	"github.com/larksuite/cli/shortcuts/common"
)

func buildMailMessageCitation(msg map[string]interface{}) citation.Citation {
	return citation.Citation{
		SourceType:  citation.SourceMail,
		URL:         strVal(msg["applink"]),
		Title:       strVal(msg["subject"]),
		Snippet:     mailCitationSnippet(msg),
		PublishTime: mailCitationPublishTime(msg),
	}
}

func buildMailMessageCitations(messages []map[string]interface{}) []citation.Citation {
	out := make([]citation.Citation, 0, len(messages))
	for _, msg := range messages {
		out = append(out, buildMailMessageCitation(msg))
	}
	return out
}

func mailMessageCitationDefinition() *common.CitationDefinition {
	return &common.CitationDefinition{
		SourceTypes: []citation.SourceType{citation.SourceMail},
		Build: func(data interface{}) []citation.Citation {
			msg, ok := data.(map[string]interface{})
			if !ok {
				return nil
			}
			return []citation.Citation{buildMailMessageCitation(msg)}
		},
	}
}

func mailMessagesCitationDefinition() *common.CitationDefinition {
	return &common.CitationDefinition{
		SourceTypes: []citation.SourceType{citation.SourceMail},
		Build: func(data interface{}) []citation.Citation {
			switch v := data.(type) {
			case mailMessagesOutput:
				return buildMailMessageCitations(v.Messages)
			case *mailMessagesOutput:
				if v == nil {
					return nil
				}
				return buildMailMessageCitations(v.Messages)
			default:
				return nil
			}
		},
	}
}

func mailTriageCitationDefinition() *common.CitationDefinition {
	return &common.CitationDefinition{
		SourceTypes: []citation.SourceType{citation.SourceMail},
		Build: func(data interface{}) []citation.Citation {
			out, ok := data.(map[string]interface{})
			if !ok {
				return nil
			}
			return buildMailMessageCitations(mapSlice(out["messages"]))
		},
	}
}

func mailCitationSnippet(msg map[string]interface{}) string {
	for _, key := range []string{"body_preview", "preview", "summary"} {
		if value := strVal(msg[key]); value != "" {
			return value
		}
	}
	return ""
}

func mailCitationPublishTime(msg map[string]interface{}) string {
	for _, key := range []string{"internal_date", "create_time", "date"} {
		if value := citation.Time(msg[key]); value != "" {
			return value
		}
	}
	return ""
}

func mapSlice(raw interface{}) []map[string]interface{} {
	switch v := raw.(type) {
	case []map[string]interface{}:
		return v
	case []interface{}:
		out := make([]map[string]interface{}, 0, len(v))
		for _, item := range v {
			if msg, ok := item.(map[string]interface{}); ok {
				out = append(out, msg)
			}
		}
		return out
	default:
		return nil
	}
}
