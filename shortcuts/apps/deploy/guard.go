// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package deploy

import (
	"path/filepath"
	"strings"

	"github.com/larksuite/cli/errs"
)

// Limits caps the payload. These values are deliberately independent of the
// +html-publish limits (10 MiB per file / 20 MiB packed) and must not be
// merged with them in later refactors.
type Limits struct {
	SingleHTMLBytes int64
	RawTotalBytes   int64
	ZipBytes        int64
}

// DefaultLimits returns the bare-HTML publish caps: 20 MiB per .html file,
// 200 MiB of raw bytes before packing and 50 MiB for the packed zip. The raw
// cap exists because the zip is assembled fully in memory — without it a huge
// directory exhausts memory before the packed-size check can fire.
func DefaultLimits() Limits {
	return Limits{
		SingleHTMLBytes: 20 * 1024 * 1024,
		RawTotalBytes:   200 * 1024 * 1024,
		ZipBytes:        50 * 1024 * 1024,
	}
}

// sensitiveExactNames are file names that must never reach a published payload.
var sensitiveExactNames = map[string]bool{
	".npmrc": true, ".netrc": true, ".pypirc": true, ".git-credentials": true,
	"id_rsa": true, "id_dsa": true, "id_ecdsa": true, "id_ed25519": true,
	"credentials": true, "service-account.json": true,
}

// isSensitiveName reports whether a base file name looks like a credential
// file. The .env family is prefix-matched so .env.local and .env.production
// are caught too — note this also catches a file literally named .env.html,
// which is why the single-file path runs this scan as well.
func isSensitiveName(name string) bool {
	if name == ".env" || strings.HasPrefix(name, ".env.") {
		return true
	}
	if strings.HasSuffix(name, ".pem") || strings.HasSuffix(name, ".p12") ||
		strings.HasSuffix(name, ".pfx") || strings.HasSuffix(name, ".keystore") {
		return true
	}
	return sensitiveExactNames[strings.ToLower(name)]
}

const maxListedInError = 10

func joinTruncated(items []string, max int) string {
	if len(items) <= max {
		return strings.Join(items, ", ")
	}
	return strings.Join(items[:max], ", ") + ", ..."
}

// Guard runs the credential scan and the two pre-pack size caps. It returns the
// waived credential files when allowSensitive is set so callers can surface
// them. Callers must run this in Validate, not DryRun, so that --dry-run also
// exits non-zero on a hit.
func Guard(candidates []Candidate, allowSensitive bool, lim Limits) ([]string, error) {
	var sensitive []string
	for _, c := range candidates {
		if isSensitiveName(filepath.Base(c.RelPath)) {
			sensitive = append(sensitive, c.RelPath)
		}
	}
	if len(sensitive) > 0 && !allowSensitive {
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument,
			"the payload contains %d credential file(s) that should not be published: %s",
			len(sensitive), joinTruncated(sensitive, maxListedInError)).
			WithHint("remove them from the payload, or pass --allow-sensitive if shipping them is intentional")
	}

	var oversize []string
	var total int64
	for _, c := range candidates {
		total += c.Size
		if strings.EqualFold(filepath.Ext(c.RelPath), ".html") && c.Size > lim.SingleHTMLBytes {
			oversize = append(oversize, c.RelPath)
		}
	}
	if len(oversize) > 0 {
		return nil, errs.NewValidationError(errs.SubtypeFailedPrecondition,
			"%d HTML file(s) exceed the %d bytes per-file limit: %s",
			len(oversize), lim.SingleHTMLBytes, joinTruncated(oversize, maxListedInError))
	}
	if total > lim.RawTotalBytes {
		return nil, errs.NewValidationError(errs.SubtypeFailedPrecondition,
			"payload total size %d bytes exceeds the %d bytes limit before packing", total, lim.RawTotalBytes).
			WithHint("narrow the payload to the directory that actually holds the site")
	}
	return sensitive, nil
}
