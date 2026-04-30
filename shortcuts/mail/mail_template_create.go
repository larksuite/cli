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
)

var MailTemplateCreate = common.Shortcut{
	Service:     "mail",
	Command:     "+template-create",
	Description: "Create a personal mail template. Scans HTML <img src> local paths (reusing draft inline-image detection), uploads inline images and non-inline attachments to Drive, rewrites HTML to cid: references, and POSTs a Template payload to mail.user_mailbox.templates.create.",
	Risk:        "write",
	Scopes:      []string{"mail:user_mailbox.message:modify", "mail:user_mailbox:readonly"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "mailbox", Desc: "Mailbox email address that owns the template (default: me)."},
		{Name: "name", Desc: "Required. Template name (≤100 chars).", Required: true},
		{Name: "subject", Desc: "Optional. Default subject saved with the template."},
		{Name: "template-content", Desc: "Template body content. Prefer HTML. Referenced local images (<img src=\"./file.png\">) are auto-uploaded to Drive and rewritten to cid: refs."},
		{Name: "template-content-file", Desc: "Optional. Path to a file whose contents become --template-content. Relative path only. Mutually exclusive with --template-content."},
		{Name: "plain-text", Type: "bool", Desc: "Mark the template as plain-text mode (is_plain_text_mode=true). Inline images still require HTML content; use only for pure plain-text templates."},
		{Name: "to", Desc: "Optional. Default To recipient list. Separate multiple addresses with commas. Display-name format is supported."},
		{Name: "cc", Desc: "Optional. Default Cc recipient list. Separate multiple addresses with commas."},
		{Name: "bcc", Desc: "Optional. Default Bcc recipient list. Separate multiple addresses with commas."},
		{Name: "attach", Desc: "Optional. Non-inline attachment file path(s), comma-separated (relative path only). Each file is uploaded to Drive; the order follows the flag order exactly (order-sensitive for LARGE/SMALL classification)."},
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		mailboxID := resolveComposeMailboxID(runtime)
		content, _, rcErr := resolveTemplateContent(runtime)
		if rcErr != nil {
			fmt.Fprintf(runtime.IO().ErrOut, "warning: dry-run could not load template content: %v\n", rcErr)
		}
		logTemplateInfo(runtime, "create.dry_run", map[string]interface{}{
			"mailbox_id":         mailboxID,
			"is_plain_text_mode": runtime.Bool("plain-text"),
			"name_len":           len([]rune(runtime.Str("name"))),
			"attachments_total":  len(splitByComma(runtime.Str("attach"))) + len(parseLocalImgs(content)),
			"inline_count":       len(parseLocalImgs(content)),
			"tos_count":          countAddresses(runtime.Str("to")),
			"ccs_count":          countAddresses(runtime.Str("cc")),
			"bccs_count":         countAddresses(runtime.Str("bcc")),
		})
		api := common.NewDryRunAPI().
			Desc("Create a new mail template. The command scans HTML for local <img src> references, uploads each inline image to Drive (≤20MB single upload_all; >20MB upload_prepare+upload_part+upload_finish), rewrites <img src> values to cid: references, uploads any non-inline --attach files the same way, and finally POSTs a Template payload to mail.user_mailbox.templates.create.")
		// Surface the Drive upload steps explicitly so AI callers see the
		// chunked vs single-part branch point for each local image.
		for _, img := range parseLocalImgs(content) {
			addTemplateUploadSteps(runtime, api, img.Path)
		}
		for _, p := range splitByComma(runtime.Str("attach")) {
			addTemplateUploadSteps(runtime, api, p)
		}
		api = api.POST(templateMailboxPath(mailboxID)).
			Body(map[string]interface{}{
				"template": map[string]interface{}{
					"name":               runtime.Str("name"),
					"subject":            runtime.Str("subject"),
					"template_content":   "<rewritten-HTML-or-text>",
					"is_plain_text_mode": runtime.Bool("plain-text"),
					"tos":                renderTemplateAddresses(runtime.Str("to")),
					"ccs":                renderTemplateAddresses(runtime.Str("cc")),
					"bccs":               renderTemplateAddresses(runtime.Str("bcc")),
					"attachments":        "<computed from uploads>",
				},
			})
		return api
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if strings.TrimSpace(runtime.Str("name")) == "" {
			return output.ErrValidation("--name is required")
		}
		if len([]rune(runtime.Str("name"))) > 100 {
			return output.ErrValidation("--name must be at most 100 characters")
		}
		if runtime.Str("template-content") != "" && runtime.Str("template-content-file") != "" {
			return output.ErrValidation("--template-content and --template-content-file are mutually exclusive")
		}
		return nil
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		mailboxID := resolveComposeMailboxID(runtime)
		content, _, err := resolveTemplateContent(runtime)
		if err != nil {
			return err
		}
		name := runtime.Str("name")
		subject := runtime.Str("subject")
		isPlainText := runtime.Bool("plain-text")
		tos := renderTemplateAddresses(runtime.Str("to"))
		ccs := renderTemplateAddresses(runtime.Str("cc"))
		bccs := renderTemplateAddresses(runtime.Str("bcc"))

		content = wrapTemplateContentIfNeeded(content, isPlainText)
		if int64(len(content)) > maxTemplateContentBytes {
			return output.ErrValidation("template content exceeds %d MB (got %.1f MB)",
				maxTemplateContentBytes/(1024*1024),
				float64(len(content))/1024/1024)
		}

		rewritten, atts, err := buildTemplatePayloadFromFlags(
			ctx, runtime, name, subject, content, tos, ccs, bccs,
			splitByComma(runtime.Str("attach")),
		)
		if err != nil {
			return err
		}
		inlineCount, largeCount := countAttachmentsByType(atts)
		logTemplateInfo(runtime, "create.execute", map[string]interface{}{
			"mailbox_id":         mailboxID,
			"is_plain_text_mode": isPlainText,
			"name_len":           len([]rune(name)),
			"attachments_total":  len(atts),
			"inline_count":       inlineCount,
			"large_count":        largeCount,
			"tos_count":          len(tos),
			"ccs_count":          len(ccs),
			"bccs_count":         len(bccs),
		})

		payload := &templatePayload{
			Name:            name,
			Subject:         subject,
			TemplateContent: rewritten,
			IsPlainTextMode: isPlainText,
			Tos:             tos,
			Ccs:             ccs,
			Bccs:            bccs,
			Attachments:     atts,
		}

		resp, err := createTemplate(runtime, mailboxID, payload)
		if err != nil {
			return fmt.Errorf("create template failed: %w", err)
		}
		tpl, _ := extractTemplatePayload(resp)
		out := map[string]interface{}{
			"template": tpl,
		}
		runtime.OutFormat(out, nil, func(w io.Writer) {
			fmt.Fprintln(w, "Template created.")
			if tpl != nil {
				fmt.Fprintf(w, "template_id: %s\n", tpl.TemplateID)
				fmt.Fprintf(w, "name: %s\n", tpl.Name)
				fmt.Fprintf(w, "attachments: %d\n", len(tpl.Attachments))
			}
		})
		return nil
	},
}

// resolveTemplateContent returns the final template_content string, loading
// --template-content-file when set. The second return value is the unmodified
// source path (if any) to assist DryRun logging.
func resolveTemplateContent(runtime *common.RuntimeContext) (content, sourcePath string, err error) {
	if raw := runtime.Str("template-content"); raw != "" {
		return raw, "", nil
	}
	path := runtime.Str("template-content-file")
	if path == "" {
		return "", "", nil
	}
	f, err := runtime.FileIO().Open(path)
	if err != nil {
		return "", path, output.ErrValidation("open --template-content-file %s: %v", path, err)
	}
	defer f.Close()
	buf, err := io.ReadAll(f)
	if err != nil {
		return "", path, output.ErrValidation("read --template-content-file %s: %v", path, err)
	}
	return string(buf), path, nil
}

// addTemplateUploadSteps enumerates the Drive steps needed to upload one
// local file, based on its on-disk size. Used by DryRun output.
func addTemplateUploadSteps(runtime *common.RuntimeContext, api *common.DryRunAPI, path string) {
	if strings.TrimSpace(path) == "" {
		return
	}
	info, err := runtime.FileIO().Stat(path)
	if err != nil {
		api.POST("/open-apis/drive/v1/medias/upload_all").Desc("Upload: " + path + " (size unknown: " + err.Error() + ")")
		return
	}
	if info.Size() <= common.MaxDriveMediaUploadSinglePartSize {
		api.POST("/open-apis/drive/v1/medias/upload_all").Desc("Upload " + path)
		return
	}
	api.POST("/open-apis/drive/v1/medias/upload_prepare").Desc("Large file prepare: " + path)
	api.POST("/open-apis/drive/v1/medias/upload_part").Desc("Large file parts")
	api.POST("/open-apis/drive/v1/medias/upload_finish").Desc("Large file finish")
}
