// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"context"
	"fmt"
	"io"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
)

// MailTemplateList lists all email templates in the specified mailbox.
var MailTemplateList = common.Shortcut{
	Service:     "mail",
	Command:     "+template-list",
	Description: "List all email templates in the current mailbox. Returns template_id, name, subject and create_time for each template.",
	Risk:        "read",
	Scopes:      []string{"mail:user_mailbox.message:readonly"},
	AuthTypes:   []string{"user"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "mailbox", Default: "me", Desc: "Mailbox ID or email address (default: me)"},
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		mailboxID := resolveMailboxID(runtime)
		return common.NewDryRunAPI().
			Desc("List all email templates for the specified mailbox").
			GET(mailboxPath(mailboxID, "templates"))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		mailboxID := resolveMailboxID(runtime)
		data, err := runtime.CallAPI("GET", mailboxPath(mailboxID, "templates"), nil, nil)
		if err != nil {
			return output.Errorf(output.ExitAPI, "api_error", "list templates failed: %s", err)
		}
		items := extractTemplateList(data)
		runtime.OutFormat(map[string]interface{}{"items": items}, &output.Meta{Count: len(items)},
			func(w io.Writer) {
				if len(items) == 0 {
					fmt.Fprintln(w, "No templates found.")
					return
				}
				rows := make([]map[string]interface{}, 0, len(items))
				for _, t := range items {
					tm, _ := t.(map[string]interface{})
					if tm == nil {
						continue
					}
					rows = append(rows, map[string]interface{}{
						"template_id": tm["template_id"],
						"name":        tm["name"],
						"subject":     common.TruncateStr(fmt.Sprint(tm["subject"]), 40),
						"create_time": tm["create_time"],
					})
				}
				output.PrintTable(w, rows)
				fmt.Fprintf(w, "\n%d template(s)\n", len(items))
			})
		return nil
	},
}

// extractTemplateList returns the list of templates from an open-apis list response.
// The API uses the "items" field (see tech-design ListUserMailboxTemplateResponse);
// older call sites used "templates", so both are accepted for forward/backward compat.
func extractTemplateList(data map[string]interface{}) []interface{} {
	if data == nil {
		return nil
	}
	if list, ok := data["items"].([]interface{}); ok {
		return list
	}
	if list, ok := data["templates"].([]interface{}); ok {
		return list
	}
	return nil
}
