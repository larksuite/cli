// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
	draftpkg "github.com/larksuite/cli/shortcuts/mail/draft"
	"github.com/larksuite/cli/shortcuts/mail/emlbuilder"
)

// MailForward is the `+forward` shortcut: forward an existing message to
// new recipients, saving a draft by default (or sending immediately with
// --confirm-send). Original message block is included automatically.
var MailForward = common.Shortcut{
	Service:     "mail",
	Command:     "+forward",
	Description: "Forward a message and save as draft (default). Use --confirm-send to send immediately after user confirmation. Original message block included automatically.",
	Risk:        "write",
	Scopes:      []string{"mail:user_mailbox.message:modify", "mail:user_mailbox.message:readonly", "mail:user_mailbox:readonly", "mail:user_mailbox.message.address:read", "mail:user_mailbox.message.subject:read", "mail:user_mailbox.message.body:read"},
	AuthTypes:   []string{"user"},
	Flags: []common.Flag{
		{Name: "message-id", Desc: "Required. Message ID to forward", Required: true},
		{Name: "to", Desc: "Recipient email address(es), comma-separated"},
		{Name: "body", Desc: "Body prepended before the forwarded message. Prefer HTML for rich formatting; plain text is also supported. Body type is auto-detected from the forward body and the original message. Use --plain-text to force plain-text mode."},
		{Name: "from", Desc: "Sender email address for the From header. When using an alias (send_as) address, set this to the alias and use --mailbox for the owning mailbox. Defaults to the mailbox's primary address."},
		{Name: "mailbox", Desc: "Mailbox email address that owns the draft (default: falls back to --from, then me). Use this when the sender (--from) differs from the mailbox, e.g. sending via an alias or send_as address."},
		{Name: "cc", Desc: "CC email address(es), comma-separated"},
		{Name: "bcc", Desc: "BCC email address(es), comma-separated"},
		{Name: "plain-text", Type: "bool", Desc: "Force plain-text mode, ignoring all HTML auto-detection. Cannot be used with --inline."},
		{Name: "attach", Desc: "Attachment file path(s), comma-separated, appended after original attachments (relative path only)"},
		{Name: "inline", Desc: "Inline images as a JSON array. Each entry: {\"cid\":\"<unique-id>\",\"file_path\":\"<relative-path>\"}. All file_path values must be relative paths. Cannot be used with --plain-text. CID images are embedded via <img src=\"cid:...\"> in the HTML body. CID is a unique identifier, e.g. a random hex string like \"a1b2c3d4e5f6a7b8c9d0\"."},
		{Name: "confirm-send", Type: "bool", Desc: "Send the forward immediately instead of saving as draft. Only use after the user has explicitly confirmed recipients and content."},
		{Name: "send-time", Desc: "Scheduled send time as a Unix timestamp in seconds. Must be at least 5 minutes in the future. Use with --confirm-send to schedule the email."},
		{Name: "request-receipt", Type: "bool", Desc: "Request a read receipt (Message Disposition Notification, RFC 3798) addressed to the sender. Recipient mail clients may prompt the user, send automatically, or silently ignore — delivery of a receipt is not guaranteed."},
		{Name: "subject", Desc: "Optional. Override the auto-generated Fw: subject. When set, the shortcut uses this value verbatim instead of prefixing the original subject."},
		{Name: "template-id", Desc: "Optional. Apply a saved template by ID (decimal integer string) before composing. The template's body/to/cc/bcc/attachments are merged into the forward draft (template values appended to user flags / forward-derived values; no de-duplication)."},
		signatureFlag,
		priorityFlag,
		eventSummaryFlag, eventStartFlag, eventEndFlag, eventLocationFlag},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		messageId := runtime.Str("message-id")
		to := runtime.Str("to")
		confirmSend := runtime.Bool("confirm-send")
		mailboxID := resolveComposeMailboxID(runtime)
		desc := "Forward: fetch original message → resolve sender address → save as draft"
		if confirmSend {
			desc = "Forward (--confirm-send): fetch original message → resolve sender address → create draft → send draft"
		}
		api := common.NewDryRunAPI().Desc(desc)
		if tid := runtime.Str("template-id"); tid != "" {
			api = api.GET(templateMailboxPath(mailboxID, tid)).
				Desc("Fetch template to merge with forward compose flags.")
		}
		api = api.GET(mailboxPath(mailboxID, "messages", messageId)).
			GET(mailboxPath(mailboxID, "profile")).
			POST(mailboxPath(mailboxID, "drafts")).
			Body(map[string]interface{}{"raw": "<base64url-EML>", "_to": to})
		if confirmSend {
			api = api.POST(mailboxPath(mailboxID, "drafts", "<draft_id>", "send"))
		}
		return api
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if err := validateTemplateID(runtime.Str("template-id")); err != nil {
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
		// With --template-id, recipients may come from the template; defer
		// the check to Execute (post-applyTemplate). Mirrors +send.
		if runtime.Bool("confirm-send") && runtime.Str("template-id") == "" {
			if err := validateComposeHasAtLeastOneRecipient(runtime.Str("to"), runtime.Str("cc"), runtime.Str("bcc")); err != nil {
				return err
			}
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
		to := runtime.Str("to")
		body := runtime.Str("body")
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
		if err := validateForwardAttachmentURLs(sourceMsg); err != nil {
			return fmt.Errorf("forward blocked: %w", err)
		}
		orig := sourceMsg.Original

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
				templateShortcutForward, tpl,
				to, ccFlag, bccFlag,
				buildForwardSubject(orig.subject), body,
				"", "", "", runtime.Str("subject"), "",
			)
			to = merged.To
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
			inlineCount, largeCount := countAttachmentsByType(tpl.Attachments)
			logTemplateInfo(runtime, "apply.forward", map[string]interface{}{
				"mailbox_id":         mailboxID,
				"template_id":        tid,
				"is_plain_text_mode": plainText,
				"attachments_total":  len(tpl.Attachments),
				"inline_count":       inlineCount,
				"large_count":        largeCount,
				"tos_count":          countAddresses(to),
				"ccs_count":          countAddresses(ccFlag),
				"bccs_count":         countAddresses(bccFlag),
			})
		}
		subjectOverride := strings.TrimSpace(runtime.Str("subject"))

		// Post-merge recipient check for --confirm-send + --template-id:
		// Validate skipped this when a template was supplied; enforce it now
		// after applyTemplate has folded in the template addresses.
		if confirmSend && templateID != "" {
			if err := validateComposeHasAtLeastOneRecipient(to, ccFlag, bccFlag); err != nil {
				return err
			}
		}

		if err := validateRecipientCount(to, ccFlag, bccFlag); err != nil {
			return err
		}

		subjectLine := buildForwardSubject(orig.subject)
		if subjectOverride != "" {
			subjectLine = subjectOverride
		}
		bld := emlbuilder.New().WithFileIO(runtime.FileIO()).
			Subject(subjectLine).
			ToAddrs(parseNetAddrs(to))
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
		useHTML := !plainText && (bodyIsHTML(body) || bodyIsHTML(orig.bodyRaw) || sigResult != nil)
		if strings.TrimSpace(inlineFlag) != "" && !useHTML {
			return fmt.Errorf("--inline requires HTML mode, but neither the new body nor the original message contains HTML")
		}
		inlineSpecs, err := parseInlineSpecs(inlineFlag)
		if err != nil {
			return err
		}
		var autoResolvedPaths []string
		var composedHTMLBody string
		var composedTextBody string
		var srcInlineBytes int64
		if useHTML {
			if err := validateInlineImageURLs(sourceMsg); err != nil {
				return fmt.Errorf("forward blocked: %w", err)
			}
			processedBody := buildBodyDiv(body, bodyIsHTML(body))
			origLargeAttCard := stripLargeAttachmentCard(&orig)
			for id := range sourceMsg.FailedAttachmentIDs {
				if updated, ok := draftpkg.RemoveLargeFileItemFromHTML(origLargeAttCard, id); ok {
					origLargeAttCard = updated
				}
			}
			forwardQuote := buildForwardQuoteHTML(&orig)
			var srcCIDs []string
			bld, srcCIDs, srcInlineBytes, err = addInlineImagesToBuilder(runtime, bld, sourceMsg.InlineImages)
			if err != nil {
				return err
			}
			resolved, refs, resolveErr := draftpkg.ResolveLocalImagePaths(processedBody)
			if resolveErr != nil {
				return resolveErr
			}
			bodyWithSig := resolved
			if sigResult != nil {
				bodyWithSig += draftpkg.SignatureSpacing() + draftpkg.BuildSignatureHTML(sigResult.ID, sigResult.RenderedContent)
			}
			composedHTMLBody = bodyWithSig + origLargeAttCard + forwardQuote
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
			composedTextBody = buildForwardedMessage(&orig, body)
			bld = bld.TextBody([]byte(composedTextBody))
		}
		// Embed template SMALL non-inline attachments regardless of body mode.
		// Template LARGE entries keep going through the X-Lms-Large-Attachment-Ids
		// header below; inline already ran in the HTML branch above.
		var templateSmallBytes int64
		bld, templateSmallBytes, err = embedTemplateSmallAttachments(ctx, runtime, bld, mailboxID, templateID, templateSmallAttachments)
		if err != nil {
			return err
		}
		bld = applyPriority(bld, priority)
		if calData := buildCalendarBody(runtime, senderEmail, to, ccFlag); calData != nil {
			bld = bld.CalendarBody(calData)
		} else if len(sourceMsg.OriginalCalendarICS) > 0 {
			bld = bld.CalendarBody(sourceMsg.OriginalCalendarICS)
		}
		// Download original attachments, separating normal from large.
		type downloadedAtt struct {
			content     []byte
			contentType string
			filename    string
		}
		var origAtts []downloadedAtt
		var largeAttIDs []largeAttID
		var skippedAtts []string
		for _, att := range sourceMsg.ForwardAttachments {
			if sourceMsg.FailedAttachmentIDs[att.ID] {
				skippedAtts = append(skippedAtts, att.Filename)
				continue
			}
			if att.AttachmentType == attachmentTypeLarge {
				largeAttIDs = append(largeAttIDs, largeAttID{ID: att.ID})
				continue
			}
			content, err := downloadAttachmentContent(runtime, att.DownloadURL)
			if err != nil {
				return fmt.Errorf("failed to download original attachment %s: %w", att.Filename, err)
			}
			contentType := att.ContentType
			if contentType == "" {
				contentType = "application/octet-stream"
			}
			origAtts = append(origAtts, downloadedAtt{content, contentType, att.Filename})
		}
		if len(skippedAtts) > 0 {
			fmt.Fprintf(runtime.IO().ErrOut, "warning: skipped %d invalid attachment(s): %s\n",
				len(skippedAtts), strings.Join(skippedAtts, ", "))
		}

		// Classify ALL attachments (original + user-added) together so that
		// original attachments exceeding the EML limit are uploaded as large
		// attachments instead of being embedded.
		allInlinePaths := append(inlineSpecFilePaths(inlineSpecs), autoResolvedPaths...)
		composedBodySize := int64(len(composedHTMLBody) + len(composedTextBody))
		emlBase := estimateEMLBaseSize(runtime.FileIO(), composedBodySize, allInlinePaths, srcInlineBytes) + templateSmallBytes

		var allFiles []attachmentFile
		for i, att := range origAtts {
			allFiles = append(allFiles, attachmentFile{
				FileName:    att.filename,
				Size:        int64(len(att.content)),
				SourceIndex: i,
			})
		}
		userFiles, err := statAttachmentFiles(runtime.FileIO(), splitByComma(attachFlag))
		if err != nil {
			return err
		}
		for _, f := range userFiles {
			if f.Size > MaxLargeAttachmentSize {
				return output.ErrValidation("attachment %s (%.1f GB) exceeds the %.0f GB single file limit",
					f.FileName, float64(f.Size)/1024/1024/1024, float64(MaxLargeAttachmentSize)/1024/1024/1024)
			}
		}
		totalCount := len(origAtts) + len(largeAttIDs) + len(userFiles)
		if totalCount > MaxAttachmentCount {
			return output.ErrValidation("attachment count %d exceeds the limit of %d", totalCount, MaxAttachmentCount)
		}
		allFiles = append(allFiles, userFiles...)
		classified := classifyAttachments(allFiles, emlBase)

		// Embed normal attachments. Pass application/octet-stream instead of
		// the original's declared content-type: the backend canonicalizes
		// regular attachments to octet-stream on save/readback (see
		// AddFileAttachment's comment in emlbuilder/builder.go:459). Forwarding
		// an original image/png attachment with its real content-type trips
		// the backend's is_inline heuristic — the draft read-back surfaces
		// the attachment as is_inline=true with cid="" and the mail client
		// drops it from the attachment list. Mirror AddFileAttachment's
		// canonical type so originals round-trip as real attachments.
		for _, f := range classified.Normal {
			if f.Path == "" {
				att := origAtts[f.SourceIndex]
				bld = bld.AddAttachment(att.content, "application/octet-stream", att.filename)
			} else {
				bld = bld.AddFileAttachment(f.Path)
			}
		}

		// Upload oversized attachments as large attachments.
		if len(classified.Oversized) > 0 {
			if composedHTMLBody == "" && composedTextBody == "" {
				return output.ErrValidation("large attachments require a body; " +
					"empty messages cannot include the download link")
			}
			if runtime.Config == nil || runtime.UserOpenId() == "" {
				var totalBytes int64
				for _, f := range classified.Oversized {
					totalBytes += f.Size
				}
				return output.ErrValidation("total attachment size %.1f MB exceeds the 25 MB EML limit; "+
					"large attachment upload requires user identity (--as user)",
					float64(totalBytes)/1024/1024)
			}

			var allOversized []attachmentFile
			for _, f := range classified.Oversized {
				if f.Path == "" {
					att := origAtts[f.SourceIndex]
					allOversized = append(allOversized, attachmentFile{
						FileName: att.filename,
						Size:     int64(len(att.content)),
						Data:     att.content,
					})
				} else {
					allOversized = append(allOversized, f)
				}
			}
			uploadResults, err := uploadLargeAttachments(ctx, runtime, allOversized)
			if err != nil {
				return err
			}

			if composedHTMLBody != "" {
				largeHTML := buildLargeAttachmentHTML(runtime.Config.Brand, resolveLang(runtime), uploadResults)
				bld = bld.HTMLBody([]byte(draftpkg.InsertBeforeQuoteOrAppend(composedHTMLBody, largeHTML)))
			} else {
				largeText := buildLargeAttachmentPlainText(runtime.Config.Brand, resolveLang(runtime), uploadResults)
				bld = bld.TextBody([]byte(composedTextBody + largeText))
			}

			for _, r := range uploadResults {
				largeAttIDs = append(largeAttIDs, largeAttID{ID: r.FileToken})
			}

			fmt.Fprintf(runtime.IO().ErrOut, "  %d normal attachment(s) embedded in EML\n", len(classified.Normal))
			fmt.Fprintf(runtime.IO().ErrOut, "  %d large attachment(s) uploaded (download links in body)\n", len(classified.Oversized))
		}

		// Merge forward-derived (originals + user uploads) with
		// template-supplied LARGE attachment file_keys into a single header
		// value. emlbuilder.Builder.Header() appends; emitting two
		// X-Lms-Large-Attachment-Ids lines causes the server (and most
		// RFC 5322 parsers) to read only the first, silently dropping the
		// other set. Dedup by ID so a template that re-uses a forwarded
		// LARGE file_key doesn't double-register the reference.
		seenLargeID := make(map[string]bool, len(largeAttIDs)+len(templateLargeAttachmentIDs))
		mergedLargeAttIDs := make([]largeAttID, 0, len(largeAttIDs)+len(templateLargeAttachmentIDs))
		for _, e := range largeAttIDs {
			if e.ID == "" || seenLargeID[e.ID] {
				continue
			}
			seenLargeID[e.ID] = true
			mergedLargeAttIDs = append(mergedLargeAttIDs, e)
		}
		for _, id := range templateLargeAttachmentIDs {
			if id == "" || seenLargeID[id] {
				continue
			}
			seenLargeID[id] = true
			mergedLargeAttIDs = append(mergedLargeAttIDs, largeAttID{ID: id})
		}
		if len(mergedLargeAttIDs) > 0 {
			idsJSON, err := json.Marshal(mergedLargeAttIDs)
			if err != nil {
				return fmt.Errorf("failed to encode large attachment IDs: %w", err)
			}
			bld = bld.Header(draftpkg.LargeAttachmentIDsHeader, base64.StdEncoding.EncodeToString(idsJSON))
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
			return fmt.Errorf("failed to send forward (draft %s created but not sent): %w", draftResult.DraftID, err)
		}
		runtime.Out(buildDraftSendOutput(resData, mailboxID), nil)
		hintMarkAsRead(runtime, mailboxID, messageId)
		return nil
	},
}
