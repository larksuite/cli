// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"fmt"
	"io"
	"strings"
	"unicode"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
)

const mailMessageManageBatchSize = 20

var messageManageSystemLabels = map[string]string{
	"UNREAD":               "UNREAD",
	"IMPORTANT":            "IMPORTANT",
	"OTHER":                "OTHER",
	"FLAGGED":              "FLAGGED",
	"READ_RECEIPT_REQUEST": "READ_RECEIPT_REQUEST",
}

var messageManageSystemFolders = map[string]string{
	"INBOX":    "INBOX",
	"SENT":     "SENT",
	"SPAM":     "SPAM",
	"ARCHIVE":  "ARCHIVED",
	"ARCHIVED": "ARCHIVED",
}

type messageManageSummary struct {
	SuccessMessageIDs []string               `json:"success_message_ids"`
	FailedMessageIDs  []messageManageFailure `json:"failed_message_ids"`
}

type messageManageFailure struct {
	MessageID string `json:"message_id"`
	Reason    string `json:"reason"`
}

type validationAPIPlan struct {
	Method       string `json:"method"`
	Path         string `json:"path"`
	WillValidate bool   `json:"will_validate"`
}

func normalizeMessageManageIDs(raw []string) ([]string, error) {
	if len(raw) == 0 {
		return nil, mailValidationParamError("--message-ids", "--message-ids is required")
	}
	parts, err := splitMessageManageIDTokens(raw)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for i, part := range parts {
		if part == "" {
			return nil, mailValidationParamError("--message-ids", "--message-ids entry %d is empty; remove extra commas or provide valid message IDs", i+1)
		}
		id := strings.TrimSpace(part)
		if id == "" {
			return nil, mailValidationParamError("--message-ids", "--message-ids entry %d is empty; remove extra commas or provide valid message IDs", i+1)
		}
		if id != part {
			return nil, mailValidationParamError("--message-ids", "--message-ids entry %d (%q): must not contain leading or trailing whitespace", i+1, part)
		}
		if err := validateMessageManageID(id, i); err != nil {
			return nil, err
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return nil, mailValidationParamError("--message-ids", "--message-ids is required")
	}
	return ids, nil
}

func splitMessageManageIDTokens(raw []string) ([]string, error) {
	parts := make([]string, 0, len(raw))
	for i, token := range raw {
		for _, r := range token {
			if unicode.IsSpace(r) || unicode.IsControl(r) {
				return nil, mailValidationParamError("--message-ids", "--message-ids entry %d (%q): must not contain whitespace or control characters", i+1, token)
			}
		}
		parts = append(parts, strings.Split(token, ",")...)
	}
	return parts, nil
}

func validateMessageManageID(id string, index int) error {
	if len(id) < 16 {
		return mailValidationParamError("--message-ids", "--message-ids entry %d (%q): length must be at least 16 characters", index+1, id)
	}
	if strings.Trim(id, "0123456789") == "" {
		return mailValidationParamError("--message-ids", "--message-ids entry %d (%q): numeric primary IDs are not supported; pass the Open API message_id from mail output", index+1, id)
	}
	for _, r := range id {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			return mailValidationParamError("--message-ids", "--message-ids entry %d (%q): must not contain whitespace or control characters", index+1, id)
		}
		if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			continue
		}
		switch r {
		case '+', '/', '=', '_', '-':
			continue
		default:
			return mailValidationParamError("--message-ids", "--message-ids entry %d (%q): contains characters outside the Open API message_id character set", index+1, id)
		}
	}
	return nil
}

func normalizeMessageManageLabels(raw []string, flagName string) ([]string, []string, error) {
	labels := make([]string, 0, len(raw))
	custom := make([]string, 0, len(raw))
	seen := make(map[string]struct{}, len(raw))
	for i, part := range raw {
		id := strings.TrimSpace(part)
		if id == "" {
			return nil, nil, mailValidationParamError(flagName, "%s entry %d is empty; remove extra commas or provide valid label IDs", flagName, i+1)
		}
		if id != part {
			return nil, nil, mailValidationParamError(flagName, "%s entry %d (%q): must not contain leading or trailing whitespace", flagName, i+1, part)
		}
		normalized := id
		if system, ok := messageManageSystemLabels[strings.ToUpper(id)]; ok {
			normalized = system
		} else {
			custom = append(custom, id)
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		labels = append(labels, normalized)
	}
	if len(labels) > 20 {
		return nil, nil, mailValidationParamError(flagName, "%s accepts at most 20 label IDs (got %d)", flagName, len(labels))
	}
	return labels, custom, nil
}

func validateLabelIntersection(add, remove []string) error {
	removeSet := make(map[string]struct{}, len(remove))
	for _, id := range remove {
		removeSet[id] = struct{}{}
	}
	for _, id := range add {
		if _, ok := removeSet[id]; ok {
			return mailValidationParamError("--add-label-ids", "label cannot be both added and removed: %s", id)
		}
	}
	return nil
}

func normalizeMessageManageFolder(raw string) (string, bool, error) {
	if raw == "" {
		return "", false, nil
	}
	folder := strings.TrimSpace(raw)
	if folder == "" {
		return "", false, mailValidationParamError("--add-folder", "--add-folder must not be empty")
	}
	if folder != raw {
		return "", false, mailValidationParamError("--add-folder", "--add-folder %q must not contain leading or trailing whitespace", raw)
	}
	if strings.EqualFold(folder, "TRASH") {
		return "", false, mailValidationParamError("--add-folder", "TRASH is not supported by +message-modify; use +message-trash")
	}
	if system, ok := messageManageSystemFolders[strings.ToUpper(folder)]; ok {
		return system, false, nil
	}
	return folder, true, nil
}

func chunkMessageManageIDs(ids []string) [][]string {
	if len(ids) == 0 {
		return nil
	}
	chunks := make([][]string, 0, (len(ids)+mailMessageManageBatchSize-1)/mailMessageManageBatchSize)
	for start := 0; start < len(ids); start += mailMessageManageBatchSize {
		end := start + mailMessageManageBatchSize
		if end > len(ids) {
			end = len(ids)
		}
		chunks = append(chunks, ids[start:end])
	}
	return chunks
}

func validateCustomMessageManageLabels(rt *common.RuntimeContext, mailboxID string, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	if err := validateLabelReadScope(rt); err != nil {
		return err
	}
	seen := map[string]struct{}{}
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		if _, err := rt.CallAPITyped("GET", mailboxPath(mailboxID, "labels", id), nil, nil); err != nil {
			return mailDecorateProblemMessage(err, "label not found: %s", id)
		}
	}
	return nil
}

func validateCustomMessageManageFolder(rt *common.RuntimeContext, mailboxID, id string) error {
	if id == "" {
		return nil
	}
	if err := validateFolderReadScope(rt); err != nil {
		return err
	}
	if _, err := rt.CallAPITyped("GET", mailboxPath(mailboxID, "folders", id), nil, nil); err != nil {
		return mailDecorateProblemMessage(err, "folder not found: %s", id)
	}
	return nil
}

func messageManageBody(ids, addLabels, removeLabels []string, addFolder string) map[string]interface{} {
	body := map[string]interface{}{"message_ids": ids}
	if len(addLabels) > 0 {
		body["add_label_ids"] = addLabels
	}
	if len(removeLabels) > 0 {
		body["remove_label_ids"] = removeLabels
	}
	if addFolder != "" {
		body["add_folder"] = addFolder
	}
	return body
}

func messageManageValidationPlan(mailboxID string, customLabels []string, customFolder string) []validationAPIPlan {
	plans := make([]validationAPIPlan, 0, len(customLabels)+1)
	seenLabels := map[string]struct{}{}
	for _, id := range customLabels {
		if _, ok := seenLabels[id]; ok {
			continue
		}
		seenLabels[id] = struct{}{}
		plans = append(plans, validationAPIPlan{
			Method:       "GET",
			Path:         mailboxPath(mailboxID, "labels", id),
			WillValidate: true,
		})
	}
	if customFolder != "" {
		plans = append(plans, validationAPIPlan{
			Method:       "GET",
			Path:         mailboxPath(mailboxID, "folders", customFolder),
			WillValidate: true,
		})
	}
	return plans
}

func emitMessageManageSummary(rt *common.RuntimeContext, summary messageManageSummary, noAPICalls bool) {
	rt.OutFormat(summary, &output.Meta{Count: len(summary.SuccessMessageIDs)}, func(w io.Writer) {
		fmt.Fprintf(w, "success_message_ids: %d\n", len(summary.SuccessMessageIDs))
		fmt.Fprintf(w, "failed_message_ids: %d\n", len(summary.FailedMessageIDs))
		if noAPICalls {
			fmt.Fprintln(w, "No changes requested; no API calls were made.")
		}
		for _, item := range summary.FailedMessageIDs {
			fmt.Fprintf(w, "- %s: %s\n", item.MessageID, item.Reason)
		}
	})
}
