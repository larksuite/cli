// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package transport

import (
	"context"
	"net/http"

	exttransport "github.com/larksuite/cli/extension/transport"
)

var _ RoundTripperDecorator = (*ExtensionMiddleware)(nil)

type resolvedExtension struct {
	provider    exttransport.Provider
	interceptor exttransport.Interceptor
}

func resolveExtension() *resolvedExtension {
	p := exttransport.GetProvider()
	if p == nil {
		return nil
	}
	interceptor := p.ResolveInterceptor(context.Background())
	if interceptor == nil {
		return nil
	}
	return &resolvedExtension{provider: p, interceptor: interceptor}
}

func (e *resolvedExtension) wrap(base http.RoundTripper, class exttransport.RequestClass, enforceScope bool) http.RoundTripper {
	if base == nil {
		base = Shared()
	}
	if e == nil {
		return base
	}
	if enforceScope {
		if scoped, ok := e.provider.(exttransport.ScopedProvider); ok && !scoped.SupportsRequestClass(class) {
			return base
		}
	}
	return &ExtensionMiddleware{Base: base, Ext: e.interceptor, ExtName: e.provider.Name()}
}

// ExtensionMiddleware wraps the built-in transport chain with extension
// pre/post hooks. The built-in chain always executes unless an
// exttransport.AbortableInterceptor rejects the request.
//
// The original request context is restored after the pre hook to prevent an
// extension from replacing cancellation, deadlines, or built-in values. The
// request is cloned so URL and header mutations do not alter the caller's
// request object. The body remains shared; interceptors that consume it must
// restore it before returning.
type ExtensionMiddleware struct {
	Base    http.RoundTripper
	Ext     exttransport.Interceptor
	ExtName string
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

	var (
		post     func(*http.Response, error)
		abortErr error
	)
	if a, ok := m.Ext.(exttransport.AbortableInterceptor); ok {
		post, abortErr = a.PreRoundTripE(req)
	} else {
		post = m.Ext.PreRoundTrip(req)
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
// extension. With no registered provider or no resolved interceptor, base is
// returned unchanged.
func WrapWithExtension(base http.RoundTripper) http.RoundTripper {
	return resolveExtension().wrap(base, "", false)
}

// WrapWithExtensionForClass wraps base only when the registered provider
// supports class. Providers without the optional ScopedProvider interface keep
// their historical all-request behavior.
func WrapWithExtensionForClass(base http.RoundTripper, class exttransport.RequestClass) http.RoundTripper {
	return resolveExtension().wrap(base, class, true)
}
