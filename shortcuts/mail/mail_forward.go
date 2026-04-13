// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

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
	},
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
		if runtime.Bool("confirm-send") {
			if err := validateComposeHasAtLeastOneRecipient(runtime.Str("to"), runtime.Str("cc"), runtime.Str("bcc")); err != nil {
				return err
			}
		}
		return validateComposeInlineAndAttachments(runtime.FileIO(), runtime.Str("attach"), runtime.Str("inline"), runtime.Bool("plain-text"), "")
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

		mailboxID := resolveComposeMailboxID(runtime)
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
		useHTML := !plainText && (bodyIsHTML(body) || bodyIsHTML(orig.bodyRaw))
		if strings.TrimSpace(inlineFlag) != "" && !useHTML {
			return fmt.Errorf("--inline requires HTML mode, but neither the new body nor the original message contains HTML")
		}
		inlineSpecs, err := parseInlineSpecs(inlineFlag)
		if err != nil {
			return err
		}
		var autoResolvedPaths []string
		if useHTML {
			if err := validateInlineImageURLs(sourceMsg); err != nil {
				return fmt.Errorf("forward blocked: %w", err)
			}
			processedBody := buildBodyDiv(body, bodyIsHTML(body))
			forwardQuote := buildForwardQuoteHTML(&orig)
			var srcCIDs []string
			bld, srcCIDs, err = addInlineImagesToBuilder(runtime, bld, sourceMsg.InlineImages)
			if err != nil {
				return err
			}
			resolved, refs, resolveErr := draftpkg.ResolveLocalImagePaths(processedBody)
			if resolveErr != nil {
				return resolveErr
			}
			fullHTML := resolved + forwardQuote
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
			bld = bld.TextBody([]byte(buildForwardedMessage(&orig, body)))
		}
		// Download original attachments and accumulate size for limit check
		type downloadedAtt struct {
			content     []byte
			contentType string
			filename    string
		}
		var origAtts []downloadedAtt
		var origAttBytes int64
		type largeAttID struct {
			ID string `json:"id"`
		}
		var largeAttIDs []largeAttID
		for _, att := range sourceMsg.ForwardAttachments {
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
			origAttBytes += int64(len(content))
		}
		if len(largeAttIDs) > 0 {
			idsJSON, err := json.Marshal(largeAttIDs)
			if err != nil {
				return fmt.Errorf("failed to encode large attachment IDs: %w", err)
			}
			bld = bld.Header("X-Lms-Large-Attachment-Ids", base64.StdEncoding.EncodeToString(idsJSON))
		}
		allFilePaths := append(append(splitByComma(attachFlag), inlineSpecFilePaths(inlineSpecs)...), autoResolvedPaths...)
		if err := checkAttachmentSizeLimit(runtime.FileIO(), allFilePaths, origAttBytes, len(origAtts)); err != nil {
			return err
		}
		for _, att := range origAtts {
			bld = bld.AddAttachment(att.content, att.contentType, att.filename)
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
			return fmt.Errorf("failed to send forward (draft %s created but not sent): %w", draftID, err)
		}
		runtime.Out(buildSendResult(resData, mailboxID), nil)
		hintMarkAsRead(runtime, mailboxID, messageId)
		return nil
	},
}
