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
		{Name: "subject", Desc: "Required. Final draft subject. Pass the full subject you want to appear in the draft.", Required: true},
		{Name: "body", Desc: "Required. Full email body. Prefer HTML for rich formatting (bold, lists, links); plain text is also supported. Body type is auto-detected. Use --plain-text to force plain-text mode.", Required: true},
		{Name: "from", Desc: "Optional. Sender email address for the From header. When using an alias (send_as) address, set this to the alias and use --mailbox for the owning mailbox. If omitted, the mailbox's primary address is used."},
		{Name: "mailbox", Desc: "Optional. Mailbox email address that owns the draft (default: falls back to --from, then me). Use this when the sender (--from) differs from the mailbox, e.g. sending via an alias or send_as address."},
		{Name: "cc", Desc: "Optional. Full Cc recipient list. Separate multiple addresses with commas. Display-name format is supported."},
		{Name: "bcc", Desc: "Optional. Full Bcc recipient list. Separate multiple addresses with commas. Display-name format is supported."},
		{Name: "plain-text", Type: "bool", Desc: "Force plain-text mode, ignoring HTML auto-detection. Cannot be used with --inline."},
		{Name: "attach", Desc: "Optional. Regular attachment file paths (relative path only). Separate multiple paths with commas. Each path must point to a readable local file."},
		{Name: "inline", Desc: "Optional. Inline images as a JSON array. Each entry: {\"cid\":\"<unique-id>\",\"file_path\":\"<relative-path>\"}. All file_path values must be relative paths. Cannot be used with --plain-text. CID images are embedded via <img src=\"cid:...\"> in the HTML body. CID is a unique identifier, e.g. a random hex string like \"a1b2c3d4e5f6a7b8c9d0\"."},
		signatureFlag,
		priorityFlag,
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		input, err := parseDraftCreateInput(runtime)
		if err != nil {
			return common.NewDryRunAPI().Set("error", err.Error())
		}
		mailboxID := resolveComposeMailboxID(runtime)
		return common.NewDryRunAPI().
			Desc("Create a new empty draft without sending it. The command resolves the sender address (from --from, --mailbox, or mailbox profile), builds a complete EML from `to/subject/body` plus any optional cc/bcc/attachment/inline inputs, and finally calls drafts.create. `--body` content type is auto-detected (HTML or plain text); use `--plain-text` to force plain-text mode. For inline images, CIDs can be any unique strings, e.g. random hex. Use the dedicated reply or forward shortcuts for reply-style drafts instead of adding reply-thread headers here.").
			GET(mailboxPath(mailboxID, "profile")).
			POST(mailboxPath(mailboxID, "drafts")).
			Body(map[string]interface{}{
				"raw": "<base64url-EML>",
				"_preview": map[string]interface{}{
					"to":      input.To,
					"subject": input.Subject,
				},
			})
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if strings.TrimSpace(runtime.Str("subject")) == "" {
			return output.ErrValidation("--subject is required; pass the final email subject")
		}
		if strings.TrimSpace(runtime.Str("body")) == "" {
			return output.ErrValidation("--body is required; pass the full email body")
		}
		if err := validateSignatureWithPlainText(runtime.Bool("plain-text"), runtime.Str("signature-id")); err != nil {
			return err
		}
		if err := validateComposeInlineAndAttachments(runtime.FileIO(), runtime.Str("attach"), runtime.Str("inline"), runtime.Bool("plain-text"), runtime.Str("body")); err != nil {
			return err
		}
		return validatePriorityFlag(runtime)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		input, err := parseDraftCreateInput(runtime)
		if err != nil {
			return err
		}
		priority, err := parsePriority(runtime.Str("priority"))
		if err != nil {
			return err
		}
		mailboxID := resolveComposeMailboxID(runtime)
		sigResult, err := resolveSignature(ctx, runtime, mailboxID, runtime.Str("signature-id"), runtime.Str("from"))
		if err != nil {
			return err
		}
		rawEML, err := buildRawEMLForDraftCreate(ctx, runtime, input, sigResult, priority)
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

func parseDraftCreateInput(runtime *common.RuntimeContext) (draftCreateInput, error) {
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
	if strings.TrimSpace(input.Subject) == "" {
		return input, output.ErrValidation("--subject is required; pass the final email subject")
	}
	if strings.TrimSpace(input.Body) == "" {
		return input, output.ErrValidation("--body is required; pass the full email body")
	}
	return input, nil
}

func buildRawEMLForDraftCreate(ctx context.Context, runtime *common.RuntimeContext, input draftCreateInput, sigResult *signatureResult, priority string) (string, error) {
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
		if err := validateInlineCIDs(resolved, allCIDs, nil); err != nil {
			return "", err
		}
	} else {
		composedTextBody = input.Body
		bld = bld.TextBody([]byte(composedTextBody))
	}
	bld = applyPriority(bld, priority)
	allInlinePaths := append(inlineSpecFilePaths(inlineSpecs), autoResolvedPaths...)
	composedBodySize := int64(len(composedHTMLBody) + len(composedTextBody))
	emlBase := estimateEMLBaseSize(runtime.FileIO(), composedBodySize, allInlinePaths, 0)
	bld, err = processLargeAttachments(ctx, runtime, bld, composedHTMLBody, composedTextBody, splitByComma(input.Attach), emlBase, 0)
	if err != nil {
		return "", err
	}
	rawEML, err := bld.BuildBase64URL()
	if err != nil {
		return "", output.ErrValidation("build EML failed: %v", err)
	}
	return rawEML, nil
}
