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
		signatureFlag,
		priorityFlag},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		messageId := runtime.Str("message-id")
		to := runtime.Str("to")
		confirmSend := runtime.Bool("confirm-send")
		mailboxID := resolveComposeMailboxID(runtime)
		desc := "Forward: fetch original message → resolve sender address → save as draft"
		if confirmSend {
			desc = "Forward (--confirm-send): fetch original message → resolve sender address → create draft → send draft"
		}
		api := common.NewDryRunAPI().
			Desc(desc).
			GET(mailboxPath(mailboxID, "messages", messageId)).
			GET(mailboxPath(mailboxID, "profile")).
			POST(mailboxPath(mailboxID, "drafts")).
			Body(map[string]interface{}{"raw": "<base64url-EML>", "_to": to})
		if confirmSend {
			api = api.POST(mailboxPath(mailboxID, "drafts", "<draft_id>", "send"))
		}
		return api
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if err := validateConfirmSendScope(runtime); err != nil {
			return err
		}
		if err := validateSendTime(runtime); err != nil {
			return err
		}
		if runtime.Bool("confirm-send") {
			if err := validateComposeHasAtLeastOneRecipient(runtime.Str("to"), runtime.Str("cc"), runtime.Str("bcc")); err != nil {
				return err
			}
		}
		if err := validateSignatureWithPlainText(runtime.Bool("plain-text"), runtime.Str("signature-id")); err != nil {
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

		senderEmail := resolveComposeSenderEmail(runtime)
		if senderEmail == "" {
			senderEmail = orig.headTo
		}

		if err := validateRecipientCount(to, ccFlag, bccFlag); err != nil {
			return err
		}

		bld := emlbuilder.New().WithFileIO(runtime.FileIO()).
			Subject(buildForwardSubject(orig.subject)).
			ToAddrs(parseNetAddrs(to))
		if senderEmail != "" {
			bld = bld.From("", senderEmail)
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
			if err := validateInlineCIDs(bodyWithSig, append(userCIDs, signatureCIDs(sigResult)...), srcCIDs); err != nil {
				return err
			}
		} else {
			composedTextBody = buildForwardedMessage(&orig, body)
			bld = bld.TextBody([]byte(composedTextBody))
		}
		bld = applyPriority(bld, priority)
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
		emlBase := estimateEMLBaseSize(runtime.FileIO(), composedBodySize, allInlinePaths, srcInlineBytes)

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

		// Embed normal attachments.
		for _, f := range classified.Normal {
			if f.Path == "" {
				att := origAtts[f.SourceIndex]
				bld = bld.AddAttachment(att.content, att.contentType, att.filename)
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

		if len(largeAttIDs) > 0 {
			idsJSON, err := json.Marshal(largeAttIDs)
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
