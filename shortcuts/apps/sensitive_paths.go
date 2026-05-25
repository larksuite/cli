// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"path/filepath"
	"strings"
)

// isSensitiveRelPath reports whether a relative path inside the candidate
// manifest is a well-known env / credential file that should not ship to a
// public-internet share URL. The check is path-element-wise (each
// "/"-delimited segment is inspected) so credential files nested under
// arbitrary subdirectories are still caught.
//
// Used by +html-publish: dry-run AND Execute both block by default when any
// candidate matches. Pass --allow-sensitive to override (legitimate cases:
// a documentation site shipping example credential files on purpose).
//
// Scope is intentionally narrow — only files that conventionally hold API
// tokens or service credentials, not the broader "anything cryptographic"
// surface. SSH private keys, generic *.pem / *.key, and SCM internals are
// out of scope here; if they leak it's a separate problem to address.
func isSensitiveRelPath(rel string) bool {
	if rel == "" {
		return false
	}
	parts := strings.Split(rel, "/")
	for i, p := range parts {
		switch {
		case p == ".env" || strings.HasPrefix(p, ".env."):
			return true
		case p == ".npmrc":
			return true
		case p == ".netrc":
			return true
		case p == ".git-credentials":
			return true
		}
		if i == 0 {
			continue
		}
		parent := parts[i-1]
		switch parent {
		case ".aws":
			if p == "credentials" {
				return true
			}
		case ".docker":
			if p == "config.json" {
				return true
			}
		case ".kube":
			if p == "config" {
				return true
			}
		}
	}
	return false
}

// isSensitiveCandidate is the call-site wrapper used by +html-publish. It
// catches the cloud-SDK matchers (.aws/credentials, .docker/config.json,
// .kube/config) when --path itself is the conventional parent dir, in which
// case walker strips that segment via filepath.Rel and RelPath alone no
// longer carries enough context for isSensitiveRelPath to anchor on.
//
// Strategy: scan RelPath first (covers the common in-tree case and is the
// only path with stable, user-facing semantics); on miss, fall back to the
// absolute filesystem path so the parent segment becomes visible again.
// The element-wise matcher inside isSensitiveRelPath only fires on adjacent
// "parent/file" pairs, so unrelated absolute prefixes (e.g. /Users/alice)
// can't cause false positives.
func isSensitiveCandidate(c htmlPublishCandidate) bool {
	if isSensitiveRelPath(c.RelPath) {
		return true
	}
	abs, err := filepath.Abs(c.AbsPath)
	if err != nil {
		return false
	}
	return isSensitiveRelPath(filepath.ToSlash(abs))
}
