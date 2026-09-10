// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"fmt"
	"io"
	"net/url"
	"sort"
	"strings"
	"unicode"

	"github.com/larksuite/cli/internal/validate"
)

var conciseInlineReplacer = strings.NewReplacer(
	`\`, `\\`,
	"`", `\`+"`",
	`*`, `\*`,
	`_`, `\_`,
	`[`, `\[`,
	`]`, `\]`,
	`<`, `\<`,
	`>`, `\>`,
	`#`, `\#`,
	`~`, `\~`,
)

type conciseMessageView struct {
	Title     string
	ChatID    string
	ThreadID  string
	Messages  []map[string]interface{}
	HasMore   bool
	NextToken string
}

type conciseParticipant struct {
	ID        string
	Name      string
	Type      string
	OpenBotID string
}

type conciseMessageStats struct {
	Replies int
	Threads map[string]struct{}
}

func renderMessagesConcise(w io.Writer, view conciseMessageView) error {
	var b strings.Builder
	title := view.Title
	if title == "" {
		title = "Messages"
	}
	fmt.Fprintf(&b, "# %s\n", conciseInline(title))
	if view.ChatID != "" {
		fmt.Fprintf(&b, "\n- chat_id: %s\n", conciseCode(view.ChatID))
	}
	if view.ThreadID != "" {
		fmt.Fprintf(&b, "\n- thread_id: %s\n", conciseCode(view.ThreadID))
	}

	participants := collectConciseParticipants(view.Messages)
	if len(participants) > 0 {
		b.WriteString("\n## Participants\n")
		for _, participant := range participants {
			fmt.Fprintf(&b, "\n- %s (%s", conciseInline(participant.Name), conciseCode(participant.ID))
			if participant.Type != "" {
				fmt.Fprintf(&b, ", %s", conciseInline(participant.Type))
			}
			if participant.OpenBotID != "" {
				fmt.Fprintf(&b, ", bot_open_id: %s", conciseCode(participant.OpenBotID))
			}
			b.WriteString(")\n")
		}
	}

	b.WriteString("\n## Messages\n")
	stats := conciseMessageStats{Threads: map[string]struct{}{}}
	if len(view.Messages) == 0 {
		b.WriteString("\nNo messages found.\n")
	} else {
		for _, message := range view.Messages {
			b.WriteByte('\n')
			renderConciseMessage(&b, message, "", false, &stats)
		}
	}

	b.WriteString("\n## Summary\n")
	fmt.Fprintf(&b, "\n- messages: %d\n", len(view.Messages))
	if view.ChatID != "" {
		fmt.Fprintf(&b, "- thread_replies: %d\n", stats.Replies)
		fmt.Fprintf(&b, "- threads: %d\n", len(stats.Threads))
	}
	fmt.Fprintf(&b, "- has_more: %t\n", view.HasMore)
	if view.NextToken != "" {
		fmt.Fprintf(&b, "- next_token: %s\n", conciseCode(view.NextToken))
	}

	_, err := io.WriteString(w, b.String())
	return err
}

func renderConciseMessage(
	b *strings.Builder,
	message map[string]interface{},
	indent string,
	reply bool,
	stats *conciseMessageStats,
) {
	parts := make([]string, 0, 6)
	if reply {
		parts = append(parts, "**Reply**")
	}
	if created := conciseString(message["create_time"]); created != "" {
		parts = append(parts, conciseCode(created))
	}
	parts = append(parts, "**"+conciseInline(conciseSenderName(message))+"**")
	if msgType := conciseString(message["msg_type"]); msgType != "" {
		parts = append(parts, conciseCode(msgType))
	}
	if messageID := conciseString(message["message_id"]); messageID != "" {
		parts = append(parts, "message_id: "+conciseCode(messageID))
	}
	if threadID := conciseString(message["thread_id"]); threadID != "" {
		parts = append(parts, "thread_id: "+conciseCode(threadID))
		stats.Threads[threadID] = struct{}{}
	}
	if conciseBool(message["updated"]) {
		parts = append(parts, "edited")
	}
	if conciseBool(message["deleted"]) {
		parts = append(parts, "deleted")
	}
	fmt.Fprintf(b, "%s- %s\n", indent, strings.Join(parts, " · "))
	if appLink := conciseString(message["message_app_link"]); appLink != "" {
		fmt.Fprintf(b, "%s  app_link: %s\n", indent, conciseLink(appLink))
	}

	content := conciseString(message["content"])
	if conciseBool(message["deleted"]) {
		content = "[deleted]"
	} else if strings.TrimSpace(content) == "" {
		content = "[no content]"
	}
	writeConciseQuote(b, indent+"  ", content)

	if reactions := conciseReactionSummary(message); len(reactions) > 0 {
		fmt.Fprintf(b, "%s  reactions: %s\n", indent, strings.Join(reactions, ", "))
	}
	if conciseBool(message["reactions_error"]) {
		fmt.Fprintf(b, "%s  reactions: unavailable\n", indent)
	}

	parentID := conciseString(message["message_id"])
	replies := conciseMessageSlice(message["thread_replies"])
	renderedReplies := 0
	for _, child := range replies {
		if parentID != "" && conciseString(child["message_id"]) == parentID {
			continue
		}
		if renderedReplies == 0 {
			fmt.Fprintf(b, "%s  replies:\n", indent)
		}
		renderConciseMessage(b, child, indent+"  ", true, stats)
		renderedReplies++
		stats.Replies++
	}
	if conciseBool(message["thread_has_more"]) {
		fmt.Fprintf(b, "%s  thread_replies: incomplete\n", indent)
	}
	if conciseBool(message["thread_replies_error"]) {
		fmt.Fprintf(b, "%s  thread_replies: unavailable\n", indent)
	}
}

func writeConciseQuote(b *strings.Builder, indent, content string) {
	content = validate.SanitizeForTerminal(content)
	for _, line := range strings.Split(content, "\n") {
		fmt.Fprintf(b, "%s> %s\n", indent, line)
	}
}

func collectConciseParticipants(messages []map[string]interface{}) []conciseParticipant {
	var participants []conciseParticipant
	seen := make(map[string]struct{})
	var visit func([]map[string]interface{})
	visit = func(items []map[string]interface{}) {
		for _, message := range items {
			sender, _ := message["sender"].(map[string]interface{})
			id := conciseString(sender["id"])
			if id != "" {
				if _, exists := seen[id]; !exists {
					seen[id] = struct{}{}
					name := conciseString(sender["name"])
					if name == "" {
						name = id
					}
					participants = append(participants, conciseParticipant{
						ID:        id,
						Name:      name,
						Type:      conciseString(sender["sender_type"]),
						OpenBotID: conciseString(sender["open_bot_id"]),
					})
				}
			}
			visit(conciseMessageSlice(message["thread_replies"]))
		}
	}
	visit(messages)
	return participants
}

func conciseReactionSummary(message map[string]interface{}) []string {
	reactions, _ := message["reactions"].(map[string]interface{})
	counts := conciseInterfaceSlice(reactions["counts"])
	summary := make([]string, 0, len(counts))
	for _, raw := range counts {
		count, _ := raw.(map[string]interface{})
		reactionType := conciseString(count["reaction_type"])
		if reactionType == "" {
			continue
		}
		summary = append(summary, conciseCode(fmt.Sprintf("%s x%v", reactionType, count["count"])))
	}
	sort.Strings(summary)
	return summary
}

func conciseMessageSlice(value interface{}) []map[string]interface{} {
	switch items := value.(type) {
	case []map[string]interface{}:
		return items
	case []interface{}:
		out := make([]map[string]interface{}, 0, len(items))
		for _, item := range items {
			if message, ok := item.(map[string]interface{}); ok {
				out = append(out, message)
			}
		}
		return out
	default:
		return nil
	}
}

func conciseInterfaceSlice(value interface{}) []interface{} {
	switch items := value.(type) {
	case []interface{}:
		return items
	case []map[string]interface{}:
		out := make([]interface{}, len(items))
		for i := range items {
			out[i] = items[i]
		}
		return out
	default:
		return nil
	}
}

func conciseSenderName(message map[string]interface{}) string {
	sender, _ := message["sender"].(map[string]interface{})
	if name := conciseString(sender["name"]); name != "" {
		return name
	}
	if id := conciseString(sender["id"]); id != "" {
		return id
	}
	return "system"
}

func conciseInline(value string) string {
	return conciseInlineReplacer.Replace(conciseSingleLine(value))
}

// conciseCode renders untrusted metadata as a single Markdown code span. The
// fence is longer than any backtick run in the value, so opaque IDs and tokens
// cannot terminate the span and inject new Markdown structure.
func conciseCode(value string) string {
	value = conciseSingleLine(value)
	maxRun := 0
	currentRun := 0
	for _, r := range value {
		if r == '`' {
			currentRun++
			maxRun = max(maxRun, currentRun)
		} else {
			currentRun = 0
		}
	}
	fence := strings.Repeat("`", maxRun+1)
	if value == "" {
		return fence + " " + fence
	}
	if strings.HasPrefix(value, "`") || strings.HasSuffix(value, "`") {
		return fence + " " + value + " " + fence
	}
	return fence + value + fence
}

// conciseLink keeps normal HTTP(S) app links clickable. Values containing
// Markdown delimiters, whitespace, or an invalid/unsafe URL are shown as an
// inert code span instead of being allowed to escape an autolink.
func conciseLink(value string) string {
	value = validate.SanitizeForTerminal(value)
	if conciseSafeAutolink(value) {
		return "<" + value + ">"
	}
	return conciseCode(value)
}

func conciseSafeAutolink(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if unicode.IsSpace(r) || r < 0x21 || r > 0x7e || strings.ContainsRune("<>`\\\"'", r) {
			return false
		}
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" {
		return false
	}
	return parsed.Scheme == "https" || parsed.Scheme == "http"
}

func conciseSingleLine(value string) string {
	value = validate.SanitizeForTerminal(value)
	return strings.Join(strings.Fields(value), " ")
}

func conciseString(value interface{}) string {
	text, _ := value.(string)
	return text
}

func conciseBool(value interface{}) bool {
	boolean, _ := value.(bool)
	return boolean
}
