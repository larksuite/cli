// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package transport

import (
	"context"
	"net/http"

	"github.com/larksuite/cli/errs"
	exttransport "github.com/larksuite/cli/extension/transport"
	"github.com/larksuite/cli/internal/core"
)

type requestClassContextKey struct{}
type forcedRequestClassContextKey struct{}

// HTTPPolicyRouter selects an HTTP transport policy from request intent and
// the endpoint catalog. Explicit request intent takes precedence; otherwise
// known platform endpoints use the platform policy and all other URLs use the
// external policy.
type HTTPPolicyRouter struct {
	platform http.RoundTripper
	external http.RoundTripper
}

// RoundTripperDecorator describes a transport layer that can be rebuilt over
// a cloned base transport. Connection-policy helpers use this contract to
// preserve retry, response, and extension layers while safely customizing the
// innermost *http.Transport.
type RoundTripperDecorator interface {
	BaseRoundTripper() http.RoundTripper
	WithBaseRoundTripper(http.RoundTripper) http.RoundTripper
}

// NewHTTPPolicyRouter constructs a router over two policy chains. A nil chain
// falls back to the shared proxy-aware transport. The currently registered
// extension provider is resolved once and applied according to its optional
// ScopedProvider contract.
func NewHTTPPolicyRouter(platform, external http.RoundTripper) *HTTPPolicyRouter {
	if platform == nil {
		platform = Shared()
	}
	if external == nil {
		external = Shared()
	}

	extension := resolveExtension()
	return &HTTPPolicyRouter{
		platform: extension.wrap(platform, exttransport.RequestClassPlatform, true),
		external: extension.wrap(external, exttransport.RequestClassExternal, true),
	}
}

// RoundTrip dispatches the request to its selected policy chain.
func (r *HTTPPolicyRouter) RoundTrip(req *http.Request) (*http.Response, error) {
	if req == nil {
		return nil, errs.NewInternalError(
			errs.SubtypeUnknown,
			"HTTP policy router received a nil request",
		)
	}
	class, err := classifyRequest(req)
	if err != nil {
		return nil, err
	}
	if class == exttransport.RequestClassPlatform {
		return r.platform.RoundTrip(req)
	}
	return r.external.RoundTrip(req)
}

func (r *HTTPPolicyRouter) transportForClass(class exttransport.RequestClass) (http.RoundTripper, bool) {
	switch class {
	case exttransport.RequestClassPlatform:
		return r.platform, true
	case exttransport.RequestClassExternal:
		return r.external, true
	default:
		return nil, false
	}
}

func classifyRequest(req *http.Request) (exttransport.RequestClass, error) {
	if explicit, ok := req.Context().Value(requestClassContextKey{}).(exttransport.RequestClass); ok {
		switch explicit {
		case exttransport.RequestClassPlatform, exttransport.RequestClassExternal:
			return explicit, nil
		default:
			return "", errs.NewInternalError(
				errs.SubtypeUnknown,
				"unsupported HTTP request class %q",
				explicit,
			)
		}
	}
	if core.IsPlatformEndpointURL(req.URL) {
		return exttransport.RequestClassPlatform, nil
	}
	return exttransport.RequestClassExternal, nil
}

// WithRequestClass returns a shallow copy of req with explicit routing intent.
func WithRequestClass(req *http.Request, class exttransport.RequestClass) *http.Request {
	if req == nil {
		return nil
	}
	ctx := context.WithValue(req.Context(), requestClassContextKey{}, class)
	return req.WithContext(ctx)
}

func withForcedRequestClass(req *http.Request, class exttransport.RequestClass) *http.Request {
	if req == nil {
		return nil
	}
	if _, forced := req.Context().Value(forcedRequestClassContextKey{}).(struct{}); forced {
		return req
	}
	ctx := context.WithValue(req.Context(), requestClassContextKey{}, class)
	ctx = context.WithValue(ctx, forcedRequestClassContextKey{}, struct{}{})
	return req.WithContext(ctx)
}

type requestClassTransport struct {
	base  http.RoundTripper
	class exttransport.RequestClass
}

func (t *requestClassTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return t.base.RoundTrip(withForcedRequestClass(req, t.class))
}

// CloneHTTPTransport exposes a structural cloning capability without requiring
// higher-level safety helpers to import this package. The explicit request
// class selects the policy branch that must be rebuilt.
func (t *requestClassTransport) CloneHTTPTransport() (http.RoundTripper, *http.Transport, bool) {
	return CloneHTTPTransportForRequestClass(t.base, t.class)
}

// TransformHTTPTransport clones the selected policy branch and replaces its
// concrete transport in place. Keeping the replacement at the graph leaf is
// important for policies that must observe requests after outer decorators
// have run, such as proxy selection.
func (t *requestClassTransport) TransformHTTPTransport(transform func(*http.Transport) (http.RoundTripper, bool)) (http.RoundTripper, bool) {
	return transformHTTPTransportForRequestClass(t.base, t.class, transform, 0)
}

// ClientForRequestClass clones client and forces all of its requests through a
// specific policy class. The original client is never mutated.
func ClientForRequestClass(client *http.Client, class exttransport.RequestClass) *http.Client {
	if client == nil {
		client = &http.Client{}
	}
	cloned := *client
	base := client.Transport
	if base == nil {
		base = Shared()
	}
	cloned.Transport = &requestClassTransport{base: base, class: class}
	return &cloned
}

// CloneHTTPTransportForRequestClass selects one policy branch, clones its
// innermost *http.Transport, and rebuilds every composable decorator around
// the clone. Callers can customize concrete before using rebuilt. The original
// transport graph is never mutated.
func CloneHTTPTransportForRequestClass(base http.RoundTripper, class exttransport.RequestClass) (rebuilt http.RoundTripper, concrete *http.Transport, ok bool) {
	rebuilt, ok = transformHTTPTransportForRequestClass(base, class, func(cloned *http.Transport) (http.RoundTripper, bool) {
		concrete = cloned
		return cloned, true
	}, 0)
	if !ok {
		return nil, nil, false
	}
	return rebuilt, concrete, true
}

func transformHTTPTransportForRequestClass(
	base http.RoundTripper,
	class exttransport.RequestClass,
	transform func(*http.Transport) (http.RoundTripper, bool),
	depth int,
) (http.RoundTripper, bool) {
	if depth > 32 {
		return nil, false
	}
	if base == nil || transform == nil {
		if transform == nil {
			return nil, false
		}
		base = Shared()
	}

	switch current := base.(type) {
	case *http.Transport:
		cloned := cloneHTTPTransport(current)
		rebuilt, valid := transform(cloned)
		return rebuilt, valid && rebuilt != nil
	case *requestClassTransport:
		return transformHTTPTransportForRequestClass(current.base, class, transform, depth+1)
	case *HTTPPolicyRouter:
		selected, valid := current.transportForClass(class)
		if !valid {
			return nil, false
		}
		return transformHTTPTransportForRequestClass(selected, class, transform, depth+1)
	case RoundTripperDecorator:
		inner := current.BaseRoundTripper()
		if inner == nil || inner == base {
			return nil, false
		}
		rebuiltInner, valid := transformHTTPTransportForRequestClass(inner, class, transform, depth+1)
		if !valid {
			return nil, false
		}
		rebuilt := current.WithBaseRoundTripper(rebuiltInner)
		return rebuilt, rebuilt != nil
	default:
		return nil, false
	}
}

func cloneHTTPTransport(source *http.Transport) *http.Transport {
	cloned := source.Clone()
	// Clone leaves an auto-configured h2 handler on source.
	if cloned.TLSNextProto == nil {
		if _, ok := source.TLSNextProto["h2"]; ok {
			cloned.ForceAttemptHTTP2 = true
		}
	}
	return cloned
}
