// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package transport

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	exttransport "github.com/larksuite/cli/extension/transport"
	"github.com/larksuite/cli/internal/urlrewrite"
)

var _ RoundTripperDecorator = (*ExtensionMiddleware)(nil)

type resolvedExtension struct {
	provider    exttransport.Provider
	interceptor exttransport.Interceptor
	rewriter    exttransport.URLRewriter
}

func resolveExtension() *resolvedExtension {
	p := exttransport.GetProvider()
	if p == nil {
		return nil
	}

	ctx := context.Background()
	extension := &resolvedExtension{
		provider:    p,
		interceptor: p.ResolveInterceptor(ctx),
		rewriter:    urlrewrite.ResolveProvider(ctx, p),
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
	rewriter := e.rewriter
	if enforceScope && interceptor != nil {
		if scoped, ok := e.provider.(exttransport.ScopedProvider); ok && !scoped.SupportsRequestClass(class) {
			interceptor = nil
		}
	}
	// Automatic network rewriting is limited to resolver-owned platform URLs.
	// CLI-owned external URLs are rewritten explicitly where they are built, so
	// user-provided and pre-signed external requests remain verbatim.
	if class != exttransport.RequestClassPlatform {
		rewriter = nil
	}
	if interceptor == nil && rewriter == nil {
		return base
	}
	return &ExtensionMiddleware{
		Base:     base,
		Ext:      interceptor,
		ExtName:  e.provider.Name(),
		rewriter: rewriter,
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
	rewriter exttransport.URLRewriter
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
		rewritten := m.rewriter.RewriteURL(req.URL.String())
		if rewritten != req.URL.String() {
			rewrittenURL, err := url.Parse(rewritten)
			if err != nil {
				return nil, fmt.Errorf("extension %q rewrote request URL to an invalid value: %w", m.ExtName, err)
			}
			req.URL = rewrittenURL
			req.Host = rewrittenURL.Host
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

// WrapWithExtension wraps base with the currently registered request
// interceptor. Callers that need automatic platform URL rewriting use
// WrapWithExtensionForClass with RequestClassPlatform.
func WrapWithExtension(base http.RoundTripper) http.RoundTripper {
	return resolveExtension().wrap(base, "", false)
}

// WrapWithExtensionForClass applies URL rewriting to resolver-owned platform
// requests and wraps base with the interceptor when the registered provider
// supports class. External URLs are left unchanged here; fixed CLI-owned
// external URLs are rewritten at their construction sites. Providers without
// ScopedProvider keep their historical all-request interceptor behavior.
func WrapWithExtensionForClass(base http.RoundTripper, class exttransport.RequestClass) http.RoundTripper {
	return resolveExtension().wrap(base, class, true)
}
