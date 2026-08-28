// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package transport

import (
	"context"
	"net/http"
	"net/url"

	"github.com/larksuite/cli/errs"
	exttransport "github.com/larksuite/cli/extension/transport"
	"github.com/larksuite/cli/internal/urlrewrite"
)

var _ RoundTripperDecorator = (*ExtensionMiddleware)(nil)

type resolvedExtension struct {
	provider    exttransport.Provider
	interceptor exttransport.Interceptor
	rewriter    *urlrewrite.Resolver
}

func resolveExtension() *resolvedExtension {
	p := exttransport.GetProvider()
	if p == nil {
		return nil
	}

	extension := &resolvedExtension{
		provider:    p,
		interceptor: p.ResolveInterceptor(context.Background()),
	}
	if _, ok := p.(exttransport.URLRewriterProvider); ok {
		extension.rewriter = urlrewrite.ResolveProvider(context.Background(), p)
	}
	if extension.interceptor == nil && extension.rewriter == nil {
		return nil
	}
	return extension
}

func (e *resolvedExtension) wrap(base http.RoundTripper, class exttransport.RequestClass, enforceScope bool) http.RoundTripper {
	if base == nil {
		base = Shared()
	}
	if e == nil {
		return base
	}
	interceptor := e.interceptor
	if enforceScope && interceptor != nil {
		if scoped, ok := e.provider.(exttransport.ScopedProvider); ok && !scoped.SupportsRequestClass(class) {
			interceptor = nil
		}
	}
	if interceptor == nil && e.rewriter == nil {
		return base
	}
	return &ExtensionMiddleware{
		Base:     base,
		Ext:      interceptor,
		ExtName:  e.provider.Name(),
		rewriter: e.rewriter,
	}
}

// ExtensionMiddleware wraps the built-in transport chain with URL rewriting
// and extension pre/post hooks. The built-in chain always executes unless an
// exttransport.AbortableInterceptor rejects the request.
//
// The original request context is restored after the pre hook to prevent an
// extension from replacing cancellation, deadlines, or built-in values. The
// request is cloned so URL and header mutations do not alter the caller's
// request object. The body remains shared; interceptors that consume it must
// restore it before returning.
type ExtensionMiddleware struct {
	Base     http.RoundTripper
	Ext      exttransport.Interceptor
	ExtName  string
	rewriter *urlrewrite.Resolver
}

// BaseRoundTripper returns the wrapped built-in transport chain.
func (m *ExtensionMiddleware) BaseRoundTripper() http.RoundTripper {
	if m.Base == nil {
		return Shared()
	}
	return m.Base
}

// WithBaseRoundTripper clones the middleware over base.
func (m *ExtensionMiddleware) WithBaseRoundTripper(base http.RoundTripper) http.RoundTripper {
	cloned := *m
	cloned.Base = base
	return &cloned
}

// RoundTrip invokes the extension pre hook, the wrapped transport, and then
// the optional post hook. Abortable interceptors can stop the request before
// the wrapped transport is called.
func (m *ExtensionMiddleware) RoundTrip(req *http.Request) (*http.Response, error) {
	origCtx := req.Context()
	req = req.Clone(origCtx)
	if m.rewriter != nil {
		rewritten, err := m.rewriter.Rewrite(req.URL.String())
		if err != nil {
			return nil, err
		}
		if rewritten != req.URL.String() {
			// Resolver validates changed URLs with url.Parse before returning.
			rewrittenURL, err := url.Parse(rewritten)
			if err != nil {
				return nil, errs.NewInternalError(
					errs.SubtypeUnknown,
					"URL rewrite validation returned an unparsable URL",
				)
			}
			req.URL = rewrittenURL
		}
	}

	var (
		post     func(*http.Response, error)
		abortErr error
	)
	if m.Ext != nil {
		if a, ok := m.Ext.(exttransport.AbortableInterceptor); ok {
			post, abortErr = a.PreRoundTripE(req)
		} else {
			post = m.Ext.PreRoundTrip(req)
		}
	}
	if abortErr != nil {
		if post != nil {
			post(nil, abortErr)
		}
		return nil, &exttransport.AbortError{Extension: m.ExtName, Reason: abortErr}
	}

	req = req.WithContext(origCtx)
	resp, err := m.BaseRoundTripper().RoundTrip(req)
	if post != nil {
		post(resp, err)
	}
	return resp, err
}

// WrapWithExtension wraps base with the currently registered transport
// extension. With no registered provider, base is returned unchanged.
func WrapWithExtension(base http.RoundTripper) http.RoundTripper {
	return resolveExtension().wrap(base, "", false)
}

// WrapWithExtensionForClass wraps base only when the registered provider
// supports class. Providers without the optional ScopedProvider interface keep
// their historical all-request behavior.
func WrapWithExtensionForClass(base http.RoundTripper, class exttransport.RequestClass) http.RoundTripper {
	return resolveExtension().wrap(base, class, true)
}
