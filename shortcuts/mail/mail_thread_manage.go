// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"context"
	"strings"

	"github.com/larksuite/cli/shortcuts/common"
)

const (
	mailThreadModifyMethod = "POST"
	mailThreadTrashMethod  = "POST"
)

type threadModifyInput struct {
	ThreadIDs        []string
	AddLabelIDs      []string
	RemoveLabelIDs   []string
	FolderID         string
	FolderIDProvided bool
}

type threadAPIRequest struct {
	Method string
	Path   string
	Body   map[string]interface{}
}

// MailThreadModify exposes only the fields supported by
// user_mailbox.threads.batch_modify. Callers cannot inject arbitrary request
// data or the protocol-level add_folder field name.
var MailThreadModify = common.Shortcut{
	Service:     "mail",
	Command:     "+thread-modify",
	Description: "Modify entire mail threads by adding or removing label IDs, or moving them to a folder.",
	Risk:        "write",
	Scopes:      []string{"mail:user_mailbox.message:modify"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "mailbox", Default: "me", Desc: "Mailbox email address that owns the threads (default: me)."},
		{Name: "thread-id", Type: "string_slice", Required: true, Desc: "Thread ID; comma-separated or repeat the flag."},
		{Name: "add-label-id", Type: "string_slice", Desc: "Label ID to add; comma-separated or repeat the flag."},
		{Name: "remove-label-id", Type: "string_slice", Desc: "Label ID to remove; comma-separated or repeat the flag."},
		{Name: "folder-id", Desc: "Folder ID to move the threads to."},
	},
	Validate: validateThreadModify,
	DryRun:   dryRunThreadModify,
	Execute:  executeThreadModify,
}

// MailThreadTrash sends one batch request and therefore neither fans the
// operation out nor invents per-thread success results.
var MailThreadTrash = common.Shortcut{
	Service:     "mail",
	Command:     "+thread-trash",
	Description: "Soft-delete entire mail threads in one batch request. Requires --yes.",
	Risk:        "high-risk-write",
	Scopes:      []string{"mail:user_mailbox.message:modify"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "mailbox", Default: "me", Desc: "Mailbox email address that owns the threads (default: me)."},
		{Name: "thread-id", Type: "string_slice", Required: true, Desc: "Thread ID; comma-separated or repeat the flag."},
	},
	Validate: validateThreadTrash,
	DryRun:   dryRunThreadTrash,
	Execute:  executeThreadTrash,
}

func validateThreadModify(ctx context.Context, rt *common.RuntimeContext) error {
	if err := validateBotMailboxNotMe(rt); err != nil {
		return err
	}
	_, err := threadModifyAPIRequest(rt)
	return err
}

func dryRunThreadModify(ctx context.Context, rt *common.RuntimeContext) *common.DryRunAPI {
	req, _ := threadModifyAPIRequest(rt)
	return common.NewDryRunAPI().
		Desc("Modify entire mail threads in one batch request").
		POST(req.Path).
		Body(req.Body)
}

func executeThreadModify(ctx context.Context, rt *common.RuntimeContext) error {
	req, err := threadModifyAPIRequest(rt)
	if err != nil {
		return err
	}
	data, err := rt.CallAPITyped(req.Method, req.Path, nil, req.Body)
	if err != nil {
		return err
	}
	rt.Out(data, nil)
	return nil
}

func validateThreadTrash(ctx context.Context, rt *common.RuntimeContext) error {
	if err := validateBotMailboxNotMe(rt); err != nil {
		return err
	}
	_, err := threadTrashAPIRequest(rt)
	return err
}

func dryRunThreadTrash(ctx context.Context, rt *common.RuntimeContext) *common.DryRunAPI {
	req, _ := threadTrashAPIRequest(rt)
	return common.NewDryRunAPI().
		Desc("Soft-delete entire mail threads in one batch request").
		POST(req.Path).
		Body(req.Body)
}

func executeThreadTrash(ctx context.Context, rt *common.RuntimeContext) error {
	req, err := threadTrashAPIRequest(rt)
	if err != nil {
		return err
	}
	data, err := rt.CallAPITyped(req.Method, req.Path, nil, req.Body)
	if err != nil {
		return err
	}
	rt.Out(data, nil)
	return nil
}

func threadModifyAPIRequest(rt *common.RuntimeContext) (threadAPIRequest, error) {
	return buildThreadModifyRequest(resolveMailboxID(rt), threadModifyInput{
		ThreadIDs:        rt.StrSlice("thread-id"),
		AddLabelIDs:      rt.StrSlice("add-label-id"),
		RemoveLabelIDs:   rt.StrSlice("remove-label-id"),
		FolderID:         rt.Str("folder-id"),
		FolderIDProvided: rt.Changed("folder-id"),
	})
}

func threadTrashAPIRequest(rt *common.RuntimeContext) (threadAPIRequest, error) {
	return buildThreadTrashRequest(resolveMailboxID(rt), rt.StrSlice("thread-id"))
}

// buildThreadModifyRequest is the pure source of truth shared by validation,
// dry-run, and execution. Its body is an explicit allowlist by construction.
func buildThreadModifyRequest(mailboxID string, raw threadModifyInput) (threadAPIRequest, error) {
	threadIDs, err := normalizeThreadIDs(raw.ThreadIDs, "--thread-id", true)
	if err != nil {
		return threadAPIRequest{}, err
	}
	addLabelIDs, err := normalizeThreadIDs(raw.AddLabelIDs, "--add-label-id", false)
	if err != nil {
		return threadAPIRequest{}, err
	}
	removeLabelIDs, err := normalizeThreadIDs(raw.RemoveLabelIDs, "--remove-label-id", false)
	if err != nil {
		return threadAPIRequest{}, err
	}
	if err := validateThreadLabelIntersection(addLabelIDs, removeLabelIDs); err != nil {
		return threadAPIRequest{}, err
	}

	folderID := ""
	if raw.FolderIDProvided {
		folderID = strings.TrimSpace(raw.FolderID)
		if folderID == "" {
			return threadAPIRequest{}, mailValidationParamError("--folder-id", "--folder-id must not be empty")
		}
	}
	if len(addLabelIDs) == 0 && len(removeLabelIDs) == 0 && folderID == "" {
		return threadAPIRequest{}, mailValidationParamError("--thread-modify", "provide at least one of --add-label-id, --remove-label-id, or --folder-id")
	}

	body := map[string]interface{}{"thread_ids": threadIDs}
	if len(addLabelIDs) > 0 {
		body["add_label_ids"] = addLabelIDs
	}
	if len(removeLabelIDs) > 0 {
		body["remove_label_ids"] = removeLabelIDs
	}
	if folderID != "" {
		body["add_folder"] = folderID
	}
	return threadAPIRequest{
		Method: mailThreadModifyMethod,
		Path:   mailboxPath(mailboxID, "threads", "batch_modify"),
		Body:   body,
	}, nil
}

// buildThreadTrashRequest mirrors user_mailbox.threads.batch_trash exactly:
// the request has one and only one body field, thread_ids.
func buildThreadTrashRequest(mailboxID string, rawThreadIDs []string) (threadAPIRequest, error) {
	threadIDs, err := normalizeThreadIDs(rawThreadIDs, "--thread-id", true)
	if err != nil {
		return threadAPIRequest{}, err
	}
	return threadAPIRequest{
		Method: mailThreadTrashMethod,
		Path:   mailboxPath(mailboxID, "threads", "batch_trash"),
		Body:   map[string]interface{}{"thread_ids": threadIDs},
	}, nil
}

func normalizeThreadIDs(raw []string, flagName string, required bool) ([]string, error) {
	if len(raw) == 0 {
		if required {
			return nil, mailValidationParamError(flagName, "%s is required", flagName)
		}
		return nil, nil
	}

	ids := make([]string, 0, len(raw))
	seen := make(map[string]struct{}, len(raw))
	for _, token := range raw {
		for _, part := range strings.Split(token, ",") {
			id := strings.TrimSpace(part)
			if id == "" {
				return nil, mailValidationParamError(flagName, "%s contains an empty ID", flagName)
			}
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			ids = append(ids, id)
		}
	}
	if required && len(ids) == 0 {
		return nil, mailValidationParamError(flagName, "%s must include at least one ID", flagName)
	}
	return ids, nil
}

func validateThreadLabelIntersection(addLabelIDs, removeLabelIDs []string) error {
	added := make(map[string]struct{}, len(addLabelIDs))
	for _, id := range addLabelIDs {
		added[id] = struct{}{}
	}
	conflicts := make([]string, 0)
	for _, id := range removeLabelIDs {
		if _, ok := added[id]; ok {
			conflicts = append(conflicts, id)
		}
	}
	if len(conflicts) > 0 {
		return mailValidationParamError("--add-label-id", "label IDs cannot be both added and removed: %s", strings.Join(conflicts, ", "))
	}
	return nil
}
