// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"context"
	"fmt"
	"strings"

	"github.com/larksuite/cli/shortcuts/common"
	draftpkg "github.com/larksuite/cli/shortcuts/mail/draft"
	"github.com/larksuite/cli/shortcuts/mail/emlbuilder"
)

var MailReplyAll = common.Shortcut{
	Service:     "mail",
	Command:     "+reply-all",
	Description: "Reply to all recipients and save as draft (default). Use --confirm-send to send immediately after user confirmation. Includes all original To and CC automatically.",
	Risk:        "write",
	Scopes:      []string{"mail:user_mailbox.message:modify", "mail:user_mailbox.message:readonly", "mail:user_mailbox:readonly", "mail:user_mailbox.message.address:read", "mail:user_mailbox.message.subject:read", "mail:user_mailbox.message.body:read"},
	AuthTypes:   []string{"user"},
	Flags: []common.Flag{
		{Name: "message-id", Desc: "Required. Message ID to reply to all recipients", Required: true},
		{Name: "body", Desc: "Required. Reply body. Prefer HTML for rich formatting; plain text is also supported. Body type is auto-detected from the reply body and the original message. Use --plain-text to force plain-text mode.", Required: true},
		{Name: "from", Desc: "Sender email address for the From header. When using an alias (send_as) address, set this to the alias and use --mailbox for the owning mailbox. Defaults to the mailbox's primary address."},
		{Name: "mailbox", Desc: "Mailbox email address that owns the draft (default: falls back to --from, then me). Use this when the sender (--from) differs from the mailbox, e.g. sending via an alias or send_as address."},
		{Name: "to", Desc: "Additional To address(es), comma-separated (appended to original recipients)"},
		{Name: "cc", Desc: "Additional CC email address(es), comma-separated"},
		{Name: "bcc", Desc: "BCC email address(es), comma-separated"},
		{Name: "remove", Desc: "Address(es) to exclude from the outgoing reply, comma-separated"},
		{Name: "plain-text", Type: "bool", Desc: "Force plain-text mode, ignoring all HTML auto-detection. Cannot be used with --inline."},
		{Name: "attach", Desc: "Attachment file path(s), comma-separated (relative path only)"},
		{Name: "inline", Desc: "Inline images as a JSON array. Each entry: {\"cid\":\"<unique-id>\",\"file_path\":\"<relative-path>\"}. All file_path values must be relative paths. Cannot be used with --plain-text. CID images are embedded via <img src=\"cid:...\"> in the HTML body. CID is a unique identifier, e.g. a random hex string like \"a1b2c3d4e5f6a7b8c9d0\"."},
		{Name: "confirm-send", Type: "bool", Desc: "Send the reply immediately instead of saving as draft. Only use after the user has explicitly confirmed recipients and content."},
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		messageId := runtime.Str("message-id")
		confirmSend := runtime.Bool("confirm-send")
		mailboxID := resolveComposeMailboxID(runtime)
		desc := "Reply-all: fetch original message (with recipients) → resolve sender address → save as draft"
		if confirmSend {
			desc = "Reply-all (--confirm-send): fetch original message (with recipients) → resolve sender address → create draft → send draft"
		}
		api := common.NewDryRunAPI().
			Desc(desc).
			GET(mailboxPath(mailboxID, "messages", messageId)).
			GET(mailboxPath(mailboxID, "profile")).
			POST(mailboxPath(mailboxID, "drafts")).
			Body(map[string]interface{}{"raw": "<base64url-EML>"})
		if confirmSend {
			api = api.POST(mailboxPath(mailboxID, "drafts", "<draft_id>", "send"))
		}
		return api
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if err := validateConfirmSendScope(runtime); err != nil {
			return err
		}
		return validateComposeInlineAndAttachments(runtime.FileIO(), runtime.Str("attach"), runtime.Str("inline"), runtime.Bool("plain-text"), "")
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		messageId := runtime.Str("message-id")
		body := runtime.Str("body")
		toFlag := runtime.Str("to")
		ccFlag := runtime.Str("cc")
		bccFlag := runtime.Str("bcc")
		removeFlag := runtime.Str("remove")
		plainText := runtime.Bool("plain-text")
		attachFlag := runtime.Str("attach")
		inlineFlag := runtime.Str("inline")
		confirmSend := runtime.Bool("confirm-send")

		inlineSpecs, err := parseInlineSpecs(inlineFlag)
		if err != nil {
			return err
		}

		mailboxID := resolveComposeMailboxID(runtime)
		sourceMsg, err := fetchComposeSourceMessage(runtime, mailboxID, messageId)
		if err != nil {
			return fmt.Errorf("failed to fetch original message: %w", err)
		}
		orig := sourceMsg.Original

		senderEmail := resolveComposeSenderEmail(runtime)
		if senderEmail == "" {
			senderEmail = orig.headTo
		}

		var removeList []string
		for _, r := range strings.Split(removeFlag, ",") {
			if s := strings.TrimSpace(r); s != "" {
				removeList = append(removeList, s)
			}
		}
		selfEmails := fetchSelfEmailSet(runtime, mailboxID)
		excluded := buildExcludeSet(selfEmails, removeList)
		replyToAddr := orig.replyTo
		if replyToAddr == "" {
			replyToAddr = orig.headFrom
		}
		isSelfSent := selfEmails[strings.ToLower(orig.headFrom)] || (senderEmail != "" && strings.EqualFold(orig.headFrom, senderEmail))
		toList, ccList := buildReplyAllRecipients(replyToAddr, orig.toAddresses, orig.ccAddresses, senderEmail, excluded, isSelfSent)

		toList = mergeAddrLists(toList, toFlag)
		ccList = mergeAddrLists(ccList, ccFlag)

		if err := validateRecipientCount(toList, ccList, bccFlag); err != nil {
			return err
		}

		useHTML := !plainText && (bodyIsHTML(body) || bodyIsHTML(orig.bodyRaw))
		if strings.TrimSpace(inlineFlag) != "" && !useHTML {
			return fmt.Errorf("--inline requires HTML mode, but neither the new body nor the original message contains HTML")
		}
		var bodyStr string
		if useHTML {
			bodyStr = buildBodyDiv(body, bodyIsHTML(body))
		} else {
			bodyStr = body
		}
		quoted := quoteForReply(&orig, useHTML)
		bld := emlbuilder.New().WithFileIO(runtime.FileIO()).
			Subject(buildReplySubject(orig.subject)).
			ToAddrs(parseNetAddrs(toList))
		if senderEmail != "" {
			bld = bld.From("", senderEmail)
		}
		if ccList != "" {
			bld = bld.CCAddrs(parseNetAddrs(ccList))
		}
		if bccFlag != "" {
			bld = bld.BCCAddrs(parseNetAddrs(bccFlag))
		}
		if inReplyTo := normalizeMessageID(orig.smtpMessageId); inReplyTo != "" {
			bld = bld.InReplyTo(inReplyTo)
		}
		if messageId != "" {
			bld = bld.LMSReplyToMessageID(messageId)
		}
		var autoResolvedPaths []string
		if useHTML {
			if err := validateInlineImageURLs(sourceMsg); err != nil {
				return fmt.Errorf("HTML reply-all blocked: %w", err)
			}
			var srcCIDs []string
			bld, srcCIDs, err = addInlineImagesToBuilder(runtime, bld, sourceMsg.InlineImages)
			if err != nil {
				return err
			}
			resolved, refs, resolveErr := draftpkg.ResolveLocalImagePaths(bodyStr)
			if resolveErr != nil {
				return resolveErr
			}
			fullHTML := resolved + quoted
			bld = bld.HTMLBody([]byte(fullHTML))
			var userCIDs []string
			for _, ref := range refs {
				bld = bld.AddFileInline(ref.FilePath, ref.CID)
				autoResolvedPaths = append(autoResolvedPaths, ref.FilePath)
				userCIDs = append(userCIDs, ref.CID)
			}
			for _, spec := range inlineSpecs {
				bld = bld.AddFileInline(spec.FilePath, spec.CID)
				userCIDs = append(userCIDs, spec.CID)
			}
			if err := validateInlineCIDs(resolved, userCIDs, srcCIDs); err != nil {
				return err
			}
		} else {
			bld = bld.TextBody([]byte(bodyStr + quoted))
		}
		allFilePaths := append(append(splitByComma(attachFlag), inlineSpecFilePaths(inlineSpecs)...), autoResolvedPaths...)
		if err := checkAttachmentSizeLimit(runtime.FileIO(), allFilePaths, 0); err != nil {
			return err
		}
		for _, path := range splitByComma(attachFlag) {
			bld = bld.AddFileAttachment(path)
		}
		rawEML, err := bld.BuildBase64URL()
		if err != nil {
			return fmt.Errorf("failed to build EML: %w", err)
		}

		draftID, err := draftpkg.CreateWithRaw(runtime, mailboxID, rawEML)
		if err != nil {
			return fmt.Errorf("failed to create draft: %w", err)
		}
		if !confirmSend {
			runtime.Out(map[string]interface{}{
				"draft_id": draftID,
				"tip":      fmt.Sprintf(`draft saved. To send: lark-cli mail user_mailbox.drafts send --params '{"user_mailbox_id":"%s","draft_id":"%s"}'`, mailboxID, draftID),
			}, nil)
			hintSendDraft(runtime, mailboxID, draftID)
			return nil
		}
		resData, err := draftpkg.Send(runtime, mailboxID, draftID)
		if err != nil {
			return fmt.Errorf("failed to send reply-all (draft %s created but not sent): %w", draftID, err)
		}
		runtime.Out(map[string]interface{}{
			"message_id": resData["message_id"],
			"thread_id":  resData["thread_id"],
		}, nil)
		hintMarkAsRead(runtime, mailboxID, messageId)
		return nil
	},
}

// buildExcludeSet returns a lowercase set of addresses to exclude from reply-all.
// selfEmails contains all known addresses for the current user (enterprise + personal).
func buildExcludeSet(selfEmails map[string]bool, remove []string) map[string]bool {
	set := make(map[string]bool)
	for addr := range selfEmails {
		set[addr] = true
	}
	for _, r := range remove {
		if s := strings.ToLower(strings.TrimSpace(r)); s != "" {
			set[s] = true
		}
	}
	return set
}

// buildReplyAllRecipients constructs the To and Cc lists for a reply-all.
//
// Normal case: the original sender (or Reply-To) goes to To; all other original
// To/Cc recipients go to Cc.
//
// Self-sent case (isSelfSent=true): the original To recipients stay in To and
// the original Cc recipients stay in Cc, preserving the distinction from the
// original message. If a Reply-To header was set, its address is also added to To.
// This aligns with the Lark client (rust-sdk) behavior.
func buildReplyAllRecipients(origFrom string, origTo, origCC []string, senderEmail string, excluded map[string]bool, isSelfSent bool) (to, cc string) {
	// Copy excluded to avoid mutating the caller's map.
	excl := make(map[string]bool, len(excluded)+1)
	for k, v := range excluded {
		excl[k] = v
	}
	excluded = excl
	// Ensure senderEmail (which may be an alias or shared mailbox) is also excluded.
	if senderEmail != "" {
		excluded[strings.ToLower(senderEmail)] = true
	}

	if isSelfSent {
		// Self-sent: preserve original To/Cc distinction.
		seen := make(map[string]bool)
		var toList []string
		for _, addr := range origTo {
			lower := strings.ToLower(addr)
			if excluded[lower] || seen[lower] {
				continue
			}
			seen[lower] = true
			toList = append(toList, addr)
		}
		// If Reply-To is set (origFrom differs from self), include it in To.
		if lf := strings.ToLower(origFrom); !excluded[lf] && !seen[lf] {
			toList = append(toList, origFrom)
			seen[lf] = true
		}
		var ccList []string
		for _, addr := range origCC {
			lower := strings.ToLower(addr)
			if excluded[lower] || seen[lower] {
				continue
			}
			seen[lower] = true
			ccList = append(ccList, addr)
		}
		return strings.Join(toList, ", "), strings.Join(ccList, ", ")
	}

	// Normal case: original sender → To; origTo+origCC → Cc.
	if !excluded[strings.ToLower(origFrom)] {
		to = origFrom
	}

	seen := make(map[string]bool)
	seen[strings.ToLower(origFrom)] = true
	var ccList []string
	for _, addr := range append(origTo, origCC...) {
		lower := strings.ToLower(addr)
		if excluded[lower] || seen[lower] {
			continue
		}
		seen[lower] = true
		ccList = append(ccList, addr)
	}
	cc = strings.Join(ccList, ", ")
	return to, cc
}
