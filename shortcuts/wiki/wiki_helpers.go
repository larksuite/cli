// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package wiki

import (
	"strings"

	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/shortcuts/common"
)

// wikiNodeURL returns the user-facing link for a wiki node. The create/copy
// OpenAPI responses carry a real `url` (undocumented in the server-docs schema
// but present in practice); prefer it so the CLI surfaces the canonical link.
// Fall back to BuildResourceURL synthesis only when the response omits it.
//
// Shared by +node-create and +node-copy, hence kept here rather than in either
// command's file.
func wikiNodeURL(brand core.LarkBrand, node *wikiNodeRecord) string {
	if node == nil {
		return ""
	}
	if u := strings.TrimSpace(node.URL); u != "" {
		return u
	}
	return common.BuildResourceURL(brand, "wiki", node.NodeToken)
}
