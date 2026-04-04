// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
	draftpkg "github.com/larksuite/cli/shortcuts/mail/draft"
)

var MailDraftEdit = common.Shortcut{
	Service:     "mail",
	Command:     "+draft-edit",
	Description: "Use when updating an existing mail draft without sending it. Prefer this shortcut over calling raw drafts.get or drafts.update directly, because it performs draft-safe MIME read/patch/write editing while preserving unchanged structure, attachments, and headers where possible.",
	Risk:        "write",
	Scopes:      []string{"mail:user_mailbox.message:modify", "mail:user_mailbox.message:readonly", "mail:user_mailbox:readonly"},
	AuthTypes:   []string{"user"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "from", Default: "me", Desc: "Mailbox email address containing the draft (default: me)"},
		{Name: "draft-id", Desc: "Target draft ID. Required for real edits. It can be omitted only when using the --print-patch-template flag by itself."},
		{Name: "set-subject", Desc: "Replace the subject with this final value. Use this for full subject replacement, not for appending a fragment to the existing subject."},
		{Name: "set-to", Desc: "Replace the entire To recipient list with the addresses provided here. Separate multiple addresses with commas. Display-name format is supported."},
		{Name: "set-cc", Desc: "Replace the entire Cc recipient list with the addresses provided here. Separate multiple addresses with commas. Display-name format is supported."},
		{Name: "set-bcc", Desc: "Replace the entire Bcc recipient list with the addresses provided here. Separate multiple addresses with commas. Display-name format is supported."},
		{Name: "patch-file", Desc: "Edit entry point for body edits, incremental recipient changes, header edits, attachment changes, or inline-image changes. All body edits MUST go through --patch-file. Two body ops: set_body (full replacement including quote) and set_reply_body (replaces only user-authored content, auto-preserves quote block). Run --inspect first to check has_quoted_content, then --print-patch-template for the JSON structure. Relative path only."},
		{Name: "print-patch-template", Type: "bool", Desc: "Print the JSON template and supported operations for the --patch-file flag. Recommended first step before generating a patch file. No draft read or write is performed."},
		{Name: "inspect", Type: "bool", Desc: "Inspect the draft without modifying it. Returns the draft projection including subject, recipients, body summary, has_quoted_content (whether the draft contains a reply/forward quote block), attachments_summary (with part_id and cid for each attachment), and inline_summary. Run this BEFORE editing body to check has_quoted_content: if true, use set_reply_body in --patch-file to preserve the quote; if false, use set_body."},
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		if runtime.Bool("print-patch-template") {
			return common.NewDryRunAPI().
				Set("mode", "print-patch-template").
				Set("template", buildDraftEditPatchTemplate())
		}
		draftID := runtime.Str("draft-id")
		if draftID == "" {
			return common.NewDryRunAPI().Set("error", "--draft-id is required for real draft edits; only --print-patch-template can be used without a draft id")
		}
		mailboxID := resolveComposeMailboxID(runtime)
		if runtime.Bool("inspect") {
			return common.NewDryRunAPI().
				Desc("Inspect a draft without modifying it: fetch the raw EML, parse it into MIME structure, and return the projection (subject, recipients, body, attachments_summary, inline_summary). No write is performed.").
				GET(mailboxPath(mailboxID, "drafts", draftID)).
				Params(map[string]interface{}{"format": "raw"})
		}
		patch, err := buildDraftEditPatch(runtime)
		if err != nil {
			return common.NewDryRunAPI().Set("error", err.Error())
		}
		return common.NewDryRunAPI().
			Desc("Edit an existing draft without sending it: first call drafts.get(format=raw) to fetch the current EML, parse it into MIME structure, apply either direct flags or the typed patch from patch-file, re-serialize the updated draft, and then call drafts.update. This is a minimal-edit pipeline rather than a full rebuild, so unchanged headers, attachments, and MIME subtrees are preserved where possible. Body edits must go through --patch-file using set_body or set_reply_body ops. It also has no optimistic locking, so concurrent edits to the same draft are last-write-wins.").
			GET(mailboxPath(mailboxID, "drafts", draftID)).
			Params(map[string]interface{}{"format": "raw"}).
			PUT(mailboxPath(mailboxID, "drafts", draftID)).
			Body(map[string]interface{}{
				"raw":     "<base64url-EML>",
				"_patch":  patch.Summary(),
				"_notice": "This edit flow has no optimistic locking. If the same draft is changed concurrently, the last writer wins.",
			})
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if runtime.Bool("print-patch-template") {
			runtime.Out(buildDraftEditPatchTemplate(), nil)
			return nil
		}
		draftID := runtime.Str("draft-id")
		if draftID == "" {
			return output.ErrValidation("--draft-id is required for real draft edits; if you only need a patch template, run with --print-patch-template")
		}
		mailboxID := resolveComposeMailboxID(runtime)
		if runtime.Bool("inspect") {
			return executeDraftInspect(runtime, mailboxID, draftID)
		}
		patch, err := buildDraftEditPatch(runtime)
		if err != nil {
			return err
		}
		rawDraft, err := draftpkg.GetRaw(runtime, mailboxID, draftID)
		if err != nil {
			return fmt.Errorf("read draft raw EML failed: %w", err)
		}
		snapshot, err := draftpkg.Parse(rawDraft)
		if err != nil {
			return output.ErrValidation("parse draft raw EML failed: %v", err)
		}
		if err := draftpkg.Apply(snapshot, patch); err != nil {
			return output.ErrValidation("apply draft patch failed: %v", err)
		}
		serialized, err := draftpkg.Serialize(snapshot)
		if err != nil {
			return output.ErrValidation("serialize draft failed: %v", err)
		}
		if err := draftpkg.UpdateWithRaw(runtime, mailboxID, draftID, serialized); err != nil {
			return fmt.Errorf("update draft failed: %w", err)
		}
		projection := draftpkg.Project(snapshot)
		out := map[string]interface{}{
			"draft_id":   draftID,
			"warning":    "This edit flow has no optimistic locking. If the same draft is changed concurrently, the last writer wins.",
			"projection": projection,
		}
		runtime.OutFormat(out, nil, func(w io.Writer) {
			fmt.Fprintln(w, "Draft updated.")
			fmt.Fprintf(w, "draft_id: %s\n", draftID)
			if projection.Subject != "" {
				fmt.Fprintf(w, "subject: %s\n", sanitizeForTerminal(projection.Subject))
			}
			if recipients := prettyDraftAddresses(projection.To); recipients != "" {
				fmt.Fprintf(w, "to: %s\n", sanitizeForTerminal(recipients))
			}
			if projection.BodyText != "" {
				fmt.Fprintf(w, "body_text: %s\n", sanitizeForTerminal(projection.BodyText))
			}
			if projection.BodyHTMLSummary != "" {
				fmt.Fprintf(w, "body_html_summary: %s\n", sanitizeForTerminal(projection.BodyHTMLSummary))
			}
			if len(projection.AttachmentsSummary) > 0 {
				fmt.Fprintf(w, "attachments: %d\n", len(projection.AttachmentsSummary))
			}
			if len(projection.InlineSummary) > 0 {
				fmt.Fprintf(w, "inline_parts: %d\n", len(projection.InlineSummary))
			}
			if len(projection.Warnings) > 0 {
				fmt.Fprintf(w, "warnings: %s\n", sanitizeForTerminal(strings.Join(projection.Warnings, "; ")))
			}
			fmt.Fprintln(w, "warning: This edit flow has no optimistic locking. If the same draft is changed concurrently, the last writer wins.")
		})
		return nil
	},
}

func executeDraftInspect(runtime *common.RuntimeContext, mailboxID, draftID string) error {
	rawDraft, err := draftpkg.GetRaw(runtime, mailboxID, draftID)
	if err != nil {
		return fmt.Errorf("read draft raw EML failed: %w", err)
	}
	snapshot, err := draftpkg.Parse(rawDraft)
	if err != nil {
		return output.ErrValidation("parse draft raw EML failed: %v", err)
	}
	projection := draftpkg.Project(snapshot)
	out := map[string]interface{}{
		"draft_id":   draftID,
		"projection": projection,
	}
	runtime.OutFormat(out, nil, func(w io.Writer) {
		fmt.Fprintln(w, "Draft inspection (read-only, no changes applied).")
		fmt.Fprintf(w, "draft_id: %s\n", draftID)
		if projection.Subject != "" {
			fmt.Fprintf(w, "subject: %s\n", sanitizeForTerminal(projection.Subject))
		}
		if recipients := prettyDraftAddresses(projection.To); recipients != "" {
			fmt.Fprintf(w, "to: %s\n", sanitizeForTerminal(recipients))
		}
		if recipients := prettyDraftAddresses(projection.Cc); recipients != "" {
			fmt.Fprintf(w, "cc: %s\n", sanitizeForTerminal(recipients))
		}
		if projection.BodyText != "" {
			fmt.Fprintf(w, "body_text: %s\n", sanitizeForTerminal(projection.BodyText))
		}
		if projection.BodyHTMLSummary != "" {
			fmt.Fprintf(w, "body_html_summary: %s\n", sanitizeForTerminal(projection.BodyHTMLSummary))
		}
		if projection.HasQuotedContent {
			fmt.Fprintln(w, "has_quoted_content: true (use set_reply_body op in --patch-file to edit body while preserving the quote)")
		}
		if len(projection.AttachmentsSummary) > 0 {
			fmt.Fprintf(w, "attachments (%d):\n", len(projection.AttachmentsSummary))
			for _, att := range projection.AttachmentsSummary {
				fmt.Fprintf(w, "  - part_id=%s  filename=%s  content_type=%s  cid=%s\n",
					att.PartID, att.FileName, att.ContentType, att.CID)
			}
		}
		if len(projection.InlineSummary) > 0 {
			fmt.Fprintf(w, "inline_parts (%d):\n", len(projection.InlineSummary))
			for _, inl := range projection.InlineSummary {
				fmt.Fprintf(w, "  - part_id=%s  filename=%s  content_type=%s  cid=%s\n",
					inl.PartID, inl.FileName, inl.ContentType, inl.CID)
			}
		}
		if len(projection.Warnings) > 0 {
			fmt.Fprintf(w, "warnings: %s\n", sanitizeForTerminal(strings.Join(projection.Warnings, "; ")))
		}
	})
	return nil
}

func prettyDraftAddresses(addrs []draftpkg.Address) string {
	if len(addrs) == 0 {
		return ""
	}
	parts := make([]string, 0, len(addrs))
	for _, addr := range addrs {
		parts = append(parts, addr.String())
	}
	return strings.Join(parts, ", ")
}

func buildDraftEditPatch(runtime *common.RuntimeContext) (draftpkg.Patch, error) {
	patch := draftpkg.Patch{
		Options: draftpkg.PatchOptions{
			AllowProtectedHeaderEdits: runtime.Bool("allow-protected-header-edit"),
			RewriteEntireDraft:        runtime.Bool("rewrite-entire-draft"),
		},
	}

	patchFile := strings.TrimSpace(runtime.Str("patch-file"))
	if patchFile != "" {
		filePatch, err := loadPatchFile(patchFile)
		if err != nil {
			return patch, err
		}
		patch.Ops = append(patch.Ops, filePatch.Ops...)
		if filePatch.Options.AllowProtectedHeaderEdits {
			patch.Options.AllowProtectedHeaderEdits = true
		}
		if filePatch.Options.RewriteEntireDraft {
			patch.Options.RewriteEntireDraft = true
		}
	}

	setRecipients := func(field, raw string) {
		if strings.TrimSpace(raw) == "" {
			return
		}
		addrs := parseNetAddrs(raw)
		opAddrs := make([]draftpkg.Address, 0, len(addrs))
		for _, addr := range addrs {
			opAddrs = append(opAddrs, draftpkg.Address{
				Name:    addr.Name,
				Address: addr.Address,
			})
		}
		patch.Ops = append(patch.Ops, draftpkg.PatchOp{
			Op:        "set_recipients",
			Field:     field,
			Addresses: opAddrs,
		})
	}

	if value := strings.TrimSpace(runtime.Str("set-subject")); value != "" {
		patch.Ops = append(patch.Ops, draftpkg.PatchOp{Op: "set_subject", Value: value})
	}
	if err := validateRecipientCount(runtime.Str("set-to"), runtime.Str("set-cc"), runtime.Str("set-bcc")); err != nil {
		return patch, err
	}
	setRecipients("to", runtime.Str("set-to"))
	setRecipients("cc", runtime.Str("set-cc"))
	setRecipients("bcc", runtime.Str("set-bcc"))

	if len(patch.Ops) == 0 {
		return patch, output.ErrValidation("at least one edit operation is required; use direct flags such as --set-subject/--set-to, or use --patch-file for body edits and other advanced operations (run --print-patch-template first)")
	}
	return patch, patch.Validate()
}

func loadPatchFile(path string) (draftpkg.Patch, error) {
	var patch draftpkg.Patch
	safePath, err := validate.SafeInputPath(path)
	if err != nil {
		return patch, fmt.Errorf("--patch-file %q: %w", path, err)
	}
	data, err := os.ReadFile(safePath)
	if err != nil {
		return patch, err
	}
	if err := json.Unmarshal(data, &patch); err != nil {
		return patch, fmt.Errorf("parse patch file: %w", err)
	}
	return patch, patch.Validate()
}

func buildDraftEditPatchTemplate() map[string]interface{} {
	return map[string]interface{}{
		"description": "Typed patch JSON for `mail +draft-edit --patch-file`. This is not RFC 6902 JSON Patch.",
		"template": map[string]interface{}{
			"ops": []map[string]interface{}{
				{"op": "set_subject", "value": "Updated subject"},
				{"op": "set_recipients", "field": "to", "addresses": []map[string]interface{}{{"address": "alice@example.com", "name": "Alice"}}},
				{"op": "set_body", "value": "Updated body"},
			},
			"options": map[string]interface{}{
				"rewrite_entire_draft":         false,
				"allow_protected_header_edits": false,
			},
		},
		"options_help": map[string]interface{}{
			"rewrite_entire_draft":         "Default false. Set to true only when the edit must synthesize or restructure body parts, for example adding a missing primary body part.",
			"allow_protected_header_edits": "Default false. Set to true only when the user explicitly wants to edit protected headers and understands the threading or delivery risk.",
		},
		"supported_ops": []map[string]interface{}{
			{"op": "set_subject", "shape": map[string]interface{}{"value": "string"}},
			{"op": "set_recipients", "shape": map[string]interface{}{"field": "to|cc|bcc", "addresses": []map[string]interface{}{{"address": "string", "name": "string(optional)"}}}},
			{"op": "add_recipient", "shape": map[string]interface{}{"field": "to|cc|bcc", "address": "string", "name": "string(optional)"}},
			{"op": "remove_recipient", "shape": map[string]interface{}{"field": "to|cc|bcc", "address": "string"}},
			{"op": "set_body", "shape": map[string]interface{}{"value": "string"}},
			{"op": "set_reply_body", "shape": map[string]interface{}{"value": "string (user-authored content only, WITHOUT the quote block; the quote block is re-appended automatically)"}},
			{"op": "set_header", "shape": map[string]interface{}{"name": "string", "value": "string"}},
			{"op": "remove_header", "shape": map[string]interface{}{"name": "string"}},
			{"op": "add_attachment", "shape": map[string]interface{}{"path": "string(relative path)"}},
			{"op": "remove_attachment", "shape": map[string]interface{}{"target": map[string]interface{}{"part_id": "string(optional)", "cid": "string(optional)"}}},
			{"op": "add_inline", "shape": map[string]interface{}{"path": "string(relative path)", "cid": "string", "filename": "string(optional)", "content_type": "string(optional)"}},
			{"op": "replace_inline", "shape": map[string]interface{}{"target": map[string]interface{}{"part_id": "string(optional)", "cid": "string(optional)"}, "path": "string(relative path)", "cid": "string(optional)", "filename": "string(optional)", "content_type": "string(optional)"}},
			{"op": "remove_inline", "shape": map[string]interface{}{"target": map[string]interface{}{"part_id": "string(optional)", "cid": "string(optional)"}}},
		},
		"supported_ops_by_group": []map[string]interface{}{
			{
				"group": "subject_and_body",
				"ops": []map[string]interface{}{
					{"op": "set_subject", "shape": map[string]interface{}{"value": "string"}},
					{"op": "set_body", "shape": map[string]interface{}{"value": "string"}},
					{"op": "set_reply_body", "shape": map[string]interface{}{"value": "string (user-authored content only, WITHOUT the quote block; the quote block is re-appended automatically)"}},
				},
			},
			{
				"group": "recipients",
				"ops": []map[string]interface{}{
					{"op": "set_recipients", "shape": map[string]interface{}{"field": "to|cc|bcc", "addresses": []map[string]interface{}{{"address": "string", "name": "string(optional)"}}}},
					{"op": "add_recipient", "shape": map[string]interface{}{"field": "to|cc|bcc", "address": "string", "name": "string(optional)"}},
					{"op": "remove_recipient", "shape": map[string]interface{}{"field": "to|cc|bcc", "address": "string"}},
				},
			},
			{
				"group": "headers",
				"ops": []map[string]interface{}{
					{"op": "set_header", "shape": map[string]interface{}{"name": "string", "value": "string"}},
					{"op": "remove_header", "shape": map[string]interface{}{"name": "string"}},
				},
			},
			{
				"group": "attachments_and_inline",
				"ops": []map[string]interface{}{
					{"op": "add_attachment", "shape": map[string]interface{}{"path": "string(relative path)"}},
					{"op": "remove_attachment", "shape": map[string]interface{}{"target": map[string]interface{}{"part_id": "string(optional)", "cid": "string(optional)"}}},
					{"op": "add_inline", "shape": map[string]interface{}{"path": "string(relative path)", "cid": "string", "filename": "string(optional)", "content_type": "string(optional)"}},
					{"op": "replace_inline", "shape": map[string]interface{}{"target": map[string]interface{}{"part_id": "string(optional)", "cid": "string(optional)"}, "path": "string(relative path)", "cid": "string(optional)", "filename": "string(optional)", "content_type": "string(optional)"}},
					{"op": "remove_inline", "shape": map[string]interface{}{"target": map[string]interface{}{"part_id": "string(optional)", "cid": "string(optional)"}}},
				},
			},
		},
		"recommended_usage": []string{
			"Use direct flags (--set-subject, --set-to, --set-cc, --set-bcc) for simple metadata edits",
			"Use --patch-file for ALL body edits and advanced changes (recipients, headers, attachments, inline images)",
			"Before editing body, run --inspect to check has_quoted_content; if true, use set_reply_body instead of set_body",
		},
		"body_edit_decision_guide": []map[string]interface{}{
			{"situation": "plain draft or non-reply/forward draft", "recommended_op": "set_body — replaces entire body"},
			{"situation": "draft has both text/plain and text/html", "recommended_op": "set_body — updates HTML body and regenerates plain-text summary; pass HTML input because the original main body is text/html"},
			{"situation": "draft created by +reply or +forward (has_quoted_content=true)", "recommended_op": "set_reply_body — replaces only the user-authored portion and automatically preserves the quoted original message; if user explicitly wants to remove the quote, use set_body instead"},
		},
		"notes": []string{
			"`ops` is executed in order",
			"all file paths (--patch-file and `path` fields in ops) must be relative — no absolute paths or .. traversal",
			"all body edits MUST go through --patch-file; there is no --set-body flag",
			"`set_body` replaces the ENTIRE body including any reply/forward quote block; when the draft has both text/plain and text/html, it updates the HTML body and regenerates the plain-text summary, so the input should be HTML",
			"`set_reply_body` replaces only the user-authored portion of the body and automatically re-appends the trailing reply/forward quote block (generated by +reply or +forward); the value you pass should contain ONLY the new user-authored content WITHOUT the quote block — the quote block will be re-inserted automatically; if the user wants to modify content INSIDE the quote block, use `set_body` instead for full replacement; if the draft has no quote block, it behaves identically to `set_body`",
			"`add_inline` only adds the MIME binary part; it does NOT insert an <img> tag into the HTML body; to display the image in the body, you must ALSO use set_body/set_reply_body to insert <img src=\"cid:...\"> into the body content; forgetting this causes the inline part to become an orphaned attachment when sent",
			"`body_kind` only supports text/plain and text/html",
			"`selector` currently only supports primary",
			"`remove_attachment` target supports part_id or cid; priority: part_id > cid",
			"`remove_attachment`/`remove_inline` require part_id or cid; to discover these values, run `+draft-edit --draft-id <id> --inspect` first — the response `projection.attachments_summary` and `projection.inline_summary` list every part with its part_id, cid, and filename",
			"`add_inline`/`replace_inline`/`remove_inline` are for CID-based inline images",
			"`replace_inline` keeps the original filename and content_type when those fields are omitted",
			"protected headers require `allow_protected_header_edits=true`",
		},
		"command_example":    "lark-cli mail +draft-edit --print-patch-template",
		"patch_file_example": "lark-cli mail +draft-edit --draft-id d_xxx --patch-file ./patch.json",
	}
}
