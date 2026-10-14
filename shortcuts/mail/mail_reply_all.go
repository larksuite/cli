// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"context"
	"fmt"
	netmail "net/mail"
	"strings"

	"github.com/larksuite/cli/shortcuts/common"
	draftpkg "github.com/larksuite/cli/shortcuts/mail/draft"
	"github.com/larksuite/cli/shortcuts/mail/emlbuilder"
)

// MailReplyAll is the `+reply-all` shortcut: reply to the sender plus all
// recipients of a message (with address dedup and self-exclusion), saving a
// draft by default (or sending immediately with --confirm-send).
var MailReplyAll = common.Shortcut{
	Service:     "mail",
	Command:     "+reply-all",
	Description: "Reply to all recipients and save as draft (default). Use --confirm-send to send immediately after user confirmation. Includes all original To and CC automatically.",
	Risk:        "write",
	Scopes:      []string{"mail:user_mailbox.message:modify", "mail:user_mailbox.message:readonly", "mail:user_mailbox:readonly", "mail:user_mailbox.message.address:read", "mail:user_mailbox.message.subject:read", "mail:user_mailbox.message.body:read"},
	AuthTypes:   []string{"user"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "message-id", Desc: "Required. Message ID to reply to all recipients", Required: true},
		{Name: "body", Desc: "Reply body. Prefer HTML for rich formatting; plain text is also supported. Body type is auto-detected from the reply body and the original message. Use --plain-text to force plain-text mode. Mutually exclusive with --body-file. Required unless --template-id supplies a non-empty body."},
		bodyFileFlag,
		{Name: "from", Desc: "Sender email address for the From header. When using an alias (send_as) address, set this to the alias and use --mailbox for the owning mailbox. Defaults to the mailbox's primary address."},
		{Name: "mailbox", Desc: "Mailbox email address that owns the draft (default: falls back to --from, then me). Use this when the sender (--from) differs from the mailbox, e.g. sending via an alias or send_as address."},
		{Name: "to", Type: "string_array", Desc: "Additional To address. Repeat --to once per recipient; quote each value; appended to original recipients."},
		{Name: "cc", Type: "string_array", Desc: "Additional CC email address. Repeat --cc once per recipient; quote each value."},
		{Name: "bcc", Type: "string_array", Desc: "BCC email address. Repeat --bcc once per recipient; quote each value."},
		{Name: "remove", Type: "string_array", Desc: "Address to exclude from the outgoing reply. Repeat --remove once per address; quote each value."},
		{Name: "plain-text", Type: "bool", Desc: "Force plain-text mode, ignoring all HTML auto-detection. Cannot be used with --inline."},
		{Name: "attach", Type: "string_array", Desc: "Attachment file path, relative path only. Repeat --attach once per file; quote each value."},
		{Name: "inline", Type: "string_array", Desc: "Inline image as one JSON object. Repeat --inline once per image; quote each value. Example value: '{\"cid\":\"<unique-id>\",\"file_path\":\"<relative-path>\"}'. file_path must be relative. Reference it from HTML as <img src=\"cid:<unique-id>\">. CID must be unique, e.g. a random hex string. Cannot be used with --plain-text."},
		{Name: "confirm-send", Type: "bool", Desc: "Send the reply immediately instead of saving as draft. Only use after the user has explicitly confirmed recipients and content."},
		{Name: "send-time", Desc: "Scheduled send time as a Unix timestamp in seconds. Must be at least 5 minutes in the future. Use with --confirm-send to schedule the email."},
		{Name: "request-receipt", Type: "bool", Desc: "Request a read receipt (Message Disposition Notification, RFC 3798) addressed to the sender. Recipient mail clients may prompt the user, send automatically, or silently ignore — delivery of a receipt is not guaranteed."},
		{Name: "subject", Desc: "Optional. Override the auto-generated Re: subject. When set, the shortcut uses this value verbatim instead of prefixing the original subject."},
		{Name: "template-id", Desc: "Optional. Apply a saved template by ID (decimal integer string) before composing. The template's body/to/cc/bcc/attachments are appended to the reply-derived values."},
		signatureFlag,
		noSignatureFlag,
		priorityFlag,
		eventSummaryFlag, eventStartFlag, eventEndFlag, eventLocationFlag,
		showLintDetailsFlag},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		messageId := runtime.Str("message-id")
		confirmSend := runtime.Bool("confirm-send")
		mailboxID := resolveComposeMailboxID(runtime)
		desc := "Reply-all: fetch original message (with recipients) → resolve sender address → save as draft"
		if confirmSend {
			desc = "Reply-all (--confirm-send): fetch original message (with recipients) → resolve sender address → create draft → send draft"
		}
		api := common.NewDryRunAPI().Desc(desc)
		if tid := runtime.Str("template-id"); tid != "" {
			api = api.GET(templateMailboxPath(mailboxID, tid)).
				Desc("Fetch template to merge with reply-all-derived recipients / body.")
		}
		api = api.GET(mailboxPath(mailboxID, "messages", messageId)).
			GET(mailboxPath(mailboxID, "profile")).
			GET(mailboxPath(mailboxID, "settings", "send_as")).
			POST(mailboxPath(mailboxID, "drafts")).
			Body(map[string]interface{}{"raw": "<base64url-EML>"})
		if confirmSend {
			api = api.POST(mailboxPath(mailboxID, "drafts", "<draft_id>", "send"))
		}
		return api
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if _, err := buildRemoveSet(runtime.StrArray("remove")); err != nil {
			return err
		}
		attach := normalizeCommaFlagValues(runtime.StrArray("attach"))
		inline, err := normalizeInlineFlagValues(runtime.StrArray("inline"))
		if err != nil {
			return err
		}
		if err := validateTemplateID(runtime.Str("template-id")); err != nil {
			return err
		}
		hasTemplate := runtime.Str("template-id") != ""
		bodyFlag := runtime.Str("body")
		bodyFile := strings.TrimSpace(runtime.Str("body-file"))
		if err := validateBodyFileMutex(bodyFlag, bodyFile, runtime.ValidatePath); err != nil {
			return err
		}
		body, bErr := resolveBodyFromFlags(runtime)
		if bErr != nil {
			return bErr
		}
		if err := validateRequiredResolvedBody(body, hasTemplate, "--body or --body-file is required; pass the reply body (or use --template-id)"); err != nil {
			return err
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
		if err := validateNoSignatureConflict(runtime.Bool("no-signature"), runtime.Str("signature-id")); err != nil {
			return err
		}
		if err := validateEventFlags(runtime); err != nil {
			return err
		}
		if err := validateComposeInlineAndAttachments(runtime.FileIO(), attach, inline, runtime.Bool("plain-text"), ""); err != nil {
			return err
		}
		return validatePriorityFlag(runtime)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		messageId := runtime.Str("message-id")
		body, bErr := resolveBodyFromFlags(runtime)
		if bErr != nil {
			return bErr
		}
		toFlag := normalizeRecipientFlagValues(runtime.StrArray("to"))
		ccFlag := normalizeRecipientFlagValues(runtime.StrArray("cc"))
		bccFlag := normalizeRecipientFlagValues(runtime.StrArray("bcc"))
		removeList := runtime.StrArray("remove")
		removeSet, err := buildRemoveSet(removeList)
		if err != nil {
			return err
		}
		plainText := runtime.Bool("plain-text")
		attachFlag := normalizeCommaFlagValues(runtime.StrArray("attach"))
		inlineFlag, err := normalizeInlineFlagValues(runtime.StrArray("inline"))
		if err != nil {
			return err
		}
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

		mailboxID := resolveComposeMailboxID(runtime)
		sourceMsg, err := fetchComposeSourceMessage(runtime, mailboxID, messageId)
		if err != nil {
			return mailDecorateProblemMessage(err, "failed to fetch original message")
		}
		orig := sourceMsg.Original
		stripLargeAttachmentCard(&orig)

		resolvedSender := resolveComposeSenderEmail(runtime)
		// Check --request-receipt BEFORE the orig.headTo fallback below:
		// the receipt's Disposition-Notification-To must point to an address
		// the caller explicitly controls, not to a fallback picked from the
		// original mail's headers (which may belong to someone else in a
		// shared-mailbox / multi-recipient scenario).
		if err := requireSenderForRequestReceipt(runtime, resolvedSender); err != nil {
			return err
		}
		senderEmail := resolvedSender
		if senderEmail == "" {
			senderEmail = orig.headTo
		}

		// Signature ID is resolved here (after senderEmail is finalised) so DefaultReplyID
		// matches the correct usage. The actual image download in resolveSignature is deferred
		// to after applyTemplate so the final plainText value (which a template can override
		// via IsPlainTextMode) is used for the downloadImages decision.
		signatureID := runtime.Str("signature-id")
		noSignature := runtime.Bool("no-signature")
		if noSignature {
			signatureID = ""
		} else if signatureID == "" {
			signatureID = autoResolveSignatureID(runtime, mailboxID, senderEmail, true /*isReply*/)
		}

		selfEmails := fetchSelfEmailSet(runtime, mailboxID)
		replyTarget := orig.replyTo
		isSelfSent := recipientSetContains(selfEmails, orig.headFrom) || sameRecipientAddress(orig.headFrom, senderEmail)
		if !isSelfSent && replyTarget == "" {
			replyTarget = orig.headFrom
		}
		toList, ccList := buildReplyAllRecipients(replyTarget, orig.toAddresses, orig.ccAddresses, senderEmail, selfEmails, isSelfSent)

		toList = mergeAddrLists(toList, toFlag)
		ccList = mergeAddrLists(ccList, ccFlag)

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
				templateShortcutReplyAll, tpl,
				toList, ccList, bccFlag,
				buildReplySubject(orig.subject), body,
				"", "", "", runtime.Str("subject"), "",
			)
			toList = merged.To
			ccList = merged.Cc
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
			inlineCount, largeCount := countAttachmentsByType(tpl.Attachments)
			logTemplateInfo(runtime, "apply.reply_all", map[string]interface{}{
				"mailbox_id":         mailboxID,
				"template_id":        tid,
				"is_plain_text_mode": plainText,
				"attachments_total":  len(tpl.Attachments),
				"inline_count":       inlineCount,
				"large_count":        largeCount,
				"tos_count":          countAddresses(toList),
				"ccs_count":          countAddresses(ccList),
				"bccs_count":         countAddresses(bccFlag),
			})
		}
		toList, ccList, bccFlag = filterAndDeduplicateReplyAllRecipients(toList, ccList, bccFlag, removeSet)
		if err := validateReplyAllRecipients(toList, ccList, bccFlag); err != nil {
			return err
		}
		// Resolve signature after template processing so plainText reflects any IsPlainTextMode
		// override from the template. This avoids downloading HTML signature images when the
		// template forces plain-text mode, which could cause CDN 403/5xx or timeout errors.
		sigResult, sigErr := resolveSignature(ctx, runtime, mailboxID, signatureID, senderEmail,
			runtime.Str("signature-id") != "", !plainText)
		if sigErr != nil {
			return sigErr
		}
		subjectOverride := strings.TrimSpace(runtime.Str("subject"))

		if err := validateRecipientCount(toList, ccList, bccFlag); err != nil {
			return err
		}

		useHTML := !plainText && (bodyIsHTML(body) || bodyIsHTML(orig.bodyRaw) || sigResult != nil)
		if strings.TrimSpace(inlineFlag) != "" && !useHTML {
			return mailValidationParamError("--inline", "--inline requires HTML mode, but neither the new body nor the original message contains HTML")
		}
		var bodyStr string
		if useHTML {
			bodyStr = buildBodyDiv(body, bodyIsHTML(body))
		} else {
			bodyStr = body
		}
		quoted := quoteForReply(&orig, useHTML)
		subjectLine := buildReplySubject(orig.subject)
		if subjectOverride != "" {
			subjectLine = subjectOverride
		}
		bld := emlbuilder.New().WithFileIO(runtime.FileIO()).
			Subject(subjectLine).
			ToAddrs(parseNetAddrs(toList))
		if senderEmail != "" {
			bld = bld.From("", senderEmail)
		}
		// Note: requireSenderForRequestReceipt already ran above against
		// resolvedSender (pre-fallback). When --request-receipt is set we
		// are guaranteed resolvedSender != "", so senderEmail == resolvedSender.
		if runtime.Bool("request-receipt") {
			bld = bld.DispositionNotificationTo("", senderEmail)
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
		var composedHTMLBody string
		var composedTextBody string
		var srcInlineBytes int64
		// Lint findings flowing into the writing-path stdout envelope.
		lintApplied, lintBlocked := emptyLintEnvelopeFields()
		if useHTML {
			if err := validateInlineImageURLs(sourceMsg); err != nil {
				return mailDecorateProblemMessage(err, "HTML reply-all blocked")
			}
			var srcCIDs []string
			bld, srcCIDs, srcInlineBytes, err = addInlineImagesToBuilder(runtime, bld, sourceMsg.InlineImages)
			if err != nil {
				return err
			}
			resolved, refs, resolveErr := draftpkg.ResolveLocalImagePaths(bodyStr)
			if resolveErr != nil {
				return mailValidationError("failed to resolve local image paths: %v", resolveErr).WithCause(resolveErr)
			}
			bodyWithSig := resolved
			if sigResult != nil {
				bodyWithSig += draftpkg.SignatureSpacing() + draftpkg.BuildSignatureHTML(sigResult.ID, sigResult.RenderedContent)
			}
			// Writing-path lint: same pattern as +reply — operate on bodyWithSig
			// only; the `quoted` block from the original message must NOT be
			// re-linted (it may contain Feishu-native quote-block classes that
			// the lint allow-list intentionally permits in pass-through).
			cleaned, rep := runWritePathLint(bodyWithSig)
			bodyWithSig = cleaned
			lintApplied, lintBlocked = rep.Applied, rep.Blocked
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
			composedTextBody = injectPlainTextSignature(bodyStr, sigResult) + quoted
			bld = bld.TextBody([]byte(composedTextBody))
		}
		// Embed template SMALL non-inline attachments regardless of body mode.
		var templateSmallBytes int64
		bld, templateSmallBytes, err = embedTemplateSmallAttachments(ctx, runtime, bld, mailboxID, templateID, templateSmallAttachments)
		if err != nil {
			return err
		}
		bld = applyPriority(bld, priority)
		if calData := buildCalendarBody(runtime, senderEmail, toList, ccList); calData != nil {
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
			return mailValidationError("failed to build EML: %v", err).WithCause(err)
		}

		draftResult, err := draftpkg.CreateWithRaw(runtime, mailboxID, rawEML)
		if err != nil {
			return mailDecorateProblemMessage(err, "failed to create draft")
		}
		showLintDetails := runtime.Bool("show-lint-details")
		if !confirmSend {
			out := buildDraftSavedOutput(draftResult, mailboxID)
			applyLintToEnvelope(out, lintApplied, lintBlocked, showLintDetails)
			addComposeHint(out)
			runtime.Out(out, nil)
			hintSendDraft(runtime, mailboxID, draftResult.DraftID)
			return nil
		}
		resData, err := draftpkg.Send(runtime, mailboxID, draftResult.DraftID, sendTime)
		if err != nil {
			return mailDecorateProblemMessage(err, "failed to send reply-all (draft %s created but not sent)", draftResult.DraftID)
		}
		out := buildDraftSendOutput(resData, mailboxID)
		applyLintToEnvelope(out, lintApplied, lintBlocked, showLintDetails)
		addComposeHint(out)
		runtime.Out(out, nil)
		hintMarkAsRead(runtime, mailboxID, messageId)
		return nil
	},
}

// sameRecipientAddress reports whether two mailbox strings identify the same
// non-empty email address.
func sameRecipientAddress(left, right string) bool {
	leftKey := recipientAddressKey(left)
	return leftKey != "" && leftKey == recipientAddressKey(right)
}

// recipientSetContains reports whether raw belongs to a normalized set of
// current-user addresses. Normalizing both sides keeps display-name and case
// differences from changing self-sent detection.
func recipientSetContains(set map[string]bool, raw string) bool {
	key := recipientAddressKey(raw)
	return key != "" && set[key]
}

// buildRemoveSet validates explicit removals and indexes them by bare email
// address. Keeping this separate from automatic self-exclusion ensures
// self-sent messages retain their original To/Cc recipients unless the user
// explicitly removes them.
func buildRemoveSet(remove []string) (map[string]bool, error) {
	set := make(map[string]bool, len(remove))
	for _, rawList := range remove {
		rawList = strings.TrimSpace(rawList)
		if rawList == "" {
			return nil, mailValidationParamError("--remove", "email address must not be empty")
		}
		addresses, err := netmail.ParseAddressList(rawList)
		if err != nil {
			return nil, mailValidationParamError("--remove", "invalid email address %q", rawList).
				WithCause(err)
		}
		for _, addr := range addresses {
			key := strings.ToLower(strings.TrimSpace(addr.Address))
			if key == "" {
				return nil, mailValidationParamError("--remove", "invalid email address %q", rawList)
			}
			set[key] = true
		}
	}
	return set, nil
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
func buildReplyAllRecipients(replyTarget string, origTo, origCC []string, senderEmail string, selfEmails map[string]bool, isSelfSent bool) (to, cc string) {
	// Copy excluded to avoid mutating the caller's map.
	excluded := make(map[string]bool, len(selfEmails)+1)
	for k, v := range selfEmails {
		excluded[strings.ToLower(strings.TrimSpace(k))] = v
	}
	// Ensure senderEmail (which may be an alias or shared mailbox) is also excluded.
	if senderEmail != "" {
		excluded[recipientAddressKey(senderEmail)] = true
	}

	if isSelfSent {
		// Self-sent: preserve original To/Cc distinction.
		seen := make(map[string]bool)
		var toList []string
		for _, addr := range origTo {
			key := recipientAddressKey(addr)
			if key == "" || seen[key] {
				continue
			}
			seen[key] = true
			toList = append(toList, addr)
		}
		// replyTarget is non-empty only when the source message had a real
		// Reply-To header; a From fallback is never supplied for self-sent mail.
		if key := recipientAddressKey(replyTarget); key != "" && !seen[key] {
			toList = append(toList, replyTarget)
			seen[key] = true
		}
		var ccList []string
		for _, addr := range origCC {
			key := recipientAddressKey(addr)
			if key == "" || seen[key] {
				continue
			}
			seen[key] = true
			ccList = append(ccList, addr)
		}
		return strings.Join(toList, ", "), strings.Join(ccList, ", ")
	}

	// Normal case: original sender → To; origTo+origCC → Cc.
	if key := recipientAddressKey(replyTarget); key != "" && !excluded[key] {
		to = replyTarget
	}

	seen := make(map[string]bool)
	if key := recipientAddressKey(replyTarget); key != "" {
		seen[key] = true
	}
	var ccList []string
	for _, addr := range append(origTo, origCC...) {
		key := recipientAddressKey(addr)
		if key == "" || excluded[key] || seen[key] {
			continue
		}
		seen[key] = true
		ccList = append(ccList, addr)
	}
	cc = strings.Join(ccList, ", ")
	return to, cc
}

// filterAndDeduplicateReplyAllRecipients applies explicit removals after all
// sources (original message, flags and template) have been merged. It reuses
// parseNetAddrs for the existing stable, case-insensitive per-list de-duplication.
func filterAndDeduplicateReplyAllRecipients(to, cc, bcc string, remove map[string]bool) (string, string, string) {
	filter := func(raw string) string {
		addresses := parseNetAddrs(raw)
		kept := make([]string, 0, len(addresses))
		for _, address := range addresses {
			if _, err := netmail.ParseAddress(address.Address); err != nil {
				continue
			}
			key := strings.ToLower(strings.TrimSpace(address.Address))
			if key == "" || remove[key] {
				continue
			}
			kept = append(kept, (Mailbox{Name: address.Name, Email: address.Address}).rawString())
		}
		return strings.Join(kept, ", ")
	}
	return filter(to), filter(cc), filter(bcc)
}

// validateReplyAllRecipients rejects a reply-all whose filtering step left no
// valid address in To, Cc or Bcc.
func validateReplyAllRecipients(to, cc, bcc string) error {
	if strings.TrimSpace(to) != "" || strings.TrimSpace(cc) != "" || strings.TrimSpace(bcc) != "" {
		return nil
	}
	return mailValidationError("no valid recipients remain after applying --remove").
		WithParams(mailInvalidParam("--remove", "all recipients were removed"))
}
