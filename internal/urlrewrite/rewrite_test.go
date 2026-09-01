// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package urlrewrite

import (
	"context"
	"testing"

	exttransport "github.com/larksuite/cli/extension/transport"
)

type testProvider struct {
	rewriter exttransport.URLRewriter
}

func (testProvider) Name() string { return "test" }

func (testProvider) ResolveInterceptor(context.Context) exttransport.Interceptor { return nil }

type legacyProvider struct{}

func (legacyProvider) Name() string { return "legacy" }

func (legacyProvider) ResolveInterceptor(context.Context) exttransport.Interceptor { return nil }

type rewriteFunc func(string) string

func (f rewriteFunc) RewriteURL(rawURL string) string { return f(rawURL) }

func (p testProvider) ResolveURLRewriter(context.Context) exttransport.URLRewriter { return p.rewriter }

func withProvider(t *testing.T, provider exttransport.Provider) {
	t.Helper()
	previous := exttransport.GetProvider()
	exttransport.Register(provider)
	t.Cleanup(func() { exttransport.Register(previous) })
}

func TestRewriteIdentityWithoutURLRewriter(t *testing.T) {
	raw := "https://example.test/a%2Fb?x=1+2&x=3"

	for _, tc := range []struct {
		name     string
		provider exttransport.Provider
	}{
		{name: "no provider"},
		{name: "legacy provider", provider: legacyProvider{}},
		{name: "nil rewriter", provider: testProvider{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			withProvider(t, tc.provider)

			got := Rewrite(raw)
			if got != raw {
				t.Fatalf("Rewrite() = %q, want exact %q", got, raw)
			}
		})
	}
}

func TestResolveProviderUsesCapturedProvider(t *testing.T) {
	captured := testProvider{rewriter: rewriteFunc(func(string) string {
		return "https://captured.example.test/path"
	})}
	withProvider(t, testProvider{rewriter: rewriteFunc(func(string) string {
		return "https://registered.example.test/path"
	})})

	got := ResolveProvider(context.Background(), captured).Rewrite("https://source.example.test/path")
	if got != "https://captured.example.test/path" {
		t.Fatalf("Rewrite() = %q, want URL from captured provider", got)
	}
}

func TestRewriteReturnsExtensionValueVerbatim(t *testing.T) {
	const rewritten = "/extension-owned/value"
	withProvider(t, testProvider{rewriter: rewriteFunc(func(string) string { return rewritten })})

	if got := Rewrite("https://source.example.test/path"); got != rewritten {
		t.Fatalf("Rewrite() = %q, want %q", got, rewritten)
	}
}

var _ exttransport.Provider = testProvider{}
var _ exttransport.URLRewriterProvider = testProvider{}
