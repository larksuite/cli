// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package urlrewrite resolves and applies the optional URL rewrite extension.
package urlrewrite

import (
	"context"

	exttransport "github.com/larksuite/cli/extension/transport"
)

// Resolver holds the URL rewriter resolved for one caller.
//
// A nil rewriter is an identity resolver. Resolve once when a caller needs to
// apply the same extension to multiple URLs.
type Resolver struct {
	rewriter exttransport.URLRewriter
}

// Resolve resolves the URL rewriter from the registered transport provider.
// Providers that do not implement URLRewriterProvider, and providers that
// return a nil rewriter, produce an identity resolver.
func Resolve(ctx context.Context) *Resolver {
	return ResolveProvider(ctx, exttransport.GetProvider())
}

// ResolveProvider resolves the URL rewriter from p. Callers that have already
// selected a provider should use this function so related extension hooks use
// the same provider instance.
func ResolveProvider(ctx context.Context, provider exttransport.Provider) *Resolver {
	p, ok := provider.(exttransport.URLRewriterProvider)
	if !ok {
		return &Resolver{}
	}
	return &Resolver{rewriter: p.ResolveURLRewriter(ctx)}
}

// Rewrite resolves the registered URL rewriter with a background context and
// applies it to rawURL. Rewriting is a synchronous in-process string mapping;
// callers that already captured a provider can use ResolveProvider instead.
func Rewrite(rawURL string) string {
	return Resolve(context.Background()).Rewrite(rawURL)
}

// Rewrite applies the resolved URL rewriter to rawURL. The extension is trusted
// in-process code and owns the returned value; URL-consuming call sites apply
// their existing parsing and transport behavior.
func (r *Resolver) Rewrite(rawURL string) string {
	if r == nil || r.rewriter == nil {
		return rawURL
	}
	return r.rewriter.RewriteURL(rawURL)
}
