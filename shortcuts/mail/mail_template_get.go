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

// MailTemplateGet fetches a single email template by ID.
var MailTemplateGet = common.Shortcut{
	Service:     "mail",
	Command:     "+template-get",
	Description: "Get a specific email template by ID, including subject, body and default recipients.",
	Risk:        "read",
	Scopes:      []string{"mail:user_mailbox.message:readonly"},
	AuthTypes:   []string{"user"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "template-id", Desc: "Required. Template ID", Required: true},
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
			Desc("Get email template details").
			GET(mailboxPath(mailboxID, "templates", templateID))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		mailboxID := resolveMailboxID(runtime)
		templateID := runtime.Str("template-id")
		data, err := runtime.CallAPI("GET", mailboxPath(mailboxID, "templates", templateID), nil, nil)
		if err != nil {
			return output.Errorf(output.ExitAPI, "api_error", "get template failed: %s", err)
		}
		tmpl := extractTemplateObject(data)
		if tmpl == nil {
			return output.Errorf(output.ExitAPI, "api_error", "template %s not found", templateID)
		}
		runtime.OutFormat(tmpl, nil, func(w io.Writer) {
			fmt.Fprintf(w, "Template: %s\n", strVal(tmpl["name"]))
			fmt.Fprintf(w, "Subject:  %s\n", strVal(tmpl["subject"]))
			fmt.Fprintf(w, "ID:       %s\n", strVal(tmpl["template_id"]))
			if to := tmpl["tos"]; to != nil {
				fmt.Fprintf(w, "To:       %s\n", describeAddressField(to))
			}
			if cc := tmpl["ccs"]; cc != nil {
				fmt.Fprintf(w, "Cc:       %s\n", describeAddressField(cc))
			}
			if bcc := tmpl["bccs"]; bcc != nil {
				fmt.Fprintf(w, "Bcc:      %s\n", describeAddressField(bcc))
			}
		})
		return nil
	},
}

// extractTemplateObject returns the "template" field from an API response,
// tolerating responses that already flattened the object at the top level.
func extractTemplateObject(data map[string]interface{}) map[string]interface{} {
	if data == nil {
		return nil
	}
	if tmpl, ok := data["template"].(map[string]interface{}); ok {
		return tmpl
	}
	if _, hasID := data["template_id"]; hasID {
		return data
	}
	return nil
}

// describeAddressField renders a template address list for pretty output.
// It accepts []interface{} of {mail_address,name} maps or a plain string.
func describeAddressField(v interface{}) string {
	switch t := v.(type) {
	case string:
		return t
	case []interface{}:
		parts := make([]string, 0, len(t))
		for _, item := range t {
			m, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			email := strVal(m["mail_address"])
			if email == "" {
				email = strVal(m["email"])
			}
			name := strVal(m["name"])
			if name != "" {
				parts = append(parts, fmt.Sprintf("%s <%s>", name, email))
			} else if email != "" {
				parts = append(parts, email)
			}
		}
		return strings.Join(parts, ", ")
	}
	return ""
}
