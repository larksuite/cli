// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package deploy

import (
	"fmt"
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

// parentAnchoredCredentials are credential files whose own name is too generic
// to match on its own — .docker/config.json and .kube/config would otherwise
// sail through a base-name check as "config.json" and "config". The key is the
// conventional parent directory, the value the file name inside it.
var parentAnchoredCredentials = map[string]map[string]bool{
	".docker": {"config.json": true},
	".kube":   {"config": true},
}

// secretOnlyDirs exist solely to hold secrets, so anything directly inside
// them is treated as a credential regardless of its name.
var secretOnlyDirs = map[string]bool{".ssh": true, ".gnupg": true, ".aws": true}

// isSensitiveRel reports whether a "/"-delimited relative path holds a
// credential. It checks the leaf name and, because some credential files carry
// generic names, also the parent directory each segment sits in.
func isSensitiveRel(rel string) bool {
	parts := strings.Split(rel, "/")
	if isSensitiveName(parts[len(parts)-1]) {
		return true
	}
	for i := 1; i < len(parts); i++ {
		parent := strings.ToLower(parts[i-1])
		name := strings.ToLower(parts[i])
		if secretOnlyDirs[parent] {
			return true
		}
		if names, ok := parentAnchoredCredentials[parent]; ok && names[name] {
			return true
		}
	}
	return false
}

// isSensitiveName reports whether a base file name looks like a credential
// file. The .env family is prefix-matched so .env.local and .env.production
// are caught too — note this also catches a file literally named .env.html,
// which is why the single-file path runs this scan as well.
func isSensitiveName(name string) bool {
	// Matching is case-insensitive throughout: macOS and Windows file systems
	// are case-insensitive, so a file named .ENV or ID_RSA is the same file to
	// the user and must not slip past the scan.
	lower := strings.ToLower(name)
	if lower == ".env" || strings.HasPrefix(lower, ".env.") {
		return true
	}
	if strings.HasSuffix(lower, ".pem") || strings.HasSuffix(lower, ".p12") ||
		strings.HasSuffix(lower, ".pfx") || strings.HasSuffix(lower, ".keystore") {
		return true
	}
	return sensitiveExactNames[lower]
}

const maxListedInError = 10

func joinTruncated(items []string, max int) string {
	if len(items) <= max {
		return strings.Join(items, ", ")
	}
	return strings.Join(items[:max], ", ") + ", ..."
}

// HumanBytes renders a byte count the way the flag help and the docs write
// limits, so an operator can line the two up without doing arithmetic.
func HumanBytes(n int64) string {
	const mib = 1024 * 1024
	if n >= mib {
		return fmt.Sprintf("%.1f MiB", float64(n)/float64(mib))
	}
	if n >= 1024 {
		return fmt.Sprintf("%.1f KiB", float64(n)/1024)
	}
	return fmt.Sprintf("%d B", n)
}

// Guard runs the credential scan and the two pre-pack size caps. It returns the
// waived credential files when allowSensitive is set so callers can surface
// them. Callers must run this in Validate, not DryRun, so that --dry-run also
// exits non-zero on a hit.
func Guard(candidates []Candidate, allowSensitive bool, lim Limits) ([]string, error) {
	var sensitive []string
	for _, c := range candidates {
		if isSensitiveRel(c.RelPath) {
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
			oversize = append(oversize, fmt.Sprintf("%s (%s)", c.RelPath, HumanBytes(c.Size)))
		}
	}
	if len(oversize) > 0 {
		return nil, errs.NewValidationError(errs.SubtypeFailedPrecondition,
			"%d HTML file(s) exceed the %s per-file limit: %s",
			len(oversize), HumanBytes(lim.SingleHTMLBytes), joinTruncated(oversize, maxListedInError)).
			WithHint("split or trim the oversized page(s); the cap applies to each single .html file")
	}
	if total > lim.RawTotalBytes {
		return nil, errs.NewValidationError(errs.SubtypeFailedPrecondition,
			"payload total size %s exceeds the %s limit before packing",
			HumanBytes(total), HumanBytes(lim.RawTotalBytes)).
			WithHint("narrow --dir to the directory that actually holds the site, or drop large assets from it")
	}
	return sensitive, nil
}
