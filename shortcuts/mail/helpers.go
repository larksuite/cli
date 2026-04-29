// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"encoding/base64"
	"encoding/json"
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
	"time"

	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
	draftpkg "github.com/larksuite/cli/shortcuts/mail/draft"
	"github.com/larksuite/cli/shortcuts/mail/emlbuilder"
	"github.com/larksuite/cli/shortcuts/mail/ics"
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

// hintReadReceiptRequest prints a stderr tip when a message that the caller
// just read requested a read receipt (carries the READ_RECEIPT_REQUEST label).
// The tip is emitted at CLI level so any caller — agents that read SKILL.md
// and those that don't — sees the prompt. Privacy is sensitive here: sending
// a receipt tells the remote party "I have read your message", so the tip
// explicitly instructs the caller to ask the user before responding.
//
// All four interpolated values (fromEmail, subject, mailboxID, messageID)
// come from untrusted email content or raw API input; they are run through
// sanitizeForSingleLine (for fromEmail) / %q (for subject) / shellQuoteForHint
// (for the command-line values) so a crafted "From: x@y.com\ntip: reply
// harmless-looking-addr@attacker..." can't forge extra tip lines, and values
// with shell metacharacters survive copy-paste intact.
func hintReadReceiptRequest(runtime *common.RuntimeContext, mailboxID, messageID, fromEmail, subject string) {
	fmt.Fprintf(runtime.IO().ErrOut,
		"tip: sender requested a read receipt (READ_RECEIPT_REQUEST).\n"+
			"  - do NOT auto-act; ask the user first (from=%s, subject=%q)\n"+
			"  - if the user agrees to confirm they have read it:\n"+
			"    lark-cli mail +send-receipt --mailbox '%s' --message-id '%s' --yes\n"+
			"  - if the user wants to dismiss the banner without sending a receipt:\n"+
			"    lark-cli mail +decline-receipt --mailbox '%s' --message-id '%s'\n",
		sanitizeForSingleLine(fromEmail), sanitizeForSingleLine(subject),
		shellQuoteForHint(mailboxID), shellQuoteForHint(messageID),
		shellQuoteForHint(mailboxID), shellQuoteForHint(messageID))
}

// shellQuoteForHint returns s sanitized for single-line terminal output AND
// safe to embed inside single-quoted shell arguments: each single quote in
// the payload is rewritten as '\” (close-quote, escaped quote, re-open
// quote). Callers are expected to wrap the result in outer single quotes,
// as hintReadReceiptRequest does in its format string. Use this only for
// user-copy-paste hints, not for building commands that the CLI itself
// executes.
func shellQuoteForHint(s string) string {
	return strings.ReplaceAll(sanitizeForSingleLine(s), "'", `'\''`)
}

// requireSenderForRequestReceipt returns a validation error when --request-
// receipt is set but no sender address could be resolved. The Disposition-
// Notification-To header can only be addressed to a known sender — silently
// dropping the header when senderEmail is empty would mislead the caller into
// believing a receipt was requested when it wasn't. Intended to be called
// from a shortcut's Execute right after the sender address has been resolved.
//
// The error wording is deliberately generic about recovery: compose shortcuts
// (+send, +reply, +reply-all, +forward, +draft-create) can accept --from to
// set the sender, but +draft-edit's --from names the mailbox that owns the
// draft, not the DNT address — for that case the recovery is to make sure
// the draft already has a valid From header. Pointing at --from unconditionally
// would send +draft-edit users to the wrong flag.
func requireSenderForRequestReceipt(runtime *common.RuntimeContext, senderEmail string) error {
	if !runtime.Bool("request-receipt") {
		return nil
	}
	if strings.TrimSpace(senderEmail) == "" {
		return output.ErrValidation(
			"--request-receipt requires a resolvable sender address; specify a sender address where supported, or ensure the draft has a From address")
	}
	return nil
}

// validateHeaderAddress rejects addresses that cannot be safely embedded in
// a MIME header value: anything with a control character (CR / LF / DEL /
// other C0) or a dangerous Unicode code point (BiDi / zero-width / line
// separator) would let a malicious From header inject additional headers or
// visually spoof a recipient.
//
// This mirrors emlbuilder.validateHeaderValue and exists separately for
// call sites that build header patches directly (e.g. mail_draft_edit
// synthesizing a set_header op for Disposition-Notification-To) without
// going through the builder.
func validateHeaderAddress(addr string) error {
	for _, r := range addr {
		if r != '\t' && (r < 0x20 || r == 0x7f) {
			return fmt.Errorf("address contains control character: %q", addr)
		}
		if common.IsDangerousUnicode(r) {
			return fmt.Errorf("address contains dangerous Unicode code point: %q", addr)
		}
	}
	return nil
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
			"calendar_event":                         "Parsed calendar invitation; present when the email contains a text/calendar part",
			"calendar_event.method":                  "iTIP method, e.g. REQUEST, CANCEL, REPLY",
			"calendar_event.uid":                     "Globally unique event identifier (UID property)",
			"calendar_event.summary":                 "Event title (SUMMARY property)",
			"calendar_event.start":                   "Event start time in RFC 3339 / ISO 8601 format (UTC)",
			"calendar_event.end":                     "Event end time in RFC 3339 / ISO 8601 format (UTC)",
			"calendar_event.location":                "Event location string; omitted when not set",
			"calendar_event.organizer":               "Organizer email address",
			"calendar_event.attendees":               "List of attendee email addresses",
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

// extractPrimaryEmail returns the user's primary email address from a
// mailbox profile API response (key "primary_email_address"), or "" when the
// field is missing or empty.
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

// folderInfo is the normalized local representation of a mailbox folder,
// used by the folder-resolution helpers.
type folderInfo struct {
	ID             string
	Name           string
	ParentFolderID string
}

// labelInfo is the normalized local representation of a mailbox label,
// used by the label-resolution helpers.
type labelInfo struct {
	ID   string
	Name string
}

// resolveFolderID accepts either a folder ID or a folder name and returns
// the canonical folder ID. System folder aliases (INBOX, SENT, etc.) are
// resolved locally without an API call; custom folders are looked up via
// the mailbox folders endpoint.
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

// resolveFolderName accepts either a folder ID or a folder name and returns
// the human-readable folder name. Used for output rendering where the user
// wants to see the name they originally chose, not the opaque ID.
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

// resolveLabelID accepts either a label ID or a label name and returns the
// canonical label ID. System label aliases (UNREAD, STARRED, etc.) resolve
// locally; custom labels are looked up via the mailbox labels endpoint.
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

// resolveLabelName accepts either a label ID or a label name and returns
// the human-readable label name (mirror of resolveFolderName for labels).
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

// resolveFolderQueryName resolves a folder ID or name to the API-side query
// value (search-style folder syntax). Used by +triage / search to translate
// user-facing folder identifiers into API-acceptable strings.
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

// resolveFolderQueryNameFromID resolves a folder ID (already known) to its
// API-side query value, skipping the by-name lookup path.
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

// resolveLabelQueryName mirrors resolveFolderQueryName for labels: returns
// the search-style label query value from a label ID or name.
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

// resolveLabelQueryNameFromID mirrors resolveFolderQueryNameFromID for
// labels: shortcut path when the label ID is already known.
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

// resolveFolderNames resolves a list of folder IDs / names to their
// human-readable names. Stops at the first error; partial results are not
// returned.
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

// resolveLabelNames is the label-side counterpart of resolveFolderNames.
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

// resolveFolderSystemAliasOrID returns the canonical system folder ID for
// the given input (an alias like "INBOX" or an ID). Returns (id, true) when
// recognised; ("", false) for non-system inputs.
func resolveFolderSystemAliasOrID(input string) (string, bool) {
	if id, ok := folderAliasToSystemID[strings.ToLower(strings.TrimSpace(input))]; ok {
		return id, true
	}
	return normalizeSystemID(input, folderSystemIDs)
}

// resolveLabelSystemID is the label counterpart of
// resolveFolderSystemAliasOrID: returns the system label ID when input
// matches a known system label.
func resolveLabelSystemID(input string) (string, bool) {
	return resolveSystemLabel(input)
}

// normalizeSystemID checks whether input is a known system identifier
// listed in systemIDs and returns the canonical form. Returns ("", false)
// when input does not match any system ID.
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

// addUniqueID appends id to *dst when id is non-empty and not already in
// the seen set. Both dst and seen are updated in place.
func addUniqueID(dst *[]string, seen map[string]bool, id string) {
	if id == "" || seen[id] {
		return
	}
	seen[id] = true
	*dst = append(*dst, id)
}

// listMailboxFolders fetches every custom folder for a mailbox via the
// folders.list API. System folders are NOT included; callers that need them
// should fall back to local resolution via resolveFolderSystemAliasOrID.
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

// listMailboxLabels is the label counterpart of listMailboxFolders.
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

// resolveByID looks up input as an ID in items, returning input itself when
// found. kind ("folder" / "label") and mailboxID are used to construct the
// not-found hint. Generic over T so the same logic serves both folder and
// label tables.
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

// resolveByName looks up input as a name in items and returns the matching
// ID. Errors out on duplicates so callers get a clear "ambiguous name"
// signal rather than silently picking one match.
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

// resolveNameValueByID is the inverse of resolveByID: it looks up an ID
// and returns the matching name, used by the *QueryName resolvers.
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

// resolveNameValueByNameAllowDuplicates is like resolveByName but tolerates
// duplicate names — returning the first match. Used in query-style contexts
// where ambiguity is acceptable because the API itself disambiguates server-
// side.
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

// resolveLookupHint returns the CLI command a user should run to list
// valid IDs / names for the given lookup kind ("folder" / "label") and
// mailbox. Used in not-found error messages so callers see an immediate
// recovery path.
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

// messageGetFormat maps an html flag to the server-side messages.get format
// value: "full" when HTML body is wanted, "plain_text_full" otherwise (the
// server then omits body_html, saving bandwidth).
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

// warningEntry is a single structured warning emitted alongside primary
// output (e.g. when an attachment fails to download but the message itself
// is still returned). Serialized via the shared "warnings" output channel.
type warningEntry struct {
	Code         string `json:"code"`
	Level        string `json:"level"`
	MessageID    string `json:"message_id"`
	AttachmentID string `json:"attachment_id"`
	Retryable    bool   `json:"retryable"`
	Detail       string `json:"detail"`
}

// mailAddressOutput is the JSON-serialized address form used in public
// output (name + email). Distinct from mailAddressPair which is the
// internal value type used during body composition.
type mailAddressOutput struct {
	Email string `json:"email"`
	Name  string `json:"name"`
}

// mailAddressPair is a name+email pair used for display in HTML and plaintext quote blocks.
type mailAddressPair struct {
	Email string
	Name  string
}

// toAddressPairList converts JSON-output addresses (mailAddressOutput) to
// the internal mailAddressPair type used during body composition,
// dropping entries without an email address.
func toAddressPairList(raw []mailAddressOutput) []mailAddressPair {
	out := make([]mailAddressPair, 0, len(raw))
	for _, addr := range raw {
		if addr.Email != "" {
			out = append(out, mailAddressPair{Email: addr.Email, Name: addr.Name})
		}
	}
	return out
}

// mailAttachmentOutput is the JSON form of a regular (non-inline)
// attachment: ID, filename, content type, attachment type code, and the
// time-limited download URL when requested.
type mailAttachmentOutput struct {
	ID             string `json:"id"`
	Filename       string `json:"filename"`
	ContentType    string `json:"content_type,omitempty"`
	AttachmentType int    `json:"attachment_type"`
	DownloadURL    string `json:"download_url,omitempty"`
}

// mailImageOutput is the JSON form of a CID-referenced inline image in the
// HTML body. CID is required; DownloadURL is optional.
type mailImageOutput struct {
	ID          string `json:"id"`
	Filename    string `json:"filename"`
	ContentType string `json:"content_type,omitempty"`
	CID         string `json:"cid"`
	DownloadURL string `json:"download_url,omitempty"`
}

// mailPublicAttachmentOutput is the unified attachment shape exposed on the
// public "attachments" field of message output — merges inline and regular
// attachments with an IsInline flag and optional CID.
type mailPublicAttachmentOutput struct {
	ID             string `json:"id"`
	Filename       string `json:"filename"`
	ContentType    string `json:"content_type,omitempty"`
	AttachmentType int    `json:"attachment_type,omitempty"`
	IsInline       bool   `json:"is_inline"`
	CID            string `json:"cid,omitempty"`
}

// mailSecurityLevelOutput is the JSON form of the message's risk banner
// classification (external / phishing / similar). Present only when the
// backend flags the message; omitted on trusted messages.
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
	CalendarEvent        *calendarEventOutput     `json:"calendar_event,omitempty"`
	Attachments          []mailAttachmentOutput   `json:"attachments"`
	Images               []mailImageOutput        `json:"images"`
	Warnings             []warningEntry           `json:"warnings,omitempty"`
}

type calendarEventOutput struct {
	Method    string   `json:"method,omitempty"`
	UID       string   `json:"uid,omitempty"`
	Summary   string   `json:"summary,omitempty"`
	Start     string   `json:"start,omitempty"`
	End       string   `json:"end,omitempty"`
	Location  string   `json:"location,omitempty"`
	Organizer string   `json:"organizer,omitempty"`
	Attendees []string `json:"attendees,omitempty"`
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

// fetchAttachmentURLsWith resolves time-limited download URLs for each
// attachment ID via the attachments.download_url API. Returns a per-ID URL
// map plus a list of warnings for IDs the backend declined to resolve.
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

// rawMessageExcludedFields lists API response fields that must NOT be
// auto-passed through to the public output because they are replaced by a
// derived public shape (see buildPublicAttachments / derivedMessageFields).
var rawMessageExcludedFields = map[string]struct{}{
	"attachments": {},
}

// derivedMessageFields names the public output keys that are synthesized
// from the raw API response rather than copied through verbatim. Used by
// shouldExposeRawMessageField and by the output schema printed for agents.
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
		out["priority_type"] = normalized.PriorityType
		out["priority_type_text"] = normalized.PriorityTypeText
	}
	out["body_plain_text"] = normalized.BodyPlainText
	out["body_preview"] = normalized.BodyPreview
	if html && normalized.BodyHTML != "" {
		out["body_html"] = normalized.BodyHTML
	}
	if normalized.CalendarEvent != nil {
		out["calendar_event"] = normalized.CalendarEvent
	}
	out["attachments"] = buildPublicAttachments(msg)

	return out
}

// buildPublicAttachments returns the unified "attachments" list for
// message output, merging inline and regular attachments into a single
// shape with the IsInline flag set accordingly.
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

// derivedDraftID returns the draft identifier for a message that is
// itself a draft (message_state == draft). For non-draft messages returns
// "". messageID is used as fallback when the backend omits draft_id.
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
	// Priority: prefer label_ids (HIGH_PRIORITY/LOW_PRIORITY), fall back to priority_type field.
	priorityType := strVal(msg["priority_type"])
	out.PriorityType = priorityType
	if priorityType != "" {
		out.PriorityTypeText = priorityTypeText(priorityType)
	}
	for _, label := range out.LabelIDs {
		switch label {
		case "HIGH_PRIORITY":
			out.PriorityType = "1"
			out.PriorityTypeText = "high"
		case "LOW_PRIORITY":
			out.PriorityType = "5"
			out.PriorityTypeText = "low"
		}
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

	// Calendar event
	if bodyCalendar := strVal(msg["body_calendar"]); bodyCalendar != "" {
		if decoded := decodeBase64URL(bodyCalendar); decoded != "" {
			if parsed := ics.ParseEvent(decoded); parsed != nil {
				ce := &calendarEventOutput{
					Method:    parsed.Method,
					UID:       parsed.UID,
					Summary:   parsed.Summary,
					Location:  parsed.Location,
					Organizer: parsed.Organizer,
					Attendees: parsed.Attendees,
				}
				if !parsed.Start.IsZero() {
					ce.Start = parsed.Start.UTC().Format(time.RFC3339)
				}
				if !parsed.End.IsZero() {
					ce.End = parsed.End.UTC().Format(time.RFC3339)
				}
				out.CalendarEvent = ce
			}
		}
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

			if isInline && cid != "" {
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

// pickSafeMessageFields returns a shallow copy of msg containing only
// fields safe to expose in public output (per shouldExposeRawMessageField).
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

// shouldExposeRawMessageField reports whether key from a raw message
// response is safe to pass through to public output (i.e. not a body field
// handled separately and not in rawMessageExcludedFields).
func shouldExposeRawMessageField(key string) bool {
	if strings.HasPrefix(key, "body_") {
		return false
	}
	_, blocked := rawMessageExcludedFields[key]
	return !blocked
}

// attachmentTypeSmall is the API value for a regular attachment: embedded in
// the EML at send time (base64, counted against the 25 MB single-message limit).
// attachmentTypeLarge is the API value for a large attachment that is already
// embedded as a download link inside the message body. These must not be
// downloaded and re-attached during forward: the link in the body is sufficient
// and downloading could cause OOM for very large files.
// Both values align with the IDL i32 enum on Attachment.attachment_type.
const (
	attachmentTypeSmall = 1
	attachmentTypeLarge = 2
)

// forwardSourceAttachment is the compose-side view of an attachment on the
// original message being forwarded. AttachmentType 1 means a normal
// attachment that will be downloaded and re-attached; type 2 (large) is
// represented as an in-body link instead.
type forwardSourceAttachment struct {
	ID             string
	Filename       string
	ContentType    string
	AttachmentType int // 1=normal, 2=large (link in body, skip download)
	DownloadURL    string
}

// inlineSourcePart is the compose-side view of a CID-referenced inline
// resource on the original message that will be re-embedded in the
// reply / forward.
type inlineSourcePart struct {
	ID          string
	Filename    string
	ContentType string
	CID         string
	DownloadURL string
}

// composeSourceMessage bundles everything a reply / forward operation needs
// to know about the original message: the normalized originalMessage, the
// list of forward-able attachments, the list of inline parts to re-embed,
// and the set of attachment IDs whose download preflight failed.
type composeSourceMessage struct {
	Original            originalMessage
	ForwardAttachments  []forwardSourceAttachment
	InlineImages        []inlineSourcePart
	FailedAttachmentIDs map[string]bool
	OriginalCalendarICS []byte // raw ICS bytes from body_calendar (for forward passthrough)
}

// fetchComposeSourceMessage loads a message via the +message pipeline and converts it
// to compose-friendly data (quote metadata + forward attachments).
func fetchComposeSourceMessage(runtime *common.RuntimeContext, mailboxID, messageID string) (composeSourceMessage, error) {
	msg, err := fetchFullMessage(runtime, mailboxID, messageID, true)
	if err != nil {
		return composeSourceMessage{}, err
	}
	var originalCalICS []byte
	if bodyCalendar := strVal(msg["body_calendar"]); bodyCalendar != "" {
		if decoded := decodeBase64URL(bodyCalendar); decoded != "" {
			originalCalICS = []byte(decoded)
		}
	}
	attIDs := extractAttachmentIDs(msg)
	urlMap, warnings := fetchAttachmentURLs(runtime, mailboxID, messageID, attIDs)
	failedIDs := make(map[string]bool)
	for _, w := range warnings {
		if w.Code == "attachment_download_url_failed_id" && w.AttachmentID != "" {
			failedIDs[w.AttachmentID] = true
		}
	}
	out := buildMessageForCompose(msg, urlMap, true)
	orig := toOriginalMessageForCompose(out)
	return composeSourceMessage{
		Original:            orig,
		ForwardAttachments:  toForwardSourceAttachments(out),
		InlineImages:        toInlineSourceParts(out),
		FailedAttachmentIDs: failedIDs,
		OriginalCalendarICS: originalCalICS,
	}, nil
}

// validateForwardAttachmentURLs checks that all forwarded attachments (non-inline)
// have valid download URLs. Inline images are checked separately by validateInlineImageURLs.
func validateForwardAttachmentURLs(src composeSourceMessage) error {
	var missing []string
	for _, att := range src.ForwardAttachments {
		if att.AttachmentType == attachmentTypeLarge {
			continue
		}
		if src.FailedAttachmentIDs[att.ID] {
			continue
		}
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

// toOriginalMessageForCompose lifts the normalized message representation
// into the originalMessage value type used by +reply / +forward body
// builders.
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

// toForwardSourceAttachments extracts the forward-capable attachments from
// a normalized message (non-inline attachments, both regular and large).
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

// toInlineSourceParts extracts the CID-referenced inline resources from a
// normalized message for re-embedding in a reply / forward.
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

// strVal returns v as a string when it is one, otherwise "". Used to
// safely extract string fields from decoded JSON maps.
func strVal(v interface{}) string {
	s, _ := v.(string)
	return s
}

// intVal returns v as an int, parsing string forms and coercing JSON
// float64 when needed. Returns 0 when v is nil or non-numeric.
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

// decodeBase64URL returns the decoded bytes of a base64url-encoded string
// (either padded or raw). Returns "" on decode error.
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
// prevent terminal injection from untrusted email content. LF is preserved
// because legitimate multi-line content (body_text, body_html_summary) is
// printed through this helper; use sanitizeForSingleLine when the caller
// needs a single-line guarantee.
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

// sanitizeForSingleLine is sanitizeForTerminal plus LF removal, for callers
// whose output must stay on one logical line — stderr hints, embedded
// command-line arguments, etc. A malicious From header or subject containing
// "\ntip: ..." can no longer forge extra lines in the prompt and trick a
// reader into thinking the CLI emitted them.
func sanitizeForSingleLine(s string) string {
	return strings.ReplaceAll(sanitizeForTerminal(s), "\n", "")
}

// toAddressObject converts a raw address field (map form) from the API
// response into mailAddressOutput. Returns zero value when v isn't a map.
func toAddressObject(v interface{}) mailAddressOutput {
	if m, ok := v.(map[string]interface{}); ok {
		return mailAddressOutput{Email: strVal(m["mail_address"]), Name: strVal(m["name"])}
	}
	return mailAddressOutput{}
}

// toAddressList converts a raw address-list field from the API response
// (array of maps) into []mailAddressOutput.
func toAddressList(v interface{}) []mailAddressOutput {
	list, _ := v.([]interface{})
	out := make([]mailAddressOutput, 0, len(list))
	for _, item := range list {
		out = append(out, toAddressObject(item))
	}
	return out
}

// toAddressEmailList extracts just the email addresses from a list of
// mailAddressOutput, dropping entries with empty email.
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

// toStringList coerces a JSON array of strings / anything-stringifiable
// into []string. Returns nil when v is not an array.
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

// toSecurityLevel extracts the risk-banner info from a raw message's
// security_level field. Returns nil when absent / not flagged.
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

// boolVal returns v as a bool when it is one, otherwise false.
func boolVal(v interface{}) bool {
	b, _ := v.(bool)
	return b
}

// firstNonEmpty returns the first non-empty value in values, or "" when
// all values are empty.
func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

// resolveAttachmentContentType returns the MIME type of an attachment,
// falling back to the extension-based guess when the API response doesn't
// include one.
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

// messageStateText maps the numeric message_state code (1/2/3) to the
// human-readable label received / sent / draft. Unknown values become
// "unknown".
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

// priorityTypeText maps the server priority enum ("HIGH" / "LOW" /
// "NORMAL" / empty) to the CLI-facing label shown in message output.
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

// priorityFlag is the common flag definition for --priority, shared by all compose shortcuts.
var priorityFlag = common.Flag{
	Name: "priority",
	Desc: "Email priority: high, normal, low. If omitted, no priority header is set.",
}

// parsePriority parses the --priority flag value and returns the X-Cli-Priority
// header value. Returns "" if the priority should not be set (empty or "normal").
func parsePriority(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "":
		return "", nil
	case "high":
		return "1", nil
	case "normal":
		return "", nil
	case "low":
		return "5", nil
	default:
		return "", fmt.Errorf("invalid --priority value %q: expected high, normal, or low", value)
	}
}

// validatePriorityFlag validates the --priority flag value in Validate, so invalid
// values are caught before Execute (and before dry-run prints an API plan).
func validatePriorityFlag(runtime *common.RuntimeContext) error {
	v := runtime.Str("priority")
	if v == "" {
		return nil
	}
	_, err := parsePriority(v)
	return err
}

// applyPriority sets the X-Cli-Priority header on the EML builder if priority is non-empty.
func applyPriority(bld emlbuilder.Builder, priority string) emlbuilder.Builder {
	if priority == "" {
		return bld
	}
	return bld.Header("X-Cli-Priority", priority)
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

// normalizeMessageID strips angle brackets and whitespace from an RFC 5322
// Message-ID so it can be used as a bare value in In-Reply-To / References
// headers (emlbuilder re-wraps in angle brackets itself).
func normalizeMessageID(id string) string {
	trimmed := strings.TrimSpace(id)
	trimmed = strings.TrimPrefix(trimmed, "<")
	trimmed = strings.TrimSuffix(trimmed, ">")
	return strings.TrimSpace(trimmed)
}

// buildDraftSendOutput formats a successful drafts.send response into the
// public output map (message_id / thread_id plus an optional recall tip
// when the backend reports the message is within the recall window).
func buildDraftSendOutput(resData map[string]interface{}, mailboxID string) map[string]interface{} {
	out := map[string]interface{}{
		"message_id": resData["message_id"],
		"thread_id":  resData["thread_id"],
	}
	if recallStatus, ok := resData["recall_status"].(string); ok && recallStatus == "available" {
		messageID, _ := resData["message_id"].(string)
		out["recall_available"] = true
		out["recall_tip"] = fmt.Sprintf(
			`This message can be recalled within 24 hours. To recall: lark-cli mail user_mailbox.sent_messages recall --params '{"user_mailbox_id":"%s","message_id":"%s"}'`,
			mailboxID, messageID)
	}
	if automationDisable, ok := resData["automation_send_disable"]; ok {
		if automation, ok := automationDisable.(map[string]interface{}); ok {
			if reason, ok := automation["reason"].(string); ok && strings.TrimSpace(reason) != "" {
				out["automation_send_disable_reason"] = strings.TrimSpace(reason)
			}
			if reference, ok := automation["reference"].(string); ok && strings.TrimSpace(reference) != "" {
				out["automation_send_disable_reference"] = strings.TrimSpace(reference)
			}
		}
	}
	return out
}

// buildDraftSavedOutput formats a successful drafts.create / drafts.update
// response into the public output map (draft_id + optional preview URL).
func buildDraftSavedOutput(draftResult draftpkg.DraftResult, mailboxID string) map[string]interface{} {
	out := map[string]interface{}{
		"draft_id": draftResult.DraftID,
		"tip":      fmt.Sprintf(`draft saved. To send: lark-cli mail user_mailbox.drafts send --params '{"user_mailbox_id":"%s","draft_id":"%s"}'`, mailboxID, draftResult.DraftID),
	}
	if draftResult.Reference != "" {
		out["reference"] = draftResult.Reference
	}
	return out
}

// normalizeInlineCID strips angle brackets from a Content-ID so it can be
// referenced in <img src="cid:..."> and emlbuilder.AddFileInline
// consistently (both expect the bare CID).
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

// addInlineImagesToBuilder downloads each inline image referenced in images
// and attaches it to bld with the caller-supplied CID preserved. Returns the
// extended builder, the list of CIDs that were actually attached (empty CIDs
// are skipped), and the total bytes of downloaded inline content (for
// attachment-size budgeting upstream). Errors propagate immediately; callers
// should not reuse the builder on error since partial state may have been
// committed.
func addInlineImagesToBuilder(runtime *common.RuntimeContext, bld emlbuilder.Builder, images []inlineSourcePart) (emlbuilder.Builder, []string, int64, error) {
	var cids []string
	var totalBytes int64
	for _, img := range images {
		content, err := downloadAttachmentContent(runtime, img.DownloadURL)
		if err != nil {
			return bld, nil, 0, fmt.Errorf("failed to download inline resource %s: %w", img.Filename, err)
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
		totalBytes += int64(len(content))
	}
	return bld, cids, totalBytes, nil
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

// validateEventSendTimeExclusion checks that --send-time and --event-* are not
// used together. This is enforced here (in Validate, before Execute) because the
// Shortcut framework does not expose a cobra-level hook for MarkFlagsMutuallyExclusive.
func validateEventSendTimeExclusion(runtime *common.RuntimeContext) error {
	if runtime.Str("send-time") == "" {
		return nil
	}
	for _, f := range []string{"event-summary", "event-start", "event-end", "event-location"} {
		if runtime.Str(f) != "" {
			return common.FlagErrorf("--send-time and --event-* are mutually exclusive: a calendar invitation must be sent immediately so recipients can respond before the event")
		}
	}
	return nil
}

// validateSendTime checks that --send-time, if provided, requires --confirm-send,
// is a valid Unix timestamp in seconds, and is at least 5 minutes in the future.
func validateSendTime(runtime *common.RuntimeContext) error {
	sendTime := runtime.Str("send-time")
	if sendTime == "" {
		return nil
	}
	if !runtime.Bool("confirm-send") {
		return fmt.Errorf("--send-time requires --confirm-send to be set")
	}
	ts, err := strconv.ParseInt(sendTime, 10, 64)
	if err != nil {
		return fmt.Errorf("--send-time must be a valid Unix timestamp in seconds, got %q", sendTime)
	}
	minTime := time.Now().Unix() + 5*60
	if ts < minTime {
		return fmt.Errorf("--send-time must be at least 5 minutes in the future (minimum: %d, got: %d)", minTime, ts)
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

// validateComposeHasAtLeastOneRecipient ensures a compose-style invocation
// has at least one recipient field populated. Returns ErrValidation when
// all three (to/cc/bcc) are empty or whitespace-only.
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

// validateComposeInlineAndAttachments validates the --attach / --inline
// flag pair before sending: it rejects --inline with --plain-text or with
// a non-HTML body, and checks that every --attach path passes filename /
// extension / size rules via the shared filecheck rules.
func validateComposeInlineAndAttachments(fio fileio.FileIO, attachFlag, inlineFlag string, plainText bool, body string) error {
	if strings.TrimSpace(inlineFlag) != "" {
		if plainText {
			return fmt.Errorf("--inline is not supported with --plain-text (inline images require HTML body)")
		}
		if body != "" && !bodyIsHTML(body) {
			return fmt.Errorf("--inline requires an HTML body (the provided body appears to be plain text; add HTML tags or remove --inline)")
		}
	}
	inlineSpecs, err := parseInlineSpecs(inlineFlag)
	if err != nil {
		return err
	}
	// Preflight: verify explicit file paths exist and pass blocked-extension
	// checks so that --dry-run surfaces local errors before Execute.
	allPaths := append(splitByComma(attachFlag), inlineSpecFilePaths(inlineSpecs)...)
	if _, err := statAttachmentFiles(fio, allPaths); err != nil {
		return err
	}
	return nil
}

// buildCalendarBodyFromArgs builds ICS from explicit string arguments (for draft-edit).
// Callers are expected to have pre-validated startStr/endStr via parseEventTimeRange;
// parse errors are silently ignored here and produce a zero-time DTSTART/DTEND.
func buildCalendarBodyFromArgs(summary, startStr, endStr, location, senderEmail, toAddrs, ccAddrs string) []byte {
	if summary == "" {
		return nil
	}
	start, _ := parseISO8601(startStr)
	end, _ := parseISO8601(endStr)

	var attendees []ics.Address
	for _, addr := range parseNetAddrs(toAddrs) {
		if addr.Address != "" {
			attendees = append(attendees, ics.Address{Name: addr.Name, Email: addr.Address})
		}
	}
	for _, addr := range parseNetAddrs(ccAddrs) {
		if addr.Address != "" {
			attendees = append(attendees, ics.Address{Name: addr.Name, Email: addr.Address})
		}
	}

	return ics.Build(ics.Event{
		Summary:   summary,
		Location:  location,
		Start:     start,
		End:       end,
		Organizer: ics.Address{Email: senderEmail},
		Attendees: attendees,
	})
}

// joinAddresses joins draft Address list into comma-separated string.
func joinAddresses(addrs []draftpkg.Address) string {
	if len(addrs) == 0 {
		return ""
	}
	parts := make([]string, len(addrs))
	for i, a := range addrs {
		parts[i] = a.Address
	}
	return strings.Join(parts, ",")
}

// Calendar event flag definitions, shared by all compose shortcuts.
// Declared as individual vars (like priorityFlag and signatureFlag) so
// callers can list them explicitly in their Flags slice without relying
// on slice-index access.
var (
	eventSummaryFlag  = common.Flag{Name: "event-summary", Desc: "Calendar event title. Setting this enables calendar invitation mode."}
	eventStartFlag    = common.Flag{Name: "event-start", Desc: "Event start time (ISO 8601, e.g. 2026-04-20T14:00+08:00). Required when --event-summary is set."}
	eventEndFlag      = common.Flag{Name: "event-end", Desc: "Event end time (ISO 8601). Required when --event-summary is set."}
	eventLocationFlag = common.Flag{Name: "event-location", Desc: "Event location (optional)."}
)

// validateEventFlags checks that --event-summary, --event-start, --event-end are either all set or all empty.
func validateEventFlags(runtime *common.RuntimeContext) error {
	summary := runtime.Str("event-summary")
	start := runtime.Str("event-start")
	end := runtime.Str("event-end")
	location := runtime.Str("event-location")

	hasAny := summary != "" || start != "" || end != "" || location != ""
	hasAll := summary != "" && start != "" && end != ""

	if hasAny && !hasAll {
		return fmt.Errorf("--event-summary, --event-start, and --event-end must all be provided together")
	}
	if summary == "" {
		return nil
	}
	if _, _, err := parseEventTimeRange(start, end); err != nil {
		return prefixEventRangeError("--event-", err)
	}
	return nil
}

// parseEventTimeRange parses start/end ISO 8601 strings and verifies that
// end is strictly after start. Shared by validateEventFlags (compose path)
// and buildDraftEditPatch (draft-edit path) so the rules stay in one place.
func parseEventTimeRange(start, end string) (time.Time, time.Time, error) {
	startT, err := parseISO8601(start)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("start: invalid ISO 8601 time %q", start)
	}
	endT, err := parseISO8601(end)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("end: invalid ISO 8601 time %q", end)
	}
	if !endT.After(startT) {
		return time.Time{}, time.Time{}, fmt.Errorf("end time must be after start time")
	}
	return startT, endT, nil
}

// prefixEventRangeError rewrites parseEventTimeRange's "start:" / "end:"
// error with the caller's flag-name prefix so users see the exact flag
// that caused the failure.
func prefixEventRangeError(flagPrefix string, err error) error {
	msg := err.Error()
	switch {
	case strings.HasPrefix(msg, "start: "):
		return fmt.Errorf("%sstart: %s", flagPrefix, strings.TrimPrefix(msg, "start: "))
	case strings.HasPrefix(msg, "end: "):
		return fmt.Errorf("%send: %s", flagPrefix, strings.TrimPrefix(msg, "end: "))
	default:
		return err
	}
}

// parseISO8601 parses common ISO 8601 time formats.
func parseISO8601(s string) (time.Time, error) {
	formats := []string{
		time.RFC3339,
		"2006-01-02T15:04:05Z07:00",
		"2006-01-02T15:04Z07:00",
		"2006-01-02T15:04:05",
		"2006-01-02T15:04",
		"2006-01-02",
	}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("cannot parse %q as ISO 8601", s)
}

// buildCalendarBody generates an ICS VCALENDAR from compose flags and returns the bytes.
// Returns nil if --event-summary is not set.
func buildCalendarBody(runtime *common.RuntimeContext, senderEmail string, toAddrs, ccAddrs string) []byte {
	return buildCalendarBodyFromArgs(
		runtime.Str("event-summary"),
		runtime.Str("event-start"),
		runtime.Str("event-end"),
		runtime.Str("event-location"),
		senderEmail, toAddrs, ccAddrs,
	)
}
