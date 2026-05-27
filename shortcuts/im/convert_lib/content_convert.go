// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package convertlib

import (
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"reflect"
	"strconv"
	"strings"

	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/shortcuts/common"
)

// ContentConverter defines the interface for converting a message type's raw content to human-readable text.
type ContentConverter interface {
	Convert(ctx *ConvertContext) string
}

// ConvertContext holds all context needed for content conversion.
type ConvertContext struct {
	RawContent string
	MentionMap map[string]string
	// MessageID and Runtime are used by merge_forward to fetch and expand sub-messages via API.
	// For other message types these can be zero values.
	MessageID string
	Runtime   *common.RuntimeContext
	// SenderNames is a shared cache of open_id -> display name, accumulated across messages
	// to avoid redundant contact API calls. May be nil.
	SenderNames map[string]string
}

// converters maps message types to their ContentConverter implementations.
var converters map[string]ContentConverter

func init() {
	converters = map[string]ContentConverter{
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
}

// ConvertBodyContent converts body.content (a raw JSON string) to human-readable text.
func ConvertBodyContent(msgType string, ctx *ConvertContext) string {
	if ctx.RawContent == "" {
		return ""
	}
	if c, ok := converters[msgType]; ok {
		return c.Convert(ctx)
	}
	return fmt.Sprintf("[%s]", msgType)
}

// FormatEventMessage converts an event-pushed message to a human-readable map.
// Event messages have a different structure from API responses:
//   - message_type (not msg_type), content is a direct JSON string (not under body.content)
//   - mentions are nested under message.mentions
//
// This is the entry point for im.message.receive_v1 event processors.
func FormatEventMessage(msgType, rawContent, messageID string, mentions []interface{}) map[string]interface{} {
	content := ConvertBodyContent(msgType, &ConvertContext{
		RawContent: rawContent,
		MentionMap: BuildMentionKeyMap(mentions),
		MessageID:  messageID,
	})

	msg := map[string]interface{}{
		"msg_type": msgType,
		"content":  content,
	}

	if len(mentions) > 0 {
		simplified := make([]map[string]interface{}, 0, len(mentions))
		for _, raw := range mentions {
			item, _ := raw.(map[string]interface{})
			key, _ := item["key"].(string)
			name, _ := item["name"].(string)
			simplified = append(simplified, map[string]interface{}{
				"key":  key,
				"id":   extractMentionOpenId(item["id"]),
				"name": name,
			})
		}
		msg["mentions"] = simplified
	}

	return msg
}

// FormatMessageItem converts a raw API message item to a human-readable map.
// senderNames is an optional shared cache (open_id -> name) accumulated across messages;
// pass nil to disable sender name caching.
func FormatMessageItem(m map[string]interface{}, runtime *common.RuntimeContext, senderNames ...map[string]string) map[string]interface{} {
	var nameCache map[string]string
	if len(senderNames) > 0 {
		nameCache = senderNames[0]
	}
	msgType, _ := m["msg_type"].(string)
	messageId, _ := m["message_id"].(string)
	mentions, _ := m["mentions"].([]interface{})
	deleted, _ := m["deleted"].(bool)
	updated, _ := m["updated"].(bool)

	content := ""
	if body, ok := m["body"].(map[string]interface{}); ok {
		rawContent, _ := body["content"].(string)
		content = ConvertBodyContent(msgType, &ConvertContext{
			RawContent:  rawContent,
			MentionMap:  BuildMentionKeyMap(mentions),
			MessageID:   messageId,
			Runtime:     runtime,
			SenderNames: nameCache,
		})
	}

	msg := map[string]interface{}{
		"message_id":  messageId,
		"msg_type":    msgType,
		"content":     content,
		"sender":      m["sender"],
		"create_time": common.FormatTime(m["create_time"]),
		"deleted":     deleted,
		"updated":     updated,
	}

	// thread_id takes priority; fall back to reply_to (parent_id) if no thread
	if tid, _ := m["thread_id"].(string); tid != "" {
		msg["thread_id"] = tid
	} else if pid, _ := m["parent_id"].(string); pid != "" {
		msg["reply_to"] = pid
	}

	// Preserve API-provided fields (even if this formatter doesn't otherwise use them).
	// update_time is only meaningful when the message was actually edited;
	// the server echoes update_time == create_time for unedited messages, which
	// would otherwise make every output look "updated" to downstream consumers.
	if updated {
		if v, ok := m["update_time"]; ok && v != nil {
			if s, isStr := v.(string); isStr {
				if strings.TrimSpace(s) != "" {
					msg["update_time"] = common.FormatTime(s)
				}
			} else {
				msg["update_time"] = common.FormatTime(v)
			}
		}
	}
	if v, ok := m["chat_id"]; ok {
		msg["chat_id"] = v
	}
	if v, ok := m["message_position"]; ok {
		msg["message_position"] = v
	}
	if v, ok := m["thread_message_position"]; ok {
		msg["thread_message_position"] = v
	}

	// Prefer API-provided message_app_link when it's a non-empty string; otherwise assemble deterministically.
	appLink, _ := m["message_app_link"].(string)
	appLink = strings.TrimSpace(appLink)
	if appLink == "" && runtime != nil && runtime.Config != nil {
		appLink = assembleMessageAppLink(m, runtime.Config.Brand)
	}
	if appLink != "" {
		msg["message_app_link"] = appLink
	}

	if len(mentions) > 0 {
		simplified := make([]map[string]interface{}, 0, len(mentions))
		for _, raw := range mentions {
			item, _ := raw.(map[string]interface{})
			key, _ := item["key"].(string)
			name, _ := item["name"].(string)
			simplified = append(simplified, map[string]interface{}{
				"key":  key,
				"id":   extractMentionOpenId(item["id"]),
				"name": name,
			})
		}
		msg["mentions"] = simplified
	}

	return msg
}

func assembleMessageAppLink(m map[string]interface{}, brand core.LarkBrand) string {
	domain := resolveAppLinkDomain(brand)
	if domain == "" {
		return ""
	}

	chatID, _ := m["chat_id"].(string)
	threadID, _ := m["thread_id"].(string)
	msgPos, okMsgPos := normalizeMessagePosition(m["message_position"])
	threadPos, okThreadPos := normalizeMessagePosition(m["thread_message_position"])

	// Thread app link requires both thread_id and chat_id.
	// Emit both underscore-less (openthreadid/openchatid) and snake_case (open_thread_id/open_chat_id)
	// query keys so PC and mobile clients can both resolve the link.
	if threadID != "" && chatID != "" && okThreadPos {
		u := &url.URL{Scheme: "https", Host: domain, Path: "/client/thread/open"}
		q := url.Values{}
		q.Set("openthreadid", threadID)
		q.Set("openchatid", chatID)
		q.Set("open_thread_id", threadID)
		q.Set("open_chat_id", chatID)
		q.Set("thread_position", threadPos)
		u.RawQuery = q.Encode()
		return u.String()
	}
	if chatID != "" && okMsgPos {
		u := &url.URL{Scheme: "https", Host: domain, Path: "/client/chat/open"}
		q := url.Values{}
		q.Set("openChatId", chatID)
		q.Set("position", msgPos)
		u.RawQuery = q.Encode()
		return u.String()
	}
	return ""
}

func normalizeMessagePosition(v interface{}) (string, bool) {
	if v == nil {
		return "", false
	}
	switch vv := v.(type) {
	case float32:
		f := float64(vv)
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return "", false
		}
		if math.Trunc(f) == f {
			return strconv.FormatInt(int64(f), 10), true
		}
		return strconv.FormatFloat(f, 'f', -1, 64), true
	case float64:
		if math.IsNaN(vv) || math.IsInf(vv, 0) {
			return "", false
		}
		if math.Trunc(vv) == vv {
			return strconv.FormatInt(int64(vv), 10), true
		}
		return strconv.FormatFloat(vv, 'f', -1, 64), true
	case int:
		return strconv.Itoa(vv), true
	case int8:
		return strconv.FormatInt(int64(vv), 10), true
	case int16:
		return strconv.FormatInt(int64(vv), 10), true
	case int32:
		return strconv.FormatInt(int64(vv), 10), true
	case int64:
		return strconv.FormatInt(vv, 10), true
	case uint:
		return strconv.FormatUint(uint64(vv), 10), true
	case uint8:
		return strconv.FormatUint(uint64(vv), 10), true
	case uint16:
		return strconv.FormatUint(uint64(vv), 10), true
	case uint32:
		return strconv.FormatUint(uint64(vv), 10), true
	case uint64:
		return strconv.FormatUint(vv, 10), true
	case uintptr:
		return strconv.FormatUint(uint64(vv), 10), true
	case json.Number:
		s := strings.TrimSpace(vv.String())
		if s == "" {
			return "", false
		}
		f, err := strconv.ParseFloat(s, 64)
		if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
			return "", false
		}
		if math.Trunc(f) == f {
			return strconv.FormatInt(int64(f), 10), true
		}
		return strconv.FormatFloat(f, 'f', -1, 64), true
	case string:
		s := strings.TrimSpace(vv)
		if s == "" {
			return "", false
		}
		f, err := strconv.ParseFloat(s, 64)
		if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
			return "", false
		}
		if math.Trunc(f) == f {
			return strconv.FormatInt(int64(f), 10), true
		}
		return strconv.FormatFloat(f, 'f', -1, 64), true
	default:
		// Fallback for typed numeric values (e.g. int32/uint64 via struct -> interface{}), pointers, etc.
		rv := reflect.ValueOf(v)
		for rv.Kind() == reflect.Ptr {
			if rv.IsNil() {
				return "", false
			}
			rv = rv.Elem()
		}
		switch rv.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			return strconv.FormatInt(rv.Int(), 10), true
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
			return strconv.FormatUint(rv.Uint(), 10), true
		case reflect.Float32, reflect.Float64:
			f := rv.Float()
			if math.IsNaN(f) || math.IsInf(f, 0) {
				return "", false
			}
			if math.Trunc(f) == f {
				return strconv.FormatInt(int64(f), 10), true
			}
			return strconv.FormatFloat(f, 'f', -1, 64), true
		default:
			return "", false
		}
	}
}

func resolveAppLinkDomain(brand core.LarkBrand) string {
	appLink := core.ResolveEndpoints(brand).AppLink
	u, err := url.Parse(appLink)
	if err != nil {
		return ""
	}
	return u.Host
}

// extractMentionOpenId extracts open_id from mention id (string or {"open_id":...} object).
func extractMentionOpenId(id interface{}) string {
	if s, ok := id.(string); ok {
		return s
	}
	if m, ok := id.(map[string]interface{}); ok {
		if openId, ok := m["open_id"].(string); ok {
			return openId
		}
	}
	return ""
}

// TruncateContent truncates a string for table display.
func TruncateContent(s string, max int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max]) + "…"
}
