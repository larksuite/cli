// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package riskcontrol

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/larksuite/cli/internal/core"
	internaltransport "github.com/larksuite/cli/internal/transport"
)

var _ internaltransport.RoundTripperDecorator = (*Transport)(nil)

const (
	HeaderProductModel     = "X-Agent-Device-Type"
	HeaderOSType           = "X-Agent-Os-Type"
	HeaderCredentialSource = "X-Agent-Credential-Source"
)

var restrictedHeaders = [...]string{HeaderProductModel, HeaderOSType, HeaderCredentialSource}

// Transport is the feature's final outbound boundary. It removes caller- or
// extension-supplied signal headers first and writes trusted values only when
// workspace policy enables risk control and the request targets an official
// SDK origin.
type Transport struct {
	next   http.RoundTripper
	source Source
}

// NewTransport creates the final SDK outbound policy boundary. A nil source
// means workspace policy disabled risk control, so all signal injection,
// including credential source, is disabled. Restricted headers are still
// stripped from caller- and extension-supplied requests.
func NewTransport(next http.RoundTripper, source Source) *Transport {
	if next == nil {
		next = internaltransport.Fallback()
	}
	return &Transport{
		next:   next,
		source: source,
	}
}

// BaseRoundTripper exposes the network transport so policy routers can clone
// and rebuild the complete decorator graph without dropping risk control.
func (t *Transport) BaseRoundTripper() http.RoundTripper {
	if t == nil || t.next == nil {
		return internaltransport.Fallback()
	}
	return t.next
}

// WithBaseRoundTripper returns an equivalent risk-control boundary over base.
func (t *Transport) WithBaseRoundTripper(base http.RoundTripper) http.RoundTripper {
	if t == nil {
		return NewTransport(base, nil)
	}
	cloned := *t
	if base == nil {
		base = internaltransport.Fallback()
	}
	cloned.next = base
	return &cloned
}

// RoundTrip implements http.RoundTripper.
func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	if req.Header == nil {
		req.Header = make(http.Header)
	}
	stripRestrictedHeaders(req.Header)

	// A nil source is the caller's disabled/unavailable policy marker. Treat it
	// as the master gate for the complete signal set, not only device collection.
	if t.source != nil && t.routeAllowsSignals(req) {
		if source, ok := core.CredentialSourceFromContext(req.Context()); ok {
			req.Header.Set(HeaderCredentialSource, string(source))
		}
		snapshot := t.source.Snapshot()
		if isSupportedOSType(snapshot.OSType) {
			req.Header.Set(HeaderOSType, string(snapshot.OSType))
		}
		if model := normalizeDeviceModel(snapshot.ProductModel); model != "" {
			req.Header.Set(HeaderProductModel, model)
		}
	}
	return t.next.RoundTrip(req)
}

func isSupportedOSType(value OSType) bool {
	switch value {
	case OSTypeWindows, OSTypeLinux, OSTypeMacOS:
		return true
	default:
		return false
	}
}

func stripRestrictedHeaders(header http.Header) {
	for name := range header {
		for _, restricted := range restrictedHeaders {
			if strings.EqualFold(name, restricted) {
				delete(header, name)
				break
			}
		}
	}
}

type origin struct {
	scheme string
	host   string
	port   string
}

var officialFeishuOrigins = [...]origin{
	apiOrigin(core.ResolveEndpoints(core.BrandFeishu).Open),
	apiOrigin(core.ResolveEndpoints(core.BrandLark).Open),
	apiOrigin(core.ResolveEndpoints(core.BrandFeishu).Accounts),
	apiOrigin(core.ResolveEndpoints(core.BrandLark).Accounts),
	apiOrigin(core.ResolveEndpoints(core.BrandFeishu).MCP),
	apiOrigin(core.ResolveEndpoints(core.BrandLark).MCP),
}

func (t *Transport) routeAllowsSignals(req *http.Request) bool {
	if req == nil || req.URL == nil {
		return false
	}
	return isOfficialFeishuOrigin(originOf(req.URL))
}

func originOf(value *url.URL) origin {
	if value == nil {
		return origin{}
	}
	scheme := strings.ToLower(value.Scheme)
	port := value.Port()
	if port == "" {
		switch scheme {
		case "https":
			port = "443"
		case "http":
			port = "80"
		}
	}
	return origin{scheme: scheme, host: strings.ToLower(value.Hostname()), port: port}
}

func apiOrigin(endpointURL string) origin {
	endpoint, err := url.Parse(endpointURL)
	if err != nil {
		return origin{}
	}
	return originOf(endpoint)
}

func isOfficialFeishuOrigin(candidate origin) bool {
	if candidate.scheme != "https" || candidate.port != "443" {
		return false
	}
	for _, official := range officialFeishuOrigins {
		if candidate == official {
			return true
		}
	}
	return false
}
