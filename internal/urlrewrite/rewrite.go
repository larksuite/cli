// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package urlrewrite resolves and applies the optional URL rewrite extension.
package urlrewrite

import (
	"context"

	exttransport "github.com/larksuite/cli/extension/transport"
)

// ResolveProvider resolves the URL rewriter from provider. Providers that do
// not implement URLRewriterProvider, and providers that return a nil rewriter,
// resolve to nil. Callers that have already selected a provider should use
// this function so related extension hooks use the same provider instance.
func ResolveProvider(ctx context.Context, provider exttransport.Provider) exttransport.URLRewriter {
	p, ok := provider.(exttransport.URLRewriterProvider)
	if !ok {
		return nil
	}
	return p.ResolveURLRewriter(ctx)
}

// Rewrite applies the registered URL rewriter to rawURL, returning rawURL
// unchanged when no rewriter is registered. Rewriting is a synchronous
// in-process string mapping; the extension is trusted in-process code and
// owns the returned value, so URL-consuming call sites apply their existing
// parsing and transport behavior.
func Rewrite(rawURL string) string {
	rewriter := ResolveProvider(context.Background(), exttransport.GetProvider())
	if rewriter == nil {
		return rawURL
	}
	return rewriter.RewriteURL(rawURL)
}
