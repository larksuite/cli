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

// MailTemplateCreate creates a new email template.
var MailTemplateCreate = common.Shortcut{
	Service:     "mail",
	Command:     "+template-create",
	Description: "Create a new email template. The --body value is written to the request body as body_html (auto-detected HTML or plain text). Use --plain-text to force plain-text mode.",
	Risk:        "write",
	Scopes:      []string{"mail:user_mailbox.message:modify"},
	AuthTypes:   []string{"user"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "name", Desc: "Required. Template name (max 100 chars)", Required: true},
		{Name: "subject", Desc: "Email subject for the template"},
		{Name: "body", Desc: "Email body (HTML or plain text). The value is written to body_html in the request body."},
		{Name: "to", Desc: "Default To recipients, comma-separated"},
		{Name: "cc", Desc: "Default CC recipients, comma-separated"},
		{Name: "bcc", Desc: "Default BCC recipients, comma-separated"},
		{Name: "plain-text", Type: "bool", Desc: "Force plain-text mode for body"},
		{Name: "mailbox", Default: "me", Desc: "Mailbox ID or email address (default: me)"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		name := strings.TrimSpace(runtime.Str("name"))
		if name == "" {
			return output.ErrValidation("--name is required")
		}
		if len([]rune(name)) > 100 {
			return output.ErrValidation("--name exceeds 100 character limit")
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		mailboxID := resolveMailboxID(runtime)
		return common.NewDryRunAPI().
			Desc("Create a new email template").
			POST(mailboxPath(mailboxID, "templates")).
			Body(map[string]interface{}{"template": buildTemplateCreateBody(runtime)})
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		mailboxID := resolveMailboxID(runtime)
		// Wrap the template object per IDL api.body="template" annotation so
		// that apigw unwraps it into CreateUserMailboxTemplateRequest.Template.
		body := map[string]interface{}{"template": buildTemplateCreateBody(runtime)}
		data, err := runtime.CallAPI("POST", mailboxPath(mailboxID, "templates"), nil, body)
		if err != nil {
			return output.Errorf(output.ExitAPI, "api_error", "create template failed: %s", err)
		}
		tmpl := extractTemplateObject(data)
		if tmpl == nil {
			return output.Errorf(output.ExitAPI, "api_error", "create template: missing template in response")
		}
		runtime.OutFormat(tmpl, nil, func(w io.Writer) {
			fmt.Fprintln(w, "Template created.")
			fmt.Fprintf(w, "template_id: %s\n", strVal(tmpl["template_id"]))
			fmt.Fprintf(w, "name: %s\n", strVal(tmpl["name"]))
		})
		return nil
	},
}

// buildTemplateCreateBody assembles the JSON body for +template-create.
// The value of --body is always written to body_html so that the server
// receives the user-supplied content (see verification report TPL-CREATE-01).
func buildTemplateCreateBody(runtime *common.RuntimeContext) map[string]interface{} {
	body := map[string]interface{}{
		"name": runtime.Str("name"),
	}
	if s := runtime.Str("subject"); s != "" {
		body["subject"] = s
	}
	// --body is the user's email body. It must be written to the
	// request body's body_html field — the server uses body_html for both
	// HTML and plain-text bodies, with is_plain_text_mode toggling the
	// rendering mode.
	if b := runtime.Str("body"); b != "" {
		body["body_html"] = b
	}
	if runtime.Bool("plain-text") {
		body["is_plain_text_mode"] = true
	}
	// Address list JSON keys match the IDL Template struct's api.json
	// annotations (tos/ccs/bccs), while the --to/--cc/--bcc CLI flag names
	// stay singular for a stable user surface.
	if to := runtime.Str("to"); to != "" {
		body["tos"] = parseAddressListForAPI(to)
	}
	if cc := runtime.Str("cc"); cc != "" {
		body["ccs"] = parseAddressListForAPI(cc)
	}
	if bcc := runtime.Str("bcc"); bcc != "" {
		body["bccs"] = parseAddressListForAPI(bcc)
	}
	return body
}

// parseAddressListForAPI converts a comma-separated recipient string into the
// open-api MailAddress array shape: [{"mail_address": "...", "name": "..."}].
// Entries with an empty email are dropped; empty input returns nil.
func parseAddressListForAPI(raw string) []map[string]interface{} {
	boxes := ParseMailboxList(raw)
	if len(boxes) == 0 {
		return nil
	}
	out := make([]map[string]interface{}, 0, len(boxes))
	for _, m := range boxes {
		entry := map[string]interface{}{"mail_address": m.Email}
		if m.Name != "" {
			entry["name"] = m.Name
		}
		out = append(out, entry)
	}
	return out
}
