// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package urlrewrite

import (
	"context"
	"testing"

	exttransport "github.com/larksuite/cli/extension/transport"
	testurlrewrite "github.com/larksuite/cli/internal/testutil/urlrewrite"
)

type testProvider struct {
	rewriter exttransport.URLRewriter
}

func (testProvider) Name() string                                                  { return "test" }
func (testProvider) ResolveInterceptor(context.Context) exttransport.Interceptor   { return nil }
func (p testProvider) ResolveURLRewriter(context.Context) exttransport.URLRewriter { return p.rewriter }

type rewriteFunc func(string) string

func (f rewriteFunc) RewriteURL(rawURL string) string { return f(rawURL) }

func TestResolveProvider(t *testing.T) {
	for _, provider := range []exttransport.Provider{nil, testProvider{}} {
		if got := ResolveProvider(context.Background(), provider); got != nil {
			t.Fatalf("ResolveProvider(%T) = %v, want nil", provider, got)
		}
	}

	got := ResolveProvider(context.Background(), testProvider{rewriter: rewriteFunc(func(string) string { return "/rewritten" })})
	if got == nil || got.RewriteURL("https://example.test/x") != "/rewritten" {
		t.Fatalf("ResolveProvider() = %v, want the provider's rewriter", got)
	}
}

func TestRewriteUsesRegisteredProvider(t *testing.T) {
	const rewritten = "/extension-owned/value"
	testurlrewrite.Register(t, func(string) string { return rewritten })

	if got := Rewrite("https://source.example.test/path"); got != rewritten {
		t.Fatalf("Rewrite() = %q, want %q", got, rewritten)
	}
}

func TestRewriteWithoutProviderIsIdentity(t *testing.T) {
	const raw = "https://example.test/a%2Fb?x=1+2"
	if got := Rewrite(raw); got != raw {
		t.Fatalf("Rewrite() = %q, want %q", got, raw)
	}
}
