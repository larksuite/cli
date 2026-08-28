// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package urlrewrite resolves and applies the optional URL rewrite extension.
package urlrewrite

import (
	"context"
	"net/url"

	"github.com/larksuite/cli/errs"
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

// Rewrite resolves the registered URL rewriter and applies it to rawURL.
func Rewrite(ctx context.Context, rawURL string) (string, error) {
	return Resolve(ctx).Rewrite(rawURL)
}

// Rewrite applies the resolved URL rewriter to rawURL. Identity results are
// returned verbatim. Changed values must be absolute HTTP(S) URLs without
// userinfo.
func (r *Resolver) Rewrite(rawURL string) (string, error) {
	if r == nil || r.rewriter == nil {
		return rawURL, nil
	}

	rewritten := r.rewriter.RewriteURL(rawURL)
	if rewritten == rawURL {
		return rawURL, nil
	}
	if !validURL(rewritten) {
		return "", invalidRewriteError()
	}
	return rewritten, nil
}

func validURL(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	return u.IsAbs() &&
		(u.Scheme == "http" || u.Scheme == "https") &&
		u.Host != "" &&
		u.User == nil
}

func invalidRewriteError() *errs.ConfigError {
	return errs.NewConfigError(
		errs.SubtypeInvalidConfig,
		"registered URL rewriter returned an invalid absolute HTTP(S) URL",
	).WithHint("check the URL rewrite configuration")
}
