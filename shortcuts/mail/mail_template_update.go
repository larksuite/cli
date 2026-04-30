// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
)

var MailTemplateUpdate = common.Shortcut{
	Service:     "mail",
	Command:     "+template-update",
	Description: "Update an existing mail template. Supports --inspect (read-only projection), --print-patch-template (prints a JSON skeleton for --patch-file), and flat flags (--set-subject / --set-name / etc). Internally it GETs the template, applies the patch, rewrites <img> local paths to cid: refs, and PUTs a full-replace update (no optimistic locking: last-write-wins).",
	Risk:        "write",
	Scopes:      []string{"mail:user_mailbox.message:modify", "mail:user_mailbox:readonly"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "mailbox", Desc: "Mailbox email address that owns the template (default: me)."},
		{Name: "template-id", Desc: "Template ID to update. Required except when using --print-patch-template by itself.", Required: false},
		{Name: "inspect", Type: "bool", Desc: "Inspect the template without modifying it. Returns the current template projection (name/subject/addresses/attachments). No write is performed."},
		{Name: "print-patch-template", Type: "bool", Desc: "Print a JSON template describing the supported --patch-file structure. No network call is made."},
		{Name: "patch-file", Desc: "Path to a JSON patch file (relative path only). Shape is the same as --print-patch-template output."},
		{Name: "set-name", Desc: "Replace the template name (≤100 chars)."},
		{Name: "set-subject", Desc: "Replace the template subject."},
		{Name: "set-template-content", Desc: "Replace the template body content. Prefer HTML for rich formatting."},
		{Name: "set-template-content-file", Desc: "Replace template body content with the contents of a file (relative path only). Mutually exclusive with --set-template-content."},
		{Name: "set-plain-text", Type: "bool", Desc: "Set is_plain_text_mode=true."},
		{Name: "set-to", Desc: "Replace the To recipient list. Separate multiple addresses with commas. Pass --set-to=\"\" to clear the list."},
		{Name: "set-cc", Desc: "Replace the Cc recipient list. Pass --set-cc=\"\" to clear the list."},
		{Name: "set-bcc", Desc: "Replace the Bcc recipient list. Pass --set-bcc=\"\" to clear the list."},
		{Name: "attach", Desc: "Additional non-inline attachment file path(s), comma-separated. Each file is uploaded to Drive and appended to the template's attachments[] in the exact flag order."},
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		if runtime.Bool("print-patch-template") {
			return common.NewDryRunAPI().
				Set("mode", "print-patch-template").
				Set("template", buildTemplatePatchSkeleton())
		}
		mailboxID := resolveComposeMailboxID(runtime)
		tid := runtime.Str("template-id")
		if tid == "" {
			return common.NewDryRunAPI().Set("error", "--template-id is required except with --print-patch-template")
		}
		if runtime.Bool("inspect") {
			return common.NewDryRunAPI().
				Desc("Inspect the template without modifying it.").
				GET(templateMailboxPath(mailboxID, tid))
		}
		api := common.NewDryRunAPI().
			Desc("Update an existing mail template: GET the template, apply --set-* / --patch-file / --attach changes, upload any new local <img> references and --attach files to Drive, rewrite HTML to cid: references, and PUT a full-replace payload. The template endpoints have no optimistic locking; concurrent updates are last-write-wins.").
			GET(templateMailboxPath(mailboxID, tid))
		content, _, _ := resolveTemplateUpdateContent(runtime)
		for _, img := range parseLocalImgs(content) {
			addTemplateUploadSteps(runtime, api, img.Path)
		}
		for _, p := range splitByComma(runtime.Str("attach")) {
			addTemplateUploadSteps(runtime, api, p)
		}
		api = api.PUT(templateMailboxPath(mailboxID, tid)).
			Body(map[string]interface{}{
				"template": "<merged from GET + patch flags>",
				"_warning": "No optimistic locking — last write wins.",
			})
		logTemplateInfo(runtime, "update.dry_run", map[string]interface{}{
			"mailbox_id":         mailboxID,
			"template_id":        tid,
			"is_plain_text_mode": runtime.Bool("set-plain-text"),
			"name_len":           len([]rune(runtime.Str("set-name"))),
			"attachments_total":  len(splitByComma(runtime.Str("attach"))) + len(parseLocalImgs(content)),
			"inline_count":       len(parseLocalImgs(content)),
			"tos_count":          countAddresses(runtime.Str("set-to")),
			"ccs_count":          countAddresses(runtime.Str("set-cc")),
			"bccs_count":         countAddresses(runtime.Str("set-bcc")),
		})
		return api
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if runtime.Bool("print-patch-template") {
			return nil
		}
		if err := validateTemplateID(runtime.Str("template-id")); err != nil {
			return err
		}
		if runtime.Str("template-id") == "" {
			return output.ErrValidation("--template-id is required (or use --print-patch-template to print the patch skeleton)")
		}
		if runtime.Str("set-template-content") != "" && runtime.Str("set-template-content-file") != "" {
			return output.ErrValidation("--set-template-content and --set-template-content-file are mutually exclusive")
		}
		if name := runtime.Str("set-name"); name != "" && len([]rune(name)) > 100 {
			return output.ErrValidation("--set-name must be at most 100 characters")
		}
		return nil
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if runtime.Bool("print-patch-template") {
			runtime.Out(buildTemplatePatchSkeleton(), nil)
			return nil
		}
		mailboxID := resolveComposeMailboxID(runtime)
		tid := runtime.Str("template-id")

		tpl, err := fetchTemplate(runtime, mailboxID, tid)
		if err != nil {
			return err
		}

		if runtime.Bool("inspect") {
			out := map[string]interface{}{"template": tpl}
			runtime.OutFormat(out, nil, func(w io.Writer) {
				fmt.Fprintln(w, "Template inspection (read-only).")
				if tpl != nil {
					fmt.Fprintf(w, "template_id: %s\n", tpl.TemplateID)
					fmt.Fprintf(w, "name: %s\n", tpl.Name)
					if tpl.Subject != "" {
						fmt.Fprintf(w, "subject: %s\n", tpl.Subject)
					}
					fmt.Fprintf(w, "is_plain_text_mode: %v\n", tpl.IsPlainTextMode)
					fmt.Fprintf(w, "attachments: %d\n", len(tpl.Attachments))
				}
			})
			return nil
		}

		// Apply flat --set-* flags.
		if v := runtime.Str("set-name"); v != "" {
			tpl.Name = v
		}
		if v := runtime.Str("set-subject"); v != "" {
			tpl.Subject = v
		}
		newContent, _, err := resolveTemplateUpdateContent(runtime)
		if err != nil {
			return err
		}
		contentChanged := false
		if newContent != "" {
			tpl.TemplateContent = newContent
			contentChanged = true
		}
		if runtime.Bool("set-plain-text") {
			tpl.IsPlainTextMode = true
		}
		// Use Changed() so an explicit empty value (--set-to="") clears the
		// list. The non-empty-only check would treat clear and "not provided"
		// the same and silently drop the clear.
		if runtime.Changed("set-to") {
			tpl.Tos = renderTemplateAddresses(runtime.Str("set-to"))
		}
		if runtime.Changed("set-cc") {
			tpl.Ccs = renderTemplateAddresses(runtime.Str("set-cc"))
		}
		if runtime.Changed("set-bcc") {
			tpl.Bccs = renderTemplateAddresses(runtime.Str("set-bcc"))
		}

		// Apply JSON patch file (simple shallow merge). This is a convenience
		// for agents that want to assemble updates off-line; the CLI simply
		// overlays non-empty values onto the fetched template.
		if pf := strings.TrimSpace(runtime.Str("patch-file")); pf != "" {
			f, err := runtime.FileIO().Open(pf)
			if err != nil {
				return output.ErrValidation("open --patch-file %s: %v", pf, err)
			}
			buf, readErr := io.ReadAll(f)
			f.Close()
			if readErr != nil {
				return output.ErrValidation("read --patch-file %s: %v", pf, readErr)
			}
			var patch templatePatchFile
			if err := json.Unmarshal(buf, &patch); err != nil {
				return output.ErrValidation("parse --patch-file %s: %v", pf, err)
			}
			if patch.TemplateContent != nil {
				contentChanged = true
			}
			applyTemplatePatchFile(tpl, &patch)
		}

		// Apply plain-text → HTML line-break upgrade to newly supplied content
		// so template preview renders line breaks the same way a draft composed
		// from this template would render after sending. Only transform when
		// this update call actually changed the content: if the user left the
		// body alone, we must not re-wrap what the server already stored (doing
		// so would double-wrap existing HTML bodies on every update).
		if contentChanged {
			tpl.TemplateContent = wrapTemplateContentIfNeeded(tpl.TemplateContent, tpl.IsPlainTextMode)
		}
		if int64(len(tpl.TemplateContent)) > maxTemplateContentBytes {
			return output.ErrValidation("template content exceeds %d MB (got %.1f MB)",
				maxTemplateContentBytes/(1024*1024),
				float64(len(tpl.TemplateContent))/1024/1024)
		}

		// Re-resolve <img> references against the (possibly updated) content.
		rewritten, newAtts, err := buildTemplatePayloadFromFlags(
			ctx, runtime, tpl.Name, tpl.Subject, tpl.TemplateContent,
			tpl.Tos, tpl.Ccs, tpl.Bccs,
			splitByComma(runtime.Str("attach")),
		)
		if err != nil {
			return err
		}
		tpl.TemplateContent = rewritten
		// When the body changed, drop existing inline attachments whose CID
		// is no longer referenced in the new template_content. Otherwise
		// every <img> replace/delete leaves an orphan Drive-backed row
		// behind and the template eventually trips TemplateTotalSizeLimit.
		// Non-inline attachments are kept regardless because they aren't
		// addressed via cid: refs. Skipped when the body wasn't touched —
		// the existing cid: refs in the stored content still reference all
		// existing inline rows, so removing any would break the template.
		if contentChanged {
			kept := tpl.Attachments[:0]
			for _, a := range tpl.Attachments {
				if a.IsInline && a.CID != "" && !strings.Contains(tpl.TemplateContent, "cid:"+a.CID) {
					continue
				}
				kept = append(kept, a)
			}
			tpl.Attachments = kept
		}
		// Merge: keep existing template attachments (already uploaded, have
		// file_keys), append newly uploaded ones. The EML-size/LARGE switch
		// applies independently per call because this is a full-replace PUT.
		//
		// Dedup by (ID, CID) so repeated `+template-update --attach foo.png`
		// runs don't accumulate duplicate rows (same Drive file_key /
		// same inline cid); the first occurrence wins.
		seenAttKey := make(map[string]bool, len(tpl.Attachments))
		attKey := func(a templateAttachment) string { return a.ID + "|" + a.CID }
		for _, a := range tpl.Attachments {
			seenAttKey[attKey(a)] = true
		}
		for _, a := range newAtts {
			if seenAttKey[attKey(a)] {
				continue
			}
			seenAttKey[attKey(a)] = true
			tpl.Attachments = append(tpl.Attachments, a)
		}
		// Server rejects the PUT with errno 99992402
		// `template.attachments[*].body is required` when any entry's
		// `body` field is empty. Fetched entries may round-trip without
		// the body populated (the GET response omits raw bytes). Re-fill
		// body from the file_key (which the backend resolves identically)
		// so full-replace updates survive the required-field check.
		for i := range tpl.Attachments {
			if tpl.Attachments[i].Body == "" {
				tpl.Attachments[i].Body = tpl.Attachments[i].ID
			}
		}

		inlineCount, largeCount := countAttachmentsByType(tpl.Attachments)
		logTemplateInfo(runtime, "update.execute", map[string]interface{}{
			"mailbox_id":         mailboxID,
			"template_id":        tid,
			"is_plain_text_mode": tpl.IsPlainTextMode,
			"name_len":           len([]rune(tpl.Name)),
			"attachments_total":  len(tpl.Attachments),
			"inline_count":       inlineCount,
			"large_count":        largeCount,
			"tos_count":          len(tpl.Tos),
			"ccs_count":          len(tpl.Ccs),
			"bccs_count":         len(tpl.Bccs),
		})

		resp, err := updateTemplate(runtime, mailboxID, tid, tpl)
		if err != nil {
			return fmt.Errorf("update template failed: %w", err)
		}
		updated, _ := extractTemplatePayload(resp)
		out := map[string]interface{}{
			"template": updated,
			"warning":  "Template endpoints have no optimistic locking; concurrent updates are last-write-wins.",
		}
		runtime.OutFormat(out, nil, func(w io.Writer) {
			fmt.Fprintln(w, "Template updated (last-write-wins; concurrent writers may overwrite each other).")
			if updated != nil {
				fmt.Fprintf(w, "template_id: %s\n", updated.TemplateID)
				fmt.Fprintf(w, "name: %s\n", updated.Name)
				fmt.Fprintf(w, "attachments: %d\n", len(updated.Attachments))
			}
		})
		fmt.Fprintln(runtime.IO().ErrOut,
			"warning: template endpoints have no optimistic locking; concurrent updates are last-write-wins.")
		return nil
	},
}

// resolveTemplateUpdateContent returns the override body content from
// --set-template-content / --set-template-content-file. Empty string means
// the caller should keep the existing template_content.
func resolveTemplateUpdateContent(runtime *common.RuntimeContext) (content, sourcePath string, err error) {
	if raw := runtime.Str("set-template-content"); raw != "" {
		return raw, "", nil
	}
	path := runtime.Str("set-template-content-file")
	if path == "" {
		return "", "", nil
	}
	f, err := runtime.FileIO().Open(path)
	if err != nil {
		return "", path, output.ErrValidation("open --set-template-content-file %s: %v", path, err)
	}
	defer f.Close()
	buf, err := io.ReadAll(f)
	if err != nil {
		return "", path, output.ErrValidation("read --set-template-content-file %s: %v", path, err)
	}
	return string(buf), path, nil
}

// templatePatchFile mirrors the --print-patch-template skeleton and the
// --patch-file JSON. Any field set to a non-nil value overrides the fetched
// template's corresponding field.
type templatePatchFile struct {
	Name            *string             `json:"name,omitempty"`
	Subject         *string             `json:"subject,omitempty"`
	TemplateContent *string             `json:"template_content,omitempty"`
	IsPlainTextMode *bool               `json:"is_plain_text_mode,omitempty"`
	Tos             *[]templateMailAddr `json:"tos,omitempty"`
	Ccs             *[]templateMailAddr `json:"ccs,omitempty"`
	Bccs            *[]templateMailAddr `json:"bccs,omitempty"`
}

func applyTemplatePatchFile(tpl *templatePayload, patch *templatePatchFile) {
	if patch == nil {
		return
	}
	if patch.Name != nil {
		tpl.Name = *patch.Name
	}
	if patch.Subject != nil {
		tpl.Subject = *patch.Subject
	}
	if patch.TemplateContent != nil {
		tpl.TemplateContent = *patch.TemplateContent
	}
	if patch.IsPlainTextMode != nil {
		tpl.IsPlainTextMode = *patch.IsPlainTextMode
	}
	if patch.Tos != nil {
		tpl.Tos = *patch.Tos
	}
	if patch.Ccs != nil {
		tpl.Ccs = *patch.Ccs
	}
	if patch.Bccs != nil {
		tpl.Bccs = *patch.Bccs
	}
}

// buildTemplatePatchSkeleton returns the JSON skeleton printed by
// --print-patch-template to guide agents assembling a --patch-file.
func buildTemplatePatchSkeleton() map[string]interface{} {
	return map[string]interface{}{
		"name":               "string (≤100 chars, optional)",
		"subject":            "string (optional)",
		"template_content":   "string (HTML or plain text; local <img src> paths are auto-uploaded)",
		"is_plain_text_mode": "bool (optional)",
		"tos":                []map[string]string{{"mail_address": "string", "name": "string(optional)"}},
		"ccs":                []map[string]string{{"mail_address": "string", "name": "string(optional)"}},
		"bccs":               []map[string]string{{"mail_address": "string", "name": "string(optional)"}},
	}
}
