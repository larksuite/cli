// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"context"
	"fmt"
	"sort"
	"strconv"

	"github.com/larksuite/cli/shortcuts/common"
)

// mailThreadOutput is the +thread JSON output: the thread identifier,
// the number of messages in it, and the messages themselves in
// chronological order.
type mailThreadOutput struct {
	ThreadID     string                   `json:"thread_id"`
	MessageCount int                      `json:"message_count"`
	Messages     []map[string]interface{} `json:"messages"`
}

// sortThreadMessagesByInternalDate filters out messages without a message_id
// and orders the rest ascending by internal_date (parsed via
// parseInternalDateMillis). Used to give +thread output a stable
// chronological order regardless of API return order.
func sortThreadMessagesByInternalDate(outs []map[string]interface{}) []map[string]interface{} {
	messages := make([]map[string]interface{}, 0, len(outs))
	for _, o := range outs {
		if strVal(o["message_id"]) != "" {
			messages = append(messages, o)
		}
	}

	sort.Slice(messages, func(i, j int) bool {
		di, _ := strconv.ParseInt(strVal(messages[i]["internal_date"]), 10, 64)
		dj, _ := strconv.ParseInt(strVal(messages[j]["internal_date"]), 10, 64)
		return di < dj
	})
	return messages
}

// MailThread is the `+thread` shortcut: fetch a full mail conversation by
// thread ID, returning every message in chronological order.
var MailThread = common.Shortcut{
	Service:     "mail",
	Command:     "+thread",
	Description: "Use when querying a full mail conversation/thread by thread ID. Returns all messages in chronological order, including replies and drafts, with body content and attachments metadata, including inline images.",
	Risk:        "read",
	Scopes:      []string{"mail:user_mailbox.message:readonly", "mail:user_mailbox.message.address:read", "mail:user_mailbox.message.subject:read", "mail:user_mailbox.message.body:read"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "mailbox", Default: "me", Desc: "email address (default: me)"},
		{Name: "thread-id", Desc: "Required. Email thread ID", Required: true},
		{Name: "html", Type: "bool", Default: "true", Desc: "Whether to return HTML body (false returns plain text only to save bandwidth)"},
		{Name: "include-spam-trash", Type: "bool", Desc: "Also return messages from SPAM and TRASH folders (excluded by default)"},
		{Name: "print-output-schema", Type: "bool", Desc: "Print output field reference (run this first to learn field names before parsing output)"},
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		mailboxID := resolveMailboxID(runtime)
		threadID := runtime.Str("thread-id")
		params := map[string]interface{}{"format": messageGetFormat(runtime.Bool("html"))}
		if runtime.Bool("include-spam-trash") {
			params["include_spam_trash"] = true
		}
		return common.NewDryRunAPI().
			Desc("Fetch all emails in thread with full body content").
			GET(mailboxPath(mailboxID, "threads", threadID)).
			Params(params)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if runtime.Bool("print-output-schema") {
			printMessageOutputSchema(runtime)
			return nil
		}
		mailboxID := resolveMailboxID(runtime)
		hintIdentityFirst(runtime, mailboxID)
		threadID := runtime.Str("thread-id")
		html := runtime.Bool("html")

		// List thread messages with full body content in one call.
		params := map[string]interface{}{"format": messageGetFormat(html)}
		if runtime.Bool("include-spam-trash") {
			params["include_spam_trash"] = true
		}
		listData, err := runtime.CallAPI("GET", mailboxPath(mailboxID, "threads", threadID), params, nil)
		if err != nil {
			return fmt.Errorf("failed to get thread: %w", err)
		}
		// New API: data.thread.messages[]; fallback to old API: data.items[].message
		var items []interface{}
		if thread, ok := listData["thread"].(map[string]interface{}); ok {
			items, _ = thread["messages"].([]interface{})
		}
		if len(items) == 0 {
			items, _ = listData["items"].([]interface{})
		}
		if len(items) == 0 {
			runtime.Out(mailThreadOutput{ThreadID: threadID, MessageCount: 0, Messages: []map[string]interface{}{}}, nil)
			return nil
		}

		outs := make([]map[string]interface{}, 0, len(items))
		for _, item := range items {
			envelope, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			// Old API wraps each message inside a "message" sub-object; new API puts fields directly.
			msg := envelope
			if inner, ok := envelope["message"].(map[string]interface{}); ok {
				msg = inner
			}
			outs = append(outs, buildMessageOutput(msg, html))
		}

		// Sort by internal_date ascending.
		messages := sortThreadMessagesByInternalDate(outs)

		runtime.Out(mailThreadOutput{ThreadID: threadID, MessageCount: len(messages), Messages: messages}, nil)
		for _, item := range items {
			envelope, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			msg := envelope
			if inner, ok := envelope["message"].(map[string]interface{}); ok {
				msg = inner
			}
			maybeHintReadReceiptRequest(runtime, mailboxID, strVal(msg["message_id"]), msg)
		}
		return nil
	},
}
