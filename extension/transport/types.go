// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package transport

import (
	"context"
	"net/http"
)

// Provider creates Interceptor instances.
// Follows the same API style as extension/credential.Provider and extension/fileio.Provider.
type Provider interface {
	Name() string
	ResolveInterceptor(ctx context.Context) Interceptor
}

// Interceptor defines network-layer customization via a pre/post hook pair.
// The built-in transport chain always executes between PreRoundTrip and the
// returned post function, and cannot be skipped or overridden by the extension.
//
// PreRoundTrip is called before the built-in chain. Use it to add custom
// headers, rewrite the host, or start trace spans. Built-in decorators run
// after this and will override any same-named security headers set here.
// The extension must not replace req.Context() — the middleware restores
// the original context after PreRoundTrip returns.
//
// The returned function (if non-nil) is called after the built-in chain
// completes. Use it for logging, ending trace spans, or recording metrics.
//
// Body note: the middleware Clones the caller's request before invoking the
// interceptor, which copies headers/URL/etc. but shares the underlying
// io.ReadCloser. Extensions that read req.Body are responsible for restoring
// a replayable body (e.g. via req.GetBody) before returning, otherwise the
// built-in chain will see an exhausted stream.
type Interceptor interface {
	PreRoundTrip(req *http.Request) func(resp *http.Response, err error)
}

// AbortableInterceptor is an optional extension of Interceptor that lets an
// extension reject a request before the built-in chain runs. Extensions that
// implement this interface are detected by the built-in middleware via a
// type assertion; both methods must be present, but when an extension
// implements PreRoundTripE the middleware will NOT call PreRoundTrip.
//
// Returning a non-nil error from PreRoundTripE aborts the request: the
// built-in chain is not executed and the middleware returns an *AbortError
// wrapping the reason. The returned post function (if non-nil) is still
// invoked with (nil, reason) so that extensions can unwind any state they
// created in the pre hook (spans, metrics, audit records).
//
// Extensions that only care about the abortable variant can provide a no-op
// PreRoundTrip method alongside PreRoundTripE to satisfy Interceptor.
type AbortableInterceptor interface {
	Interceptor
	PreRoundTripE(req *http.Request) (post func(resp *http.Response, err error), err error)
}
