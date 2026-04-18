// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"context"
	"strings"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
)

// MailTemplateDelete deletes an email template.
var MailTemplateDelete = common.Shortcut{
	Service:     "mail",
	Command:     "+template-delete",
	Description: "Delete an email template by ID.",
	Risk:        "delete",
	Scopes:      []string{"mail:user_mailbox.message:modify"},
	AuthTypes:   []string{"user"},
	Flags: []common.Flag{
		{Name: "template-id", Desc: "Required. Template ID to delete", Required: true},
		{Name: "mailbox", Default: "me", Desc: "Mailbox ID or email address (default: me)"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if strings.TrimSpace(runtime.Str("template-id")) == "" {
			return output.ErrValidation("--template-id is required")
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		mailboxID := resolveMailboxID(runtime)
		templateID := runtime.Str("template-id")
		return common.NewDryRunAPI().
			Desc("Delete an email template").
			DELETE(mailboxPath(mailboxID, "templates", templateID))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		mailboxID := resolveMailboxID(runtime)
		templateID := runtime.Str("template-id")
		if _, err := runtime.CallAPI("DELETE", mailboxPath(mailboxID, "templates", templateID), nil, nil); err != nil {
			return output.Errorf(output.ExitAPI, "api_error", "delete template failed: %s", err)
		}
		runtime.Out(map[string]interface{}{
			"deleted":     true,
			"template_id": templateID,
		}, nil)
		return nil
	},
}
