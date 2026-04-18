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

// MailTemplateUpdate updates an existing email template.
//
// The update semantic is GET-merge-PUT: we first fetch the existing template,
// overlay the user-provided flags, and then PUT the full object back. This
// gives callers a "partial update" feel even though the backend PUT semantics
// is full replacement.
var MailTemplateUpdate = common.Shortcut{
	Service:     "mail",
	Command:     "+template-update",
	Description: "Update an existing email template. Only specified fields are changed; omitted fields keep their current values (GET → merge → PUT).",
	Risk:        "write",
	Scopes:      []string{"mail:user_mailbox.message:modify"},
	AuthTypes:   []string{"user"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "template-id", Desc: "Required. Template ID to update", Required: true},
		{Name: "name", Desc: "New template name"},
		{Name: "subject", Desc: "New email subject"},
		{Name: "body", Desc: "New email body (HTML or plain text). Written to body_html."},
		{Name: "to", Desc: "New default To recipients"},
		{Name: "cc", Desc: "New default CC recipients"},
		{Name: "bcc", Desc: "New default BCC recipients"},
		{Name: "plain-text", Type: "bool", Desc: "Force plain-text mode"},
		{Name: "mailbox", Default: "me", Desc: "Mailbox ID or email address (default: me)"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if strings.TrimSpace(runtime.Str("template-id")) == "" {
			return output.ErrValidation("--template-id is required")
		}
		if name := strings.TrimSpace(runtime.Str("name")); name != "" && len([]rune(name)) > 100 {
			return output.ErrValidation("--name exceeds 100 character limit")
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		mailboxID := resolveMailboxID(runtime)
		templateID := runtime.Str("template-id")
		return common.NewDryRunAPI().
			Desc("Update an existing email template (GET → merge → PUT)").
			GET(mailboxPath(mailboxID, "templates", templateID)).
			PUT(mailboxPath(mailboxID, "templates", templateID))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		mailboxID := resolveMailboxID(runtime)
		templateID := runtime.Str("template-id")

		existing, err := runtime.CallAPI("GET", mailboxPath(mailboxID, "templates", templateID), nil, nil)
		if err != nil {
			return output.Errorf(output.ExitAPI, "api_error", "get template failed: %s", err)
		}
		tmpl := extractTemplateObject(existing)
		if tmpl == nil {
			return output.Errorf(output.ExitAPI, "api_error", "template %s not found", templateID)
		}
		mergeTemplateUpdateFlags(runtime, tmpl)

		// Wrap the merged template object per IDL api.body="template" so the
		// apigw routes the payload into the PUT request's Template field.
		putBody := map[string]interface{}{"template": tmpl}
		data, err := runtime.CallAPI("PUT", mailboxPath(mailboxID, "templates", templateID), nil, putBody)
		if err != nil {
			return output.Errorf(output.ExitAPI, "api_error", "update template failed: %s", err)
		}
		updated := extractTemplateObject(data)
		if updated == nil {
			updated = tmpl
		}
		runtime.OutFormat(updated, nil, func(w io.Writer) {
			fmt.Fprintln(w, "Template updated.")
			fmt.Fprintf(w, "template_id: %s\n", strVal(updated["template_id"]))
		})
		return nil
	},
}

// mergeTemplateUpdateFlags overlays user-provided flags onto an existing
// template object. It mutates tmpl in place.
func mergeTemplateUpdateFlags(runtime *common.RuntimeContext, tmpl map[string]interface{}) {
	if n := runtime.Str("name"); n != "" {
		tmpl["name"] = n
	}
	if s := runtime.Str("subject"); s != "" {
		tmpl["subject"] = s
	}
	if b := runtime.Str("body"); b != "" {
		tmpl["body_html"] = b
	}
	if runtime.Bool("plain-text") {
		tmpl["is_plain_text_mode"] = true
	}
	// Address list JSON keys align with the IDL Template struct
	// (api.json="tos"/"ccs"/"bccs"); CLI flags remain singular.
	if to := runtime.Str("to"); to != "" {
		tmpl["tos"] = parseAddressListForAPI(to)
	}
	if cc := runtime.Str("cc"); cc != "" {
		tmpl["ccs"] = parseAddressListForAPI(cc)
	}
	if bcc := runtime.Str("bcc"); bcc != "" {
		tmpl["bccs"] = parseAddressListForAPI(bcc)
	}
}
