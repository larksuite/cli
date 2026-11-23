// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package urlrewrite installs URL rewriters for tests.
package urlrewrite

import (
	"context"
	"testing"

	exttransport "github.com/larksuite/cli/extension/transport"
)

type provider struct {
	rewriter rewriteFunc
}

func (provider) Name() string { return "test-url-rewrite" }

func (provider) ResolveInterceptor(context.Context) exttransport.Interceptor { return nil }

func (p provider) ResolveURLRewriter(context.Context) exttransport.URLRewriter {
	return p.rewriter
}

type rewriteFunc func(string) string

func (f rewriteFunc) RewriteURL(rawURL string) string { return f(rawURL) }

// Register installs rewrite for the duration of the test. Tests using it must
// not run in parallel because the extension registry is process-wide.
func Register(t *testing.T, rewrite func(string) string) {
	t.Helper()
	previous := exttransport.GetProvider()
	exttransport.Register(provider{rewriter: rewrite})
	t.Cleanup(func() { exttransport.Register(previous) })
}
