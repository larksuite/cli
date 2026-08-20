// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
)

type threadModifyInput struct {
	ThreadIDs      []string
	AddLabelIDs    []string
	RemoveLabelIDs []string
	FolderID       string
}

const mailThreadManageBatchSize = 20

type threadManageSummary struct {
	SuccessThreadIDs []string              `json:"success_thread_ids"`
	FailedThreadIDs  []threadManageFailure `json:"failed_thread_ids"`
}

type threadManageFailure struct {
	ThreadID string `json:"thread_id"`
	Reason   string `json:"reason"`
}

// MailThreadModify is the `+thread-modify` shortcut: apply label changes or a
// folder move to existing mail threads in batches of 20.
var MailThreadModify = common.Shortcut{
	Service:     "mail",
	Command:     "+thread-modify",
	Description: "Modify existing mail threads by adding/removing label IDs or moving them to a folder. Batches thread IDs in groups of 20 and keeps output compact.",
	Risk:        "write",
	Scopes:      []string{"mail:user_mailbox.message:modify"},
	ConditionalScopes: []string{
		"mail:user_mailbox.folder:read",
	},
	AuthTypes: []string{"user"},
	HasFormat: true,
	Flags: []common.Flag{
		{Name: "mailbox", Default: "me", Desc: "Mailbox email address that owns the threads (default: me)."},
		{Name: "thread-ids", Type: "string_array", Required: true, Desc: "Thread IDs to modify; comma-separated or repeat the flag."},
		{Name: "add-label-ids", Type: "string_slice", Desc: "Label IDs to add. System labels unread/important/other/flagged are normalized to upper case."},
		{Name: "remove-label-ids", Type: "string_slice", Desc: "Label IDs to remove. Cannot overlap with --add-label-ids."},
		{Name: "add-folder", Aliases: []string{"folder-id"}, Desc: "Folder ID to move threads to."},
	},
	Validate: validateThreadModify,
	DryRun:   dryRunThreadModify,
	Execute:  executeThreadModify,
}

// MailThreadTrash is the `+thread-trash` shortcut: soft-delete existing mail
// threads through the thread batch_trash route. Risk is high-risk-write, so the
// runner requires --yes before Execute.
var MailThreadTrash = common.Shortcut{
	Service:     "mail",
	Command:     "+thread-trash",
	Description: "Soft-delete existing mail threads. Batches thread IDs in groups of 20 and calls batch_trash sequentially. Requires --yes.",
	Risk:        "high-risk-write",
	Scopes:      []string{"mail:user_mailbox.message:modify"},
	AuthTypes:   []string{"user"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "mailbox", Default: "me", Desc: "Mailbox email address that owns the threads (default: me)."},
		{Name: "thread-ids", Type: "string_array", Required: true, Desc: "Thread IDs to soft-delete; comma-separated or repeat the flag."},
	},
	Validate: validateThreadTrash,
	DryRun:   dryRunThreadTrash,
	Execute:  executeThreadTrash,
}

func validateThreadModify(ctx context.Context, rt *common.RuntimeContext) error {
	_, err := buildThreadModifyInput(rt)
	return err
}

func dryRunThreadModify(ctx context.Context, rt *common.RuntimeContext) *common.DryRunAPI {
	mailboxID := resolveMailboxID(rt)
	input, _ := buildThreadModifyInput(rt)
	api := common.NewDryRunAPI().
		Desc("Modify threads sequentially in batches of 20").
		Set("batch_size", mailThreadManageBatchSize).
		Set("batches", chunkThreadManageIDs(input.ThreadIDs))
	for _, batch := range chunkThreadManageIDs(input.ThreadIDs) {
		api = api.POST(mailboxPath(mailboxID, "threads", "batch_modify")).
			Body(threadModifyBody(threadModifyInput{
				ThreadIDs:      batch,
				AddLabelIDs:    input.AddLabelIDs,
				RemoveLabelIDs: input.RemoveLabelIDs,
				FolderID:       input.FolderID,
			}))
	}
	return api
}

func executeThreadModify(ctx context.Context, rt *common.RuntimeContext) error {
	mailboxID := resolveMailboxID(rt)
	input, err := buildThreadModifyInput(rt)
	if err != nil {
		return err
	}
	summary := threadManageSummary{FailedThreadIDs: []threadManageFailure{}}
	for _, batch := range chunkThreadManageIDs(input.ThreadIDs) {
		_, err := rt.CallAPITyped("POST", mailboxPath(mailboxID, "threads", "batch_modify"), nil,
			threadModifyBody(threadModifyInput{
				ThreadIDs:      batch,
				AddLabelIDs:    input.AddLabelIDs,
				RemoveLabelIDs: input.RemoveLabelIDs,
				FolderID:       input.FolderID,
			}))
		if err != nil {
			decorated := mailDecorateProblemMessage(err, "failed to modify threads")
			for _, id := range batch {
				summary.FailedThreadIDs = append(summary.FailedThreadIDs, threadManageFailure{ThreadID: id, Reason: decorated.Error()})
			}
			continue
		}
		summary.SuccessThreadIDs = append(summary.SuccessThreadIDs, batch...)
	}
	emitThreadManageSummary(rt, summary)
	if len(summary.SuccessThreadIDs) == 0 && len(summary.FailedThreadIDs) > 0 {
		return mailFailedPreconditionError("all thread modify batches failed")
	}
	return nil
}

func validateThreadTrash(ctx context.Context, rt *common.RuntimeContext) error {
	_, err := normalizeThreadManageIDs(rt.StrArray("thread-ids"))
	return err
}

func dryRunThreadTrash(ctx context.Context, rt *common.RuntimeContext) *common.DryRunAPI {
	mailboxID := resolveMailboxID(rt)
	threadIDs, _ := normalizeThreadManageIDs(rt.StrArray("thread-ids"))
	api := common.NewDryRunAPI().
		Desc("Soft-delete threads sequentially in batches of 20").
		Set("batch_size", mailThreadManageBatchSize).
		Set("batches", chunkThreadManageIDs(threadIDs))
	for _, batch := range chunkThreadManageIDs(threadIDs) {
		api = api.POST(mailboxPath(mailboxID, "threads", "batch_trash")).
			Body(map[string]interface{}{"thread_ids": batch})
	}
	return api
}

func executeThreadTrash(ctx context.Context, rt *common.RuntimeContext) error {
	mailboxID := resolveMailboxID(rt)
	threadIDs, err := normalizeThreadManageIDs(rt.StrArray("thread-ids"))
	if err != nil {
		return err
	}
	summary := threadManageSummary{FailedThreadIDs: []threadManageFailure{}}
	for _, batch := range chunkThreadManageIDs(threadIDs) {
		_, err := rt.CallAPITyped("POST", mailboxPath(mailboxID, "threads", "batch_trash"), nil,
			map[string]interface{}{"thread_ids": batch})
		if err != nil {
			decorated := mailDecorateProblemMessage(err, "failed to trash threads")
			for _, id := range batch {
				summary.FailedThreadIDs = append(summary.FailedThreadIDs, threadManageFailure{ThreadID: id, Reason: decorated.Error()})
			}
			continue
		}
		summary.SuccessThreadIDs = append(summary.SuccessThreadIDs, batch...)
	}
	emitThreadManageSummary(rt, summary)
	if len(summary.SuccessThreadIDs) == 0 && len(summary.FailedThreadIDs) > 0 {
		return mailFailedPreconditionError("all thread trash batches failed")
	}
	return nil
}

func buildThreadModifyInput(rt *common.RuntimeContext) (threadModifyInput, error) {
	threadIDs, err := normalizeThreadManageIDs(rt.StrArray("thread-ids"))
	if err != nil {
		return threadModifyInput{}, err
	}
	addLabels, _, err := normalizeMessageManageLabels(rt.StrSlice("add-label-ids"), "--add-label-ids")
	if err != nil {
		return threadModifyInput{}, err
	}
	removeLabels, _, err := normalizeMessageManageLabels(rt.StrSlice("remove-label-ids"), "--remove-label-ids")
	if err != nil {
		return threadModifyInput{}, err
	}
	if err := validateLabelIntersection(addLabels, removeLabels); err != nil {
		return threadModifyInput{}, err
	}
	folderID, err := normalizeThreadManageFolder(rt.Str("add-folder"))
	if err != nil {
		return threadModifyInput{}, err
	}
	if len(addLabels) == 0 && len(removeLabels) == 0 && folderID == "" {
		return threadModifyInput{}, mailValidationParamError("--thread-modify", "provide at least one of --add-label-ids, --remove-label-ids, or --add-folder")
	}
	return threadModifyInput{
		ThreadIDs:      threadIDs,
		AddLabelIDs:    addLabels,
		RemoveLabelIDs: removeLabels,
		FolderID:       folderID,
	}, nil
}

func normalizeThreadManageIDs(raw []string) ([]string, error) {
	if len(raw) == 0 {
		return nil, mailValidationParamError("--thread-ids", "--thread-ids is required")
	}
	ids := make([]string, 0, len(raw))
	seen := map[string]struct{}{}
	for tokenIndex, token := range raw {
		for _, r := range token {
			if r == '\n' || r == '\r' || r == '\t' {
				return nil, mailValidationParamError("--thread-ids", "--thread-ids entry %d (%q): must not contain whitespace or control characters", tokenIndex+1, token)
			}
		}
		for partIndex, part := range strings.Split(token, ",") {
			if part == "" {
				return nil, mailValidationParamError("--thread-ids", "--thread-ids contains empty value; remove extra commas or provide valid thread IDs")
			}
			id := strings.TrimSpace(part)
			if id == "" {
				return nil, mailValidationParamError("--thread-ids", "--thread-ids contains empty value; remove extra commas or provide valid thread IDs")
			}
			if id != part {
				return nil, mailValidationParamError("--thread-ids", "--thread-ids entry %d (%q): must not contain leading or trailing whitespace", partIndex+1, part)
			}
			if err := validateThreadManageID(id, partIndex); err != nil {
				return nil, err
			}
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return nil, mailValidationParamError("--thread-ids", "--thread-ids must include at least one non-empty thread ID")
	}
	return ids, nil
}

func validateThreadManageID(id string, index int) error {
	if strings.Trim(id, "0123456789") == "" {
		return mailValidationParamError("--thread-ids", "--thread-ids entry %d (%q): numeric primary IDs are not supported; pass the Open API thread_id from mail output", index+1, id)
	}
	for _, r := range id {
		if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			continue
		}
		switch r {
		case '+', '/', '=', '_', '-':
			continue
		default:
			return mailValidationParamError("--thread-ids", "--thread-ids entry %d (%q): contains characters outside the Open API thread_id character set", index+1, id)
		}
	}
	return nil
}

func normalizeThreadManageFolder(raw string) (string, error) {
	if raw == "" {
		return "", nil
	}
	folder := strings.TrimSpace(raw)
	if folder == "" {
		return "", mailValidationParamError("--add-folder", "--add-folder must not be empty")
	}
	if strings.EqualFold(folder, "TRASH") {
		return "", mailValidationParamError("--add-folder", "TRASH is not supported by +thread-modify; use +thread-trash")
	}
	if system, ok := messageManageSystemFolders[strings.ToUpper(folder)]; ok {
		return system, nil
	}
	return folder, nil
}

func threadModifyBody(input threadModifyInput) map[string]interface{} {
	body := map[string]interface{}{"thread_ids": input.ThreadIDs}
	if len(input.AddLabelIDs) > 0 {
		body["add_label_ids"] = input.AddLabelIDs
	}
	if len(input.RemoveLabelIDs) > 0 {
		body["remove_label_ids"] = input.RemoveLabelIDs
	}
	if input.FolderID != "" {
		body["add_folder"] = input.FolderID
	}
	return body
}

func chunkThreadManageIDs(ids []string) [][]string {
	if len(ids) == 0 {
		return nil
	}
	chunks := make([][]string, 0, (len(ids)+mailThreadManageBatchSize-1)/mailThreadManageBatchSize)
	for start := 0; start < len(ids); start += mailThreadManageBatchSize {
		end := start + mailThreadManageBatchSize
		if end > len(ids) {
			end = len(ids)
		}
		chunks = append(chunks, ids[start:end])
	}
	return chunks
}

func emitThreadManageSummary(rt *common.RuntimeContext, summary threadManageSummary) {
	rt.OutFormat(summary, &output.Meta{Count: len(summary.SuccessThreadIDs)}, func(w io.Writer) {
		fmt.Fprintf(w, "success_thread_ids: %d\n", len(summary.SuccessThreadIDs))
		fmt.Fprintf(w, "failed_thread_ids: %d\n", len(summary.FailedThreadIDs))
		for _, item := range summary.FailedThreadIDs {
			fmt.Fprintf(w, "- %s: %s\n", item.ThreadID, item.Reason)
		}
	})
}
