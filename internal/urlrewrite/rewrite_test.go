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

func (testProvider) Name() string                                                  { return "test" }
func (testProvider) ResolveInterceptor(context.Context) exttransport.Interceptor   { return nil }
func (p testProvider) ResolveURLRewriter(context.Context) exttransport.URLRewriter { return p.rewriter }

type rewriteFunc func(string) string

func (f rewriteFunc) RewriteURL(rawURL string) string { return f(rawURL) }

func TestResolveProvider(t *testing.T) {
	const raw = "https://example.test/a%2Fb?x=1+2"
	for _, provider := range []exttransport.Provider{nil, testProvider{}} {
		if got := ResolveProvider(context.Background(), provider).Rewrite(raw); got != raw {
			t.Fatalf("identity Rewrite() = %q, want %q", got, raw)
		}
	}

	provider := testProvider{rewriter: rewriteFunc(func(string) string { return "/rewritten" })}
	if got := ResolveProvider(context.Background(), provider).Rewrite(raw); got != "/rewritten" {
		t.Fatalf("Rewrite() = %q, want extension value", got)
	}
}

func TestRewriteUsesRegisteredProvider(t *testing.T) {
	const rewritten = "/extension-owned/value"
	previous := exttransport.GetProvider()
	exttransport.Register(testProvider{rewriter: rewriteFunc(func(string) string { return rewritten })})
	t.Cleanup(func() { exttransport.Register(previous) })

	if got := Rewrite("https://source.example.test/path"); got != rewritten {
		t.Fatalf("Rewrite() = %q, want %q", got, rewritten)
	}
}
