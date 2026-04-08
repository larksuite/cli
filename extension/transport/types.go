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
type Interceptor interface {
	PreRoundTrip(req *http.Request) func(resp *http.Response, err error)
}
