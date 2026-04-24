// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
	draftpkg "github.com/larksuite/cli/shortcuts/mail/draft"
	"github.com/larksuite/cli/shortcuts/mail/emlbuilder"
)

// draftCreateInput bundles all +draft-create user flags into a single
// struct so parseDraftCreateInput / buildRawEMLForDraftCreate have a
// uniform value type to pass around.
type draftCreateInput struct {
	To        string
	Subject   string
	Body      string
	From      string
	CC        string
	BCC       string
	Attach    string
	Inline    string
	PlainText bool
}

// MailDraftCreate is the `+draft-create` shortcut: create a brand-new mail
// draft from scratch. For reply drafts use +reply; for forward drafts use
// +forward.
var MailDraftCreate = common.Shortcut{
	Service:     "mail",
	Command:     "+draft-create",
	Description: "Create a brand-new mail draft from scratch (NOT for reply or forward). For reply drafts use +reply; for forward drafts use +forward. Only use +draft-create when composing a new email with no parent message.",
	Risk:        "write",
	Scopes:      []string{"mail:user_mailbox.message:modify", "mail:user_mailbox:readonly"},
	AuthTypes:   []string{"user"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "to", Desc: "Optional. Full To recipient list. Separate multiple addresses with commas. Display-name format is supported. When omitted, the draft is created without recipients (they can be added later via +draft-edit)."},
		{Name: "subject", Desc: "Final draft subject. Pass the full subject you want to appear in the draft. Required unless --template-id supplies a non-empty subject."},
		{Name: "body", Desc: "Full email body. Prefer HTML for rich formatting (bold, lists, links); plain text is also supported. Body type is auto-detected. Use --plain-text to force plain-text mode. Required unless --template-id supplies a non-empty body."},
		{Name: "from", Desc: "Optional. Sender email address for the From header. When using an alias (send_as) address, set this to the alias and use --mailbox for the owning mailbox. If omitted, the mailbox's primary address is used."},
		{Name: "mailbox", Desc: "Optional. Mailbox email address that owns the draft (default: falls back to --from, then me). Use this when the sender (--from) differs from the mailbox, e.g. sending via an alias or send_as address."},
		{Name: "cc", Desc: "Optional. Full Cc recipient list. Separate multiple addresses with commas. Display-name format is supported."},
		{Name: "bcc", Desc: "Optional. Full Bcc recipient list. Separate multiple addresses with commas. Display-name format is supported."},
		{Name: "plain-text", Type: "bool", Desc: "Force plain-text mode, ignoring HTML auto-detection. Cannot be used with --inline."},
		{Name: "attach", Desc: "Optional. Regular attachment file paths (relative path only). Separate multiple paths with commas. Each path must point to a readable local file."},
		{Name: "inline", Desc: "Optional. Inline images as a JSON array. Each entry: {\"cid\":\"<unique-id>\",\"file_path\":\"<relative-path>\"}. All file_path values must be relative paths. Cannot be used with --plain-text. CID images are embedded via <img src=\"cid:...\"> in the HTML body. CID is a unique identifier, e.g. a random hex string like \"a1b2c3d4e5f6a7b8c9d0\"."},
		{Name: "request-receipt", Type: "bool", Desc: "Request a read receipt (Message Disposition Notification, RFC 3798) addressed to the sender. Recipient mail clients may prompt the user, send automatically, or silently ignore — delivery of a receipt is not guaranteed."},
		{Name: "template-id", Desc: "Optional. Apply a saved template by ID (decimal integer string) before composing. The template's subject/body/to/cc/bcc/attachments are merged with user-supplied flags (user flags win). Requires --as user."},
		signatureFlag,
		priorityFlag,
		eventSummaryFlag, eventStartFlag, eventEndFlag, eventLocationFlag,
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		mailboxID := resolveComposeMailboxID(runtime)
		api := common.NewDryRunAPI().
			Desc("Create a new empty draft without sending it. The command resolves the sender address (from --from, --mailbox, or mailbox profile), builds a complete EML from `to/subject/body` plus any optional cc/bcc/attachment/inline inputs, and finally calls drafts.create. `--body` content type is auto-detected (HTML or plain text); use `--plain-text` to force plain-text mode. For inline images, CIDs can be any unique strings, e.g. random hex. Use the dedicated reply or forward shortcuts for reply-style drafts instead of adding reply-thread headers here.")
		if tid := runtime.Str("template-id"); tid != "" {
			api = api.GET(templateMailboxPath(mailboxID, tid)).
				Desc("Fetch template to merge with compose flags (subject/body/to/cc/bcc/attachments).")
		}
		api = api.GET(mailboxPath(mailboxID, "profile")).
			POST(mailboxPath(mailboxID, "drafts")).
			Body(map[string]interface{}{
				"raw": "<base64url-EML>",
				"_preview": map[string]interface{}{
					"to":      runtime.Str("to"),
					"subject": runtime.Str("subject"),
				},
			})
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
		if err := validateSignatureWithPlainText(runtime.Bool("plain-text"), runtime.Str("signature-id")); err != nil {
			return err
		}
		if err := validateEventFlags(runtime); err != nil {
			return err
		}
		if err := validateComposeInlineAndAttachments(runtime.FileIO(), runtime.Str("attach"), runtime.Str("inline"), runtime.Bool("plain-text"), runtime.Str("body")); err != nil {
			return err
		}
		return validatePriorityFlag(runtime)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		priority, err := parsePriority(runtime.Str("priority"))
		if err != nil {
			return err
		}
		mailboxID := resolveComposeMailboxID(runtime)
		input := draftCreateInput{
			To:        runtime.Str("to"),
			Subject:   runtime.Str("subject"),
			Body:      runtime.Str("body"),
			From:      runtime.Str("from"),
			CC:        runtime.Str("cc"),
			BCC:       runtime.Str("bcc"),
			Attach:    runtime.Str("attach"),
			Inline:    runtime.Str("inline"),
			PlainText: runtime.Bool("plain-text"),
		}
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
				templateShortcutDraftCreate, tpl,
				"", "", "", "", "",
				input.To, input.CC, input.BCC, input.Subject, input.Body,
			)
			input.To = merged.To
			input.CC = merged.Cc
			input.BCC = merged.Bcc
			input.Subject = merged.Subject
			input.Body = merged.Body
			if !input.PlainText && merged.IsPlainTextMode {
				input.PlainText = true
			}
			templateLargeAttachmentIDs = merged.LargeAttachmentIDs
			templateInlineAttachments = merged.InlineAttachments
			templateSmallAttachments = merged.SmallAttachments
			for _, w := range merged.Warnings {
				fmt.Fprintf(runtime.IO().ErrOut, "warning: %s\n", w)
			}
			inlineCount, largeCount := countAttachmentsByType(tpl.Attachments)
			logTemplateInfo(runtime, "apply.draft_create", map[string]interface{}{
				"mailbox_id":         mailboxID,
				"template_id":        tid,
				"is_plain_text_mode": input.PlainText,
				"attachments_total":  len(tpl.Attachments),
				"inline_count":       inlineCount,
				"large_count":        largeCount,
				"tos_count":          countAddresses(input.To),
				"ccs_count":          countAddresses(input.CC),
				"bccs_count":         countAddresses(input.BCC),
			})
		}
		if strings.TrimSpace(input.Subject) == "" {
			return output.ErrValidation("effective subject is empty after applying template; pass --subject explicitly")
		}
		if strings.TrimSpace(input.Body) == "" {
			return output.ErrValidation("effective body is empty after applying template; pass --body explicitly")
		}
		sigResult, err := resolveSignature(ctx, runtime, mailboxID, runtime.Str("signature-id"), runtime.Str("from"))
		if err != nil {
			return err
		}
		rawEML, err := buildRawEMLForDraftCreate(ctx, runtime, input, sigResult, priority,
			templateLargeAttachmentIDs, mailboxID, templateID, templateInlineAttachments, templateSmallAttachments)
		if err != nil {
			return err
		}
		draftResult, err := draftpkg.CreateWithRaw(runtime, mailboxID, rawEML)
		if err != nil {
			return fmt.Errorf("create draft failed: %w", err)
		}
		out := map[string]interface{}{"draft_id": draftResult.DraftID}
		if draftResult.Reference != "" {
			out["reference"] = draftResult.Reference
		}
		runtime.OutFormat(out, nil, func(w io.Writer) {
			fmt.Fprintln(w, "Draft created.")
			// Intentionally keep +draft-create output minimal: unlike reply/forward/send
			// draft-save flows, it does not add a follow-up send tip.
			fmt.Fprintf(w, "draft_id: %s\n", draftResult.DraftID)
			if reference, _ := out["reference"].(string); reference != "" {
				fmt.Fprintf(w, "reference: %s\n", reference)
			}
		})
		return nil
	},
}

// buildRawEMLForDraftCreate assembles a base64url-encoded EML for the
// +draft-create shortcut. It resolves the sender from runtime / input,
// validates recipient counts, applies signature templates, resolves local
// image paths to CID-referenced inline parts, enforces attachment limits,
// applies priority headers, and optionally adds the Disposition-Notification-
// To header when --request-receipt is set. senderEmail is required; empty
// senderEmail returns an error early. The returned string is ready to POST
// to the drafts endpoint. ctx is plumbed through for large-attachment
// processing.
func buildRawEMLForDraftCreate(
	ctx context.Context,
	runtime *common.RuntimeContext,
	input draftCreateInput,
	sigResult *signatureResult,
	priority string,
	templateLargeAttachmentIDs []string,
	mailboxID, templateID string,
	templateInlineAttachments []templateInlineRef,
	templateSmallAttachments []templateAttachmentRef,
) (string, error) {
	senderEmail := resolveComposeSenderEmail(runtime)
	if senderEmail == "" {
		return "", fmt.Errorf("unable to determine sender email; please specify --from explicitly")
	}

	if err := validateRecipientCount(input.To, input.CC, input.BCC); err != nil {
		return "", err
	}

	bld := emlbuilder.New().WithFileIO(runtime.FileIO()).
		AllowNoRecipients().
		Subject(input.Subject)
	if strings.TrimSpace(input.To) != "" {
		bld = bld.ToAddrs(parseNetAddrs(input.To))
	}
	if senderEmail != "" {
		bld = bld.From("", senderEmail)
	}
	// senderEmail non-emptiness is already enforced above (L140); the flag-
	// driven guard here only exists to make the relationship explicit to
	// readers. requireSenderForRequestReceipt unifies this with the other
	// compose shortcuts; if it ever trips in this path, the above check
	// regressed.
	if err := requireSenderForRequestReceipt(runtime, senderEmail); err != nil {
		return "", err
	}
	if runtime.Bool("request-receipt") {
		bld = bld.DispositionNotificationTo("", senderEmail)
	}
	if input.CC != "" {
		bld = bld.CCAddrs(parseNetAddrs(input.CC))
	}
	if input.BCC != "" {
		bld = bld.BCCAddrs(parseNetAddrs(input.BCC))
	}
	inlineSpecs, err := parseInlineSpecs(input.Inline)
	if err != nil {
		return "", output.ErrValidation("%v", err)
	}
	var autoResolvedPaths []string
	var composedHTMLBody string
	var composedTextBody string
	if input.PlainText {
		composedTextBody = input.Body
		bld = bld.TextBody([]byte(composedTextBody))
	} else if bodyIsHTML(input.Body) || sigResult != nil {
		htmlBody := input.Body
		if !bodyIsHTML(input.Body) {
			htmlBody = buildBodyDiv(input.Body, false)
		}
		resolved, refs, resolveErr := draftpkg.ResolveLocalImagePaths(htmlBody)
		if resolveErr != nil {
			return "", resolveErr
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
			return "", err
		}
		allCIDs = append(allCIDs, tplInlineCIDs...)
		if err := validateInlineCIDs(resolved, allCIDs, nil); err != nil {
			return "", err
		}
	} else {
		composedTextBody = input.Body
		bld = bld.TextBody([]byte(composedTextBody))
	}
	// Embed template SMALL non-inline attachments via AddAttachment. No-op
	// when the template contributes none; runs in both HTML and plain-text
	// branches because regular attachments are independent of body mode.
	var templateSmallBytes int64
	bld, templateSmallBytes, err = embedTemplateSmallAttachments(ctx, runtime, bld, mailboxID, templateID, templateSmallAttachments)
	if err != nil {
		return "", err
	}
	bld = applyPriority(bld, priority)
	if calData := buildCalendarBody(runtime, senderEmail, input.To, input.CC); calData != nil {
		bld = bld.CalendarBody(calData)
	}
	allInlinePaths := append(inlineSpecFilePaths(inlineSpecs), autoResolvedPaths...)
	composedBodySize := int64(len(composedHTMLBody) + len(composedTextBody))
	emlBase := estimateEMLBaseSize(runtime.FileIO(), composedBodySize, allInlinePaths, 0) + templateSmallBytes
	bld, err = processLargeAttachments(ctx, runtime, bld, composedHTMLBody, composedTextBody, splitByComma(input.Attach), emlBase, 0)
	if err != nil {
		return "", err
	}
	if hdr, hdrErr := encodeTemplateLargeAttachmentHeader(templateLargeAttachmentIDs); hdrErr == nil && hdr != "" {
		bld = bld.Header(draftpkg.LargeAttachmentIDsHeader, hdr)
	}
	rawEML, err := bld.BuildBase64URL()
	if err != nil {
		return "", output.ErrValidation("build EML failed: %v", err)
	}
	return rawEML, nil
}
