// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package agents

// PageParams is the pagination request the framework hands every list hook. It
// is the Feishu-OpenAPI cursor model (page_token / page_size) reduced to the two
// fields a provider needs: an opaque cursor and a requested size. The framework
// fills it from the --page-token / --page-size flags before calling a list hook.
type PageParams struct {
	Token string // opaque cursor from a prior response; "" = first page
	Size  int    // requested page size; 0 = provider default
}

// PageInfo is what a list hook returns alongside the page's items. NextToken is
// the opaque cursor the caller echoes back (as PageParams.Token) to fetch the
// following page; an empty NextToken with HasMore=false marks the last page. The
// framework surfaces it as meta.has_more / meta.page_token and, when there is a
// next page, a ready-made "next page" next-action command.
type PageInfo struct {
	NextToken string // opaque cursor for the next page; "" = last page
	HasMore   bool
}
