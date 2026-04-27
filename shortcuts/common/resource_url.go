// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"strings"

	"github.com/larksuite/cli/internal/core"
)

// BuildResourceURL returns a brand-standard, user-facing URL for a freshly
// created Lark resource. It is intended as a fallback when the create API does
// not return a URL field (e.g. drive +upload, wiki +node-create) or when the
// returned URL is empty (e.g. degraded MCP responses for docs +create v1).
//
// The returned URL points at the brand's standard host (www.feishu.cn /
// www.larksuite.com), which transparently redirects to the tenant-specific
// domain. It is NOT a guess at the tenant's vanity domain.
//
// Returns "" when token is empty or kind is unrecognized — callers should
// only set the field when the result is non-empty so that "" never overrides
// a real URL the backend already returned.
func BuildResourceURL(brand core.LarkBrand, kind, token string) string {
	token = strings.TrimSpace(token)
	if token == "" {
		return ""
	}

	host := "https://www.feishu.cn"
	if brand == core.BrandLark {
		host = "https://www.larksuite.com"
	}

	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "docx":
		return host + "/docx/" + token
	case "doc":
		return host + "/doc/" + token
	case "sheet":
		return host + "/sheets/" + token
	case "bitable":
		return host + "/base/" + token
	case "wiki":
		return host + "/wiki/" + token
	case "file":
		return host + "/file/" + token
	case "folder":
		return host + "/drive/folder/" + token
	case "mindnote":
		return host + "/mindnote/" + token
	case "slides":
		return host + "/slides/" + token
	default:
		return ""
	}
}
