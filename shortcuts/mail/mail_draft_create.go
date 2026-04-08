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
		{Name: "from", Desc: "Optional. Sender email address; also selects the mailbox to create the draft in. If omitted, the current signed-in user's primary mailbox address is used."},
		{Name: "cc", Desc: "Optional. Full Cc recipient list. Separate multiple addresses with commas. Display-name format is supported."},
		{Name: "bcc", Desc: "Optional. Full Bcc recipient list. Separate multiple addresses with commas. Display-name format is supported."},
		{Name: "plain-text", Type: "bool", Desc: "Force plain-text mode, ignoring HTML auto-detection. Cannot be used with --inline."},
		{Name: "attach", Desc: "Optional. Regular attachment file paths (relative path only). Separate multiple paths with commas. Each path must point to a readable local file."},
		{Name: "inline", Desc: "Optional. Inline images as a JSON array. Each entry: {\"cid\":\"<unique-id>\",\"file_path\":\"<relative-path>\"}. All file_path values must be relative paths. Cannot be used with --plain-text. CID images are embedded via <img src=\"cid:...\"> in the HTML body. CID is a unique identifier, e.g. a random hex string like \"a1b2c3d4e5f6a7b8c9d0\"."},
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		input, err := parseDraftCreateInput(runtime)
		if err != nil {
			return common.NewDryRunAPI().Set("error", err.Error())
		}
		mailboxID := resolveComposeMailboxID(runtime)
		return common.NewDryRunAPI().
			Desc("Create a new empty draft without sending it. The command first reads the current mailbox profile to determine the default sender when `--from` is omitted, then builds a complete EML from `to/subject/body` plus any optional cc/bcc/attachment/inline inputs, and finally calls drafts.create. `--body` content type is auto-detected (HTML or plain text); use `--plain-text` to force plain-text mode. For inline images, CIDs can be any unique strings, e.g. random hex. Use the dedicated reply or forward shortcuts for reply-style drafts instead of adding reply-thread headers here.").
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
		if err := validateComposeInlineAndAttachments(runtime.FileIO(), runtime.Str("attach"), runtime.Str("inline"), runtime.Bool("plain-text"), runtime.Str("body")); err != nil {
			return err
		}
		return nil
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		input, err := parseDraftCreateInput(runtime)
		if err != nil {
			return err
		}
		rawEML, err := buildRawEMLForDraftCreate(runtime, input)
		if err != nil {
			return err
		}
		mailboxID := resolveComposeMailboxID(runtime)
		draftID, err := draftpkg.CreateWithRaw(runtime, mailboxID, rawEML)
		if err != nil {
			return fmt.Errorf("create draft failed: %w", err)
		}
		out := map[string]interface{}{"draft_id": draftID}
		runtime.OutFormat(out, nil, func(w io.Writer) {
			fmt.Fprintln(w, "Draft created.")
			fmt.Fprintf(w, "draft_id: %s\n", draftID)
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

func buildRawEMLForDraftCreate(runtime *common.RuntimeContext, input draftCreateInput) (string, error) {
	senderEmail := input.From
	if senderEmail == "" {
		senderEmail = fetchCurrentUserEmail(runtime)
		if senderEmail == "" {
			return "", fmt.Errorf("unable to determine sender email; please specify --from explicitly")
		}
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
	if input.PlainText {
		bld = bld.TextBody([]byte(input.Body))
	} else if bodyIsHTML(input.Body) {
		bld = bld.HTMLBody([]byte(input.Body))
	} else {
		bld = bld.TextBody([]byte(input.Body))
	}
	inlineSpecs, err := parseInlineSpecs(input.Inline)
	if err != nil {
		return "", output.ErrValidation("%v", err)
	}
	for _, path := range splitByComma(input.Attach) {
		bld = bld.AddFileAttachment(path)
	}
	for _, spec := range inlineSpecs {
		bld = bld.AddFileInline(spec.FilePath, spec.CID)
	}
	rawEML, err := bld.BuildBase64URL()
	if err != nil {
		return "", output.ErrValidation("build EML failed: %v", err)
	}
	return rawEML, nil
}
