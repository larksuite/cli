// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"context"
	"fmt"
	"io"

	"github.com/larksuite/cli/shortcuts/common"
)

// MailDeclineReceipt is the `+decline-receipt` shortcut: dismiss the read-
// receipt request banner on an incoming message WITHOUT sending a receipt.
// Mirrors the Lark client's "不发送" button next to the read-receipt prompt —
// the client talks to the internal MessageModify RPC with RemoveLabelIds=
// ["-607"]; this shortcut calls the public OpenAPI user_mailbox.message.modify
// which accepts the symbolic "READ_RECEIPT_REQUEST" label name (the public
// endpoint performs the symbolic→numeric translation server-side). Either
// path lands on the same MessageModify codepath, closing the banner.
// Removes only the READ_RECEIPT_REQUEST system label; no outgoing mail is
// produced. Idempotent: running it on a message that no longer carries the
// label is a no-op, not an error.
var MailDeclineReceipt = common.Shortcut{
	Service:     "mail",
	Command:     "+decline-receipt",
	Description: "Dismiss the read-receipt request banner on an incoming mail by clearing its READ_RECEIPT_REQUEST label, without sending a receipt. Use when the user wants to silence the prompt but refuse to confirm they have read it. Idempotent — safe to re-run.",
	Risk:        "write",
	Scopes: []string{
		"mail:user_mailbox.message:modify",
		"mail:user_mailbox.message:readonly",
		"mail:user_mailbox:readonly",
		// fetchFullMessage(..., false) uses format=plain_text_full which the
		// backend scope-checks against body:read even though we only inspect
		// label_ids. Declared explicitly to keep Scopes truthful.
		"mail:user_mailbox.message.body:read",
	},
	AuthTypes: []string{"user"},
	Flags: []common.Flag{
		{Name: "message-id", Desc: "Required. Message ID of the incoming mail that requested a read receipt.", Required: true},
		{Name: "mailbox", Desc: "Mailbox email address that owns the message (default: me)."},
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		messageID := runtime.Str("message-id")
		mailboxID := resolveComposeMailboxID(runtime)
		return common.NewDryRunAPI().
			Desc("Decline read receipt: fetch the original message → verify the READ_RECEIPT_REQUEST label is present → PUT user_mailbox.message.modify (the OpenAPI wrapper around the MessageModify RPC the Lark client's \"不发送\" button triggers) with remove_label_ids=[\"READ_RECEIPT_REQUEST\"]. No outgoing mail is produced; the banner is cleared locally. Idempotent when the label is already absent.").
			GET(mailboxPath(mailboxID, "messages", messageID)).
			Params(map[string]interface{}{"format": messageGetFormat(false)}).
			PUT(mailboxPath(mailboxID, "messages", messageID, "modify")).
			Body(map[string]interface{}{
				"remove_label_ids": []string{readReceiptRequestLabel},
			})
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		messageID := runtime.Str("message-id")
		mailboxID := resolveComposeMailboxID(runtime)

		msg, err := fetchFullMessage(runtime, mailboxID, messageID, false)
		if err != nil {
			return fmt.Errorf("failed to fetch original message: %w", err)
		}

		out := map[string]interface{}{
			"message_id":             messageID,
			"decline_receipt_for_id": messageID,
		}

		if !hasReadReceiptRequestLabel(msg) {
			out["declined"] = false
			out["already_cleared"] = true
			runtime.OutFormat(out, nil, func(w io.Writer) {
				fmt.Fprintln(w, "Read-receipt request already cleared — nothing to do.")
				fmt.Fprintf(w, "message_id: %s\n", messageID)
			})
			return nil
		}

		if _, err := runtime.CallAPI("PUT",
			mailboxPath(mailboxID, "messages", messageID, "modify"),
			nil,
			map[string]interface{}{
				"remove_label_ids": []string{readReceiptRequestLabel},
			},
		); err != nil {
			return fmt.Errorf("failed to clear READ_RECEIPT_REQUEST label: %w", err)
		}

		out["declined"] = true
		runtime.OutFormat(out, nil, func(w io.Writer) {
			fmt.Fprintln(w, "已关闭已读回执请求（未发送回执）/ Read-receipt request dismissed (no receipt sent).")
			fmt.Fprintf(w, "message_id: %s\n", messageID)
		})
		return nil
	},
}
