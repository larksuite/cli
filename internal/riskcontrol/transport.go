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

const (
	HeaderProductModel = "X-Agent-Device-Type"
	HeaderOSType       = "X-Agent-Os-Type"
)

var restrictedHeaders = [...]string{HeaderProductModel, HeaderOSType}

// Transport is the feature's final outbound boundary. It removes caller- or
// extension-supplied signal headers first and writes trusted values only after
// authorizing an official SDK origin and authentication state.
type Transport struct {
	next   http.RoundTripper
	source Source
}

// NewTransport creates the final SDK outbound policy boundary. A nil source
// disables collection and injection while preserving restricted-header
// stripping for opt-out and extension-credential requests.
func NewTransport(next http.RoundTripper, source Source) *Transport {
	if next == nil {
		next = internaltransport.Fallback()
	}
	return &Transport{
		next:   next,
		source: source,
	}
}

// RoundTrip implements http.RoundTripper.
func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	if req.Header == nil {
		req.Header = make(http.Header)
	}
	stripRestrictedHeaders(req.Header)

	if t.source != nil && t.routeAllowsSignals(req) {
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

// HasAccessToken reports whether an SDK request already carries a Bearer access
// token. It deliberately excludes app credentials used to bootstrap token
// exchange and token headers owned by non-SDK clients.
func HasAccessToken(header http.Header) bool {
	if header == nil {
		return false
	}
	authorization := strings.TrimSpace(header.Get("Authorization"))
	if len(authorization) > len("Bearer ") && strings.EqualFold(authorization[:len("Bearer ")], "Bearer ") {
		return strings.TrimSpace(authorization[len("Bearer "):]) != ""
	}
	return false
}

type origin struct {
	scheme string
	host   string
	port   string
}

var officialOpenAPIOrigins = [...]origin{
	openAPIOrigin(core.BrandFeishu),
	openAPIOrigin(core.BrandLark),
}

func (t *Transport) routeAllowsSignals(req *http.Request) bool {
	if req == nil || req.URL == nil {
		return false
	}
	return HasAccessToken(req.Header) && isOfficialOpenAPIOrigin(originOf(req.URL))
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

func openAPIOrigin(brand core.LarkBrand) origin {
	endpoint, err := url.Parse(core.ResolveEndpoints(brand).Open)
	if err != nil {
		return origin{}
	}
	return originOf(endpoint)
}

func isOfficialOpenAPIOrigin(candidate origin) bool {
	if candidate.scheme != "https" || candidate.port != "443" {
		return false
	}
	for _, official := range officialOpenAPIOrigins {
		if candidate == official {
			return true
		}
	}
	return false
}
