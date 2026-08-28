// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package urlrewrite

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/larksuite/cli/errs"
	exttransport "github.com/larksuite/cli/extension/transport"
	"github.com/larksuite/cli/internal/output"
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

			got, err := Rewrite(context.Background(), raw)
			if err != nil {
				t.Fatalf("Rewrite() error = %v", err)
			}
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

	got, err := ResolveProvider(context.Background(), captured).Rewrite("https://source.example.test/path")
	if err != nil {
		t.Fatalf("Rewrite() error = %v", err)
	}
	if got != "https://captured.example.test/path" {
		t.Fatalf("Rewrite() = %q, want URL from captured provider", got)
	}
}

func TestRewriteIdentityPreservesRawURL(t *testing.T) {
	raw := "not a valid URL %2F?x=1+2&x=3"
	withProvider(t, testProvider{rewriter: rewriteFunc(func(string) string { return raw })})

	got, err := Rewrite(context.Background(), raw)
	if err != nil {
		t.Fatalf("Rewrite() error = %v", err)
	}
	if got != raw {
		t.Fatalf("Rewrite() = %q, want exact %q", got, raw)
	}
}

func TestRewriteAcceptsChangedAbsoluteHTTPURL(t *testing.T) {
	const want = "http://mirror.example.test:8080/a%2Fb?x=1+2&x=3#fragment"
	withProvider(t, testProvider{rewriter: rewriteFunc(func(string) string { return want })})

	got, err := Rewrite(context.Background(), "https://source.example.test/path")
	if err != nil {
		t.Fatalf("Rewrite() error = %v", err)
	}
	if got != want {
		t.Fatalf("Rewrite() = %q, want %q", got, want)
	}
}

func TestRewriteRejectsInvalidChangedURL(t *testing.T) {
	for _, rewritten := range []string{
		"",
		"/relative/path",
		"ftp://example.test/path",
		"https://user:password@example.test/path",
		"https://",
		"https://example.test/%zz",
	} {
		t.Run(rewritten, func(t *testing.T) {
			withProvider(t, testProvider{rewriter: rewriteFunc(func(string) string { return rewritten })})

			_, err := Rewrite(context.Background(), "https://source.example.test/path?secret=one")
			var configErr *errs.ConfigError
			if !errors.As(err, &configErr) {
				t.Fatalf("Rewrite() error = %T %v, want *errs.ConfigError", err, err)
			}
			if configErr.Subtype != errs.SubtypeInvalidConfig {
				t.Errorf("subtype = %q, want %q", configErr.Subtype, errs.SubtypeInvalidConfig)
			}
			if configErr.Message != "registered URL rewriter returned an invalid absolute HTTP(S) URL" {
				t.Errorf("message = %q", configErr.Message)
			}
			if configErr.Hint != "check the URL rewrite configuration" {
				t.Errorf("hint = %q", configErr.Hint)
			}
			if got := output.ExitCodeOf(err); got != output.ExitAuth {
				t.Errorf("exit code = %d, want %d", got, output.ExitAuth)
			}
			for _, sensitive := range []string{"source.example.test", "secret=one", rewritten} {
				if sensitive != "" && strings.Contains(err.Error(), sensitive) {
					t.Errorf("error leaked %q: %v", sensitive, err)
				}
			}
		})
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
			got, err := resolver.Rewrite("https://source.example.test/path")
			if err != nil {
				t.Errorf("Rewrite() error = %v", err)
			}
			if got != "https://mirror.example.test/path" {
				t.Errorf("Rewrite() = %q", got)
			}
		}()
	}
	group.Wait()
}

var _ exttransport.Provider = testProvider{}
var _ exttransport.URLRewriterProvider = testProvider{}
