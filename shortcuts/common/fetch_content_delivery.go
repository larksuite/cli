// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"crypto/sha256"
	"fmt"
	"io"
	"unicode/utf8"

	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/internal/contentartifact"
	"github.com/larksuite/cli/internal/output"
)

const (
	// FetchContentSpillThreshold is the local temporary-file threshold for
	// --full responses. It measures the final UTF-8 body, not the JSON envelope.
	FetchContentSpillThreshold = 24 * 1024
	fetchContentPreviewLimit   = 512
)

// FetchContentFile describes content saved outside stdout. Path is absolute so
// callers can find a temporary file after changing directories.
type FetchContentFile struct {
	Path      string `json:"path"`
	SizeBytes int64  `json:"size_bytes"`
	SHA256    string `json:"sha256"`
	Encoding  string `json:"encoding"`
	Temporary bool   `json:"temporary"`
	Hint      string `json:"hint"`
}

// FetchContentDelivery holds either the inline body or metadata for a saved
// copy. Small and paginated reads keep the existing inline output.
type FetchContentDelivery struct {
	Content    string
	InlineHint string
	File       *FetchContentFile
	Preview    string
}

// Inline reports whether the content should remain in its legacy JSON field.
func (d FetchContentDelivery) Inline() bool { return d.File == nil }

// PrepareFetchContentDelivery scans the complete response before saving the
// fetched body, then returns either inline content or saved-file metadata.
func PrepareFetchContentDelivery(runtime *RuntimeContext, safetyData any, content, contentJQPath string) (FetchContentDelivery, output.ScanResult, error) {
	scan := runtime.ScanOutputForSafety(safetyData)
	if scan.Blocked {
		return FetchContentDelivery{}, scan, scan.BlockErr
	}

	delivery := FetchContentDelivery{Content: content}
	autoSpill := runtime.Bool("full") &&
		runtime.JqExpr == "" && len([]byte(content)) > FetchContentSpillThreshold
	if !autoSpill {
		return delivery, scan, nil
	}
	fallbackHint := fmt.Sprintf(
		"Content remains inline because temporary-file delivery failed and may be truncated. If incomplete, rerun locally with --full --jq '%s' and redirect stdout to a new file; use --page-token only when shell redirection is unavailable.",
		contentJQPath,
	)
	support, ok := runtime.FileIO().(fileio.LocalTemporaryFileSupport)
	if !ok || !support.SupportsLocalTemporaryFiles() {
		delivery.InlineHint = fallbackHint
		return delivery, scan, nil
	}

	body := []byte(content)
	path, size, err := contentartifact.WriteTempMarkdown(body)
	if err != nil {
		delivery.InlineHint = fallbackHint
		return delivery, scan, nil //nolint:nilerr // Temporary-file delivery is optional; preserve the inline content and recovery hint.
	}

	sum := sha256.Sum256(body)
	delivery = FetchContentDelivery{
		File: &FetchContentFile{
			Path:      path,
			SizeBytes: size,
			SHA256:    fmt.Sprintf("%x", sum),
			Encoding:  "utf-8",
			Temporary: true,
			Hint: fmt.Sprintf(
				"Oversized content was saved to temporary file: %s. Consider reading or searching this file locally for follow-up questions before fetching the resource again.",
				path,
			),
		},
		Preview: fetchContentPreview(body),
	}
	return delivery, scan, nil
}

func fetchContentPreview(body []byte) string {
	if len(body) <= fetchContentPreviewLimit {
		return string(body)
	}
	// Reserve space for the ellipsis while keeping the complete preview at or
	// below the byte limit and never splitting a UTF-8 code point.
	end := fetchContentPreviewLimit - len("…")
	for end > 0 && !utf8.Valid(body[:end]) {
		end--
	}
	return string(body[:end]) + "…"
}

// WriteFetchContentPretty prints the body inline or identifies its saved file.
func WriteFetchContentPretty(w io.Writer, delivery FetchContentDelivery) {
	if delivery.Inline() {
		if delivery.InlineHint != "" {
			fmt.Fprintf(w, "Hint: %s\n\n", delivery.InlineHint)
		}
		fmt.Fprintln(w, delivery.Content)
		return
	}
	fmt.Fprintf(w, "Content saved to: %s\n", delivery.File.Path)
	fmt.Fprintf(w, "Size: %d bytes\n", delivery.File.SizeBytes)
	fmt.Fprintf(w, "SHA-256: %s\n", delivery.File.SHA256)
	fmt.Fprintf(w, "Temporary: %t\n", delivery.File.Temporary)
	fmt.Fprintf(w, "Hint: %s\n", delivery.File.Hint)
	if delivery.Preview != "" {
		fmt.Fprintf(w, "\nPreview:\n%s\n", delivery.Preview)
	}
}
