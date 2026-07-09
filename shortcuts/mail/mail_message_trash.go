// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"context"

	"github.com/larksuite/cli/shortcuts/common"
)

// MailMessageTrash is the `+message-trash` shortcut: soft-delete existing
// messages in batches of 20 via batch_trash. Risk is high-risk-write, so the
// runner requires --yes before Execute.
var MailMessageTrash = common.Shortcut{
	Service:     "mail",
	Command:     "+message-trash",
	Description: "Soft-delete existing mail messages. Batches message IDs in groups of 20 and calls batch_trash sequentially. Requires --yes.",
	Risk:        "high-risk-write",
	Scopes:      []string{"mail:user_mailbox.message:modify"},
	AuthTypes:   []string{"user"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "mailbox", Desc: "Mailbox email address that owns the messages (default: me)."},
		{Name: "message-ids", Type: "string_array", Required: true, Desc: "Message IDs to soft-delete; comma-separated or repeat the flag."},
	},
	Validate: validateMessageTrash,
	DryRun:   dryRunMessageTrash,
	Execute:  executeMessageTrash,
}

func validateMessageTrash(ctx context.Context, rt *common.RuntimeContext) error {
	_, err := normalizeMessageManageIDs(rt.StrArray("message-ids"))
	return err
}

func dryRunMessageTrash(ctx context.Context, rt *common.RuntimeContext) *common.DryRunAPI {
	mailboxID := resolveMailboxID(rt)
	messageIDs, _ := normalizeMessageManageIDs(rt.StrArray("message-ids"))
	api := common.NewDryRunAPI().
		Desc("Soft-delete messages sequentially in batches of 20").
		Set("batch_size", mailMessageManageBatchSize).
		Set("batches", chunkMessageManageIDs(messageIDs))
	for _, batch := range chunkMessageManageIDs(messageIDs) {
		api = api.POST(mailboxPath(mailboxID, "messages", "batch_trash")).
			Body(map[string]interface{}{"message_ids": batch})
	}
	return api
}

func executeMessageTrash(ctx context.Context, rt *common.RuntimeContext) error {
	mailboxID := resolveMailboxID(rt)
	messageIDs, err := normalizeMessageManageIDs(rt.StrArray("message-ids"))
	if err != nil {
		return err
	}

	summary := messageManageSummary{FailedMessageIDs: []messageManageFailure{}}
	for _, batch := range chunkMessageManageIDs(messageIDs) {
		_, err := rt.CallAPITyped("POST", mailboxPath(mailboxID, "messages", "batch_trash"), nil,
			map[string]interface{}{"message_ids": batch})
		if err != nil {
			for _, id := range batch {
				summary.FailedMessageIDs = append(summary.FailedMessageIDs, messageManageFailure{MessageID: id, Reason: err.Error()})
			}
			continue
		}
		summary.SuccessMessageIDs = append(summary.SuccessMessageIDs, batch...)
	}
	emitMessageManageSummary(rt, summary, false)
	if len(summary.SuccessMessageIDs) == 0 && len(summary.FailedMessageIDs) > 0 {
		return mailFailedPreconditionError("all message trash batches failed")
	}
	return nil
}
