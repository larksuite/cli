// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
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

type conciseChatSection struct {
	ChatID        string
	ChatName      string
	ChatType      string
	ChatPartnerID string
	ThreadID      string
	Messages      []map[string]interface{}
}

type conciseMessageViewType string

const (
	conciseMessageViewChat   conciseMessageViewType = "chat"
	conciseMessageViewThread conciseMessageViewType = "thread"
)

type conciseMessageView struct {
	Type         conciseMessageViewType
	Title        string
	ChatSections []conciseChatSection
	HasMore      bool
	NextToken    string
}

type conciseParticipant struct {
	ID        string
	Name      string
	Type      string
	OpenBotID string
}

type conciseMessageStats struct {
	Messages int
	Replies  int
	Threads  map[string]struct{}
}

func validateConciseOutputFlags(runtime *common.RuntimeContext) error {
	if !runtime.Bool("concise") {
		return nil
	}

	conflict := ""
	switch {
	case runtime.Bool("json"):
		conflict = "--json"
	case runtime.Str("jq") != "":
		conflict = "--jq"
	case runtime.Changed("format"):
		conflict = "--format"
	}
	if conflict == "" {
		return nil
	}
	return errs.NewValidationError(errs.SubtypeInvalidArgument,
		"--concise cannot be used with %s", conflict).
		WithParam("--concise").
		WithParams(
			errs.InvalidParam{Name: "--concise", Reason: "mutually exclusive with " + conflict},
			errs.InvalidParam{Name: conflict, Reason: "mutually exclusive with --concise"},
		)
}

// outputMessagesConcise keeps the command-specific Markdown path inside IM.
// Generic formats continue to use RuntimeContext.OutFormat unchanged.
func outputMessagesConcise(runtime *common.RuntimeContext, view conciseMessageView) error {
	streams := runtime.IO()
	scanResult := output.ScanForSafety(runtime.Cmd.CommandPath(), view, streams.ErrOut)
	if scanResult.Blocked {
		return scanResult.BlockErr
	}
	if scanResult.Alert != nil {
		if err := output.WriteAlertWarning(streams.ErrOut, scanResult.Alert); err != nil {
			return errs.NewInternalError(errs.SubtypeUnknown, "failed to write concise output warning").WithCause(err)
		}
	}

	var rendered bytes.Buffer
	if err := renderMessagesConcise(&rendered, view); err != nil {
		return errs.NewInternalError(errs.SubtypeUnknown, "failed to render concise output").WithCause(err)
	}
	if _, err := io.Copy(streams.Out, &rendered); err != nil {
		return errs.NewInternalError(errs.SubtypeUnknown, "failed to write concise output").WithCause(err)
	}
	return nil
}

func renderMessagesConcise(w io.Writer, view conciseMessageView) error {
	var b strings.Builder
	title := view.Title
	if title == "" {
		title = "Messages"
	}
	fmt.Fprintf(&b, "# %s\n", conciseInline(title))

	if len(view.ChatSections) == 1 {
		renderConciseSectionMetadata(&b, view.ChatSections[0], true)
	}

	participants := collectConciseParticipants(view.ChatSections)
	if len(participants) > 0 {
		b.WriteString("\n## Participants\n\n")
		for _, participant := range participants {
			fmt.Fprintf(&b, "- %s (%s", conciseInline(participant.Name), conciseCode(participant.ID))
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
	for _, section := range view.ChatSections {
		stats.Messages += len(section.Messages)
		if section.ThreadID != "" {
			stats.Threads[section.ThreadID] = struct{}{}
		}
	}
	if stats.Messages == 0 {
		b.WriteString("\nNo messages found.\n")
	} else {
		multipleSections := len(view.ChatSections) > 1
		for index, section := range view.ChatSections {
			if multipleSections {
				fmt.Fprintf(&b, "\n### %s\n", conciseSectionTitle(section, index))
				renderConciseSectionMetadata(&b, section, false)
			}
			if len(section.Messages) == 0 {
				if multipleSections {
					b.WriteString("\nNo messages found in this section.\n")
				} else {
					b.WriteString("\nNo messages found.\n")
				}
				continue
			}
			for _, message := range section.Messages {
				b.WriteByte('\n')
				renderConciseMessage(&b, message, "", false, &stats)
			}
		}
	}

	b.WriteString("\n## Summary\n")
	fmt.Fprintf(&b, "\n- messages: %d\n", stats.Messages)
	if view.Type == conciseMessageViewChat {
		fmt.Fprintf(&b, "- thread_replies: %d\n", stats.Replies)
		fmt.Fprintf(&b, "- threads: %d\n", len(stats.Threads))
		fmt.Fprintf(&b, "- chats: %d\n", conciseChatCount(view.ChatSections))
	}
	fmt.Fprintf(&b, "- has_more: %t\n", view.HasMore)
	if view.HasMore && view.NextToken != "" {
		fmt.Fprintf(&b, "- next_token: %s\n", conciseCode(view.NextToken))
	}

	_, err := io.WriteString(w, b.String())
	return err
}

func renderConciseSectionMetadata(b *strings.Builder, section conciseChatSection, includeName bool) {
	if section.ChatID != "" {
		fmt.Fprintf(b, "\n- chat_id: %s\n", conciseCode(section.ChatID))
	}
	if includeName && section.ChatName != "" {
		fmt.Fprintf(b, "- chat_name: %s\n", conciseInline(section.ChatName))
	}
	if section.ChatType != "" {
		fmt.Fprintf(b, "- chat_type: %s\n", conciseCode(section.ChatType))
	}
	if section.ChatPartnerID != "" {
		fmt.Fprintf(b, "- chat_partner: %s\n", conciseCode(section.ChatPartnerID))
	}
	if section.ThreadID != "" {
		fmt.Fprintf(b, "- thread_id: %s\n", conciseCode(section.ThreadID))
	}
}

func conciseSectionTitle(section conciseChatSection, index int) string {
	if section.ChatName != "" {
		return "Chat: " + conciseInline(section.ChatName)
	}
	if section.ChatType == "p2p" {
		return "Chat: P2P"
	}
	if section.ChatID != "" {
		return "Chat: " + conciseCode(section.ChatID)
	}
	if section.ThreadID != "" {
		return "Thread: " + conciseCode(section.ThreadID)
	}
	return fmt.Sprintf("Section %d", index+1)
}

func conciseChatCount(sections []conciseChatSection) int {
	chatIDs := make(map[string]struct{})
	for _, section := range sections {
		if section.ChatID != "" {
			chatIDs[section.ChatID] = struct{}{}
		}
	}
	return len(chatIDs)
}

func renderConciseMessage(
	b *strings.Builder,
	message map[string]interface{},
	indent string,
	reply bool,
	stats *conciseMessageStats,
) {
	parts := make([]string, 0, 8)
	if reply {
		parts = append(parts, "**Reply**")
	}
	if created := conciseString(message["create_time"]); created != "" {
		parts = append(parts, conciseCode(created))
	}
	parts = append(parts, conciseSender(message))
	if msgType := conciseString(message["msg_type"]); msgType != "" {
		parts = append(parts, conciseCode(msgType))
	}
	if messageID := conciseString(message["message_id"]); messageID != "" {
		parts = append(parts, "message_id: "+conciseCode(messageID))
	}
	if replyTo := conciseString(message["reply_to"]); replyTo != "" {
		parts = append(parts, "reply_to: "+conciseCode(replyTo))
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

	content := conciseString(message["content"])
	if conciseBool(message["deleted"]) {
		content = "[deleted]"
	} else if strings.TrimSpace(content) == "" {
		content = "[no content]"
	}
	writeConciseQuote(b, indent+"  ", content)

	localPaths, resourceFailures := conciseResourceSummary(message)
	if len(localPaths) > 0 {
		fmt.Fprintf(b, "%s  resources: %s\n", indent, strings.Join(localPaths, ", "))
	}
	if resourceFailures > 0 {
		fmt.Fprintf(b, "%s  resource_failures: %d\n", indent, resourceFailures)
	}
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
		fmt.Fprintf(b, "%s  thread_has_more: true (thread replies incomplete)\n", indent)
	}
	if conciseBool(message["thread_replies_error"]) {
		fmt.Fprintf(b, "%s  thread_replies_error: true (thread replies unavailable)\n", indent)
	}
}

func writeConciseQuote(b *strings.Builder, indent, content string) {
	content = validate.SanitizeForTerminal(content)
	for _, line := range strings.Split(content, "\n") {
		fmt.Fprintf(b, "%s> %s\n", indent, line)
	}
}

func collectConciseParticipants(sections []conciseChatSection) []conciseParticipant {
	var participants []conciseParticipant
	seen := make(map[string]struct{})
	var visit func([]map[string]interface{}, string)
	visit = func(items []map[string]interface{}, parentID string) {
		for _, message := range items {
			messageID := conciseString(message["message_id"])
			if parentID != "" && messageID == parentID {
				continue
			}
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
			visit(conciseMessageSlice(message["thread_replies"]), messageID)
		}
	}
	for _, section := range sections {
		visit(section.Messages, "")
	}
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
	return summary
}

func conciseResourceSummary(message map[string]interface{}) ([]string, int) {
	resources := conciseInterfaceSlice(message["resources"])
	localPaths := make([]string, 0, len(resources))
	failures := 0
	for _, raw := range resources {
		resource, _ := raw.(map[string]interface{})
		if localPath := conciseString(resource["local_path"]); localPath != "" {
			localPaths = append(localPaths, conciseCode(localPath))
		}
		if resourceError, exists := resource["error"]; exists {
			switch value := resourceError.(type) {
			case bool:
				if value {
					failures++
				}
			case string:
				if strings.TrimSpace(value) != "" {
					failures++
				}
			case nil:
			default:
				failures++
			}
		}
	}
	return localPaths, failures
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

func conciseSender(message map[string]interface{}) string {
	sender, _ := message["sender"].(map[string]interface{})
	name := conciseString(sender["name"])
	id := conciseString(sender["id"])
	if name == "" {
		name = id
	}
	if name == "" {
		return "**unknown_sender**"
	}
	rendered := "**" + conciseInline(name) + "**"
	if id != "" {
		rendered += " (" + conciseCode(id) + ")"
	}
	return rendered
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
