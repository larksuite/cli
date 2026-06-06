// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"context"
	"fmt"
	"strings"

	"github.com/larksuite/cli/shortcuts/common"
)

var MailSignatureUpdate = common.Shortcut{
	Service:     "mail",
	Command:     "+signature-update",
	Description: "Update an existing personal (USER) mail signature with full-replace semantics. Omitted fields are cleared; local HTML images are uploaded and rewritten to cid: references before PUT.",
	Risk:        "write",
	Scopes:      []string{"mail:user_mailbox.message:modify", "mail:user_mailbox:readonly"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "mailbox", Desc: "Mailbox email address that owns the signature (default: me)."},
		{Name: "signature-id", Desc: "Signature ID to update.", Required: true},
		{Name: "name", Desc: "Required. Replacement signature name.", Required: true},
		{Name: "content", Desc: "Replacement signature body. Prefer HTML; local <img src> values are auto-uploaded and rewritten to cid: refs."},
		{Name: "content-file", Desc: "Path to a file used as --content. Relative path only. Mutually exclusive with --content."},
		{Name: "device", Desc: "PC (default) or MOBILE."},
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		mailboxID := resolveComposeMailboxID(runtime)
		signatureID := runtime.Str("signature-id")
		content, _, rcErr := resolveSignatureContent(runtime)
		if rcErr != nil {
			fmt.Fprintf(runtime.IO().ErrOut, "warning: dry-run could not load signature content: %v\n", rcErr)
		}
		api := common.NewDryRunAPI().
			Desc("Update a personal mail signature with full-replace semantics. Omitted fields are cleared. The command uploads local inline images to Drive, rewrites HTML to cid: references, and PUTs a USER signature payload.")
		for _, img := range parseLocalImgs(content) {
			addTemplateUploadSteps(runtime, api, img.Path)
		}
		device, _ := signatureDeviceFromRuntime(runtime)
		return api.PUT(signatureMailboxPath(mailboxID, signatureID)).
			Body(map[string]interface{}{
				"signature": map[string]interface{}{
					"id":               signatureID,
					"name":             runtime.Str("name"),
					"content":          "<rewritten-HTML-or-empty>",
					"signature_type":   "USER",
					"signature_device": string(device),
					"images":           "<computed from uploads>",
				},
				"_warning": "Full replace: omitted fields are cleared.",
			})
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if err := validateBotMailboxNotMe(runtime); err != nil {
			return err
		}
		if strings.TrimSpace(runtime.Str("signature-id")) == "" {
			return mailValidationParamError("--signature-id", "--signature-id is required")
		}
		if strings.TrimSpace(runtime.Str("name")) == "" {
			return mailValidationParamError("--name", "--name is required")
		}
		if runtime.Str("content") != "" && runtime.Str("content-file") != "" {
			return mailValidationError("--content and --content-file are mutually exclusive").
				WithParams(
					mailInvalidParam("--content", "mutually exclusive with --content-file"),
					mailInvalidParam("--content-file", "mutually exclusive with --content"),
				)
		}
		_, err := signatureDeviceFromRuntime(runtime)
		return err
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		mailboxID := resolveComposeMailboxID(runtime)
		signatureID := runtime.Str("signature-id")
		content, _, err := resolveSignatureContent(runtime)
		if err != nil {
			return err
		}
		device, err := signatureDeviceFromRuntime(runtime)
		if err != nil {
			return err
		}
		payload, err := buildSignaturePayloadFromFlags(ctx, runtime, runtime.Str("name"), content, device)
		if err != nil {
			return err
		}
		fmt.Fprintln(runtime.IO().ErrOut, "warning: signature update is full-replace; omitted fields are cleared.")
		resp, err := updateSignature(runtime, mailboxID, signatureID, payload)
		if err != nil {
			return decorateSignatureWriteError(err, "update signature failed")
		}
		sig, err := extractSignaturePayload(resp)
		if err != nil {
			return err
		}
		outputSignatureResult(runtime, "Signature updated.", sig)
		return nil
	},
}
