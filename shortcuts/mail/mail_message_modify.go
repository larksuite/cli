// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"context"

	"github.com/larksuite/cli/shortcuts/common"
)

type messageModifyInput struct {
	MessageIDs         []string
	AddLabelIDs        []string
	RemoveLabelIDs     []string
	AddFolder          string
	CustomLabelIDs     []string
	CustomFolderID     string
	ValidationAPIPlans []validationAPIPlan
}

// MailMessageModify is the `+message-modify` shortcut: apply labels, unread
// state labels, or a folder move to existing messages in batches of 20.
var MailMessageModify = common.Shortcut{
	Service:     "mail",
	Command:     "+message-modify",
	Description: "Modify existing mail messages by adding/removing label IDs or moving them to a folder. Batches message IDs in groups of 20 and keeps output compact.",
	Risk:        "write",
	Scopes:      []string{"mail:user_mailbox.message:modify"},
	ConditionalScopes: []string{
		"mail:user_mailbox.folder:read",
	},
	AuthTypes: []string{"user"},
	HasFormat: true,
	Flags: []common.Flag{
		{Name: "mailbox", Desc: "Mailbox email address that owns the messages (default: me)."},
		{Name: "message-ids", Type: "string_array", Required: true, Desc: "Message IDs to modify; comma-separated or repeat the flag."},
		{Name: "add-label-ids", Type: "string_slice", Desc: "Label IDs to add. System labels unread/important/other/flagged are normalized to upper case."},
		{Name: "remove-label-ids", Type: "string_slice", Desc: "Label IDs to remove. System labels unread/important/other/flagged are normalized to upper case."},
		{Name: "add-folder", Desc: "Folder ID to move messages to. System folders inbox/sent/spam/archive/archived are normalized; TRASH is rejected, use +message-trash."},
	},
	Validate: validateMessageModify,
	DryRun:   dryRunMessageModify,
	Execute:  executeMessageModify,
}

func validateMessageModify(ctx context.Context, rt *common.RuntimeContext) error {
	_, err := buildMessageModifyInput(rt)
	return err
}

func dryRunMessageModify(ctx context.Context, rt *common.RuntimeContext) *common.DryRunAPI {
	mailboxID := resolveMailboxID(rt)
	input, _ := buildMessageModifyInput(rt)
	api := common.NewDryRunAPI().
		Desc("Modify messages sequentially in batches of 20; dry-run does not call label/folder validation APIs").
		Set("batch_size", mailMessageManageBatchSize).
		Set("batches", chunkMessageManageIDs(input.MessageIDs)).
		Set("validation_api_plan", input.ValidationAPIPlans)
	for _, batch := range chunkMessageManageIDs(input.MessageIDs) {
		api = api.POST(mailboxPath(mailboxID, "messages", "batch_modify")).
			Body(messageManageBody(batch, input.AddLabelIDs, input.RemoveLabelIDs, input.AddFolder))
	}
	return api
}

func executeMessageModify(ctx context.Context, rt *common.RuntimeContext) error {
	mailboxID := resolveMailboxID(rt)
	input, err := buildMessageModifyInput(rt)
	if err != nil {
		return err
	}
	if err := validateCustomMessageManageLabels(rt, mailboxID, input.CustomLabelIDs); err != nil {
		return err
	}
	if err := validateCustomMessageManageFolder(rt, mailboxID, input.CustomFolderID); err != nil {
		return err
	}

	if len(input.AddLabelIDs) == 0 && len(input.RemoveLabelIDs) == 0 && input.AddFolder == "" {
		emitMessageManageSummary(rt, messageManageSummary{
			SuccessMessageIDs: input.MessageIDs,
			FailedMessageIDs:  []messageManageFailure{},
		}, true)
		return nil
	}

	summary := messageManageSummary{FailedMessageIDs: []messageManageFailure{}}
	for _, batch := range chunkMessageManageIDs(input.MessageIDs) {
		_, err := rt.CallAPITyped("POST", mailboxPath(mailboxID, "messages", "batch_modify"), nil,
			messageManageBody(batch, input.AddLabelIDs, input.RemoveLabelIDs, input.AddFolder))
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
		return mailFailedPreconditionError("all message modify batches failed")
	}
	return nil
}

func buildMessageModifyInput(rt *common.RuntimeContext) (messageModifyInput, error) {
	messageIDs, err := normalizeMessageManageIDs(rt.StrArray("message-ids"))
	if err != nil {
		return messageModifyInput{}, err
	}
	addLabels, customAddLabels, err := normalizeMessageManageLabels(rt.StrSlice("add-label-ids"), "--add-label-ids")
	if err != nil {
		return messageModifyInput{}, err
	}
	removeLabels, customRemoveLabels, err := normalizeMessageManageLabels(rt.StrSlice("remove-label-ids"), "--remove-label-ids")
	if err != nil {
		return messageModifyInput{}, err
	}
	if err := validateLabelIntersection(addLabels, removeLabels); err != nil {
		return messageModifyInput{}, err
	}
	folder, customFolder, err := normalizeMessageManageFolder(rt.Str("add-folder"))
	if err != nil {
		return messageModifyInput{}, err
	}
	customLabels := append(customAddLabels, customRemoveLabels...)
	customFolderID := ""
	if customFolder {
		customFolderID = folder
	}
	return messageModifyInput{
		MessageIDs:         messageIDs,
		AddLabelIDs:        addLabels,
		RemoveLabelIDs:     removeLabels,
		AddFolder:          folder,
		CustomLabelIDs:     customLabels,
		CustomFolderID:     customFolderID,
		ValidationAPIPlans: messageManageValidationPlan(resolveMailboxID(rt), customLabels, customFolderID),
	}, nil
}
