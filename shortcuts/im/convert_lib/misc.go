// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package convertlib

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/larksuite/cli/shortcuts/common"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
)

type stickerConverter struct{}

func (stickerConverter) Convert(_ *ConvertContext) string { return "[Sticker]" }

type videoChatConverter struct{}

func (videoChatConverter) Convert(_ *ConvertContext) string { return "[Video call]" }

type shareChatConverter struct{}

func (shareChatConverter) Convert(ctx *ConvertContext) string {
	parsed, err := ParseJSONObject(ctx.RawContent)
	if err != nil {
		return invalidJSONPlaceholder("chat card")
	}
	if id, _ := parsed["chat_id"].(string); id != "" {
		return fmt.Sprintf("[Chat card: %s]", id)
	}
	return "[Chat card]"
}

// systemPlaceholderRe matches {word} tokens in system message templates.
var systemPlaceholderRe = regexp.MustCompile(`\{(\w+)}`)

type shareUserConverter struct{}

// Convert converts a share_chat message content JSON to human-readable string.
func (shareUserConverter) Convert(ctx *ConvertContext) string {
	parsed, err := ParseJSONObject(ctx.RawContent)
	if err != nil {
		return invalidJSONPlaceholder("user card")
	}
	if id, _ := parsed["user_id"].(string); id != "" {
		return fmt.Sprintf("[User card: %s]", id)
	}
	return "[User card]"
}

type locationConverter struct{}

func (locationConverter) Convert(ctx *ConvertContext) string {
	parsed, err := ParseJSONObject(ctx.RawContent)
	if err != nil {
		return invalidJSONPlaceholder("location")
	}
	if name, _ := parsed["name"].(string); name != "" {
		return fmt.Sprintf("[Location: %s]", name)
	}
	return "[Location]"
}

type folderConverter struct{}

func (folderConverter) Convert(ctx *ConvertContext) string {
	parsed, err := ParseJSONObject(ctx.RawContent)
	if err != nil {
		return invalidJSONPlaceholder("folder")
	}
	key, _ := parsed["file_key"].(string)
	if key == "" {
		return "[Folder]"
	}
	name, _ := parsed["file_name"].(string)

	// 展开一层：调 openapi children（recursive=false），输出第一层 + children_count + 深层提示
	// 需要 Runtime + MessageID（srctype=message&srcid=MessageID）；不可用时降级为旧输出
	if ctx.Runtime != nil && ctx.MessageID != "" {
		if tree := fetchFolderChildrenTree(ctx.Runtime, key, name, ctx.MessageID); tree != "" {
			return tree
		}
	}
	if name != "" {
		return fmt.Sprintf(`<folder key="%s" name="%s"/>`, cardEscapeAttr(key), cardEscapeAttr(name))
	}
	return fmt.Sprintf(`<folder key="%s"/>`, cardEscapeAttr(key))
}

// fetchFolderChildrenTree 调 openapi 展开文件夹一层，返回树形文本（含 children_count 深层提示）。
// 失败时返回空串，由调用方降级为旧输出。
func fetchFolderChildrenTree(runtime *common.RuntimeContext, folderKey, folderName, messageID string) string {
	data, err := runtime.DoAPIJSONTyped(http.MethodGet, "/open-apis/im/v1/files/"+folderKey+"/folder",
		larkcore.QueryParams{
			"srctype":   []string{"message"},
			"srcid":     []string{messageID},
			"recursive": []string{"false"},
		}, nil)
	if err != nil || data == nil {
		return ""
	}
	rawItems, _ := data["items"].([]interface{})
	if len(rawItems) == 0 {
		return ""
	}
	// 只展开一层：file 用 <file name key/>；子文件夹用 <folder name key child_count/>（不递归，child_count 提示深层）
	// 根 folder 带 child_count（=all_count 子项总数）+ has_more（items 数 < all_count 时标注还有更多未展示）
	hasMore := false
	var allCount int64
	if v, ok := data["all_count"]; ok {
		allCount = numToInt64(v)
		if allCount > int64(len(rawItems)) {
			hasMore = true
		}
	}
	var b strings.Builder
	b.WriteString(`<folder name="` + cardEscapeAttr(folderName) + `" key="` + cardEscapeAttr(folderKey) + `"`)
	if allCount > 0 {
		fmt.Fprintf(&b, ` child_count="%d"`, allCount)
	}
	if hasMore {
		b.WriteString(` has_more="true"`)
	}
	b.WriteString(`>`)
	for _, raw := range rawItems {
		item, _ := raw.(map[string]interface{})
		k, _ := item["file_key"].(string)
		n, _ := item["name"].(string)
		isFolder, _ := item["is_folder"].(bool)
		if isFolder {
			cc := numToInt64(item["children_count"])
			fmt.Fprintf(&b, `<folder name="%s" key="%s" child_count="%d"/>`,
				cardEscapeAttr(n), cardEscapeAttr(k), cc)
		} else {
			fmt.Fprintf(&b, `<file name="%s" key="%s"/>`, cardEscapeAttr(n), cardEscapeAttr(k))
		}
	}
	b.WriteString("</folder>")
	return b.String()
}


// numToInt64 兼容 JSON number（json.Number）/ float64 / int 的类型转换。
func numToInt64(v interface{}) int64 {
	switch n := v.(type) {
	case json.Number:
		if i, err := n.Int64(); err == nil {
			return i
		}
	case float64:
		return int64(n)
	case float32:
		return int64(n)
	case int:
		return int64(n)
	case int64:
		return n
	}
	return 0
}

type calendarEventConverter struct{}

// Convert converts a share_calendar_event message content JSON to human-readable string.
// Includes open_calendar_id and open_event_id as XML attributes so agents can look up the event.
func (calendarEventConverter) Convert(ctx *ConvertContext) string {
	parsed, err := ParseJSONObject(ctx.RawContent)
	if err != nil {
		return invalidJSONPlaceholder("calendar")
	}
	calendarID, _ := parsed["open_calendar_id"].(string)
	eventID, _ := parsed["open_event_id"].(string)
	var attrs string
	if calendarID != "" {
		attrs += fmt.Sprintf(` open_calendar_id="%s"`, cardEscapeAttr(calendarID))
	}
	if eventID != "" {
		attrs += fmt.Sprintf(` open_event_id="%s"`, cardEscapeAttr(eventID))
	}
	attrs += calendarShareTokenAttr(parsed)
	return formatCalendarContent(parsed, "calendar_share", attrs)
}

type calendarInviteConverter struct{}

// Convert converts a calendar message content JSON to human-readable string.
func (calendarInviteConverter) Convert(ctx *ConvertContext) string {
	parsed, err := ParseJSONObject(ctx.RawContent)
	if err != nil {
		return invalidJSONPlaceholder("calendar")
	}
	return formatCalendarContent(parsed, "calendar_invite", "")
}

type generalCalendarConverter struct{}

func (generalCalendarConverter) Convert(ctx *ConvertContext) string {
	parsed, err := ParseJSONObject(ctx.RawContent)
	if err != nil {
		return invalidJSONPlaceholder("calendar")
	}
	return formatCalendarContent(parsed, "calendar", calendarShareTokenAttr(parsed))
}

func calendarShareTokenAttr(parsed map[string]interface{}) string {
	shareToken, _ := parsed["share_token"].(string)
	if shareToken == "" {
		return ""
	}
	return fmt.Sprintf(` share_token="%s"`, cardEscapeAttr(shareToken))
}

// formatCalendarContent builds a human-readable string from a calendar JSON object.
// Expected fields: summary (string), start_time (epoch string), end_time (epoch string).
// extraAttrs is an optional string of XML attributes (e.g. ` open_event_id="xxx"`) appended to the opening tag.
func formatCalendarContent(parsed map[string]interface{}, tag, extraAttrs string) string {
	summary, _ := parsed["summary"].(string)
	startTime, _ := parsed["start_time"].(string)
	endTime, _ := parsed["end_time"].(string)

	var inner []string
	if summary != "" {
		inner = append(inner, summary)
	}

	start := formatTimestamp(startTime)
	end := formatTimestamp(endTime)
	if start != "" && end != "" {
		inner = append(inner, start+" ~ "+end)
	} else if start != "" {
		inner = append(inner, start)
	}

	body := strings.Join(inner, "\n")
	if body == "" {
		body = tag
	}
	return fmt.Sprintf("<%s%s>\n%s\n</%s>", tag, extraAttrs, xmlEscapeBody(body), tag)
}

type voteConverter struct{}

func (voteConverter) Convert(ctx *ConvertContext) string {
	parsed, err := ParseJSONObject(ctx.RawContent)
	if err != nil {
		return invalidJSONPlaceholder("vote")
	}
	topic, _ := parsed["topic"].(string)

	var inner []string
	if topic != "" {
		inner = append(inner, topic)
	}
	if opts, ok := parsed["options"].([]interface{}); ok {
		for _, o := range opts {
			if s, ok := o.(string); ok && s != "" {
				inner = append(inner, "• "+s)
			}
		}
	}
	// status: 0 = open, non-zero = closed (based on internal VoteStatus enum)
	if status, ok := parsed["status"].(float64); ok && status != 0 {
		inner = append(inner, "(Closed)")
	}

	body := strings.Join(inner, "\n")
	if body == "" {
		body = "vote"
	}
	return fmt.Sprintf("<vote>\n%s\n</vote>", xmlEscapeBody(body))
}

type hongbaoConverter struct{}

func (hongbaoConverter) Convert(ctx *ConvertContext) string {
	parsed, err := ParseJSONObject(ctx.RawContent)
	if err != nil {
		return invalidJSONPlaceholder("hongbao")
	}
	if text, _ := parsed["text"].(string); text != "" {
		return fmt.Sprintf(`<hongbao text=%q/>`, text)
	}
	return "<hongbao/>"
}

type todoConverter struct{}

func (todoConverter) Convert(ctx *ConvertContext) string {
	parsed, err := ParseJSONObject(ctx.RawContent)
	if err != nil {
		return invalidJSONPlaceholder("todo")
	}

	taskID, _ := parsed["task_id"].(string)
	var taskAttr string
	if taskID != "" {
		taskAttr = fmt.Sprintf(` task_id="%s"`, cardEscapeAttr(taskID))
	}

	var inner []string
	if summary, ok := parsed["summary"].(map[string]interface{}); ok {
		if title, _ := summary["title"].(string); title != "" {
			inner = append(inner, title)
		}
		if blocks, ok := summary["content"].([]interface{}); ok {
			if text := extractPostBlocksText(blocks); text != "" {
				inner = append(inner, text)
			}
		}
	}
	if dueTime, _ := parsed["due_time"].(string); dueTime != "" {
		if formatted := formatTimestamp(dueTime); formatted != "" {
			inner = append(inner, "Due: "+formatted)
		}
	}

	body := strings.Join(inner, "\n")
	if body == "" {
		body = "todo"
	}
	return fmt.Sprintf("<todo%s>\n%s\n</todo>", taskAttr, xmlEscapeBody(body))
}

type systemConverter struct{}

func (systemConverter) Convert(ctx *ConvertContext) string {
	parsed, err := ParseJSONObject(ctx.RawContent)
	if err != nil {
		return invalidJSONPlaceholder("system message")
	}

	tmpl, _ := parsed["template"].(string)
	if tmpl == "" {
		return "[System message]"
	}

	content := tmpl

	if fromUsers, ok := parsed["from_user"].([]interface{}); ok {
		var names []string
		for _, u := range fromUsers {
			if s, ok := u.(string); ok && s != "" {
				names = append(names, s)
			}
		}
		content = strings.ReplaceAll(content, "{from_user}", strings.Join(names, ", "))
	} else {
		content = strings.ReplaceAll(content, "{from_user}", "")
	}

	if toChatters, ok := parsed["to_chatters"].([]interface{}); ok {
		var names []string
		for _, u := range toChatters {
			if s, ok := u.(string); ok && s != "" {
				names = append(names, s)
			}
		}
		content = strings.ReplaceAll(content, "{to_chatters}", strings.Join(names, ", "))
	} else {
		content = strings.ReplaceAll(content, "{to_chatters}", "")
	}

	if divider, ok := parsed["divider_text"].(map[string]interface{}); ok {
		text, _ := divider["text"].(string)
		content = strings.ReplaceAll(content, "{divider_text}", text)
	} else {
		content = strings.ReplaceAll(content, "{divider_text}", "")
	}

	// Generic pass: replace any remaining {key} placeholders with matching
	// string-typed fields in the JSON (e.g. {name}, {operator}).
	content = systemPlaceholderRe.ReplaceAllStringFunc(content, func(match string) string {
		key := match[1 : len(match)-1]
		if val, _ := parsed[key].(string); val != "" {
			return val
		}
		return match // preserve unknown placeholders intact
	})

	return strings.TrimSpace(content)
}
