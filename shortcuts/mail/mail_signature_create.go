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
	"github.com/larksuite/cli/shortcuts/mail/signature"
)

var MailSignatureCreate = common.Shortcut{
	Service:     "mail",
	Command:     "+signature-create",
	Description: "Create a personal USER mail signature. HTML content is preserved; plain text is wrapped for email rendering. Local image paths are not uploaded.",
	Risk:        "write",
	Scopes:      []string{"mail:user_mailbox.message:modify", "mail:user_mailbox:readonly"},
	AuthTypes:   []string{"user"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "mailbox", Desc: "Mailbox address that owns the signature (default: me)."},
		{Name: "name", Desc: "Required. Signature name (≤100 chars).", Required: true},
		{Name: "content", Desc: "Signature content. HTML is preserved; plain text is wrapped for email rendering."},
		{Name: "content-file", Desc: "Relative file path for signature content. Mutually exclusive with --content."},
		{Name: "device", Default: "pc", Desc: "Signature device: pc or mobile."},
		{Name: "images-json", Desc: "JSON array of signature image metadata. cid values must match <img src=\"cid:...\"> references."},
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		mailboxID := resolveComposeMailboxID(runtime)
		content, _, err := resolveSignatureContent(runtime, "content", "content-file")
		contentSource := "inline"
		if runtime.Str("content-file") != "" {
			contentSource = runtime.Str("content-file")
		}
		api := common.NewDryRunAPI().
			Desc("Create a personal USER mail signature. The command validates content/images locally, then POSTs a signature wrapper.").
			POST(mailboxPath(mailboxID, "settings", "signatures")).
			Body(map[string]interface{}{
				"signature": map[string]interface{}{
					"name":             strings.TrimSpace(runtime.Str("name")),
					"content_source":   contentSource,
					"content_preview":  contentPreview(content, 120, resolveLang(runtime)),
					"signature_device": strings.ToUpper(runtime.Str("device")),
					"images":           "<parsed from --images-json>",
					"signature_type":   "USER",
				},
			})
		if err != nil {
			api.Set("validation_warning", err.Error())
		}
		return api
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if strings.TrimSpace(runtime.Str("name")) == "" {
			return output.ErrValidation("--name is required")
		}
		if len([]rune(strings.TrimSpace(runtime.Str("name")))) > 100 {
			return output.ErrValidation("--name must be at most 100 characters")
		}
		if runtime.Changed("content") && runtime.Str("content-file") != "" {
			return output.ErrValidation("--content and --content-file are mutually exclusive")
		}
		if _, err := normalizeSignatureDevice(runtime.Str("device"), "--device"); err != nil {
			return err
		}
		content, _, err := resolveSignatureContent(runtime, "content", "content-file")
		if err != nil {
			return err
		}
		images, _, err := parseSignatureImagesJSON(runtime.Str("images-json"), "images-json")
		if err != nil {
			return err
		}
		return validateSignatureImageRefs(content, images)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		mailboxID := resolveComposeMailboxID(runtime)
		device, err := normalizeSignatureDevice(runtime.Str("device"), "--device")
		if err != nil {
			return err
		}
		content, _, err := resolveSignatureContent(runtime, "content", "content-file")
		if err != nil {
			return err
		}
		images, _, err := parseSignatureImagesJSON(runtime.Str("images-json"), "images-json")
		if err != nil {
			return err
		}
		if err := validateSignatureImageRefs(content, images); err != nil {
			return err
		}

		created, err := signature.Create(runtime, mailboxID, signature.Signature{
			Name:            strings.TrimSpace(runtime.Str("name")),
			SignatureType:   signature.SignatureTypeUser,
			SignatureDevice: device,
			Content:         content,
			Images:          sanitizeSignatureImages(images),
		})
		if err != nil {
			return err
		}
		out := signatureOutput(created, resolveLang(runtime))
		delete(out, "changed_fields")
		delete(out, "last_write_policy")
		runtime.OutFormat(out, nil, func(w io.Writer) {
			formatSignatureSummary(w, "created", created, resolveLang(runtime))
			fmt.Fprintln(w, "Use this signature_id with mail +send / +reply / +forward --signature-id.")
		})
		return nil
	},
}
