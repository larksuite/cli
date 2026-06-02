// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"context"
	"fmt"
	"io"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/larksuite/cli/shortcuts/mail/signature"
)

var MailSignatureDelete = common.Shortcut{
	Service:     "mail",
	Command:     "+signature-delete",
	Description: "Delete a personal USER mail signature after GET validation. Requires --yes for non-interactive safety.",
	Risk:        "high-risk-write",
	Scopes:      []string{"mail:user_mailbox.message:modify", "mail:user_mailbox:readonly"},
	AuthTypes:   []string{"user"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "mailbox", Desc: "Mailbox address that owns the signature (default: me)."},
		{Name: "signature-id", Desc: "USER signature ID to delete.", Required: true},
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		mailboxID := resolveComposeMailboxID(runtime)
		signatureID := runtime.Str("signature-id")
		return common.NewDryRunAPI().
			Desc("Delete a personal USER mail signature: GET current signatures, reject TENANT or missing targets, DELETE the signature, then best-effort GET to verify absence. Default signature application refs are cleared by the server.").
			GET(mailboxPath(mailboxID, "settings", "signatures")).
			DELETE(mailboxPath(mailboxID, "settings", "signatures", signatureID)).
			GET(mailboxPath(mailboxID, "settings", "signatures"))
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if err := validateSignatureID(runtime.Str("signature-id")); err != nil {
			return err
		}
		if runtime.Str("signature-id") == "" {
			return output.ErrValidation("--signature-id is required")
		}
		if !runtime.Bool("dry-run") && !runtime.Bool("yes") {
			return output.ErrValidation("--yes is required to delete a signature in non-interactive mode")
		}
		return nil
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		mailboxID := resolveComposeMailboxID(runtime)
		signatureID := runtime.Str("signature-id")
		current, err := findSignature(runtime, mailboxID, signatureID)
		if err != nil {
			return err
		}
		if current.SignatureType != signature.SignatureTypeUser {
			return output.ErrValidation("only USER signatures can be deleted")
		}
		if err := signature.Delete(runtime, mailboxID, signatureID); err != nil {
			return err
		}
		verifyStatus := "unknown"
		if resp, err := signature.ListAll(runtime, mailboxID); err == nil {
			verifyStatus = "absent"
			for _, sig := range resp.Signatures {
				if sig.ID == signatureID {
					verifyStatus = "unknown"
					break
				}
			}
		}
		out := map[string]interface{}{
			"deleted":              true,
			"deleted_signature_id": signatureID,
			"name":                 current.Name,
			"signature_device":     current.SignatureDevice,
			"verify_status":        verifyStatus,
		}
		runtime.OutFormat(out, nil, func(w io.Writer) {
			fmt.Fprintln(w, "Signature deleted.")
			fmt.Fprintf(w, "deleted_signature_id: %s\n", signatureID)
			fmt.Fprintf(w, "verify_status: %s\n", verifyStatus)
		})
		return nil
	},
}
