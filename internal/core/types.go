// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package core

import (
	"net/url"
	"strings"
)

// LarkBrand represents the Lark platform brand.
// "feishu" targets China-mainland, "lark" targets international.
// ParseBrand and ResolveEndpoints map unrecognized values to BrandFeishu.
type LarkBrand string

const (
	BrandFeishu LarkBrand = "feishu"
	BrandLark   LarkBrand = "lark"
)

// ParseBrand normalizes a brand string (case-insensitive, whitespace-tolerant);
// anything other than "lark" normalizes to BrandFeishu.
func ParseBrand(value string) LarkBrand {
	if strings.ToLower(strings.TrimSpace(value)) == "lark" {
		return BrandLark
	}
	return BrandFeishu
}

// ProfileSource identifies which input channel selected the invocation's
// profile. Errors and status output use it to point at the thing the user
// must actually fix: an argv flag they just typed, an environment variable
// that may have been exported long ago, or the persisted config default.
type ProfileSource uint8

const (
	ProfileFromConfig      ProfileSource = iota // no explicit selector; persisted currentApp applies
	ProfileFromFlag                             // --profile on this invocation (including --profile=)
	ProfileFromEnvironment                      // LARKSUITE_CLI_PROFILE
)

// OAuthTokenV3Path is the unified OAuth 2.0 Token Endpoint path on the accounts
// domain. It serves every grant type (client_credentials for TAT,
// authorization_code / device_code / refresh_token for UAT) and replaces the
// legacy per-token endpoints (e.g. /open-apis/auth/v3/tenant_access_token/internal).
const OAuthTokenV3Path = "/oauth/v3/token"

// Endpoints holds resolved endpoint URLs for different Lark services.
type Endpoints struct {
	Open     string // e.g. "https://open.feishu.cn"
	Accounts string // e.g. "https://accounts.feishu.cn"
	MCP      string // e.g. "https://mcp.feishu.cn"
	AppLink  string // e.g. "https://applink.feishu.cn"
}

// ResolveEndpoints resolves endpoint URLs for the brand, normalizing its
// input so stored values with unusual casing still resolve correctly.
func ResolveEndpoints(brand LarkBrand) Endpoints {
	switch ParseBrand(string(brand)) {
	case BrandLark:
		return Endpoints{
			Open:     "https://open.larksuite.com",
			Accounts: "https://accounts.larksuite.com",
			MCP:      "https://mcp.larksuite.com",
			AppLink:  "https://applink.larksuite.com",
		}
	default:
		return Endpoints{
			Open:     "https://open.feishu.cn",
			Accounts: "https://accounts.feishu.cn",
			MCP:      "https://mcp.feishu.cn",
			AppLink:  "https://applink.feishu.cn",
		}
	}
}

// ResolveOpenBaseURL returns the Open API base URL for the given brand.
func ResolveOpenBaseURL(brand LarkBrand) string {
	return ResolveEndpoints(brand).Open
}

var platformEndpointHosts = func() map[string]struct{} {
	hosts := make(map[string]struct{})
	for _, brand := range []LarkBrand{BrandFeishu, BrandLark} {
		endpoints := ResolveEndpoints(brand)
		for _, rawURL := range []string{endpoints.Open, endpoints.Accounts, endpoints.MCP, endpoints.AppLink} {
			parsed, err := url.Parse(rawURL)
			if err == nil && parsed.Hostname() != "" {
				hosts[strings.ToLower(parsed.Hostname())] = struct{}{}
			}
		}
	}
	return hosts
}()

// IsPlatformEndpointHost reports whether hostname exactly matches one of the
// endpoint hosts produced by ResolveEndpoints. It intentionally does not use a
// suffix match: lookalike external domains must never enter the platform
// transport extension.
func IsPlatformEndpointHost(hostname string) bool {
	_, ok := platformEndpointHosts[strings.ToLower(hostname)]
	return ok
}

// IsPlatformEndpointURL reports whether candidate uses a secure origin for a
// configured platform endpoint. Non-TLS and non-standard-port lookalikes are
// excluded even when their hostname matches.
func IsPlatformEndpointURL(candidate *url.URL) bool {
	if candidate == nil || !strings.EqualFold(candidate.Scheme, "https") {
		return false
	}
	if port := candidate.Port(); port != "" && port != "443" {
		return false
	}
	return IsPlatformEndpointHost(candidate.Hostname())
}
