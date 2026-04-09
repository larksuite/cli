// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	netmail "net/mail"
	"net/url"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
	draftpkg "github.com/larksuite/cli/shortcuts/mail/draft"
	"github.com/larksuite/cli/shortcuts/mail/emlbuilder"
)

// hintIdentityFirst prints a one-line tip to stderr for read-only mail shortcuts
// that don't internally call user_mailboxes.profile. This helps models and users
// discover the identity-first workflow without needing skill documentation.
func hintIdentityFirst(runtime *common.RuntimeContext, mailboxID string) {
	fmt.Fprintf(runtime.IO().ErrOut,
		"tip: run \"lark-cli mail user_mailboxes profile --params '{\"user_mailbox_id\":\"%s\"}'\" to confirm your email identity\n",
		sanitizeForTerminal(mailboxID))
}

// hintSendDraft prints a post-draft-save tip to stderr telling the user
// (or the calling agent) how to send the draft that was just created.
func hintSendDraft(runtime *common.RuntimeContext, mailboxID, draftID string) {
	fmt.Fprintf(runtime.IO().ErrOut,
		"tip: draft saved. To send this draft, run:\n"+
			`  lark-cli mail user_mailbox.drafts send --params '{"user_mailbox_id":"%s","draft_id":"%s"}'`+"\n",
		sanitizeForTerminal(mailboxID), sanitizeForTerminal(draftID))
}

// hintMarkAsRead prints a post-send tip to stderr suggesting the user mark the
// original message as read after a reply/reply-all/forward operation.
func hintMarkAsRead(runtime *common.RuntimeContext, mailboxID, originalMessageID string) {
	fmt.Fprintf(runtime.IO().ErrOut,
		"tip: mark original as read? lark-cli mail user_mailbox.messages batch_modify_message"+
			` --params '{"user_mailbox_id":"%s"}' --data '{"message_ids":["%s"],"remove_label_ids":["UNREAD"]}'`+"\n",
		sanitizeForTerminal(mailboxID), sanitizeForTerminal(originalMessageID))
}

// messageOutputSchema returns a JSON description of +message / +messages / +thread output fields.
// Used by --print-output-schema to let callers discover field names without reading skill docs.
func printMessageOutputSchema(runtime *common.RuntimeContext) {
	schema := map[string]interface{}{
		"_description": "Output field reference for mail +message / +messages / +thread",
		"fields": map[string]string{
			"message_id":                             "Email message ID",
			"thread_id":                              "Thread ID",
			"subject":                                "Email subject",
			"head_from":                              "Sender object: {mail_address, name}",
			"to":                                     "To recipients: [{mail_address, name}]",
			"cc":                                     "CC recipients: [{mail_address, name}]",
			"bcc":                                    "BCC recipients: [{mail_address, name}]",
			"date":                                   "Time in EML (milliseconds)",
			"date_formatted":                         "Human-readable send time, e.g. '2026-03-19 16:33'",
			"smtp_message_id":                        "SMTP Message-ID conforming to RFC 2822",
			"in_reply_to":                            "In-Reply-To email header",
			"references":                             "References email header, list of ancestor SMTP message IDs",
			"internal_date":                          "Create/receive/send time (milliseconds)",
			"message_state":                          "Message state: 1 = received, 2 = sent, 3 = draft",
			"message_state_text":                     "unknown / received / sent / draft",
			"folder_id":                              "Folder ID. Values: INBOX, SENT, SPAM, ARCHIVED, STRANGER, or custom folder ID",
			"label_ids":                              "List of label IDs",
			"priority_type":                          "Priority value. Values: 0 = no priority, 1 = high, 3 = normal, 5 = low",
			"priority_type_text":                     "unknown / high / normal / low",
			"security_level":                         "Security/risk assessment object; present when the server has risk metadata",
			"security_level.is_risk":                 "Boolean. true if the message is flagged as risky",
			"security_level.risk_banner_level":       "Risk severity. Values: WARNING (warning), DANGER (danger), INFO (informational)",
			"security_level.risk_banner_reason":      "Risk reason. Values: NO_REASON, IMPERSONATE_DOMAIN (similar-domain spoofing), IMPERSONATE_KP_NAME (key-person name spoofing), UNAUTH_EXTERNAL (unauthenticated external domain), MALICIOUS_URL, MALICIOUS_ATTACHMENT, PHISHING, IMPERSONATE_PARTNER (partner spoofing), EXTERNAL_ENCRYPTION_ATTACHMENT (external encrypted attachment)",
			"security_level.is_header_from_external": "Boolean. true if the sender is from an external domain",
			"security_level.via_domain":              "SPF/DKIM domain shown when the email is sent on behalf of or forged, e.g. 'larksuite.com'",
			"security_level.spam_banner_type":        "Spam reason. Values: USER_REPORT (user reported spam), USER_BLOCK (sender blocked by user), ANTI_SPAM (system classified as spam), USER_RULE (matched inbox rule into spam), BLOCK_DOMIN (domain blocked by user), BLOCK_ADDRESS (address blocked by user)",
			"security_level.spam_user_rule_id":       "ID of the matched inbox rule",
			"security_level.spam_banner_info":        "Address or domain that matched the user's blocklist, e.g. 'larksuite.com'",
			"draft_id":                               "Draft ID, obtainable via list drafts API",
			"reply_to":                               "Reply-To email header",
			"reply_to_smtp_message_id":               "Reply-To SMTP Message-ID",
			"body_plain_text":                        "Preferred body field for LLM reading; base64url-decoded and ANSI-sanitized",
			"body_preview":                           "First 100 characters of plaintext body content, for quick preview of core email content",
			"body_html":                              "Raw HTML body; omitted when --html=false",
			"attachments":                            "Unified list of regular attachments and inline images",
			"attachments[].id":                       "Attachment ID (use with download_url API)",
			"attachments[].filename":                 "Attachment filename",
			"attachments[].content_type":             "MIME content type of the attachment",
			"attachments[].attachment_type":          "Attachment type. Values: 1 = normal, 2 = large attachment",
			"attachments[].is_inline":                "true = inline image, false = regular attachment",
			"attachments[].cid":                      "Content-ID for inline images (maps to <img src='cid:...'>)",
		},
		"thread_extra_fields": map[string]string{
			"thread_id":     "Thread ID",
			"message_count": "Number of messages in thread",
			"messages":      "Message array sorted by internal_date ascending (oldest first)",
		},
		"messages_extra_fields": map[string]string{
			"total":                   "Number of successfully returned messages",
			"unavailable_message_ids": "Requested IDs not returned by the API",
		},
	}
	runtime.Out(schema, nil)
}

// printWatchOutputSchema prints the per-format field reference for +watch output.
// Used by --print-output-schema to let callers discover field names without reading skill docs.
func printWatchOutputSchema(runtime *common.RuntimeContext) {
	schema := map[string]interface{}{
		"minimal": map[string]interface{}{
			"message": map[string]interface{}{
				"message_id":    "<message_id>",
				"thread_id":     "<thread_id>",
				"folder_id":     "INBOX",
				"label_ids":     []string{"UNREAD", "IMPORTANT"},
				"internal_date": "1700000000000",
				"message_state": 1,
			},
		},
		"metadata": map[string]interface{}{
			"message": map[string]interface{}{
				"message_id":      "<message_id>",
				"thread_id":       "<thread_id>",
				"subject":         "<subject>",
				"head_from":       map[string]string{"mail_address": "<address>", "name": "<name>"},
				"to":              []map[string]string{{"mail_address": "<address>", "name": "<name>"}},
				"body_preview":    "<preview>",
				"internal_date":   "1700000000000",
				"folder_id":       "INBOX",
				"label_ids":       []string{"UNREAD", "IMPORTANT"},
				"message_state":   1,
				"in_reply_to":     "",
				"references":      "",
				"reply_to":        "",
				"smtp_message_id": "<smtp_message_id>",
				"security_level":  map[string]bool{"is_risk": false},
				"attachments":     []interface{}{},
			},
		},
		"plain_text_full": map[string]interface{}{
			"message": map[string]interface{}{
				"_note":           "all fields from metadata, plus:",
				"body_plain_text": "<plain text body>",
			},
		},
		"full": map[string]interface{}{
			"message": map[string]interface{}{
				"_note":     "all fields from plain_text_full, plus:",
				"body_html": "<html body>",
				"attachments": []map[string]interface{}{
					{
						"id":              "<attachment_id>",
						"filename":        "<filename>",
						"content_type":    "<mime_type>",
						"is_inline":       false,
						"cid":             "",
						"attachment_type": 1,
					},
				},
			},
		},
		"event": map[string]interface{}{
			"header": map[string]string{
				"event_id":    "<event_id>",
				"create_time": "1700000000000",
			},
			"event": map[string]interface{}{
				"mail_address": "<address>",
				"message_id":   "<message_id>",
				"mailbox_type": 1,
			},
		},
	}
	b, _ := json.MarshalIndent(schema, "", "  ")
	fmt.Fprintln(runtime.IO().Out, string(b))
}

// resolveMailboxID returns the user_mailbox_id from --mailbox flag, defaulting to "me".
func resolveMailboxID(runtime *common.RuntimeContext) string {
	id := runtime.Str("mailbox")
	if id == "" {
		return "me"
	}
	return id
}

// resolveComposeMailboxID returns the mailbox ID for compose shortcuts.
// Priority: --mailbox > --from > "me".
// When sending via an alias (send_as), use --mailbox for the owning mailbox
// and --from for the alias sender address.
func resolveComposeMailboxID(runtime *common.RuntimeContext) string {
	if mb := runtime.Str("mailbox"); mb != "" {
		return mb
	}
	if from := runtime.Str("from"); from != "" {
		return from
	}
	return "me"
}

// mailboxPath builds the full open-api path for a user mailbox sub-resource.
// Each path segment is escaped independently to avoid reserved-char path breakage.
func mailboxPath(mailboxID string, segments ...string) string {
	parts := make([]string, 0, len(segments)+1)
	parts = append(parts, url.PathEscape(mailboxID))
	for _, seg := range segments {
		if seg == "" {
			continue
		}
		parts = append(parts, url.PathEscape(seg))
	}
	return "/open-apis/mail/v1/user_mailboxes/" + strings.Join(parts, "/")
}

// fetchMailboxPrimaryEmail retrieves mailbox primary_email_address from
// user_mailboxes.profile. Returns the email address or an error.
func fetchMailboxPrimaryEmail(runtime *common.RuntimeContext, mailboxID string) (string, error) {
	if mailboxID == "" {
		mailboxID = "me"
	}
	data, err := runtime.CallAPI("GET", mailboxPath(mailboxID, "profile"), nil, nil)
	if err != nil {
		return "", err
	}
	if email := extractPrimaryEmail(data); email != "" {
		return email, nil
	}
	if nested, ok := data["data"].(map[string]interface{}); ok {
		if email := extractPrimaryEmail(nested); email != "" {
			return email, nil
		}
	}
	return "", fmt.Errorf("profile API returned no primary_email_address")
}

func extractPrimaryEmail(data map[string]interface{}) string {
	if email, ok := data["primary_email_address"].(string); ok && strings.TrimSpace(email) != "" {
		return strings.TrimSpace(email)
	}
	if mailbox, ok := data["user_mailbox"].(map[string]interface{}); ok {
		if email, ok := mailbox["primary_email_address"].(string); ok && strings.TrimSpace(email) != "" {
			return strings.TrimSpace(email)
		}
	}
	return ""
}

// resolveComposeSenderEmail determines the sender email for compose shortcuts.
// Priority: --from > --mailbox > profile("me").
// The profile API only supports "me", so when --mailbox is set to a non-"me"
// address (e.g. a shared mailbox), its value is used directly as the sender.
func resolveComposeSenderEmail(runtime *common.RuntimeContext) string {
	if from := runtime.Str("from"); from != "" {
		return from
	}
	if mb := runtime.Str("mailbox"); mb != "" && mb != "me" {
		return mb
	}
	email, _ := fetchMailboxPrimaryEmail(runtime, "me")
	return email
}

// fetchSelfEmailSet returns a set of addresses to exclude as "self" in
// reply-all. It always tries profile("me"); when mailboxID or senderEmail
// differ from "me", those are added to the set as well so that shared-
// mailbox and alias addresses are also excluded.
func fetchSelfEmailSet(runtime *common.RuntimeContext, mailboxID string) map[string]bool {
	set := make(map[string]bool)
	// Always include the "me" primary email.
	if email, _ := fetchMailboxPrimaryEmail(runtime, "me"); email != "" {
		set[strings.ToLower(email)] = true
	}
	// Include mailboxID itself (covers shared mailbox addresses).
	if mailboxID != "" && mailboxID != "me" {
		set[strings.ToLower(mailboxID)] = true
	}
	// Include --from alias address so it's excluded from reply-all recipients.
	if from := runtime.Str("from"); from != "" {
		set[strings.ToLower(from)] = true
	}
	return set
}

// folderAliasToSystemID maps friendly folder alias to system folder ID.
var folderAliasToSystemID = map[string]string{
	"inbox":    "INBOX",
	"sent":     "SENT",
	"draft":    "DRAFT",
	"trash":    "TRASH",
	"spam":     "SPAM",
	"archive":  "ARCHIVED",
	"archived": "ARCHIVED",
}

// folderSystemIDToAlias maps system folder IDs to the search API query names.
// Note: the search API uses "archive" (not "archived") for the ARCHIVED folder.
var folderSystemIDToAlias = map[string]string{
	"INBOX":    "inbox",
	"SENT":     "sent",
	"DRAFT":    "draft",
	"TRASH":    "trash",
	"SPAM":     "spam",
	"ARCHIVED": "archive",
}

// searchOnlyFolderNames are folder names accepted only by the search API,
// not present in the folder list API. They are passed through as-is.
var searchOnlyFolderNames = map[string]bool{
	"scheduled": true,
}

// folderSystemIDs are known built-in folder IDs that can be passed directly.
var folderSystemIDs = map[string]bool{
	"INBOX":    true,
	"SENT":     true,
	"DRAFT":    true,
	"TRASH":    true,
	"SPAM":     true,
	"ARCHIVED": true,
}

// labelSystemIDs are known built-in label IDs that can be passed directly.
var labelSystemIDs = map[string]bool{
	"FLAGGED":   true,
	"IMPORTANT": true,
	"OTHER":     true,
}

// systemLabelAliases maps all recognized user inputs (lowercase) to canonical system label IDs.
// These system labels can be passed via either --filter folder or --filter label.
// On search path they are sent as folder values; on list path they are sent as label_id.
var systemLabelAliases = map[string]string{
	// IMPORTANT
	"important": "IMPORTANT",
	"priority":  "IMPORTANT",
	"重要邮件":      "IMPORTANT",
	// FLAGGED
	"flagged": "FLAGGED",
	"已加旗标":    "FLAGGED",
	// OTHER
	"other": "OTHER",
	"其他邮件":  "OTHER",
}

// systemLabelSearchName maps system label IDs to the search API folder values.
// Note: the search API uses "priority" (not "important") for the IMPORTANT label.
var systemLabelSearchName = map[string]string{
	"FLAGGED":   "flagged",
	"IMPORTANT": "priority",
	"OTHER":     "other",
}

// resolveSystemLabel checks if input is a system label alias (case-insensitive).
// Returns the canonical system label ID and true, or ("", false).
func resolveSystemLabel(input string) (string, bool) {
	if id, ok := systemLabelAliases[strings.ToLower(strings.TrimSpace(input))]; ok {
		return id, true
	}
	// Also check uppercase form directly (e.g. "FLAGGED", "IMPORTANT", "OTHER").
	if id, ok := normalizeSystemID(input, labelSystemIDs); ok {
		return id, true
	}
	return "", false
}

type folderInfo struct {
	ID             string
	Name           string
	ParentFolderID string
}

type labelInfo struct {
	ID   string
	Name string
}

func resolveFolderID(runtime *common.RuntimeContext, mailboxID, input string) (string, error) {
	value := strings.TrimSpace(input)
	if value == "" {
		return "", nil
	}
	if id, ok := normalizeSystemID(value, folderSystemIDs); ok {
		return id, nil
	}
	folders, err := listMailboxFolders(runtime, mailboxID)
	if err != nil {
		return "", err
	}
	return resolveByID("folder", value, mailboxID, folders, func(item folderInfo) string { return item.ID })
}

func resolveFolderName(runtime *common.RuntimeContext, mailboxID, input string) (string, error) {
	value := strings.TrimSpace(input)
	if value == "" {
		return "", nil
	}
	if id, ok := resolveFolderSystemAliasOrID(value); ok {
		return id, nil
	}
	folders, err := listMailboxFolders(runtime, mailboxID)
	if err != nil {
		return "", err
	}
	return resolveByName("folder", value, mailboxID, folders,
		func(item folderInfo) string { return item.ID },
		func(item folderInfo) string { return item.Name },
	)
}

func resolveLabelID(runtime *common.RuntimeContext, mailboxID, input string) (string, error) {
	value := strings.TrimSpace(input)
	if value == "" {
		return "", nil
	}
	if id, ok := resolveLabelSystemID(value); ok {
		return id, nil
	}
	labels, err := listMailboxLabels(runtime, mailboxID)
	if err != nil {
		return "", err
	}
	return resolveByID("label", value, mailboxID, labels, func(item labelInfo) string { return item.ID })
}

func resolveLabelName(runtime *common.RuntimeContext, mailboxID, input string) (string, error) {
	value := strings.TrimSpace(input)
	if value == "" {
		return "", nil
	}
	if id, ok := resolveLabelSystemID(value); ok {
		return id, nil
	}
	labels, err := listMailboxLabels(runtime, mailboxID)
	if err != nil {
		return "", err
	}
	id, err := resolveByName("label", value, mailboxID, labels,
		func(item labelInfo) string { return item.ID },
		func(item labelInfo) string { return item.Name },
	)
	if err != nil {
		if matchID := matchLabelSuffixID(value, labels); matchID != "" {
			return matchID, nil
		}
		return "", err
	}
	return id, nil
}

func resolveFolderQueryName(runtime *common.RuntimeContext, mailboxID, input string) (string, error) {
	value := strings.TrimSpace(input)
	if value == "" {
		return "", nil
	}
	if searchOnlyFolderNames[strings.ToLower(value)] {
		return strings.ToLower(value), nil
	}
	if id, ok := resolveFolderSystemAliasOrID(value); ok {
		return folderSystemIDToAlias[id], nil
	}
	folders, err := listMailboxFolders(runtime, mailboxID)
	if err != nil {
		return "", err
	}
	name, err := resolveNameValueByNameAllowDuplicates("folder", value, mailboxID, folders,
		func(item folderInfo) string { return item.ID },
		func(item folderInfo) string { return item.Name },
	)
	if err != nil {
		return "", err
	}
	return folderSearchPath(name, value, folders), nil
}

func resolveFolderQueryNameFromID(runtime *common.RuntimeContext, mailboxID, input string) (string, error) {
	value := strings.TrimSpace(input)
	if value == "" {
		return "", nil
	}
	if id, ok := resolveFolderSystemAliasOrID(value); ok {
		return folderSystemIDToAlias[id], nil
	}
	folders, err := listMailboxFolders(runtime, mailboxID)
	if err != nil {
		return "", err
	}
	name, err := resolveNameValueByID("folder", value, mailboxID, folders,
		func(item folderInfo) string { return item.ID },
		func(item folderInfo) string { return item.Name },
	)
	if err != nil {
		return "", err
	}
	return folderSearchPath(name, value, folders), nil
}

// folderSearchPath returns the search API folder path for a resolved folder name.
// For subfolders, the search API requires "parent_name/child_name" format.
func folderSearchPath(resolvedName, input string, folders []folderInfo) string {
	lower := strings.ToLower(strings.TrimSpace(input))
	for _, f := range folders {
		if strings.ToLower(f.Name) != lower && f.ID != input {
			continue
		}
		if f.ParentFolderID == "" || f.ParentFolderID == "0" {
			return resolvedName
		}
		for _, parent := range folders {
			if parent.ID == f.ParentFolderID {
				return parent.Name + "/" + resolvedName
			}
		}
		return resolvedName
	}
	return resolvedName
}

func resolveLabelQueryName(runtime *common.RuntimeContext, mailboxID, input string) (string, error) {
	value := strings.TrimSpace(input)
	if value == "" {
		return "", nil
	}
	if id, ok := resolveLabelSystemID(value); ok {
		return systemLabelSearchName[id], nil
	}
	labels, err := listMailboxLabels(runtime, mailboxID)
	if err != nil {
		return "", err
	}
	name, err := resolveNameValueByNameAllowDuplicates("label", value, mailboxID, labels,
		func(item labelInfo) string { return item.ID },
		func(item labelInfo) string { return item.Name },
	)
	if err != nil {
		// Sub-label names contain the full path (e.g. "parent/child").
		// If exact match fails, try suffix match for child label names.
		if match := matchLabelSuffix(value, labels); match != "" {
			return match, nil
		}
		return "", err
	}
	return name, nil
}

func resolveLabelQueryNameFromID(runtime *common.RuntimeContext, mailboxID, input string) (string, error) {
	value := strings.TrimSpace(input)
	if value == "" {
		return "", nil
	}
	if id, ok := resolveLabelSystemID(value); ok {
		return systemLabelSearchName[id], nil
	}
	labels, err := listMailboxLabels(runtime, mailboxID)
	if err != nil {
		return "", err
	}
	return resolveNameValueByID("label", value, mailboxID, labels,
		func(item labelInfo) string { return item.ID },
		func(item labelInfo) string { return item.Name },
	)
}

// matchLabelSuffix finds a label whose name ends with "/input" (case-insensitive)
// and returns the full label name. Used for search path resolution.
func matchLabelSuffix(input string, labels []labelInfo) string {
	lower := strings.ToLower(input)
	suffix := "/" + lower
	for _, l := range labels {
		name := strings.TrimSpace(l.Name)
		if strings.HasSuffix(strings.ToLower(name), suffix) {
			return name
		}
	}
	return ""
}

// matchLabelSuffixID finds a label whose name ends with "/input" (case-insensitive)
// and returns the label ID. Used for list path resolution.
func matchLabelSuffixID(input string, labels []labelInfo) string {
	lower := strings.ToLower(input)
	suffix := "/" + lower
	for _, l := range labels {
		name := strings.TrimSpace(l.Name)
		if strings.HasSuffix(strings.ToLower(name), suffix) {
			return l.ID
		}
	}
	return ""
}

func resolveFolderNames(runtime *common.RuntimeContext, mailboxID string, values []string) ([]string, error) {
	resolved := make([]string, 0, len(values))
	seen := make(map[string]bool)
	names := make([]string, 0, len(values))
	for _, raw := range values {
		value := strings.TrimSpace(raw)
		if value == "" {
			continue
		}
		if id, ok := resolveFolderSystemAliasOrID(value); ok {
			addUniqueID(&resolved, seen, id)
			continue
		}
		names = append(names, value)
	}
	if len(names) == 0 {
		return resolved, nil
	}

	folders, err := listMailboxFolders(runtime, mailboxID)
	if err != nil {
		return nil, err
	}
	for _, value := range names {
		id, err := resolveByName("folder", value, mailboxID, folders,
			func(item folderInfo) string { return item.ID },
			func(item folderInfo) string { return item.Name },
		)
		if err != nil {
			return nil, err
		}
		addUniqueID(&resolved, seen, id)
	}
	return resolved, nil
}

func resolveLabelNames(runtime *common.RuntimeContext, mailboxID string, values []string) ([]string, error) {
	resolved := make([]string, 0, len(values))
	seen := make(map[string]bool)
	names := make([]string, 0, len(values))
	for _, raw := range values {
		value := strings.TrimSpace(raw)
		if value == "" {
			continue
		}
		if id, ok := resolveLabelSystemID(value); ok {
			addUniqueID(&resolved, seen, id)
			continue
		}
		names = append(names, value)
	}
	if len(names) == 0 {
		return resolved, nil
	}

	labels, err := listMailboxLabels(runtime, mailboxID)
	if err != nil {
		return nil, err
	}
	for _, value := range names {
		id, err := resolveByName("label", value, mailboxID, labels,
			func(item labelInfo) string { return item.ID },
			func(item labelInfo) string { return item.Name },
		)
		if err != nil {
			return nil, err
		}
		addUniqueID(&resolved, seen, id)
	}
	return resolved, nil
}

func resolveFolderSystemAliasOrID(input string) (string, bool) {
	if id, ok := folderAliasToSystemID[strings.ToLower(strings.TrimSpace(input))]; ok {
		return id, true
	}
	return normalizeSystemID(input, folderSystemIDs)
}

func resolveLabelSystemID(input string) (string, bool) {
	return resolveSystemLabel(input)
}

func normalizeSystemID(input string, systemIDs map[string]bool) (string, bool) {
	canonical := strings.ToUpper(strings.TrimSpace(input))
	if canonical == "" {
		return "", false
	}
	if systemIDs[canonical] {
		return canonical, true
	}
	return "", false
}

func addUniqueID(dst *[]string, seen map[string]bool, id string) {
	if id == "" || seen[id] {
		return
	}
	seen[id] = true
	*dst = append(*dst, id)
}

func listMailboxFolders(runtime *common.RuntimeContext, mailboxID string) ([]folderInfo, error) {
	if err := validateFolderReadScope(runtime); err != nil {
		return nil, err
	}
	data, err := runtime.CallAPI("GET", mailboxPath(mailboxID, "folders"), nil, nil)
	if err != nil {
		return nil, output.ErrValidation("unable to resolve --folder: failed to list folders (%v). %s", err, resolveLookupHint("folder", mailboxID))
	}
	items, _ := data["items"].([]interface{})
	folders := make([]folderInfo, 0, len(items))
	for _, item := range items {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		id := strVal(m["id"])
		if id == "" {
			continue
		}
		folders = append(folders, folderInfo{ID: id, Name: strVal(m["name"]), ParentFolderID: strVal(m["parent_folder_id"])})
	}
	return folders, nil
}

func listMailboxLabels(runtime *common.RuntimeContext, mailboxID string) ([]labelInfo, error) {
	if err := validateLabelReadScope(runtime); err != nil {
		return nil, err
	}
	data, err := runtime.CallAPI("GET", mailboxPath(mailboxID, "labels"), nil, nil)
	if err != nil {
		return nil, output.ErrValidation("unable to resolve --label: failed to list labels (%v). %s", err, resolveLookupHint("label", mailboxID))
	}
	items, _ := data["items"].([]interface{})
	labels := make([]labelInfo, 0, len(items))
	for _, item := range items {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		id := strVal(m["id"])
		if id == "" {
			continue
		}
		labels = append(labels, labelInfo{ID: id, Name: strVal(m["name"])})
	}
	return labels, nil
}

func resolveByID[T any](kind, input, mailboxID string, items []T, idFn func(T) string) (string, error) {
	value := strings.TrimSpace(input)
	if value == "" {
		return "", nil
	}
	for _, item := range items {
		if id := idFn(item); id != "" && id == value {
			return id, nil
		}
	}
	return "", output.ErrValidation("%s %q not_exists. %s", kind, value, resolveLookupHint(kind, mailboxID))
}

func resolveByName[T any](kind, input, mailboxID string, items []T, idFn func(T) string, nameFn func(T) string) (string, error) {
	value := strings.TrimSpace(input)
	if value == "" {
		return "", nil
	}

	for _, item := range items {
		if id := idFn(item); id != "" && id == value {
			return "", output.ErrValidation("%s %q looks like an ID; please use %s_id", kind, value, kind)
		}
	}

	lower := strings.ToLower(value)
	matches := make([]string, 0, 2)
	matchSet := make(map[string]bool)
	for _, item := range items {
		name := strings.TrimSpace(nameFn(item))
		if name == "" || strings.ToLower(name) != lower {
			continue
		}
		id := idFn(item)
		if id == "" || matchSet[id] {
			continue
		}
		matchSet[id] = true
		matches = append(matches, id)
	}

	if len(matches) == 1 {
		return matches[0], nil
	}
	if len(matches) > 1 {
		return "", output.ErrValidation("%s name %q matches multiple IDs (%s); please use an ID", kind, value, strings.Join(matches, ","))
	}
	return "", output.ErrValidation("%s %q not_exists. %s", kind, value, resolveLookupHint(kind, mailboxID))
}

func resolveNameValueByID[T any](kind, input, mailboxID string, items []T, idFn func(T) string, nameFn func(T) string) (string, error) {
	value := strings.TrimSpace(input)
	if value == "" {
		return "", nil
	}
	for _, item := range items {
		if id := idFn(item); id != "" && id == value {
			name := strings.TrimSpace(nameFn(item))
			if name == "" {
				return "", output.ErrValidation("%s %q has empty name; cannot use it with query filters", kind, value)
			}
			return name, nil
		}
	}
	return "", output.ErrValidation("%s %q not_exists. %s", kind, value, resolveLookupHint(kind, mailboxID))
}

func resolveNameValueByNameAllowDuplicates[T any](kind, input, mailboxID string, items []T, idFn func(T) string, nameFn func(T) string) (string, error) {
	value := strings.TrimSpace(input)
	if value == "" {
		return "", nil
	}
	for _, item := range items {
		if id := idFn(item); id != "" && id == value {
			return "", output.ErrValidation("%s %q looks like an ID; please use %s_id", kind, value, kind)
		}
	}
	lower := strings.ToLower(value)
	for _, item := range items {
		name := strings.TrimSpace(nameFn(item))
		if name == "" || strings.ToLower(name) != lower {
			continue
		}
		return name, nil
	}
	return "", output.ErrValidation("%s %q not_exists. %s", kind, value, resolveLookupHint(kind, mailboxID))
}

func resolveLookupHint(kind, mailboxID string) string {
	if mailboxID == "" {
		mailboxID = "me"
	}
	switch kind {
	case "folder":
		return fmt.Sprintf("Run `lark-cli mail user_mailbox.folders list --params '{\"user_mailbox_id\":\"%s\"}'` to inspect available folder IDs and names.", mailboxID)
	case "label":
		return fmt.Sprintf("Run `lark-cli api GET '/open-apis/mail/v1/user_mailboxes/%s/labels' --as user` to inspect available label IDs and names.", validate.EncodePathSegment(mailboxID))
	default:
		return ""
	}
}

// fetchFullMessage calls message.get.
// html=true  -> format=full
// html=false -> format=plain_text_full (server omits body_html)
func fetchFullMessage(runtime *common.RuntimeContext, mailboxID, messageID string, html bool) (map[string]interface{}, error) {
	params := map[string]interface{}{"format": messageGetFormat(html)}
	data, err := runtime.CallAPI("GET", mailboxPath(mailboxID, "messages", messageID), params, nil)
	if err != nil {
		return nil, err
	}
	msg, _ := data["message"].(map[string]interface{})
	if msg == nil {
		return nil, fmt.Errorf("API response missing message field")
	}
	return msg, nil
}

// fetchFullMessages calls messages.batch_get and preserves the requested ID order.
// It returns the fetched raw message objects plus any IDs not returned by the API.
func fetchFullMessages(runtime *common.RuntimeContext, mailboxID string, messageIDs []string, html bool) ([]map[string]interface{}, []string, error) {
	if len(messageIDs) == 0 {
		return nil, nil, nil
	}
	const maxBatchGetMessageIDs = 20
	byID := make(map[string]map[string]interface{}, len(messageIDs))
	for start := 0; start < len(messageIDs); start += maxBatchGetMessageIDs {
		end := start + maxBatchGetMessageIDs
		if end > len(messageIDs) {
			end = len(messageIDs)
		}
		data, err := runtime.CallAPI("POST", mailboxPath(mailboxID, "messages", "batch_get"), nil, map[string]interface{}{
			"format":      messageGetFormat(html),
			"message_ids": messageIDs[start:end],
		})
		if err != nil {
			return nil, nil, err
		}
		rawMessages, _ := data["messages"].([]interface{})
		for _, item := range rawMessages {
			msg, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			messageID := strVal(msg["message_id"])
			if messageID == "" {
				continue
			}
			byID[messageID] = msg
		}
	}

	ordered := make([]map[string]interface{}, 0, len(messageIDs))
	missing := make([]string, 0)
	for _, messageID := range messageIDs {
		if msg, ok := byID[messageID]; ok {
			ordered = append(ordered, msg)
			continue
		}
		missing = append(missing, messageID)
	}
	return ordered, missing, nil
}

func messageGetFormat(html bool) string {
	if html {
		return "full"
	}
	return "plain_text_full"
}

// extractAttachmentIDs returns the attachment IDs from a raw message map.
func extractAttachmentIDs(msg map[string]interface{}) []string {
	rawAtts, _ := msg["attachments"].([]interface{})
	ids := make([]string, 0, len(rawAtts))
	for _, item := range rawAtts {
		if att, ok := item.(map[string]interface{}); ok {
			if id := strVal(att["id"]); id != "" {
				ids = append(ids, id)
			}
		}
	}
	return ids
}

type warningEntry struct {
	Code         string `json:"code"`
	Level        string `json:"level"`
	MessageID    string `json:"message_id"`
	AttachmentID string `json:"attachment_id"`
	Retryable    bool   `json:"retryable"`
	Detail       string `json:"detail"`
}

type mailAddressOutput struct {
	Email string `json:"email"`
	Name  string `json:"name"`
}

// mailAddressPair is a name+email pair used for display in HTML and plaintext quote blocks.
type mailAddressPair struct {
	Email string
	Name  string
}

func toAddressPairList(raw []mailAddressOutput) []mailAddressPair {
	out := make([]mailAddressPair, 0, len(raw))
	for _, addr := range raw {
		if addr.Email != "" {
			out = append(out, mailAddressPair{Email: addr.Email, Name: addr.Name})
		}
	}
	return out
}

type mailAttachmentOutput struct {
	ID             string `json:"id"`
	Filename       string `json:"filename"`
	ContentType    string `json:"content_type,omitempty"`
	AttachmentType int    `json:"attachment_type"`
	DownloadURL    string `json:"download_url,omitempty"`
}

type mailImageOutput struct {
	ID          string `json:"id"`
	Filename    string `json:"filename"`
	ContentType string `json:"content_type,omitempty"`
	CID         string `json:"cid"`
	DownloadURL string `json:"download_url,omitempty"`
}

type mailPublicAttachmentOutput struct {
	ID             string `json:"id"`
	Filename       string `json:"filename"`
	ContentType    string `json:"content_type,omitempty"`
	AttachmentType int    `json:"attachment_type,omitempty"`
	IsInline       bool   `json:"is_inline"`
	CID            string `json:"cid,omitempty"`
}

type mailSecurityLevelOutput struct {
	IsRisk               bool   `json:"is_risk"`
	RiskBannerLevel      string `json:"risk_banner_level"`
	RiskBannerReason     string `json:"risk_banner_reason"`
	IsHeaderFromExternal bool   `json:"is_header_from_external"`
	ViaDomain            string `json:"via_domain"`
	SpamBannerType       string `json:"spam_banner_type"`
	SpamUserRuleID       string `json:"spam_user_rule_id"`
	SpamBannerInfo       string `json:"spam_banner_info"`
}

// normalizedMessageForCompose is an internal-only shape used by reply/forward flows.
// It is not the public JSON contract of `mail +message` / `mail +thread`.
type normalizedMessageForCompose struct {
	MessageID            string                   `json:"message_id"`
	ThreadID             string                   `json:"thread_id"`
	SMTPMessageID        string                   `json:"smtp_message_id"`
	Subject              string                   `json:"subject"`
	From                 mailAddressOutput        `json:"from"`
	To                   []mailAddressOutput      `json:"to"`
	CC                   []mailAddressOutput      `json:"cc"`
	BCC                  []mailAddressOutput      `json:"bcc"`
	Date                 string                   `json:"date"`
	InReplyTo            string                   `json:"in_reply_to"`
	ReplyTo              string                   `json:"reply_to,omitempty"`
	ReplyToSMTPMessageID string                   `json:"reply_to_smtp_message_id,omitempty"`
	References           []string                 `json:"references"`
	InternalDate         string                   `json:"internal_date"`
	DateFormatted        string                   `json:"date_formatted"`
	MessageState         int                      `json:"message_state"`
	MessageStateText     string                   `json:"message_state_text"`
	FolderID             string                   `json:"folder_id"`
	LabelIDs             []string                 `json:"label_ids"`
	PriorityType         string                   `json:"priority_type,omitempty"`
	PriorityTypeText     string                   `json:"priority_type_text,omitempty"`
	SecurityLevel        *mailSecurityLevelOutput `json:"security_level,omitempty"`
	BodyPlainText        string                   `json:"body_plain_text"`
	BodyPreview          string                   `json:"body_preview"`
	BodyHTML             string                   `json:"body_html,omitempty"`
	Attachments          []mailAttachmentOutput   `json:"attachments"`
	Images               []mailImageOutput        `json:"images"`
	Warnings             []warningEntry           `json:"warnings,omitempty"`
}

// fetchAttachmentURLs fetches download URLs for the given attachment IDs in batches of 20.
// List params are embedded directly in the URL (SDK workaround for repeated query params).
// It never returns an error: failed batches/IDs are converted to structured warnings so caller can continue.
func fetchAttachmentURLs(runtime *common.RuntimeContext, mailboxID, messageID string, ids []string) (map[string]string, []warningEntry) {
	callAPI := func(url string) (map[string]interface{}, error) {
		return runtime.CallAPI("GET", url, nil, nil)
	}
	emitWarning := func(w warningEntry) {
		fmt.Fprintf(runtime.IO().ErrOut, "warning: code=%s message_id=%s attachment_id=%s retryable=%t detail=%s\n", w.Code, w.MessageID, w.AttachmentID, w.Retryable, w.Detail)
	}
	return fetchAttachmentURLsWith(runtime, mailboxID, messageID, ids, callAPI, emitWarning)
}

func fetchAttachmentURLsWith(
	runtime *common.RuntimeContext,
	mailboxID, messageID string,
	ids []string,
	callAPI func(url string) (map[string]interface{}, error),
	emitWarning func(w warningEntry),
) (map[string]string, []warningEntry) {
	if len(ids) == 0 {
		return nil, nil
	}
	urlMap := make(map[string]string, len(ids))
	warnings := make([]warningEntry, 0)
	const batchSize = 20
	for i := 0; i < len(ids); i += batchSize {
		end := i + batchSize
		if end > len(ids) {
			end = len(ids)
		}
		batch := ids[i:end]

		parts := make([]string, len(batch))
		for j, id := range batch {
			parts[j] = "attachment_ids=" + url.QueryEscape(id)
		}
		apiURL := mailboxPath(mailboxID, "messages", messageID, "attachments", "download_url") +
			"?" + strings.Join(parts, "&")

		data, err := callAPI(apiURL)
		if err != nil {
			warn := warningEntry{
				Code:         "attachment_download_url_api_error",
				Level:        "warning",
				MessageID:    messageID,
				AttachmentID: "",
				Retryable:    true,
				Detail:       err.Error(),
			}
			warnings = append(warnings, warn)
			emitWarning(warn)
			continue
		}

		if urls, ok := data["download_urls"].([]interface{}); ok {
			for _, item := range urls {
				if m, ok := item.(map[string]interface{}); ok {
					attID := strVal(m["attachment_id"])
					dlURL := strVal(m["download_url"])
					if attID != "" {
						urlMap[attID] = dlURL
					}
				}
			}
		}
		if failed, ok := data["failed_ids"].([]interface{}); ok {
			for _, f := range failed {
				if id, ok := f.(string); ok && id != "" {
					warn := warningEntry{
						Code:         "attachment_download_url_failed_id",
						Level:        "warning",
						MessageID:    messageID,
						AttachmentID: id,
						Retryable:    false,
						Detail:       "attachment id returned in failed_ids",
					}
					warnings = append(warnings, warn)
					emitWarning(warn)
				}
			}
		}
	}
	return urlMap, warnings
}

var rawMessageExcludedFields = map[string]struct{}{
	"attachments": {},
}

var derivedMessageFields = []string{
	"draft_id",
	"body_plain_text",
	"body_preview",
	"body_html",
	"attachments",
	"date_formatted",
	"message_state_text",
	"priority_type_text",
}

// buildMessageOutput assembles the public shortcut output from a raw message map and attachment URL map.
//
// Output model:
//   - raw passthrough: safe message metadata fields that do not need special processing
//   - derived fields: decoded body, attachment list, and helper text fields
//
// Raw passthrough excludes:
//   - all `body_*` fields
//   - `attachments`
//
// Derived fields are listed in `derivedMessageFields`.
func buildMessageOutput(msg map[string]interface{}, html bool) map[string]interface{} {
	out := pickSafeMessageFields(msg)
	normalized := buildMessageForCompose(msg, nil, html)

	if draftID := derivedDraftID(msg, normalized.MessageID); draftID != "" {
		out["draft_id"] = draftID
	}
	if normalized.ReplyTo != "" {
		out["reply_to"] = normalized.ReplyTo
	}
	if normalized.ReplyToSMTPMessageID != "" {
		out["reply_to_smtp_message_id"] = normalized.ReplyToSMTPMessageID
	}
	out["date_formatted"] = normalized.DateFormatted
	out["message_state_text"] = normalized.MessageStateText
	if normalized.PriorityType != "" {
		out["priority_type_text"] = normalized.PriorityTypeText
	}
	out["body_plain_text"] = normalized.BodyPlainText
	out["body_preview"] = normalized.BodyPreview
	if html && normalized.BodyHTML != "" {
		out["body_html"] = normalized.BodyHTML
	}
	out["attachments"] = buildPublicAttachments(msg)

	return out
}

func buildPublicAttachments(msg map[string]interface{}) []mailPublicAttachmentOutput {
	rawAtts, _ := msg["attachments"].([]interface{})
	out := make([]mailPublicAttachmentOutput, 0, len(rawAtts))
	for _, item := range rawAtts {
		att, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		id := strVal(att["id"])
		filename := strVal(att["filename"])
		contentType := resolveAttachmentContentType(att, filename)
		isInline, _ := att["is_inline"].(bool)
		out = append(out, mailPublicAttachmentOutput{
			ID:             id,
			Filename:       filename,
			ContentType:    contentType,
			AttachmentType: intVal(att["attachment_type"]),
			IsInline:       isInline,
			CID:            strVal(att["cid"]),
		})
	}
	return out
}

func derivedDraftID(msg map[string]interface{}, messageID string) string {
	if draftID := strVal(msg["draft_id"]); draftID != "" {
		return draftID
	}
	if strings.EqualFold(strVal(msg["folder_id"]), "DRAFT") {
		return messageID
	}
	return ""
}

// buildMessageForCompose assembles the internal normalized message structure used by compose flows.
//   - base64url-decodes body fields
//   - splits attachments into images (is_inline=true) and attachments (is_inline=false)
//   - omits body_html when html=false
//   - falls back body_plain_text → body_preview when empty
//   - sanitizes body_plain_text for terminal output (strips ANSI escapes and bare CR)
func buildMessageForCompose(msg map[string]interface{}, urlMap map[string]string, html bool) normalizedMessageForCompose {
	out := normalizedMessageForCompose{
		MessageID:     strVal(msg["message_id"]),
		ThreadID:      strVal(msg["thread_id"]),
		SMTPMessageID: strVal(msg["smtp_message_id"]),
		Subject:       strVal(msg["subject"]),
		From:          toAddressObject(msg["head_from"]),
		To:            toAddressList(msg["to"]),
		CC:            toAddressList(msg["cc"]),
		BCC:           toAddressList(msg["bcc"]),
		Date:          strVal(msg["date"]),
		InReplyTo:     strVal(msg["in_reply_to"]),
		References:    toStringList(msg["references"]),
	}
	out.ReplyTo = strVal(msg["reply_to"])
	out.ReplyToSMTPMessageID = strVal(msg["reply_to_smtp_message_id"])

	// State
	internalDate := strVal(msg["internal_date"])
	out.InternalDate = internalDate
	out.DateFormatted = common.FormatTime(internalDate)
	state := intVal(msg["message_state"])
	out.MessageState = state
	out.MessageStateText = messageStateText(state)
	out.FolderID = strVal(msg["folder_id"])
	out.LabelIDs = toStringList(msg["label_ids"])
	priorityType := strVal(msg["priority_type"])
	out.PriorityType = priorityType
	if priorityType != "" {
		out.PriorityTypeText = priorityTypeText(priorityType)
	}
	if securityLevel := toSecurityLevel(msg["security_level"]); securityLevel != nil {
		out.SecurityLevel = securityLevel
	}

	// Body
	plainText := decodeBase64URL(strVal(msg["body_plain_text"]))
	preview := decodeBase64URL(strVal(msg["body_preview"]))
	if plainText == "" {
		plainText = preview
	}
	out.BodyPlainText = sanitizeForTerminal(plainText)
	out.BodyPreview = preview
	if html {
		out.BodyHTML = decodeBase64URL(strVal(msg["body_html"]))
	}

	// Attachments
	attachments := make([]mailAttachmentOutput, 0)
	images := make([]mailImageOutput, 0)
	if rawAtts, ok := msg["attachments"].([]interface{}); ok {
		for _, item := range rawAtts {
			att, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			id := strVal(att["id"])
			filename := strVal(att["filename"])
			attType := intVal(att["attachment_type"])
			isInline, _ := att["is_inline"].(bool)
			cid := strVal(att["cid"])
			contentType := resolveAttachmentContentType(att, filename)
			dlURL := urlMap[id]

			if isInline {
				images = append(images, mailImageOutput{
					ID:          id,
					Filename:    filename,
					ContentType: contentType,
					CID:         cid,
					DownloadURL: dlURL,
				})
			} else {
				attachments = append(attachments, mailAttachmentOutput{
					ID:             id,
					Filename:       filename,
					ContentType:    contentType,
					AttachmentType: attType,
					DownloadURL:    dlURL,
				})
			}
		}
	}
	out.Attachments = attachments
	out.Images = images

	return out
}

func pickSafeMessageFields(msg map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(msg))
	for key, value := range msg {
		if !shouldExposeRawMessageField(key) {
			continue
		}
		out[key] = value
	}
	return out
}

func shouldExposeRawMessageField(key string) bool {
	if strings.HasPrefix(key, "body_") {
		return false
	}
	_, blocked := rawMessageExcludedFields[key]
	return !blocked
}

// attachmentTypeLarge is the API value for a large attachment that is already
// embedded as a download link inside the message body. These must not be
// downloaded and re-attached during forward: the link in the body is sufficient
// and downloading could cause OOM for very large files.
const attachmentTypeLarge = 2

type forwardSourceAttachment struct {
	ID             string
	Filename       string
	ContentType    string
	AttachmentType int // 1=normal, 2=large (link in body, skip download)
	DownloadURL    string
}

type inlineSourcePart struct {
	ID          string
	Filename    string
	ContentType string
	CID         string
	DownloadURL string
}

type composeSourceMessage struct {
	Original           originalMessage
	ForwardAttachments []forwardSourceAttachment
	InlineImages       []inlineSourcePart
}

// fetchComposeSourceMessage loads a message via the +message pipeline and converts it
// to compose-friendly data (quote metadata + forward attachments).
func fetchComposeSourceMessage(runtime *common.RuntimeContext, mailboxID, messageID string) (composeSourceMessage, error) {
	msg, err := fetchFullMessage(runtime, mailboxID, messageID, true)
	if err != nil {
		return composeSourceMessage{}, err
	}
	attIDs := extractAttachmentIDs(msg)
	urlMap, _ := fetchAttachmentURLs(runtime, mailboxID, messageID, attIDs)
	out := buildMessageForCompose(msg, urlMap, true)
	orig := toOriginalMessageForCompose(out)
	return composeSourceMessage{
		Original:           orig,
		ForwardAttachments: toForwardSourceAttachments(out),
		InlineImages:       toInlineSourceParts(out),
	}, nil
}

// validateForwardAttachmentURLs checks that all forwarded attachments (non-inline)
// have valid download URLs. Inline images are checked separately by validateInlineImageURLs.
func validateForwardAttachmentURLs(src composeSourceMessage) error {
	var missing []string
	for _, att := range src.ForwardAttachments {
		if att.DownloadURL == "" {
			missing = append(missing, fmt.Sprintf("attachment %q (%s)", att.Filename, att.ID))
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("failed to fetch download URLs for: %s", strings.Join(missing, ", "))
	}
	return nil
}

// validateInlineImageURLs checks only inline images have valid download URLs.
// Use for HTML reply/reply-all where inline images are embedded in the quoted body.
func validateInlineImageURLs(src composeSourceMessage) error {
	var missing []string
	for _, img := range src.InlineImages {
		if img.DownloadURL == "" {
			missing = append(missing, fmt.Sprintf("inline image %q (%s)", img.Filename, img.ID))
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("failed to fetch download URLs for: %s", strings.Join(missing, ", "))
	}
	return nil
}

func toOriginalMessageForCompose(out normalizedMessageForCompose) originalMessage {
	fromEmail, fromName := out.From.Email, out.From.Name
	toList := toAddressEmailList(out.To)
	ccList := toAddressEmailList(out.CC)
	toFullList := toAddressPairList(out.To)
	ccFullList := toAddressPairList(out.CC)
	headTo := ""
	if len(toList) > 0 {
		headTo = toList[0]
	}

	headDate := ""
	if internalDate := out.InternalDate; internalDate != "" {
		if ms, err := strconv.ParseInt(internalDate, 10, 64); err == nil {
			headDate = formatMailDate(ms, detectSubjectLang(out.Subject))
		}
	}

	bodyHTML := out.BodyHTML
	bodyText := out.BodyPlainText
	bodyRaw := bodyHTML
	if bodyRaw == "" {
		bodyRaw = bodyText
	}

	references := ""
	if len(out.References) > 0 {
		references = strings.Join(out.References, " ")
	}

	// Strip CR and LF from the inherited subject to prevent header injection when
	// this value is later passed to emlbuilder.Subject() in reply/forward flows.
	// A malicious source email could carry "\r\nBcc: evil@evil.com" in its Subject.
	safeSubject := strings.NewReplacer("\r", "", "\n", "").Replace(out.Subject)

	return originalMessage{
		subject:              safeSubject,
		headFrom:             fromEmail,
		headFromName:         fromName,
		headTo:               headTo,
		replyTo:              out.ReplyTo,
		replyToSMTPMessageID: out.ReplyToSMTPMessageID,
		smtpMessageId:        out.SMTPMessageID,
		threadId:             out.ThreadID,
		bodyRaw:              bodyRaw,
		headDate:             headDate,
		references:           references,
		toAddresses:          toList,
		ccAddresses:          ccList,
		toAddressesFull:      toFullList,
		ccAddressesFull:      ccFullList,
	}
}

func toForwardSourceAttachments(out normalizedMessageForCompose) []forwardSourceAttachment {
	atts := make([]forwardSourceAttachment, 0, len(out.Attachments))
	for _, att := range out.Attachments {
		atts = append(atts, forwardSourceAttachment{
			ID:             att.ID,
			Filename:       att.Filename,
			ContentType:    att.ContentType,
			AttachmentType: att.AttachmentType,
			DownloadURL:    att.DownloadURL,
		})
	}
	return atts
}

func toInlineSourceParts(out normalizedMessageForCompose) []inlineSourcePart {
	parts := make([]inlineSourcePart, 0, len(out.Images))
	for _, img := range out.Images {
		if img.CID == "" {
			continue
		}
		parts = append(parts, inlineSourcePart{
			ID:          img.ID,
			Filename:    img.Filename,
			ContentType: img.ContentType,
			CID:         img.CID,
			DownloadURL: img.DownloadURL,
		})
	}
	return parts
}

// downloadAttachmentContent fetches the content at downloadURL.
// Lark pre-signed download URLs embed an authcode in the query string and do
// not require an Authorization header, so we never send the Bearer token.
func downloadAttachmentContent(runtime *common.RuntimeContext, downloadURL string) ([]byte, error) {
	u, err := url.Parse(downloadURL)
	if err != nil {
		return nil, fmt.Errorf("invalid attachment download URL: %w", err)
	}
	if u.Scheme != "https" {
		return nil, fmt.Errorf("attachment download URL must use https (got %q)", u.Scheme)
	}
	if u.Host == "" {
		return nil, fmt.Errorf("attachment download URL has no host")
	}

	httpClient, err := runtime.Factory.HttpClient()
	if err != nil {
		return nil, fmt.Errorf("failed to get HTTP client: %w", err)
	}
	req, err := http.NewRequestWithContext(runtime.Ctx(), http.MethodGet, downloadURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to build attachment download request: %w", err)
	}
	// Do NOT send Authorization: the download_url is a pre-signed URL with an
	// authcode embedded in the query string. Attaching the Bearer token would
	// leak it to whatever host the URL points at (SSRF / token exfiltration).
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to download attachment: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("failed to download attachment: HTTP %d", resp.StatusCode)
	}
	limitedReader := io.LimitReader(resp.Body, int64(MaxAttachmentDownloadBytes)+1)
	data, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil, fmt.Errorf("failed to read attachment content: %w", err)
	}
	if len(data) > MaxAttachmentDownloadBytes {
		return nil, fmt.Errorf("attachment download exceeds %d MB size limit", MaxAttachmentDownloadBytes/1024/1024)
	}
	return data, nil
}

// --- internal helpers ---

func strVal(v interface{}) string {
	s, _ := v.(string)
	return s
}

func intVal(v interface{}) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case json.Number:
		i, _ := n.Int64()
		return int(i)
	}
	return 0
}

func decodeBase64URL(s string) string {
	if s == "" {
		return ""
	}
	b, err := base64.URLEncoding.DecodeString(s)
	if err != nil {
		b, err = base64.RawURLEncoding.DecodeString(s)
		if err != nil {
			return s
		}
	}
	return string(b)
}

// decodeBodyFields decodes body_html and body_plain_text from src into dst.
// Fields absent or empty in src are skipped. Both padding and no-padding base64url variants
// are accepted by the underlying decodeBase64URL call.
func decodeBodyFields(src, dst map[string]interface{}) {
	for _, field := range []string{"body_html", "body_plain_text"} {
		if s := strVal(src[field]); s != "" {
			dst[field] = decodeBase64URL(s)
		}
	}
}

// ansiEscapeRe matches ANSI CSI escape sequences (ESC '[' ... <final byte>).
var ansiEscapeRe = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

// sanitizeForTerminal strips ANSI escape sequences, bare CR characters, and
// dangerous Unicode code points (BiDi overrides, zero-width chars, etc.) to
// prevent terminal injection from untrusted email content.
func sanitizeForTerminal(s string) string {
	s = ansiEscapeRe.ReplaceAllString(s, "")
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r == '\r' {
			continue
		}
		if common.IsDangerousUnicode(r) {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func toAddressObject(v interface{}) mailAddressOutput {
	if m, ok := v.(map[string]interface{}); ok {
		return mailAddressOutput{Email: strVal(m["mail_address"]), Name: strVal(m["name"])}
	}
	return mailAddressOutput{}
}

func toAddressList(v interface{}) []mailAddressOutput {
	list, _ := v.([]interface{})
	out := make([]mailAddressOutput, 0, len(list))
	for _, item := range list {
		out = append(out, toAddressObject(item))
	}
	return out
}

func toAddressEmailList(raw []mailAddressOutput) []string {
	out := make([]string, 0, len(raw))
	for _, addr := range raw {
		email := addr.Email
		if email != "" {
			out = append(out, email)
		}
	}
	return out
}

func toStringList(v interface{}) []string {
	list, _ := v.([]interface{})
	out := make([]string, 0, len(list))
	for _, item := range list {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func toSecurityLevel(v interface{}) *mailSecurityLevelOutput {
	raw, ok := v.(map[string]interface{})
	if !ok || raw == nil {
		return nil
	}
	riskBannerLevel := strVal(raw["risk_banner_level"])
	riskBannerReason := strVal(raw["risk_banner_reason"])
	spamBannerType := strVal(raw["spam_banner_type"])
	return &mailSecurityLevelOutput{
		IsRisk:               boolVal(raw["is_risk"]),
		RiskBannerLevel:      riskBannerLevel,
		RiskBannerReason:     riskBannerReason,
		IsHeaderFromExternal: boolVal(raw["is_header_from_external"]),
		ViaDomain:            strVal(raw["via_domain"]),
		SpamBannerType:       spamBannerType,
		SpamUserRuleID:       strVal(raw["spam_user_rule_id"]),
		SpamBannerInfo:       strVal(raw["spam_banner_info"]),
	}
}

func boolVal(v interface{}) bool {
	b, _ := v.(bool)
	return b
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func resolveAttachmentContentType(att map[string]interface{}, filename string) string {
	if ct := strVal(att["content_type"]); ct != "" {
		return ct
	}
	if ext := strings.ToLower(filepath.Ext(filename)); ext != "" {
		if ct := mime.TypeByExtension(ext); ct != "" {
			return ct
		}
	}
	return "application/octet-stream"
}

func messageStateText(state int) string {
	switch state {
	case 1:
		return "received"
	case 2:
		return "sent"
	case 3:
		return "draft"
	default:
		return "unknown"
	}
}

func priorityTypeText(priorityType string) string {
	switch priorityType {
	case "0":
		return "unknown"
	case "1":
		return "high"
	case "3":
		return "normal"
	case "5":
		return "low"
	default:
		return "unknown"
	}
}

// parseNetAddrs converts a comma-separated address string to []net/mail.Address.
// It reuses ParseMailboxList for display-name-aware parsing and deduplicates
// by email address (case-insensitive), preserving the first occurrence.
func parseNetAddrs(raw string) []netmail.Address {
	boxes := ParseMailboxList(raw)
	seen := make(map[string]bool, len(boxes))
	out := make([]netmail.Address, 0, len(boxes))
	for _, m := range boxes {
		key := strings.ToLower(m.Email)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, netmail.Address{Name: m.Name, Address: m.Email})
	}
	return out
}

// mergeAddrLists merges two comma-separated address lists, deduplicating by
// email (case-insensitive). Addresses in base come first; addresses in extra
// that already appear in base are silently dropped.
func mergeAddrLists(base, extra string) string {
	if extra == "" {
		return base
	}
	if base == "" {
		return extra
	}
	seen := make(map[string]bool)
	for _, m := range ParseMailboxList(base) {
		seen[strings.ToLower(m.Email)] = true
	}
	var additions []string
	for _, m := range ParseMailboxList(extra) {
		lower := strings.ToLower(m.Email)
		if seen[lower] {
			continue
		}
		seen[lower] = true
		additions = append(additions, m.String())
	}
	if len(additions) == 0 {
		return base
	}
	return base + ", " + strings.Join(additions, ", ")
}

// ---- Compose domain types --------------------------------------------------

// originalMessage holds the metadata and body extracted from the original email.
type originalMessage struct {
	subject              string
	headFrom             string
	headFromName         string // display name of sender, for attribution line
	headTo               string // first recipient (likely current user's email)
	replyTo              string // Reply-To address; reply/reply-all should prefer this over headFrom
	replyToSMTPMessageID string // SMTP Message-ID of the Reply-To target
	smtpMessageId        string
	threadId             string
	bodyRaw              string            // raw body from API (may be HTML)
	headDate             string            // Date header, for attribution line
	references           string            // space-separated RFC 2822 References chain from original
	toAddresses          []string          // email-only list, used by reply-all recipient logic
	ccAddresses          []string          // email-only list, used by reply-all recipient logic
	toAddressesFull      []mailAddressPair // name+email pairs for quote display
	ccAddressesFull      []mailAddressPair // name+email pairs for quote display
}

func normalizeMessageID(id string) string {
	trimmed := strings.TrimSpace(id)
	trimmed = strings.TrimPrefix(trimmed, "<")
	trimmed = strings.TrimSuffix(trimmed, ">")
	return strings.TrimSpace(trimmed)
}

func normalizeInlineCID(cid string) string {
	trimmed := strings.TrimSpace(cid)
	if len(trimmed) >= 4 && strings.EqualFold(trimmed[:4], "cid:") {
		trimmed = trimmed[4:]
	}
	trimmed = strings.TrimPrefix(trimmed, "<")
	trimmed = strings.TrimSuffix(trimmed, ">")
	return strings.TrimSpace(trimmed)
}

// validateInlineCIDs checks bidirectional CID consistency between HTML body and
// inline MIME parts — the same checks as postProcessInlineImages in draft-edit.
//  1. Every cid: reference in HTML must have a corresponding inline part (checked
//     against userCIDs + extraCIDs combined).
//  2. Every user-provided inline part must be referenced in HTML (orphan check
//     against userCIDs only — extraCIDs such as source-message images in
//     reply/forward are excluded because quoting may drop some references).
func validateInlineCIDs(html string, userCIDs, extraCIDs []string) error {
	allCIDs := append(append([]string{}, userCIDs...), extraCIDs...)
	if err := draftpkg.ValidateCIDReferences(html, allCIDs); err != nil {
		return err
	}
	if len(userCIDs) > 0 {
		orphaned := draftpkg.FindOrphanedCIDs(html, userCIDs)
		if len(orphaned) > 0 {
			return fmt.Errorf("inline images with cids %v are not referenced by any <img src=\"cid:...\"> in the HTML body and will appear as unexpected attachments; remove unused --inline entries or add matching <img> tags", orphaned)
		}
	}
	return nil
}

func addInlineImagesToBuilder(runtime *common.RuntimeContext, bld emlbuilder.Builder, images []inlineSourcePart) (emlbuilder.Builder, []string, error) {
	var cids []string
	for _, img := range images {
		content, err := downloadAttachmentContent(runtime, img.DownloadURL)
		if err != nil {
			return bld, nil, fmt.Errorf("failed to download inline resource %s: %w", img.Filename, err)
		}
		cid := normalizeInlineCID(img.CID)
		if cid == "" {
			continue
		}
		contentType := img.ContentType
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		bld = bld.AddInline(content, contentType, img.Filename, cid)
		cids = append(cids, cid)
	}
	return bld, cids, nil
}

// InlineSpec represents one inline image entry from the --inline JSON array.
// CID must be a valid RFC 2822 content-id (e.g. a random hex string).
// FilePath is the local path to the image file.
type InlineSpec struct {
	CID      string `json:"cid"`
	FilePath string `json:"file_path"`
}

// parseInlineSpecs parses the --inline flag value as a JSON array of InlineSpec.
// Returns an empty slice when raw is empty.
func parseInlineSpecs(raw string) ([]InlineSpec, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var specs []InlineSpec
	if err := json.Unmarshal([]byte(raw), &specs); err != nil {
		return nil, fmt.Errorf("--inline must be a JSON array, e.g. '[{\"cid\":\"a1b2c3d4e5f6a7b8c9d0\",\"file_path\":\"./banner.png\"}]': %w", err)
	}
	for i, s := range specs {
		if strings.TrimSpace(s.CID) == "" {
			return nil, fmt.Errorf("--inline entry %d: \"cid\" must not be empty", i)
		}
		if strings.TrimSpace(s.FilePath) == "" {
			return nil, fmt.Errorf("--inline entry %d: \"file_path\" must not be empty", i)
		}
	}
	return specs, nil
}

// inlineSpecFilePaths returns the file paths from a slice of InlineSpec, for use in size checks.
func inlineSpecFilePaths(specs []InlineSpec) []string {
	if len(specs) == 0 {
		return nil
	}
	paths := make([]string, len(specs))
	for i, s := range specs {
		paths[i] = s.FilePath
	}
	return paths
}

// checkAttachmentSizeLimit returns an error if the combined attachment count exceeds
// MaxAttachmentCount or the combined size exceeds MaxAttachmentBytes.
// filePaths are read via os.Stat (no full read); extraBytes / extraCount account for
// already-loaded content (e.g. downloaded original attachments in +forward).
func checkAttachmentSizeLimit(fio fileio.FileIO, filePaths []string, extraBytes int64, extraCount ...int) error {
	extra := 0
	for _, c := range extraCount {
		extra += c
	}
	total := extra + len(filePaths)
	if total > MaxAttachmentCount {
		return fmt.Errorf("attachment count %d exceeds the limit of %d", total, MaxAttachmentCount)
	}
	totalBytes := extraBytes
	for _, p := range filePaths {
		info, err := fio.Stat(p)
		if err != nil {
			if errors.Is(err, fileio.ErrPathValidation) {
				return fmt.Errorf("unsafe attachment path %s: %w", p, err)
			}
			return fmt.Errorf("failed to stat attachment %s: %w", p, err)
		}
		totalBytes += info.Size()
	}
	if totalBytes > MaxAttachmentBytes {
		return fmt.Errorf("total attachment size %.1f MB exceeds the 25 MB limit",
			float64(totalBytes)/1024/1024)
	}
	return nil
}

// validateConfirmSendScope checks that the user's token includes the
// mail:user_mailbox.message:send scope when --confirm-send is set.
// This scope is not declared in the shortcut's static Scopes (to keep the
// default draft-only path accessible without the sensitive send permission),
// so we validate it dynamically here.
func validateConfirmSendScope(runtime *common.RuntimeContext) error {
	if !runtime.Bool("confirm-send") {
		return nil
	}
	appID := runtime.Config.AppID
	userOpenId := runtime.UserOpenId()
	if appID == "" || userOpenId == "" {
		return nil
	}
	stored := auth.GetStoredToken(appID, userOpenId)
	if stored == nil {
		return nil
	}
	required := []string{"mail:user_mailbox.message:send"}
	if missing := auth.MissingScopes(stored.Scope, required); len(missing) > 0 {
		return output.ErrWithHint(output.ExitAuth, "missing_scope",
			fmt.Sprintf("--confirm-send requires scope: %s", strings.Join(missing, ", ")),
			fmt.Sprintf("run `lark-cli auth login --scope \"%s\"` to grant the send permission", strings.Join(missing, " ")))
	}
	return nil
}

// validateFolderReadScope checks that the user's token includes the
// mail:user_mailbox.folder:read scope. Called on-demand by listMailboxFolders
// before hitting the folders API. System folders are resolved locally and
// never reach this check.
func validateFolderReadScope(runtime *common.RuntimeContext) error {
	appID := runtime.Config.AppID
	userOpenId := runtime.UserOpenId()
	if appID == "" || userOpenId == "" {
		return nil
	}
	stored := auth.GetStoredToken(appID, userOpenId)
	if stored == nil {
		return nil
	}
	required := []string{"mail:user_mailbox.folder:read"}
	if missing := auth.MissingScopes(stored.Scope, required); len(missing) > 0 {
		return output.ErrWithHint(output.ExitAuth, "missing_scope",
			fmt.Sprintf("folder resolution requires scope: %s", strings.Join(missing, ", ")),
			fmt.Sprintf("run `lark-cli auth login --scope \"%s\"` to grant folder read permission", strings.Join(missing, " ")))
	}
	return nil
}

// validateLabelReadScope checks that the user's token includes the
// mail:user_mailbox.message:modify scope. Called on-demand by listMailboxLabels
// before hitting the labels API. System labels are resolved locally and
// never reach this check.
func validateLabelReadScope(runtime *common.RuntimeContext) error {
	appID := runtime.Config.AppID
	userOpenId := runtime.UserOpenId()
	if appID == "" || userOpenId == "" {
		return nil
	}
	stored := auth.GetStoredToken(appID, userOpenId)
	if stored == nil {
		return nil
	}
	required := []string{"mail:user_mailbox.message:modify"}
	if missing := auth.MissingScopes(stored.Scope, required); len(missing) > 0 {
		return output.ErrWithHint(output.ExitAuth, "missing_scope",
			fmt.Sprintf("label resolution requires scope: %s", strings.Join(missing, ", ")),
			fmt.Sprintf("run `lark-cli auth login --scope \"%s\"` to grant label access permission", strings.Join(missing, " ")))
	}
	return nil
}

func validateComposeHasAtLeastOneRecipient(to, cc, bcc string) error {
	if strings.TrimSpace(to) == "" && strings.TrimSpace(cc) == "" && strings.TrimSpace(bcc) == "" {
		return fmt.Errorf("at least one recipient (--to, --cc, or --bcc) is required")
	}
	return validateRecipientCount(to, cc, bcc)
}

// validateRecipientCount checks that the total number of recipients across
// To, CC, and BCC does not exceed MaxRecipientCount.
func validateRecipientCount(to, cc, bcc string) error {
	count := len(ParseMailboxList(to)) + len(ParseMailboxList(cc)) + len(ParseMailboxList(bcc))
	if count > MaxRecipientCount {
		return fmt.Errorf("total recipient count %d exceeds the limit of %d (To + CC + BCC combined)", count, MaxRecipientCount)
	}
	return nil
}

func validateComposeInlineAndAttachments(fio fileio.FileIO, attachFlag, inlineFlag string, plainText bool, body string) error {
	if strings.TrimSpace(inlineFlag) != "" {
		if plainText {
			return fmt.Errorf("--inline is not supported with --plain-text (inline images require HTML body)")
		}
		if body != "" && !bodyIsHTML(body) {
			return fmt.Errorf("--inline requires an HTML body (the provided body appears to be plain text; add HTML tags or remove --inline)")
		}
	}
	// Validate explicitly provided files (--attach + --inline) early so that
	// dry-run and reply/forward can catch local errors before Execute.
	// Auto-resolved local images are only known at Execute time, so Execute
	// performs a second, complete size check that includes them.
	inlineSpecs, err := parseInlineSpecs(inlineFlag)
	if err != nil {
		return err
	}
	allFiles := append(splitByComma(attachFlag), inlineSpecFilePaths(inlineSpecs)...)
	return checkAttachmentSizeLimit(fio, allFiles, 0)
}
