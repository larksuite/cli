// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"context"
	"fmt"
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
)

// signatureFlag is the common flag definition for --signature-id, shared by all compose shortcuts.
var signatureFlag = common.Flag{
	Name: "signature-id",
	Desc: "Optional. Signature ID to append after body content. Run `mail +signature` to list available signatures.",
}

// noSignatureFlag is shared by all 5 compose shortcuts.
var noSignatureFlag = common.Flag{
	Name: "no-signature",
	Type: "bool",
	Desc: "Skip automatic default signature insertion. Mutually exclusive with --signature-id.",
}

// validateNoSignatureConflict returns a structured validation error when
// --no-signature and --signature-id are both set; they are mutually exclusive.
func validateNoSignatureConflict(noSignature bool, signatureID string) error {
	if noSignature && signatureID != "" {
		return mailValidationParamError("--no-signature", "--no-signature and --signature-id are mutually exclusive")
	}
	return nil
}

// autoResolveSignatureID resolves the default signature ID for the given mailbox/sender.
// isReply=true uses DefaultReplyID (+reply/+reply-all/+forward);
// isReply=false uses DefaultSendID (+send/+draft-create).
// Returns "" on API failure (writes stderr warning) or when no default is configured.
func autoResolveSignatureID(runtime *common.RuntimeContext, mailboxID, senderEmail string, isReply bool) string {
	resp, err := signature.ListAll(runtime, mailboxID)
	if err != nil {
		fmt.Fprintf(runtime.IO().ErrOut,
			"warning: failed to fetch default signature: %v; sending without signature\n", err)
		return ""
	}
	if isReply {
		return signature.DefaultReplyID(resp.Usages, senderEmail)
	}
	return signature.DefaultSendID(resp.Usages, senderEmail)
}

// injectPlainTextSignature appends a plain-text rendering of the signature to a
// plain-text body. The HTML signature (sig.RenderedContent) is converted via
// draftpkg.PlainTextFromHTML; inline images are dropped (plain text has none).
// Returns textBody unchanged when sig is nil.
func injectPlainTextSignature(textBody string, sig *signatureResult) string {
	if sig == nil {
		return textBody
	}
	sigText := strings.TrimRight(draftpkg.PlainTextFromHTML(sig.RenderedContent), "\n")
	if sigText == "" {
		return textBody
	}
	return textBody + "\n\n" + sigText
}

// signatureResult holds the pre-processed signature data ready for HTML injection.
type signatureResult struct {
	ID              string
	RenderedContent string
	Images          []draftpkg.SignatureImage
}

// resolveSignature fetches, interpolates, and optionally downloads images for a signature.
// fromEmail is the --from address (may be an alias); used to match the correct
// sender identity for template interpolation. Pass "" to use the primary address.
//
// userExplicit must be true when the caller obtained signatureID from a user-supplied flag
// (--signature-id); false when the ID was auto-resolved from default usages. When false,
// a "not found" error from the signatures API is treated as graceful degradation (no
// signature) rather than a hard failure — this protects against stale default IDs.
//
// includeImages controls whether inline image attachments are downloaded. Pass false for
// plain-text compose paths to avoid unnecessary network I/O (images are discarded in
// plain-text mode anyway).
func resolveSignature(ctx context.Context, runtime *common.RuntimeContext, mailboxID, signatureID, fromEmail string, userExplicit, includeImages bool) (*signatureResult, error) {
	if signatureID == "" {
		return nil, nil
	}

	sig, err := signature.Get(runtime, mailboxID, signatureID)
	if err != nil {
		if !userExplicit && errs.IsValidation(err) {
			// Stale auto-resolved default signature ID — degrade gracefully instead of
			// blocking the entire send/reply/forward operation.
			fmt.Fprintf(runtime.IO().ErrOut,
				"warning: default signature %q not found in current list; sending without signature\n", signatureID)
			return nil, nil
		}
		return nil, err
	}

	// Resolve sender info for template interpolation.
	lang := resolveLang(runtime)
	senderName, senderEmail := resolveSenderInfo(runtime, mailboxID, fromEmail)
	rendered := signature.InterpolateTemplate(sig, lang, senderName, senderEmail)

	// Download signature inline images only when the compose path needs them.
	// Plain-text paths discard images, so skip the download to avoid unnecessary
	// network I/O (and potential failures from expired pre-signed URLs).
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
