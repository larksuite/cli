// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"bytes"
	"context"
	stdhtml "html"
	"strings"

	"github.com/larksuite/cli/shortcuts/common"
	"github.com/larksuite/cli/shortcuts/mail/signature"
	xhtml "golang.org/x/net/html"
)

var mailSendSignatureFlag = common.Flag{
	Name: "signature-id",
	Desc: "Optional. Signature ID to append. Overrides the default send signature for this email.",
}

var mailSendNoSignatureFlag = common.Flag{
	Name: "no-signature",
	Type: "bool",
	Desc: "Skip the default send signature for this email.",
}

type mailSendSignatureOptions struct {
	SignatureID string
	NoSignature bool
}

func validateMailSendSignatureFlags(runtime *common.RuntimeContext) error {
	if runtime.Bool("no-signature") && strings.TrimSpace(runtime.Str("signature-id")) != "" {
		return mailValidationError("--signature-id and --no-signature are mutually exclusive").
			WithParams(
				mailInvalidParam("--signature-id", "mutually exclusive with --no-signature"),
				mailInvalidParam("--no-signature", "mutually exclusive with --signature-id"),
			)
	}
	return nil
}

func resolveMailSendComposeSignature(ctx context.Context, runtime *common.RuntimeContext, mailboxID, senderEmail string, opts mailSendSignatureOptions) (*signatureResult, error) {
	if opts.NoSignature {
		return nil, nil
	}

	signatureID := strings.TrimSpace(opts.SignatureID)
	if signatureID != "" {
		return resolveSignature(ctx, runtime, mailboxID, signatureID, senderEmail)
	}

	resp, err := signature.ListAll(runtime, mailboxID)
	if err != nil {
		return nil, mailAppendProblemHint(
			mailDecorateProblemMessage(err, "failed to look up default send signature"),
			"pass --no-signature to send without a signature",
		)
	}

	defaultID := selectMailSendDefaultSignatureID(resp.Usages, senderEmail)
	if mailSendSignatureIDIsEmpty(defaultID) {
		return nil, nil
	}
	if !mailSendSignatureExists(resp.Signatures, defaultID) {
		return nil, mailValidationError("default send signature %q was configured for %q but was not returned by settings/signatures", defaultID, mailSendSignatureSenderLabel(senderEmail)).
			WithHint("run `lark-cli mail +signature` to inspect signatures, or pass --no-signature to send without a signature")
	}
	return resolveSignature(ctx, runtime, mailboxID, defaultID, senderEmail)
}

func selectMailSendDefaultSignatureID(usages []signature.SignatureUsage, senderEmail string) string {
	senderEmail = strings.TrimSpace(senderEmail)
	if senderEmail == "" {
		if len(usages) != 1 {
			return ""
		}
		return strings.TrimSpace(usages[0].SendMailSignatureID)
	}
	for _, usage := range usages {
		if strings.EqualFold(strings.TrimSpace(usage.EmailAddress), senderEmail) {
			return strings.TrimSpace(usage.SendMailSignatureID)
		}
	}
	return ""
}

func mailSendSignatureExists(signatures []signature.Signature, signatureID string) bool {
	for _, sig := range signatures {
		if strings.TrimSpace(sig.ID) == signatureID {
			return true
		}
	}
	return false
}

func mailSendSignatureIDIsEmpty(signatureID string) bool {
	signatureID = strings.TrimSpace(signatureID)
	return signatureID == "" || signatureID == "0"
}

func mailSendSignatureSenderLabel(senderEmail string) string {
	if strings.TrimSpace(senderEmail) == "" {
		return "the resolved sender"
	}
	return senderEmail
}

func appendMailSendPlainTextSignature(body string, sig *signatureResult, lang string) string {
	if sig == nil {
		return body
	}
	signatureText := renderMailSendSignaturePlainText(sig.RenderedContent, lang)
	if strings.TrimSpace(signatureText) == "" {
		return body
	}
	body = strings.TrimRight(body, "\r\n")
	if strings.TrimSpace(body) == "" {
		return signatureText
	}
	return body + "\n\n" + signatureText
}

func renderMailSendSignaturePlainText(content, lang string) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return ""
	}
	doc, err := xhtml.Parse(strings.NewReader(content))
	if err != nil {
		return strings.TrimSpace(stdhtml.UnescapeString(content))
	}
	var buf bytes.Buffer
	renderMailSendSignatureText(&buf, doc, mailSendSignatureImagePlaceholder(lang))
	return compactMailSendSignaturePlainText(buf.String())
}

func renderMailSendSignatureText(buf *bytes.Buffer, root *xhtml.Node, imagePlaceholder string) {
	if root == nil {
		return
	}

	type pendingEntry struct {
		node  *xhtml.Node
		child *xhtml.Node
	}
	stack := []pendingEntry{{node: root, child: root.FirstChild}}

	for len(stack) > 0 {
		top := &stack[len(stack)-1]
		if top.child == nil {
			if mailSendSignatureBlockNode(top.node) {
				writeMailSendSignatureBreak(buf)
			}
			stack = stack[:len(stack)-1]
			continue
		}

		node := top.child
		top.child = node.NextSibling

		if node.Type == xhtml.ElementNode {
			tag := strings.ToLower(node.Data)
			if mailSendSignatureSkipsText(tag) {
				continue
			}
			switch tag {
			case "br":
				writeMailSendSignatureBreak(buf)
				continue
			case "img":
				appendMailSendSignatureText(buf, imagePlaceholder)
				continue
			}
			if mailSendSignatureBlockTag(tag) {
				writeMailSendSignatureBreak(buf)
			}
		}
		if node.Type == xhtml.TextNode {
			appendMailSendSignatureTextNode(buf, node.Data, imagePlaceholder)
		}
		if node.FirstChild != nil {
			stack = append(stack, pendingEntry{node: node, child: node.FirstChild})
		}
	}
}

func appendMailSendSignatureTextNode(buf *bytes.Buffer, raw, imagePlaceholder string) {
	unescaped := stdhtml.UnescapeString(raw)
	if !mailSendSignatureLooksLikeHTMLFragment(unescaped) {
		appendMailSendSignatureText(buf, unescaped)
		return
	}

	doc, err := xhtml.Parse(strings.NewReader(unescaped))
	if err != nil {
		appendMailSendSignatureText(buf, unescaped)
		return
	}

	var nested bytes.Buffer
	renderMailSendSignatureText(&nested, doc, imagePlaceholder)
	text := compactMailSendSignaturePlainText(nested.String())
	if text == "" {
		return
	}
	for i, line := range strings.Split(text, "\n") {
		if i > 0 {
			writeMailSendSignatureBreak(buf)
		}
		appendMailSendSignatureText(buf, line)
	}
}

func appendMailSendSignatureText(buf *bytes.Buffer, raw string) {
	parts := strings.Fields(stdhtml.UnescapeString(raw))
	if len(parts) == 0 {
		return
	}
	text := strings.Join(parts, " ")
	if buf.Len() > 0 {
		last := buf.Bytes()[buf.Len()-1]
		if last != '\n' && last != ' ' {
			buf.WriteByte(' ')
		}
	}
	buf.WriteString(text)
}

func writeMailSendSignatureBreak(buf *bytes.Buffer) {
	for buf.Len() > 0 {
		last := buf.Bytes()[buf.Len()-1]
		if last != ' ' && last != '\t' && last != '\r' {
			break
		}
		buf.Truncate(buf.Len() - 1)
	}
	if buf.Len() == 0 {
		return
	}
	if buf.Bytes()[buf.Len()-1] != '\n' {
		buf.WriteByte('\n')
	}
}

func compactMailSendSignaturePlainText(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	lines := strings.Split(text, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		out = append(out, line)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

func mailSendSignatureImagePlaceholder(lang string) string {
	if strings.HasPrefix(strings.ToLower(lang), "zh") {
		return "[图片]"
	}
	return "[image]"
}

func mailSendSignatureLooksLikeHTMLFragment(raw string) bool {
	lower := strings.ToLower(raw)
	for _, tag := range mailSendSignatureHTMLFragmentTags {
		if mailSendSignatureContainsHTMLTag(lower, tag) {
			return true
		}
	}
	return false
}

func mailSendSignatureContainsHTMLTag(lower, tag string) bool {
	for _, prefix := range []string{"<" + tag, "</" + tag} {
		start := 0
		for {
			idx := strings.Index(lower[start:], prefix)
			if idx < 0 {
				break
			}
			end := start + idx + len(prefix)
			if end >= len(lower) || mailSendSignatureTagBoundary(lower[end]) {
				return true
			}
			start += idx + 1
		}
	}
	return false
}

func mailSendSignatureTagBoundary(ch byte) bool {
	switch ch {
	case ' ', '\t', '\n', '\r', '>', '/', ':':
		return true
	default:
		return false
	}
}

var mailSendSignatureHTMLFragmentTags = []string{
	"a", "address", "article", "aside", "b", "blockquote", "body", "br", "div", "em", "font", "footer", "h1", "h2", "h3", "h4", "h5", "h6", "header", "html", "i", "img", "li", "ol", "p", "section", "span", "strong", "table", "tbody", "td", "tfoot", "th", "thead", "tr", "ul",
}

func mailSendSignatureSkipsText(tag string) bool {
	switch tag {
	case "head", "script", "style", "title", "meta", "link", "noscript":
		return true
	default:
		return false
	}
}

func mailSendSignatureBlockNode(node *xhtml.Node) bool {
	return node != nil && node.Type == xhtml.ElementNode && mailSendSignatureBlockTag(strings.ToLower(node.Data))
}

func mailSendSignatureBlockTag(tag string) bool {
	switch tag {
	case "address", "article", "aside", "blockquote", "dd", "div", "dl", "dt", "fieldset", "figcaption", "figure", "footer", "form", "h1", "h2", "h3", "h4", "h5", "h6", "header", "hr", "li", "main", "nav", "ol", "p", "pre", "section", "table", "tbody", "tfoot", "thead", "tr", "ul":
		return true
	default:
		return false
	}
}
