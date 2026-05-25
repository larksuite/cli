// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import "strings"

// isSensitiveRelPath reports whether a relative path inside the candidate
// manifest looks like something that should not ship to a public-internet
// share URL — secrets, credentials, SCM internals, SSH keys. The check is
// path-element-wise (each "/"-delimited segment is inspected) so secrets
// nested under arbitrary subdirectories are still caught.
//
// Used by +html-publish dry-run to populate a "warnings" field; the
// caller still proceeds (this is advisory, not a hard block) so legit
// edge cases (e.g. a documentation site that has a .env example file
// on purpose) are not gated, but the user/agent sees the list.
func isSensitiveRelPath(rel string) bool {
	if rel == "" {
		return false
	}
	parts := strings.Split(rel, "/")
	for i, p := range parts {
		switch {
		case p == ".git":
			return true
		case p == ".env" || strings.HasPrefix(p, ".env."):
			return true
		case p == ".npmrc" || p == ".netrc":
			return true
		case p == "credentials" || p == "config":
			if i > 0 {
				parent := parts[i-1]
				if parent == ".aws" || parent == ".docker" || parent == ".gcloud" || parent == ".kube" {
					return true
				}
			}
		case strings.HasPrefix(p, "id_rsa") || strings.HasPrefix(p, "id_ed25519") || strings.HasPrefix(p, "id_ecdsa") || strings.HasPrefix(p, "id_dsa"):
			return true
		case strings.HasSuffix(p, ".pem") || strings.HasSuffix(p, ".key"):
			return true
		case strings.HasSuffix(p, ".json") && p == "config.json" && i > 0 && parts[i-1] == ".docker":
			return true
		}
	}
	return false
}
