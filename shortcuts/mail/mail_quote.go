// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"fmt"
	"math/rand"
	"net/url"
	"regexp"
	"strings"
	"time"

	draftpkg "github.com/larksuite/cli/shortcuts/mail/draft"
)

// ---- HTML quote block builders (Lark adit-html-block structure) ---------------
//
// These helpers mirror the structure used by Lark's mail composer:
//   - Reply/Reply-all: adit-html-block--collapsed (collapsible quoted block)
//   - Forward:         adit-html-block--header    (always-expanded header block)

// ---- CSS class / style constants -------------------------------------------

const (
	// quoteIDChars is the character set for generated quote element IDs.
	quoteIDChars = "abcdefghijklmnopqrstuvwxyz0123456789"

	// quoteBlockBorderStyle is the shared border style for all adit-html-block wrappers.
	quoteBlockBorderStyle = "border-left: none; padding-left: 0px;"

	// metaBlockStyle is the shared style for quote meta info blocks (From/Date/Subject/To/Cc).
	metaBlockStyle = "padding: 12px; background: rgb(245, 246, 247); color: rgb(31, 35, 41); border-radius: 4px; margin-bottom: 12px;"

	// replyMetaMargin is the margin-top applied to reply/reply-all meta blocks.
	replyMetaMargin = "margin-top: 24px;"

	// forwardMetaMargin is the margin-top applied to forward meta blocks (closer to separator).
	forwardMetaMargin = "margin-top: 2px;"

	// separatorStyle is the style for the forward separator line.
	separatorStyle = "color: rgb(100, 106, 115); margin-top: 24px; margin-bottom: 8px;"

	// bodyDivStyle is applied to the user-authored body <div> in HTML emails.
	bodyDivStyle = "word-break:break-word;line-height:1.6;font-size:14px;color:rgb(0,0,0);"

	// addressAnchorStyle is the inline style for mailto hyperlinks.
	addressAnchorStyle = "overflow-wrap: break-word; color: inherit; text-decoration: none; white-space: pre-wrap; hyphens: none; word-break: break-word; cursor: pointer;"
)

// ---- HTML format-string templates ------------------------------------------
// All structural HTML is kept here so functions contain only logic, not markup.

const (
	// addressAnchorFmt renders a mailto <a> element: (escapedAddr, escapedAddr, escapedAddr).
	addressAnchorFmt = `<a class="quote-head-meta-mailto" data-mailto="mailto:%s" href="mailto:%s" style="` + addressAnchorStyle + `">%s</a>`

	// metaRowFmt renders one labeled metadata row (label, content).
	metaRowFmt = `<div class="lme-line-signal"><span style="">%s: %s</span></div>`

	// replyMetaWrapperFmt wraps meta rows for reply/reply-all (margin, style, inner).
	replyMetaWrapperFmt = `<div class="adit-html-block__attr history-quote-meta-wrapper history-quote-gap-tag" style="%s %s"><div style="word-break: break-word;">%s</div></div>`

	// forwardMetaWrapperFmt wraps meta rows for forward (id, margin, style, inner).
	forwardMetaWrapperFmt = `<div id="%s" class="adit-html-block__header history-quote-meta-after-forward-title history-quote-meta-wrapper" style="%s %s"><div style="word-break: break-word;">%s</div></div>`

	// separatorDivFmt renders the forward separator line (style, text).
	separatorDivFmt = `<div class="history-quote-forward-title lme-line-signal history-quote-gap-tag" style="%s">%s</div>`

	// plainTextBodyFmt wraps a plain-text quoted body for use inside HTML emails (escapedText).
	plainTextBodyFmt = `<pre style="white-space:pre-wrap">%s</pre>`
)

// ---- Quote wrapper formats (var, not const, because they reference draftpkg.QuoteWrapperClass) ---

var (
	// replyQuoteHTMLFmt is the outer collapsed-block structure for reply (style, prefix, body).
	replyQuoteHTMLFmt = `<div class="` + draftpkg.QuoteWrapperClass + `"><div data-html-block="quote" data-mail-html-ignore="">` +
		`<div class="adit-html-block adit-html-block--collapsed" style="%s">` +
		`<div><div>%s%s</div></div>` +
		`</div></div></div>`

	// forwardQuoteHTMLFmt is the outer header-block structure for forward (outerID, style, innerID, sep, meta, body).
	forwardQuoteHTMLFmt = `<div id="%s" class="` + draftpkg.QuoteWrapperClass + `"><div data-html-block="quote" data-mail-html-ignore="">` +
		`<div class="adit-html-block adit-html-block--header" style="%s">` +
		`<div id="%s">%s%s%s</div>` +
		`</div></div></div>`
)

// genID returns an element id of the form "<prefix>XXXXXX"
// where XXXXXX is 6 random alphanumeric characters.
func genID(prefix string) string {
	b := make([]byte, 6)
	for i := range b {
		b[i] = quoteIDChars[rand.Intn(len(quoteIDChars))]
	}
	return prefix + string(b)
}

// detectSubjectLang returns "zh" if the subject contains CJK characters, "en" otherwise.
func detectSubjectLang(subject string) string {
	for _, r := range subject {
		if (r >= '\u4e00' && r <= '\u9fff') || // CJK Unified Ideographs
			(r >= '\u3400' && r <= '\u4dbf') || // CJK Extension A
			(r >= '\uf900' && r <= '\ufaff') || // CJK Compatibility Ideographs
			(r >= '\u3040' && r <= '\u30ff') { // Hiragana + Katakana
			return "zh"
		}
	}
	return "en"
}

type quoteMetaLabelSet struct {
	From          string
	Date          string
	Subject       string
	To            string
	Cc            string
	Separator     string // plaintext forward separator line
	Colon         string // "：" for Chinese, ": " for English (used in plaintext)
	ReplyPrefix   string // subject prefix for reply, e.g. "Re: " or "回复："
	ForwardPrefix string // subject prefix for forward, e.g. "Fwd: " or "转发："
}

// quoteMetaLabels returns the label set appropriate for the given subject language.
func quoteMetaLabels(subject string) quoteMetaLabelSet {
	if detectSubjectLang(subject) == "zh" {
		return quoteMetaLabelSet{
			From:          "发件人",
			Date:          "时间",
			Subject:       "主题",
			To:            "收件人",
			Cc:            "抄送",
			Separator:     "--------- 转发消息 ---------",
			Colon:         "：",
			ReplyPrefix:   "回复：",
			ForwardPrefix: "转发：",
		}
	}
	return quoteMetaLabelSet{
		From:          "From",
		Date:          "Date",
		Subject:       "Subject",
		To:            "To",
		Cc:            "Cc",
		Separator:     "---------- Forwarded message ---------",
		Colon:         ": ",
		ReplyPrefix:   "Re: ",
		ForwardPrefix: "Fwd: ",
	}
}

// buildAddressAnchor renders an email address as a mailto hyperlink (<a> only).
// The href uses URL encoding (RFC 6068) to prevent mailto: parameter injection;
// the display text and data attribute use HTML entity encoding.
func buildAddressAnchor(addr string) string {
	urlEncoded := url.PathEscape(addr)
	displayText := htmlEscape(addr)
	return fmt.Sprintf(addressAnchorFmt, displayText, urlEncoded, displayText)
}

// buildAddressHTML renders a single address.
//
//	With name:    "Name"&lt;<a>addr</a>&gt;
//	Without name: &lt;<a>addr</a>&gt;
func buildAddressHTML(name, addr string) string {
	anchor := buildAddressAnchor(addr)
	if name != "" {
		return fmt.Sprintf(`"%s"&lt;%s&gt;`, htmlEscape(name), anchor)
	}
	return `&lt;` + anchor + `&gt;`
}

// buildAddressPairListHTML renders a list of name+email pairs.
// Each address is wrapped in its own <span>.
func buildAddressPairListHTML(pairs []mailAddressPair) string {
	if len(pairs) == 0 {
		return ""
	}
	items := make([]string, 0, len(pairs))
	for _, p := range pairs {
		items = append(items, `<span>`+buildAddressHTML(p.Name, p.Email)+`</span>`)
	}
	return strings.Join(items, ", ")
}

// buildAddressListHTML renders a list of email-only addresses.
// Kept as a fallback for contexts where display names are unavailable.
func buildAddressListHTML(addrs []string) string {
	if len(addrs) == 0 {
		return ""
	}
	items := make([]string, 0, len(addrs))
	for _, addr := range addrs {
		items = append(items, `<span>`+buildAddressHTML("", addr)+`</span>`)
	}
	return strings.Join(items, ", ")
}

// buildMetaRow renders a single labeled metadata row.
// The entire content (label + value) is wrapped in <span style="">.
func buildMetaRow(label, content string) string {
	return fmt.Sprintf(metaRowFmt, htmlEscape(label), content)
}

// buildReplyMetaWrapper wraps meta rows for reply/reply-all quote blocks.
func buildReplyMetaWrapper(inner string) string {
	return fmt.Sprintf(replyMetaWrapperFmt, replyMetaMargin, metaBlockStyle, inner)
}

// buildForwardMetaWrapper wraps meta rows for forward quote blocks.
// Uses adit-html-block__header class and margin-top: 2px (not 24px).
func buildForwardMetaWrapper(inner string) string {
	return fmt.Sprintf(forwardMetaWrapperFmt, genID("lark-mail-meta-cli"), forwardMetaMargin, metaBlockStyle, inner)
}

// buildMetaRows assembles the inner HTML rows (From/Date/Subject/To/Cc) shared by
// both reply and forward quote blocks.
func buildMetaRows(orig *originalMessage) string {
	labels := quoteMetaLabels(orig.subject)
	var rows strings.Builder
	rows.WriteString(buildMetaRow(labels.From, buildAddressHTML(orig.headFromName, orig.headFrom)))
	if orig.headDate != "" {
		rows.WriteString(buildMetaRow(labels.Date, htmlEscape(orig.headDate)))
	}
	if orig.subject != "" {
		rows.WriteString(buildMetaRow(labels.Subject, htmlEscape(orig.subject)))
	}
	if len(orig.toAddressesFull) > 0 {
		rows.WriteString(buildMetaRow(labels.To, buildAddressPairListHTML(orig.toAddressesFull)))
	} else if len(orig.toAddresses) > 0 {
		rows.WriteString(buildMetaRow(labels.To, buildAddressListHTML(orig.toAddresses)))
	}
	if len(orig.ccAddressesFull) > 0 {
		rows.WriteString(buildMetaRow(labels.Cc, buildAddressPairListHTML(orig.ccAddressesFull)))
	} else if len(orig.ccAddresses) > 0 {
		rows.WriteString(buildMetaRow(labels.Cc, buildAddressListHTML(orig.ccAddresses)))
	}
	return rows.String()
}

// buildReplyPrefixHTML constructs the metadata prefix block for reply emails.
// Rendered as adit-html-block__attr with From/Date/Subject/To/Cc fields.
func buildReplyPrefixHTML(orig *originalMessage) string {
	return buildReplyMetaWrapper(buildMetaRows(orig))
}

// buildBodyDiv wraps the user-authored body content in a styled <div>.
// If isHTML is true, content is embedded as-is; otherwise it is HTML-escaped.
func buildBodyDiv(content string, isHTML bool) string {
	if content == "" {
		return ""
	}
	var inner string
	if isHTML {
		inner = content
	} else {
		inner = strings.ReplaceAll(htmlEscape(content), "\n", "<br>")
	}
	return fmt.Sprintf(`<div style="%s">%s</div>`, bodyDivStyle, inner)
}

// buildReplyQuoteHTML builds the collapsed quote block for reply/reply-all emails.
// Returns empty string when there is no content to quote.
func buildReplyQuoteHTML(orig *originalMessage) string {
	if orig.bodyRaw == "" && orig.headFrom == "" {
		return ""
	}
	prefixHTML := buildReplyPrefixHTML(orig)
	bodyHTML := orig.bodyRaw
	if bodyHTML != "" && !bodyIsHTML(bodyHTML) {
		bodyHTML = fmt.Sprintf(plainTextBodyFmt, htmlEscape(bodyHTML))
	}
	var bodyPart string
	if bodyHTML != "" {
		bodyPart = `<div>` + bodyHTML + `</div>`
	}
	return fmt.Sprintf(replyQuoteHTMLFmt, quoteBlockBorderStyle, prefixHTML, bodyPart)
}

// buildForwardSeparatorHTML builds the separator line div placed before the meta block.
func buildForwardSeparatorHTML(orig *originalMessage) string {
	labels := quoteMetaLabels(orig.subject)
	return fmt.Sprintf(separatorDivFmt, separatorStyle, htmlEscape(labels.Separator))
}

// buildForwardMetaHTML constructs the meta block (From/Date/Subject/To/Cc) for forwarded emails.
func buildForwardMetaHTML(orig *originalMessage) string {
	return buildForwardMetaWrapper(buildMetaRows(orig))
}

// buildForwardQuoteHTML builds the header quote block for forwarded emails.
// The separator div is placed outside the meta wrapper, matching the Lark client structure.
func buildForwardQuoteHTML(orig *originalMessage) string {
	separatorHTML := buildForwardSeparatorHTML(orig)
	metaHTML := buildForwardMetaHTML(orig)
	bodyHTML := orig.bodyRaw
	if bodyHTML != "" && !bodyIsHTML(bodyHTML) {
		bodyHTML = fmt.Sprintf(plainTextBodyFmt, htmlEscape(bodyHTML))
	}
	var bodyPart string
	if bodyHTML != "" {
		bodyPart = `<div>` + bodyHTML + `</div>`
	}
	return fmt.Sprintf(forwardQuoteHTMLFmt, genID("lark-mail-quote-cli"), quoteBlockBorderStyle, genID("lark-mail-quote-cli"), separatorHTML, metaHTML, bodyPart)
}

// zhWeekdays maps time.Weekday to Chinese weekday names in Lark's format.
var zhWeekdays = [7]string{"周日", "周一", "周二", "周三", "周四", "周五", "周六"}

// formatMailDate formats a Unix millisecond timestamp for use in quote blocks.
// lang is the detected language ("zh" or "en", from detectSubjectLang).
// The time is rendered in the system local timezone:
//   - "zh": "2006年1月2日 (周X) 15:04"  (matches Feishu native client)
//   - "en": "Mon, 02 Jan 2006 15:04 MST"
func formatMailDate(ms int64, lang string) string {
	t := time.UnixMilli(ms).Local()
	if lang == "zh" {
		return fmt.Sprintf("%s (%s) %s",
			t.Format("2006年1月2日"),
			zhWeekdays[t.Weekday()],
			t.Format("15:04"))
	}
	return t.Format("Mon, 02 Jan 2006 15:04 MST")
}

// htmlTagRe matches known HTML tags followed by a tag-terminating character
// (whitespace, >, or />). This avoids false positives like "<brief.pdf>" or
// "price < 100". The tag list follows the Chromium/WHATWG MIME sniffing approach
// with additional common tags for email content.
var htmlTagRe = regexp.MustCompile(
	`(?i)<(?:` +
		`!doctype\s+html|!--|` +
		`html|head|body|div|p|br|span|a|b|i|em|strong|` +
		`h[1-6]|ul|ol|li|table|tr|td|th|img|font|style|script|` +
		`iframe|title|form|input|select|textarea|button|label|` +
		`blockquote|pre|code|hr|section|article|header|footer|nav|main` +
		`)[\s/>]`)

// bodyIsHTML reports whether s appears to contain HTML markup.
func bodyIsHTML(s string) bool {
	if !strings.Contains(s, "<") {
		return false
	}
	return htmlTagRe.MatchString(s)
}

// htmlEscape escapes the five standard XML/HTML special characters.
func htmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, `"`, "&quot;")
	s = strings.ReplaceAll(s, "'", "&#39;")
	return s
}

// stripHTMLForQuote converts an HTML body to plain text suitable for quoted replies.
func stripHTMLForQuote(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	i := 0
	for i < len(s) {
		if s[i] != '<' {
			b.WriteByte(s[i])
			i++
			continue
		}
		end := strings.IndexByte(s[i:], '>')
		if end < 0 {
			b.WriteString(s[i:])
			break
		}
		tag := strings.ToLower(strings.TrimSpace(s[i+1 : i+end]))
		i += end + 1

		fields := strings.Fields(tag)
		if len(fields) > 0 && (fields[0] == "script" || fields[0] == "style") {
			closeTag := "</" + fields[0] + ">"
			if idx := strings.Index(strings.ToLower(s[i:]), closeTag); idx >= 0 {
				i += idx + len(closeTag)
			}
			continue
		}

		switch {
		case tag == "br" || tag == "br/" || tag == "br /":
			b.WriteByte('\n')
		case strings.HasPrefix(tag, "/p") || strings.HasPrefix(tag, "/div") ||
			strings.HasPrefix(tag, "/tr") || tag == "/h1" || tag == "/h2" ||
			tag == "/h3" || tag == "/h4" || tag == "/h5" || tag == "/h6" ||
			tag == "/li":
			b.WriteByte('\n')
		}
	}

	result := b.String()
	result = strings.ReplaceAll(result, "&amp;", "&")
	result = strings.ReplaceAll(result, "&lt;", "<")
	result = strings.ReplaceAll(result, "&gt;", ">")
	result = strings.ReplaceAll(result, "&quot;", `"`)
	result = strings.ReplaceAll(result, "&#39;", "'")
	result = strings.ReplaceAll(result, "&nbsp;", " ")

	for strings.Contains(result, "\n\n\n") {
		result = strings.ReplaceAll(result, "\n\n\n", "\n\n")
	}
	return strings.TrimSpace(result)
}

// stripLargeAttachmentCard removes the large attachment HTML card from
// orig.bodyRaw and returns the extracted card HTML (empty if none found).
// Forward uses the returned card to re-insert it into the body area;
// reply/reply-all discards the return value so the card doesn't appear
// in the quoted block.
func stripLargeAttachmentCard(orig *originalMessage) string {
	if !draftpkg.HTMLContainsLargeAttachment(orig.bodyRaw) {
		return ""
	}
	bodyWithout, card, trailing := draftpkg.SplitAtLargeAttachment(orig.bodyRaw)
	orig.bodyRaw = bodyWithout + trailing
	return card
}

// quoteForReply formats the original message body as a quoted block.
// HTML replies use the Lark adit-html-block--collapsed structure;
// plain-text replies use the classic "> " prefix format with meta header.
func quoteForReply(orig *originalMessage, html bool) string {
	if html {
		return buildReplyQuoteHTML(orig)
	}

	// Plain-text path: meta header + "> " prefixed body lines.
	if orig.bodyRaw == "" && orig.headFrom == "" {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("\n\n")
	// Build meta header lines (From/Subject/Date/To/Cc), each prefixed with "> "
	sb.WriteString(buildReplyMetaPlainText(orig))
	// Blank line between meta and body
	sb.WriteString(">\n")
	text := stripHTMLForQuote(orig.bodyRaw)
	for _, line := range strings.Split(text, "\n") {
		sb.WriteString("> ")
		sb.WriteString(line)
		sb.WriteString("\n")
	}
	return sb.String()
}

// buildReplyMetaPlainText builds the meta header block for plain-text replies.
// Each line is prefixed with "> " and follows the same label/format logic as HTML replies.
func buildReplyMetaPlainText(orig *originalMessage) string {
	return buildPlainTextMetaRows(orig, "> ")
}

// buildPlainTextMetaRows builds the meta rows for plain-text output.
// linePrefix is prepended to each line (e.g., "> " for replies, "" for forwards).
// Field order matches buildMetaRows: From -> Date -> Subject -> To -> Cc.
func buildPlainTextMetaRows(orig *originalMessage, linePrefix string) string {
	labels := quoteMetaLabels(orig.subject)
	var sb strings.Builder

	// From
	if orig.headFrom != "" {
		from := buildPlainTextAddress(orig.headFromName, orig.headFrom)
		sb.WriteString(linePrefix)
		sb.WriteString(labels.From)
		sb.WriteString(labels.Colon)
		sb.WriteString(from)
		sb.WriteString("\n")
	}

	// Date
	if orig.headDate != "" {
		sb.WriteString(linePrefix)
		sb.WriteString(labels.Date)
		sb.WriteString(labels.Colon)
		sb.WriteString(orig.headDate)
		sb.WriteString("\n")
	}

	// Subject
	if orig.subject != "" {
		sb.WriteString(linePrefix)
		sb.WriteString(labels.Subject)
		sb.WriteString(labels.Colon)
		sb.WriteString(orig.subject)
		sb.WriteString("\n")
	}

	// To
	if len(orig.toAddressesFull) > 0 {
		sb.WriteString(linePrefix)
		sb.WriteString(labels.To)
		sb.WriteString(labels.Colon)
		sb.WriteString(buildPlainTextAddressList(orig.toAddressesFull))
		sb.WriteString("\n")
	} else if len(orig.toAddresses) > 0 {
		sb.WriteString(linePrefix)
		sb.WriteString(labels.To)
		sb.WriteString(labels.Colon)
		sb.WriteString(strings.Join(orig.toAddresses, ", "))
		sb.WriteString("\n")
	}

	// Cc
	if len(orig.ccAddressesFull) > 0 {
		sb.WriteString(linePrefix)
		sb.WriteString(labels.Cc)
		sb.WriteString(labels.Colon)
		sb.WriteString(buildPlainTextAddressList(orig.ccAddressesFull))
		sb.WriteString("\n")
	} else if len(orig.ccAddresses) > 0 {
		sb.WriteString(linePrefix)
		sb.WriteString(labels.Cc)
		sb.WriteString(labels.Colon)
		sb.WriteString(strings.Join(orig.ccAddresses, ", "))
		sb.WriteString("\n")
	}

	return sb.String()
}

// buildPlainTextAddress formats a single address for plain-text output.
// With name:    "Name" <email>
// Without name: <email>
func buildPlainTextAddress(name, email string) string {
	if name != "" {
		return fmt.Sprintf(`"%s" <%s>`, name, email)
	}
	return fmt.Sprintf("<%s>", email)
}

// buildPlainTextAddressList formats a list of addresses for plain-text output.
func buildPlainTextAddressList(pairs []mailAddressPair) string {
	if len(pairs) == 0 {
		return ""
	}
	items := make([]string, 0, len(pairs))
	for _, p := range pairs {
		items = append(items, buildPlainTextAddress(p.Name, p.Email))
	}
	return strings.Join(items, ", ")
}

// removeDuplicateSubjectPrefix strips leading reply/forward prefixes (case-insensitive)
// to prevent accumulation on chained replies/forwards (e.g. "Re: Re: Re: topic").
// Handles both ASCII prefixes (Re:, Fwd:, Fw:) and Chinese prefixes (回复：, 转发：).
func removeDuplicateSubjectPrefix(subject string) string {
	for {
		trimmed := strings.TrimSpace(subject)
		lower := strings.ToLower(trimmed)
		switch {
		case strings.HasPrefix(lower, "re:"):
			subject = strings.TrimSpace(trimmed[3:])
		case strings.HasPrefix(lower, "fwd:"):
			subject = strings.TrimSpace(trimmed[4:])
		case strings.HasPrefix(lower, "fw:"):
			subject = strings.TrimSpace(trimmed[3:])
		case strings.HasPrefix(trimmed, "回复："):
			subject = strings.TrimSpace(trimmed[len("回复："):])
		case strings.HasPrefix(trimmed, "转发："):
			subject = strings.TrimSpace(trimmed[len("转发："):])
		default:
			return trimmed
		}
	}
}

// buildReplySubject prepends the language-appropriate reply prefix once,
// stripping any existing prefixes to prevent accumulation on chained replies.
func buildReplySubject(original string) string {
	return quoteMetaLabels(original).ReplyPrefix + removeDuplicateSubjectPrefix(original)
}

// buildForwardSubject prepends the language-appropriate forward prefix once,
// stripping any existing prefixes to prevent accumulation on chained forwards.
func buildForwardSubject(original string) string {
	return quoteMetaLabels(original).ForwardPrefix + removeDuplicateSubjectPrefix(original)
}

// buildForwardedMessage formats the original message as a plain-text forwarding block.
func buildForwardedMessage(orig *originalMessage, body string) string {
	labels := quoteMetaLabels(orig.subject)
	var sb strings.Builder
	if body != "" {
		sb.WriteString(body)
		sb.WriteString("\n\n")
	}
	sb.WriteString(labels.Separator + "\n")
	sb.WriteString(buildPlainTextMetaRows(orig, ""))
	sb.WriteString("\n")
	if orig.bodyRaw != "" {
		sb.WriteString(stripHTMLForQuote(orig.bodyRaw))
	}
	return sb.String()
}
