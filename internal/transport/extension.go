// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package transport

import (
	"context"
	"errors"
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
		platform: class == exttransport.RequestClassPlatform,
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
	platform bool
}

type extensionPlatformKey struct{}

// IsExtensionPlatformRequest reports routing intent captured before extension
// hooks change the destination. External traffic never receives this marker.
func IsExtensionPlatformRequest(req *http.Request) bool {
	platform, _ := req.Context().Value(extensionPlatformKey{}).(bool)
	return platform
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
	origCtx = context.WithValue(origCtx, extensionPlatformKey{}, m.platform)
	logicalURL := req.URL.String()
	req = req.Clone(origCtx)
	if m.rewriter != nil {
		rewritten := m.rewriter.RewriteURL(req.URL.String())
		if rewritten != req.URL.String() {
			rewrittenURL, err := url.Parse(rewritten)
			if err != nil {
				if req.Body != nil {
					_ = req.Body.Close()
				}
				// Parse errors contain the full input, which may include credentials.
				return nil, errs.NewNetworkError(errs.SubtypeNetworkTransport,
					"extension %q rewrote request URL to an invalid value", m.ExtName).WithCause(err)
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
	effectiveURL := ""
	if req.URL != nil && req.URL.String() != logicalURL {
		safeURL := *req.URL
		safeURL.User = nil
		safeURL.RawQuery = ""
		safeURL.ForceQuery = false
		safeURL.Fragment = ""
		effectiveURL = safeURL.String()
	}
	resp, err := m.BaseRoundTripper().RoundTrip(req)
	if post != nil {
		post(resp, err)
	}
	if err != nil && effectiveURL != "" {
		err = &effectiveRequestError{cause: err, url: effectiveURL}
	}
	return resp, err
}

// Keep the effective address on the cause: net/http adds its own url.Error
// using the caller's original URL after RoundTrip returns.
type effectiveRequestError struct {
	cause error
	url   string
}

var _ urlrewrite.RequestError = (*effectiveRequestError)(nil)

func (e *effectiveRequestError) Error() string               { return e.cause.Error() }
func (e *effectiveRequestError) Unwrap() error               { return e.cause }
func (e *effectiveRequestError) EffectiveRequestURL() string { return e.url }

func (e *effectiveRequestError) Timeout() bool {
	var timeout interface{ Timeout() bool }
	return errors.As(e.cause, &timeout) && timeout.Timeout()
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
