// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"context"
	"strings"

	"github.com/larksuite/cli/shortcuts/common"
)

type threadModifyInput struct {
	ThreadIDs      []string
	AddLabelIDs    []string
	RemoveLabelIDs []string
	FolderID       string
}

// MailThreadModify is the `+thread-modify` shortcut: apply label changes or a
// folder move to existing mail threads in one batch_modify request.
var MailThreadModify = common.Shortcut{
	Service:     "mail",
	Command:     "+thread-modify",
	Description: "Modify existing mail threads by adding/removing label IDs or moving them to a folder in one batch request.",
	Risk:        "write",
	Scopes:      []string{"mail:user_mailbox.message:modify"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "mailbox-id", Aliases: []string{"mailbox"}, Default: "me", Desc: "Mailbox email address that owns the threads (default: me)."},
		{Name: "thread-id", Type: "string_array", Desc: "Required. Thread ID to modify; repeat the flag for multiple threads."},
		{Name: "add-label-id", Type: "string_slice", Desc: "Label ID to add; repeat the flag for multiple labels."},
		{Name: "remove-label-id", Type: "string_slice", Desc: "Label ID to remove; cannot overlap with --add-label-id."},
		{Name: "folder-id", Aliases: []string{"add-folder"}, Desc: "Folder ID to move threads to."},
	},
	Validate: validateThreadModify,
	DryRun:   dryRunThreadModify,
	Execute:  executeThreadModify,
}

// MailThreadTrash is the `+thread-trash` shortcut: soft-delete existing mail
// threads through one batch_trash request. Risk is high-risk-write, so the
// runner requires --yes before Execute.
var MailThreadTrash = common.Shortcut{
	Service:     "mail",
	Command:     "+thread-trash",
	Description: "Soft-delete existing mail threads in one batch request. Requires --yes.",
	Risk:        "high-risk-write",
	Scopes:      []string{"mail:user_mailbox.message:modify"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "mailbox-id", Aliases: []string{"mailbox"}, Default: "me", Desc: "Mailbox email address that owns the threads (default: me)."},
		{Name: "thread-id", Type: "string_array", Desc: "Required. Thread ID to soft-delete; repeat the flag for multiple threads."},
	},
	Validate: validateThreadTrash,
	DryRun:   dryRunThreadTrash,
	Execute:  executeThreadTrash,
}

func validateThreadModify(ctx context.Context, rt *common.RuntimeContext) error {
	if err := validateBotMailboxNotMe(rt); err != nil {
		return err
	}
	_, err := buildThreadModifyInput(rt)
	return err
}

func dryRunThreadModify(ctx context.Context, rt *common.RuntimeContext) *common.DryRunAPI {
	mailboxID := resolveMailboxID(rt)
	input, _ := buildThreadModifyInput(rt)
	return common.NewDryRunAPI().
		Desc("Modify threads in one batch request").
		POST(mailboxPath(mailboxID, "threads", "batch_modify")).
		Body(threadModifyBody(input))
}

func executeThreadModify(ctx context.Context, rt *common.RuntimeContext) error {
	mailboxID := resolveMailboxID(rt)
	input, err := buildThreadModifyInput(rt)
	if err != nil {
		return err
	}
	data, err := rt.CallAPITyped("POST", mailboxPath(mailboxID, "threads", "batch_modify"), nil, threadModifyBody(input))
	if err != nil {
		return err
	}
	rt.OutFormat(data, nil, nil)
	return nil
}

func validateThreadTrash(ctx context.Context, rt *common.RuntimeContext) error {
	if err := validateBotMailboxNotMe(rt); err != nil {
		return err
	}
	_, err := normalizeThreadManageIDs(rt.StrArray("thread-id"))
	return err
}

func dryRunThreadTrash(ctx context.Context, rt *common.RuntimeContext) *common.DryRunAPI {
	mailboxID := resolveMailboxID(rt)
	threadIDs, _ := normalizeThreadManageIDs(rt.StrArray("thread-id"))
	return common.NewDryRunAPI().
		Desc("Soft-delete threads in one batch request").
		POST(mailboxPath(mailboxID, "threads", "batch_trash")).
		Body(map[string]interface{}{"thread_ids": threadIDs})
}

func executeThreadTrash(ctx context.Context, rt *common.RuntimeContext) error {
	mailboxID := resolveMailboxID(rt)
	threadIDs, err := normalizeThreadManageIDs(rt.StrArray("thread-id"))
	if err != nil {
		return err
	}
	data, err := rt.CallAPITyped("POST", mailboxPath(mailboxID, "threads", "batch_trash"), nil,
		map[string]interface{}{"thread_ids": threadIDs})
	if err != nil {
		return err
	}
	rt.OutFormat(data, nil, nil)
	return nil
}

func buildThreadModifyInput(rt *common.RuntimeContext) (threadModifyInput, error) {
	threadIDs, err := normalizeThreadManageIDs(rt.StrArray("thread-id"))
	if err != nil {
		return threadModifyInput{}, err
	}
	addLabels, err := normalizeThreadManageLabels(rt.StrSlice("add-label-id"), "--add-label-id")
	if err != nil {
		return threadModifyInput{}, err
	}
	removeLabels, err := normalizeThreadManageLabels(rt.StrSlice("remove-label-id"), "--remove-label-id")
	if err != nil {
		return threadModifyInput{}, err
	}
	if err := validateThreadLabelIntersection(addLabels, removeLabels); err != nil {
		return threadModifyInput{}, err
	}
	folderID, err := normalizeThreadManageFolder(rt.Str("folder-id"))
	if err != nil {
		return threadModifyInput{}, err
	}
	if len(addLabels) == 0 && len(removeLabels) == 0 && folderID == "" {
		return threadModifyInput{}, mailValidationParamError("--thread-modify", "provide at least one of --add-label-id, --remove-label-id, or --folder-id")
	}
	return threadModifyInput{
		ThreadIDs:      threadIDs,
		AddLabelIDs:    addLabels,
		RemoveLabelIDs: removeLabels,
		FolderID:       folderID,
	}, nil
}

func validateThreadLabelIntersection(add, remove []string) error {
	removeSet := make(map[string]struct{}, len(remove))
	for _, id := range remove {
		removeSet[id] = struct{}{}
	}
	for _, id := range add {
		if _, ok := removeSet[id]; ok {
			return mailValidationParamError("--add-label-id", "label cannot be both added and removed: %s", id)
		}
	}
	return nil
}

func normalizeThreadManageLabels(raw []string, flagName string) ([]string, error) {
	labels := make([]string, 0, len(raw))
	seen := make(map[string]struct{}, len(raw))
	for i, part := range raw {
		id := strings.TrimSpace(part)
		if id == "" {
			return nil, mailValidationParamError(flagName, "%s entry %d is empty; remove extra commas or provide valid label IDs", flagName, i+1)
		}
		if id != part {
			return nil, mailValidationParamError(flagName, "%s entry %d (%q): must not contain leading or trailing whitespace", flagName, i+1, part)
		}
		upper := strings.ToUpper(id)
		if upper == readReceiptRequestLabel {
			return nil, mailValidationParamError(flagName, "thread 级别不能管理 `READ_RECEIPT_REQUEST`")
		}
		normalized := id
		switch upper {
		case "UNREAD", "IMPORTANT", "OTHER", "FLAGGED":
			normalized = upper
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		labels = append(labels, normalized)
	}
	if len(labels) > 20 {
		return nil, mailValidationParamError(flagName, "%s accepts at most 20 label IDs (got %d)", flagName, len(labels))
	}
	return labels, nil
}

func normalizeThreadManageIDs(raw []string) ([]string, error) {
	if len(raw) == 0 {
		return nil, mailValidationParamError("--thread-id", "--thread-id is required")
	}
	ids := make([]string, 0, len(raw))
	seen := map[string]struct{}{}
	for tokenIndex, token := range raw {
		for _, r := range token {
			if r == '\n' || r == '\r' || r == '\t' {
				return nil, mailValidationParamError("--thread-id", "--thread-id entry %d (%q): must not contain whitespace or control characters", tokenIndex+1, token)
			}
		}
		for partIndex, part := range strings.Split(token, ",") {
			if part == "" {
				return nil, mailValidationParamError("--thread-id", "--thread-id contains empty value; remove extra commas or provide valid thread IDs")
			}
			id := strings.TrimSpace(part)
			if id == "" {
				return nil, mailValidationParamError("--thread-id", "--thread-id contains empty value; remove extra commas or provide valid thread IDs")
			}
			if id != part {
				return nil, mailValidationParamError("--thread-id", "--thread-id entry %d (%q): must not contain leading or trailing whitespace", partIndex+1, part)
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
		return nil, mailValidationParamError("--thread-id", "--thread-id must include at least one non-empty thread ID")
	}
	return ids, nil
}

func validateThreadManageID(id string, index int) error {
	if strings.Trim(id, "0123456789") == "" {
		return mailValidationParamError("--thread-id", "--thread-id entry %d (%q): numeric primary IDs are not supported; pass the Open API thread_id from mail output", index+1, id)
	}
	for _, r := range id {
		if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			continue
		}
		switch r {
		case '+', '/', '=', '_', '-':
			continue
		default:
			return mailValidationParamError("--thread-id", "--thread-id entry %d (%q): contains characters outside the Open API thread_id character set", index+1, id)
		}
	}
	return nil
}

func normalizeThreadManageFolder(raw string) (string, error) {
	return normalizeThreadManageFolderForFlag(raw, "--folder-id")
}

func normalizeThreadManageFolderForFlag(raw, flagName string) (string, error) {
	if raw == "" {
		return "", nil
	}
	folder := strings.TrimSpace(raw)
	if folder == "" {
		return "", mailValidationParamError(flagName, "%s must not be empty", flagName)
	}
	if strings.EqualFold(folder, "TRASH") {
		return "", mailValidationParamError(flagName, "TRASH is not supported by +thread-modify; use +thread-trash")
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
