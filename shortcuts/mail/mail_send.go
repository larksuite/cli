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

// MailSend is the `+send` shortcut: compose a new email and save it as a
// draft by default (or send immediately with --confirm-send).
var MailSend = common.Shortcut{
	Service:     "mail",
	Command:     "+send",
	Description: "Compose a new email and save as draft (default). Use --confirm-send to send immediately after user confirmation.",
	Risk:        "write",
	Scopes:      []string{"mail:user_mailbox.message:send", "mail:user_mailbox.message:modify", "mail:user_mailbox:readonly"},
	AuthTypes:   []string{"user"},
	Flags: []common.Flag{
		{Name: "to", Desc: "Recipient email address(es), comma-separated"},
		{Name: "subject", Desc: "Email subject. Required unless --template-id supplies a non-empty subject."},
		{Name: "body", Desc: "Email body. Prefer HTML for rich formatting (bold, lists, links); plain text is also supported. Body type is auto-detected. Use --plain-text to force plain-text mode. Required unless --template-id supplies a non-empty body."},
		{Name: "from", Desc: "Sender email address for the From header. When using an alias (send_as) address, set this to the alias and use --mailbox for the owning mailbox. Defaults to the mailbox's primary address."},
		{Name: "mailbox", Desc: "Mailbox email address that owns the draft (default: falls back to --from, then me). Use this when the sender (--from) differs from the mailbox, e.g. sending via an alias or send_as address."},
		{Name: "cc", Desc: "CC email address(es), comma-separated"},
		{Name: "bcc", Desc: "BCC email address(es), comma-separated"},
		{Name: "plain-text", Type: "bool", Desc: "Force plain-text mode, ignoring HTML auto-detection. Cannot be used with --inline."},
		{Name: "attach", Desc: "Attachment file path(s), comma-separated (relative path only)"},
		{Name: "inline", Desc: "Inline images as a JSON array. Each entry: {\"cid\":\"<unique-id>\",\"file_path\":\"<relative-path>\"}. All file_path values must be relative paths. Cannot be used with --plain-text. CID images are embedded via <img src=\"cid:...\"> in the HTML body. CID is a unique identifier, e.g. a random hex string like \"a1b2c3d4e5f6a7b8c9d0\"."},
		{Name: "confirm-send", Type: "bool", Desc: "Send the email immediately instead of saving as draft. Only use after the user has explicitly confirmed recipients and content."},
		{Name: "send-time", Desc: "Scheduled send time as a Unix timestamp in seconds. Must be at least 5 minutes in the future. Use with --confirm-send to schedule the email."},
		{Name: "request-receipt", Type: "bool", Desc: "Request a read receipt (Message Disposition Notification, RFC 3798) addressed to the sender. Recipient mail clients may prompt the user, send automatically, or silently ignore — delivery of a receipt is not guaranteed."},
		{Name: "template-id", Desc: "Optional. Apply a saved template by ID (decimal integer string) before composing. The template's subject/body/to/cc/bcc/attachments are merged with user-supplied flags (user flags win). Requires --as user."},
		signatureFlag,
		priorityFlag,
		eventSummaryFlag, eventStartFlag, eventEndFlag, eventLocationFlag},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		to := runtime.Str("to")
		subject := runtime.Str("subject")
		confirmSend := runtime.Bool("confirm-send")
		mailboxID := resolveComposeMailboxID(runtime)
		desc := "Compose email → save as draft"
		if confirmSend {
			desc = "Compose email → save as draft → send draft"
		}
		api := common.NewDryRunAPI().Desc(desc)
		if tid := runtime.Str("template-id"); tid != "" {
			api = api.GET(templateMailboxPath(mailboxID, tid)).
				Desc("Fetch template to merge with compose flags (subject/body/to/cc/bcc/attachments).")
		}
		api = api.GET(mailboxPath(mailboxID, "profile")).
			POST(mailboxPath(mailboxID, "drafts")).
			Body(map[string]interface{}{
				"raw": "<base64url-EML>",
				"_preview": map[string]interface{}{
					"to":      to,
					"subject": subject,
				},
			})
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
		if !hasTemplate && strings.TrimSpace(runtime.Str("subject")) == "" {
			return output.ErrValidation("--subject is required; pass the final email subject (or use --template-id)")
		}
		if !hasTemplate && strings.TrimSpace(runtime.Str("body")) == "" {
			return output.ErrValidation("--body is required; pass the full email body (or use --template-id)")
		}
		// With --template-id, tos/ccs/bccs may come from the template, so
		// defer the at-least-one-recipient check to Execute (after
		// applyTemplate has merged the template addresses in).
		if !hasTemplate {
			if err := validateComposeHasAtLeastOneRecipient(runtime.Str("to"), runtime.Str("cc"), runtime.Str("bcc")); err != nil {
				return err
			}
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
		if err := validateComposeInlineAndAttachments(runtime.FileIO(), runtime.Str("attach"), runtime.Str("inline"), runtime.Bool("plain-text"), runtime.Str("body")); err != nil {
			return err
		}
		if err := validateEventFlags(runtime); err != nil {
			return err
		}
		return validatePriorityFlag(runtime)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		to := runtime.Str("to")
		subject := runtime.Str("subject")
		body := runtime.Str("body")
		ccFlag := runtime.Str("cc")
		bccFlag := runtime.Str("bcc")
		plainText := runtime.Bool("plain-text")
		attachFlag := runtime.Str("attach")
		inlineFlag := runtime.Str("inline")
		confirmSend := runtime.Bool("confirm-send")
		sendTime := runtime.Str("send-time")

		senderEmail := resolveComposeSenderEmail(runtime)
		signatureID := runtime.Str("signature-id")
		priority, err := parsePriority(runtime.Str("priority"))
		if err != nil {
			return err
		}

		mailboxID := resolveComposeMailboxID(runtime)

		// --template-id merge: fetch template and apply it to compose state.
		var templateLargeAttachmentIDs []string
		var templateInlineAttachments []templateInlineRef
		var templateSmallAttachments []templateAttachmentRef
		templateID := runtime.Str("template-id")
		if tid := templateID; tid != "" {
			tpl, err := fetchTemplate(runtime, mailboxID, tid)
			if err != nil {
				return err
			}
			merged := applyTemplate(
				templateShortcutSend, tpl,
				"", "", "", /* no pre-existing draft addrs for +send */
				"", "",
				to, ccFlag, bccFlag, subject, body,
			)
			to = merged.To
			ccFlag = merged.Cc
			bccFlag = merged.Bcc
			subject = merged.Subject
			body = merged.Body
			if !runtime.Bool("plain-text") && merged.IsPlainTextMode {
				plainText = true
			}
			templateLargeAttachmentIDs = merged.LargeAttachmentIDs
			templateInlineAttachments = merged.InlineAttachments
			templateSmallAttachments = merged.SmallAttachments
			for _, w := range merged.Warnings {
				fmt.Fprintf(runtime.IO().ErrOut, "warning: %s\n", w)
			}
			inlineCount, largeCount := countAttachmentsByType(tpl.Attachments)
			logTemplateInfo(runtime, "apply.send", map[string]interface{}{
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
			// Post-merge recipient check: Validate skipped the pre-apply check
			// when --template-id was set, so enforce it here once the template
			// addresses are folded in.
			if err := validateComposeHasAtLeastOneRecipient(to, ccFlag, bccFlag); err != nil {
				return err
			}
		}

		sigResult, err := resolveSignature(ctx, runtime, mailboxID, signatureID, senderEmail)
		if err != nil {
			return err
		}

		bld := emlbuilder.New().WithFileIO(runtime.FileIO()).
			Subject(subject).
			ToAddrs(parseNetAddrs(to))
		if senderEmail != "" {
			bld = bld.From("", senderEmail)
		}
		if err := requireSenderForRequestReceipt(runtime, senderEmail); err != nil {
			return err
		}
		if runtime.Bool("request-receipt") {
			bld = bld.DispositionNotificationTo("", senderEmail)
		}
		if ccFlag != "" {
			bld = bld.CCAddrs(parseNetAddrs(ccFlag))
		}
		if bccFlag != "" {
			bld = bld.BCCAddrs(parseNetAddrs(bccFlag))
		}
		inlineSpecs, err := parseInlineSpecs(inlineFlag)
		if err != nil {
			return err
		}
		var autoResolvedPaths []string
		var composedHTMLBody string
		var composedTextBody string
		if plainText {
			composedTextBody = body
			bld = bld.TextBody([]byte(composedTextBody))
		} else if bodyIsHTML(body) || sigResult != nil {
			// If signature is requested on plain-text body, auto-upgrade to HTML.
			htmlBody := body
			if !bodyIsHTML(body) {
				htmlBody = buildBodyDiv(body, false)
			}
			resolved, refs, resolveErr := draftpkg.ResolveLocalImagePaths(htmlBody)
			if resolveErr != nil {
				return resolveErr
			}
			resolved = injectSignatureIntoBody(resolved, sigResult)
			composedHTMLBody = resolved
			bld = bld.HTMLBody([]byte(composedHTMLBody))
			bld = addSignatureImagesToBuilder(bld, sigResult)
			var allCIDs []string
			for _, ref := range refs {
				bld = bld.AddFileInline(ref.FilePath, ref.CID)
				autoResolvedPaths = append(autoResolvedPaths, ref.FilePath)
				allCIDs = append(allCIDs, ref.CID)
			}
			for _, spec := range inlineSpecs {
				bld = bld.AddFileInline(spec.FilePath, spec.CID)
				allCIDs = append(allCIDs, spec.CID)
			}
			allCIDs = append(allCIDs, signatureCIDs(sigResult)...)
			var tplInlineCIDs []string
			bld, tplInlineCIDs, err = embedTemplateInlineAttachments(ctx, runtime, bld, resolved, mailboxID, templateID, templateInlineAttachments)
			if err != nil {
				return err
			}
			allCIDs = append(allCIDs, tplInlineCIDs...)
			if err := validateInlineCIDs(resolved, allCIDs, nil); err != nil {
				return err
			}
		} else {
			composedTextBody = body
			bld = bld.TextBody([]byte(composedTextBody))
		}
		// Embed template SMALL non-inline attachments via AddAttachment.
		// Runs after the body branch so the part list is already set; the
		// call is a no-op when the template contributes no SMALL entries.
		var templateSmallBytes int64
		bld, templateSmallBytes, err = embedTemplateSmallAttachments(ctx, runtime, bld, mailboxID, templateID, templateSmallAttachments)
		if err != nil {
			return err
		}
		bld = applyPriority(bld, priority)
		if calData := buildCalendarBody(runtime, senderEmail, to, ccFlag); calData != nil {
			bld = bld.CalendarBody(calData)
		}
		allInlinePaths := append(inlineSpecFilePaths(inlineSpecs), autoResolvedPaths...)
		composedBodySize := int64(len(composedHTMLBody) + len(composedTextBody))
		emlBase := estimateEMLBaseSize(runtime.FileIO(), composedBodySize, allInlinePaths, 0) + templateSmallBytes
		bld, err = processLargeAttachments(ctx, runtime, bld, composedHTMLBody, composedTextBody, splitByComma(attachFlag), emlBase, 0)
		if err != nil {
			return err
		}

		// Inject any template-provided LARGE attachment file_keys as an
		// extra X-Lms-Large-Attachment-Ids header so the server references
		// them when rendering the draft.
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
			return fmt.Errorf("failed to send email (draft %s created but not sent): %w", draftResult.DraftID, err)
		}
		runtime.Out(buildDraftSendOutput(resData, mailboxID), nil)
		return nil
	},
}

// splitByComma splits a comma-separated string, trimming whitespace from each entry,
// and omitting empty entries.  Used for file-path lists (--attach, --inline).
func splitByComma(s string) []string {
	if s == "" {
		return nil
	}
	var out []string
	for _, p := range strings.Split(s, ",") {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}
