// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"context"
	"fmt"
	"strings"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
	draftpkg "github.com/larksuite/cli/shortcuts/mail/draft"
	"github.com/larksuite/cli/shortcuts/mail/emlbuilder"
)

// MailTemplateSend composes an email from a template and saves it as a draft
// (or sends immediately with --confirm-send).
var MailTemplateSend = common.Shortcut{
	Service: "mail",
	Command: "+template-send",
	Description: "Compose an email from a template and save as draft (default). " +
		"Use --confirm-send to send the draft immediately after user confirmation.",
	Risk: "write",
	Scopes: []string{
		"mail:user_mailbox.message:readonly",
		"mail:user_mailbox.message:send",
		"mail:user_mailbox.message:modify",
		"mail:user_mailbox:readonly",
	},
	AuthTypes: []string{"user"},
	HasFormat: true,
	Flags: []common.Flag{
		{Name: "template-id", Desc: "Required. Template ID to use", Required: true},
		{Name: "to", Desc: "Override To recipients (comma-separated). If omitted, uses template defaults."},
		{Name: "subject", Desc: "Override subject. If omitted, uses template subject."},
		{Name: "body", Desc: "Override body. If omitted, uses template body."},
		{Name: "cc", Desc: "Override CC recipients"},
		{Name: "bcc", Desc: "Override BCC recipients"},
		{Name: "from", Desc: "Sender address (defaults to the authenticated user's primary mailbox)"},
		{Name: "confirm-send", Type: "bool", Desc: "Send the email immediately instead of saving as draft."},
		{Name: "mailbox", Default: "me", Desc: "Mailbox ID (default: me)"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if strings.TrimSpace(runtime.Str("template-id")) == "" {
			return output.ErrValidation("--template-id is required")
		}
		return validateConfirmSendScope(runtime)
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		mailboxID := resolveMailboxID(runtime)
		templateID := runtime.Str("template-id")
		confirmSend := runtime.Bool("confirm-send")
		desc := "Load template → build EML → create draft"
		if confirmSend {
			desc += " → send draft"
		}
		api := common.NewDryRunAPI().
			Desc(desc).
			GET(mailboxPath(mailboxID, "templates", templateID)).
			GET(mailboxPath(mailboxID, "profile")).
			POST(mailboxPath(mailboxID, "drafts"))
		if confirmSend {
			api = api.POST(mailboxPath(mailboxID, "drafts", "<draft_id>", "send"))
		}
		return api
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		mailboxID := resolveMailboxID(runtime)
		templateID := runtime.Str("template-id")
		confirmSend := runtime.Bool("confirm-send")

		// 1. Fetch the template.
		tmplResp, err := runtime.CallAPI("GET", mailboxPath(mailboxID, "templates", templateID), nil, nil)
		if err != nil {
			return output.Errorf(output.ExitAPI, "api_error", "get template failed: %s", err)
		}
		tmpl := extractTemplateObject(tmplResp)
		if tmpl == nil {
			return output.Errorf(output.ExitAPI, "api_error", "template %s not found", templateID)
		}

		// 2. Merge flags over the template defaults.
		merged, err := mergeTemplateSendFields(runtime, tmpl)
		if err != nil {
			return err
		}

		// 3. Resolve sender.
		senderEmail := strings.TrimSpace(runtime.Str("from"))
		if senderEmail == "" {
			if email, fetchErr := fetchMailboxPrimaryEmail(runtime, "me"); fetchErr == nil {
				senderEmail = email
			}
		}

		// 4. Build EML.
		rawEML, err := buildTemplateEML(merged, senderEmail)
		if err != nil {
			return output.Errorf(output.ExitAPI, "build_error", "build EML from template failed: %s", err)
		}

		// 5. Create draft.
		draftID, err := draftpkg.CreateWithRaw(runtime, mailboxID, rawEML)
		if err != nil {
			return fmt.Errorf("create draft from template failed: %w", err)
		}

		if !confirmSend {
			runtime.Out(map[string]interface{}{
				"draft_id":    draftID,
				"template_id": templateID,
				"tip": fmt.Sprintf(
					`draft saved from template. To send: lark-cli mail user_mailbox.drafts send --params '{"user_mailbox_id":"%s","draft_id":"%s"}'`,
					mailboxID, draftID),
			}, nil)
			hintSendDraft(runtime, mailboxID, draftID)
			return nil
		}

		// 6. Send immediately.
		resData, err := draftpkg.Send(runtime, mailboxID, draftID)
		if err != nil {
			return fmt.Errorf("draft %s created from template but send failed: %w", draftID, err)
		}
		runtime.Out(map[string]interface{}{
			"message_id":  resData["message_id"],
			"thread_id":   resData["thread_id"],
			"template_id": templateID,
		}, nil)
		return nil
	},
}

// templateSendFields holds the merged fields used to construct the outgoing EML.
type templateSendFields struct {
	Subject   string
	Body      string
	To        string
	CC        string
	BCC       string
	PlainText bool
}

// mergeTemplateSendFields overlays user flags on top of the template defaults
// and validates the recipient requirement.
func mergeTemplateSendFields(runtime *common.RuntimeContext, tmpl map[string]interface{}) (templateSendFields, error) {
	merged := templateSendFields{
		Subject:   coalesceStr(runtime.Str("subject"), strVal(tmpl["subject"])),
		Body:      coalesceStr(runtime.Str("body"), strVal(tmpl["body_html"])),
		To:        coalesceStr(runtime.Str("to"), stringifyTemplateAddressList(tmpl["to"])),
		CC:        coalesceStr(runtime.Str("cc"), stringifyTemplateAddressList(tmpl["cc"])),
		BCC:       coalesceStr(runtime.Str("bcc"), stringifyTemplateAddressList(tmpl["bcc"])),
		PlainText: boolVal(tmpl["is_plain_text_mode"]),
	}
	if strings.TrimSpace(merged.To) == "" {
		return merged, output.ErrValidation(
			"no recipients: template has no default To and --to was not specified")
	}
	return merged, nil
}

// buildTemplateEML constructs a base64url-encoded EML from the merged template fields.
func buildTemplateEML(merged templateSendFields, senderEmail string) (string, error) {
	bld := emlbuilder.New().
		Subject(merged.Subject).
		ToAddrs(parseNetAddrs(merged.To))
	if senderEmail != "" {
		bld = bld.From("", senderEmail)
	}
	if merged.CC != "" {
		bld = bld.CCAddrs(parseNetAddrs(merged.CC))
	}
	if merged.BCC != "" {
		bld = bld.BCCAddrs(parseNetAddrs(merged.BCC))
	}
	switch {
	case merged.PlainText:
		bld = bld.TextBody([]byte(merged.Body))
	case bodyIsHTML(merged.Body):
		bld = bld.HTMLBody([]byte(merged.Body))
	default:
		bld = bld.TextBody([]byte(merged.Body))
	}
	return bld.BuildBase64URL()
}

// coalesceStr returns the first non-empty (after trimming) value, or "" if none.
func coalesceStr(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// stringifyTemplateAddressList renders an API-shaped address list back to the
// comma-separated "Name <email>, email2" form used by --to/--cc/--bcc.
func stringifyTemplateAddressList(v interface{}) string {
	list, ok := v.([]interface{})
	if !ok {
		if s, ok := v.(string); ok {
			return s
		}
		return ""
	}
	parts := make([]string, 0, len(list))
	for _, item := range list {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		email := strVal(m["mail_address"])
		if email == "" {
			email = strVal(m["email"])
		}
		if email == "" {
			continue
		}
		name := strVal(m["name"])
		if name != "" {
			parts = append(parts, fmt.Sprintf("%s <%s>", name, email))
		} else {
			parts = append(parts, email)
		}
	}
	return strings.Join(parts, ", ")
}
