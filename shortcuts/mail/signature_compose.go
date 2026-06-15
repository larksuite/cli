// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"context"
	stdhtml "html"
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
	nethtml "golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// signatureFlag is the common flag definition for --signature-id, shared by all compose shortcuts.
var signatureFlag = common.Flag{
	Name: "signature-id",
	Desc: "Optional. Signature ID to append after body content. Run `mail +signature` to list available signatures.",
}

// noSignatureFlag is send-only: skip automatic default signature lookup and injection.
var noSignatureFlag = common.Flag{
	Name: "no-signature",
	Type: "bool",
	Desc: "Skip auto-appending the default send signature.",
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
	return resolveSignatureWithImages(ctx, runtime, mailboxID, signatureID, fromEmail, true)
}

func resolveSignatureTextOnly(ctx context.Context, runtime *common.RuntimeContext, mailboxID, signatureID, fromEmail string) (*signatureResult, error) {
	return resolveSignatureWithImages(ctx, runtime, mailboxID, signatureID, fromEmail, false)
}

func resolveSignatureWithImages(ctx context.Context, runtime *common.RuntimeContext, mailboxID, signatureID, fromEmail string, includeImages bool) (*signatureResult, error) {
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
	if includeImages {
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
	}

	return &signatureResult{
		ID:              sig.ID,
		RenderedContent: rendered,
		Images:          images,
	}, nil
}

func resolveDefaultSendSignatureID(runtime *common.RuntimeContext, mailboxID, senderEmail string) (string, error) {
	resp, err := signature.ListAll(runtime, mailboxID)
	if err != nil {
		return "", err
	}
	candidates := []string{senderEmail}
	if strings.TrimSpace(senderEmail) == "" {
		if _, resolvedEmail := resolveSenderInfo(runtime, mailboxID, ""); resolvedEmail != "" {
			candidates = append(candidates, resolvedEmail)
		}
	}
	if mailboxID != "" && mailboxID != "me" {
		candidates = append(candidates, mailboxID)
	}
	return defaultSendSignatureIDFromUsages(resp.Usages, candidates...), nil
}

func defaultSendSignatureIDFromUsages(usages []signature.SignatureUsage, candidates ...string) string {
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		for _, usage := range usages {
			if !strings.EqualFold(strings.TrimSpace(usage.EmailAddress), candidate) {
				continue
			}
			id := strings.TrimSpace(usage.SendMailSignatureID)
			if id == "" || id == "0" {
				return ""
			}
			return id
		}
	}
	return ""
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

func appendPlainTextSignature(textBody string, sig *signatureResult) string {
	if sig == nil {
		return textBody
	}
	sigText := strings.TrimSpace(signatureHTMLToPlainText(sig.RenderedContent))
	if sigText == "" {
		return textBody
	}
	body := strings.TrimRight(textBody, " \t\r\n")
	if body == "" {
		return sigText
	}
	return body + "\n\n" + sigText
}

func signatureHTMLToPlainText(htmlText string) string {
	nodes, err := nethtml.ParseFragment(strings.NewReader(htmlText), &nethtml.Node{
		Type:     nethtml.ElementNode,
		DataAtom: atom.Body,
		Data:     "body",
	})
	if err != nil {
		return strings.TrimSpace(stdhtml.UnescapeString(htmlText))
	}
	var b strings.Builder
	for _, n := range nodes {
		appendHTMLNodeText(&b, n)
	}
	return normalizePlainSignatureText(b.String())
}

func appendHTMLNodeText(b *strings.Builder, n *nethtml.Node) {
	switch n.Type {
	case nethtml.TextNode:
		writePlainTextWords(b, stdhtml.UnescapeString(n.Data))
	case nethtml.ElementNode:
		switch strings.ToLower(n.Data) {
		case "br":
			writePlainTextNewline(b)
			return
		case "img":
			return
		case "script", "style", "head", "template", "noscript":
			return
		case "a":
			var child strings.Builder
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				appendHTMLNodeText(&child, c)
			}
			label := strings.TrimSpace(normalizePlainSignatureText(child.String()))
			href := attrValue(n, "href")
			if href != "" && label != "" && href != label {
				writePlainTextWords(b, label+" ("+href+")")
			} else if label != "" {
				writePlainTextWords(b, label)
			} else if href != "" {
				writePlainTextWords(b, href)
			}
			return
		case "p", "div", "section", "article", "header", "footer", "blockquote", "tr", "table", "ul", "ol":
			writePlainTextNewline(b)
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				appendHTMLNodeText(b, c)
			}
			writePlainTextNewline(b)
			return
		case "li":
			writePlainTextNewline(b)
			writePlainTextWords(b, "-")
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				appendHTMLNodeText(b, c)
			}
			writePlainTextNewline(b)
			return
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		appendHTMLNodeText(b, c)
	}
}

func writePlainTextWords(b *strings.Builder, text string) {
	words := strings.Join(strings.Fields(text), " ")
	if words == "" {
		return
	}
	current := b.String()
	if current != "" && !strings.HasSuffix(current, " ") && !strings.HasSuffix(current, "\n") {
		b.WriteByte(' ')
	}
	b.WriteString(words)
}

func writePlainTextNewline(b *strings.Builder) {
	current := b.String()
	if current == "" || strings.HasSuffix(current, "\n") {
		return
	}
	b.WriteByte('\n')
}

func normalizePlainSignatureText(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	lines := strings.Split(text, "\n")
	out := make([]string, 0, len(lines))
	blank := false
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			if len(out) > 0 && !blank {
				out = append(out, "")
				blank = true
			}
			continue
		}
		out = append(out, line)
		blank = false
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

func attrValue(n *nethtml.Node, key string) string {
	for _, attr := range n.Attr {
		if strings.EqualFold(attr.Key, key) {
			return strings.TrimSpace(attr.Val)
		}
	}
	return ""
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

// validateSignatureWithPlainText returns an error if both --plain-text and --signature-id are set.
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
