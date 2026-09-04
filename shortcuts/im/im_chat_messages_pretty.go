// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
)

const chatMessagePrettyDivider = "────────────────────────"

// renderChatMessagesPretty renders the already-projected message data as a
// readable conversation transcript. It deliberately consumes only the public
// shortcut projection: OpenAPI transport fields stay available in JSON but do
// not leak into the human view.
func renderChatMessagesPretty(w io.Writer, messages []map[string]interface{}, hasMore bool, pageToken string) {
	if len(messages) == 0 {
		fmt.Fprintln(w, "No messages in this time range.")
		fmt.Fprintln(w)
		writeChatMessagesPrettyFooter(w, 0, 0, hasMore, pageToken)
		return
	}

	visibleMessages := make([]map[string]interface{}, 0, len(messages))
	seenMessageIDs := make(map[string]struct{}, len(messages))
	for _, msg := range messages {
		id, _ := msg["message_id"].(string)
		if id != "" {
			if _, duplicate := seenMessageIDs[id]; duplicate {
				continue
			}
			seenMessageIDs[id] = struct{}{}
		}
		visibleMessages = append(visibleMessages, msg)
	}

	threadReplyCount := 0
	for i, msg := range visibleMessages {
		if i > 0 {
			fmt.Fprintln(w)
		}
		renderChatMessagePretty(w, msg)
		threadReplyCount += renderChatMessageRepliesPretty(w, msg)
		fmt.Fprintln(w, chatMessagePrettyDivider)
	}

	fmt.Fprintln(w)
	writeChatMessagesPrettyFooter(w, len(visibleMessages), threadReplyCount, hasMore, pageToken)
}

func renderChatMessagePretty(w io.Writer, msg map[string]interface{}) {
	createTime := prettyMessageTime(msg)
	sender := prettyMessageSender(msg)
	fmt.Fprintf(w, "%s · %s\n", createTime, sender)
	writePrettyQuotedContent(w, prettyMessageContent(msg), "")

	metadata := prettyMessageMetadata(msg)
	if len(metadata) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, strings.Join(metadata, " · "))
	}
	if reactions := prettyReactionSummary(msg); reactions != "" {
		fmt.Fprintf(w, "表情：%s\n", reactions)
	}
}

func renderChatMessageRepliesPretty(w io.Writer, root map[string]interface{}) int {
	rootID, _ := root["message_id"].(string)
	threadID, _ := root["thread_id"].(string)
	rawReplies, repliesPresent := root["thread_replies"]
	replies := prettyMessageMaps(rawReplies)

	seen := make(map[string]struct{}, len(replies)+1)
	if rootID != "" {
		seen[rootID] = struct{}{}
	}
	visible := make([]map[string]interface{}, 0, len(replies))
	for _, reply := range replies {
		id, _ := reply["message_id"].(string)
		if id != "" {
			if _, duplicate := seen[id]; duplicate {
				continue
			}
			seen[id] = struct{}{}
		}
		visible = append(visible, reply)
	}

	for _, reply := range visible {
		fmt.Fprintln(w)
		fmt.Fprintf(w, "  ↳ %s · %s\n", prettyReplyTime(prettyMessageTime(root), prettyMessageTime(reply)), prettyMessageSender(reply))
		writePrettyQuotedContent(w, prettyMessageContent(reply), "    ")
		if reactions := prettyReactionSummary(reply); reactions != "" {
			fmt.Fprintf(w, "    表情：%s\n", reactions)
		}
	}

	threadRepliesError, _ := root["thread_replies_error"].(bool)
	threadHasMore, _ := root["thread_has_more"].(bool)
	if threadRepliesError {
		fmt.Fprintln(w)
		fmt.Fprintf(w, "  ↳ 话题回复获取失败 · thread: %s\n", prettyID(threadID))
	} else if threadID != "" && !repliesPresent {
		// A successful thread fetch includes at least its root message. An
		// absent field therefore normally means the cross-thread expansion
		// budget was exhausted; keep the limitation visible in pretty output.
		fmt.Fprintln(w)
		fmt.Fprintf(w, "  ↳ 话题回复未展开 · thread: %s\n", threadID)
	}
	if threadHasMore {
		fmt.Fprintln(w)
		fmt.Fprintf(w, "  ↳ 还有更多话题回复 · thread: %s\n", prettyID(threadID))
	}

	return len(visible)
}

func prettyMessageMetadata(msg map[string]interface{}) []string {
	parts := make([]string, 0, 3)
	if id, _ := msg["message_id"].(string); id != "" {
		parts = append(parts, "message: "+id)
	}
	if threadID, _ := msg["thread_id"].(string); threadID != "" {
		parts = append(parts, "thread: "+threadID)
	}
	if replyTo, _ := msg["reply_to"].(string); replyTo != "" {
		parts = append(parts, "reply_to: "+replyTo)
	}
	return parts
}

func prettyMessageSender(msg map[string]interface{}) string {
	if sender, ok := msg["sender"].(map[string]interface{}); ok {
		if display := senderDisplay(sender); display != "" {
			return display
		}
		if senderType, _ := sender["sender_type"].(string); senderType != "" {
			return senderType
		}
	}
	if msgType, _ := msg["msg_type"].(string); msgType == "system" {
		return "系统"
	}
	return "未知发送者"
}

func prettyMessageTime(msg map[string]interface{}) string {
	if createTime, _ := msg["create_time"].(string); createTime != "" {
		return createTime
	}
	return "未知时间"
}

func prettyReplyTime(rootTime, replyTime string) string {
	rootDate, rootClock, rootOK := strings.Cut(rootTime, " ")
	replyDate, replyClock, replyOK := strings.Cut(replyTime, " ")
	if rootOK && replyOK && rootDate == replyDate && rootClock != "" && replyClock != "" {
		return replyClock
	}
	return replyTime
}

func prettyMessageContent(msg map[string]interface{}) string {
	if deleted, _ := msg["deleted"].(bool); deleted {
		return "已撤回"
	}
	if content, _ := msg["content"].(string); content != "" {
		return content
	}
	if msgType, _ := msg["msg_type"].(string); msgType != "" {
		if msgType == "text" || msgType == "post" {
			return "[空消息]"
		}
		return "[" + msgType + "]"
	}
	return "[unknown message]"
}

func writePrettyQuotedContent(w io.Writer, content, indent string) {
	fmt.Fprintf(w, "%s> %s\n", indent, escapePrettyContent(content))
}

func escapePrettyContent(content string) string {
	return strings.NewReplacer(
		"\\", "\\\\",
		"\r", "\\r",
		"\n", "\\n",
		"\t", "\\t",
	).Replace(content)
}

func prettyReactionSummary(msg map[string]interface{}) string {
	reactions, ok := msg["reactions"].(map[string]interface{})
	if !ok {
		return ""
	}

	var summaries []string
	for _, count := range prettyMessageMaps(reactions["counts"]) {
		reactionType, _ := count["reaction_type"].(string)
		formattedCount, ok := prettyPositiveCount(count["count"])
		if reactionType == "" || !ok {
			continue
		}
		summaries = append(summaries, reactionType+"×"+formattedCount)
	}
	return strings.Join(summaries, "、")
}

func prettyPositiveCount(raw interface{}) (string, bool) {
	value, err := strconv.ParseFloat(strings.TrimSpace(fmt.Sprint(raw)), 64)
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) || value <= 0 {
		return "", false
	}
	if math.Trunc(value) == value {
		return strconv.FormatInt(int64(value), 10), true
	}
	return strconv.FormatFloat(value, 'f', -1, 64), true
}

func prettyMessageMaps(raw interface{}) []map[string]interface{} {
	switch values := raw.(type) {
	case []map[string]interface{}:
		return values
	case []interface{}:
		result := make([]map[string]interface{}, 0, len(values))
		for _, value := range values {
			if item, ok := value.(map[string]interface{}); ok {
				result = append(result, item)
			}
		}
		return result
	default:
		return nil
	}
}

func writeChatMessagesPrettyFooter(w io.Writer, messages, replies int, hasMore bool, pageToken string) {
	if pageToken == "" {
		pageToken = "-"
	}
	fmt.Fprintf(w, "%d messages · %d thread replies · has_more: %t · page_token: %s\n", messages, replies, hasMore, pageToken)
}

func prettyID(id string) string {
	if id == "" {
		return "-"
	}
	return id
}
