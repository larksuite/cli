// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
	draftpkg "github.com/larksuite/cli/shortcuts/mail/draft"
	"github.com/larksuite/cli/shortcuts/mail/emlbuilder"
	"github.com/larksuite/cli/shortcuts/mail/signature"
	xhtml "golang.org/x/net/html"
)

// signatureFlag is the common flag definition for --signature-id, shared by all compose shortcuts.
var signatureFlag = common.Flag{
	Name: "signature-id",
	Desc: "Optional. Signature ID to append after body content. Run `mail +signature` to list available signatures.",
}

var noSignatureFlag = common.Flag{
	Name: "no-signature",
	Type: "bool",
	Desc: "Skip both explicit and default signature lookup for this compose action.",
}

// signatureResult holds the pre-processed signature data ready for HTML injection.
type signatureResult struct {
	ID              string
	RenderedContent string
	Images          []draftpkg.SignatureImage
}

// resolveSignature fetches, interpolates, and downloads images for a signature.
// fromEmail is the --from address (may be an alias); used to match the correct
// sender identity for template interpolation. Pass "" to use the primary address.
func resolveSignature(ctx context.Context, runtime *common.RuntimeContext, mailboxID, signatureID, fromEmail string) (*signatureResult, error) {
	if signatureID == "" {
		return nil, nil
	}

	sig, err := signature.Get(runtime, mailboxID, signatureID)
	if err != nil {
		return nil, err
	}

	// Resolve sender info for template interpolation.
	lang := resolveLang(runtime)
	senderName, senderEmail := resolveSenderInfo(runtime, mailboxID, fromEmail)
	rendered := signature.InterpolateTemplate(sig, lang, senderName, senderEmail)

	// Download signature inline images. The file_key field contains a
	// direct download URL provided by the mail backend.
	var images []draftpkg.SignatureImage
	for _, img := range sig.Images {
		if img.DownloadURL == "" || img.CID == "" {
			continue
		}
		data, ct, err := downloadSignatureImage(runtime, img.DownloadURL, img.ImageName)
		if err != nil {
			return nil, mailDecorateProblemMessage(err, "failed to download signature image %s", img.ImageName)
		}
		images = append(images, draftpkg.SignatureImage{
			CID:         img.CID,
			ContentType: ct,
			FileName:    img.ImageName,
			Data:        data,
		})
	}

	return &signatureResult{
		ID:              sig.ID,
		RenderedContent: rendered,
		Images:          images,
	}, nil
}

// resolveComposeSignature applies the +send signature policy order:
// --no-signature → explicit --signature-id → auto default signature → no signature.
func resolveComposeSignature(ctx context.Context, runtime *common.RuntimeContext, mailboxID, senderEmail string) (*signatureResult, error) {
	if runtime.Bool("no-signature") {
		return nil, nil
	}
	if signatureID := runtime.Str("signature-id"); signatureID != "" {
		return resolveSignature(ctx, runtime, mailboxID, signatureID, senderEmail)
	}
	defaultID, err := resolveDefaultSendSignature(runtime, mailboxID, senderEmail)
	if err != nil {
		return nil, err
	}
	if defaultID == "" {
		return nil, nil
	}
	return resolveSignature(ctx, runtime, mailboxID, defaultID, senderEmail)
}

// resolveDefaultSendSignature returns the default send signature ID that matches
// the concrete sender email. Empty / "0" usage IDs mean "no default".
func resolveDefaultSendSignature(runtime *common.RuntimeContext, mailboxID, senderEmail string) (string, error) {
	resp, err := signature.ListAll(runtime, mailboxID)
	if err != nil {
		return "", err
	}
	senderEmail = strings.TrimSpace(senderEmail)
	if senderEmail == "" {
		return "", nil
	}
	for _, usage := range resp.Usages {
		if !strings.EqualFold(strings.TrimSpace(usage.EmailAddress), senderEmail) {
			continue
		}
		id := strings.TrimSpace(usage.SendMailSignatureID)
		if id == "" || id == "0" {
			return "", nil
		}
		return id, nil
	}
	return "", nil
}

// injectSignatureIntoBody inserts signature HTML into the body, placing
// it right after the user-authored region and before any system-managed
// tail (large attachment card or quote block). Any existing signature is
// removed first. Returns the new full HTML body.
//
// Delegates to draftpkg.PlaceSignatureBeforeSystemTail for the actual
// placement, sharing a single source of truth with the edit-time
// insert_signature op so both paths yield identical structure.
func injectSignatureIntoBody(bodyHTML string, sig *signatureResult) string {
	if sig == nil {
		return bodyHTML
	}
	sigBlock := draftpkg.SignatureSpacing() + draftpkg.BuildSignatureHTML(sig.ID, sig.RenderedContent)
	return draftpkg.PlaceSignatureBeforeSystemTail(bodyHTML, sigBlock)
}

// addSignatureImagesToBuilder adds signature inline images to the EML builder.
func addSignatureImagesToBuilder(bld emlbuilder.Builder, sig *signatureResult) emlbuilder.Builder {
	if sig == nil {
		return bld
	}
	for _, img := range sig.Images {
		cid := normalizeInlineCID(img.CID)
		if cid == "" {
			continue
		}
		bld = bld.AddInline(img.Data, img.ContentType, img.FileName, cid)
	}
	return bld
}

// resolveSenderInfo fetches send_as addresses and returns the name/email
// for signature interpolation. If fromEmail is non-empty, it matches
// that address in the sendable list (for alias/send_as scenarios);
// otherwise falls back to the first (primary) address.
func resolveSenderInfo(runtime *common.RuntimeContext, mailboxID, fromEmail string) (name, email string) {
	data, err := runtime.CallAPITyped("GET", mailboxPath(mailboxID, "settings", "send_as"), nil, nil)
	if err != nil {
		return "", ""
	}
	addrs, ok := data["sendable_addresses"].([]interface{})
	if !ok || len(addrs) == 0 {
		return "", ""
	}
	// If fromEmail is specified, find the matching address.
	if fromEmail != "" {
		for _, a := range addrs {
			m, ok := a.(map[string]interface{})
			if !ok {
				continue
			}
			e, _ := m["email_address"].(string)
			if strings.EqualFold(e, fromEmail) {
				n, _ := m["name"].(string)
				return n, e
			}
		}
	}
	// Fall back to the first sendable address (primary).
	first, ok := addrs[0].(map[string]interface{})
	if !ok {
		return "", ""
	}
	n, _ := first["name"].(string)
	e, _ := first["email_address"].(string)
	return n, e
}

// downloadSignatureImage downloads a signature image by its direct URL.
// Security: enforces https, does not send Bearer token (URL is pre-signed),
// uses context timeout, and limits response size. Aligned with
// downloadAttachmentContent in helpers.go.
func downloadSignatureImage(runtime *common.RuntimeContext, downloadURL, filename string) ([]byte, string, error) {
	u, err := url.Parse(downloadURL)
	if err != nil {
		return nil, "", mailInvalidResponseError("signature image download: invalid URL: %v", err).WithCause(err)
	}
	if u.Scheme != "https" {
		return nil, "", mailInvalidResponseError("signature image download: URL must use https (got %q)", u.Scheme)
	}
	if u.Host == "" {
		return nil, "", mailInvalidResponseError("signature image download: URL has no host")
	}

	httpClient, err := runtime.Factory.HttpClient()
	if err != nil {
		return nil, "", errs.NewInternalError(errs.SubtypeSDKError, "signature image download: %v", err).WithCause(err)
	}
	ctx, cancel := context.WithTimeout(runtime.Ctx(), 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return nil, "", errs.NewInternalError(errs.SubtypeSDKError, "signature image download: %v", err).WithCause(err)
	}
	// Do NOT send Authorization: the download URL is pre-signed.

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, "", errs.NewNetworkError(errs.SubtypeNetworkTransport, "signature image download: %v", err).WithCause(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		if resp.StatusCode >= 500 {
			return nil, "", errs.NewNetworkError(errs.SubtypeNetworkServer, "signature image download: HTTP %d: %s", resp.StatusCode, string(body)).
				WithCode(resp.StatusCode).
				WithRetryable()
		}
		subtype := errs.SubtypeUnknown
		if resp.StatusCode == http.StatusNotFound {
			subtype = errs.SubtypeNotFound
		}
		return nil, "", errs.NewAPIError(subtype, "signature image download: HTTP %d: %s", resp.StatusCode, string(body)).WithCode(resp.StatusCode)
	}

	const maxSize = 10 * 1024 * 1024
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxSize+1))
	if err != nil {
		return nil, "", errs.NewNetworkError(errs.SubtypeNetworkTransport, "signature image download: read body: %v", err).WithCause(err)
	}
	if len(data) > maxSize {
		return nil, "", mailFailedPreconditionError("signature image download: file exceeds 10MB limit")
	}

	ct := resp.Header.Get("Content-Type")
	if ct == "" || ct == "application/octet-stream" {
		ct = contentTypeFromFilename(filename)
	}

	return data, ct, nil
}

func contentTypeFromFilename(name string) string {
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".svg":
		return "image/svg+xml"
	case ".bmp":
		return "image/bmp"
	default:
		return "application/octet-stream"
	}
}

// signatureCIDs returns the CID list from a signatureResult, for inline CID validation.
func signatureCIDs(sig *signatureResult) []string {
	if sig == nil {
		return nil
	}
	cids := make([]string, 0, len(sig.Images))
	for _, img := range sig.Images {
		cid := normalizeInlineCID(img.CID)
		if cid != "" {
			cids = append(cids, cid)
		}
	}
	return cids
}

// validateSignatureFlags rejects the only raw-input conflict in +send's
// signature policy: explicit opt-out and explicit signature ID at the same time.
func validateSignatureFlags(noSignature bool, signatureID string) error {
	if noSignature && signatureID != "" {
		return mailValidationError("--no-signature and --signature-id are mutually exclusive").
			WithParams(
				mailInvalidParam("--no-signature", "mutually exclusive with --signature-id"),
				mailInvalidParam("--signature-id", "mutually exclusive with --no-signature"),
			)
	}
	return nil
}

// validateSignatureWithPlainText remains for reply/draft siblings that still
// intentionally reject plain-text + signature-id outside this sprint's scope.
func validateSignatureWithPlainText(plainText bool, signatureID string) error {
	if plainText && signatureID != "" {
		return mailValidationError("--plain-text and --signature-id are mutually exclusive: signatures require HTML mode").
			WithParams(
				mailInvalidParam("--plain-text", "mutually exclusive with --signature-id"),
				mailInvalidParam("--signature-id", "requires HTML mode"),
			)
	}
	return nil
}

func appendPlainTextSignature(body, signatureText string) string {
	signatureText = strings.TrimSpace(signatureText)
	if signatureText == "" {
		return body
	}
	if strings.TrimSpace(body) == "" {
		return signatureText
	}
	return body + "\n\n" + signatureText
}

func renderPlainTextSignature(sig *signatureResult) string {
	if sig == nil {
		return ""
	}
	return plainTextSignatureFromHTML(sig.RenderedContent)
}

func plainTextSignatureFromHTML(raw string) string {
	doc, err := xhtml.Parse(strings.NewReader(raw))
	if err != nil {
		return strings.TrimSpace(raw)
	}
	var buf strings.Builder
	renderPlainTextNode(&buf, doc)
	return normalizePlainTextOutput(buf.String())
}

func renderPlainTextNode(buf *strings.Builder, n *xhtml.Node) {
	if n == nil || isSignatureNonTextTag(n) {
		return
	}
	if n.Type == xhtml.TextNode {
		appendPlainTextToken(buf, collapseSignatureWhitespace(n.Data))
		return
	}
	if n.Type == xhtml.ElementNode {
		switch strings.ToLower(n.Data) {
		case "img":
			return
		case "a":
			text := normalizePlainTextOutput(collectPlainTextNodeText(n.FirstChild))
			if text == "" {
				text = strings.TrimSpace(nodeAttr(n, "href"))
			}
			appendPlainTextToken(buf, text)
			return
		}
		if isSignatureBlockBoundary(n) {
			appendPlainTextLineBreak(buf)
		}
	}
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		renderPlainTextNode(buf, child)
	}
	if isSignatureBlockBoundary(n) {
		appendPlainTextLineBreak(buf)
	}
}

func collectPlainTextNodeText(n *xhtml.Node) string {
	var buf strings.Builder
	var walk func(*xhtml.Node)
	walk = func(node *xhtml.Node) {
		if node == nil || isSignatureNonTextTag(node) {
			return
		}
		if node.Type == xhtml.TextNode {
			appendPlainTextToken(&buf, collapseSignatureWhitespace(node.Data))
			return
		}
		if node.Type == xhtml.ElementNode && strings.EqualFold(node.Data, "img") {
			return
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(n)
	return buf.String()
}

func appendPlainTextToken(buf *strings.Builder, token string) {
	token = strings.TrimSpace(token)
	if token == "" {
		return
	}
	if buf.Len() > 0 {
		last := buf.String()[buf.Len()-1]
		if last != '\n' && last != ' ' {
			buf.WriteByte(' ')
		}
	}
	buf.WriteString(token)
}

func appendPlainTextLineBreak(buf *strings.Builder) {
	if buf.Len() == 0 {
		return
	}
	if buf.String()[buf.Len()-1] != '\n' {
		buf.WriteByte('\n')
	}
}

func normalizePlainTextOutput(raw string) string {
	lines := strings.Split(raw, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			out = append(out, line)
		}
	}
	return strings.Join(out, "\n")
}

func collapseSignatureWhitespace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func isSignatureNonTextTag(n *xhtml.Node) bool {
	if n == nil || n.Type != xhtml.ElementNode {
		return false
	}
	switch strings.ToLower(n.Data) {
	case "head", "meta", "script", "noscript", "style", "link", "title":
		return true
	default:
		return false
	}
}

func isSignatureBlockBoundary(n *xhtml.Node) bool {
	if n == nil || n.Type != xhtml.ElementNode {
		return false
	}
	switch strings.ToLower(n.Data) {
	case "address", "article", "aside", "blockquote", "br", "dd", "div", "dl", "dt",
		"figcaption", "figure", "footer", "form", "h1", "h2", "h3", "h4", "h5", "h6",
		"header", "hr", "li", "main", "nav", "ol", "p", "pre", "section", "table", "tr", "ul":
		return true
	default:
		return false
	}
}

func nodeAttr(n *xhtml.Node, key string) string {
	if n == nil {
		return ""
	}
	for _, attr := range n.Attr {
		if strings.EqualFold(attr.Key, key) {
			return attr.Val
		}
	}
	return ""
}
