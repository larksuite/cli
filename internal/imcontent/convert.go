// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package imcontent converts Lark message content into human-readable text.
package imcontent

import "fmt"

// ContentConverter converts one message type's raw content.
type ContentConverter interface {
	Convert(ctx *ConvertContext) string
}

// ConvertContext contains the data needed by pure message conversion.
type ConvertContext struct {
	RawContent string
	MentionMap map[string]string
	Mentions   []interface{}
}

var converters = map[string]ContentConverter{
	"text":                 textConverter{},
	"post":                 postConverter{},
	"image":                imageConverter{},
	"file":                 fileConverter{},
	"audio":                audioMsgConverter{},
	"video":                videoMsgConverter{},
	"media":                videoMsgConverter{},
	"sticker":              stickerConverter{},
	"interactive":          interactiveConverter{},
	"share_chat":           shareChatConverter{},
	"share_user":           shareUserConverter{},
	"location":             locationConverter{},
	"merge_forward":        mergeForwardConverter{},
	"folder":               folderConverter{},
	"share_calendar_event": calendarEventConverter{},
	"calendar":             calendarInviteConverter{},
	"general_calendar":     generalCalendarConverter{},
	"video_chat":           videoChatConverter{},
	"system":               systemConverter{},
	"todo":                 todoConverter{},
	"vote":                 voteConverter{},
	"hongbao":              hongbaoConverter{},
}

// ConvertBodyContent converts a raw message body to human-readable text.
func ConvertBodyContent(messageType string, ctx *ConvertContext) string {
	if ctx == nil || ctx.RawContent == "" {
		return ""
	}
	if converter, ok := converters[messageType]; ok {
		return converter.Convert(ctx)
	}
	return fmt.Sprintf("[%s]", messageType)
}

type mergeForwardConverter struct{}

func (mergeForwardConverter) Convert(ctx *ConvertContext) string {
	ids := ParseMergeForwardIDs(ctx.RawContent)
	if len(ids) > 0 {
		return fmt.Sprintf("[Merged forward: %d messages]", len(ids))
	}
	return "[Merged forward]"
}

// ParseMergeForwardIDs extracts message IDs from merge_forward content.
func ParseMergeForwardIDs(raw string) []string {
	parsed, err := ParseJSONObject(raw)
	if err != nil {
		return nil
	}
	rawIDs, _ := parsed["create_message_ids"].([]interface{})
	ids := make([]string, 0, len(rawIDs))
	for _, rawID := range rawIDs {
		if id, ok := rawID.(string); ok {
			ids = append(ids, id)
		}
	}
	return ids
}

func extractMentionOpenID(id interface{}) string {
	switch value := id.(type) {
	case string:
		return value
	case map[string]interface{}:
		openID, _ := value["open_id"].(string)
		return openID
	default:
		return ""
	}
}
