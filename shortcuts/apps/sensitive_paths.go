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

// hasParentAnchoredCredentialPair scans a "/"-delimited path for the
// cloud-SDK matchers that depend on a conventional parent dir:
// .aws/credentials, .docker/config.json, .kube/config. The leaf-name
// matchers (.env / .npmrc / ...) intentionally do NOT run here, so callers
// can probe a path that includes surrounding root context without risking
// a leaf-rule false-positive on the context segment itself (e.g. a literal
// ".env" directory somewhere in --path's ancestry).
func hasParentAnchoredCredentialPair(path string) bool {
	parts := strings.Split(path, "/")
	for i := 1; i < len(parts); i++ {
		switch parts[i-1] {
		case ".aws":
			if parts[i] == "credentials" {
				return true
			}
		case ".docker":
			if parts[i] == "config.json" {
				return true
			}
		case ".kube":
			if parts[i] == "config" {
				return true
			}
		}
	}
	return false
}

// isSensitiveCandidate is the call-site wrapper used by +html-publish.
//
// Two passes:
//
//  1. Scan RelPath with the full matcher (isSensitiveRelPath). Handles the
//     common in-tree case (e.g. ./site/.env, ./dist/.docker/config.json).
//  2. Re-probe at the boundary between rootPath and the candidate, using
//     ONLY hasParentAnchoredCredentialPair. walker strips the root segment
//     via filepath.Rel, so when --path is itself the conventional parent
//     dir (e.g. ./.aws) RelPath comes back as a bare "credentials" and
//     step 1 has no parent to anchor on. Re-prepending the root's basename
//     — or, for the single-file form, the parent dir's basename of
//     rootPath — exposes the missing segment. Leaf matchers are NOT re-run
//     in this pass, so an ancestor like /home/alice/.env/dist can't
//     false-positive every file beneath it just because ".env" appears in
//     the root context.
//
// Pure string-level reasoning over rootPath — no filesystem access, no
// reliance on cwd — so it composes with the project's fileio sandbox and
// stays inside the shortcuts-layer constraint against direct fs lookups.
func isSensitiveCandidate(rootPath string, c htmlPublishCandidate) bool {
	if isSensitiveRelPath(c.RelPath) {
		return true
	}
	for _, ctx := range []string{filepath.Base(rootPath), filepath.Base(filepath.Dir(rootPath))} {
		switch ctx {
		case "", ".", "..", "/":
			continue
		}
		if hasParentAnchoredCredentialPair(filepath.ToSlash(filepath.Join(ctx, c.RelPath))) {
			return true
		}
	}
	return false
}
