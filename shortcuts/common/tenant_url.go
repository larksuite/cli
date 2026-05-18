// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"strings"
)

// FetchTenantResourceURL tries to resolve a tenant-specific URL for a resource
// via the drive meta batch_query API (with_url=true). If the API succeeds and
// returns a non-empty URL, that URL is returned. Otherwise it falls back to
// BuildResourceURL using the brand-standard host.
//
// This is the preferred way to obtain resource URLs because batch_query
// returns the tenant's actual vanity domain (e.g. example.feishu.cn) rather
// than the generic www.feishu.cn / www.larksuite.com hosts, which may not
// redirect correctly for private-deployment tenants.
//
// The kind parameter must be a value BuildResourceURL recognizes (docx, sheet,
// wiki, file, folder, slides, bitable, mindnote). doc_type for the meta API
// is derived from kind automatically.
//
// Returns "" only when both the meta lookup and the brand fallback fail
// (typically because token is empty).
func FetchTenantResourceURL(runtime *RuntimeContext, kind, token string) string {
	token = strings.TrimSpace(token)
	if token == "" {
		return ""
	}

	docType := kindToDocType(kind)
	if docType != "" {
		if metaData, err := runtime.CallAPI(
			"POST",
			"/open-apis/drive/v1/metas/batch_query",
			nil,
			map[string]interface{}{
				"request_docs": []map[string]interface{}{
					{
						"doc_token": token,
						"doc_type":  docType,
					},
				},
				"with_url": true,
			},
		); err == nil {
			metas := GetSlice(metaData, "metas")
			if len(metas) > 0 {
				if meta, ok := metas[0].(map[string]interface{}); ok {
					if url := strings.TrimSpace(GetString(meta, "url")); url != "" {
						return url
					}
				}
			}
		}
	}

	return BuildResourceURL(runtime.Config.Brand, kind, token)
}

// kindToDocType maps a BuildResourceURL kind to the doc_type value used by
// the drive meta batch_query API. Returns "" for kinds that have no
// corresponding drive meta type (e.g. unknown kinds).
func kindToDocType(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "docx", "doc":
		return "docx"
	case "sheet":
		return "sheet"
	case "bitable":
		return "bitable"
	case "wiki":
		return "wiki"
	case "file":
		return "file"
	case "folder":
		return "folder"
	case "mindnote":
		return "mindnote"
	case "slides":
		return "slides"
	default:
		return ""
	}
}
