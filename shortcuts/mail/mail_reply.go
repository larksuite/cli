// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"context"
	"fmt"
	"strings"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
	draftpkg "github.com/larksuite/cli/shortcuts/mail/draft"
	"github.com/larksuite/cli/shortcuts/mail/emlbuilder"
)

// MailReply is the `+reply` shortcut: reply to the sender of a message,
// saving a draft by default (or sending immediately with --confirm-send).
// Automatically sets Re: subject, In-Reply-To, and References headers.
var MailReply = common.Shortcut{
	Service:     "mail",
	Command:     "+reply",
	Description: "Reply to a message and save as draft (default). Use --confirm-send to send immediately after user confirmation. Sets Re: subject, In-Reply-To, and References headers automatically.",
	Risk:        "write",
	Scopes:      []string{"mail:user_mailbox.message:modify", "mail:user_mailbox.message:readonly", "mail:user_mailbox:readonly", "mail:user_mailbox.message.address:read", "mail:user_mailbox.message.subject:read", "mail:user_mailbox.message.body:read"},
	AuthTypes:   []string{"user"},
	Flags: []common.Flag{
		{Name: "message-id", Desc: "Required. Message ID to reply to", Required: true},
		{Name: "body", Desc: "Reply body. Prefer HTML for rich formatting; plain text is also supported. Body type is auto-detected from the reply body and the original message. Use --plain-text to force plain-text mode. Required unless --template-id supplies a non-empty body."},
		{Name: "from", Desc: "Sender email address for the From header. When using an alias (send_as) address, set this to the alias and use --mailbox for the owning mailbox. Defaults to the mailbox's primary address."},
		{Name: "mailbox", Desc: "Mailbox email address that owns the draft (default: falls back to --from, then me). Use this when the sender (--from) differs from the mailbox, e.g. sending via an alias or send_as address."},
		{Name: "to", Desc: "Additional To address(es), comma-separated (appended to original sender's address)"},
		{Name: "cc", Desc: "Additional CC email address(es), comma-separated"},
		{Name: "bcc", Desc: "BCC email address(es), comma-separated"},
		{Name: "plain-text", Type: "bool", Desc: "Force plain-text mode, ignoring all HTML auto-detection. Cannot be used with --inline."},
		{Name: "attach", Desc: "Attachment file path(s), comma-separated (relative path only)"},
		{Name: "inline", Desc: "Inline images as a JSON array. Each entry: {\"cid\":\"<unique-id>\",\"file_path\":\"<relative-path>\"}. All file_path values must be relative paths. Cannot be used with --plain-text. CID images are embedded via <img src=\"cid:...\"> in the HTML body. CID is a unique identifier, e.g. a random hex string like \"a1b2c3d4e5f6a7b8c9d0\"."},
		{Name: "confirm-send", Type: "bool", Desc: "Send the reply immediately instead of saving as draft. Only use after the user has explicitly confirmed recipients and content."},
		{Name: "send-time", Desc: "Scheduled send time as a Unix timestamp in seconds. Must be at least 5 minutes in the future. Use with --confirm-send to schedule the email."},
		{Name: "request-receipt", Type: "bool", Desc: "Request a read receipt (Message Disposition Notification, RFC 3798) addressed to the sender. Recipient mail clients may prompt the user, send automatically, or silently ignore — delivery of a receipt is not guaranteed."},
		{Name: "subject", Desc: "Optional. Override the auto-generated Re: subject. When set, the shortcut uses this value verbatim instead of prefixing the original subject."},
		{Name: "template-id", Desc: "Optional. Apply a saved template by ID (decimal integer string) before composing. The template's body/to/cc/bcc/attachments are appended to the reply-derived values (no de-duplication; see warning in Execute output)."},
		signatureFlag,
		priorityFlag,
		eventSummaryFlag, eventStartFlag, eventEndFlag, eventLocationFlag},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		messageId := runtime.Str("message-id")
		confirmSend := runtime.Bool("confirm-send")
		mailboxID := resolveComposeMailboxID(runtime)
		desc := "Reply: fetch original message → resolve sender address → save as draft"
		if confirmSend {
			desc = "Reply (--confirm-send): fetch original message → resolve sender address → create draft → send draft"
		}
		api := common.NewDryRunAPI().Desc(desc)
		if tid := runtime.Str("template-id"); tid != "" {
			api = api.GET(templateMailboxPath(mailboxID, tid)).
				Desc("Fetch template to merge with reply-derived recipients / body.")
		}
		api = api.GET(mailboxPath(mailboxID, "messages", messageId)).
			GET(mailboxPath(mailboxID, "profile")).
			POST(mailboxPath(mailboxID, "drafts")).
			Body(map[string]interface{}{"raw": "<base64url-EML>"})
		if confirmSend {
			api = api.POST(mailboxPath(mailboxID, "drafts", "<draft_id>", "send"))
		}
		return api
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if err := validateTemplateID(runtime.Str("template-id")); err != nil {
			return err
		}
		hasTemplate := runtime.Str("template-id") != ""
		if !hasTemplate && strings.TrimSpace(runtime.Str("body")) == "" {
			return output.ErrValidation("--body is required; pass the reply body (or use --template-id)")
		}
		if err := validateConfirmSendScope(runtime); err != nil {
			return err
		}
		if err := validateEventSendTimeExclusion(runtime); err != nil {
			return err
		}
		if err := validateSendTime(runtime); err != nil {
			return err
		}
		if err := validateSignatureWithPlainText(runtime.Bool("plain-text"), runtime.Str("signature-id")); err != nil {
			return err
		}
		if err := validateEventFlags(runtime); err != nil {
			return err
		}
		if err := validateComposeInlineAndAttachments(runtime.FileIO(), runtime.Str("attach"), runtime.Str("inline"), runtime.Bool("plain-text"), ""); err != nil {
			return err
		}
		return validatePriorityFlag(runtime)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		messageId := runtime.Str("message-id")
		body := runtime.Str("body")
		toFlag := runtime.Str("to")
		ccFlag := runtime.Str("cc")
		bccFlag := runtime.Str("bcc")
		plainText := runtime.Bool("plain-text")
		attachFlag := runtime.Str("attach")
		inlineFlag := runtime.Str("inline")
		confirmSend := runtime.Bool("confirm-send")
		sendTime := runtime.Str("send-time")

		priority, err := parsePriority(runtime.Str("priority"))
		if err != nil {
			return err
		}

		inlineSpecs, err := parseInlineSpecs(inlineFlag)
		if err != nil {
			return err
		}

		signatureID := runtime.Str("signature-id")
		mailboxID := resolveComposeMailboxID(runtime)
		sigResult, sigErr := resolveSignature(ctx, runtime, mailboxID, signatureID, runtime.Str("from"))
		if sigErr != nil {
			return sigErr
		}
		sourceMsg, err := fetchComposeSourceMessage(runtime, mailboxID, messageId)
		if err != nil {
			return fmt.Errorf("failed to fetch original message: %w", err)
		}
		orig := sourceMsg.Original
		stripLargeAttachmentCard(&orig)

		resolvedSender := resolveComposeSenderEmail(runtime)
		// Check --request-receipt BEFORE the orig.headTo fallback below:
		// the receipt's Disposition-Notification-To must point to an address
		// the caller explicitly controls, not to a fallback picked from the
		// original mail's headers (which may belong to someone else when the
		// mailbox is only on CC or in a shared-mailbox scenario).
		if err := requireSenderForRequestReceipt(runtime, resolvedSender); err != nil {
			return err
		}
		senderEmail := resolvedSender
		if senderEmail == "" {
			senderEmail = orig.headTo
		}

		replyTo := orig.replyTo
		if replyTo == "" {
			replyTo = orig.headFrom
		}
		replyTo = mergeAddrLists(replyTo, toFlag)

		// --template-id merge (§5.5 Q1-Q5).
		var templateLargeAttachmentIDs []string
		var templateInlineAttachments []templateInlineRef
		var templateSmallAttachments []templateAttachmentRef
		templateID := runtime.Str("template-id")
		if tid := templateID; tid != "" {
			tpl, tErr := fetchTemplate(runtime, mailboxID, tid)
			if tErr != nil {
				return tErr
			}
			merged := applyTemplate(
				templateShortcutReply, tpl,
				replyTo, ccFlag, bccFlag,
				buildReplySubject(orig.subject), body,
				"", "", "", runtime.Str("subject"), "",
			)
			replyTo = merged.To
			ccFlag = merged.Cc
			bccFlag = merged.Bcc
			body = merged.Body
			if !plainText && merged.IsPlainTextMode {
				plainText = true
			}
			templateLargeAttachmentIDs = merged.LargeAttachmentIDs
			templateInlineAttachments = merged.InlineAttachments
			templateSmallAttachments = merged.SmallAttachments
			for _, w := range merged.Warnings {
				fmt.Fprintf(runtime.IO().ErrOut, "warning: %s\n", w)
			}
			// Reply/reply-all/forward keep the Re:/Fw:-prefixed auto-subject
			// (or the user's --subject); template subject is deliberately
			// ignored for these shortcuts so threading with the original
			// conversation is preserved.
			inlineCount, largeCount := countAttachmentsByType(tpl.Attachments)
			logTemplateInfo(runtime, "apply.reply", map[string]interface{}{
				"mailbox_id":         mailboxID,
				"template_id":        tid,
				"is_plain_text_mode": plainText,
				"attachments_total":  len(tpl.Attachments),
				"inline_count":       inlineCount,
				"large_count":        largeCount,
				"tos_count":          countAddresses(replyTo),
				"ccs_count":          countAddresses(ccFlag),
				"bccs_count":         countAddresses(bccFlag),
			})
		}
		// --subject (explicit override) takes precedence over auto-generated.
		subjectOverride := strings.TrimSpace(runtime.Str("subject"))

		useHTML := !plainText && (bodyIsHTML(body) || bodyIsHTML(orig.bodyRaw) || sigResult != nil)
		if strings.TrimSpace(inlineFlag) != "" && !useHTML {
			return fmt.Errorf("--inline requires HTML mode, but neither the new body nor the original message contains HTML")
		}
		var bodyStr string
		if useHTML {
			bodyStr = buildBodyDiv(body, bodyIsHTML(body))
		} else {
			bodyStr = body
		}
		if err := validateRecipientCount(replyTo, ccFlag, bccFlag); err != nil {
			return err
		}

		quoted := quoteForReply(&orig, useHTML)
		subjectLine := buildReplySubject(orig.subject)
		if subjectOverride != "" {
			subjectLine = subjectOverride
		}
		bld := emlbuilder.New().WithFileIO(runtime.FileIO()).
			Subject(subjectLine).
			ToAddrs(parseNetAddrs(replyTo))
		if senderEmail != "" {
			bld = bld.From("", senderEmail)
		}
		// Note: requireSenderForRequestReceipt already ran above against
		// resolvedSender (pre-fallback). When --request-receipt is set we
		// are guaranteed resolvedSender != "", so senderEmail == resolvedSender.
		if runtime.Bool("request-receipt") {
			bld = bld.DispositionNotificationTo("", senderEmail)
		}
		if ccFlag != "" {
			bld = bld.CCAddrs(parseNetAddrs(ccFlag))
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
		var composedHTMLBody string
		var composedTextBody string
		var srcInlineBytes int64
		if useHTML {
			if err := validateInlineImageURLs(sourceMsg); err != nil {
				return fmt.Errorf("HTML reply blocked: %w", err)
			}
			var srcCIDs []string
			bld, srcCIDs, srcInlineBytes, err = addInlineImagesToBuilder(runtime, bld, sourceMsg.InlineImages)
			if err != nil {
				return err
			}
			resolved, refs, resolveErr := draftpkg.ResolveLocalImagePaths(bodyStr)
			if resolveErr != nil {
				return resolveErr
			}
			bodyWithSig := resolved
			if sigResult != nil {
				bodyWithSig += draftpkg.SignatureSpacing() + draftpkg.BuildSignatureHTML(sigResult.ID, sigResult.RenderedContent)
			}
			composedHTMLBody = bodyWithSig + quoted
			bld = bld.HTMLBody([]byte(composedHTMLBody))
			bld = addSignatureImagesToBuilder(bld, sigResult)
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
			var tplInlineCIDs []string
			bld, tplInlineCIDs, err = embedTemplateInlineAttachments(ctx, runtime, bld, bodyWithSig, mailboxID, templateID, templateInlineAttachments)
			if err != nil {
				return err
			}
			userCIDs = append(userCIDs, tplInlineCIDs...)
			if err := validateInlineCIDs(bodyWithSig, append(userCIDs, signatureCIDs(sigResult)...), srcCIDs); err != nil {
				return err
			}
		} else {
			composedTextBody = bodyStr + quoted
			bld = bld.TextBody([]byte(composedTextBody))
		}
		// Embed template SMALL non-inline attachments regardless of body mode.
		var templateSmallBytes int64
		bld, templateSmallBytes, err = embedTemplateSmallAttachments(ctx, runtime, bld, mailboxID, templateID, templateSmallAttachments)
		if err != nil {
			return err
		}
		bld = applyPriority(bld, priority)
		if calData := buildCalendarBody(runtime, senderEmail, replyTo, ccFlag); calData != nil {
			bld = bld.CalendarBody(calData)
		}
		allInlinePaths := append(inlineSpecFilePaths(inlineSpecs), autoResolvedPaths...)
		composedBodySize := int64(len(composedHTMLBody) + len(composedTextBody))
		emlBase := estimateEMLBaseSize(runtime.FileIO(), composedBodySize, allInlinePaths, srcInlineBytes) + templateSmallBytes
		bld, err = processLargeAttachments(ctx, runtime, bld, composedHTMLBody, composedTextBody, splitByComma(attachFlag), emlBase, 0)
		if err != nil {
			return err
		}
		if hdr, hdrErr := encodeTemplateLargeAttachmentHeader(templateLargeAttachmentIDs); hdrErr == nil && hdr != "" {
			bld = bld.Header(draftpkg.LargeAttachmentIDsHeader, hdr)
		}
		rawEML, err := bld.BuildBase64URL()
		if err != nil {
			return fmt.Errorf("failed to build EML: %w", err)
		}

		draftResult, err := draftpkg.CreateWithRaw(runtime, mailboxID, rawEML)
		if err != nil {
			return fmt.Errorf("failed to create draft: %w", err)
		}
		if !confirmSend {
			runtime.Out(buildDraftSavedOutput(draftResult, mailboxID), nil)
			hintSendDraft(runtime, mailboxID, draftResult.DraftID)
			return nil
		}
		resData, err := draftpkg.Send(runtime, mailboxID, draftResult.DraftID, sendTime)
		if err != nil {
			return fmt.Errorf("failed to send reply (draft %s created but not sent): %w", draftResult.DraftID, err)
		}
		runtime.Out(buildDraftSendOutput(resData, mailboxID), nil)
		hintMarkAsRead(runtime, mailboxID, messageId)
		return nil
	},
}
