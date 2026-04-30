// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"context"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
)

// mailMessagesOutput is the +messages JSON output: the batch-get result,
// plus the total count and any requested IDs the backend did not return.
type mailMessagesOutput struct {
	Messages              []map[string]interface{} `json:"messages"`
	Total                 int                      `json:"total"`
	UnavailableMessageIDs []string                 `json:"unavailable_message_ids,omitempty"`
}

// MailMessages is the `+messages` shortcut: batch-fetch full content for
// up to 20 message IDs in a single call, preserving request order.
var MailMessages = common.Shortcut{
	Service:     "mail",
	Command:     "+messages",
	Description: "Use when reading full content for multiple emails by message ID. Prefer this shortcut over calling raw mail user_mailbox.messages batch_get directly, because it base64url-decodes body fields and returns normalized per-message output that is easier to consume.",
	Risk:        "read",
	Scopes:      []string{"mail:user_mailbox.message:readonly", "mail:user_mailbox.message.address:read", "mail:user_mailbox.message.subject:read", "mail:user_mailbox.message.body:read"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "mailbox", Default: "me", Desc: "email address (default: me)"},
		{Name: "message-ids", Desc: `Required. Comma-separated email message IDs. Example: "id1,id2,id3"`, Required: true},
		{Name: "html", Type: "bool", Default: "true", Desc: "Whether to return HTML body (false returns plain text only to save bandwidth)"},
		{Name: "print-output-schema", Type: "bool", Desc: "Print output field reference (run this first to learn field names before parsing output)"},
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		mailboxID := resolveMailboxID(runtime)
		messageIDs := splitByComma(runtime.Str("message-ids"))
		body := map[string]interface{}{
			"format":      messageGetFormat(runtime.Bool("html")),
			"message_ids": []string{"<message_id_1>", "<message_id_2>"},
		}
		if len(messageIDs) > 0 {
			body["message_ids"] = messageIDs
		}
		return common.NewDryRunAPI().
			Desc("Fetch multiple emails via messages.batch_get (auto-chunked in batches of 20 IDs during execution)").
			POST(mailboxPath(mailboxID, "messages", "batch_get")).
			Body(body)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if runtime.Bool("print-output-schema") {
			printMessageOutputSchema(runtime)
			return nil
		}
		mailboxID := resolveMailboxID(runtime)
		hintIdentityFirst(runtime, mailboxID)
		messageIDs := splitByComma(runtime.Str("message-ids"))
		if len(messageIDs) == 0 {
			return output.ErrValidation("--message-ids is required; provide one or more message IDs separated by commas")
		}
		html := runtime.Bool("html")

		rawMessages, missingMessageIDs, err := fetchFullMessages(runtime, mailboxID, messageIDs, html)
		if err != nil {
			return err
		}

		messages := make([]map[string]interface{}, 0, len(rawMessages))
		for _, msg := range rawMessages {
			messages = append(messages, buildMessageOutput(msg, html))
		}

		runtime.Out(mailMessagesOutput{
			Messages:              messages,
			Total:                 len(messages),
			UnavailableMessageIDs: missingMessageIDs,
		}, nil)
		for _, msg := range rawMessages {
			maybeHintReadReceiptRequest(runtime, mailboxID, strVal(msg["message_id"]), msg)
		}
		return nil
	},
}
