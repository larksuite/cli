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

const mailThreadManageMaxIDs = 20

type threadModifyOutput struct {
	Operation          string   `json:"operation"`
	Mailbox            string   `json:"mailbox"`
	SubmittedThreadIDs []string `json:"submitted_thread_ids"`
	SubmittedCount     int      `json:"submitted_count"`
	AddLabelIDs        []string `json:"add_label_ids"`
	RemoveLabelIDs     []string `json:"remove_label_ids"`
	AddFolder          string   `json:"add_folder"`
}

type threadTrashOutput struct {
	Operation          string   `json:"operation"`
	Mailbox            string   `json:"mailbox"`
	SubmittedThreadIDs []string `json:"submitted_thread_ids"`
	SubmittedCount     int      `json:"submitted_count"`
}

// MailThreadModify is the `+thread-modify` shortcut: apply label changes or a
// folder move to existing mail threads with request-side output only.
var MailThreadModify = common.Shortcut{
	Service:     "mail",
	Command:     "+thread-modify",
	Description: "Modify existing mail threads by adding/removing label IDs or moving them to a folder. Output reports submitted thread IDs only, not server-side final success counts.",
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
	Description: "Soft-delete existing mail threads. Output reports submitted thread IDs only, not server-side final success counts. Requires --yes.",
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
	return common.NewDryRunAPI().
		Desc("Modify threads with one batch_modify request; submitted_count is request-side only and does not mean the server changed every thread").
		POST(mailboxPath(mailboxID, "threads", "batch_modify")).
		Body(threadModifyBody(input))
}

func executeThreadModify(ctx context.Context, rt *common.RuntimeContext) error {
	mailboxID := resolveMailboxID(rt)
	input, err := buildThreadModifyInput(rt)
	if err != nil {
		return err
	}
	if _, err := rt.CallAPITyped("POST", mailboxPath(mailboxID, "threads", "batch_modify"), nil, threadModifyBody(input)); err != nil {
		return mailDecorateProblemMessage(err, "failed to modify threads")
	}
	emitThreadManageOutput(rt, threadModifyOutput{
		Operation:          "thread_modify",
		Mailbox:            mailboxID,
		SubmittedThreadIDs: input.ThreadIDs,
		SubmittedCount:     len(input.ThreadIDs),
		AddLabelIDs:        input.AddLabelIDs,
		RemoveLabelIDs:     input.RemoveLabelIDs,
		AddFolder:          input.FolderID,
	})
	return nil
}

func validateThreadTrash(ctx context.Context, rt *common.RuntimeContext) error {
	_, err := normalizeThreadManageIDs(rt.StrArray("thread-ids"))
	return err
}

func dryRunThreadTrash(ctx context.Context, rt *common.RuntimeContext) *common.DryRunAPI {
	mailboxID := resolveMailboxID(rt)
	threadIDs, _ := normalizeThreadManageIDs(rt.StrArray("thread-ids"))
	return common.NewDryRunAPI().
		Desc("Soft-delete threads with one batch_trash request; submitted_count is request-side only and does not mean the server trashed every thread").
		POST(mailboxPath(mailboxID, "threads", "batch_trash")).
		Body(map[string]interface{}{"thread_ids": threadIDs})
}

func executeThreadTrash(ctx context.Context, rt *common.RuntimeContext) error {
	mailboxID := resolveMailboxID(rt)
	threadIDs, err := normalizeThreadManageIDs(rt.StrArray("thread-ids"))
	if err != nil {
		return err
	}
	if _, err := rt.CallAPITyped("POST", mailboxPath(mailboxID, "threads", "batch_trash"), nil, map[string]interface{}{"thread_ids": threadIDs}); err != nil {
		return mailDecorateProblemMessage(err, "failed to trash threads")
	}
	emitThreadManageOutput(rt, threadTrashOutput{
		Operation:          "thread_trash",
		Mailbox:            mailboxID,
		SubmittedThreadIDs: threadIDs,
		SubmittedCount:     len(threadIDs),
	})
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
	if len(ids) > mailThreadManageMaxIDs {
		return nil, mailValidationParamError("--thread-ids", "thread_ids accepts at most %d thread IDs (got %d)", mailThreadManageMaxIDs, len(ids))
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

type threadSubmittedOutput interface {
	submittedOperation() string
	submittedCount() int
}

func (out threadModifyOutput) submittedOperation() string { return out.Operation }
func (out threadModifyOutput) submittedCount() int        { return out.SubmittedCount }
func (out threadTrashOutput) submittedOperation() string  { return out.Operation }
func (out threadTrashOutput) submittedCount() int         { return out.SubmittedCount }

func emitThreadManageOutput[T threadSubmittedOutput](rt *common.RuntimeContext, out T) {
	rt.OutFormat(out, &output.Meta{Count: out.submittedCount()}, func(w io.Writer) {
		fmt.Fprintf(w, "%s: submitted %d thread(s)\n", out.submittedOperation(), out.submittedCount())
		fmt.Fprintln(w, "submitted_count is request-side only; it does not represent server-side final success.")
	})
}
