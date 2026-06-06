// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"context"
	"fmt"
	"strings"

	"github.com/larksuite/cli/shortcuts/common"
)

var MailSignatureCreate = common.Shortcut{
	Service:     "mail",
	Command:     "+signature-create",
	Description: "Create a personal (USER) mail signature. Scans HTML <img src> local paths, uploads inline images to Drive, rewrites them to cid: references, and POSTs to settings/signatures.",
	Risk:        "write",
	Scopes:      []string{"mail:user_mailbox.message:modify", "mail:user_mailbox:readonly"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "mailbox", Desc: "Mailbox email address that owns the signature (default: me)."},
		{Name: "name", Desc: "Required. Signature name.", Required: true},
		{Name: "content", Desc: "Signature body. Prefer HTML; local <img src> values are auto-uploaded and rewritten to cid: refs."},
		{Name: "content-file", Desc: "Path to a file used as --content. Relative path only. Mutually exclusive with --content."},
		{Name: "device", Desc: "PC (default) or MOBILE."},
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		mailboxID := resolveComposeMailboxID(runtime)
		content, _, rcErr := resolveSignatureContent(runtime)
		if rcErr != nil {
			fmt.Fprintf(runtime.IO().ErrOut, "warning: dry-run could not load signature content: %v\n", rcErr)
		}
		api := common.NewDryRunAPI().
			Desc("Create a personal mail signature. The command scans HTML for local <img src> references, uploads each inline image to Drive, rewrites <img src> values to cid: references, and POSTs a USER signature payload.")
		for _, img := range parseLocalImgs(content) {
			addTemplateUploadSteps(runtime, api, img.Path)
		}
		device, _ := signatureDeviceFromRuntime(runtime)
		return api.POST(signatureMailboxPath(mailboxID)).
			Body(map[string]interface{}{
				"signature": map[string]interface{}{
					"name":             runtime.Str("name"),
					"content":          "<rewritten-HTML>",
					"signature_type":   "USER",
					"signature_device": string(device),
					"images":           "<computed from uploads>",
				},
			})
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if err := validateBotMailboxNotMe(runtime); err != nil {
			return err
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
		resp, err := createSignature(runtime, mailboxID, payload)
		if err != nil {
			return decorateSignatureWriteError(err, "create signature failed")
		}
		sig, err := extractSignaturePayload(resp)
		if err != nil {
			return err
		}
		outputSignatureResult(runtime, "Signature created.", sig)
		return nil
	},
}
