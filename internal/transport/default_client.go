// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package transport

import (
	"context"
	"github.com/larksuite/cli/brand"
	"net/http"
	"net/url"
	"strings"
	"sync"

	larkws "github.com/larksuite/oapi-sdk-go/v3/ws"

	"github.com/larksuite/cli/errs"
	exttransport "github.com/larksuite/cli/extension/transport"
)

type requestMatcher func(*http.Request) bool
type transportPolicyBuilder func(http.RoundTripper) http.RoundTripper

type sdkBootstrapRedirectContextKey struct{}

var (
	// larkws pins this client during package initialization.
	sdkBootstrapHTTPClient = http.DefaultClient
	installDefaultClientMu sync.Mutex
)

// sdkBootstrapTransport applies the platform HTTP policy only to dependency
// bootstrap requests selected by match. Unmatched DefaultClient traffic is
// delegated directly to the previous transport.
type sdkBootstrapTransport struct {
	base                http.RoundTripper
	match               requestMatcher
	buildPlatformPolicy transportPolicyBuilder

	policyMu sync.RWMutex
}

func (t *sdkBootstrapTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if !t.isBootstrapRequest(req) {
		return t.fallbackTransport().RoundTrip(req)
	}

	base := t.base
	if base == nil {
		// Resolve Shared lazily so bridge installation never initializes
		// workspace-scoped proxy state ahead of workspace selection.
		base = Shared()
	}
	buildPlatformPolicy := t.platformPolicyBuilder()
	if buildPlatformPolicy == nil {
		return nil, errs.NewInternalError(
			errs.SubtypeUnknown,
			"SDK bootstrap transport policy is not configured",
		)
	}
	base = buildPlatformPolicy(base)
	if base == nil {
		return nil, errs.NewInternalError(
			errs.SubtypeUnknown,
			"SDK bootstrap transport policy returned a nil transport",
		)
	}

	// Resolve extensions per hop so redirects retain platform policy.
	extended := WrapWithExtensionForClass(base, exttransport.RequestClassPlatform)
	guarded := &sameOriginRedirectTransport{base: extended}
	return guarded.RoundTrip(req)
}

func (t *sdkBootstrapTransport) platformPolicyBuilder() transportPolicyBuilder {
	t.policyMu.RLock()
	defer t.policyMu.RUnlock()
	return t.buildPlatformPolicy
}

func (t *sdkBootstrapTransport) setPlatformPolicyBuilder(build transportPolicyBuilder) {
	t.policyMu.Lock()
	t.buildPlatformPolicy = build
	t.policyMu.Unlock()
}

func (t *sdkBootstrapTransport) isBootstrapRequest(req *http.Request) bool {
	if req == nil {
		return false
	}
	if _, redirected := req.Context().Value(sdkBootstrapRedirectContextKey{}).(struct{}); redirected {
		return true
	}
	return t.match != nil && t.match(req)
}

func (t *sdkBootstrapTransport) fallbackTransport() http.RoundTripper {
	if t.base != nil {
		return t.base
	}
	// Preserve net/http's dynamic nil-Transport fallback.
	return http.DefaultTransport
}

// sameOriginRedirectTransport rejects redirects before net/http can replay a
// bootstrap request to a different logical origin.
type sameOriginRedirectTransport struct {
	base http.RoundTripper
}

func (t *sameOriginRedirectTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.base.RoundTrip(req)
	if err != nil || resp == nil || !isFollowedRedirect(resp.StatusCode) {
		return resp, err
	}

	location := resp.Header.Get("Location")
	if location == "" {
		return resp, nil
	}
	target, parseErr := req.URL.Parse(location)
	if parseErr != nil {
		if resp.Body != nil {
			_ = resp.Body.Close()
		}
		return nil, errs.NewInternalError(
			errs.SubtypeInvalidResponse,
			"platform request returned an invalid redirect location: %v",
			parseErr,
		).WithCause(parseErr)
	}
	if sameOrigin(req.URL, target) {
		return resp, nil
	}

	if resp.Body != nil {
		_ = resp.Body.Close()
	}
	return nil, errs.NewSecurityPolicyError(
		errs.SubtypeAccessDenied,
		"platform bootstrap blocked cross-origin redirect from %q to %q",
		originName(req.URL),
		originName(target),
	)
}

// sdkBootstrapRedirectPolicy preserves the prior hook and marks each redirect hop.
func sdkBootstrapRedirectPolicy(
	match requestMatcher,
	previous func(*http.Request, []*http.Request) error,
) func(*http.Request, []*http.Request) error {
	return func(req *http.Request, via []*http.Request) error {
		if previous != nil {
			if err := previous(req, via); err != nil {
				return err
			}
		} else if len(via) >= 10 {
			// Retain net/http's default redirect limit.
			return errs.NewNetworkError(
				errs.SubtypeNetworkTransport,
				"stopped after 10 redirects",
			)
		}

		if req == nil || len(via) == 0 || match == nil || !match(via[0]) {
			return nil
		}

		ctx := context.WithValue(req.Context(), sdkBootstrapRedirectContextKey{}, struct{}{})
		*req = *req.WithContext(ctx)
		return nil
	}
}

func originName(candidate *url.URL) string {
	if candidate == nil {
		return ""
	}
	return strings.ToLower(candidate.Scheme) + "://" + candidate.Host
}

func sameOrigin(left, right *url.URL) bool {
	if left == nil || right == nil {
		return false
	}
	return strings.EqualFold(left.Scheme, right.Scheme) &&
		strings.EqualFold(left.Hostname(), right.Hostname()) &&
		originPort(left) == originPort(right)
}

func originPort(candidate *url.URL) string {
	if port := candidate.Port(); port != "" {
		return port
	}
	switch strings.ToLower(candidate.Scheme) {
	case "http":
		return "80"
	case "https":
		return "443"
	default:
		return ""
	}
}

func isFollowedRedirect(status int) bool {
	switch status {
	case http.StatusMovedPermanently,
		http.StatusFound,
		http.StatusSeeOther,
		http.StatusTemporaryRedirect,
		http.StatusPermanentRedirect:
		return true
	default:
		return false
	}
}

// InstallSDKTransportBridge wraps larkws's captured HTTP bootstrap client. All
// requests through that client hit the bridge, but only matched bootstrap
// traffic uses platform policy. The SDK owns the subsequent WebSocket dial,
// which does not use this net/http transport.
func InstallSDKTransportBridge(buildPlatformPolicy func(http.RoundTripper) http.RoundTripper) {
	installDefaultClientMu.Lock()
	defer installDefaultClientMu.Unlock()
	installSDKTransportBridge(
		sdkBootstrapHTTPClient,
		isSDKWebSocketBootstrapRequest,
		buildPlatformPolicy,
	)
}

func isSDKWebSocketBootstrapRequest(req *http.Request) bool {
	return req != nil &&
		req.Method == http.MethodPost &&
		brand.IsPlatformEndpointURL(req.URL) &&
		req.URL.Path == larkws.GenEndpointUri
}

func installSDKTransportBridge(
	client *http.Client,
	match requestMatcher,
	buildPlatformPolicy transportPolicyBuilder,
) {
	if client == nil {
		return
	}
	if existing, ok := client.Transport.(*sdkBootstrapTransport); ok {
		existing.setPlatformPolicyBuilder(buildPlatformPolicy)
		return
	}
	base := client.Transport
	previousRedirect := client.CheckRedirect
	client.Transport = &sdkBootstrapTransport{
		base:                base,
		match:               match,
		buildPlatformPolicy: buildPlatformPolicy,
	}
	client.CheckRedirect = sdkBootstrapRedirectPolicy(match, previousRedirect)
}
