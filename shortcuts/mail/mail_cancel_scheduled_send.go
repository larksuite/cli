// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"context"
	"fmt"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
	draftpkg "github.com/larksuite/cli/shortcuts/mail/draft"
)

var MailCancelScheduledSend = common.Shortcut{
	Service:     "mail",
	Command:     "+cancel-scheduled-send",
	Description: "Cancel a scheduled email send and restore the message as a draft.",
	Risk:        "write",
	Scopes:      []string{"mail:user_mailbox.message:send"},
	AuthTypes:   []string{"user"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "message-id", Desc: "Required. Message ID of the scheduled send to cancel", Required: true},
		{Name: "mailbox", Desc: "Mailbox email address that owns the scheduled message (default: me)."},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if runtime.Str("message-id") == "" {
			return output.ErrValidation("--message-id is required")
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		mailboxID := resolveComposeMailboxID(runtime)
		messageID := runtime.Str("message-id")
		return common.NewDryRunAPI().
			Desc("Cancel a scheduled send and restore the draft").
			POST(mailboxPath(mailboxID, "messages", messageID, "cancel_scheduled_send"))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		mailboxID := resolveComposeMailboxID(runtime)
		messageID := runtime.Str("message-id")

		resp, err := draftpkg.CancelScheduledSend(runtime, mailboxID, messageID)
		if err != nil {
			return output.ErrWithHint(output.ExitAPI, "api_error", fmt.Sprintf("Failed to cancel scheduled send: %v", err), "Check the message ID and make sure the message is still scheduled for delivery.")
		}

		out := map[string]interface{}{
			"message_id": messageID,
			"mailbox_id": mailboxID,
		}
		for k, v := range resp {
			out[k] = v
		}
		out["status"] = "canceled"
		runtime.Out(out, nil)
		return nil
	},
}
