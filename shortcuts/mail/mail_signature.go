// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"context"
	"regexp"
	"strings"

	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/larksuite/cli/shortcuts/mail/signature"
)

var MailSignature = common.Shortcut{
	Service:     "mail",
	Command:     "+signature",
	Description: "List or view email signatures with default usage info.",
	Risk:        "read",
	Scopes:      []string{"mail:user_mailbox:readonly"},
	AuthTypes:   []string{"user"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "from", Default: "me", Desc: "Mailbox address (default: me)"},
		{Name: "detail", Desc: "Signature ID to view rendered details. Omit to list all signatures."},
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		mailboxID := runtime.Str("from")
		if mailboxID == "" {
			mailboxID = "me"
		}
		return common.NewDryRunAPI().
			Desc("List or view email signatures").
			GET(mailboxPath(mailboxID, "signatures"))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		mailboxID := runtime.Str("from")
		if mailboxID == "" {
			mailboxID = "me"
		}
		detailID := runtime.Str("detail")

		resp, err := signature.ListAll(runtime, mailboxID)
		if err != nil {
			return err
		}

		if detailID != "" {
			return executeSignatureDetail(runtime, resp, detailID, mailboxID)
		}
		return executeSignatureList(runtime, resp)
	},
}

func executeSignatureList(runtime *common.RuntimeContext, resp *signature.GetSignaturesResponse) error {
	// Build default signature ID maps from usages.
	sendDefaults := map[string]bool{}
	replyDefaults := map[string]bool{}
	for _, usage := range resp.Usages {
		if usage.SendMailSignatureID != "" && usage.SendMailSignatureID != "0" {
			sendDefaults[usage.SendMailSignatureID] = true
		}
		if usage.ReplySignatureID != "" && usage.ReplySignatureID != "0" {
			replyDefaults[usage.ReplySignatureID] = true
		}
	}

	lang := resolveLang(runtime)
	items := make([]map[string]interface{}, 0, len(resp.Signatures))
	for _, sig := range resp.Signatures {
		item := map[string]interface{}{
			"id":   sig.ID,
			"name": sig.Name,
			"type": string(sig.SignatureType),
		}
		if len(sig.Images) > 0 {
			item["images"] = len(sig.Images)
		}

		// Short content preview (rendered for TENANT).
		rendered := signature.InterpolateTemplate(&sig, lang, "", "")
		item["content_preview"] = contentPreview(rendered, 200, lang)

		if sendDefaults[sig.ID] {
			item["is_send_default"] = true
		}
		if replyDefaults[sig.ID] {
			item["is_reply_default"] = true
		}

		items = append(items, item)
	}

	runtime.OutFormat(
		map[string]interface{}{"signatures": items},
		&output.Meta{Count: len(items)},
		nil,
	)
	return nil
}

func executeSignatureDetail(runtime *common.RuntimeContext, resp *signature.GetSignaturesResponse, sigID, mailboxID string) error {
	var sig *signature.Signature
	for i := range resp.Signatures {
		if resp.Signatures[i].ID == sigID {
			sig = &resp.Signatures[i]
			break
		}
	}
	if sig == nil {
		return output.ErrValidation("signature not found: %s", sigID)
	}

	lang := resolveLang(runtime)

	detail := map[string]interface{}{
		"id":   sig.ID,
		"name": sig.Name,
		"type": string(sig.SignatureType),
	}

	// Usage info.
	for _, usage := range resp.Usages {
		if usage.SendMailSignatureID == sig.ID {
			detail["is_send_default"] = true
		}
		if usage.ReplySignatureID == sig.ID {
			detail["is_reply_default"] = true
		}
	}

	// Images metadata — output the full structure from API.
	if len(sig.Images) > 0 {
		detail["images"] = sig.Images
	}

	// Template variables (TENANT signatures): show resolved values.
	if sig.HasTemplateVars() {
		vars := make(map[string]string, len(sig.UserFields))
		for key, field := range sig.UserFields {
			vars[key] = field.Resolve(lang)
		}
		detail["template_vars"] = vars
	}

	// Rendered content preview.
	rendered := signature.InterpolateTemplate(sig, lang, "", "")
	detail["content_preview"] = contentPreview(rendered, 200, lang)

	runtime.Out(detail, nil)
	return nil
}

// resolveLang maps CLI config lang ("zh"/"en") to i18n key ("zh_cn"/"en_us").
func resolveLang(runtime *common.RuntimeContext) string {
	multi, err := core.LoadMultiAppConfig()
	if err != nil {
		return "zh_cn"
	}
	cfg, err := runtime.Factory.Config()
	if err != nil {
		return "zh_cn"
	}
	app := multi.FindApp(cfg.ProfileName)
	if app == nil {
		return "zh_cn"
	}
	switch app.Lang {
	case "en":
		return "en_us"
	case "ja":
		return "ja_jp"
	default:
		return "zh_cn"
	}
}

// contentPreview converts HTML to a compact plain-text preview.
// <img> tags become a localized image placeholder, all other tags become
// spaces, then consecutive whitespace is collapsed. Result is truncated
// to maxLen runes.
func contentPreview(html string, maxLen int, lang string) string {
	placeholder := "[image]"
	if strings.HasPrefix(lang, "zh") {
		placeholder = "[图片]"
	}
	imgRe := regexp.MustCompile(`<img[^>]*>`)
	s := imgRe.ReplaceAllString(html, placeholder)

	// Strip remaining tags, replacing each with a space.
	var result strings.Builder
	inTag := false
	for _, r := range s {
		switch {
		case r == '<':
			inTag = true
			result.WriteByte(' ')
		case r == '>':
			inTag = false
		case !inTag:
			result.WriteRune(r)
		}
	}

	// Collapse whitespace and trim.
	text := strings.Join(strings.Fields(result.String()), " ")
	text = strings.TrimSpace(text)

	runes := []rune(text)
	if len(runes) <= maxLen {
		return text
	}
	return string(runes[:maxLen]) + "..."
}
