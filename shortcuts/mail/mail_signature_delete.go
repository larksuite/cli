// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"context"
	"io"
	"strings"

	"github.com/larksuite/cli/shortcuts/common"
)

var MailSignatureDelete = common.Shortcut{
	Service:     "mail",
	Command:     "+signature-delete",
	Description: "Delete a personal (USER) mail signature by ID.",
	Risk:        "write",
	Scopes:      []string{"mail:user_mailbox.message:modify", "mail:user_mailbox:readonly"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "mailbox", Desc: "Mailbox email address that owns the signature (default: me)."},
		{Name: "signature-id", Desc: "Signature ID to delete.", Required: true},
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		mailboxID := resolveComposeMailboxID(runtime)
		return common.NewDryRunAPI().
			Desc("Delete a personal mail signature.").
			DELETE(signatureMailboxPath(mailboxID, runtime.Str("signature-id")))
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if err := validateBotMailboxNotMe(runtime); err != nil {
			return err
		}
		if strings.TrimSpace(runtime.Str("signature-id")) == "" {
			return mailValidationParamError("--signature-id", "--signature-id is required")
		}
		return nil
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		mailboxID := resolveComposeMailboxID(runtime)
		signatureID := runtime.Str("signature-id")
		if err := deleteSignature(runtime, mailboxID, signatureID); err != nil {
			return decorateSignatureWriteError(err, "delete signature failed")
		}
		out := map[string]interface{}{
			"deleted":      true,
			"mailbox_id":   mailboxID,
			"signature_id": signatureID,
		}
		runtime.OutFormat(out, nil, func(w io.Writer) {
			w.Write([]byte("Signature deleted.\n"))
			w.Write([]byte("signature_id: " + signatureID + "\n"))
		})
		return nil
	},
}
