// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"context"
	"fmt"

	"github.com/larksuite/cli/shortcuts/common"
)

// MailMessage is the `+message` shortcut: fetch full content of a single
// email by message ID (normalized body + attachments / inline metadata).
var MailMessage = common.Shortcut{
	Service:     "mail",
	Command:     "+message",
	Description: "Use when reading full content for a single email by message ID. Returns normalized body content plus attachments metadata, including inline images.",
	Risk:        "read",
	Scopes:      []string{"mail:user_mailbox.message:readonly", "mail:user_mailbox.message.address:read", "mail:user_mailbox.message.subject:read", "mail:user_mailbox.message.body:read"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "mailbox", Default: "me", Desc: "email address (default: me)"},
		{Name: "message-id", Desc: "Required. Email message ID", Required: true},
		{Name: "html", Type: "bool", Default: "true", Desc: "Whether to return HTML body (false returns plain text only to save bandwidth)"},
		{Name: "print-output-schema", Type: "bool", Desc: "Print output field reference (run this first to learn field names before parsing output)"},
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		mailboxID := resolveMailboxID(runtime)
		messageID := runtime.Str("message-id")
		return common.NewDryRunAPI().
			Desc("Fetch full email content and attachments metadata, including inline images").
			GET(mailboxPath(mailboxID, "messages", messageID))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if runtime.Bool("print-output-schema") {
			printMessageOutputSchema(runtime)
			return nil
		}
		mailboxID := resolveMailboxID(runtime)
		hintIdentityFirst(runtime, mailboxID)
		messageID := runtime.Str("message-id")
		html := runtime.Bool("html")

		msg, err := fetchFullMessage(runtime, mailboxID, messageID, html)
		if err != nil {
			return fmt.Errorf("failed to fetch email: %w", err)
		}

		out := buildMessageOutput(msg, html)
		runtime.Out(out, nil)
		maybeHintReadReceiptRequest(runtime, mailboxID, messageID, msg)
		return nil
	},
}
