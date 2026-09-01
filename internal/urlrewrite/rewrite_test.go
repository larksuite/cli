// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package urlrewrite

import (
	"context"
	"strings"
	"sync"
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

func TestRewriteIdentityPreservesRawURL(t *testing.T) {
	raw := "not a valid URL %2F?x=1+2&x=3"
	withProvider(t, testProvider{rewriter: rewriteFunc(func(string) string { return raw })})

	got := Rewrite(raw)
	if got != raw {
		t.Fatalf("Rewrite() = %q, want exact %q", got, raw)
	}
}

func TestRewriteAcceptsChangedAbsoluteHTTPURL(t *testing.T) {
	const want = "http://mirror.example.test:8080/a%2Fb?x=1+2&x=3#fragment"
	withProvider(t, testProvider{rewriter: rewriteFunc(func(string) string { return want })})

	got := Rewrite("https://source.example.test/path")
	if got != want {
		t.Fatalf("Rewrite() = %q, want %q", got, want)
	}
}

func TestRewriteReturnsExtensionValueVerbatim(t *testing.T) {
	const rewritten = "/extension-owned/value"
	withProvider(t, testProvider{rewriter: rewriteFunc(func(string) string { return rewritten })})

	if got := Rewrite("https://source.example.test/path"); got != rewritten {
		t.Fatalf("Rewrite() = %q, want %q", got, rewritten)
	}
}

func TestResolverRewriteConcurrent(t *testing.T) {
	withProvider(t, testProvider{rewriter: rewriteFunc(func(rawURL string) string {
		return strings.Replace(rawURL, "source.example.test", "mirror.example.test", 1)
	})})

	resolver := Resolve(context.Background())
	const workers = 32
	var group sync.WaitGroup
	group.Add(workers)
	for range workers {
		go func() {
			defer group.Done()
			got := resolver.Rewrite("https://source.example.test/path")
			if got != "https://mirror.example.test/path" {
				t.Errorf("Rewrite() = %q", got)
			}
		}()
	}
	group.Wait()
}

var _ exttransport.Provider = testProvider{}
var _ exttransport.URLRewriterProvider = testProvider{}
