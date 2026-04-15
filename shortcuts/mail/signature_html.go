// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"regexp"
	"strings"

	draftpkg "github.com/larksuite/cli/shortcuts/mail/draft"
)

// signatureWrapperRe matches a <div> element whose class attribute contains
// the signature wrapper class. Same pattern as draft.signatureWrapperRe.
var signatureWrapperRe = regexp.MustCompile(
	`<div\s[^>]*class="[^"]*` + draftpkg.SignatureWrapperClass + `[^"]*"`)

// signatureIDRe extracts the id attribute value from a signature wrapper div.
var signatureIDRe = regexp.MustCompile(
	`<div\s[^>]*id="([^"]*)"[^>]*class="[^"]*` + draftpkg.SignatureWrapperClass)

// Delegate to draft package exported functions.
var buildSignatureHTML = draftpkg.BuildSignatureHTML
var buildSignatureSpacing = draftpkg.SignatureSpacing

// insertSignatureIntoBody inserts a rendered signature (spacing + wrapper div) into
// an HTML body. It removes any existing signature first, then places the new signature
// between the user-authored content and the quote block (if any).
func insertSignatureIntoBody(bodyHTML, sigID, renderedContent string) string {
	// Remove existing signature (if any).
	cleaned := removeSignatureFromHTML(bodyHTML)

	// Split at quote block.
	userContent, quote := draftpkg.SplitAtQuote(cleaned)

	// Build signature block.
	sigBlock := buildSignatureSpacing() + buildSignatureHTML(sigID, renderedContent)

	return userContent + sigBlock + quote
}

// removeSignatureFromHTML delegates to draft.RemoveSignatureHTML.
var removeSignatureFromHTML = draftpkg.RemoveSignatureHTML

// hasSignature returns true if the HTML contains a signature wrapper div.
func hasSignature(html string) bool {
	return signatureWrapperRe.MatchString(html)
}

// extractSignatureID extracts the id attribute from the signature wrapper div.
// Returns empty string if no signature is found or id cannot be extracted.
func extractSignatureID(html string) string {
	matches := signatureIDRe.FindStringSubmatch(html)
	if len(matches) < 2 {
		return ""
	}
	return matches[1]
}

// splitAtSignature splits HTML into three parts: before the signature (including
// any content before the spacing), the signature block (spacing + wrapper div),
// and after the signature (quote block, etc.).
// If no signature is found, returns (html, "", "").
func splitAtSignature(html string) (before, sig, after string) {
	loc := signatureWrapperRe.FindStringIndex(html)
	if loc == nil {
		return html, "", ""
	}

	sigStart := loc[0]
	sigEnd := draftpkg.FindMatchingCloseDiv(html, sigStart)

	// Extend sigStart backward to include preceding spacing divs.
	beforeSig := html[:sigStart]
	spacingLoc := draftpkg.SignatureSpacingRe().FindStringIndex(beforeSig)
	if spacingLoc != nil {
		sigStart = spacingLoc[0]
	}

	return html[:sigStart], html[sigStart:sigEnd], html[sigEnd:]
}

// cidSrcRe matches src="cid:..." in HTML to extract CID references.
var cidSrcRe = regexp.MustCompile(`(?i)src="cid:([^"]+)"`)

// collectSignatureCIDs extracts all CID references from a signature HTML block.
func collectSignatureCIDs(sigHTML string) []string {
	matches := cidSrcRe.FindAllStringSubmatch(sigHTML, -1)
	cids := make([]string, 0, len(matches))
	for _, m := range matches {
		if len(m) >= 2 {
			cids = append(cids, m[1])
		}
	}
	return cids
}

// isCIDReferencedInHTML checks if a CID is still referenced in the given HTML.
func isCIDReferencedInHTML(html, cid string) bool {
	return strings.Contains(html, "cid:"+cid)
}
