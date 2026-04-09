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

var MailSend = common.Shortcut{
	Service:     "mail",
	Command:     "+send",
	Description: "Compose a new email and save as draft (default). Use --confirm-send to send immediately after user confirmation.",
	Risk:        "write",
	Scopes:      []string{"mail:user_mailbox.message:send", "mail:user_mailbox.message:modify", "mail:user_mailbox:readonly"},
	AuthTypes:   []string{"user"},
	Flags: []common.Flag{
		{Name: "to", Desc: "Recipient email address(es), comma-separated"},
		{Name: "subject", Desc: "Required. Email subject", Required: true},
		{Name: "body", Desc: "Required. Email body. Prefer HTML for rich formatting (bold, lists, links); plain text is also supported. Body type is auto-detected. Use --plain-text to force plain-text mode.", Required: true},
		{Name: "from", Desc: "Sender email address for the From header. When using an alias (send_as) address, set this to the alias and use --mailbox for the owning mailbox. Defaults to the mailbox's primary address."},
		{Name: "mailbox", Desc: "Mailbox email address that owns the draft (default: falls back to --from, then me). Use this when the sender (--from) differs from the mailbox, e.g. sending via an alias or send_as address."},
		{Name: "cc", Desc: "CC email address(es), comma-separated"},
		{Name: "bcc", Desc: "BCC email address(es), comma-separated"},
		{Name: "plain-text", Type: "bool", Desc: "Force plain-text mode, ignoring HTML auto-detection. Cannot be used with --inline."},
		{Name: "attach", Desc: "Attachment file path(s), comma-separated (relative path only)"},
		{Name: "inline", Desc: "Inline images as a JSON array. Each entry: {\"cid\":\"<unique-id>\",\"file_path\":\"<relative-path>\"}. All file_path values must be relative paths. Cannot be used with --plain-text. CID images are embedded via <img src=\"cid:...\"> in the HTML body. CID is a unique identifier, e.g. a random hex string like \"a1b2c3d4e5f6a7b8c9d0\"."},
		{Name: "confirm-send", Type: "bool", Desc: "Send the email immediately instead of saving as draft. Only use after the user has explicitly confirmed recipients and content."},
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		to := runtime.Str("to")
		subject := runtime.Str("subject")
		confirmSend := runtime.Bool("confirm-send")
		mailboxID := resolveComposeMailboxID(runtime)
		desc := "Compose email → save as draft"
		if confirmSend {
			desc = "Compose email → save as draft → send draft"
		}
		api := common.NewDryRunAPI().
			Desc(desc).
			GET(mailboxPath(mailboxID, "profile")).
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
		if err := validateComposeHasAtLeastOneRecipient(runtime.Str("to"), runtime.Str("cc"), runtime.Str("bcc")); err != nil {
			return err
		}
		return validateComposeInlineAndAttachments(runtime.FileIO(), runtime.Str("attach"), runtime.Str("inline"), runtime.Bool("plain-text"), runtime.Str("body"))
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

		senderEmail := resolveComposeSenderEmail(runtime)

		bld := emlbuilder.New().WithFileIO(runtime.FileIO()).
			Subject(subject).
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
		inlineSpecs, err := parseInlineSpecs(inlineFlag)
		if err != nil {
			return err
		}
		var autoResolvedPaths []string
		if plainText {
			bld = bld.TextBody([]byte(body))
		} else if bodyIsHTML(body) {
			resolved, refs, resolveErr := draftpkg.ResolveLocalImagePaths(body)
			if resolveErr != nil {
				return resolveErr
			}
			bld = bld.HTMLBody([]byte(resolved))
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
			if err := validateInlineCIDs(resolved, allCIDs, nil); err != nil {
				return err
			}
		} else {
			bld = bld.TextBody([]byte(body))
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

		mailboxID := resolveComposeMailboxID(runtime)
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
			return fmt.Errorf("failed to send email (draft %s created but not sent): %w", draftID, err)
		}
		runtime.Out(map[string]interface{}{
			"message_id": resData["message_id"],
			"thread_id":  resData["thread_id"],
		}, nil)
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
