// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/larksuite/cli/shortcuts/common"
	draftpkg "github.com/larksuite/cli/shortcuts/mail/draft"
	"github.com/larksuite/cli/shortcuts/mail/emlbuilder"
)

// readReceiptRequestLabel is the system label applied to incoming messages
// that carry a Disposition-Notification-To header (SystemLabelReadReceiptRequest=-607).
const readReceiptRequestLabel = "READ_RECEIPT_REQUEST"

// receiptMetaLabelSet groups the localized strings used by the auto-generated
// receipt Subject and body. Mirrors the quoteMetaLabelSet pattern in
// mail_quote.go used by reply / forward.
//
// Labels bake their trailing punctuation ("：" / ": ") in so that callers can
// concatenate without language-specific logic.
type receiptMetaLabelSet struct {
	SubjectPrefix string // "已读回执：" / "Read receipt: "
	Lead          string // first-line statement in the receipt body
	Subject       string // label for the original mail subject
	To            string // label for the original mail's recipient (= the mailbox reading it, which is also the From of the outgoing receipt). This field is the LABEL rendered in the receipt body's quote block — the receipt's envelope recipient (original sender) is set separately via emlbuilder's To() call.
	Sent          string // label for the original send time
	Read          string // label for the current read time (when the receipt was generated)
}

// receiptMetaLabels returns the zh / en label set; "zh" is selected when
// detectSubjectLang finds CJK content. Matches the CLI-wide convention set by
// mail_quote.go:quoteMetaLabels — zh / en only, driven by the original subject.
func receiptMetaLabels(lang string) receiptMetaLabelSet {
	if lang == "zh" {
		return receiptMetaLabelSet{
			SubjectPrefix: "已读回执：",
			Lead:          "您发送的邮件已被阅读，详情如下：",
			Subject:       "主题：",
			To:            "收件人：",
			Sent:          "发送时间：",
			Read:          "阅读时间：",
		}
	}
	return receiptMetaLabelSet{
		SubjectPrefix: "Read receipt: ",
		Lead:          "Your message has been read. Details:",
		Subject:       "Subject: ",
		To:            "To: ",
		Sent:          "Sent: ",
		Read:          "Read: ",
	}
}

// MailSendReceipt is the `+send-receipt` shortcut: send an auto-generated
// read-receipt reply (RFC 3798 MDN) for an incoming message that carries
// the READ_RECEIPT_REQUEST label. Risk is "high-risk-write"; callers must
// pass --yes.
var MailSendReceipt = common.Shortcut{
	Service:     "mail",
	Command:     "+send-receipt",
	Description: "Send a read-receipt reply for an incoming message that requested one (i.e. carries the READ_RECEIPT_REQUEST label). Body is auto-generated (subject / recipient / send time / read time) to match the Lark client's receipt format — callers cannot customize it, matching the industry norm that read-receipt bodies are system-generated templates, not free-form replies. Intended for agent use after the user confirms.",
	Risk:        "high-risk-write",
	Scopes: []string{
		"mail:user_mailbox.message:send",
		"mail:user_mailbox.message:modify",
		"mail:user_mailbox.message:readonly",
		"mail:user_mailbox:readonly",
		"mail:user_mailbox.message.address:read",
		"mail:user_mailbox.message.subject:read",
		// +send-receipt doesn't read the body content itself, but
		// fetchFullMessage(..., false) uses format=plain_text_full which
		// the backend scope-checks against body:read. Declared explicitly
		// to keep the static Scopes truthful and aligned with +triage /
		// +message / +thread which all list this scope.
		"mail:user_mailbox.message.body:read",
	},
	AuthTypes: []string{"user"},
	Flags: []common.Flag{
		{Name: "message-id", Desc: "Required. Message ID of the incoming mail that requested a read receipt.", Required: true},
		{Name: "mailbox", Desc: "Mailbox email address that owns the receipt reply (default: me)."},
		{Name: "from", Desc: "Sender email address for the From header. Defaults to the mailbox's primary address."},
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		messageID := runtime.Str("message-id")
		mailboxID := resolveComposeMailboxID(runtime)
		return common.NewDryRunAPI().
			Desc("Send read receipt: fetch the original message → verify the READ_RECEIPT_REQUEST label is present → build a reply with subject \"已读回执：<original>\" (zh) or \"Read receipt: <original>\" (en) picked by CJK detection on the original subject, In-Reply-To / References threading, and X-Lark-Read-Receipt-Mail: 1 → create draft and send. The backend extracts the private header, sets BodyExtra.IsReadReceiptMail, and DraftSend applies the READ_RECEIPT_SENT label to the outgoing message.").
			GET(mailboxPath(mailboxID, "messages", messageID)).
			Params(map[string]interface{}{"format": messageGetFormat(false)}).
			GET(mailboxPath(mailboxID, "profile")).
			POST(mailboxPath(mailboxID, "drafts")).
			Body(map[string]interface{}{"raw": "<base64url-EML>"}).
			POST(mailboxPath(mailboxID, "drafts", "<draft_id>", "send"))
	},
	// No Validate: +send-receipt takes no user-provided content (subject /
	// body / recipients are all derived from the original message). The
	// :send scope is declared in static Scopes above and pre-checked by
	// runner.checkShortcutScopes before Execute runs, so dynamic scope
	// validation here would be redundant. Mirrors +send, which also keeps
	// :send in static Scopes and skips validateConfirmSendScope.
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		messageID := runtime.Str("message-id")

		mailboxID := resolveComposeMailboxID(runtime)

		msg, err := fetchFullMessage(runtime, mailboxID, messageID, false)
		if err != nil {
			return fmt.Errorf("failed to fetch original message: %w", err)
		}
		if !hasReadReceiptRequestLabel(msg) {
			return fmt.Errorf("message %s did not request a read receipt (no %s label); refusing to send receipt", messageID, readReceiptRequestLabel)
		}

		origSubject := strVal(msg["subject"])
		origSMTPID := normalizeMessageID(strVal(msg["smtp_message_id"]))
		origFromEmail, _ := extractAddressPair(msg["head_from"])
		origReferences := joinReferences(msg["references"])
		origSendMillis := parseInternalDateMillis(msg["internal_date"])

		if origFromEmail == "" {
			return fmt.Errorf("original message %s has no sender address; cannot address receipt", messageID)
		}

		senderEmail := resolveComposeSenderEmail(runtime)
		if senderEmail == "" {
			return fmt.Errorf("unable to determine sender email; please specify --from explicitly")
		}

		lang := detectSubjectLang(origSubject)
		readTime := time.Now()
		textBody := buildReceiptTextBody(lang, origSubject, senderEmail, origSendMillis, readTime)
		htmlBody := buildReceiptHTMLBody(lang, origSubject, senderEmail, origSendMillis, readTime)

		bld := emlbuilder.New().WithFileIO(runtime.FileIO()).
			Subject(buildReceiptSubject(origSubject)).
			From("", senderEmail).
			To("", origFromEmail).
			TextBody([]byte(textBody)).
			HTMLBody([]byte(htmlBody)).
			IsReadReceiptMail(true)
		if origSMTPID != "" {
			bld = bld.InReplyTo(origSMTPID)
		}
		if refs := buildReceiptReferences(origReferences, origSMTPID); refs != "" {
			bld = bld.References(refs)
		}
		if messageID != "" {
			bld = bld.LMSReplyToMessageID(messageID)
		}

		rawEML, err := bld.BuildBase64URL()
		if err != nil {
			return fmt.Errorf("failed to build receipt EML: %w", err)
		}

		draftResult, err := draftpkg.CreateWithRaw(runtime, mailboxID, rawEML)
		if err != nil {
			return fmt.Errorf("failed to create receipt draft: %w", err)
		}
		resData, err := draftpkg.Send(runtime, mailboxID, draftResult.DraftID, "")
		if err != nil {
			return fmt.Errorf("failed to send receipt (draft %s created but not sent): %w", draftResult.DraftID, err)
		}

		out := buildDraftSendOutput(resData, mailboxID)
		out["receipt_for_message_id"] = messageID
		runtime.OutFormat(out, nil, func(w io.Writer) {
			fmt.Fprintln(w, "已对原邮件发送回执 / Read receipt sent.")
			fmt.Fprintf(w, "receipt_for_message_id: %s\n", messageID)
		})
		return nil
	},
}

// hasReadReceiptRequestLabel returns true when the message's label_ids include
// either the symbolic name "READ_RECEIPT_REQUEST" or the numeric system-label
// id "-607" (backends have returned both forms historically).
func hasReadReceiptRequestLabel(msg map[string]interface{}) bool {
	labels := toStringList(msg["label_ids"])
	for _, l := range labels {
		if l == readReceiptRequestLabel || l == "-607" {
			return true
		}
	}
	return false
}

// maybeHintReadReceiptRequest prints a stderr tip if the just-read message
// carries a read-receipt request. Noop for messages without the label or
// without a resolvable message_id. Called by +message / +messages / +thread
// after primary JSON output so callers and humans both see it.
func maybeHintReadReceiptRequest(runtime *common.RuntimeContext, mailboxID, messageID string, msg map[string]interface{}) {
	if messageID == "" || !hasReadReceiptRequestLabel(msg) {
		return
	}
	fromEmail, _ := extractAddressPair(msg["head_from"])
	subject := strVal(msg["subject"])
	hintReadReceiptRequest(runtime, mailboxID, messageID, fromEmail, subject)
}

// buildReceiptSubject prepends the language-appropriate receipt prefix once.
// Language is detected from the original subject itself, matching
// buildReplySubject / buildForwardSubject in mail_quote.go.
//
// Idempotent: if the subject already starts with a known receipt prefix
// (zh "已读回执：" or en "Read receipt: "), the existing prefix is stripped
// before the language-appropriate one is re-applied. This matters when the
// input is already a receipt (unusual, but not rejected elsewhere) and keeps
// us from producing "Read receipt: 已读回执：..." chains.
//
// NOTE: the backend GetRealSubject regex is driven by TCC
// MailPrefixConfig.SubjectPrefixListForAdvancedSearch — that list must include
// both "已读回执：" and "Read receipt: " for conversation aggregation to work
// across languages. zh was already covered; en requires a TCC update.
func buildReceiptSubject(original string) string {
	trimmed := strings.TrimSpace(original)
	// Detect language on the ORIGINAL subject so that the prefix we re-apply
	// matches the author's intent even when every remaining CJK character
	// lives inside a prefix we're about to strip (e.g. "已读回执：已读回执：x"
	// → strip both prefixes → "x", but the author obviously wanted zh).
	lang := detectSubjectLang(trimmed)
	// Strip either known prefix case-insensitively (en), exact (zh). Loop so
	// accidental chains ("Read receipt: Read receipt: ...") collapse too.
	for {
		switch {
		case strings.HasPrefix(trimmed, "已读回执："):
			trimmed = strings.TrimSpace(strings.TrimPrefix(trimmed, "已读回执："))
		case strings.HasPrefix(strings.ToLower(trimmed), "read receipt:"):
			trimmed = strings.TrimSpace(trimmed[len("read receipt:"):])
		default:
			return receiptMetaLabels(lang).SubjectPrefix + trimmed
		}
	}
}

// buildReceiptReferences appends the original message's SMTP Message-ID to its
// existing References chain, producing the References header for the receipt.
// Both inputs are optional; the return value is a space-joined list with angle
// brackets, suitable for the emlbuilder References() method.
func buildReceiptReferences(origRefs, origSMTPID string) string {
	var parts []string
	if trimmed := strings.TrimSpace(origRefs); trimmed != "" {
		parts = append(parts, trimmed)
	}
	if origSMTPID != "" {
		parts = append(parts, "<"+origSMTPID+">")
	}
	return strings.Join(parts, " ")
}

// extractAddressPair returns (email, name) from the head_from / reply_to /
// entry in the raw /messages response, handling both object and string forms.
func extractAddressPair(v interface{}) (email, name string) {
	switch t := v.(type) {
	case map[string]interface{}:
		email = strVal(t["mail_address"])
		name = strVal(t["name"])
	case string:
		email = t
	}
	return email, name
}

// parseInternalDateMillis parses the internal_date field from a /messages
// response (which the API returns as a string-encoded Unix millisecond
// timestamp). Returns 0 if the value is missing or unparseable; callers render
// a placeholder in that case rather than erroring.
func parseInternalDateMillis(v interface{}) int64 {
	s := strings.TrimSpace(strVal(v))
	if s == "" {
		return 0
	}
	ms, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0
	}
	return ms
}

// renderReceiptTime formats a millisecond timestamp for display inside the
// receipt body. Returns an empty-safe placeholder when the timestamp is 0.
// Reuses formatMailDate (mail_quote.go) so receipts read the same way as
// the quote block used by +reply / +forward.
func renderReceiptTime(ms int64, lang string) string {
	if ms <= 0 {
		return "-"
	}
	return formatMailDate(ms, lang)
}

// buildReceiptTextBody returns the plain-text body used when a +send-receipt
// sends the auto-generated acknowledgement. The layout mirrors the Lark PC /
// Mobile clients' receipt body: one header line followed by quoted key-value
// lines for subject / recipient / send time / read time. Callers cannot
// customize this body — the Subject field carries the receipt prefix which
// is the semantically meaningful signal; free-form user notes belong in a
// normal +reply instead.
func buildReceiptTextBody(lang, origSubject, origRecipient string, origSendMillis int64, readTime time.Time) string {
	labels := receiptMetaLabels(lang)
	var b strings.Builder
	b.WriteString(labels.Lead)
	b.WriteByte('\n')
	fmt.Fprintf(&b, "> %s%s\n", labels.Subject, strings.TrimSpace(origSubject))
	fmt.Fprintf(&b, "> %s%s\n", labels.To, origRecipient)
	fmt.Fprintf(&b, "> %s%s\n", labels.Sent, renderReceiptTime(origSendMillis, lang))
	fmt.Fprintf(&b, "> %s%s\n", labels.Read, formatMailDate(readTime.UnixMilli(), lang))
	return b.String()
}

// buildReceiptHTMLBody returns the HTML body for the auto-generated receipt.
// Intentionally simpler than the Lark PC client's HTML (no branded styling,
// no proprietary markers) — just enough structure (leading statement + quoted
// key-value block) to render nicely in any MUA. All user-controlled values go
// through htmlEscape to prevent injection from the original subject / headers.
func buildReceiptHTMLBody(lang, origSubject, origRecipient string, origSendMillis int64, readTime time.Time) string {
	labels := receiptMetaLabels(lang)
	var b strings.Builder
	b.WriteString(`<div style="word-break:break-word;">`)
	b.WriteString(`<div style="margin:4px 0;line-height:1.6;font-size:14px;">`)
	b.WriteString(htmlEscape(labels.Lead))
	b.WriteString(`</div>`)
	b.WriteString(`<div style="padding:12px;background:#f5f6f7;color:#1f2329;border-radius:4px;margin-top:12px;word-break:break-word;">`)
	fmt.Fprintf(&b, `<div><span>%s</span> %s</div>`, htmlEscape(labels.Subject), htmlEscape(strings.TrimSpace(origSubject)))
	fmt.Fprintf(&b, `<div><span>%s</span> %s</div>`, htmlEscape(labels.To), htmlEscape(origRecipient))
	fmt.Fprintf(&b, `<div><span>%s</span> %s</div>`, htmlEscape(labels.Sent), htmlEscape(renderReceiptTime(origSendMillis, lang)))
	fmt.Fprintf(&b, `<div><span>%s</span> %s</div>`, htmlEscape(labels.Read), htmlEscape(formatMailDate(readTime.UnixMilli(), lang)))
	b.WriteString(`</div>`)
	b.WriteString(`</div>`)
	return b.String()
}

// joinReferences flattens the references field from the raw /messages response
// into a single space-separated string (the API returns an array of IDs).
func joinReferences(v interface{}) string {
	refs := toStringList(v)
	if len(refs) == 0 {
		return ""
	}
	// Ensure each entry is surrounded by angle brackets.
	out := make([]string, 0, len(refs))
	for _, r := range refs {
		r = strings.TrimSpace(r)
		if r == "" {
			continue
		}
		if !strings.HasPrefix(r, "<") {
			r = "<" + r
		}
		if !strings.HasSuffix(r, ">") {
			r = r + ">"
		}
		out = append(out, r)
	}
	return strings.Join(out, " ")
}
